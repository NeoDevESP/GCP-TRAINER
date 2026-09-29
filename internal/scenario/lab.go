// Package scenario implements content-as-code: lab definitions (lab.yaml),
// baselines, declarative fault injection, the variant generator (Scenario
// Engine) and provisioning of a lab into a simulated world.
package scenario

import (
	"bytes"
	"fmt"
	"github.com/neodevesp/gcp-trainer/internal/desk"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/neodevesp/gcp-trainer/internal/cli"
	"github.com/neodevesp/gcp-trainer/internal/sim"
	"gopkg.in/yaml.v3"
)

// Lab is the content schema for one exercise.
type Lab struct {
	ID              string              `yaml:"id" json:"id"`
	Title           string              `yaml:"title" json:"title"`
	Summary         string              `yaml:"summary" json:"summary"`
	Version         int                 `yaml:"version" json:"version"`
	Track           string              `yaml:"track" json:"track"`
	Day             int                 `yaml:"day" json:"day,omitempty"`
	Level           string              `yaml:"level" json:"level"`
	Branch          string              `yaml:"branch" json:"branch"`
	Mode            string              `yaml:"mode" json:"mode,omitempty"` // learn, lab, production, architecture, career, unknown (Blueprint §1, §12)
	Type            string              `yaml:"type" json:"type"`
	Skills          []string            `yaml:"skills" json:"skills"`
	WellArchitected []string            `yaml:"wellArchitected" json:"wellArchitected"`
	Certs           []string            `yaml:"certs" json:"certs"`
	Difficulty      int                 `yaml:"difficulty" json:"difficulty"`
	Minutes         int                 `yaml:"minutes" json:"minutes"`
	Fidelity        Fidelity            `yaml:"fidelity" json:"fidelity"`
	Region          string              `yaml:"region" json:"region"`
	Zone            string              `yaml:"zone" json:"zone"`
	Params          map[string][]string `yaml:"params" json:"params,omitempty"`
	Story           string              `yaml:"story" json:"story"`
	Instructions    string              `yaml:"instructions" json:"instructions"`
	Objectives      []string            `yaml:"objectives" json:"objectives"`
	Baseline        []string            `yaml:"baseline" json:"baseline"`
	Setup           string              `yaml:"setup" json:"-"`
	Files           map[string]string   `yaml:"files" json:"files,omitempty"`
	Faults          []Fault             `yaml:"faults" json:"-"`
	Traffic         []sim.TrafficSpec   `yaml:"traffic" json:"-"`
	Student         Student             `yaml:"student" json:"student"`
	Policy          cli.Policy          `yaml:"policy" json:"policy"`
	Hints           []Hint              `yaml:"hints" json:"hints"`
	Solution        string              `yaml:"solution" json:"-"`
	Evidence        *Evidence           `yaml:"evidence" json:"evidence,omitempty"`
	Quiz            []Question          `yaml:"quiz" json:"quiz,omitempty"`
	Rubric          []RubricItem        `yaml:"rubric" json:"rubric"`
	PassScore       int                 `yaml:"passScore" json:"passScore"`
	Prereqs         []string            `yaml:"prereqs" json:"prereqs,omitempty"`
	RetestOf        string              `yaml:"retestOf" json:"retestOf,omitempty"`
	Timeline        []TimelineEvent     `yaml:"timeline" json:"timeline,omitempty"`
	Constraints     []string            `yaml:"constraints" json:"constraints,omitempty"`
	Ticket          *desk.Ticket        `yaml:"ticket" json:"ticket,omitempty"`       // service-desk framing (INC/REQ/CHG/PRB/SEC/COST/MIG)
	Actors          []desk.Actor        `yaml:"actors" json:"actors,omitempty"`       // simulated people the learner can question
	Generated       *GenSpec            `yaml:"generated" json:"generated,omitempty"` // set when built by the incident generator
	Company         *CompanySpec        `yaml:"company" json:"company,omitempty"`     // company-simulation mission metadata
	// InitialState starts the lab from a persisted world (company simulation)
	// instead of a fresh baseline; FixedProject is the primary project id.
	InitialState []byte   `yaml:"-" json:"-"`
	FixedProject string   `yaml:"-" json:"-"`
	Noise        []string `yaml:"noise" json:"-"`
	Dir          string   `yaml:"-" json:"-"`
}

// Fidelity declares which layers a lab supports.
type Fidelity struct {
	Default   string   `yaml:"default" json:"default"`
	Supported []string `yaml:"supported" json:"supported"`
	F2        *F2Spec  `yaml:"f2" json:"f2,omitempty"`
}

// F2Spec configures real-GCP execution.
type F2Spec struct {
	APIs          []string `yaml:"apis" json:"apis"`
	BudgetEur     float64  `yaml:"budgetEur" json:"budgetEur"`
	TTLMinutes    int      `yaml:"ttlMinutes" json:"ttlMinutes"`
	StudentRoles  []string `yaml:"studentRoles" json:"studentRoles"`
	TerraformBase string   `yaml:"terraformBaseline" json:"terraformBaseline"`
}

// Student configures the lab identity.
type Student struct {
	Account      string   `yaml:"account" json:"account"`
	Roles        []string `yaml:"roles" json:"roles"`
	Unconfigured bool     `yaml:"unconfigured" json:"unconfigured"` // start with no gcloud project set
}

// Hint is a progressive hint with an XP cost.
type Hint struct {
	Text string `yaml:"text" json:"text"`
	Cost int    `yaml:"cost" json:"cost"`
	Kind string `yaml:"kind" json:"kind"` // hint, partial, solution
}

// Evidence configures the explanation / postmortem requirement.
type Evidence struct {
	Prompt   string            `yaml:"prompt" json:"prompt"`
	Fields   []string          `yaml:"fields" json:"fields"` // e.g. rootCause, fix, prevention
	Keywords [][]string        `yaml:"keywords" json:"-"`
	MinWords int               `yaml:"minWords" json:"minWords"`
	Sample   map[string]string `yaml:"sample" json:"-"` // reference answer used by CI
}

// Question is a quiz / architecture decision item.
type Question struct {
	ID          string     `yaml:"id" json:"id"`
	Question    string     `yaml:"question" json:"question"`
	Options     []string   `yaml:"options" json:"options"`
	Answer      []int      `yaml:"answer" json:"-"`
	Explanation string     `yaml:"explanation" json:"-"`
	Points      int        `yaml:"points" json:"points"`
	Justify     bool       `yaml:"justify" json:"justify"`
	Keywords    [][]string `yaml:"keywords" json:"-"`
	// Interview mode: a follow-up is asked only after `after` was answered,
	// and only when that answer was correct/incorrect (`when`).
	After string `yaml:"after" json:"after,omitempty"`
	When  string `yaml:"when" json:"when,omitempty"` // correct, incorrect, "" = always
	Probe string `yaml:"probe" json:"-"`             // interviewer's reaction before asking
}

// RubricItem is one scored criterion.
type RubricItem struct {
	Name      string  `yaml:"name" json:"name"`
	Points    int     `yaml:"points" json:"points"`
	Validator string  `yaml:"validator" json:"validator"` // state, functional, security, cost, evidence, diagnosis, quiz
	Critical  bool    `yaml:"critical" json:"critical"`
	All       bool    `yaml:"all" json:"all"`
	Checks    []Check `yaml:"checks" json:"-"`
}

// Check is one grader assertion (see grader package for semantics).
type Check map[string]any

// TimelineEvent is part of the pager narrative of incident labs.
type TimelineEvent struct {
	At   string `yaml:"at" json:"at"`
	From string `yaml:"from" json:"from"`
	Text string `yaml:"text" json:"text"`
}

// CompanySpec places a mission in the persistent company simulation.
type CompanySpec struct {
	Stage      string   `yaml:"stage" json:"stage"`                     // career stage that unlocks it
	After      []string `yaml:"after" json:"after,omitempty"`           // missions that must be completed first
	Trigger    string   `yaml:"trigger" json:"trigger,omitempty"`       // consequence id that schedules it
	Project    string   `yaml:"project" json:"project"`                 // primary company project key (e.g. prod-web)
	Repeatable bool     `yaml:"repeatable" json:"repeatable,omitempty"` //
	Kind       string   `yaml:"kind" json:"kind,omitempty"`             // REQ, CHG, INC, SEC, COST, MIG, PRB
}

// Fault is a declarative mutation applied after the baseline.
type Fault map[string]any

// Load reads a lab from a directory containing lab.yaml (+ optional instructions.md).
func Load(dir string) (*Lab, error) {
	b, err := os.ReadFile(filepath.Join(dir, "lab.yaml"))
	if err != nil {
		return nil, err
	}
	var l Lab
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&l); err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	if ins, err := os.ReadFile(filepath.Join(dir, "instructions.md")); err == nil {
		l.Instructions = string(ins)
	}
	if sol, err := os.ReadFile(filepath.Join(dir, "solution.sh")); err == nil && l.Solution == "" {
		l.Solution = string(sol)
	}
	if l.Files == nil {
		l.Files = map[string]string{}
	}
	if fd := filepath.Join(dir, "files"); dirExists(fd) {
		_ = filepath.Walk(fd, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				rel, _ := filepath.Rel(fd, p)
				c, _ := os.ReadFile(p)
				l.Files[filepath.ToSlash(rel)] = string(c)
			}
			return nil
		})
	}
	l.Dir = dir
	l.defaults()
	return &l, nil
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func (l *Lab) defaults() {
	if l.Region == "" {
		l.Region = "europe-west1"
	}
	if l.Zone == "" {
		l.Zone = l.Region + "-b"
	}
	if l.Fidelity.Default == "" {
		l.Fidelity.Default = "F0"
	}
	if len(l.Fidelity.Supported) == 0 {
		l.Fidelity.Supported = []string{l.Fidelity.Default}
	}
	if l.Student.Account == "" {
		l.Student.Account = "student@gcplab.dev"
	}
	if len(l.Student.Roles) == 0 {
		l.Student.Roles = []string{"roles/owner"}
	}
	if l.PassScore == 0 {
		l.PassScore = 70
	}
	if l.Difficulty == 0 {
		l.Difficulty = 2
	}
	if l.Minutes == 0 {
		l.Minutes = 45
	}
	if l.Version == 0 {
		l.Version = 1
	}
	for i := range l.Hints {
		if l.Hints[i].Cost == 0 {
			l.Hints[i].Cost = []int{3, 5, 10, 10}[min(i, 3)]
		}
	}
}

// MaxPoints returns the total rubric points.
func (l *Lab) MaxPoints() int {
	t := 0
	for _, r := range l.Rubric {
		t += r.Points
	}
	return t
}

// LoadAll loads every lab under a content root (content/labs/**/lab.yaml).
func LoadAll(root string) ([]*Lab, error) {
	var labs []*Lab
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == "lab.yaml" {
			l, err := Load(filepath.Dir(p))
			if err != nil {
				return err
			}
			labs = append(labs, l)
		}
		return nil
	})
	sort.Slice(labs, func(i, j int) bool {
		if labs[i].Track != labs[j].Track {
			return labs[i].Track < labs[j].Track
		}
		if labs[i].Day != labs[j].Day {
			return labs[i].Day < labs[j].Day
		}
		return labs[i].ID < labs[j].ID
	})
	return labs, err
}

// Variant chooses parameter values deterministically from a seed and renders
// the lab's templated fields. This is the Scenario Engine: one template can
// yield many incidents with different names, regions, ports and noise.
func (l *Lab) Variant(seed int64, projectID string) (*Lab, map[string]string, error) {
	params := map[string]string{"project": projectID, "region": l.Region, "zone": l.Zone}
	keys := make([]string, 0, len(l.Params))
	for k := range l.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		vals := l.Params[k]
		if len(vals) == 0 {
			continue
		}
		params[k] = vals[int(mix(uint64(seed), k)%uint64(len(vals)))]
	}
	if _, hasZone := l.Params["zone"]; !hasZone {
		if r, ok := params["region"]; ok && r != l.Region {
			params["zone"] = r + "-b"
		}
	}
	render := func(s string) (string, error) {
		if !strings.Contains(s, "{{") {
			return s, nil
		}
		t, err := template.New("x").Option("missingkey=error").Parse(s)
		if err != nil {
			return "", err
		}
		var b bytes.Buffer
		if err := t.Execute(&b, params); err != nil {
			return "", err
		}
		return b.String(), nil
	}
	// Render by round-tripping through YAML so every string field is templated.
	raw, err := yaml.Marshal(l)
	if err != nil {
		return nil, nil, err
	}
	out, err := render(string(raw))
	if err != nil {
		return nil, nil, fmt.Errorf("lab %s: template: %w", l.ID, err)
	}
	var v Lab
	if err := yaml.Unmarshal([]byte(out), &v); err != nil {
		return nil, nil, err
	}
	v.Dir = l.Dir
	v.InitialState, v.FixedProject = l.InitialState, l.FixedProject
	v.Instructions, _ = render(l.Instructions)
	v.Solution, _ = render(l.Solution)
	v.Setup, _ = render(l.Setup)
	v.Files = map[string]string{}
	for k, c := range l.Files {
		v.Files[k], _ = render(c)
	}
	v.Region, v.Zone = params["region"], params["zone"]
	v.defaults()
	return &v, params, nil
}

// mix is splitmix64 over the seed and parameter name (well-spread variants).
func mix(seed uint64, key string) uint64 {
	h := seed
	for _, c := range key {
		h = h*31 + uint64(c)
	}
	h += 0x9e3779b97f4a7c15
	h = (h ^ (h >> 30)) * 0xbf58476d1ce4e5b9
	h = (h ^ (h >> 27)) * 0x94d049bb133111eb
	return h ^ (h >> 31)
}
