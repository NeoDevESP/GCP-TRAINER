package labtest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neodevesp/gcp-trainer/internal/grader"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

func init() {
	root, _ := filepath.Abs("../..")
	scenario.BaselineDir = filepath.Join(root, "content/baselines")
	grader.PolicyDir = filepath.Join(root, "content/policies")
}

// TestAllLabs is the content regression suite (runs every lab with 2 variants).
func TestAllLabs(t *testing.T) {
	root, _ := filepath.Abs("../../content/labs")
	labs, err := scenario.LoadAll(root)
	if err != nil {
		t.Fatal(err)
	}
	only := os.Getenv("LAB")
	for _, l := range labs {
		if only != "" && l.ID != only {
			continue
		}
		l := l
		t.Run(l.ID, func(t *testing.T) {
			for _, seed := range []int64{1, 7} {
				r := Verify(l, seed)
				if !r.OK {
					t.Errorf("seed %d params %v: %v\nfailed: %v\nbefore=%d after=%d\n%s", seed, r.Params, r.Problems, r.Failed, r.BeforeScore, r.AfterScore, r.SolutionLog)
				}
			}
		})
	}
}

// TestGeneratedIncidents verifies the failure library: every failure alone at
// difficulty 2 and composed incidents (2-3 faults, misleading tickets, noise)
// at difficulty 4-5 must be unsolved at start and solved by the composed
// official solution.
func TestGeneratedIncidents(t *testing.T) {
	root, _ := filepath.Abs("../../content/failures")
	lib, err := scenario.LoadLibrary(root)
	if err != nil {
		t.Fatal(err)
	}
	if p := lib.Validate(); len(p) > 0 {
		t.Fatalf("library problems: %v", p)
	}
	only := os.Getenv("FAILURE")
	for _, sys := range lib.Systems {
		for _, f := range sys.Failures {
			if only != "" && f.ID != only {
				continue
			}
			spec := scenario.GenSpec{System: sys.ID, Failures: []string{f.ID}, Difficulty: 2, Seed: 3}
			t.Run(f.ID, func(t *testing.T) { verifySpec(t, lib, spec) })
		}
		if only != "" {
			continue
		}
		for _, seed := range []int64{1, 2, 3, 4} {
			for _, d := range []int{4, 5} {
				spec := scenario.GenSpec{System: sys.ID, Difficulty: d, Seed: seed, Mode: map[bool]string{true: "unknown"}[seed == 4]}
				t.Run(spec.ID(), func(t *testing.T) { verifySpec(t, lib, spec) })
			}
		}
		// production mode escalates incidents to P1 (war room + postmortem) at any difficulty
		spec := scenario.GenSpec{System: sys.ID, Difficulty: 2, Seed: 5, Mode: "production"}
		t.Run("production-"+spec.ID(), func(t *testing.T) {
			l, err := lib.Generate(spec)
			if err != nil {
				t.Fatal(err)
			}
			if l.Ticket.Kind == "INC" && l.Ticket.Priority != "P1" {
				t.Errorf("production mode ticket priority = %s, want P1", l.Ticket.Priority)
			}
			verifySpec(t, lib, spec)
		})
	}
}

func verifySpec(t *testing.T, lib *scenario.Library, spec scenario.GenSpec) {
	t.Helper()
	l, err := lib.Generate(spec)
	if err != nil {
		t.Fatal(err)
	}
	r := Verify(l, spec.Seed)
	if !r.OK {
		var ids []string
		fs, _ := lib.SelectFailures(spec)
		for _, f := range fs {
			ids = append(ids, f.ID)
		}
		t.Errorf("%v %v: %v\nfailed: %v\nbefore=%d after=%d\n%s", ids, spec, r.Problems, r.Failed, r.BeforeScore, r.AfterScore, r.SolutionLog)
	}
}
