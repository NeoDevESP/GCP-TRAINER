// Package labtest is the content CI: it provisions each lab, checks that the
// broken baseline fails the grader, runs the official solution, and checks
// that the solved state passes. This is the "lab as software" pipeline.
package labtest

import (
	"fmt"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/grader"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

// Report summarises the verification of one lab variant.
type Report struct {
	LabID       string            `json:"labId"`
	Seed        int64             `json:"seed"`
	Params      map[string]string `json:"params"`
	BeforeScore int               `json:"beforeScore"`
	AfterScore  int               `json:"afterScore"`
	OK          bool              `json:"ok"`
	Problems    []string          `json:"problems"`
	SolutionLog string            `json:"-"`
	Failed      []string          `json:"failedChecks,omitempty"`
}

// SampleSubmission builds the reference answers stored in the lab.
func SampleSubmission(l *scenario.Lab) grader.Submission {
	sub := grader.Submission{Evidence: map[string]string{}, Answers: map[string][]int{}, Justifications: map[string]string{}}
	if l.Evidence != nil {
		for k, v := range l.Evidence.Sample {
			sub.Evidence[k] = v
		}
	}
	for _, q := range l.Quiz {
		sub.Answers[q.ID] = q.Answer
		var words []string
		for _, g := range q.Keywords {
			words = append(words, g[0])
		}
		sub.Justifications[q.ID] = "Because " + strings.Join(words, ", ") + "."
	}
	return sub
}

// Verify runs the full content pipeline for one lab and seed.
func Verify(l *scenario.Lab, seed int64) Report {
	r := Report{LabID: l.ID, Seed: seed}
	project := fmt.Sprintf("lab-%s-%d", strings.ReplaceAll(strings.ToLower(l.ID), "_", "-"), seed)
	if len(project) > 30 {
		project = project[:24] + fmt.Sprintf("-%05d", seed%100000)
	}
	project = strings.TrimSuffix(project, "-")
	if l.FixedProject != "" {
		project = l.FixedProject
	}
	if len(l.Rubric) == 0 {
		r.Problems = append(r.Problems, "rubric is empty")
	}
	if l.MaxPoints() != 100 {
		r.Problems = append(r.Problems, fmt.Sprintf("rubric totals %d points (expected 100)", l.MaxPoints()))
	}
	w, err := scenario.Provision(l, seed, project)
	if err != nil {
		r.Problems = append(r.Problems, "provision: "+err.Error())
		return r
	}
	r.Params = w.Params
	before := grader.Grade(w.Lab, w.State, w.Session, w.Project, grader.Submission{})
	r.BeforeScore = before.Score
	if before.Passed {
		r.Problems = append(r.Problems, fmt.Sprintf("unsolved environment already passes (score %d)", before.Score))
	}
	log, err := scenario.RunSolution(w)
	r.SolutionLog = log
	if err != nil {
		r.Problems = append(r.Problems, err.Error())
	}
	after := grader.Grade(w.Lab, w.State, w.Session, w.Project, SampleSubmission(w.Lab))
	r.AfterScore = after.Score
	for _, it := range after.Items {
		for _, c := range it.Checks {
			if !c.Pass {
				r.Failed = append(r.Failed, fmt.Sprintf("%s / %s: %s (%s)", it.Name, c.Type, c.Desc, c.Detail))
			}
		}
	}
	if after.Process != nil && len(after.Process.Violations) > 0 {
		r.Problems = append(r.Problems, "official solution violates lab constraints: "+strings.Join(after.Process.Violations, "; "))
	}
	if !after.Passed || after.Score < 90 {
		r.Problems = append(r.Problems, fmt.Sprintf("official solution scores %d (critical failed=%v)", after.Score, after.CriticalFailed))
	}
	r.OK = len(r.Problems) == 0
	return r
}
