// Package sqlengine runs the SQL of the simulated databases (BigQuery,
// Cloud SQL, Spanner) on an in-memory SQLite database. Databases are stored
// in the simulator state as plain data (DDL statements and rows), loaded into
// a fresh engine for every statement and dumped back afterwards, so the
// state stays deterministic and serialisable.
package sqlengine

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"modernc.org/sqlite"
)

// DB is a serialisable database: its DDL (tables, indexes, views) and rows.
type DB struct {
	Schema []string              `json:"schema,omitempty"`
	Tables map[string]*TableData `json:"tables,omitempty"`
}

// TableData holds the rows of one table.
type TableData struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// Column describes a column of a table defined from a schema (BigQuery).
type Column struct {
	Name string
	Type string // SQLite type: INTEGER, REAL, TEXT, BLOB
}

// Result is the outcome of one statement.
type Result struct {
	Columns  []string
	Rows     [][]any
	Affected int64
	Query    bool
}

// Conn is an engine loaded with one or more databases.
type Conn struct {
	db      *sql.DB
	schemas []string
}

var registerOnce sync.Once

func register() {
	registerOnce.Do(func() {
		_ = sqlite.RegisterDeterministicScalarFunction("safe_divide", 2, func(ctx *sqlite.FunctionContext, args []driverValue) (driverValue, error) {
			a, okA := num(args[0])
			b, okB := num(args[1])
			if !okA || !okB || b == 0 {
				return nil, nil
			}
			return a / b, nil
		})
		_ = sqlite.RegisterDeterministicScalarFunction("starts_with", 2, func(ctx *sqlite.FunctionContext, args []driverValue) (driverValue, error) {
			return boolInt(strings.HasPrefix(fmt.Sprint(args[0]), fmt.Sprint(args[1]))), nil
		})
		_ = sqlite.RegisterDeterministicScalarFunction("ends_with", 2, func(ctx *sqlite.FunctionContext, args []driverValue) (driverValue, error) {
			return boolInt(strings.HasSuffix(fmt.Sprint(args[0]), fmt.Sprint(args[1]))), nil
		})
		_ = sqlite.RegisterDeterministicScalarFunction("regexp_contains", 2, func(ctx *sqlite.FunctionContext, args []driverValue) (driverValue, error) {
			if args[0] == nil {
				return nil, nil
			}
			re, err := regexp.Compile(fmt.Sprint(args[1]))
			if err != nil {
				return nil, err
			}
			return boolInt(re.MatchString(fmt.Sprint(args[0]))), nil
		})
		_ = sqlite.RegisterDeterministicScalarFunction("regexp_extract", 2, func(ctx *sqlite.FunctionContext, args []driverValue) (driverValue, error) {
			if args[0] == nil {
				return nil, nil
			}
			re, err := regexp.Compile(fmt.Sprint(args[1]))
			if err != nil {
				return nil, err
			}
			m := re.FindStringSubmatch(fmt.Sprint(args[0]))
			switch {
			case m == nil:
				return nil, nil
			case len(m) > 1:
				return m[1], nil
			}
			return m[0], nil
		})
		_ = sqlite.RegisterDeterministicScalarFunction("regexp_replace", 3, func(ctx *sqlite.FunctionContext, args []driverValue) (driverValue, error) {
			if args[0] == nil {
				return nil, nil
			}
			re, err := regexp.Compile(fmt.Sprint(args[1]))
			if err != nil {
				return nil, err
			}
			return re.ReplaceAllString(fmt.Sprint(args[0]), strings.ReplaceAll(fmt.Sprint(args[2]), `\`, "$")), nil
		})
		_ = sqlite.RegisterDeterministicScalarFunction("split_part", 3, func(ctx *sqlite.FunctionContext, args []driverValue) (driverValue, error) {
			parts := strings.Split(fmt.Sprint(args[0]), fmt.Sprint(args[1]))
			i, _ := num(args[2])
			if int(i) < 1 || int(i) > len(parts) {
				return "", nil
			}
			return parts[int(i)-1], nil
		})
	})
}

type driverValue = driver.Value

func num(v any) (float64, bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), true
	case float64:
		return x, true
	case int:
		return float64(x), true
	}
	return 0, false
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// New opens an empty engine.
func New() (*Conn, error) {
	register()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	// One connection: every :memory: connection is a different database.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, err
	}
	return &Conn{db: db}, nil
}

// Close releases the engine.
func (c *Conn) Close() { c.db.Close() }

// Attach adds an empty schema (e.g. a BigQuery dataset).
func (c *Conn) Attach(name string) error {
	for _, s := range c.schemas {
		if s == name {
			return nil
		}
	}
	if _, err := c.db.Exec(fmt.Sprintf("ATTACH DATABASE ':memory:' AS %s", Quote(name))); err != nil {
		return err
	}
	c.schemas = append(c.schemas, name)
	return nil
}

// Quote quotes an identifier.
func Quote(id string) string { return `"` + strings.ReplaceAll(id, `"`, `""`) + `"` }

// CreateTable creates a table in a schema ("" = main) and loads rows.
func (c *Conn) CreateTable(schema, name string, cols []Column, rows [][]any) error {
	q := Quote(name)
	if schema != "" {
		q = Quote(schema) + "." + q
	}
	defs := make([]string, len(cols))
	for i, col := range cols {
		defs[i] = Quote(col.Name) + " " + col.Type
	}
	if _, err := c.db.Exec(fmt.Sprintf("CREATE TABLE %s (%s)", q, strings.Join(defs, ", "))); err != nil {
		return err
	}
	return c.insert(q, len(cols), rows)
}

func (c *Conn) insert(qname string, ncols int, rows [][]any) error {
	if len(rows) == 0 || ncols == 0 {
		return nil
	}
	ph := "(" + strings.TrimSuffix(strings.Repeat("?,", ncols), ",") + ")"
	stmt := fmt.Sprintf("INSERT INTO %s VALUES %s", qname, ph)
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	st, err := tx.Prepare(stmt)
	if err != nil {
		tx.Rollback()
		return err
	}
	for _, r := range rows {
		vals := make([]any, ncols)
		copy(vals, r)
		if _, err := st.Exec(vals...); err != nil {
			st.Close()
			tx.Rollback()
			return err
		}
	}
	st.Close()
	return tx.Commit()
}

// Load loads a dumped database into the main schema.
func (c *Conn) Load(d *DB) error {
	if d == nil {
		return nil
	}
	for _, s := range d.Schema {
		if _, err := c.db.Exec(s); err != nil {
			return fmt.Errorf("loading schema: %w", err)
		}
	}
	names := make([]string, 0, len(d.Tables))
	for n := range d.Tables {
		names = append(names, n)
	}
	sort.Strings(names)
	// Foreign keys are checked by the original statements; skip them on reload.
	c.db.Exec("PRAGMA foreign_keys = OFF")
	defer c.db.Exec("PRAGMA foreign_keys = ON")
	for _, n := range names {
		t := d.Tables[n]
		if err := c.insert(Quote(n), len(t.Columns), t.Rows); err != nil {
			return fmt.Errorf("loading %s: %w", n, err)
		}
	}
	return nil
}

// Dump returns the main schema as a DB.
func (c *Conn) Dump() (*DB, error) {
	rows, err := c.db.Query("SELECT type, name, sql FROM sqlite_master WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%' ORDER BY CASE type WHEN 'table' THEN 0 WHEN 'index' THEN 1 WHEN 'view' THEN 2 ELSE 3 END, rowid")
	if err != nil {
		return nil, err
	}
	d := &DB{Tables: map[string]*TableData{}}
	var tables []string
	for rows.Next() {
		var typ, name, s string
		if err := rows.Scan(&typ, &name, &s); err != nil {
			rows.Close()
			return nil, err
		}
		d.Schema = append(d.Schema, s)
		if typ == "table" {
			tables = append(tables, name)
		}
	}
	rows.Close()
	for _, t := range tables {
		td, err := c.TableRows("", t)
		if err != nil {
			return nil, err
		}
		d.Tables[t] = td
	}
	return d, nil
}

// TableRows reads a whole table.
func (c *Conn) TableRows(schema, name string) (*TableData, error) {
	q := Quote(name)
	if schema != "" {
		q = Quote(schema) + "." + q
	}
	r, err := c.query("SELECT * FROM " + q)
	if err != nil {
		return nil, err
	}
	return &TableData{Columns: r.Columns, Rows: r.Rows}, nil
}

// Tables lists the tables of a schema ("" = main).
func (c *Conn) Tables(schema string) ([]string, error) {
	master := "sqlite_master"
	if schema != "" {
		master = Quote(schema) + ".sqlite_master"
	}
	r, err := c.query("SELECT name FROM " + master + " WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, row := range r.Rows {
		out = append(out, fmt.Sprint(row[0]))
	}
	return out, nil
}

// Columns returns the declared columns of a table.
func (c *Conn) Columns(schema, name string) ([]Column, error) {
	prefix := ""
	if schema != "" {
		prefix = Quote(schema) + "."
	}
	r, err := c.query(fmt.Sprintf("PRAGMA %stable_info(%s)", prefix, Quote(name)))
	if err != nil {
		return nil, err
	}
	var out []Column
	for _, row := range r.Rows {
		out = append(out, Column{Name: fmt.Sprint(row[1]), Type: fmt.Sprint(row[2])})
	}
	return out, nil
}

func (c *Conn) query(q string, args ...any) (Result, error) {
	rows, err := c.db.Query(q, args...)
	if err != nil {
		return Result{}, err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	res := Result{Columns: cols, Query: true}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return res, err
		}
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		res.Rows = append(res.Rows, vals)
	}
	return res, rows.Err()
}

var reReturnsRows = regexp.MustCompile(`(?is)^\s*(SELECT|WITH|VALUES|PRAGMA|EXPLAIN)\b|\bRETURNING\b`)

// Exec runs one or more statements separated by semicolons.
func (c *Conn) Exec(script string) ([]Result, error) {
	var out []Result
	for _, st := range Split(script) {
		if reReturnsRows.MatchString(st) {
			r, err := c.query(st)
			if err != nil {
				return out, err
			}
			out = append(out, r)
			continue
		}
		r, err := c.db.Exec(st)
		if err != nil {
			return out, err
		}
		n, _ := r.RowsAffected()
		out = append(out, Result{Affected: n})
	}
	return out, nil
}

// Split splits a script into statements, respecting quotes.
func Split(script string) []string {
	var out []string
	var b strings.Builder
	var quote rune
	for _, r := range script {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"' || r == '`':
			quote = r
		case r == ';':
			if s := strings.TrimSpace(b.String()); s != "" {
				out = append(out, s)
			}
			b.Reset()
			continue
		}
		b.WriteRune(r)
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		out = append(out, s)
	}
	return out
}

// FormatTable renders rows as a boxed table (like bq and psql output).
func FormatTable(cols []string, rows [][]any, boolCols map[string]bool) string {
	cells := make([][]string, len(rows))
	width := make([]int, len(cols))
	for i, c := range cols {
		width[i] = len(c)
	}
	for r, row := range rows {
		cells[r] = make([]string, len(cols))
		for i := range cols {
			var v any
			if i < len(row) {
				v = row[i]
			}
			s := Display(v)
			if boolCols[cols[i]] {
				switch s {
				case "1":
					s = "true"
				case "0":
					s = "false"
				}
			}
			cells[r][i] = s
			if l := len([]rune(s)); l > width[i] {
				width[i] = l
			}
		}
	}
	var b strings.Builder
	sep := "+"
	for _, w := range width {
		sep += strings.Repeat("-", w+2) + "+"
	}
	line := func(vals []string) {
		b.WriteString("|")
		for i, v := range vals {
			b.WriteString(" " + v + strings.Repeat(" ", width[i]-len([]rune(v))) + " |")
		}
		b.WriteString("\n")
	}
	b.WriteString(sep + "\n")
	line(cols)
	b.WriteString(sep + "\n")
	for _, r := range cells {
		line(r)
	}
	b.WriteString(sep + "\n")
	return b.String()
}

// Display formats a value for output.
func Display(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case float64:
		if x == float64(int64(x)) && x < 1e15 && x > -1e15 {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	case []byte:
		return string(x)
	}
	return fmt.Sprint(v)
}
