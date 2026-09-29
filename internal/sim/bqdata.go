package sim

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/sqlengine"
)

// BigQuery data: tables keep their rows in Table.Data and queries run on the
// SQL engine. Tables that model huge datasets (numRows set by a lab, no rows
// loaded) get a deterministic sample so queries return plausible results;
// bytes processed are still estimated from numRows.

// maxSampleRows bounds the synthetic sample of a large table.
const maxSampleRows = 400

// BQResult is the outcome of running a statement on the data.
type BQResult struct {
	Columns  []string
	Rows     [][]any
	Affected int64
	BoolCols map[string]bool
	Query    bool
}

// sampleRows builds deterministic rows for a table without loaded data.
func (s *State) sampleRows(dataset string, t *Table) [][]any {
	n := t.Rows
	if n <= 0 || len(t.Schema) == 0 {
		return nil
	}
	if n > maxSampleRows {
		n = maxSampleRows
	}
	h := fnv.New64a()
	h.Write([]byte(dataset + "." + t.ID))
	r := rand.New(rand.NewSource(int64(h.Sum64())))
	now := s.Clock
	days := t.PartitionDays
	if days <= 0 {
		days = 30
	}
	countries := []string{"ES", "FR", "DE", "IT", "PT", "US", "GB", "MX"}
	events := []string{"view", "click", "add_to_cart", "purchase", "signup"}
	status := []string{"PAID", "PENDING", "SHIPPED", "CANCELLED"}
	rows := make([][]any, n)
	for i := range rows {
		row := make([]any, len(t.Schema))
		for j, f := range t.Schema {
			name := strings.ToLower(f.Name)
			switch strings.ToUpper(f.Type) {
			case "TIMESTAMP", "DATETIME":
				row[j] = now.Add(-time.Duration(r.Intn(days*24*60)) * time.Minute).UTC().Format("2006-01-02 15:04:05")
			case "DATE":
				row[j] = now.AddDate(0, 0, -r.Intn(days)).Format("2006-01-02")
			case "INT64", "INTEGER", "INT":
				if strings.Contains(name, "id") {
					row[j] = int64(i + 1)
				} else {
					row[j] = int64(r.Intn(1000))
				}
			case "FLOAT64", "FLOAT", "NUMERIC", "BIGNUMERIC":
				row[j] = float64(r.Intn(50000)) / 100
			case "BOOL", "BOOLEAN":
				row[j] = int64(r.Intn(2))
			default:
				switch {
				case strings.Contains(name, "country"):
					row[j] = countries[r.Intn(len(countries))]
				case strings.Contains(name, "event"):
					row[j] = events[r.Intn(len(events))]
				case strings.Contains(name, "status"):
					row[j] = status[r.Intn(len(status))]
				case strings.Contains(name, "user") || strings.Contains(name, "customer"):
					row[j] = fmt.Sprintf("u-%04d", r.Intn(300)+1)
				case strings.HasSuffix(name, "id"):
					row[j] = fmt.Sprintf("%s-%05d", strings.TrimSuffix(name, "_id"), i+1)
				case strings.Contains(name, "email"):
					row[j] = fmt.Sprintf("user%d@example.com", r.Intn(300)+1)
				default:
					row[j] = fmt.Sprintf("%s-%d", name, r.Intn(100))
				}
			}
		}
		rows[i] = row
	}
	return rows
}

func bqColumns(t *Table) []sqlengine.Column {
	cols := make([]sqlengine.Column, len(t.Schema))
	for i, f := range t.Schema {
		cols[i] = sqlengine.Column{Name: f.Name, Type: sqlengine.BigQueryType(f.Type)}
	}
	return cols
}

// bqEngine loads every dataset of the project into a fresh engine.
func (s *State) bqEngine(project string) (*sqlengine.Conn, map[string]bool, error) {
	p := s.Projects[project]
	c, err := sqlengine.New()
	if err != nil {
		return nil, nil, err
	}
	boolCols := map[string]bool{}
	var views [][2]string
	for _, dsn := range SortedKeys(p.Datasets) {
		ds := p.Datasets[dsn]
		if err := c.Attach(dsn); err != nil {
			c.Close()
			return nil, nil, err
		}
		for _, tn := range SortedKeys(ds.Tables) {
			t := ds.Tables[tn]
			if t.Kind == "MODEL" {
				continue
			}
			if (t.Kind == "VIEW" || t.Kind == "MATERIALIZED_VIEW") && t.Query != "" {
				views = append(views, [2]string{dsn, tn})
				continue
			}
			if t.Data == nil && t.Rows > 0 {
				t.Data = s.sampleRows(dsn, t)
				t.Sample = true
			}
			for _, f := range t.Schema {
				if sqlengine.IsBool(f.Type) {
					boolCols[f.Name] = true
				}
			}
			if len(t.Schema) == 0 {
				continue
			}
			if err := c.CreateTable(dsn, tn, bqColumns(t), t.Data); err != nil {
				c.Close()
				return nil, nil, fmt.Errorf("loading %s.%s: %w", dsn, tn, err)
			}
		}
	}
	// Views are materialised in dependency order.
	for pass := 0; pass < 3 && len(views) > 0; pass++ {
		var left [][2]string
		for _, v := range views {
			q, err := sqlengine.FromBigQuery(p.Datasets[v[0]].Tables[v[1]].Query, project, s.Now())
			if err == nil {
				_, err = c.Exec(fmt.Sprintf("CREATE TABLE %s.%s AS %s", sqlengine.Quote(v[0]), sqlengine.Quote(v[1]), q))
			}
			if err != nil {
				left = append(left, v)
			}
		}
		views = left
	}
	return c, boolCols, nil
}

var (
	reDML        = regexp.MustCompile(`(?is)^\s*(INSERT|UPDATE|DELETE|TRUNCATE)\b`)
	reDMLTarget  = regexp.MustCompile("(?is)^\\s*(?:INSERT\\s+(?:INTO\\s+)?|UPDATE\\s+|DELETE\\s+(?:FROM\\s+)?|TRUNCATE\\s+TABLE\\s+)`?([a-zA-Z0-9_.:-]+)`?")
	reColumnDefs = regexp.MustCompile(`(?is)^\s*\((.*)\)\s*(?:PARTITION|CLUSTER|OPTIONS|$)`)
)

// IsDML reports whether a statement modifies rows.
func IsDML(sql string) bool { return reDML.MatchString(sql) }

// QueryData runs a SELECT and returns its rows.
func (s *State) QueryData(project, sql string) (BQResult, error) {
	c, boolCols, err := s.bqEngine(project)
	if err != nil {
		return BQResult{}, err
	}
	defer c.Close()
	q, err := sqlengine.FromBigQuery(sql, project, s.Now())
	if err != nil {
		return BQResult{}, err
	}
	res, err := c.Exec(q)
	if err != nil {
		return BQResult{}, bqError(err)
	}
	if len(res) == 0 {
		return BQResult{BoolCols: boolCols}, nil
	}
	last := res[len(res)-1]
	return BQResult{Columns: last.Columns, Rows: last.Rows, Affected: last.Affected, BoolCols: boolCols, Query: last.Query}, nil
}

// bqError turns an engine error into a BigQuery-style message.
func bqError(err error) error {
	msg := err.Error()
	if i := strings.Index(msg, "SQL logic error: "); i >= 0 {
		msg = msg[i+len("SQL logic error: "):]
	}
	msg = regexp.MustCompile(`\s*\(\d+\)$`).ReplaceAllString(msg, "")
	switch {
	case strings.HasPrefix(msg, "no such column: "):
		return fmt.Errorf("Unrecognized name: %s", strings.TrimPrefix(msg, "no such column: "))
	case strings.HasPrefix(msg, "no such table: "):
		return fmt.Errorf("Not found: Table %s was not found", strings.TrimPrefix(msg, "no such table: "))
	case strings.HasPrefix(msg, "no such function: "):
		return fmt.Errorf("Function not found: %s", strings.TrimPrefix(msg, "no such function: "))
	}
	return fmt.Errorf("Syntax error: %s", msg)
}

// RunDML runs INSERT/UPDATE/DELETE/TRUNCATE and stores the new rows.
func (s *State) RunDML(project, principal, sql string) (int64, error) {
	p := s.Projects[project]
	m := reDMLTarget.FindStringSubmatch(sql)
	if m == nil {
		return 0, fmt.Errorf("Syntax error: cannot find the target table")
	}
	d, t, _, dsn := s.findTable(project, m[1])
	if t == nil || d == nil {
		return 0, fmt.Errorf("Not found: Table %s was not found", m[1])
	}
	if t.Kind != "TABLE" {
		return 0, fmt.Errorf("%s is a %s: DML is only allowed on tables", m[1], strings.ToLower(t.Kind))
	}
	if !s.Allowed(principal, "bigquery.tables.updateData", Resource{Project: project, Type: "bigquery.googleapis.com/Dataset", Name: "projects/" + project + "/datasets/" + dsn, Service: "bigquery.googleapis.com", Policies: []*Policy{&d.IAM}}) {
		return 0, fmt.Errorf("Access Denied: Table %s: User does not have bigquery.tables.updateData permission", m[1])
	}
	stmt := sql
	if regexp.MustCompile(`(?i)^\s*TRUNCATE\b`).MatchString(sql) {
		stmt = "DELETE FROM " + m[1]
	}
	c, _, err := s.bqEngine(project)
	if err != nil {
		return 0, err
	}
	defer c.Close()
	q, err := sqlengine.FromBigQuery(stmt, project, s.Now())
	if err != nil {
		return 0, err
	}
	res, err := c.Exec(q)
	if err != nil {
		return 0, bqError(err)
	}
	td, err := c.TableRows(dsn, t.ID)
	if err != nil {
		return 0, err
	}
	t.Data = td.Rows
	if !t.Sample {
		t.Rows = int64(len(t.Data))
	}
	s.recordJob(project, principal, sql, int64(len(t.Data))*16, false, "")
	var n int64
	for _, r := range res {
		n += r.Affected
	}
	_ = p
	return n, nil
}

// FillFromSelect stores the rows of a SELECT in a table (CTAS and
// --destination_table), inferring the schema when the table has none.
func (s *State) FillFromSelect(project, sel string, t *Table) error {
	r, err := s.QueryData(project, sel)
	if err != nil {
		return err
	}
	if len(t.Schema) != len(r.Columns) {
		byName := map[string]Field{}
		for _, f := range t.Schema {
			byName[strings.ToLower(f.Name)] = f
		}
		var schema []Field
		for i, cn := range r.Columns {
			f, ok := byName[strings.ToLower(cn)]
			if !ok {
				f = Field{Name: cn, Type: inferType(r.Rows, i, r.BoolCols[cn]), Bytes: 8}
			}
			f.Name = cn
			schema = append(schema, f)
		}
		t.Schema = schema
	}
	t.Data = r.Rows
	if t.Rows == 0 || !t.Sample {
		if t.Rows < int64(len(r.Rows)) || t.Rows == 0 {
			t.Rows = int64(len(r.Rows))
		}
	}
	return nil
}

func inferType(rows [][]any, i int, isBool bool) string {
	if isBool {
		return "BOOL"
	}
	for _, r := range rows {
		switch v := r[i].(type) {
		case int64, int:
			return "INT64"
		case float64:
			if v == float64(int64(v)) {
				continue
			}
			return "FLOAT64"
		case string:
			if regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`).MatchString(v) {
				return "TIMESTAMP"
			}
			if regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(v) {
				return "DATE"
			}
			return "STRING"
		}
	}
	for _, r := range rows {
		if _, ok := r[i].(float64); ok {
			return "INT64"
		}
	}
	return "STRING"
}

// ParseColumnDefs parses "(id INT64, name STRING NOT NULL, ...)".
func ParseColumnDefs(rest string) []Field {
	m := reColumnDefs.FindStringSubmatch(rest)
	if m == nil {
		return nil
	}
	var out []Field
	depth, start := 0, 0
	body := m[1]
	parts := []string{}
	for i, r := range body {
		switch r {
		case '(', '<':
			depth++
		case ')', '>':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, body[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, body[start:])
	for _, p := range parts {
		f := strings.Fields(strings.TrimSpace(p))
		if len(f) < 2 {
			continue
		}
		out = append(out, Field{Name: strings.Trim(f[0], "`"), Type: strings.ToUpper(strings.SplitN(f[1], "(", 2)[0]), Bytes: 8})
	}
	return out
}

// AppendRows adds rows to a table, matching columns by name.
func (t *Table) AppendRows(cols []string, rows [][]any) {
	idx := make([]int, len(cols))
	for i, cn := range cols {
		idx[i] = -1
		for j, f := range t.Schema {
			if strings.EqualFold(f.Name, cn) {
				idx[i] = j
			}
		}
	}
	for _, r := range rows {
		row := make([]any, len(t.Schema))
		for i, j := range idx {
			if j >= 0 && i < len(r) {
				row[j] = coerce(r[i], t.Schema[j].Type)
			}
		}
		t.Data = append(t.Data, row)
	}
	if !t.Sample {
		t.Rows = int64(len(t.Data))
	}
}

func coerce(v any, typ string) any {
	s, ok := v.(string)
	if !ok {
		if b, ok := v.(bool); ok {
			if b {
				return int64(1)
			}
			return int64(0)
		}
		return v
	}
	switch sqlengine.BigQueryType(typ) {
	case "INTEGER":
		if sqlengine.IsBool(typ) {
			switch strings.ToLower(s) {
			case "true", "1", "yes":
				return int64(1)
			case "", "null":
				return nil
			}
			return int64(0)
		}
		var n int64
		if _, err := fmt.Sscan(s, &n); err == nil {
			return n
		}
		if s == "" {
			return nil
		}
	case "REAL":
		var f float64
		if _, err := fmt.Sscan(s, &f); err == nil {
			return f
		}
		if s == "" {
			return nil
		}
	}
	return s
}

// SortedTables lists the "dataset.table" names of a project.
func (p *Project) SortedTables() []string {
	var out []string
	for dsn, ds := range p.Datasets {
		for tn := range ds.Tables {
			out = append(out, dsn+"."+tn)
		}
	}
	sort.Strings(out)
	return out
}
