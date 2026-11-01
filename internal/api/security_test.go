package api

import (
	"bytes"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The web session lives in an HttpOnly cookie; state changes made with the
// cookie need the CSRF header; the token is never accepted in the URL.
func TestCookieSessionAndCSRF(t *testing.T) {
	ts, _ := newTestServer(t)
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{Jar: jar}
	req := func(method, path, body string, hdr map[string]string) *http.Response {
		r, _ := http.NewRequest(method, ts.URL+path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		res, err := hc.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res
	}
	res := req("POST", "/api/auth/register", `{"email":"ck@example.com","name":"Ck","password":"correct horse battery"}`, nil)
	var ck *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == SessionCookie {
			ck = c
		}
	}
	if ck == nil || !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode || ck.Value == "" {
		t.Fatalf("register must set an HttpOnly, SameSite=Strict session cookie: %+v", ck)
	}
	if r := req("GET", "/api/me", "", nil); r.StatusCode != 200 {
		t.Fatalf("GET with the cookie: %d", r.StatusCode)
	}
	if r := req("PUT", "/api/me/study/runs/cm-plan", `{"x":1}`, nil); r.StatusCode != 403 {
		t.Fatalf("a change with the cookie and no CSRF header must be refused: %d", r.StatusCode)
	}
	if r := req("PUT", "/api/me/study/runs/cm-plan", `{"x":1}`, map[string]string{CSRFHeader: "web"}); r.StatusCode != 200 {
		t.Fatalf("a change with the cookie and the CSRF header: %d", r.StatusCode)
	}
	// the token in the URL is not a credential
	plain := &http.Client{}
	r, _ := plain.Get(ts.URL + "/api/me?token=" + ck.Value)
	if r.StatusCode != 401 {
		t.Fatalf("?token= must not authenticate: %d", r.StatusCode)
	}
	// Bearer still works for API clients
	rq, _ := http.NewRequest("GET", ts.URL+"/api/me", nil)
	rq.Header.Set("Authorization", "Bearer "+ck.Value)
	if r, _ := plain.Do(rq); r.StatusCode != 200 {
		t.Fatalf("Bearer: %d", r.StatusCode)
	}
	// logout clears the cookie
	req("POST", "/api/auth/logout", "", map[string]string{CSRFHeader: "web"})
	if r := req("GET", "/api/me", "", nil); r.StatusCode != 401 {
		t.Fatalf("after logout: %d", r.StatusCode)
	}
}

// The web pages carry the security headers, CSP included.
func TestSecurityHeaders(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o644)
	_, srv := newTestServer(t)
	srv.WebDir = dir
	ts := httpTestServer(t, srv)
	r, err := http.Get(ts + "/")
	if err != nil {
		t.Fatal(err)
	}
	h := r.Header
	csp := h.Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'self'", "frame-ancestors 'none'", "object-src 'none'", "base-uri 'self'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP lacks %q: %s", want, csp)
		}
	}
	for k, v := range map[string]string{"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY", "Referrer-Policy": "strict-origin-when-cross-origin"} {
		if h.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, h.Get(k), v)
		}
	}
}

func httpTestServer(t *testing.T, s *Server) string {
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts.URL
}
