package sim

import (
	"strconv"
	"strings"
	"unicode"
)

// Cloud Logging query language subset:
//   severity>=ERROR  resource.type="cloud_run_revision"  textPayload:"timeout"
//   httpRequest.status>=500  logName:"cloudaudit"  protoPayload.methodName="x"
//   free text, AND / OR / NOT, parentheses, -term.

var severityRank = map[string]int{"DEFAULT": 0, "DEBUG": 100, "INFO": 200, "NOTICE": 300, "WARNING": 400, "ERROR": 500, "CRITICAL": 600, "ALERT": 700, "EMERGENCY": 800}

// MatchFilter evaluates a logging filter against an entry.
func MatchFilter(filter string, e LogEntry) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	p := &lfParser{toks: lfLex(filter), e: e}
	return p.or()
}

func lfLex(s string) []string {
	var toks []string
	i := 0
	for i < len(s) {
		c := rune(s[i])
		if unicode.IsSpace(c) {
			i++
			continue
		}
		if c == '(' || c == ')' {
			toks = append(toks, string(c))
			i++
			continue
		}
		j := i
		inQ := false
		for j < len(s) {
			if s[j] == '"' {
				inQ = !inQ
			} else if !inQ && (unicode.IsSpace(rune(s[j])) || s[j] == '(' || s[j] == ')') {
				break
			}
			j++
		}
		toks = append(toks, s[i:j])
		i = j
	}
	return toks
}

type lfParser struct {
	toks []string
	pos  int
	e    LogEntry
}

func (p *lfParser) peek() string {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return ""
}

func (p *lfParser) or() bool {
	v := p.and()
	for p.peek() == "OR" {
		p.pos++
		r := p.and()
		v = v || r
	}
	return v
}

func (p *lfParser) and() bool {
	v := p.not()
	for {
		t := p.peek()
		if t == "" || t == ")" || t == "OR" {
			return v
		}
		if t == "AND" {
			p.pos++
		}
		r := p.not()
		v = v && r
	}
}

func (p *lfParser) not() bool {
	if p.peek() == "NOT" {
		p.pos++
		return !p.not()
	}
	return p.atom()
}

func (p *lfParser) atom() bool {
	t := p.peek()
	p.pos++
	if t == "(" {
		v := p.or()
		if p.peek() == ")" {
			p.pos++
		}
		return v
	}
	neg := false
	if strings.HasPrefix(t, "-") && len(t) > 1 {
		neg = true
		t = t[1:]
	}
	v := matchTerm(t, p.e)
	if neg {
		return !v
	}
	return v
}

func fieldValue(e LogEntry, field string) (string, bool) {
	switch field {
	case "severity":
		return e.Severity, true
	case "logName":
		return e.LogName, true
	case "resource.type":
		return e.Resource.Type, true
	case "textPayload":
		return e.Text, true
	case "timestamp":
		return e.Timestamp, true
	case "httpRequest.status":
		if e.HTTP != nil {
			return strconv.Itoa(e.HTTP.Status), true
		}
		return "", false
	case "httpRequest.requestUrl":
		if e.HTTP != nil {
			return e.HTTP.URL, true
		}
		return "", false
	}
	if strings.HasPrefix(field, "resource.labels.") {
		v, ok := e.Resource.Labels[strings.TrimPrefix(field, "resource.labels.")]
		return v, ok
	}
	if strings.HasPrefix(field, "labels.") {
		v, ok := e.Labels[strings.TrimPrefix(field, "labels.")]
		return v, ok
	}
	if strings.HasPrefix(field, "protoPayload.") {
		k := strings.TrimPrefix(field, "protoPayload.")
		v, ok := e.Proto[k]
		return v, ok
	}
	if strings.HasPrefix(field, "jsonPayload.") {
		v, ok := e.Labels[strings.TrimPrefix(field, "jsonPayload.")]
		return v, ok
	}
	return "", false
}

func matchTerm(t string, e LogEntry) bool {
	for _, op := range []string{">=", "<=", "!=", "=~", "=", ":", ">", "<"} {
		i := strings.Index(t, op)
		if i <= 0 {
			continue
		}
		field := t[:i]
		want := strings.Trim(t[i+len(op):], `"`)
		got, ok := fieldValue(e, field)
		if !ok {
			return op == "!="
		}
		switch op {
		case "=":
			return strings.EqualFold(got, want)
		case "!=":
			return !strings.EqualFold(got, want)
		case ":", "=~":
			return strings.Contains(strings.ToLower(got), strings.ToLower(want))
		case ">=", "<=", ">", "<":
			return compareLog(field, got, want, op)
		}
	}
	// free text search
	w := strings.ToLower(strings.Trim(t, `"`))
	hay := strings.ToLower(e.Text + " " + e.LogName + " " + e.Resource.Type)
	for _, v := range e.Proto {
		hay += " " + strings.ToLower(v)
	}
	for _, v := range e.Resource.Labels {
		hay += " " + strings.ToLower(v)
	}
	if e.HTTP != nil {
		hay += " " + strings.ToLower(e.HTTP.URL) + " " + strconv.Itoa(e.HTTP.Status)
	}
	return strings.Contains(hay, w)
}

func compareLog(field, got, want, op string) bool {
	var a, b float64
	if field == "severity" {
		a, b = float64(severityRank[strings.ToUpper(got)]), float64(severityRank[strings.ToUpper(want)])
	} else if n1, err1 := strconv.ParseFloat(got, 64); err1 == nil {
		n2, err2 := strconv.ParseFloat(want, 64)
		if err2 != nil {
			return false
		}
		a, b = n1, n2
	} else {
		c := strings.Compare(got, want)
		a, b = float64(c), 0
	}
	switch op {
	case ">=":
		return a >= b
	case "<=":
		return a <= b
	case ">":
		return a > b
	case "<":
		return a < b
	}
	return false
}

// QueryLogs returns entries of a project matching a filter, newest first.
func (s *State) QueryLogs(project, filter string, limit int) []LogEntry {
	var out []LogEntry
	for i := len(s.Logs) - 1; i >= 0; i-- {
		e := s.Logs[i]
		if project != "" && e.Project != project {
			continue
		}
		if MatchFilter(filter, e) {
			out = append(out, e)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out
}
