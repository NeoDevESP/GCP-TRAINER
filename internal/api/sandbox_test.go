package api

import (
	"strings"
	"testing"
)

// The free console: any world starts, commands work with owner rights, the
// world can be saved and continued, it is never graded and leaves no attempt.
func TestSandbox(t *testing.T) {
	ts, _ := newTestServer(t)
	c := &client{t: t, base: ts.URL}
	var auth struct {
		Token string `json:"token"`
	}
	c.do("POST", "/api/auth/register", map[string]string{"email": "sb@example.com", "name": "Sb", "password": "correct horse battery"}, &auth)
	c.token = auth.Token

	var list struct {
		Worlds []map[string]string `json:"worlds"`
	}
	c.do("GET", "/api/sandbox", nil, &list)
	if len(list.Worlds) < 5 {
		t.Fatalf("worlds: %+v", list.Worlds)
	}
	type info struct {
		ID, LabID, Project string
	}
	start := func(body map[string]any) info {
		var i info
		if code := c.do("POST", "/api/sandbox/start", body, &i); code != 200 {
			t.Fatalf("start %v: %d", body, code)
		}
		return i
	}
	exec := func(id, line string) (string, int) {
		var out struct {
			Output string `json:"output"`
			Exit   int    `json:"exit"`
		}
		c.do("POST", "/api/sessions/"+id+"/exec", map[string]string{"line": line}, &out)
		return out.Output, out.Exit
	}
	// every world provisions
	for _, w := range list.Worlds {
		s := start(map[string]any{"world": w["id"]})
		if !strings.HasPrefix(s.LabID, "sandbox-") || s.Project != "mi-sandbox" {
			t.Fatalf("%s: %+v", w["id"], s)
		}
	}
	s := start(map[string]any{"world": "default-network"})
	if out, code := exec(s.ID, "gcloud compute instances create web-1 --zone=europe-west1-b"); code != 0 {
		t.Fatalf("owner can create: %s", out)
	}
	if code := c.do("POST", "/api/sandbox/save", nil, nil); code != 200 {
		t.Fatalf("save: %d", code)
	}
	if code := c.do("POST", "/api/sessions/"+s.ID+"/submit", map[string]any{}, nil); code != 400 {
		t.Fatalf("the free console must not be graded: %d", code)
	}
	fresh := start(map[string]any{"world": "default-network"})
	if out, _ := exec(fresh.ID, "gcloud compute instances list"); strings.Contains(out, "web-1") {
		t.Fatal("a new world must start clean")
	}
	again := start(map[string]any{"resume": true})
	if out, _ := exec(again.ID, "gcloud compute instances list"); !strings.Contains(out, "web-1") {
		t.Fatalf("the saved world must come back: %s", out)
	}
	var atts []map[string]any
	c.do("GET", "/api/me/attempts", nil, &atts)
	if len(atts) != 0 {
		t.Fatalf("the free console must leave no attempt: %d", len(atts))
	}
	if code := c.do("POST", "/api/sandbox/start", map[string]any{"world": "nope"}, nil); code != 400 {
		t.Fatalf("unknown world: %d", code)
	}
}
