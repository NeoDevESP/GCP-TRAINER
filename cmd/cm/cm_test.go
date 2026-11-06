package main

import (
	"bufio"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/api"
	"github.com/neodevesp/gcp-trainer/internal/company"
	"github.com/neodevesp/gcp-trainer/internal/fidelity"
	"github.com/neodevesp/gcp-trainer/internal/grader"
	"github.com/neodevesp/gcp-trainer/internal/learning"
	"github.com/neodevesp/gcp-trainer/internal/orchestrator"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
	"github.com/neodevesp/gcp-trainer/internal/store"
)

// server starts the real API in memory with a signed-in account, and points
// cm's config at it.
func server(t *testing.T) *Config {
	t.Helper()
	root, _ := filepath.Abs("../../content")
	scenario.BaselineDir = filepath.Join(root, "baselines")
	grader.PolicyDir = filepath.Join(root, "policies")
	cat, err := learning.LoadCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.NewMemory(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	co, err := company.Load(filepath.Join(root, "company"), "nebula")
	if err != nil {
		t.Fatal(err)
	}
	svc := orchestrator.New(cat.Labs, fidelity.NewRouter(), st)
	svc.Company = co
	srv := &api.Server{Cat: cat, Engine: &learning.Engine{Cat: cat}, Store: st, Labs: svc, Tokens: &learning.Tokens{Secret: []byte("t"), TTL: time.Hour}, Company: co}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	t.Setenv("CM_CONFIG", filepath.Join(t.TempDir(), "cm.json"))
	t.Setenv("CM_SERVER", "")
	c := &Config{Server: ts.URL}
	var r struct {
		Token string `json:"token"`
	}
	if err := c.call("POST", "/api/auth/register", map[string]string{"email": "cm@example.com", "name": "Cm", "password": "correct horse battery"}, &r); err != nil {
		t.Fatal(err)
	}
	c.Token = r.Token
	if err := c.save(); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestQuoteKeepsArguments(t *testing.T) {
	got := quote([]string{"gcloud", "compute", "instances", "list", "--filter=labels.team=pagos AND status=RUNNING", "--format=value(name)", "it's"})
	want := `gcloud compute instances list '--filter=labels.team=pagos AND status=RUNNING' '--format=value(name)' 'it'\''s'`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

// Commands run in the free console (opened on demand) with their exit code;
// files go back and forth.
func TestRunAndFiles(t *testing.T) {
	c := server(t)
	sid, err := session(c, true)
	if err != nil || sid == "" {
		t.Fatalf("a free console opens on demand: %v", err)
	}
	r, err := execLine(*c, sid, "gcloud compute instances create web-1 --zone=europe-west1-b")
	if err != nil || r.Exit != 0 {
		t.Fatalf("create: %v %+v", err, r)
	}
	r, _ = execLine(*c, sid, quote([]string{"gcloud", "compute", "instances", "list", "--format=value(name)"}))
	if strings.TrimSpace(r.Output) != "web-1" {
		t.Fatalf("list: %q", r.Output)
	}
	if r, _ := execLine(*c, sid, "gcloud compute instances describe nope --zone=europe-west1-b"); r.Exit == 0 {
		t.Fatal("a failing command must keep its exit code")
	}
	// the same session is found again from the saved config
	c2 := loadConfig()
	if sid2, _ := session(&c2, false); sid2 != sid {
		t.Fatalf("session not remembered: %q vs %q", sid2, sid)
	}
}

// The MCP server answers the protocol and its tools reach the simulator.
func TestMCP(t *testing.T) {
	c := server(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"start_free_console","arguments":{"world":"empty"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"a.txt","content":"hola"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"run_command","arguments":{"command":"cat a.txt"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"run_command","arguments":{"command":"gcloud compute networks describe nope"}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"unknown"}`,
	}, "\n")
	var out strings.Builder
	if err := serveMCP(strings.NewReader(in), &out, c); err != nil {
		t.Fatal(err)
	}
	res := map[int]map[string]any{}
	sc := bufio.NewScanner(strings.NewReader(out.String()))
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("bad JSON line: %s", sc.Text())
		}
		res[int(m["id"].(float64))] = m
	}
	if len(res) != 7 {
		t.Fatalf("one answer per request (none for the notification): %d", len(res))
	}
	if v := res[1]["result"].(map[string]any)["protocolVersion"]; v != "2025-06-18" {
		t.Fatalf("initialize: %v", res[1])
	}
	if n := len(res[2]["result"].(map[string]any)["tools"].([]any)); n != len(mcpTools) {
		t.Fatalf("tools/list: %d", n)
	}
	text := func(id int) (string, bool) {
		r := res[id]["result"].(map[string]any)
		return r["content"].([]any)[0].(map[string]any)["text"].(string), r["isError"].(bool)
	}
	if s, bad := text(3); bad || !strings.Contains(s, "mi-sandbox") {
		t.Fatalf("start: %s", s)
	}
	if s, bad := text(5); bad || !strings.Contains(s, "hola") {
		t.Fatalf("run cat: %s", s)
	}
	if _, bad := text(6); !bad {
		t.Fatal("a failing command is reported as an error")
	}
	if res[7]["error"] == nil {
		t.Fatal("unknown method must be an error")
	}
}

// The editor files are merged, never clobbered when they have comments.
func TestVSCodeConfigMerge(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	os.WriteFile(p, []byte(`{"editor.fontSize": 15}`), 0o644)
	if err := merge(p, func(m map[string]any) { m["x"] = 1 }); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "editor.fontSize") || !strings.Contains(string(b), `"x"`) {
		t.Fatalf("merge lost settings: %s", b)
	}
	withComments := filepath.Join(dir, "c.json")
	orig := "{\n  // mine\n  \"a\": 1\n}"
	os.WriteFile(withComments, []byte(orig), 0o644)
	if err := merge(withComments, func(m map[string]any) {}); err == nil {
		t.Fatal("a file with comments must not be rewritten")
	}
	if b, _ := os.ReadFile(withComments); string(b) != orig {
		t.Fatal("file with comments changed")
	}
}
