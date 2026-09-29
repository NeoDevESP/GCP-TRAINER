package sqlengine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBigQueryDialectAndRoundTrip(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Attach("sales"); err != nil {
		t.Fatal(err)
	}
	if err := c.CreateTable("sales", "orders", []Column{{"id", "INTEGER"}, {"country", "TEXT"}, {"amount", "REAL"}, {"created", "TEXT"}, {"paid", "INTEGER"}}, [][]any{
		{1, "ES", 10.5, "2026-08-30 10:00:00", 1},
		{2, "ES", 4.5, "2026-08-01 10:00:00", 0},
		{3, "FR", 20.0, "2026-08-31 09:00:00", 1},
	}); err != nil {
		t.Fatal(err)
	}
	q, err := FromBigQuery("SELECT country, COUNTIF(paid) AS paid, SUM(amount) AS total, SAFE_DIVIDE(SUM(amount), COUNT(*)) AS avg FROM `proj.sales.orders` WHERE DATE(created) >= DATE_SUB(CURRENT_DATE(), INTERVAL 7 DAY) AND REGEXP_CONTAINS(country, r'^[A-Z]{2}$') GROUP BY country ORDER BY country", "proj", "2026-09-01T09:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Exec(q)
	if err != nil {
		t.Fatalf("%v\n%s", err, q)
	}
	got := FormatTable(res[0].Columns, res[0].Rows, nil)
	if !strings.Contains(got, "| ES      | 1    | 10.5  | 10.5 |") || !strings.Contains(got, "| FR      | 1    | 20    | 20   |") {
		t.Fatalf("unexpected result:\n%s\n%s", got, q)
	}
	if _, err := FromBigQuery("SELECT * EXCEPT(id) FROM sales.orders", "p", "2026-09-01T00:00:00Z"); err == nil {
		t.Fatal("unsupported construct must be reported")
	}
	q, _ = FromBigQuery("SELECT EXTRACT(MONTH FROM created) AS m, DATE_TRUNC(DATE(created), MONTH) AS mo, IF(amount > 10, 'big', 'small') AS size FROM sales.orders WHERE id = 3", "p", "2026-09-01T00:00:00Z")
	res, err = c.Exec(q)
	if err != nil || len(res[0].Rows) != 1 || Display(res[0].Rows[0][0]) != "8" || res[0].Rows[0][1] != "2026-08-01" || res[0].Rows[0][2] != "big" {
		t.Fatalf("%v %v\n%s", err, res, q)
	}
	td, err := c.TableRows("sales", "orders")
	if err != nil || len(td.Rows) != 3 {
		t.Fatalf("%v %v", err, td)
	}
}

func TestPostgresDumpAndReload(t *testing.T) {
	c, _ := New()
	for _, st := range []string{
		"CREATE TABLE customers (id SERIAL PRIMARY KEY, name TEXT NOT NULL, vip BOOLEAN DEFAULT FALSE)",
		"CREATE INDEX idx_name ON customers(name)",
		"INSERT INTO customers (name, vip) VALUES ('Ana', TRUE), ('Luis', FALSE)",
	} {
		s, _ := FromPostgres(st, "2026-09-01T00:00:00Z")
		if _, err := c.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	d, err := c.Dump()
	c.Close()
	if err != nil {
		t.Fatal(err)
	}
	// Survives a JSON round trip, like the simulator state.
	b, _ := json.Marshal(d)
	var d2 DB
	json.Unmarshal(b, &d2)
	c2, _ := New()
	defer c2.Close()
	if err := c2.Load(&d2); err != nil {
		t.Fatal(err)
	}
	res, err := c2.Exec("INSERT INTO customers (name) VALUES ('Eva'); SELECT id, name, vip FROM customers ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	out := FormatTable(res[1].Columns, res[1].Rows, nil)
	if !strings.Contains(out, "| 3  | Eva  | 0   |") || len(d2.Schema) != 2 {
		t.Fatalf("unexpected:\n%s\n%v", out, d2.Schema)
	}
}
