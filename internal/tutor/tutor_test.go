package tutor_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/neodevesp/gcp-trainer/internal/scenario"
	"github.com/neodevesp/gcp-trainer/internal/tutor"
)

// Console page ids (web/components/console/CloudConsole.tsx).
var pages = map[string]bool{}

func init() {
	for _, p := range strings.Fields(`iam sa roles orgpolicies scc apis billing budgets vm templates groups healthchecks disks snapshots gke workloads k8sservices k8sconfig run functions appengine scheduler tasks buckets sql filestore spanner firestore redis bigquery dataflow dataproc composer vertex pubsub vpc addresses firewall routes peering lb dns nat armor vpn topology secrets kms artifacts build deploy repos logs logmetrics sinks metrics alerting uptime activity`) {
		pages[p] = true
	}
}

func TestEveryLabHasAWalkthrough(t *testing.T) {
	root, _ := filepath.Abs("../../content/labs")
	labs, err := scenario.LoadAll(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, base := range labs {
		for _, lang := range []string{"es", "en"} {
			l, _, err := base.Localized(lang).Variant(1, "lab-proj")
			if err != nil {
				t.Fatal(err)
			}
			var notes []tutor.Note
			for _, n := range l.Tutor {
				notes = append(notes, tutor.Note{Match: n.Match, Title: n.Title, Why: n.Why})
			}
			plan := tutor.Build(l.Solution, notes, lang)
			for _, n := range notes {
				if strings.HasPrefix(n.Match, "@") {
					continue
				}
				re, err := regexp.Compile(n.Match)
				found := false
				for _, st := range plan.Steps {
					if err == nil && re.MatchString(st.Command) {
						found = true
					}
				}
				if !found {
					t.Errorf("%s: tutor note %q matches no solution step", l.ID, n.Match)
				}
			}
			if strings.TrimSpace(l.Solution) != "" && len(plan.Steps) == 0 {
				t.Errorf("%s: no steps", l.ID)
			}
			for _, s := range plan.Steps {
				first := strings.SplitN(s.Command, "\n", 2)[0]
				switch {
				case s.Title == "" || s.Why == "":
					t.Errorf("%s step %d: empty title/why: %q", l.ID, s.N, first)
				case s.Match != "" && !tutor.MatchLine(s.Match, first):
					t.Errorf("%s step %d: %q does not match its own command %q", l.ID, s.N, s.Match, first)
				case s.Where.Page != "" && !pages[s.Where.Page]:
					t.Errorf("%s step %d: unknown console page %q", l.ID, s.N, s.Where.Page)
				case s.Match == "" && !s.Manual:
					t.Errorf("%s step %d: not detectable and not manual: %q", l.ID, s.N, first)
				}
			}
		}
	}
}

func TestWalkthroughExample(t *testing.T) {
	root, _ := filepath.Abs("../../content/labs/ace-30/d04-web-vm")
	base, err := scenario.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	l, _, _ := base.Variant(1, "lab-proj")
	plan := tutor.Build(l.Solution, nil, "es")
	var b strings.Builder
	for _, s := range plan.Steps {
		b.WriteString(s.Phase + " | " + s.Title + " | " + s.Where.Path + " [" + s.Where.Action + " " + s.Where.Target + "]\n   " + s.Why + "\n")
	}
	if os.Getenv("SHOW") != "" {
		t.Log("\n" + b.String())
	}
	if len(plan.Steps) < 3 || plan.Steps[0].Where.Page == "" {
		t.Fatalf("unexpected plan:\n%s", b.String())
	}
}
