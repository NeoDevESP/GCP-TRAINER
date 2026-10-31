package learning

// Runbooks are the quick diagnosis guide: one entry per symptom a GCP
// support engineer meets (the error text, what it means, the first commands
// to run, the usual causes, the right fix, what not to do) and the practice
// cases where it happens. Every text is a [es, en] pair.

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Runbook is one symptom of content/runbooks.yaml.
type Runbook struct {
	ID      string        `yaml:"id"`
	Area    string        `yaml:"area"`
	Symptom Pair          `yaml:"symptom"`
	Errors  []string      `yaml:"errors"` // literal error texts, as GCP prints them
	Means   Pair          `yaml:"means"`
	Steps   []RunbookStep `yaml:"steps"`
	Causes  []Pair        `yaml:"causes"`
	Fix     Pair          `yaml:"fix"`
	Avoid   Pair          `yaml:"avoid"`
	Labs    []string      `yaml:"labs"`
}

// RunbookStep is a check with the command (or console place) that runs it.
type RunbookStep struct {
	Text Pair   `yaml:"text"`
	Cmd  string `yaml:"cmd"`
}

func loadRunbooks(path string) ([]*Runbook, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Runbooks []*Runbook `yaml:"runbooks"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return doc.Runbooks, nil
}

func (c *Catalog) validateRunbooks() []string {
	var problems []string
	seen := map[string]bool{}
	for _, r := range c.Runbooks {
		if r.ID == "" || seen[r.ID] {
			problems = append(problems, fmt.Sprintf("runbook %q: missing or duplicate id", r.ID))
		}
		seen[r.ID] = true
		check := func(where string, p Pair) {
			if p[0] == "" || p[1] == "" {
				problems = append(problems, fmt.Sprintf("runbook %s: %s needs Spanish and English text", r.ID, where))
			}
		}
		check("symptom", r.Symptom)
		check("means", r.Means)
		check("fix", r.Fix)
		for i, s := range r.Steps {
			check(fmt.Sprintf("steps[%d]", i), s.Text)
		}
		for i, p := range r.Causes {
			check(fmt.Sprintf("causes[%d]", i), p)
		}
		if r.Avoid[0] != "" {
			check("avoid", r.Avoid)
		}
		for _, l := range r.Labs {
			if c.Labs[l] == nil {
				problems = append(problems, fmt.Sprintf("runbook %s: unknown lab %s", r.ID, l))
			}
		}
	}
	return problems
}

// RunbookView returns the guide in one language, with the practice cases' titles.
func (c *Catalog) RunbookView(lang string) []map[string]any {
	out := []map[string]any{}
	src := c
	if c.base != nil { // a language view: the guide lives in the base catalog
		src = c.base
	}
	for _, r := range src.Runbooks {
		steps := []map[string]string{}
		for _, s := range r.Steps {
			steps = append(steps, map[string]string{"text": s.Text.In(lang), "cmd": s.Cmd})
		}
		causes := []string{}
		for _, p := range r.Causes {
			causes = append(causes, p.In(lang))
		}
		labs := []map[string]string{}
		for _, id := range r.Labs {
			if l := c.Lab(id); l != nil {
				labs = append(labs, map[string]string{"id": id, "title": l.Title, "track": l.Track})
			}
		}
		errs := r.Errors
		if errs == nil {
			errs = []string{}
		}
		out = append(out, map[string]any{"id": r.ID, "area": r.Area, "symptom": r.Symptom.In(lang), "errors": errs, "means": r.Means.In(lang),
			"steps": steps, "causes": causes, "fix": r.Fix.In(lang), "avoid": r.Avoid.In(lang), "labs": labs})
	}
	return out
}
