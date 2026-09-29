package cli

import (
	"strings"
	"testing"
)

func TestBigQueryRealData(t *testing.T) {
	s := newTestSession()
	s.Files["orders.csv"] = "order_id,country,amount,paid\nA1,ES,10.5,true\nA2,ES,4.5,false\nA3,FR,20,true\n"
	for _, l := range []string{
		"bq mk --dataset --location=EU sales",
		"gcloud storage buckets create gs://lab-proj-raw --location=EU",
		"gcloud storage cp orders.csv gs://lab-proj-raw/orders.csv",
		"bq load --source_format=CSV --skip_leading_rows=1 sales.orders gs://lab-proj-raw/orders.csv order_id:STRING,country:STRING,amount:NUMERIC,paid:BOOL",
	} {
		if r := s.Exec(l); r.Exit != 0 {
			t.Fatalf("%s: %s", l, r.Output)
		}
	}
	out := run(t, s, "bq query --use_legacy_sql=false 'SELECT country, COUNT(*) AS n, SUM(amount) AS total FROM `lab-proj.sales.orders` GROUP BY country ORDER BY country'")
	if !strings.Contains(out, "| ES      | 2 | 15    |") || !strings.Contains(out, "| FR      | 1 | 20    |") {
		t.Fatalf("unexpected result:\n%s", out)
	}
	run(t, s, "bq query --use_legacy_sql=false \"INSERT INTO sales.orders (order_id, country, amount, paid) VALUES ('A4', 'PT', 7, false)\"")
	out = run(t, s, "bq query --use_legacy_sql=false \"UPDATE sales.orders SET paid = true WHERE order_id = 'A2'\"")
	if !strings.Contains(out, "Number of affected rows: 1") {
		t.Fatalf("update: %s", out)
	}
	out = run(t, s, "bq head -n 10 sales.orders")
	if !strings.Contains(out, "| A4       | PT      | 7      | false |") || !strings.Contains(out, "| A2       | ES      | 4.5    | true  |") {
		t.Fatalf("head:\n%s", out)
	}
	run(t, s, "bq query --use_legacy_sql=false 'CREATE TABLE sales.big_orders AS SELECT order_id, amount FROM sales.orders WHERE amount > 5'")
	out = run(t, s, "bq query --use_legacy_sql=false --format=csv 'SELECT order_id FROM sales.big_orders ORDER BY order_id'")
	if strings.TrimSpace(out[strings.Index(out, "order_id"):strings.Index(out, "Bytes")]) != "order_id\nA1\nA3\nA4" {
		t.Fatalf("ctas:\n%s", out)
	}
	if r := s.Exec("bq query --use_legacy_sql=false 'SELECT nope FROM sales.orders'"); r.Exit == 0 || !strings.Contains(r.Output, "Unrecognized name: nope") {
		t.Fatalf("expected unrecognized name: %s", r.Output)
	}
	run(t, s, "bq extract sales.orders gs://lab-proj-raw/export.csv")
	if o := s.State.Projects["lab-proj"].Buckets["lab-proj-raw"].Objects["export.csv"]; o == nil || !strings.Contains(o.Content, "A4,PT,7,0") {
		t.Fatalf("extract: %+v", o)
	}
	// JSON insert and DDL with columns.
	run(t, s, "bq query --use_legacy_sql=false 'CREATE TABLE sales.customers (id INT64, name STRING)'")
	if r := s.Exec("bq insert sales.customers -"); r.Exit != 0 {
		t.Fatal(r.Output)
	}
	s.Files["c.json"] = "{\"id\": 1, \"name\": \"Ana\"}\n{\"id\": 2, \"name\": \"Luis\"}\n"
	run(t, s, "bq insert sales.customers c.json")
	out = run(t, s, "bq query --use_legacy_sql=false 'SELECT name FROM sales.customers WHERE id = 2'")
	if !strings.Contains(out, "| Luis |") {
		t.Fatalf("insert:\n%s", out)
	}
}

func TestBigQuerySampleOfModelledTable(t *testing.T) {
	s := newTestSession()
	run(t, s, "bq mk --dataset analytics")
	run(t, s, "bq mk --table --sim_rows=2000000000 analytics.events event_ts:TIMESTAMP:8,country:STRING:4,revenue:NUMERIC:16")
	out := run(t, s, "bq query --use_legacy_sql=false 'SELECT country, ROUND(SUM(revenue), 2) AS revenue FROM analytics.events GROUP BY country ORDER BY country LIMIT 3'")
	if !strings.Contains(out, "| country | revenue") || !strings.Contains(out, "GB") && !strings.Contains(out, "DE") {
		t.Fatalf("sample:\n%s", out)
	}
	if !strings.Contains(out, "Bytes processed: 37.25 GB") {
		t.Fatalf("bytes must still come from the modelled size:\n%s", out)
	}
}

func TestCloudSQLRealData(t *testing.T) {
	s := newTestSession()
	for _, l := range []string{
		"gcloud sql instances create db1 --database-version=POSTGRES_15 --tier=db-g1-small --region=europe-west1",
		"gcloud sql databases create shop --instance=db1",
		"gcloud sql users create app --instance=db1 --password=S3cret",
		"gcloud sql instances patch db1 --authorized-networks=0.0.0.0/0 --quiet",
	} {
		if r := s.Exec(l); r.Exit != 0 {
			t.Fatalf("%s: %s", l, r.Output)
		}
	}
	ip := s.State.Projects["lab-proj"].SQLInstances["db1"].PublicIP
	psql := func(sql string) string {
		return run(t, s, "PGPASSWORD=S3cret psql -h "+ip+" -U app -d shop -c \""+sql+"\"")
	}
	psql("CREATE TABLE products (id SERIAL PRIMARY KEY, name TEXT NOT NULL, price NUMERIC(10,2))")
	if out := psql("INSERT INTO products (name, price) VALUES ('taza', 9.5), ('camiseta', 19)"); !strings.Contains(out, "INSERT 0 2") {
		t.Fatalf("insert: %s", out)
	}
	out := psql("SELECT id, name, price FROM products WHERE price > 10")
	if !strings.Contains(out, "  2 | camiseta |    19\n") || !strings.Contains(out, "(1 row)") {
		t.Fatalf("select:\n%s", out)
	}
	if out := psql(`\dt`); !strings.Contains(out, "products") {
		t.Fatalf("\\dt:\n%s", out)
	}
	if out := psql("SELECT * FROM missing_table"); !strings.Contains(out, `ERROR:  relation "missing_table" does not exist`) {
		t.Fatalf("error:\n%s", out)
	}
	// MySQL instance with the mysql client.
	run(t, s, "gcloud sql instances create my1 --database-version=MYSQL_8_0 --tier=db-g1-small --region=europe-west1")
	run(t, s, "gcloud sql instances patch my1 --authorized-networks=0.0.0.0/0 --quiet")
	run(t, s, "gcloud sql databases create inv --instance=my1")
	myip := s.State.Projects["lab-proj"].SQLInstances["my1"].PublicIP
	run(t, s, "mysql -h "+myip+" -u root inv -e \"CREATE TABLE items (id INT AUTO_INCREMENT PRIMARY KEY, sku VARCHAR(20)) ENGINE=InnoDB\"")
	run(t, s, "mysql -h "+myip+" -u root inv -e \"INSERT INTO items (sku) VALUES ('A'), ('B')\"")
	if out := run(t, s, "mysql -h "+myip+" -u root inv -e \"SELECT COUNT(*) AS n FROM items\""); !strings.Contains(out, "| 2 |") || !strings.Contains(out, "1 row in set") {
		t.Fatalf("mysql:\n%s", out)
	}
}
