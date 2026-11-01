package orchestrator_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/neodevesp/gcp-trainer/internal/orchestrator"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
	"github.com/neodevesp/gcp-trainer/internal/tutor"
)

// Every lab gets a graded ladder of help, and the first levels never give
// the answer away: no level before the example command contains a change
// from the solution, and the derived example command hides the real values.
func TestHintLadderNeverLeaksEarly(t *testing.T) {
	ladders := 0
	for _, dir := range []string{"../../content/labs", "../../content/company/missions"} {
		root, _ := filepath.Abs(dir)
		labs, err := scenario.LoadAll(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, base := range labs {
			for _, lang := range []string{"es", "en"} {
				l, _, err := base.Localized(lang).Variant(1, "lab-proj")
				if err != nil {
					if strings.Contains(dir, "company") {
						continue // missions need the company's parameters
					}
					t.Fatal(err)
				}
				hs := orchestrator.HintLadder(l, lang)
				if l.Type == "boss" {
					if len(hs) != 0 {
						t.Errorf("%s: boss battles have no hints", l.ID)
					}
					continue
				}
				if len(hs) == 0 || hs[0].Level == 0 {
					continue // no ladder: the lab's own hints are used
				}
				ladders++
				authored4 := false
				for _, h := range l.Hints {
					if h.Kind == "partial" || strings.Contains(h.Text, "`") {
						authored4 = true
					}
				}
				plan := tutor.Build(l.Solution, nil, lang)
				prev := 0
				for _, h := range hs {
					if h.Level <= prev || h.Level > 5 {
						t.Errorf("%s/%s: levels out of order: %d after %d", l.ID, lang, h.Level, prev)
					}
					prev = h.Level
					for _, st := range plan.Steps {
						if st.Phase != tutor.Fix {
							continue
						}
						c := strings.TrimSpace(strings.SplitN(st.Command, "\n", 2)[0])
						if len(c) < 16 {
							continue
						}
						if h.Level <= 3 && strings.Contains(h.Text, c) {
							t.Errorf("%s/%s: level %d gives away the fix %q", l.ID, lang, h.Level, c)
						}
						if h.Level == 4 && !authored4 && strings.Contains(h.Text, c) && strings.Contains(c, "=") {
							t.Errorf("%s/%s: the example command shows the real values %q", l.ID, lang, c)
						}
					}
				}
				if last := hs[len(hs)-1]; l.Solution != "" && last.Kind != "solution" && last.Level == 5 {
					t.Errorf("%s/%s: level 5 must be the solution", l.ID, lang)
				}
			}
		}
	}
	if ladders < 100 {
		t.Fatalf("only %d labs have a ladder", ladders)
	}
	t.Logf("%d lab/language ladders checked", ladders)
}

func TestMaskHidesValues(t *testing.T) {
	cases := map[string]string{
		"gcloud projects add-iam-policy-binding zentra-prod --member=user:ana@zentra.dev --role=roles/storage.objectViewer": "gcloud projects add-iam-policy-binding … --member=… --role=…",
		"gcloud compute firewall-rules create allow-hc --network default --allow tcp:80 --source-ranges=35.191.0.0/16":      "gcloud compute firewall-rules create … --network=… --allow=… --source-ranges=…",
		"kubectl scale deployment web --replicas=3": "kubectl scale … … --replicas=…",
	}
	for in, want := range cases {
		if got := tutor.Mask(in); got != want {
			t.Errorf("Mask(%q)\n got  %q\n want %q", in, got, want)
		}
	}
}
