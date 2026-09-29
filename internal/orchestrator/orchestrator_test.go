package orchestrator

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/cli"
	"github.com/neodevesp/gcp-trainer/internal/fidelity"
	"github.com/neodevesp/gcp-trainer/internal/grader"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
	"github.com/neodevesp/gcp-trainer/internal/store"
)

const testLabID = "ace-d03-secure-bucket"

func loadLabs(t *testing.T) map[string]*scenario.Lab {
	t.Helper()
	root, _ := filepath.Abs("../../content")
	scenario.BaselineDir = filepath.Join(root, "baselines")
	grader.PolicyDir = filepath.Join(root, "policies")
	labs, err := scenario.LoadAll(filepath.Join(root, "labs"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*scenario.Lab{}
	for _, l := range labs {
		out[l.ID] = l
	}
	if out[testLabID] == nil {
		t.Fatalf("lab %s not found", testLabID)
	}
	return out
}

func newService(t *testing.T, labs map[string]*scenario.Lab) (*Service, store.Store) {
	t.Helper()
	st, err := store.NewMemory(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	svc := New(labs, fidelity.NewRouter(), st)
	t.Cleanup(svc.Close)
	return svc, st
}

// The API and the lab plane talk over HTTP with a shared token; grading can be
// delegated to an isolated worker. The results must match in-process grading.
func TestSplitPlanesAndRemoteGrader(t *testing.T) {
	labs := loadLabs(t)
	svc, _ := newService(t, labs)
	lp := httptest.NewServer(Handler(svc, "s3cret"))
	defer lp.Close()
	worker := httptest.NewServer(HandleGrade(labs, nil, nil))
	defer worker.Close()

	// wrong token is rejected
	resp, err := http.Post(lp.URL+"/lab/sessions", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d without token", resp.StatusCode)
	}
	resp.Body.Close()

	c := &Client{Base: lp.URL, Token: "s3cret"}
	info, err := c.Start(StartRequest{UserID: "u1", LabID: testLabID, Seed: 11, AttemptID: "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Fidelity != "F0" || info.Project == "" || info.Status != "running" {
		t.Fatalf("info %+v", info)
	}
	before, err := c.Grade(info.ID, grader.Submission{})
	if err != nil {
		t.Fatal(err)
	}
	if r, err := c.Exec(info.ID, "gcloud config list"); err != nil || r.Exit != 0 {
		t.Fatalf("exec %+v %v", r, err)
	}
	// apply the reference solution
	svc.mu.Lock()
	sol := svc.sessions[info.ID].Lab.Solution
	svc.mu.Unlock()
	if strings.TrimSpace(sol) == "" {
		t.Fatal("lab has no solution")
	}
	if _, err := c.Exec(info.ID, sol); err != nil {
		t.Fatal(err)
	}
	local, err := c.Grade(info.ID, grader.Submission{})
	if err != nil {
		t.Fatal(err)
	}
	if local.Score <= before.Score {
		t.Fatalf("solution did not improve the score: %d -> %d", before.Score, local.Score)
	}
	svc.GraderURL = worker.URL
	remote, err := c.Grade(info.ID, grader.Submission{})
	if err != nil {
		t.Fatal(err)
	}
	if remote.Score != local.Score || remote.Passed != local.Passed {
		t.Fatalf("remote grade %d/%v differs from local %d/%v", remote.Score, remote.Passed, local.Score, local.Passed)
	}
	// views, files, hints and export work across the wire
	if v, err := c.View(info.ID, "console", nil); err != nil || v == nil {
		t.Fatalf("view %v %v", v, err)
	}
	if err := c.PutFile(info.ID, "notes.md", "hello"); err != nil {
		t.Fatal(err)
	}
	if r, _ := c.Exec(info.ID, "cat notes.md"); !strings.Contains(r.Output, "hello") {
		t.Fatalf("file not visible in the shell: %q", r.Output)
	}
	if h, err := c.Hint(info.ID, 0); err != nil || h.Text == "" {
		t.Fatalf("hint %+v %v", h, err)
	}
	if b, err := c.Export(info.ID); err != nil || len(b) == 0 {
		t.Fatalf("export %d %v", len(b), err)
	}
	if err := c.Stop(info.ID, "submitted"); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Info(info.ID); got.Status != "submitted" {
		t.Fatalf("status after stop %q", got.Status)
	}
}

// Sessions expire on the server regardless of the browser, and are restored
// from the store after a lab-plane restart.
func TestJanitorAndRestore(t *testing.T) {
	labs := loadLabs(t)
	svc, st := newService(t, labs)
	now := time.Now()
	svc.Now = func() time.Time { return now }
	var expired []string
	svc.OnExpire = func(i SessionInfo) { expired = append(expired, i.ID) }

	info, err := svc.Start(StartRequest{UserID: "u1", LabID: testLabID, Seed: 5})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Exec(info.ID, "gcloud storage buckets list"); err != nil {
		t.Fatal(err)
	}
	svc.Close() // flushes sessions to the store

	// a new lab plane on the same store resumes the session
	svc2 := New(labs, fidelity.NewRouter(), st)
	svc2.Now = func() time.Time { return now }
	svc2.OnExpire = svc.OnExpire
	defer svc2.Close()
	got, err := svc2.Info(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != info.Project || got.Status != "running" {
		t.Fatalf("restored %+v", got)
	}
	hist, err := svc2.View(info.ID, "history", nil)
	if err != nil {
		t.Fatal(err)
	}
	if recs, ok := hist.([]cli.ExecRecord); !ok || len(recs) == 0 {
		t.Fatalf("history lost on restore: %#v", hist)
	}

	if n := svc2.Janitor(); n != 0 {
		t.Fatalf("expired %d sessions before TTL", n)
	}
	now = got.Expires.Add(time.Second)
	if n := svc2.Janitor(); n != 1 || len(expired) != 1 {
		t.Fatalf("janitor n=%d expired=%v", n, expired)
	}
	if got, _ := svc2.Info(info.ID); got.Status != "expired" {
		t.Fatalf("status %q", got.Status)
	}
}
