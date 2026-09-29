package cli

import (
	"strings"
	"testing"
)

func TestInteractiveSSH(t *testing.T) {
	s := newTestSession()
	run(t, s, "gcloud compute instances create web-1 --zone=europe-west1-b")
	r := s.Exec("gcloud compute ssh web-1 --zone=europe-west1-b")
	if r.Exit != 0 || r.Prompt != "student@web-1:~$ " || !strings.Contains(r.Output, "Linux web-1") {
		t.Fatalf("ssh: %+v", r)
	}
	if r = s.Exec("hostname"); strings.TrimSpace(r.Output) != "web-1" {
		t.Fatalf("hostname in the VM: %q", r.Output)
	}
	if r = s.Exec("sudo -i"); r.Prompt != "root@web-1:~# " {
		t.Fatalf("root prompt: %q", r.Prompt)
	}
	s.Exec("apt-get install -y nginx")
	s.Exec("exit")
	if r = s.Exec("systemctl status nginx"); !strings.Contains(r.Output, "active (running)") {
		t.Fatalf("nginx not installed: %q", r.Output)
	}
	if r = s.Exec("exit"); s.Remote != nil || !strings.Contains(r.Output, "Connection to web-1 closed") || r.Prompt != "student@cloudshell:~ (lab-proj)$ " {
		t.Fatalf("exit: %+v", r)
	}
	// Recorded as the non-interactive command, which is what grading reads.
	found := false
	for _, rec := range s.Records {
		if strings.HasPrefix(rec.Line, "gcloud compute ssh web-1 --zone=europe-west1-b --command='sudo apt-get install -y nginx'") {
			found = true
		}
	}
	if !found {
		t.Fatalf("ssh line not recorded: %+v", s.Records)
	}
	// Lines after exit in the same input run in Cloud Shell.
	r = s.Exec("gcloud compute ssh web-1 --zone=europe-west1-b\nhostname\nexit\ngcloud config get-value project")
	if !strings.Contains(r.Output, "web-1\n") || !strings.Contains(r.Output, "lab-proj") || s.Remote != nil {
		t.Fatalf("mixed input: %q", r.Output)
	}
}

func TestInteractiveSQLConnect(t *testing.T) {
	s := newTestSession()
	run(t, s, "gcloud sql instances create db1 --database-version=POSTGRES_15 --tier=db-g1-small --region=europe-west1")
	run(t, s, "gcloud sql databases create shop --instance=db1")
	r := s.Exec("gcloud sql connect db1 --user=postgres --database=shop")
	if r.Exit != 0 || r.Prompt != "shop=> " {
		t.Fatalf("connect: %+v", r)
	}
	s.Exec("CREATE TABLE t (")
	if r = s.Exec("  id INT PRIMARY KEY, name TEXT"); r.Prompt != "shop-> " {
		t.Fatalf("continuation prompt: %q", r.Prompt)
	}
	if r = s.Exec(");"); !strings.Contains(r.Output, "CREATE TABLE") {
		t.Fatalf("create: %q", r.Output)
	}
	s.Exec("INSERT INTO t VALUES (1, 'a'), (2, 'b');")
	if r = s.Exec("SELECT count(*) AS n FROM t;"); !strings.Contains(r.Output, " 2\n(1 row)") {
		t.Fatalf("select: %q", r.Output)
	}
	if r = s.Exec(`\c postgres`); r.Prompt != "postgres=> " {
		t.Fatalf("\\c: %+v", r)
	}
	if r = s.Exec(`\q`); s.Remote != nil || !strings.HasPrefix(r.Prompt, "student@cloudshell") {
		t.Fatalf("quit: %+v", r)
	}
}
