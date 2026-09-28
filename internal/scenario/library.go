package scenario

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/desk"
	"github.com/neodevesp/gcp-trainer/internal/sim"
	"gopkg.in/yaml.v3"
)

// Failure Library (Blueprint §11): every failure describes symptoms, possible
// causes, evidence, dependencies, dangerous actions, valid fixes and variants.
// One symptom maps to many causes across layers, so incidents teach
// troubleshooting by hypothesis instead of memorised recipes.

// Symptom is an observable effect (HTTP 502, permission denied, timeout...).
type Symptom struct {
	ID          string   `yaml:"id" json:"id"`
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Layers      []string `yaml:"layers" json:"layers"` // ordered causal chain to investigate
	// Vague and misleading ticket summaries used at higher difficulty.
	Vague      []string          `yaml:"vague" json:"vague"`
	Misleading map[string]string `yaml:"misleading" json:"-"` // layer -> "looks like <other layer>" summary
}

// Context is the business framing of a generated incident.
type Context struct {
	ID          string   `yaml:"id" json:"id"`
	Company     string   `yaml:"company" json:"company"`
	Service     string   `yaml:"service" json:"service"`
	Impact      string   `yaml:"impact" json:"impact"`
	Reporter    string   `yaml:"reporter" json:"reporter"`
	Priority    string   `yaml:"priority" json:"priority"`
	Constraints []string `yaml:"constraints" json:"constraints"`
}

// EvidenceSpec says how a failure is diagnosed and explained.
type EvidenceSpec struct {
	Commands  string     `yaml:"commands" json:"commands"`   // regex of diagnostic commands
	RootCause [][]string `yaml:"rootCause" json:"rootCause"` // keyword groups for the root-cause explanation
	Sample    string     `yaml:"sample" json:"sample"`
}

// FailureMode is one entry of the library.
type FailureMode struct {
	ID        string                 `yaml:"id" json:"id"`
	Title     string                 `yaml:"title" json:"title"`
	Symptom   string                 `yaml:"symptom" json:"symptom"`
	Layer     string                 `yaml:"layer" json:"layer"`
	Kind      string                 `yaml:"kind" json:"kind"` // INC (default), SEC, COST
	Skills    []string               `yaml:"skills" json:"skills"`
	Summary   string                 `yaml:"summary" json:"summary"` // accurate ticket summary
	Hints     []Hint                 `yaml:"hints" json:"-"`
	Faults    []Fault                `yaml:"faults" json:"-"`
	Facts     map[string][]desk.Fact `yaml:"facts" json:"-"` // role -> facts actors reveal
	Evidence  EvidenceSpec           `yaml:"evidence" json:"-"`
	Dangerous []string               `yaml:"dangerous" json:"dangerous"`
	Fixes     []string               `yaml:"fixes" json:"fixes"`
	Solution  string                 `yaml:"solution" json:"-"`
	Checks    []Check                `yaml:"checks" json:"-"` // extra safety / restoration checks
	Settle    int                    `yaml:"settle" json:"-"` // simulated minutes to wait after the fix
	Params    map[string][]string    `yaml:"params" json:"-"`
	Conflicts []string               `yaml:"conflicts" json:"conflicts,omitempty"` // failures that cannot be combined
	System    string                 `yaml:"-" json:"system"`
}

// System is a healthy baseline architecture that failures are injected into.
type System struct {
	ID       string            `yaml:"id" json:"id"`
	Title    string            `yaml:"title" json:"title"`
	Branch   string            `yaml:"branch" json:"branch"`
	Skills   []string          `yaml:"skills" json:"skills"`
	Baseline []string          `yaml:"baseline" json:"baseline"`
	Setup    string            `yaml:"setup" json:"-"`
	Traffic  []sim.TrafficSpec `yaml:"traffic" json:"-"`
	Healthy  []Check           `yaml:"healthy" json:"-"` // restoration checks (all must pass)
	Safety   []Check           `yaml:"safety" json:"-"`  // guardrails (critical)
	Actors   []desk.Actor      `yaml:"actors" json:"-"`
	Noise    []Fault           `yaml:"noise" json:"-"`
	Topology string            `yaml:"topology" json:"topology"`
	Decoys   []string          `yaml:"decoys" json:"decoys,omitempty"` // resources owned by other teams (protect:)
	Student  Student           `yaml:"student" json:"-"`
	Failures []*FailureMode    `yaml:"failures" json:"failures"`
}

// Library is the loaded failure library.
type Library struct {
	Symptoms map[string]*Symptom     `json:"symptoms"`
	Contexts map[string]*Context     `json:"contexts"`
	Systems  map[string]*System      `json:"systems"`
	Failures map[string]*FailureMode `json:"-"`
}

// LoadLibrary reads content/failures: symptoms.yaml, contexts.yaml and
// systems/*.yaml.
func LoadLibrary(dir string) (*Library, error) {
	lib := &Library{Symptoms: map[string]*Symptom{}, Contexts: map[string]*Context{}, Systems: map[string]*System{}, Failures: map[string]*FailureMode{}}
	var sy struct {
		Symptoms []*Symptom `yaml:"symptoms"`
	}
	if err := readStrict(filepath.Join(dir, "symptoms.yaml"), &sy); err != nil {
		return nil, err
	}
	for _, s := range sy.Symptoms {
		lib.Symptoms[s.ID] = s
	}
	var cx struct {
		Contexts []*Context `yaml:"contexts"`
	}
	if err := readStrict(filepath.Join(dir, "contexts.yaml"), &cx); err != nil {
		return nil, err
	}
	for _, c := range cx.Contexts {
		lib.Contexts[c.ID] = c
	}
	files, _ := filepath.Glob(filepath.Join(dir, "systems", "*.yaml"))
	sort.Strings(files)
	for _, f := range files {
		var s System
		if err := readStrict(f, &s); err != nil {
			return nil, err
		}
		if lib.Systems[s.ID] != nil {
			return nil, fmt.Errorf("%s: duplicate system %s", f, s.ID)
		}
		lib.Systems[s.ID] = &s
		for _, fm := range s.Failures {
			if lib.Failures[fm.ID] != nil {
				return nil, fmt.Errorf("%s: duplicate failure %s", f, fm.ID)
			}
			fm.System = s.ID
			lib.Failures[fm.ID] = fm
		}
	}
	return lib, nil
}

func readStrict(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// Validate checks referential integrity.
func (lib *Library) Validate() []string {
	var p []string
	for _, s := range lib.Systems {
		if len(s.Healthy) == 0 {
			p = append(p, "system "+s.ID+": no healthy checks")
		}
		for _, f := range s.Failures {
			if lib.Symptoms[f.Symptom] == nil {
				p = append(p, fmt.Sprintf("failure %s: unknown symptom %s", f.ID, f.Symptom))
			}
			if len(f.Faults) == 0 {
				p = append(p, "failure "+f.ID+": no faults")
			}
			if strings.TrimSpace(f.Solution) == "" {
				p = append(p, "failure "+f.ID+": no solution")
			}
			if len(f.Evidence.RootCause) == 0 || f.Evidence.Sample == "" {
				p = append(p, "failure "+f.ID+": evidence keywords and sample required")
			}
			for _, c := range f.Conflicts {
				if lib.Failures[c] == nil {
					p = append(p, fmt.Sprintf("failure %s: unknown conflict %s", f.ID, c))
				}
			}
		}
	}
	sort.Strings(p)
	return p
}

// SymptomEdge links a symptom to a possible cause.
type SymptomEdge struct {
	Symptom string `json:"symptom"`
	Layer   string `json:"layer"`
	Failure string `json:"failure"`
	Title   string `json:"title"`
	System  string `json:"system"`
}

// SymptomGraph returns symptom → layer → failure edges (e.g. HTTP 502 →
// load balancer / backend unhealthy / firewall / timeout / application /
// readiness / DNS).
func (lib *Library) SymptomGraph() []SymptomEdge {
	var out []SymptomEdge
	for _, f := range lib.Failures {
		out = append(out, SymptomEdge{Symptom: f.Symptom, Layer: f.Layer, Failure: f.ID, Title: f.Title, System: f.System})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Symptom != b.Symptom {
			return a.Symptom < b.Symptom
		}
		if a.Layer != b.Layer {
			return a.Layer < b.Layer
		}
		return a.Failure < b.Failure
	})
	return out
}
