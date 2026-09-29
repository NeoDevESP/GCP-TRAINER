package sqlengine

import (
	"fmt"
	"regexp"
	"strings"
)

// Dialect translation: BigQuery GoogleSQL and PostgreSQL statements are
// rewritten to the SQLite equivalent before running. Only the common subset
// used while learning is covered; anything else surfaces as an error.

// BigQueryTypes maps BigQuery column types to SQLite affinities.
func BigQueryType(t string) string {
	switch strings.ToUpper(strings.SplitN(t, "(", 2)[0]) {
	case "INT64", "INTEGER", "INT", "SMALLINT", "BIGINT", "TINYINT", "BYTEINT", "BOOL", "BOOLEAN":
		return "INTEGER"
	case "FLOAT64", "FLOAT", "NUMERIC", "BIGNUMERIC", "DECIMAL", "BIGDECIMAL":
		return "REAL"
	case "BYTES":
		return "BLOB"
	}
	return "TEXT"
}

// IsBool reports whether a BigQuery type is boolean.
func IsBool(t string) bool {
	switch strings.ToUpper(t) {
	case "BOOL", "BOOLEAN":
		return true
	}
	return false
}

var (
	reBacktick   = regexp.MustCompile("`([^`]+)`")
	reRawString  = regexp.MustCompile(`\b[rR]'`)
	reCastTypes  = regexp.MustCompile(`(?i)\bAS\s+(INT64|FLOAT64|NUMERIC|BIGNUMERIC|STRING|BOOL|BOOLEAN|BYTES|DATE|DATETIME|TIMESTAMP)\b\s*\)`)
	reFuncName   = regexp.MustCompile(`(?i)\b([A-Z_][A-Z0-9_]*)\s*\(`)
	reIntervalAr = regexp.MustCompile(`(?is)^\s*INTERVAL\s+(-?\d+)\s+(\w+)\s*$`)
)

// FromBigQuery rewrites a GoogleSQL statement for SQLite. project is the
// default project: table references `project.dataset.table` become
// dataset.table (each dataset is an attached schema). now is the simulated
// current time (RFC 3339).
func FromBigQuery(sqlText, project string, now string) (string, error) {
	s := reRawString.ReplaceAllString(sqlText, "'")
	s = reBacktick.ReplaceAllStringFunc(s, func(m string) string {
		parts := strings.Split(strings.ReplaceAll(strings.Trim(m, "`"), ":", "."), ".")
		if len(parts) == 3 {
			parts = parts[1:]
		}
		for i, p := range parts {
			parts[i] = Quote(p)
		}
		return strings.Join(parts, ".")
	})
	// Unquoted project.dataset.table references.
	if project != "" {
		re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(project) + `[.:]([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)`)
		s = re.ReplaceAllString(s, "$1.$2")
	}
	if regexp.MustCompile(`(?i)\bQUALIFY\b|\bSELECT\s+(AS\s+)?STRUCT\b|\bUNNEST\s*\(|\bEXCEPT\s*\(|\bREPLACE\s*\(\s*\w+\s+AS\b|\bMERGE\b`).MatchString(s) {
		return "", fmt.Errorf("this GoogleSQL construct (QUALIFY, STRUCT, UNNEST, SELECT * EXCEPT/REPLACE, MERGE) is not supported by the simulator")
	}
	s = reCastTypes.ReplaceAllStringFunc(s, func(m string) string {
		t := reCastTypes.FindStringSubmatch(m)[1]
		switch strings.ToUpper(t) {
		case "DATE", "DATETIME", "TIMESTAMP", "STRING":
			return "AS TEXT)"
		}
		return "AS " + BigQueryType(t) + ")"
	})
	date := now
	if len(date) >= 10 {
		date = date[:10]
	}
	ts := strings.Replace(strings.TrimSuffix(now, "Z"), "T", " ", 1)
	var err error
	s, err = rewriteCalls(s, func(name string, args []string) (string, bool, error) {
		up := strings.ToUpper(name)
		switch up {
		case "CURRENT_DATE":
			return "'" + date + "'", true, nil
		case "CURRENT_TIMESTAMP", "CURRENT_DATETIME", "NOW":
			return "'" + ts + "'", true, nil
		case "DATE_SUB", "DATE_ADD", "TIMESTAMP_SUB", "TIMESTAMP_ADD", "DATETIME_SUB", "DATETIME_ADD":
			if len(args) != 2 {
				return "", false, fmt.Errorf("%s expects 2 arguments", up)
			}
			m := reIntervalAr.FindStringSubmatch(args[1])
			if m == nil {
				return "", false, fmt.Errorf("%s expects INTERVAL n UNIT", up)
			}
			n := m[1]
			if strings.HasSuffix(up, "_SUB") {
				if strings.HasPrefix(n, "-") {
					n = n[1:]
				} else {
					n = "-" + n
				}
			} else if !strings.HasPrefix(n, "-") {
				n = "+" + n
			}
			unit := strings.ToLower(strings.TrimSuffix(strings.ToUpper(m[2]), "S"))
			if unit == "week" {
				unit, n = "day", fmt.Sprint(atoi(n)*7)
				if !strings.HasPrefix(n, "-") {
					n = "+" + n
				}
			}
			fn := "datetime"
			if strings.HasPrefix(up, "DATE_") {
				fn = "date"
			}
			return fmt.Sprintf("%s(%s, '%s %s')", fn, args[0], n, unit), true, nil
		case "DATE_DIFF", "TIMESTAMP_DIFF", "DATETIME_DIFF":
			if len(args) != 3 {
				return "", false, fmt.Errorf("%s expects 3 arguments", up)
			}
			mult := map[string]string{"DAY": "1", "HOUR": "24", "MINUTE": "1440", "SECOND": "86400", "WEEK": "1.0/7"}[strings.ToUpper(strings.TrimSpace(args[2]))]
			if mult == "" {
				mult = "1"
			}
			return fmt.Sprintf("CAST((julianday(%s) - julianday(%s)) * %s AS INTEGER)", args[0], args[1], mult), true, nil
		case "DATE_TRUNC", "TIMESTAMP_TRUNC", "DATETIME_TRUNC":
			if len(args) != 2 {
				return "", false, fmt.Errorf("%s expects 2 arguments", up)
			}
			switch strings.ToUpper(strings.TrimSpace(args[1])) {
			case "MONTH":
				return fmt.Sprintf("date(%s, 'start of month')", args[0]), true, nil
			case "YEAR":
				return fmt.Sprintf("date(%s, 'start of year')", args[0]), true, nil
			case "WEEK":
				return fmt.Sprintf("date(%s, 'weekday 0', '-7 days')", args[0]), true, nil
			case "HOUR":
				return fmt.Sprintf("strftime('%%Y-%%m-%%d %%H:00:00', %s)", args[0]), true, nil
			}
			return fmt.Sprintf("date(%s)", args[0]), true, nil
		case "FORMAT_DATE", "FORMAT_TIMESTAMP", "FORMAT_DATETIME":
			if len(args) < 2 {
				return "", false, fmt.Errorf("%s expects 2 arguments", up)
			}
			return fmt.Sprintf("strftime(%s, %s)", args[0], args[1]), true, nil
		case "PARSE_DATE":
			if len(args) != 2 {
				return "", false, fmt.Errorf("PARSE_DATE expects 2 arguments")
			}
			return fmt.Sprintf("date(%s)", args[1]), true, nil
		case "TIMESTAMP", "DATETIME":
			return fmt.Sprintf("datetime(%s)", strings.Join(args, ", ")), true, nil
		case "COUNTIF":
			return fmt.Sprintf("SUM(CASE WHEN (%s) THEN 1 ELSE 0 END)", strings.Join(args, ", ")), true, nil
		case "IF":
			return fmt.Sprintf("IIF(%s)", strings.Join(args, ", ")), true, nil
		case "APPROX_COUNT_DISTINCT":
			return fmt.Sprintf("COUNT(DISTINCT %s)", strings.Join(args, ", ")), true, nil
		case "STRING_AGG":
			return fmt.Sprintf("GROUP_CONCAT(%s)", strings.Join(args, ", ")), true, nil
		case "ARRAY_AGG":
			return fmt.Sprintf("json_group_array(%s)", strings.Join(args, ", ")), true, nil
		case "LOGICAL_OR":
			return fmt.Sprintf("MAX(CASE WHEN (%s) THEN 1 ELSE 0 END)", args[0]), true, nil
		case "LOGICAL_AND":
			return fmt.Sprintf("MIN(CASE WHEN (%s) THEN 1 ELSE 0 END)", args[0]), true, nil
		case "EXTRACT":
			if len(args) != 1 {
				return "", false, fmt.Errorf("EXTRACT expects (PART FROM value)")
			}
			m := regexp.MustCompile(`(?is)^\s*(\w+)\s+FROM\s+(.+)$`).FindStringSubmatch(args[0])
			if m == nil {
				return "", false, fmt.Errorf("EXTRACT expects (PART FROM value)")
			}
			f := map[string]string{"YEAR": "%Y", "MONTH": "%m", "DAY": "%d", "HOUR": "%H", "MINUTE": "%M", "SECOND": "%S", "DAYOFWEEK": "%w", "DAYOFYEAR": "%j", "WEEK": "%W"}[strings.ToUpper(m[1])]
			if f == "" {
				return "", false, fmt.Errorf("EXTRACT %s is not supported by the simulator", m[1])
			}
			if strings.ToUpper(m[1]) == "DAYOFWEEK" {
				return fmt.Sprintf("(CAST(strftime('%%w', %s) AS INTEGER) + 1)", m[2]), true, nil
			}
			return fmt.Sprintf("CAST(strftime('%s', %s) AS INTEGER)", f, m[2]), true, nil
		case "LENGTH", "CHAR_LENGTH", "CHARACTER_LENGTH":
			return fmt.Sprintf("length(%s)", strings.Join(args, ", ")), true, nil
		case "SUBSTRING":
			return fmt.Sprintf("substr(%s)", strings.Join(args, ", ")), true, nil
		case "SPLIT":
			return "", false, fmt.Errorf("SPLIT returns an ARRAY, which the simulator does not support")
		}
		return "", false, nil
	})
	if err != nil {
		return "", err
	}
	return s, nil
}

func atoi(s string) int {
	n := 0
	neg := strings.HasPrefix(s, "-")
	for _, r := range strings.TrimLeft(s, "+-") {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	if neg {
		return -n
	}
	return n
}

// rewriteCalls walks the statement and lets fn replace function calls; it
// repeats until nothing changes so nested calls are handled.
func rewriteCalls(s string, fn func(name string, args []string) (string, bool, error)) (string, error) {
	for pass := 0; pass < 20; pass++ {
		changed := false
		var out strings.Builder
		i := 0
		for i < len(s) {
			if q := s[i]; q == '\'' || q == '"' {
				j := i + 1
				for j < len(s) && s[j] != q {
					if s[j] == '\\' {
						j++
					}
					j++
				}
				if j >= len(s) {
					j = len(s) - 1
				}
				out.WriteString(s[i : j+1])
				i = j + 1
				continue
			}
			loc := reFuncName.FindStringSubmatchIndex(s[i:])
			if loc == nil || loc[0] != 0 || (i > 0 && isIdent(s[i-1])) {
				out.WriteByte(s[i])
				i++
				continue
			}
			name := s[i+loc[2] : i+loc[3]]
			open := i + loc[1] - 1
			close := matchParen(s, open)
			if close < 0 {
				out.WriteByte(s[i])
				i++
				continue
			}
			args := splitArgs(s[open+1 : close])
			rep, ok, err := fn(name, args)
			if err != nil {
				return "", err
			}
			if ok {
				out.WriteString(rep)
				changed = true
				i = close + 1
				continue
			}
			out.WriteString(s[i : open+1])
			i = open + 1
		}
		s = out.String()
		if !changed {
			break
		}
	}
	return s, nil
}

func isIdent(b byte) bool {
	return b == '_' || b == '.' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func matchParen(s string, open int) int {
	depth := 0
	var quote byte
	for j := open; j < len(s); j++ {
		c := s[j]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

func splitArgs(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	depth := 0
	var quote byte
	start := 0
	for j := 0; j < len(s); j++ {
		c := s[j]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == ',' && depth == 0:
			out = append(out, strings.TrimSpace(s[start:j]))
			start = j + 1
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

// FromPostgres rewrites a PostgreSQL statement for SQLite.
func FromPostgres(stmt, now string) (string, error) {
	s := stmt
	s = regexp.MustCompile(`(?i)\b(BIG)?SERIAL\s+PRIMARY\s+KEY\b`).ReplaceAllString(s, "INTEGER PRIMARY KEY AUTOINCREMENT")
	s = regexp.MustCompile(`(?i)\b(BIG|SMALL)?SERIAL\b`).ReplaceAllString(s, "INTEGER")
	s = regexp.MustCompile(`(?i)\bGENERATED\s+(ALWAYS|BY\s+DEFAULT)\s+AS\s+IDENTITY\b`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`(?i)::\s*(text|int|integer|bigint|numeric|date|timestamp|timestamptz|varchar|boolean|float|real|json|jsonb)\b`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`(?i)\bILIKE\b`).ReplaceAllString(s, "LIKE")
	s = regexp.MustCompile(`(?i)\bCREATE\s+EXTENSION\b.*`).ReplaceAllString(s, "SELECT 1")
	ts := strings.Replace(strings.TrimSuffix(now, "Z"), "T", " ", 1)
	s = regexp.MustCompile(`(?i)\bNOW\s*\(\s*\)|\bCURRENT_TIMESTAMP\b`).ReplaceAllString(s, "'"+ts+"'")
	if len(now) >= 10 {
		s = regexp.MustCompile(`(?i)\bCURRENT_DATE\b`).ReplaceAllString(s, "'"+now[:10]+"'")
	}
	s = regexp.MustCompile(`(?i)\bTRUE\b`).ReplaceAllString(s, "1")
	s = regexp.MustCompile(`(?i)\bFALSE\b`).ReplaceAllString(s, "0")
	return s, nil
}
