// Package learning is the Learning Plane: users, attempts, XP, mastery,
// skill graph, badges, leagues, certification readiness, recommendations and
// learning KPIs. It knows nothing about how labs are provisioned (Lab Plane).
package learning

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/scenario"
	"gopkg.in/yaml.v3"
)

// Branch is one of the ten practice branches (plus foundations).
type Branch struct {
	ID     string  `yaml:"id" json:"id"`
	Name   string  `yaml:"name" json:"name"`
	Skills []Skill `yaml:"skills" json:"skills"`
}

// Skill is an observable competence.
type Skill struct {
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Observable  string `yaml:"observable" json:"observable"`
}

// Track is an ordered learning path aligned to a certification.
type Track struct {
	ID          string   `yaml:"id" json:"id"`
	Title       string   `yaml:"title" json:"title"`
	Description string   `yaml:"description" json:"description"`
	Level       string   `yaml:"level" json:"level"`
	Certs       []string `yaml:"certs" json:"certs"`
	Reference   string   `yaml:"reference" json:"reference"` // official Google learning path used for curriculum sync
	Phase       string   `yaml:"phase" json:"phase"`         // MVP, V1, V2
	Labs        []string `yaml:"labs" json:"labs"`
}

// BadgeDef defines a badge.
type BadgeDef struct {
	ID          string   `yaml:"id" json:"id"`
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Kind        string   `yaml:"kind" json:"kind"` // technical, mastery, special
	Branch      string   `yaml:"branch" json:"branch,omitempty"`
	Skill       string   `yaml:"skill" json:"skill,omitempty"`
	MinMastery  float64  `yaml:"minMastery" json:"minMastery,omitempty"`
	MinLabs     int      `yaml:"minLabs" json:"minLabs,omitempty"`
	Labs        []string `yaml:"labs" json:"labs,omitempty"`
}

// CertBlueprint weights branches for readiness.
type CertBlueprint struct {
	ID      string             `yaml:"id" json:"id"`
	Name    string             `yaml:"name" json:"name"`
	Level   string             `yaml:"level" json:"level"`
	Weights map[string]float64 `yaml:"weights" json:"weights"`
	Guide   string             `yaml:"guide" json:"guide"`
}

// Catalog is the loaded curriculum.
type Catalog struct {
	Branches []Branch                  `json:"branches"`
	Tracks   []Track                   `json:"tracks"`
	Badges   []BadgeDef                `json:"badges"`
	Certs    []CertBlueprint           `json:"certs"`
	Labs     map[string]*scenario.Lab  `json:"-"`
	LabOrder []string                  `json:"-"`
	skillBranch map[string]string
}

// LoadCatalog reads content/{skills,tracks,badges,certs}.yaml and all labs.
func LoadCatalog(root string) (*Catalog, error) {
	c := &Catalog{Labs: map[string]*scenario.Lab{}, skillBranch: map[string]string{}}
	var sk struct {
		Branches []Branch `yaml:"branches"`
	}
	if err := readYAML(filepath.Join(root, "skills.yaml"), &sk); err != nil {
		return nil, err
	}
	c.Branches = sk.Branches
	var tr struct {
		Tracks []Track `yaml:"tracks"`
	}
	if err := readYAML(filepath.Join(root, "tracks.yaml"), &tr); err != nil {
		return nil, err
	}
	c.Tracks = tr.Tracks
	var bd struct {
		Badges []BadgeDef `yaml:"badges"`
	}
	if err := readYAML(filepath.Join(root, "badges.yaml"), &bd); err != nil {
		return nil, err
	}
	c.Badges = bd.Badges
	var ce struct {
		Certs []CertBlueprint `yaml:"certs"`
	}
	if err := readYAML(filepath.Join(root, "certs.yaml"), &ce); err != nil {
		return nil, err
	}
	c.Certs = ce.Certs
	for _, b := range c.Branches {
		for _, s := range b.Skills {
			c.skillBranch[s.ID] = b.ID
		}
	}
	labs, err := scenario.LoadAll(filepath.Join(root, "labs"))
	if err != nil {
		return nil, err
	}
	for _, l := range labs {
		if c.Labs[l.ID] != nil {
			return nil, fmt.Errorf("duplicate lab id %s", l.ID)
		}
		c.Labs[l.ID] = l
		c.LabOrder = append(c.LabOrder, l.ID)
	}
	return c, nil
}

func readYAML(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, v)
}

// BranchOf returns the branch of a skill id (prefix before the dot as fallback).
func (c *Catalog) BranchOf(skill string) string {
	if b, ok := c.skillBranch[skill]; ok {
		return b
	}
	return strings.SplitN(skill, ".", 2)[0]
}

// Validate checks referential integrity of the curriculum (used in CI).
func (c *Catalog) Validate() []string {
	var problems []string
	for _, l := range c.Labs {
		for _, s := range l.Skills {
			if _, ok := c.skillBranch[s]; !ok {
				problems = append(problems, fmt.Sprintf("lab %s: unknown skill %s", l.ID, s))
			}
		}
		if l.Branch == "" || !c.hasBranch(l.Branch) {
			problems = append(problems, fmt.Sprintf("lab %s: unknown branch %q", l.ID, l.Branch))
		}
		switch l.Type {
		case "guided", "challenge", "incident", "quiz", "capstone", "boss", "case-study":
		default:
			problems = append(problems, fmt.Sprintf("lab %s: invalid type %q", l.ID, l.Type))
		}
		switch l.Level {
		case "basic", "intermediate", "advanced", "professional":
		default:
			problems = append(problems, fmt.Sprintf("lab %s: invalid level %q", l.ID, l.Level))
		}
		if l.RetestOf != "" && c.Labs[l.RetestOf] == nil {
			problems = append(problems, fmt.Sprintf("lab %s: retestOf unknown lab %s", l.ID, l.RetestOf))
		}
	}
	for _, t := range c.Tracks {
		for _, id := range t.Labs {
			if c.Labs[id] == nil {
				problems = append(problems, fmt.Sprintf("track %s: unknown lab %s", t.ID, id))
			}
		}
	}
	for _, b := range c.Badges {
		for _, id := range b.Labs {
			if c.Labs[id] == nil {
				problems = append(problems, fmt.Sprintf("badge %s: unknown lab %s", b.ID, id))
			}
		}
	}
	sort.Strings(problems)
	return problems
}

func (c *Catalog) hasBranch(id string) bool {
	for _, b := range c.Branches {
		if b.ID == id {
			return true
		}
	}
	return false
}

// Track returns a track by id.
func (c *Catalog) Track(id string) *Track {
	for i := range c.Tracks {
		if c.Tracks[i].ID == id {
			return &c.Tracks[i]
		}
	}
	return nil
}
