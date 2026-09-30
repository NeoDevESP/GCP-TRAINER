package tutor

import (
	"regexp"
	"strings"
)

var reHeredoc = regexp.MustCompile(`<<-?\s*['"]?([A-Za-z_][A-Za-z0-9_]*)['"]?`)

// block is one command of a solution script, with the comments above it.
type block struct {
	Text    string
	Comment string
}

// splitScript cuts a shell script into commands: continuation lines, here
// documents, multi-line quoted arguments and for/while/if blocks stay whole.
func splitScript(script string) []block {
	lines := strings.Split(strings.ReplaceAll(script, "\r\n", "\n"), "\n")
	var out []block
	var comment []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "#") {
			comment = append(comment, strings.TrimSpace(strings.TrimPrefix(t, "#")))
			continue
		}
		cur := line
		// continuation lines and unbalanced quotes
		for i+1 < len(lines) && (strings.HasSuffix(strings.TrimRight(cur, " "), "\\") || !quotesBalanced(cur)) {
			i++
			if strings.HasSuffix(strings.TrimRight(cur, " "), "\\") {
				cur = strings.TrimSuffix(strings.TrimRight(cur, " "), "\\") + " " + strings.TrimSpace(lines[i])
			} else {
				cur += "\n" + lines[i]
			}
		}
		// here documents
		if m := reHeredoc.FindStringSubmatch(cur); m != nil {
			for i+1 < len(lines) {
				i++
				cur += "\n" + lines[i]
				if strings.TrimSpace(lines[i]) == m[1] {
					break
				}
			}
		}
		// compound commands
		if w := firstWord(t); w == "for" || w == "while" || w == "until" || w == "if" || w == "case" {
			depth := blockDelta(cur)
			for depth > 0 && i+1 < len(lines) {
				i++
				cur += "\n" + lines[i]
				depth += blockDelta(lines[i])
			}
		}
		out = append(out, block{Text: strings.TrimSpace(cur), Comment: strings.Join(comment, " ")})
		comment = nil
	}
	return out
}

func firstWord(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// blockDelta counts openings minus closings of shell compound commands.
func blockDelta(s string) int {
	d := 0
	for _, part := range regexp.MustCompile(`[;\n]`).Split(s, -1) {
		switch firstWord(strings.TrimSpace(part)) {
		case "for", "while", "until", "if", "case":
			d++
		case "done", "fi", "esac":
			d--
		}
	}
	return d
}

func quotesBalanced(s string) bool {
	var q rune
	esc := false
	for _, r := range s {
		switch {
		case esc:
			esc = false
		case r == '\\' && q != '\'':
			esc = true
		case q == 0 && (r == '\'' || r == '"'):
			q = r
		case r == q:
			q = 0
		}
	}
	return q == 0
}

// words splits a command line like a shell (quotes removed, $(...) kept whole).
func words(s string) []string {
	var out []string
	var b strings.Builder
	var q rune
	paren := 0
	has := false
	flush := func() {
		if has {
			out = append(out, b.String())
		}
		b.Reset()
		has = false
	}
	for _, r := range s {
		switch {
		case q != 0:
			if r == q {
				q = 0
			} else {
				b.WriteRune(r)
			}
		case r == '(' && strings.HasSuffix(b.String(), "$"):
			paren++
			b.WriteRune(r)
		case r == ')' && paren > 0:
			paren--
			b.WriteRune(r)
		case paren > 0:
			b.WriteRune(r)
		case r == '\'' || r == '"':
			q = r
			has = true
		case r == ' ' || r == '\t' || r == '\n':
			flush()
		case r == '|' || r == ';' || r == '>' || r == '&':
			flush()
			return out
		default:
			b.WriteRune(r)
			has = true
		}
	}
	flush()
	return out
}

// parsed is a command split into positional words and flags.
type parsed struct {
	Pos   []string
	Flags map[string]string
}

func parse(ws []string) parsed {
	p := parsed{Flags: map[string]string{}}
	for i := 0; i < len(ws); i++ {
		w := ws[i]
		if w == "--" {
			p.Pos = append(p.Pos, ws[i+1:]...)
			break
		}
		if strings.HasPrefix(w, "--") {
			k, v, ok := strings.Cut(w[2:], "=")
			if !ok && i+1 < len(ws) && !strings.HasPrefix(ws[i+1], "-") && !boolFlag[k] {
				v = ws[i+1]
				i++
			}
			p.Flags[k] = v
			continue
		}
		if strings.HasPrefix(w, "-") && len(w) > 1 {
			continue
		}
		p.Pos = append(p.Pos, w)
	}
	return p
}

var boolFlag = map[string]bool{
	"quiet": true, "async": true, "global": true, "allow-unauthenticated": true, "no-allow-unauthenticated": true,
	"enable-point-in-time-recovery": true, "no-promote": true, "gen2": true, "tunnel-through-iap": true, "use_legacy_sql": true,
	"uniform-bucket-level-access": true, "no-address": true, "enable-auth": true, "delete-protection": true, "enable-drop-protection": true,
	"single-node": true, "preview": true, "trigger-http": true, "internal-ip": true, "recursive": true, "all": true,
}
