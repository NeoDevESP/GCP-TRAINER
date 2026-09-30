package learning

// The "Learn" section: per-topic study material (concepts, "which service?"
// scenarios and quiz questions) read from content/learn/*.yaml. Every text is
// a [Spanish, English] pair so both languages stay side by side.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Pair is a text in Spanish and English.
type Pair [2]string

// UnmarshalYAML reads a two-item sequence.
func (p *Pair) UnmarshalYAML(n *yaml.Node) error {
	var s []string
	if err := n.Decode(&s); err != nil {
		return err
	}
	if len(s) != 2 {
		return fmt.Errorf("line %d: want [es, en], got %d items", n.Line, len(s))
	}
	p[0], p[1] = s[0], s[1]
	return nil
}

// In returns the text in lang.
func (p Pair) In(lang string) string {
	if lang == "en" && p[1] != "" {
		return p[1]
	}
	return p[0]
}

// StudyTopic is the study material of one branch.
type StudyTopic struct {
	ID        string          `yaml:"id"`
	Name      Pair            `yaml:"name"`
	Intro     Pair            `yaml:"intro"`
	Concepts  []StudyConcept  `yaml:"concepts"`
	Scenarios []StudyQuestion `yaml:"scenarios"`
	Quiz      []StudyQuestion `yaml:"quiz"`
}

// StudyConcept is one thing to know before the quiz.
type StudyConcept struct {
	ID     string `yaml:"id"`
	Term   Pair   `yaml:"term"`
	What   Pair   `yaml:"what"`
	Points []Pair `yaml:"points"`
	Tip    Pair   `yaml:"tip"`
}

// StudyQuestion is a multiple-choice question; Answer indexes Options.
type StudyQuestion struct {
	Q       Pair   `yaml:"q"`
	Options []Pair `yaml:"options"`
	Answer  int    `yaml:"answer"`
	Why     Pair   `yaml:"why"`
}

func loadStudy(dir string) ([]StudyTopic, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	sort.Strings(files)
	var out []StudyTopic
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var t StudyTopic
		if err := yaml.Unmarshal(b, &t); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		out = append(out, t)
	}
	return out, nil
}

func (c *Catalog) validateStudy() []string {
	var problems []string
	empty := func(p Pair) bool { return strings.TrimSpace(p[0]) == "" || strings.TrimSpace(p[1]) == "" }
	for _, t := range c.Study {
		if !c.hasBranch(t.ID) {
			problems = append(problems, fmt.Sprintf("learn %s: unknown branch", t.ID))
		}
		if empty(t.Name) || empty(t.Intro) || len(t.Concepts) == 0 || len(t.Quiz) == 0 {
			problems = append(problems, fmt.Sprintf("learn %s: incomplete topic", t.ID))
		}
		for _, k := range t.Concepts {
			bad := empty(k.Term) || empty(k.What) || empty(k.Tip)
			for _, p := range k.Points {
				bad = bad || empty(p)
			}
			if bad {
				problems = append(problems, fmt.Sprintf("learn %s/%s: missing text", t.ID, k.ID))
			}
		}
		for i, q := range append(append([]StudyQuestion{}, t.Scenarios...), t.Quiz...) {
			bad := empty(q.Q) || empty(q.Why) || len(q.Options) < 2 || q.Answer < 0 || q.Answer >= len(q.Options)
			for _, o := range q.Options {
				bad = bad || empty(o)
			}
			if bad {
				problems = append(problems, fmt.Sprintf("learn %s: question %d is invalid", t.ID, i+1))
			}
		}
	}
	return problems
}

// StudyView renders the study material in lang for the API.
func (c *Catalog) StudyView(lang string) []map[string]any {
	src := c
	if c.base != nil {
		src = c.base
	}
	qs := func(list []StudyQuestion) []map[string]any {
		out := []map[string]any{}
		for _, q := range list {
			opts := []string{}
			for _, o := range q.Options {
				opts = append(opts, o.In(lang))
			}
			out = append(out, map[string]any{"q": q.Q.In(lang), "options": opts, "answer": q.Answer, "why": q.Why.In(lang)})
		}
		return out
	}
	out := []map[string]any{}
	for _, t := range src.Study {
		cs := []map[string]any{}
		for _, k := range t.Concepts {
			pts := []string{}
			for _, p := range k.Points {
				pts = append(pts, p.In(lang))
			}
			cs = append(cs, map[string]any{"id": k.ID, "term": k.Term.In(lang), "what": k.What.In(lang), "points": pts, "tip": k.Tip.In(lang)})
		}
		out = append(out, map[string]any{"id": t.ID, "name": t.Name.In(lang), "intro": t.Intro.In(lang), "concepts": cs, "scenarios": qs(t.Scenarios), "quiz": qs(t.Quiz)})
	}
	return out
}
