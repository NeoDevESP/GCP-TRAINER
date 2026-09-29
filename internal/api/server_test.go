package api

import (
	"bytes"
	"encoding/json"
	"github.com/neodevesp/gcp-trainer/internal/company"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/fidelity"
	"github.com/neodevesp/gcp-trainer/internal/grader"
	"github.com/neodevesp/gcp-trainer/internal/learning"
	"github.com/neodevesp/gcp-trainer/internal/orchestrator"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
	"github.com/neodevesp/gcp-trainer/internal/store"
)

func newTestServer(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	root, _ := filepath.Abs("../../content")
	scenario.BaselineDir = filepath.Join(root, "baselines")
	grader.PolicyDir = filepath.Join(root, "policies")
	cat, err := learning.LoadCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	lib, err := scenario.LoadLibrary(filepath.Join(root, "failures"))
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
	svc.Lib, svc.Company = lib, co
	srv := &Server{Cat: cat, Engine: &learning.Engine{Cat: cat, Lib: lib}, Store: st, Labs: svc, Tokens: &learning.Tokens{Secret: []byte("test"), TTL: time.Hour}, Lib: lib, Company: co}
	srv.LoadGenerated()
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, srv
}

type client struct {
	t     *testing.T
	base  string
	token string
}

func (c *client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, c.base+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func TestLearnerFlow(t *testing.T) {
	ts, _ := newTestServer(t)
	c := &client{t: t, base: ts.URL}
	var auth struct {
		Token string        `json:"token"`
		User  learning.User `json:"user"`
	}
	if code := c.do("POST", "/api/auth/register", map[string]string{"email": "ada@example.com", "name": "Ada", "password": "correct horse battery"}, &auth); code != 200 {
		t.Fatalf("register: %d", code)
	}
	if auth.User.Role != "admin" {
		t.Fatalf("first user should bootstrap as admin, got %s", auth.User.Role)
	}
	if code := c.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatalf("unauthenticated /api/me should be 401, got %d", code)
	}
	c.token = auth.Token

	var info orchestrator.SessionInfo
	if code := c.do("POST", "/api/labs/ace-d03-secure-bucket/start", map[string]string{}, &info); code != 200 {
		t.Fatalf("start: %d", code)
	}
	var res struct {
		Output string `json:"output"`
		Exit   int    `json:"exit"`
	}
	c.do("POST", "/api/sessions/"+info.ID+"/exec", map[string]string{"line": "gcloud config list"}, &res)
	if res.Exit != 0 {
		t.Fatalf("exec failed: %s", res.Output)
	}
	var sub map[string]any
	if code := c.do("POST", "/api/sessions/"+info.ID+"/submit", map[string]any{"evidence": map[string]string{}}, &sub); code != 200 {
		t.Fatalf("submit: %d %v", code, sub)
	}
	result := sub["result"].(map[string]any)
	if result["process"] == nil {
		t.Fatal("result should include the process assessment")
	}
	var prof learning.Profile
	c.do("GET", "/api/me", nil, &prof)
	if prof.Attempts != 1 || len(prof.Student.Dimensions) != 9 || prof.Career.Stage == "" {
		t.Fatalf("unexpected profile: attempts=%d dims=%d stage=%q", prof.Attempts, len(prof.Student.Dimensions), prof.Career.Stage)
	}
	var tr learning.Transcript
	c.do("GET", "/api/me/transcript", nil, &tr)
	var ver map[string]any
	c.do("POST", "/api/auth/verify-transcript", tr, &ver)
	if ver["valid"] != true {
		t.Fatalf("transcript should verify: %v", ver)
	}
}

func TestGeneratedIncidentFlow(t *testing.T) {
	ts, _ := newTestServer(t)
	c := &client{t: t, base: ts.URL}
	var auth struct {
		Token string `json:"token"`
	}
	c.do("POST", "/api/auth/register", map[string]string{"email": "sre@example.com", "name": "Sam", "password": "correct horse battery"}, &auth)
	c.token = auth.Token
	var lib map[string]any
	if code := c.do("GET", "/api/failures", nil, &lib); code != 200 || lib["graph"] == nil {
		t.Fatalf("failure library: %d", code)
	}
	var info orchestrator.SessionInfo
	if code := c.do("POST", "/api/incidents", scenario.GenSpec{System: "three-tier", Difficulty: 3, Seed: 5}, &info); code != 200 {
		t.Fatalf("generate: %d", code)
	}
	if info.Gen == nil || info.Type != "incident" {
		t.Fatalf("session should carry the generation spec: %+v", info)
	}
	var res struct {
		Output string `json:"output"`
		Exit   int    `json:"exit"`
	}
	c.do("POST", "/api/sessions/"+info.ID+"/exec", map[string]string{"line": "ticket"}, &res)
	if res.Exit != 0 || len(res.Output) == 0 {
		t.Fatalf("ticket command failed: %v", res)
	}
	var desk map[string]any
	c.do("GET", "/api/sessions/"+info.ID+"/views/desk", nil, &desk)
	if desk["ticket"] == nil {
		t.Fatalf("desk view should expose the ticket: %v", desk)
	}
}

func TestCompanyFlow(t *testing.T) {
	ts, _ := newTestServer(t)
	c := &client{t: t, base: ts.URL}
	var auth struct {
		Token string `json:"token"`
	}
	c.do("POST", "/api/auth/register", map[string]string{"email": "intern@example.com", "name": "Ines", "password": "correct horse battery"}, &auth)
	c.token = auth.Token
	var co map[string]any
	if code := c.do("POST", "/api/company", nil, &co); code != 200 {
		t.Fatalf("create company: %d %v", code, co)
	}
	projects := co["projects"].(map[string]any)
	var info orchestrator.SessionInfo
	if code := c.do("POST", "/api/company/missions/nb-m01-orientation/start", nil, &info); code != 200 {
		t.Fatalf("start mission: %d", code)
	}
	var res struct {
		Output string `json:"output"`
		Exit   int    `json:"exit"`
	}
	c.do("POST", "/api/sessions/"+info.ID+"/exec", map[string]string{"line": "gcloud projects list"}, &res)
	c.do("POST", "/api/sessions/"+info.ID+"/exec", map[string]string{"line": "gcloud config set project " + projects["dev-web"].(string)}, &res)
	c.do("POST", "/api/sessions/"+info.ID+"/exec", map[string]string{"line": "gcloud config set compute/region europe-west1"}, &res)
	var sub map[string]any
	if code := c.do("POST", "/api/sessions/"+info.ID+"/submit", map[string]any{"answers": map[string][]int{"q1": {0}, "q2": {1}, "q3": {1}}}, &sub); code != 200 {
		t.Fatalf("submit: %d %v", code, sub)
	}
	out, _ := sub["company"].(map[string]any)
	if out == nil || out["day"].(float64) != 2 {
		t.Fatalf("company should advance to day 2: %v", sub["company"])
	}
	c.do("GET", "/api/company", nil, &co)
	if co["day"].(float64) != 2 {
		t.Fatalf("company status should persist the day: %v", co["day"])
	}
}
