package cli

import (
	"strings"
	"testing"
)

// The platform's own commands speak Spanish by default and English on
// request; real tool output (gcloud…) stays in English in both.
func TestPlatformCommandsLanguage(t *testing.T) {
	for _, tc := range []struct {
		lang, help, why, gcloud string
	}{
		{"", "Herramientas disponibles", "Cadena causal", "Created"},
		{"es", "Herramientas disponibles", "Primer eslabón roto", "Created"},
		{"en", "Available tools", "First broken link", "Created"},
	} {
		s := newTestSession()
		s.Lang = tc.lang
		if out := run(t, s, "help"); !strings.Contains(out, tc.help) {
			t.Errorf("[%s] help: want %q", tc.lang, tc.help)
		}
		run(t, s, "gcloud config set compute/zone europe-west1-b")
		out := run(t, s, "gcloud compute instances create web-1 --machine-type=e2-small --metadata=startup-script='apt-get install -y nginx'")
		if !strings.Contains(out, tc.gcloud) {
			t.Errorf("[%s] gcloud output must stay in English, got %q", tc.lang, out)
		}
		out = run(t, s, "why vm:web-1:80")
		if !strings.Contains(out, tc.why) {
			t.Errorf("[%s] why: want %q in %q", tc.lang, tc.why, out)
		}
	}
}
