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
