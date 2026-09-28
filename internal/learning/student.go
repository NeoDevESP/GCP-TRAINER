package learning

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Student Model (Blueprint §3): a skill is not a single "85% completed".
// The model keeps nine dimensions, an estimated retention per skill (forgetting
// curve), the autonomy stage and recurring error patterns. It is derived from
// attempts, so it can be recomputed at any time and never needs migrations.

// Dimension names.
const (
	DimKnowledge       = "knowledge"
	DimExecution       = "execution"
	DimTroubleshooting = "troubleshooting"
	DimArchitecture    = "architecture"
	DimSecurity        = "security"
	DimCost            = "cost"
	DimOperations      = "operations"
	DimAutonomy        = "autonomy"
	DimRetention       = "retention"
)

// AllDimensions in display order.
var AllDimensions = []string{DimKnowledge, DimExecution, DimTroubleshooting, DimArchitecture, DimSecurity, DimCost, DimOperations, DimAutonomy, DimRetention}

// DimensionScore is one dimension with its evidence count.
type DimensionScore struct {
	Dimension string  `json:"dimension"`
	Score     float64 `json:"score"`
	Evidence  int     `json:"evidence"` // attempts that measured it
}

// SkillRetention estimates how much of a demonstrated skill is still retained.
type SkillRetention struct {
	Skill        string    `json:"skill"`
	Demonstrated float64   `json:"demonstrated"` // mastery when last practised
	Retention    float64   `json:"retention"`    // estimated now (0..100)
	StabilityDay float64   `json:"stabilityDays"`
	LastPractice time.Time `json:"lastPractice"`
	DueAt        time.Time `json:"dueAt"` // when estimated recall drops below the review threshold
	Due          bool      `json:"due"`
}

// ErrorPattern is a recurring mistake across attempts.
type ErrorPattern struct {
	Pattern string   `json:"pattern"`
	Count   int      `json:"count"`
	Labs    []string `json:"labs"`
	Skills  []string `json:"skills,omitempty"`
}

// AutonomyStage is a rung of the autonomy ladder (Blueprint §21).
type AutonomyStage struct {
	Stage    string  `json:"stage"`
	Index    int     `json:"index"`
	Next     string  `json:"next,omitempty"`
	Progress float64 `json:"progress"` // towards the next stage (0..1)
	Why      string  `json:"why"`
}

// AutonomyLadder: Guided → Assisted → Independent → Professional → Senior → Expert.
var AutonomyLadder = []string{"Guided", "Assisted", "Independent", "Professional", "Senior", "Expert"}

// StudentModel is the individual state of a learner.
type StudentModel struct {
	Dimensions []DimensionScore `json:"dimensions"`
	Retention  []SkillRetention `json:"retention"`
	Autonomy   AutonomyStage    `json:"autonomy"`
	Errors     []ErrorPattern   `json:"recurringErrors"`
	Weakest    []string         `json:"weakestDimensions"`
}

// ReviewThreshold is the recall probability below which a skill is due.
const ReviewThreshold = 0.7

// explicitness returns how much guidance a lab gives (0 = fully guided).
// Production-like, unlabelled problems score higher.
func explicitness(typ, mode string) int {
	switch mode {
	case "unknown":
		return 5
	case "production", "career":
		if typ == "boss" {
			return 5
		}
		return 4
	}
	switch typ {
	case "guided", "quiz":
		return 0
	case "challenge", "case-study":
		return 1
	case "incident":
		return 2
	case "capstone":
		return 3
	case "boss":
		return 4
	}
	return 1
}

// dimensionsOf derives dimension scores (0..100) measured by one attempt.
func (e *Engine) dimensionsOf(a Attempt) map[string]float64 {
	out := map[string]float64{}
	lab := e.Cat.Lab(a.LabID)
	if lab == nil || a.Result == nil {
		return out
	}
	share := func(vs ...string) (float64, bool) {
		var got, max float64
		for _, v := range vs {
			x := a.Result.Validators[v]
			got += x[0]
			max += x[1]
		}
		if max == 0 {
			return 0, false
		}
		return 100 * got / max, true
	}
	factor := func(n string) (float64, bool) {
		if a.Result.Process == nil {
			return 0, false
		}
		for _, f := range a.Result.Process.Factors {
			if f.Name == n {
				return f.Score, true
			}
		}
		return 0, false
	}
	if v, ok := share("quiz"); ok {
		out[DimKnowledge] = v
	} else if v, ok := share("evidence"); ok {
		out[DimKnowledge] = v
	}
	if v, ok := share("state", "functional"); ok {
		out[DimExecution] = v
	}
	if lab.Type == "incident" || lab.Type == "boss" {
		if v, ok := factor("diagnosis"); ok {
			out[DimTroubleshooting] = v
		} else if v, ok := share("diagnosis", "evidence"); ok {
			out[DimTroubleshooting] = v
		}
	}
	if lab.Branch == "architecture" || lab.Type == "case-study" || lab.Type == "capstone" || hasPrefix(lab.Skills, "architecture.") {
		if v, ok := share("quiz", "reliability", "cost", "state"); ok {
			out[DimArchitecture] = v
		}
	}
	if v, ok := factor("security"); ok {
		out[DimSecurity] = v
	} else if v, ok := share("security"); ok {
		out[DimSecurity] = v
	}
	if v, ok := share("cost"); ok {
		out[DimCost] = v
	}
	if hasPrefix(lab.Skills, "observability.") || lab.Type == "incident" || lab.Type == "boss" {
		v, ok := share("functional", "state")
		if d, ok2 := factor("documentation"); ok2 && ok {
			v = 0.7*v + 0.3*d
		}
		if ok {
			out[DimOperations] = v
		}
	}
	aut := 100.0
	if len(lab.Hints) > 0 {
		aut = 100 * (1 - float64(len(a.HintsUsed))/float64(len(lab.Hints)+1))
	}
	if a.SolutionShown {
		aut = 0
	}
	// harder, less explicit problems demonstrate more autonomy
	aut = aut * (0.6 + 0.08*float64(explicitness(lab.Type, lab.Mode)))
	out[DimAutonomy] = math.Min(100, aut)
	return out
}

func hasPrefix(list []string, p string) bool {
	for _, s := range list {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// Retention returns the forgetting-curve estimate per practised skill.
// Stability starts at 3 days and multiplies by 2.5 for every further
// successful, unassisted practice spaced at least one day apart; recall is
// exp(-Δt/S).
func (e *Engine) Retention(attempts []Attempt, skills []SkillScore) []SkillRetention {
	mastery := map[string]float64{}
	for _, s := range skills {
		mastery[s.Skill] = s.Mastery
	}
	type st struct {
		stability float64
		last      time.Time
	}
	per := map[string]*st{}
	for _, a := range submitted(attempts) {
		lab := e.Cat.Lab(a.LabID)
		if lab == nil || !a.Passed {
			continue
		}
		unassisted := len(a.HintsUsed) == 0 && !a.SolutionShown
		for _, sk := range lab.Skills {
			s := per[sk]
			if s == nil {
				per[sk] = &st{stability: 3, last: *a.Finished}
				continue
			}
			if unassisted && a.Finished.Sub(s.last) >= 24*time.Hour {
				s.stability *= 2.5
			}
			s.last = *a.Finished
		}
	}
	now := e.now()
	var out []SkillRetention
	for sk, s := range per {
		dt := now.Sub(s.last).Hours() / 24
		r := math.Exp(-dt / s.stability)
		due := s.last.Add(time.Duration(-s.stability*math.Log(ReviewThreshold)*24) * time.Hour)
		out = append(out, SkillRetention{Skill: sk, Demonstrated: mastery[sk], Retention: round1(mastery[sk] * r), StabilityDay: round1(s.stability), LastPractice: s.last, DueAt: due, Due: now.After(due)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DueAt.Before(out[j].DueAt) })
	return out
}

// Autonomy places the learner on the autonomy ladder. Reaching stage L needs
// three passes of problems at explicitness ≥ L solved with ≤1 hint; Senior and
// Expert additionally require transfer across ≥3 branches.
func (e *Engine) Autonomy(attempts []Attempt) AutonomyStage {
	type pass struct {
		level  int
		branch string
	}
	var passes []pass
	for _, a := range submitted(attempts) {
		lab := e.Cat.Lab(a.LabID)
		if lab == nil || !a.Passed || a.SolutionShown || len(a.HintsUsed) > 1 {
			continue
		}
		passes = append(passes, pass{explicitness(lab.Type, lab.Mode), lab.Branch})
	}
	count := func(level int) (int, int) {
		n := 0
		br := map[string]bool{}
		for _, p := range passes {
			if p.level >= level {
				n++
				br[p.branch] = true
			}
		}
		return n, len(br)
	}
	stage := 0
	for L := 1; L < len(AutonomyLadder); L++ {
		n, branches := count(L - 1)
		need := 3
		ok := n >= need
		if L >= 4 {
			ok = ok && branches >= 3
		}
		if !ok {
			break
		}
		stage = L
	}
	res := AutonomyStage{Stage: AutonomyLadder[stage], Index: stage}
	if stage+1 < len(AutonomyLadder) {
		res.Next = AutonomyLadder[stage+1]
		n, branches := count(stage)
		res.Progress = math.Min(1, float64(n)/3)
		res.Why = fmt.Sprintf("%d/3 unassisted passes at explicitness ≥%d", n, stage)
		if stage+1 >= 4 {
			res.Why += fmt.Sprintf(", %d/3 branches (transfer)", branches)
			if branches < 3 {
				res.Progress = math.Min(res.Progress, float64(branches)/3)
			}
		}
	} else {
		res.Progress, res.Why = 1, "solves unlabelled production problems across domains without assistance"
	}
	return res
}

// RecurringErrors groups failed checks and failed commands across attempts.
func (e *Engine) RecurringErrors(attempts []Attempt) []ErrorPattern {
	type acc struct {
		n      int
		labs   map[string]bool
		skills map[string]bool
	}
	pat := map[string]*acc{}
	bump := func(k, lab string, skills []string) {
		p := pat[k]
		if p == nil {
			p = &acc{labs: map[string]bool{}, skills: map[string]bool{}}
			pat[k] = p
		}
		p.n++
		p.labs[lab] = true
		for _, s := range skills {
			p.skills[s] = true
		}
	}
	for _, a := range submitted(attempts) {
		if a.Result == nil {
			continue
		}
		lab := e.Cat.Lab(a.LabID)
		var skills []string
		if lab != nil {
			skills = lab.Skills
		}
		for _, it := range a.Result.Items {
			for _, c := range it.Checks {
				if !c.Pass {
					bump(checkPattern(c.Type, it.Validator), a.LabID, skills)
				}
			}
		}
		if pr := a.Result.Process; pr != nil {
			if len(pr.BlindFixes) > 0 {
				bump("acts before gathering evidence", a.LabID, skills)
			}
			if len(pr.RiskyGrants) > 0 {
				bump("grants over-broad access (basic roles / allUsers / 0.0.0.0/0)", a.LabID, skills)
			}
			if len(pr.Violations) > 0 {
				bump("violates stated constraints", a.LabID, skills)
			}
		}
	}
	var out []ErrorPattern
	for k, p := range pat {
		if p.n < 2 {
			continue // recurring means at least twice
		}
		ep := ErrorPattern{Pattern: k, Count: p.n}
		for l := range p.labs {
			ep.Labs = append(ep.Labs, l)
		}
		for s := range p.skills {
			ep.Skills = append(ep.Skills, s)
		}
		sort.Strings(ep.Labs)
		sort.Strings(ep.Skills)
		out = append(out, ep)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Count > out[j].Count || (out[i].Count == out[j].Count && out[i].Pattern < out[j].Pattern)
	})
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

func checkPattern(typ, validator string) string {
	switch typ {
	case "iam", "no_basic_roles":
		return "IAM: wrong or excessive permissions"
	case "http", "tcp", "egress", "google_api":
		return "connectivity or service not restored"
	case "policy":
		return "security/cost/reliability policy violated (" + validator + ")"
	case "evidence":
		return "incomplete explanation or postmortem"
	case "quiz":
		return "design / knowledge questions"
	case "command":
		return "skipped diagnostic or measurement step"
	case "cost_max", "bq_bytes_max":
		return "cost target missed"
	case "finding_absent":
		return "security finding left open"
	}
	return "resource state not as required (" + typ + ")"
}

// Student builds the full student model.
func (e *Engine) Student(attempts []Attempt, skills []SkillScore) StudentModel {
	halfLife := 30 * 24 * time.Hour
	now := e.now()
	sum := map[string]float64{}
	wsum := map[string]float64{}
	cnt := map[string]int{}
	for _, a := range submitted(attempts) {
		w := math.Pow(0.5, now.Sub(*a.Finished).Hours()/halfLife.Hours())
		for d, v := range e.dimensionsOf(a) {
			sum[d] += w * v
			wsum[d] += w
			cnt[d]++
		}
	}
	m := StudentModel{}
	m.Retention = e.Retention(attempts, skills)
	if len(m.Retention) > 0 {
		var r, d float64
		for _, x := range m.Retention {
			r += x.Retention
			d += 100
		}
		sum[DimRetention], wsum[DimRetention], cnt[DimRetention] = 100*r/d, 1, len(m.Retention)
	}
	for _, d := range AllDimensions {
		ds := DimensionScore{Dimension: d, Evidence: cnt[d]}
		if wsum[d] > 0 {
			ds.Score = round1(sum[d] / wsum[d])
		}
		m.Dimensions = append(m.Dimensions, ds)
	}
	measured := []DimensionScore{}
	for _, d := range m.Dimensions {
		if d.Evidence > 0 {
			measured = append(measured, d)
		}
	}
	sort.Slice(measured, func(i, j int) bool { return measured[i].Score < measured[j].Score })
	for i := 0; i < len(measured) && i < 2; i++ {
		m.Weakest = append(m.Weakest, measured[i].Dimension)
	}
	m.Autonomy = e.Autonomy(attempts)
	m.Errors = e.RecurringErrors(attempts)
	return m
}
