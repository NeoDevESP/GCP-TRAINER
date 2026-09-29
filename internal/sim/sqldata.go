package sim

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sqlengine"
)

// Cloud SQL data: every database of an instance is stored as DDL + rows and
// statements run on the SQL engine with PostgreSQL or MySQL syntax, printed
// like psql or the mysql client.

// IsMySQL reports whether the instance runs MySQL.
func (in *SQLInstance) IsMySQL() bool { return strings.HasPrefix(strings.ToUpper(in.Version), "MYSQL") }

var (
	reMyAutoInc = regexp.MustCompile(`(?i)\b(?:BIG)?INT(?:EGER)?(?:\(\d+\))?\s+(?:UNSIGNED\s+)?(?:NOT\s+NULL\s+)?AUTO_INCREMENT(?:\s+PRIMARY\s+KEY)?`)
	reMyEngine  = regexp.MustCompile(`(?i)\)\s*(ENGINE|DEFAULT\s+CHARSET|CHARSET)\s*=?\s*\w+.*$`)
	reUseDB     = regexp.MustCompile(`(?i)^\s*(?:USE|\\c|\\connect)\s+"?([A-Za-z0-9_-]+)"?\s*$`)
)

// RunSQL runs a script (or a psql/mysql meta command) on a database and
// returns the client output. db may change (USE / \c); the new name is returned.
func (s *State) RunSQL(in *SQLInstance, db, script string) (string, string) {
	if in.Data == nil {
		in.Data = map[string]*sqlengine.DB{}
	}
	my := in.IsMySQL()
	var out []string
	for _, st := range splitClient(script) {
		st = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(st), ";"))
		if st == "" {
			continue
		}
		if m := reUseDB.FindStringSubmatch(st); m != nil {
			if !contains(in.Databases, m[1]) {
				if my {
					out = append(out, fmt.Sprintf("ERROR 1049 (42000): Unknown database '%s'", m[1]))
				} else {
					out = append(out, fmt.Sprintf("connection to server failed: FATAL:  database \"%s\" does not exist", m[1]))
				}
				continue
			}
			db = m[1]
			if my {
				out = append(out, "Database changed")
			} else {
				out = append(out, fmt.Sprintf("You are now connected to database \"%s\".", db))
			}
			continue
		}
		out = append(out, s.runOne(in, db, st, my))
	}
	return strings.Join(out, "\n"), db
}

func splitClient(script string) []string {
	var out []string
	var buf strings.Builder
	flush := func() {
		out = append(out, sqlengine.Split(buf.String())...)
		buf.Reset()
	}
	for _, line := range strings.Split(script, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, `\`) && strings.TrimSpace(buf.String()) == "" {
			out = append(out, t)
			continue
		}
		buf.WriteString(line + "\n")
	}
	flush()
	return out
}

func (s *State) runOne(in *SQLInstance, db, st string, my bool) string {
	l := strings.ToLower(st)
	switch {
	case l == `\q` || l == "quit" || l == "exit":
		return ""
	case l == `\l` || l == `\list` || l == "show databases":
		rows := make([][]any, 0, len(in.Databases))
		for _, d := range in.Databases {
			rows = append(rows, []any{d})
		}
		if my {
			return sqlengine.FormatTable([]string{"Database"}, rows, nil) + fmt.Sprintf("%d rows in set", len(rows))
		}
		return psqlTable([]string{"Name"}, rows, "List of databases")
	case l == `\du` || l == "select user from mysql.user":
		var rows [][]any
		for _, u := range SortedKeys(in.Users) {
			rows = append(rows, []any{u})
		}
		return psqlTable([]string{"Role name"}, rows, "List of roles")
	case l == `\conninfo`:
		return fmt.Sprintf("You are connected to database \"%s\" as user \"postgres\" on host \"%s\" at port \"5432\".", db, in.PublicIP+in.PrivateIP)
	case l == `\?` || l == "help":
		return `General
  \q                     quit
  \l                     list databases
  \c DBNAME              connect to another database
  \dt                    list tables
  \d TABLE               describe a table
  \du                    list roles
  \conninfo              connection information`
	}
	if strings.HasPrefix(l, `\`) && !strings.HasPrefix(l, `\dt`) && !strings.HasPrefix(l, `\d `) && l != `\d` {
		return fmt.Sprintf("invalid command %s\nTry \\? for help.", strings.Fields(st)[0])
	}
	if strings.HasPrefix(l, "create database") {
		f := strings.Fields(st)
		name := strings.Trim(f[len(f)-1], "`\"")
		if contains(in.Databases, name) {
			return fmt.Sprintf("ERROR:  database \"%s\" already exists", name)
		}
		in.Databases = append(in.Databases, name)
		if my {
			return "Query OK, 1 row affected"
		}
		return "CREATE DATABASE"
	}
	if strings.HasPrefix(l, "drop database") {
		f := strings.Fields(st)
		name := strings.Trim(f[len(f)-1], "`\"")
		var keep []string
		for _, d := range in.Databases {
			if d != name {
				keep = append(keep, d)
			}
		}
		in.Databases = keep
		delete(in.Data, name)
		return "DROP DATABASE"
	}
	c, err := sqlengine.New()
	if err != nil {
		return "ERROR:  " + err.Error()
	}
	defer c.Close()
	if err := c.Load(in.Data[db]); err != nil {
		return "ERROR:  " + err.Error()
	}
	switch {
	case l == `\dt` || l == "show tables" || strings.HasPrefix(l, `\dt `):
		ts, _ := c.Tables("")
		if my {
			rows := make([][]any, len(ts))
			for i, t := range ts {
				rows[i] = []any{t}
			}
			if len(rows) == 0 {
				return "Empty set"
			}
			return sqlengine.FormatTable([]string{"Tables_in_" + db}, rows, nil) + fmt.Sprintf("%d rows in set", len(rows))
		}
		if len(ts) == 0 {
			return "Did not find any relations."
		}
		rows := make([][]any, len(ts))
		for i, t := range ts {
			rows[i] = []any{"public", t, "table", "postgres"}
		}
		return psqlTable([]string{"Schema", "Name", "Type", "Owner"}, rows, "List of relations")
	case strings.HasPrefix(l, `\d `) || strings.HasPrefix(l, "describe ") || strings.HasPrefix(l, "desc "):
		name := strings.Trim(strings.Fields(st)[1], "`\";")
		cols, err := c.Columns("", name)
		if err != nil || len(cols) == 0 {
			if my {
				return fmt.Sprintf("ERROR 1146 (42S02): Table '%s.%s' doesn't exist", db, name)
			}
			return fmt.Sprintf("Did not find any relation named \"%s\".", name)
		}
		rows := make([][]any, len(cols))
		for i, col := range cols {
			rows[i] = []any{col.Name, strings.ToLower(col.Type)}
		}
		if my {
			return sqlengine.FormatTable([]string{"Field", "Type"}, rows, nil) + fmt.Sprintf("%d rows in set", len(rows))
		}
		return psqlTable([]string{"Column", "Type"}, rows, fmt.Sprintf("Table \"public.%s\"", name))
	case l == `\d`:
		return s.runOne(in, db, `\dt`, my)
	}
	q := st
	if my {
		q = reMyAutoInc.ReplaceAllString(q, "INTEGER PRIMARY KEY AUTOINCREMENT")
		q = reMyEngine.ReplaceAllString(q, ")")
	} else {
		q, _ = sqlengine.FromPostgres(q, s.Now())
	}
	res, err := c.Exec(q)
	if err != nil {
		return clientError(err, my)
	}
	d, err := c.Dump()
	if err == nil {
		in.Data[db] = d
	}
	if len(res) == 0 {
		return ""
	}
	r := res[len(res)-1]
	if r.Query {
		if my {
			if len(r.Rows) == 0 {
				return "Empty set"
			}
			return sqlengine.FormatTable(r.Columns, r.Rows, nil) + fmt.Sprintf("%d row%s in set", len(r.Rows), plural(len(r.Rows)))
		}
		return psqlTable(r.Columns, r.Rows, "")
	}
	if my {
		return fmt.Sprintf("Query OK, %d row%s affected", r.Affected, plural(int(r.Affected)))
	}
	verb := strings.ToUpper(strings.Fields(st)[0])
	switch verb {
	case "INSERT":
		return fmt.Sprintf("INSERT 0 %d", r.Affected)
	case "UPDATE", "DELETE":
		return fmt.Sprintf("%s %d", verb, r.Affected)
	case "CREATE", "DROP", "ALTER":
		f := strings.Fields(strings.ToUpper(st))
		if len(f) > 1 {
			obj := f[1]
			if obj == "UNIQUE" && len(f) > 2 {
				obj = f[2]
			}
			return verb + " " + obj
		}
	}
	return verb
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func clientError(err error, my bool) string {
	msg := err.Error()
	if i := strings.Index(msg, "SQL logic error: "); i >= 0 {
		msg = msg[i+len("SQL logic error: "):]
	}
	if i := strings.Index(msg, "constraint failed: "); i >= 0 {
		msg = "duplicate key value violates unique constraint (" + msg[i+len("constraint failed: "):] + ")"
	}
	msg = regexp.MustCompile(`\s*\(\d+\)$`).ReplaceAllString(msg, "")
	if m := regexp.MustCompile(`^no such table: (?:main\.)?(\S+)`).FindStringSubmatch(msg); m != nil {
		if my {
			return fmt.Sprintf("ERROR 1146 (42S02): Table '%s' doesn't exist", m[1])
		}
		return fmt.Sprintf("ERROR:  relation \"%s\" does not exist", m[1])
	}
	if m := regexp.MustCompile(`^no such column: (\S+)`).FindStringSubmatch(msg); m != nil {
		if my {
			return fmt.Sprintf("ERROR 1054 (42S22): Unknown column '%s' in 'field list'", m[1])
		}
		return fmt.Sprintf("ERROR:  column \"%s\" does not exist", m[1])
	}
	if my {
		return "ERROR 1064 (42000): " + msg
	}
	return "ERROR:  " + msg
}

// psqlTable renders rows like psql.
func psqlTable(cols []string, rows [][]any, title string) string {
	width := make([]int, len(cols))
	for i, c := range cols {
		width[i] = len(c)
	}
	cells := make([][]string, len(rows))
	numeric := make([]bool, len(cols))
	for r, row := range rows {
		cells[r] = make([]string, len(cols))
		for i := range cols {
			v := ""
			if i < len(row) && row[i] != nil {
				v = sqlengine.Display(row[i])
				switch row[i].(type) {
				case int64, float64:
					numeric[i] = true
				}
			}
			cells[r][i] = v
			if len(v) > width[i] {
				width[i] = len(v)
			}
		}
	}
	var b strings.Builder
	total := 0
	for _, w := range width {
		total += w + 3
	}
	if title != "" {
		pad := (total - len(title)) / 2
		if pad < 0 {
			pad = 0
		}
		b.WriteString(strings.Repeat(" ", pad) + title + "\n")
	}
	for i, c := range cols {
		if i > 0 {
			b.WriteString("|")
		}
		left := (width[i] - len(c)) / 2
		b.WriteString(" " + strings.Repeat(" ", left) + c + strings.Repeat(" ", width[i]-len(c)-left) + " ")
	}
	b.WriteString("\n")
	for i, w := range width {
		if i > 0 {
			b.WriteString("+")
		}
		b.WriteString(strings.Repeat("-", w+2))
	}
	b.WriteString("\n")
	for _, r := range cells {
		var line strings.Builder
		for i, v := range r {
			if i > 0 {
				line.WriteString("|")
			}
			if numeric[i] {
				line.WriteString(" " + strings.Repeat(" ", width[i]-len(v)) + v + " ")
			} else {
				line.WriteString(" " + v + strings.Repeat(" ", width[i]-len(v)) + " ")
			}
		}
		b.WriteString(strings.TrimRight(line.String(), " ") + "\n")
	}
	b.WriteString(fmt.Sprintf("(%d row%s)", len(rows), plural(len(rows))))
	return b.String()
}
