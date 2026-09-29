package company

import (
	"path/filepath"
	"testing"

	"github.com/neodevesp/gcp-trainer/internal/grader"
	"github.com/neodevesp/gcp-trainer/internal/labtest"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

func setup(t *testing.T) (*Definition, *Company) {
	t.Helper()
	root, _ := filepath.Abs("../../content")
	scenario.BaselineDir = filepath.Join(root, "baselines")
	grader.PolicyDir = filepath.Join(root, "policies")
	d, err := Load(filepath.Join(root, "company"), "nebula")
	if err != nil {
		t.Fatal(err)
	}
	if p := d.Validate(); len(p) > 0 {
		t.Fatalf("definition problems: %v", p)
	}
	c, err := New(d, "u1", 42, scenario.BaselineDir)
	if err != nil {
		t.Fatal(err)
	}
	return d, c
}

var stages = map[string]int{"intern": 0, "junior": 1, "engineer": 2, "senior": 3, "platform": 4, "architect": 5}

// TestMissionsSolvable runs every planned mission on a fresh company world.
func TestMissionsSolvable(t *testing.T) {
	d, c := setup(t)
	for _, id := range d.MissionOrder {
		m := d.Missions[id]
		if m.Company.Trigger != "" {
			continue
		}
		t.Run(id, func(t *testing.T) {
			l := MissionLab(d, m, withRisk(c.Params(), "", ""), c.State)
			r := labtest.Verify(l, 1)
			if !r.OK {
				t.Errorf("%v\nfailed: %v\nbefore=%d after=%d\n%s", r.Problems, r.Failed, r.BeforeScore, r.AfterScore, r.SolutionLog)
			}
		})
	}
}

func withRisk(p map[string]string, risk, proj string) map[string]string {
	p["risk"], p["risk_project"] = risk, proj
	return p
}

// play runs a mission with the given commands and completes it.
func play(t *testing.T, d *Definition, c *Company, id string, cmds ...string) {
	t.Helper()
	l, proj, err := c.Start(d, id, stages, 5)
	if err != nil {
		t.Fatal(err)
	}
	w, err := scenario.Provision(l, 1, proj)
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range cmds {
		if r := w.Session.Exec(cmd); r.Exit != 0 {
			t.Fatalf("%s: %s", cmd, r.Output)
		}
	}
	res := grader.Grade(w.Lab, w.State, w.Session, w.Project, labtest.SampleSubmission(w.Lab))
	b, _ := w.State.Marshal()
	if _, err := c.Complete(d, id, res, b); err != nil {
		t.Fatal(err)
	}
}

func available(c *Company, d *Definition, id string) bool {
	for _, m := range c.Missions(d, stages, 5, "es") {
		if m.ID == id && m.Available {
			return true
		}
	}
	return false
}

// TestConsequences: risky decisions today schedule tomorrow's incidents, and
// every incident is solvable on the world the learner left behind.
func TestConsequences(t *testing.T) {
	d, c := setup(t)
	play(t, d, c, "nb-m01-orientation", "gcloud config set project "+c.Projects["dev-web"])
	if available(c, d, "nb-i01-ssh-compromise") {
		t.Fatalf("no incident should exist before a risky decision: %+v", c.Pending)
	}
	// Bad decisions: SSH open to the world; exports bucket made public.
	play(t, d, c, "nb-m03-ops-ssh", "gcloud compute firewall-rules create ops-ssh-anywhere --network=prod-vpc --allow=tcp:22 --source-ranges=0.0.0.0/0")
	play(t, d, c, "nb-m04-marketing-assets", "gcloud storage buckets add-iam-policy-binding gs://exports-"+c.Projects["prod-web"]+" --member=allUsers --role=roles/storage.objectViewer")
	for _, id := range []string{"nb-i01-ssh-compromise", "nb-i04-data-exposure"} {
		if !available(c, d, id) {
			t.Fatalf("%s should be scheduled; pending=%+v", id, c.Pending)
		}
	}
	// advance days so the slower consequences materialise (no backups, no budget)
	play(t, d, c, "nb-m02-dev-access")
	play(t, d, c, "nb-m07-privilege-review")
	for _, id := range []string{"nb-i02-data-loss", "nb-i03-cost-spike"} {
		if !available(c, d, id) {
			t.Fatalf("%s should be scheduled by day %d; pending=%+v", id, c.Day, c.Pending)
		}
	}
	for _, id := range []string{"nb-i01-ssh-compromise", "nb-i02-data-loss", "nb-i03-cost-spike", "nb-i04-data-exposure"} {
		t.Run(id, func(t *testing.T) {
			l, _, err := c.Start(d, id, stages, 5)
			if err != nil {
				t.Fatal(err)
			}
			r := labtest.Verify(l, 1)
			if !r.OK {
				t.Errorf("%v\nfailed: %v\nbefore=%d after=%d\n%s", r.Problems, r.Failed, r.BeforeScore, r.AfterScore, r.SolutionLog)
			}
		})
	}
	if c.Metrics.Security >= 100 {
		t.Errorf("security metric should reflect open findings: %+v", c.Metrics)
	}
}

// TestMitigationCancelsConsequence: fixing a risk before it materialises
// cancels the scheduled incident.
func TestMitigationCancelsConsequence(t *testing.T) {
	d, c := setup(t)
	play(t, d, c, "nb-m01-orientation")
	scheduled := false
	for _, p := range c.Pending {
		if p.Consequence == "no-backups" {
			scheduled = true
		}
	}
	if !scheduled {
		t.Fatalf("no-backups should be scheduled: %+v", c.Pending)
	}
	play(t, d, c, "nb-m06-db-resilience", "gcloud sql instances patch orders-db --backup-start-time=02:00 --enable-point-in-time-recovery")
	for _, p := range c.Pending {
		if p.Consequence == "no-backups" {
			t.Fatalf("mitigated risk should be cancelled: %+v", c.Pending)
		}
	}
}
