package cli

import (
	"strings"
	"testing"
)

func TestFunctionsLifecycle(t *testing.T) {
	s := newTestSession()
	if r := s.Exec("gcloud functions deploy hello --gen2 --runtime=python312 --region=europe-west1 --trigger-http"); r.Exit == 0 || !strings.Contains(r.Output, "cloudfunctions.googleapis.com") {
		t.Fatalf("API should be required: %q", r.Output)
	}
	run(t, s, "gcloud services enable cloudfunctions.googleapis.com run.googleapis.com cloudbuild.googleapis.com")
	s.Files[s.path("main.py")] = "import functions_framework\n\n@functions_framework.http\ndef hello(request):\n    return 'Hola desde Functions'\n"
	out := run(t, s, "gcloud functions deploy hello --gen2 --runtime=python312 --region=europe-west1 --source=. --entry-point=hello --trigger-http")
	if !strings.Contains(out, "url: https://hello-") {
		t.Fatalf("deploy: %q", out)
	}
	fn := s.State.Projects["lab-proj"].Functions["hello"]
	if r := s.Exec("curl -s " + fn.URL); !strings.Contains(r.Output, "403") && !strings.Contains(r.Output, "Forbidden") {
		t.Fatalf("private function answered: %q", r.Output)
	}
	run(t, s, "gcloud functions add-invoker-policy-binding hello --region=europe-west1 --member=allUsers")
	if r := s.Exec("curl -s " + fn.URL); !strings.Contains(r.Output, "Hola desde Functions") {
		t.Fatalf("public function: %q", r.Output)
	}
	// Pub/Sub trigger.
	s.Files[s.path("main.py")] = "def on_msg(event, ctx):\n    print('ok')\n"
	run(t, s, "gcloud functions deploy on-msg --gen2 --runtime=python312 --region=europe-west1 --trigger-topic=orders --entry-point=on_msg")
	run(t, s, "gcloud pubsub topics publish orders --message=hola")
	if n := s.State.Projects["lab-proj"].Functions["on-msg"].Invocations; n != 1 {
		t.Fatalf("invocations after publish: %d", n)
	}
	if out := run(t, s, "gcloud functions list"); !strings.Contains(out, "Pub/Sub orders") {
		t.Fatalf("list: %q", out)
	}
	if r := s.Exec("gcloud functions deploy bad --gen2 --runtime=python312 --region=europe-west1 --trigger-http --entry-point=nope"); r.Exit == 0 {
		t.Fatalf("missing entry point should fail")
	}
	run(t, s, "gcloud functions delete hello --region=europe-west1")
}

func TestSchedulerAndTasks(t *testing.T) {
	s := newTestSession()
	run(t, s, "gcloud services enable cloudscheduler.googleapis.com cloudtasks.googleapis.com")
	run(t, s, "gcloud pubsub topics create ticks")
	run(t, s, "gcloud pubsub subscriptions create ticks-sub --topic=ticks")
	run(t, s, "gcloud scheduler jobs create pubsub every-min --location=europe-west1 --schedule='* * * * *' --topic=ticks --message-body=tick")
	s.State.Step(3)
	out := run(t, s, "gcloud pubsub subscriptions pull ticks-sub --auto-ack --limit=10")
	if !strings.Contains(out, "tick") {
		t.Fatalf("scheduler did not publish: %q", out)
	}
	run(t, s, "gcloud scheduler jobs pause every-min --location=europe-west1")
	if out := run(t, s, "gcloud scheduler jobs list --location=europe-west1"); !strings.Contains(out, "PAUSED") {
		t.Fatalf("list: %q", out)
	}
	run(t, s, "gcloud tasks queues create q1 --location=europe-west1")
	run(t, s, "gcloud tasks create-http-task --queue=q1 --location=europe-west1 --url=https://example.com/work --method=POST")
	s.State.Step(2)
	if out := run(t, s, "gcloud tasks queues list --location=europe-west1"); !strings.Contains(out, "q1") {
		t.Fatalf("queues: %q", out)
	}
}

func TestAppEngineTrafficSplit(t *testing.T) {
	s := newTestSession()
	run(t, s, "gcloud services enable appengine.googleapis.com")
	if r := s.Exec("gcloud app deploy"); r.Exit == 0 {
		t.Fatal("deploy without app should fail")
	}
	run(t, s, "gcloud app create --region=europe-west")
	s.Files[s.path("app.yaml")] = "runtime: python312\n"
	s.Files[s.path("main.py")] = "def index():\n    return 'v1'\n"
	run(t, s, "gcloud app deploy --version=v1 --quiet")
	s.Files[s.path("main.py")] = "def index():\n    return 'v2'\n"
	out := run(t, s, "gcloud app deploy --version=v2 --no-promote --quiet")
	if !strings.Contains(out, "without receiving traffic") {
		t.Fatalf("no-promote: %q", out)
	}
	host := s.State.Projects["lab-proj"].AppEngine.Host
	if r := s.Exec("curl -s https://" + host); strings.TrimSpace(r.Output) != "v1" {
		t.Fatalf("v1 should serve: %q", r.Output)
	}
	if r := s.Exec("curl -s https://v2-dot-" + host); strings.TrimSpace(r.Output) != "v2" {
		t.Fatalf("version URL: %q", r.Output)
	}
	run(t, s, "gcloud app services set-traffic default --splits=v1=0.5,v2=0.5 --quiet")
	seen := map[string]bool{}
	for i := 0; i < 30; i++ {
		seen[strings.TrimSpace(s.Exec("curl -s https://"+host).Output)] = true
	}
	if !seen["v1"] || !seen["v2"] {
		t.Fatalf("split not applied: %v", seen)
	}
	if r := s.Exec("gcloud app versions delete v1 --quiet"); r.Exit == 0 {
		t.Fatal("deleting a serving version should fail")
	}
	if out := run(t, s, "gcloud app versions list"); !strings.Contains(out, "0.50") {
		t.Fatalf("versions: %q", out)
	}
}
