package cli

import (
	"strings"
	"testing"
)

func TestFirestoreREST(t *testing.T) {
	s := newTestSession()
	run(t, s, "gcloud services enable firestore.googleapis.com")
	run(t, s, "gcloud firestore databases create --location=eur3")
	base := "https://firestore.googleapis.com/v1/projects/lab-proj/databases/(default)/documents"
	if r := s.Exec("curl -s " + base + "/users"); !strings.Contains(r.Output, "UNAUTHENTICATED") {
		t.Fatalf("anonymous access: %q", r.Output)
	}
	auth := `-H "Authorization: Bearer $(gcloud auth print-access-token)"`
	out := run(t, s, `curl -s -X POST `+auth+` "`+base+`/users?documentId=ana" -d '{"fields": {"name": {"stringValue": "Ana"}, "age": {"integerValue": "31"}}}'`)
	if !strings.Contains(out, `"stringValue": "Ana"`) {
		t.Fatalf("create doc: %q", out)
	}
	if out := run(t, s, "curl -s "+auth+" "+base+"/users/ana"); !strings.Contains(out, `"integerValue": "31"`) {
		t.Fatalf("get doc: %q", out)
	}
	run(t, s, "gcloud storage buckets create gs://lab-proj-fs --location=europe-west1")
	run(t, s, "gcloud firestore export gs://lab-proj-fs/backup1")
	s.State.Projects["lab-proj"].Firestore["(default)"].Docs = map[string]map[string]any{}
	if out := run(t, s, "gcloud firestore import gs://lab-proj-fs/backup1"); !strings.Contains(out, "completedWork: '1'") {
		t.Fatalf("import: %q", out)
	}
	run(t, s, "gcloud firestore indexes composite create --collection-group=users --field-config=field-path=age,order=ascending --field-config=field-path=name,order=descending")
	if out := run(t, s, "gcloud firestore indexes composite list"); !strings.Contains(out, "age ASCENDING") {
		t.Fatalf("indexes: %q", out)
	}
}

func TestRedisFromVM(t *testing.T) {
	s := newTestSession()
	run(t, s, "gcloud services enable redis.googleapis.com")
	run(t, s, "gcloud redis instances create cache --region=europe-west1 --size=1 --tier=basic")
	host := s.State.Projects["lab-proj"].Redis["cache"].Host
	run(t, s, "gcloud compute instances create app --zone=europe-west1-b")
	if r := s.Exec("gcloud compute ssh app --zone=europe-west1-b --command='redis-cli -h " + host + " PING'"); r.Exit == 0 {
		t.Fatalf("redis-cli without redis-tools: %q", r.Output)
	}
	run(t, s, "gcloud compute ssh app --zone=europe-west1-b --command='sudo apt-get install -y redis-tools'")
	s.Exec("gcloud compute ssh app --zone=europe-west1-b")
	if r := s.Exec("redis-cli -h " + host); r.Prompt != host+":6379> " {
		t.Fatalf("interactive redis: %+v", r)
	}
	s.Exec("SET visits 10")
	if r := s.Exec("INCR visits"); strings.TrimSpace(r.Output) != "(integer) 11" {
		t.Fatalf("incr: %q", r.Output)
	}
	s.Exec("quit")
	s.Exec("exit")
	if s.Remote != nil {
		t.Fatal("session should be closed")
	}
	if r := s.Exec("redis-cli -h " + host + " PING"); r.Exit == 0 {
		t.Fatal("redis-cli from Cloud Shell should fail")
	}
}

func TestSpannerSQL(t *testing.T) {
	s := newTestSession()
	run(t, s, "gcloud services enable spanner.googleapis.com")
	run(t, s, "gcloud spanner instances create shop --config=regional-europe-west1 --processing-units=100 --description=Shop")
	run(t, s, `gcloud spanner databases create orders --instance=shop --ddl="CREATE TABLE Customers (CustomerId INT64 NOT NULL, Name STRING(100)) PRIMARY KEY (CustomerId)"`)
	run(t, s, `gcloud spanner databases execute-sql orders --instance=shop --sql="INSERT INTO Customers (CustomerId, Name) VALUES (1, 'Ana'), (2, 'Luis')"`)
	out := run(t, s, `gcloud spanner databases execute-sql orders --instance=shop --sql="SELECT CustomerId, Name FROM Customers ORDER BY CustomerId"`)
	if !strings.Contains(out, "CustomerId  Name\n1           Ana\n2           Luis") {
		t.Fatalf("query: %q", out)
	}
	run(t, s, `gcloud spanner databases ddl update orders --instance=shop --ddl="ALTER TABLE Customers ADD COLUMN Email STRING(MAX)"`)
	if out := run(t, s, "gcloud spanner databases ddl describe orders --instance=shop"); !strings.Contains(out, "ADD COLUMN Email") {
		t.Fatalf("ddl: %q", out)
	}
	if r := s.Exec(`gcloud spanner databases execute-sql orders --instance=shop --sql="SELECT * FROM Nope"`); !strings.Contains(r.Output, "Table not found") {
		t.Fatalf("error: %q", r.Output)
	}
	if r := s.Exec(`gcloud spanner instances create bad --config=regional-europe-west1 --processing-units=150`); r.Exit == 0 {
		t.Fatal("150 PU should be rejected")
	}
}

func TestDataflowDataprocComposer(t *testing.T) {
	s := newTestSession()
	run(t, s, "gcloud services enable dataflow.googleapis.com dataproc.googleapis.com composer.googleapis.com file.googleapis.com bigquery.googleapis.com")
	run(t, s, "gcloud storage buckets create gs://lab-proj-df --location=europe-west1")
	out := run(t, s, "gcloud dataflow jobs run wc --gcs-location=gs://dataflow-templates-europe-west1/latest/Word_Count --region=europe-west1 --parameters=inputFile=gs://dataflow-samples/shakespeare/kinglear.txt,output=gs://lab-proj-df/results/out")
	if !strings.Contains(out, "JOB_TYPE_BATCH") {
		t.Fatalf("run: %q", out)
	}
	if out := run(t, s, "gcloud storage cat gs://lab-proj-df/results/out-00000-of-00001"); !strings.Contains(out, "our: ") {
		t.Fatalf("word count output: %q", out)
	}
	// Streaming: Pub/Sub -> BigQuery.
	run(t, s, "gcloud pubsub topics create events")
	run(t, s, "gcloud pubsub subscriptions create events-df --topic=events")
	run(t, s, "bq mk analytics")
	run(t, s, "bq mk --table analytics.events user:STRING,amount:INTEGER")
	run(t, s, "gcloud dataflow jobs run stream --gcs-location=gs://dataflow-templates-europe-west1/latest/PubSub_Subscription_to_BigQuery --region=europe-west1 --parameters=inputSubscription=projects/lab-proj/subscriptions/events-df,outputTableSpec=lab-proj:analytics.events")
	run(t, s, `gcloud pubsub topics publish events --message='{"user": "ana", "amount": 5}'`)
	s.State.Step(1)
	if out := run(t, s, "bq query --use_legacy_sql=false 'SELECT user, amount FROM analytics.events'"); !strings.Contains(out, "ana") {
		t.Fatalf("streaming rows: %q", out)
	}
	// Dataproc.
	run(t, s, "gcloud dataproc clusters create spark1 --region=europe-west1 --single-node")
	out = run(t, s, "gcloud dataproc jobs submit spark --cluster=spark1 --region=europe-west1 --class=org.apache.spark.examples.SparkPi --jars=file:///usr/lib/spark/examples/jars/spark-examples.jar -- 1000")
	if !strings.Contains(out, "Pi is roughly 3.14") {
		t.Fatalf("spark pi: %q", out)
	}
	// Composer.
	run(t, s, "gcloud composer environments create etl --location=europe-west1")
	s.Files[s.path("daily.py")] = "from airflow import DAG\nwith DAG(dag_id='daily_sales') as dag:\n    pass\n"
	run(t, s, "gcloud composer environments storage dags import --environment=etl --location=europe-west1 --source=daily.py")
	if out := run(t, s, "gcloud composer environments run etl --location=europe-west1 dags list"); !strings.Contains(out, "daily_sales") {
		t.Fatalf("dags list: %q", out)
	}
	if out := run(t, s, "gcloud composer environments run etl --location=europe-west1 dags trigger -- daily_sales"); !strings.Contains(out, "DagRun daily_sales") {
		t.Fatalf("trigger: %q", out)
	}
	run(t, s, "gcloud filestore instances create nfs1 --zone=europe-west1-b --tier=BASIC_HDD --file-share=name=vol1,capacity=1TB --network=name=default")
	if out := run(t, s, "gcloud filestore instances list"); !strings.Contains(out, "vol1") {
		t.Fatalf("filestore: %q", out)
	}
}
