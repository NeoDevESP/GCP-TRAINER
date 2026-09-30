package api

import (
	"encoding/json"
	"testing"
)

// Study progress is stored per user: it survives across browsers, a best
// score never goes down, exam history is newest first and deduplicated, and
// a practice exam in progress can be saved and is dropped once handed in.
func TestStudyProgress(t *testing.T) {
	ts, _ := newTestServer(t)
	c := &client{t: t, base: ts.URL}
	var auth struct {
		Token string `json:"token"`
	}
	c.do("POST", "/api/auth/register", map[string]string{"email": "st@example.com", "name": "St", "password": "correct horse battery"}, &auth)
	c.token = auth.Token

	var st Study
	if code := c.do("GET", "/api/me/study", nil, &st); code != 200 || len(st.Progress) != 0 || len(st.Exams) != 0 {
		t.Fatalf("empty study: %d %+v", code, st)
	}
	if code := c.do("POST", "/api/me/study/progress", map[string]any{"iam": map[string]any{"seen": []string{"roles", "roles", "nope"}, "best": 80}}, nil); code != 200 {
		t.Fatalf("save progress: %d", code)
	}
	c.do("POST", "/api/me/study/progress", map[string]any{"iam": map[string]any{"seen": []string{"roles", "sa"}, "best": 40}}, nil)
	c.do("GET", "/api/me/study", nil, &st)
	p := st.Progress["iam"]
	if len(p.Seen) != 2 || p.Best == nil || *p.Best != 80 {
		t.Fatalf("progress not merged as expected: %+v", p)
	}
	if code := c.do("POST", "/api/me/study/progress", map[string]any{"no-such-topic": map[string]any{}}, nil); code != 400 {
		t.Fatalf("unknown topic accepted: %d", code)
	}

	run := json.RawMessage(`{"qs":[],"cur":3}`)
	if code := c.do("PUT", "/api/me/study/runs/cm-exam-run-ACE-full", run, nil); code != 200 {
		t.Fatalf("save run: %d", code)
	}
	if code := c.do("PUT", "/api/me/study/runs/../../etc", run, nil); code == 200 {
		t.Fatal("bad run key accepted")
	}
	c.do("GET", "/api/me/study", nil, &st)
	if len(st.Runs) != 1 {
		t.Fatalf("run not saved: %+v", st.Runs)
	}
	exam := map[string]any{"cert": "ACE", "at": 1000, "score": 30, "total": 50, "full": true, "seconds": 3600, "domains": []any{}}
	c.do("POST", "/api/me/study/exams", exam, nil)
	c.do("POST", "/api/me/study/exams", exam, nil) // retried upload
	exam["at"], exam["score"] = 2000, 40
	c.do("POST", "/api/me/study/exams", exam, nil)
	if code := c.do("POST", "/api/me/study/exams", map[string]any{"cert": "XYZ", "total": 10}, nil); code != 400 {
		t.Fatalf("unknown cert accepted: %d", code)
	}
	st = Study{}
	c.do("GET", "/api/me/study", nil, &st)
	if len(st.Exams) != 2 || st.Exams[0].Score != 40 || len(st.Runs) != 0 {
		t.Fatalf("exam history: %+v runs %v", st.Exams, st.Runs)
	}

	// Another user sees nothing of it.
	c.token = ""
	c.do("POST", "/api/auth/register", map[string]string{"email": "other@example.com", "name": "O", "password": "correct horse battery"}, &auth)
	c.token = auth.Token
	var other Study
	c.do("GET", "/api/me/study", nil, &other)
	if len(other.Progress) != 0 || len(other.Exams) != 0 {
		t.Fatalf("study leaked across users: %+v", other)
	}
}
