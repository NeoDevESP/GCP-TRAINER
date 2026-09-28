package learning

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// Career progression (Blueprint §14-15, §22): stages represent responsibility
// and autonomy, not years of experience. Specialisations open once the shared
// Cloud Engineer core is demonstrated. Each stage ends with a capstone.

// CareerStage is defined in content/career.yaml.
type CareerStage struct {
	ID          string             `yaml:"id" json:"id"`
	Name        string             `yaml:"name" json:"name"`
	Scenario    string             `yaml:"scenario" json:"scenario"` // kind of work at this stage
	MinAutonomy int                `yaml:"minAutonomy" json:"minAutonomy"`
	Branches    map[string]float64 `yaml:"branches" json:"branches"` // minimum branch mastery
	Capstones   []string           `yaml:"capstones" json:"capstones"`
}

// Specialization is a tree opened after the core.
type Specialization struct {
	ID        string             `yaml:"id" json:"id"`
	Name      string             `yaml:"name" json:"name"`
	Requires  string             `yaml:"requires" json:"requires"` // stage id that unlocks it
	Branches  map[string]float64 `yaml:"branches" json:"branches"`
	Skills    []string           `yaml:"skills" json:"skills"`
	Capstones []string           `yaml:"capstones" json:"capstones"`
	Certs     []string           `yaml:"certs" json:"certs"`
}

// CareerDef is the career content.
type CareerDef struct {
	Stages          []CareerStage    `yaml:"stages" json:"stages"`
	Specializations []Specialization `yaml:"specializations" json:"specializations"`
}

// SpecProgress is progress on one specialisation.
type SpecProgress struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Unlocked bool     `json:"unlocked"`
	Percent  float64  `json:"percent"`
	Done     bool     `json:"done"`
	Missing  []string `json:"missing,omitempty"`
}

// CareerProfile is the learner's professional position.
type CareerProfile struct {
	Stage           string         `json:"stage"`
	StageIndex      int            `json:"stageIndex"`
	Next            string         `json:"next,omitempty"`
	NextNeeds       []string       `json:"nextNeeds,omitempty"`
	Specializations []SpecProgress `json:"specializations"`
}

func passedLabs(attempts []Attempt) map[string]bool {
	out := map[string]bool{}
	for _, a := range submitted(attempts) {
		if a.Passed {
			out[a.LabID] = true
		}
	}
	return out
}

// stageNeeds lists what is missing to hold a stage.
func (e *Engine) stageNeeds(st CareerStage, branches map[string]float64, sm StudentModel, passed map[string]bool) []string {
	var need []string
	if sm.Autonomy.Index < st.MinAutonomy {
		need = append(need, fmt.Sprintf("autonomy %s (now %s)", AutonomyLadder[st.MinAutonomy], sm.Autonomy.Stage))
	}
	keys := make([]string, 0, len(st.Branches))
	for b := range st.Branches {
		keys = append(keys, b)
	}
	sort.Strings(keys)
	for _, b := range keys {
		if branches[b] < st.Branches[b] {
			need = append(need, fmt.Sprintf("%s mastery %.0f/%.0f", e.branchName(b), branches[b], st.Branches[b]))
		}
	}
	for _, c := range st.Capstones {
		if !passed[c] {
			title := c
			if l := e.Cat.Lab(c); l != nil {
				title = l.Title
			}
			need = append(need, "capstone: "+title)
		}
	}
	return need
}

// Career computes the career stage and specialisation progress.
func (e *Engine) Career(skills []SkillScore, branches map[string]float64, sm StudentModel, attempts []Attempt) CareerProfile {
	cd := e.Cat.Career
	cp := CareerProfile{}
	if len(cd.Stages) == 0 {
		return cp
	}
	passed := passedLabs(attempts)
	idx := 0
	for i, st := range cd.Stages {
		if i == 0 || len(e.stageNeeds(st, branches, sm, passed)) == 0 {
			idx = i
			continue
		}
		break
	}
	cp.Stage, cp.StageIndex = cd.Stages[idx].Name, idx
	if idx+1 < len(cd.Stages) {
		cp.Next = cd.Stages[idx+1].Name
		cp.NextNeeds = e.stageNeeds(cd.Stages[idx+1], branches, sm, passed)
	}
	stageIdx := map[string]int{}
	for i, st := range cd.Stages {
		stageIdx[st.ID] = i
	}
	mastery := map[string]float64{}
	for _, s := range skills {
		mastery[s.Skill] = s.Mastery
	}
	for _, sp := range cd.Specializations {
		p := SpecProgress{ID: sp.ID, Name: sp.Name, Unlocked: idx >= stageIdx[sp.Requires]}
		total, got := 0.0, 0.0
		for b, min := range sp.Branches {
			total++
			got += clamp01(branches[b] / min)
			if branches[b] < min {
				p.Missing = append(p.Missing, fmt.Sprintf("%s %.0f/%.0f", e.branchName(b), branches[b], min))
			}
		}
		for _, s := range sp.Skills {
			total++
			got += clamp01(mastery[s] / 70)
			if mastery[s] < 70 {
				p.Missing = append(p.Missing, "skill "+s)
			}
		}
		for _, c := range sp.Capstones {
			total++
			if passed[c] {
				got++
			} else {
				p.Missing = append(p.Missing, "capstone "+c)
			}
		}
		if total > 0 {
			p.Percent = round1(100 * got / total)
		}
		sort.Strings(p.Missing)
		p.Done = p.Unlocked && len(p.Missing) == 0
		cp.Specializations = append(cp.Specializations, p)
	}
	return cp
}

func clamp01(v float64) float64 {
	if v > 1 {
		return 1
	}
	if v < 0 {
		return 0
	}
	return v
}

// Transcript is a verifiable technical profile (Blueprint §20 phase 10): a
// signed summary anyone holding the platform key can verify.
type Transcript struct {
	User       string             `json:"user"`
	Name       string             `json:"name"`
	Issued     time.Time          `json:"issued"`
	Stage      string             `json:"careerStage"`
	Autonomy   string             `json:"autonomy"`
	Dimensions map[string]float64 `json:"dimensions"`
	Branches   map[string]float64 `json:"branches"`
	Labs       []string           `json:"labsPassed"`
	Badges     []string           `json:"badges"`
	Specs      []string           `json:"specializations"`
	Signature  string             `json:"signature"`
}

// NewTranscript signs a transcript for a profile with HMAC-SHA256.
func NewTranscript(p Profile, attempts []Attempt, key []byte, now time.Time) Transcript {
	t := Transcript{User: p.User.ID, Name: p.User.Name, Issued: now.UTC().Truncate(time.Second), Stage: p.Career.Stage, Autonomy: p.Student.Autonomy.Stage, Dimensions: map[string]float64{}, Branches: p.Branches}
	for _, d := range p.Student.Dimensions {
		t.Dimensions[d.Dimension] = d.Score
	}
	for l := range passedLabs(attempts) {
		t.Labs = append(t.Labs, l)
	}
	sort.Strings(t.Labs)
	for _, b := range p.Badges {
		t.Badges = append(t.Badges, b.ID)
	}
	for _, s := range p.Career.Specializations {
		if s.Done {
			t.Specs = append(t.Specs, s.ID)
		}
	}
	t.Signature = t.sign(key)
	return t
}

func (t Transcript) sign(key []byte) string {
	t.Signature = ""
	b, _ := json.Marshal(t)
	m := hmac.New(sha256.New, key)
	m.Write(b)
	return hex.EncodeToString(m.Sum(nil))
}

// Verify checks a transcript signature.
func (t Transcript) Verify(key []byte) bool {
	return hmac.Equal([]byte(t.sign(key)), []byte(t.Signature))
}
