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

func TestStudyReview(t *testing.T) {
	ts, _ := newTestServer(t)
	c := &client{t: t, base: ts.URL}
	var auth struct {
		Token string `json:"token"`
	}
	c.do("POST", "/api/auth/register", map[string]string{"email": "rv@example.com", "name": "Rv", "password": "correct horse battery"}, &auth)
	c.token = auth.Token

	if code := c.do("POST", "/api/me/study/review", map[string]any{"iam:q3": map[string]any{"box": 1, "due": 1000, "misses": 1, "at": 2000}}, nil); code != 200 {
		t.Fatalf("save review: %d", code)
	}
	// an older update must not overwrite a newer one
	c.do("POST", "/api/me/study/review", map[string]any{"iam:q3": map[string]any{"box": 4, "due": 9000, "misses": 0, "at": 1000}}, nil)
	var st Study
	c.do("GET", "/api/me/study", nil, &st)
	if it := st.Review["iam:q3"]; it.Box != 1 || it.Misses != 1 {
		t.Fatalf("older update overwrote newer: %+v", it)
	}
	for _, bad := range []map[string]any{{"nope:q1": map[string]any{"box": 1}}, {"iam:x1": map[string]any{"box": 1}}, {"iam:q1": map[string]any{"box": 9}}} {
		if code := c.do("POST", "/api/me/study/review", bad, nil); code != 400 {
			t.Fatalf("invalid review accepted: %v -> %d", bad, code)
		}
	}
	// null drops a mastered question
	c.do("POST", "/api/me/study/review", map[string]any{"iam:q3": nil}, nil)
	var after Study
	c.do("GET", "/api/me/study", nil, &after)
	if _, ok := after.Review["iam:q3"]; ok {
		t.Fatal("mastered question should be removed")
	}
}

// Quiz results are kept as a history merged across devices: the latest one
// shows a drop that the best score hides, and neither device erases the other.
func TestStudyQuizHistory(t *testing.T) {
	ts, _ := newTestServer(t)
	c := &client{t: t, base: ts.URL}
	var auth struct {
		Token string `json:"token"`
	}
	c.do("POST", "/api/auth/register", map[string]string{"email": "qh@example.com", "name": "Qh", "password": "correct horse battery"}, &auth)
	c.token = auth.Token
	c.do("POST", "/api/me/study/progress", map[string]any{"iam": map[string]any{"best": 100, "scores": []any{map[string]any{"at": 1, "score": 100}}}}, nil)
	// another device that never saw the first quiz
	c.do("POST", "/api/me/study/progress", map[string]any{"iam": map[string]any{"best": 40, "scores": []any{map[string]any{"at": 2, "score": 40}, map[string]any{"at": 3, "score": 150}}}}, nil)
	var st Study
	c.do("GET", "/api/me/study", nil, &st)
	p := st.Progress["iam"]
	if len(p.Scores) != 3 || p.Scores[0].Score != 100 || p.Scores[2].Score != 100 || p.Scores[1].Score != 40 {
		t.Fatalf("scores not merged and clamped: %+v", p.Scores)
	}
	if p.Best == nil || *p.Best != 100 {
		t.Fatalf("best lost: %+v", p.Best)
	}
	many := []any{}
	for i := 10; i < 40; i++ {
		many = append(many, map[string]any{"at": i, "score": 50})
	}
	c.do("POST", "/api/me/study/progress", map[string]any{"iam": map[string]any{"scores": many}}, nil)
	c.do("GET", "/api/me/study", nil, &st)
	if n := len(st.Progress["iam"].Scores); n != maxQuizScores {
		t.Fatalf("history not capped: %d", n)
	}
}
