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
	Branch    string          `yaml:"branch"` // skill branch (defaults to ID)
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

// StudyQuestion is a multiple-choice question; Answer indexes Options, or
// Answers lists the correct options of a "choose N" question.
type StudyQuestion struct {
	Q       Pair   `yaml:"q"`
	Options []Pair `yaml:"options"`
	Answer  int    `yaml:"answer"`
	Answers []int  `yaml:"answers"`
	Why     Pair   `yaml:"why"`
}

// Correct returns the indexes of the right options.
func (q StudyQuestion) Correct() []int {
	if len(q.Answers) > 0 {
		return q.Answers
	}
	return []int{q.Answer}
}

// StudyExam is the official structure of a certification exam.
type StudyExam struct {
	Cert       string        `yaml:"cert"`
	Minutes    int           `yaml:"minutes"`
	Questions  string        `yaml:"questions"`
	Fee        string        `yaml:"fee"`
	Validity   Pair          `yaml:"validity"`
	Experience Pair          `yaml:"experience"`
	Format     Pair          `yaml:"format"`
	Domains    []StudyDomain `yaml:"domains"`
}

// StudyDomain is one section of an exam guide.
type StudyDomain struct {
	Name       Pair     `yaml:"name"`
	Weight     float64  `yaml:"weight"`
	Topics     []string `yaml:"topics"`
	Objectives []Pair   `yaml:"objectives"`
}

// loadStudy reads learn/topics/*.yaml (several files may add to the same
// topic id) and learn/exams.yaml.
func loadStudy(dir string) ([]StudyTopic, []StudyExam, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "topics", "*.yaml"))
	sort.Strings(files)
	var out []StudyTopic
	at := map[string]int{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, nil, err
		}
		var t StudyTopic
		if err := yaml.Unmarshal(b, &t); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", f, err)
		}
		if t.Branch == "" {
			t.Branch = t.ID
		}
		if i, ok := at[t.ID]; ok {
			o := &out[i]
			o.Concepts = append(o.Concepts, t.Concepts...)
			o.Scenarios = append(o.Scenarios, t.Scenarios...)
			o.Quiz = append(o.Quiz, t.Quiz...)
			if o.Name[0] == "" {
				o.Name, o.Intro, o.Branch = t.Name, t.Intro, t.Branch
			}
			continue
		}
		at[t.ID] = len(out)
		out = append(out, t)
	}
	var ex struct {
		Exams []StudyExam `yaml:"exams"`
	}
	if b, err := os.ReadFile(filepath.Join(dir, "exams.yaml")); err == nil {
		if err := yaml.Unmarshal(b, &ex); err != nil {
			return nil, nil, fmt.Errorf("exams.yaml: %w", err)
		}
	}
	return out, ex.Exams, nil
}

func (c *Catalog) validateStudy() []string {
	var problems []string
	empty := func(p Pair) bool { return strings.TrimSpace(p[0]) == "" || strings.TrimSpace(p[1]) == "" }
	for _, t := range c.Study {
		if !c.hasBranch(t.Branch) {
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
			bad := empty(q.Q) || empty(q.Why) || len(q.Options) < 2
			seen := map[int]bool{}
			for _, a := range q.Correct() {
				bad = bad || a < 0 || a >= len(q.Options) || seen[a]
				seen[a] = true
			}
			for _, o := range q.Options {
				bad = bad || empty(o)
			}
			if bad {
				problems = append(problems, fmt.Sprintf("learn %s: question %d is invalid", t.ID, i+1))
			}
		}
	}
	topics := map[string]bool{}
	for _, t := range c.Study {
		topics[t.ID] = true
	}
	for _, e := range c.Exams {
		known := false
		for _, ce := range c.Certs {
			known = known || ce.ID == e.Cert
		}
		if !known {
			problems = append(problems, fmt.Sprintf("learn exam %s: unknown certification", e.Cert))
		}
		sum := 0.0
		for _, d := range e.Domains {
			sum += d.Weight
			if empty(d.Name) || len(d.Topics) == 0 {
				problems = append(problems, fmt.Sprintf("learn exam %s: incomplete domain", e.Cert))
			}
			for _, o := range d.Objectives {
				if empty(o) {
					problems = append(problems, fmt.Sprintf("learn exam %s: empty objective", e.Cert))
				}
			}
			for _, id := range d.Topics {
				if !topics[id] {
					problems = append(problems, fmt.Sprintf("learn exam %s: unknown topic %s", e.Cert, id))
				}
			}
		}
		if sum < 99 || sum > 101 {
			problems = append(problems, fmt.Sprintf("learn exam %s: domain weights add up to %.1f", e.Cert, sum))
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
			out = append(out, map[string]any{"q": q.Q.In(lang), "options": opts, "answers": q.Correct(), "why": q.Why.In(lang)})
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
		out = append(out, map[string]any{"id": t.ID, "branch": t.Branch, "name": t.Name.In(lang), "intro": t.Intro.In(lang), "concepts": cs, "scenarios": qs(t.Scenarios), "quiz": qs(t.Quiz)})
	}
	return out
}

// ExamView renders the exam structures in lang for the API.
func (c *Catalog) ExamView(lang string) []map[string]any {
	src := c
	if c.base != nil {
		src = c.base
	}
	out := []map[string]any{}
	for _, e := range src.Exams {
		ds := []map[string]any{}
		for _, d := range e.Domains {
			obj := []string{}
			for _, o := range d.Objectives {
				obj = append(obj, o.In(lang))
			}
			ds = append(ds, map[string]any{"name": d.Name.In(lang), "weight": d.Weight, "topics": d.Topics, "objectives": obj})
		}
		out = append(out, map[string]any{"cert": e.Cert, "minutes": e.Minutes, "questions": e.Questions, "fee": e.Fee, "validity": e.Validity.In(lang), "experience": e.Experience.In(lang), "format": e.Format.In(lang), "domains": ds})
	}
	return out
}
