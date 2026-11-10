package cli

import (
	"strings"
	"testing"
)

// The usual request: add a domain to a reCAPTCHA key without losing the
// ones it has, and without https:// (the API rejects it).
func TestRecaptchaKeys(t *testing.T) {
	s := newTestSession()
	if s.Exec("gcloud recaptcha keys create --display-name=web-previos --web --domains=app.zentra.dev --key-id=6LeTestKeyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA").Exit != 0 {
		t.Fatal("create failed")
	}
	out := run(t, s, "gcloud recaptcha keys list")
	if !strings.Contains(out, "6LeTestKey") || !strings.Contains(out, "app.zentra.dev") {
		t.Fatalf("list: %s", out)
	}
	if r := s.Exec("gcloud recaptcha keys update 6LeTestKeyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA --web --domains=https://ficheros.example.com"); r.Exit == 0 || !strings.Contains(r.Output, "without https://") {
		t.Fatalf("a URL is not a domain: %s", r.Output)
	}
	run(t, s, "gcloud recaptcha keys update 6LeTestKeyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA --web --domains=ficheros.example.com")
	out = run(t, s, "gcloud recaptcha keys describe 6LeTestKeyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	if strings.Contains(out, "app.zentra.dev") || !strings.Contains(out, "ficheros.example.com") {
		t.Fatalf("--domains replaces the list: %s", out)
	}
	run(t, s, "gcloud recaptcha keys update 6LeTestKeyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA --web --domains=app.zentra.dev,ficheros.example.com")
	out = run(t, s, "gcloud recaptcha keys describe 6LeTestKeyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA --format='value(webSettings.allowedDomains)'")
	if !strings.Contains(out, "app.zentra.dev") || !strings.Contains(out, "ficheros.example.com") {
		t.Fatalf("both domains: %s", out)
	}
	if r := s.Exec("gcloud recaptcha keys describe 6LnoSuchKey"); r.Exit == 0 || !strings.Contains(r.Output, "NOT_FOUND") {
		t.Fatalf("missing key: %s", r.Output)
	}
	auto := run(t, s, "gcloud recaptcha keys create --display-name=otra --web --domains=a.example.com")
	if !strings.Contains(auto, "Created key [6L") {
		t.Fatalf("generated id: %s", auto)
	}
}
