package api

import (
	"testing"

	"github.com/neodevesp/gcp-trainer/internal/learning"
	"github.com/neodevesp/gcp-trainer/internal/orchestrator"
	"github.com/neodevesp/gcp-trainer/internal/store"
)

// The tutor walks a lab step by step: it must be turned on explicitly (the
// attempt then counts as guided) and it notices the steps the learner runs.
func TestTutorFlow(t *testing.T) {
	ts, srv := newTestServer(t)
	c := &client{t: t, base: ts.URL}
	var auth struct {
		Token string `json:"token"`
	}
	c.do("POST", "/api/auth/register", map[string]string{"email": "tu@example.com", "name": "Tu", "password": "correct horse battery"}, &auth)
	c.token = auth.Token
	var info orchestrator.SessionInfo
	if code := c.do("POST", "/api/labs/ace-d09-health-check/start", map[string]string{}, &info); code != 200 {
		t.Fatalf("start: %d", code)
	}
	if code := c.do("GET", "/api/sessions/"+info.ID+"/views/tutor", nil, nil); code != 403 {
		t.Fatalf("tutor view before turning it on: %d", code)
	}
	if code := c.do("POST", "/api/sessions/"+info.ID+"/tutor", map[string]string{}, nil); code != 200 {
		t.Fatalf("turn on: %d", code)
	}
	var plan struct {
		Intro string `json:"intro"`
		Steps []struct {
			N       int    `json:"n"`
			Command string `json:"command"`
			Note    string `json:"note"`
		} `json:"steps"`
		Done []int `json:"done"`
	}
	c.do("GET", "/api/sessions/"+info.ID+"/views/tutor", nil, &plan)
	if plan.Intro == "" || len(plan.Steps) < 3 || len(plan.Done) != 0 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	c.do("POST", "/api/sessions/"+info.ID+"/exec", map[string]string{"line": plan.Steps[0].Command}, nil)
	c.do("GET", "/api/sessions/"+info.ID+"/views/tutor", nil, &plan)
	if len(plan.Done) != 1 || plan.Done[0] != 1 {
		t.Fatalf("first step should be detected: %v", plan.Done)
	}
	all, err := store.ListAs[learning.Attempt](srv.Store, "attempts")
	if err != nil || len(all) != 1 || !all[0].Tutor || !all[0].SolutionShown {
		t.Fatalf("attempt should be marked as tutored: %+v %v", all, err)
	}
}
