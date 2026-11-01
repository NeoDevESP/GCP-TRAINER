package api

import "testing"

// The learning mode is enforced by the server, not only by hiding buttons:
// independent attempts get no hints and no tutor, production attempts get
// hints but never the solution or the tutor, assisted attempts get both.
func TestLearningModesAreEnforced(t *testing.T) {
	ts, _ := newTestServer(t)
	c := &client{t: t, base: ts.URL}
	var auth struct {
		Token string `json:"token"`
	}
	c.do("POST", "/api/auth/register", map[string]string{"email": "modes@example.com", "name": "M", "password": "correct horse battery"}, &auth)
	c.token = auth.Token
	start := func(mode string) string {
		var info struct {
			ID        string `json:"id"`
			LearnMode string `json:"learnMode"`
		}
		if code := c.do("POST", "/api/labs/ace-d03-secure-bucket/start", map[string]string{"mode": mode}, &info); code != 200 || info.LearnMode != mode {
			t.Fatalf("start %s: %d %+v", mode, code, info)
		}
		return info.ID
	}
	if code := c.do("POST", "/api/labs/ace-d03-secure-bucket/start", map[string]string{"mode": "cheat"}, nil); code != 400 {
		t.Fatalf("unknown mode accepted: %d", code)
	}
	ind := start("independent")
	if code := c.do("POST", "/api/sessions/"+ind+"/hint", map[string]string{}, nil); code != 403 {
		t.Fatalf("independent: hint allowed (%d)", code)
	}
	if code := c.do("POST", "/api/sessions/"+ind+"/tutor", map[string]string{}, nil); code != 403 {
		t.Fatalf("independent: tutor allowed (%d)", code)
	}
	c.do("POST", "/api/sessions/"+ind+"/stop", map[string]string{}, nil)

	prod := start("production")
	if code := c.do("POST", "/api/sessions/"+prod+"/tutor", map[string]string{}, nil); code != 403 {
		t.Fatalf("production: tutor allowed (%d)", code)
	}
	gotSolution := false
	for i := 0; i < 10; i++ {
		var h struct {
			Kind string `json:"kind"`
		}
		code := c.do("POST", "/api/sessions/"+prod+"/hint", map[string]string{}, &h)
		if code != 200 { // 403 at the solution, 400 when the hints run out
			break
		}
		if h.Kind == "solution" {
			gotSolution = true
		}
	}
	if gotSolution {
		t.Fatal("production mode revealed the solution")
	}
	c.do("POST", "/api/sessions/"+prod+"/stop", map[string]string{}, nil)

	as := start("assisted")
	if code := c.do("POST", "/api/sessions/"+as+"/hint", map[string]string{}, nil); code != 200 {
		t.Fatalf("assisted: hint refused (%d)", code)
	}
}
