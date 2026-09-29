package learning

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/grader"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

// User is a learner, instructor or admin.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Role         string    `json:"role"` // student, instructor, admin
	PasswordHash string    `json:"passwordHash,omitempty"`
	Created      time.Time `json:"created"`
	Classes      []string  `json:"classes"`
	Leaderboard  bool      `json:"leaderboardOptIn"`
	Provider     string    `json:"provider"` // local, oidc
}

// Attempt is one execution of a lab by a user.
type Attempt struct {
	ID            string            `json:"id"`
	UserID        string            `json:"userId"`
	LabID         string            `json:"labId"`
	Track         string            `json:"track"`
	Seed          int64             `json:"seed"`
	Params        map[string]string `json:"params"`
	Fidelity      string            `json:"fidelity"`
	Started       time.Time         `json:"started"`
	Finished      *time.Time        `json:"finished,omitempty"`
	HintsUsed     []int             `json:"hintsUsed"`
	HintCost      int               `json:"hintCost"`
	SolutionShown bool              `json:"solutionShown"`
	Score         int               `json:"score"`
	Passed        bool              `json:"passed"`
	Critical      bool              `json:"criticalFailed"`
	XP            int               `json:"xp"`
	Bonus         int               `json:"bonus"`
	BonusReasons  []string          `json:"bonusReasons"`
	Result        *grader.Result    `json:"result,omitempty"`
	Retest        bool              `json:"retest"`
	Commands      int               `json:"commands"`
	Errors        int               `json:"errors"`
	ProvisionMs   int64             `json:"provisionMs"`
	Status        string            `json:"status"` // running, submitted, expired, abandoned
}

// DurationSec returns the attempt duration.
func (a *Attempt) DurationSec() float64 {
	if a.Finished == nil {
		return 0
	}
	return a.Finished.Sub(a.Started).Seconds()
}

// Levels of the gamified progression.
var Levels = []struct {
	Name string
	XP   int
}{{"Apprentice", 0}, {"Operator", 500}, {"Engineer", 1500}, {"Senior", 3000}, {"Specialist", 5000}, {"Architect", 8000}}

// LevelFor returns the level for an XP total and progress to the next.
func LevelFor(xp int) (string, int, float64) {
	idx := 0
	for i, l := range Levels {
		if xp >= l.XP {
			idx = i
		}
	}
	if idx == len(Levels)-1 {
		return Levels[idx].Name, idx, 1
	}
	span := float64(Levels[idx+1].XP - Levels[idx].XP)
	return Levels[idx].Name, idx, float64(xp-Levels[idx].XP) / span
}

// BonusCap is the maximum bonus XP per track (3,000 base + max 300 bonus).
const BonusCap = 300

// Award computes XP and bonus for a graded attempt given prior attempts.
// XP is earned by demonstrated competence: first pass awards score minus hint
// costs; later attempts only award the improvement over the best previous XP.
func (e *Engine) Award(a *Attempt, prior []Attempt) {
	lab := e.Cat.Lab(a.LabID)
	best := 0
	passedBefore := false
	var firstPass *Attempt
	for i := range prior {
		p := prior[i]
		if p.LabID != a.LabID || p.ID == a.ID || p.Status != "submitted" {
			continue
		}
		if p.XP > best {
			best = p.XP
		}
		if p.Passed {
			passedBefore = true
			if firstPass == nil || p.Finished.Before(*firstPass.Finished) {
				pp := p
				firstPass = &pp
			}
		}
	}
	raw := a.Score - a.HintCost
	if a.SolutionShown {
		raw = min(raw, a.Score/2)
	}
	if raw < 0 {
		raw = 0
	}
	if !a.Passed {
		raw = raw / 2 // partial credit for the attempt, still below a pass
	}
	a.XP = max(0, raw-best)
	a.Bonus, a.BonusReasons = 0, nil
	if !a.Passed || a.Critical {
		return // bonus never compensates a failure or a critical security miss
	}
	addBonus := func(n int, why string) {
		a.Bonus += n
		a.BonusReasons = append(a.BonusReasons, fmt.Sprintf("+%d %s", n, why))
	}
	if len(a.HintsUsed) == 0 && !a.SolutionShown && !passedBefore {
		addBonus(10, "solved without hints")
	}
	if lab != nil && a.DurationSec() > 0 && a.DurationSec() < float64(lab.Minutes*60)/2 && !passedBefore {
		addBonus(3, "fast (after security and functional checks passed)")
	}
	if lab != nil && lab.RetestOf != "" {
		addBonus(15, "delayed re-test of an equivalent scenario")
		a.Retest = true
	}
	if passedBefore && firstPass != nil && a.Started.Sub(*firstPass.Finished) >= 7*24*time.Hour && len(a.HintsUsed) == 0 {
		addBonus(10, "retention check (≥7 days later, no hints)")
		a.Retest = true
	}
	if lab != nil && (lab.Type == "boss") && a.Score >= 90 {
		addBonus(20, "boss battle ≥90")
	}
	// cap bonus per track
	used := 0
	for _, p := range prior {
		if p.Track == a.Track && p.ID != a.ID {
			used += p.Bonus
		}
	}
	if used+a.Bonus > BonusCap {
		a.Bonus = max(0, BonusCap-used)
		a.BonusReasons = append(a.BonusReasons, "bonus capped at 300 per track")
	}
}

// Engine computes learner analytics from the catalog and attempts.
type Engine struct {
	Cat *Catalog
	Now func() time.Time
	Lib *scenario.Library // optional: enables generated/stealth recommendations
}

// SkillScore is the mastery breakdown of a skill.
type SkillScore struct {
	Skill        string  `json:"skill"`
	Branch       string  `json:"branch"`
	Mastery      float64 `json:"mastery"`
	Correctness  float64 `json:"correctness"`
	Independence float64 `json:"independence"`
	Retention    float64 `json:"retention"`
	Difficulty   float64 `json:"difficulty"`
	Incident     float64 `json:"incidentPerformance"`
	Attempts     int     `json:"attempts"`
	MasteryBadge bool    `json:"masteryBadge"`
}

// Readiness is certification readiness with an explanation.
type Readiness struct {
	Cert      string   `json:"cert"`
	Name      string   `json:"name"`
	Percent   float64  `json:"percent"`
	Strong    []string `json:"strong"`
	Weak      []string `json:"weak"`
	Coverage  float64  `json:"coverage"`
	Explained string   `json:"explanation"`
}

// Badge is an awarded badge.
type Badge struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

// Recommendation suggests the next practice.
type Recommendation struct {
	LabID  string `json:"labId"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
	Kind   string `json:"kind"` // remediation, retest, next, mock, review, stealth, dimension, frontier
	Skill  string `json:"skill,omitempty"`
	// Gen is set when the recommendation is a generated incident.
	Gen *scenario.GenSpec `json:"gen,omitempty"`
}

// Profile is the learner dashboard.
type Profile struct {
	User            User               `json:"user"`
	XP              int                `json:"xp"`
	BonusXP         int                `json:"bonusXp"`
	Level           string             `json:"level"`
	LevelIndex      int                `json:"levelIndex"`
	LevelProgress   float64            `json:"levelProgress"`
	Streak          int                `json:"streak"`
	Skills          []SkillScore       `json:"skills"`
	Branches        map[string]float64 `json:"branches"`
	Readiness       []Readiness        `json:"readiness"`
	Badges          []Badge            `json:"badges"`
	Recommendations []Recommendation   `json:"recommendations"`
	LabsPassed      int                `json:"labsPassed"`
	Attempts        int                `json:"attempts"`
	League          string             `json:"league"`
	Student         StudentModel       `json:"student"`
	Adaptive        []Recommendation   `json:"adaptive"`
	Career          CareerProfile      `json:"career"`
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func submitted(attempts []Attempt) []Attempt {
	var out []Attempt
	for _, a := range attempts {
		if a.Status == "submitted" && a.Finished != nil {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Finished.Before(*out[j].Finished) })
	return out
}

// Mastery computes per-skill mastery with the formula
// 0.40 Correctness + 0.20 Independence + 0.15 Retention + 0.15 Difficulty + 0.10 IncidentPerformance.
func (e *Engine) Mastery(attempts []Attempt) []SkillScore {
	att := submitted(attempts)
	type acc struct {
		bestByLab                            map[string]int
		indep                                []float64
		diff                                 []float64
		incident                             []float64
		firstPass, retestPassAt              time.Time
		retest30                             bool
		n                                    int
		hasChallenge, hasIncident, hasRetest bool
	}
	per := map[string]*acc{}
	for _, a := range att {
		lab := e.Cat.Lab(a.LabID)
		if lab == nil {
			continue
		}
		for _, sk := range lab.Skills {
			s := per[sk]
			if s == nil {
				s = &acc{bestByLab: map[string]int{}}
				per[sk] = s
			}
			s.n++
			if a.Score > s.bestByLab[a.LabID] {
				s.bestByLab[a.LabID] = a.Score
			}
			ind := 100.0
			if len(lab.Hints) > 0 {
				ind = 100 * (1 - float64(len(a.HintsUsed))/float64(len(lab.Hints)+1))
			}
			if a.SolutionShown {
				ind = 0
			}
			s.indep = append(s.indep, ind)
			if a.Passed {
				s.diff = append(s.diff, float64(lab.Difficulty)/5*100)
				if s.firstPass.IsZero() {
					s.firstPass = *a.Finished
				} else if a.Started.Sub(s.firstPass) >= 7*24*time.Hour && len(a.HintsUsed) == 0 && !a.SolutionShown {
					s.retestPassAt = *a.Finished
					s.hasRetest = true
					if a.Started.Sub(s.firstPass) >= 30*24*time.Hour {
						s.retest30 = true
					}
				}
				if lab.RetestOf != "" && len(a.HintsUsed) == 0 && !a.SolutionShown {
					s.hasRetest = true
					if s.retestPassAt.IsZero() {
						s.retestPassAt = *a.Finished
					}
				}
				switch lab.Type {
				case "challenge", "capstone":
					s.hasChallenge = true
				case "incident", "boss":
					s.hasIncident = true
				}
			}
			if lab.Type == "incident" || lab.Type == "boss" || lab.Type == "capstone" {
				s.incident = append(s.incident, float64(a.Score))
			}
		}
	}
	var out []SkillScore
	for sk, s := range per {
		var sum float64
		for _, v := range s.bestByLab {
			sum += float64(v)
		}
		ss := SkillScore{Skill: sk, Branch: e.Cat.BranchOf(sk), Attempts: s.n}
		ss.Correctness = sum / float64(max(1, len(s.bestByLab)))
		ss.Independence = avg(s.indep)
		switch {
		case s.retest30:
			ss.Retention = 100
		case s.hasRetest:
			ss.Retention = 70
		}
		ss.Difficulty = avg(s.diff)
		ss.Incident = avg(s.incident)
		ss.Mastery = round1(0.40*ss.Correctness + 0.20*ss.Independence + 0.15*ss.Retention + 0.15*ss.Difficulty + 0.10*ss.Incident)
		ss.MasteryBadge = s.hasChallenge && s.hasIncident && s.hasRetest
		out = append(out, ss)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Skill < out[j].Skill })
	return out
}

func avg(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// BranchScores aggregates skill mastery over the branch's full skill list,
// so untouched skills count as zero (a user with 9,000 XP can still be weak in networking).
func (e *Engine) BranchScores(skills []SkillScore) map[string]float64 {
	bySkill := map[string]float64{}
	for _, s := range skills {
		bySkill[s.Skill] = s.Mastery
	}
	out := map[string]float64{}
	for _, b := range e.Cat.Branches {
		if len(b.Skills) == 0 {
			continue
		}
		sum := 0.0
		for _, s := range b.Skills {
			sum += bySkill[s.ID]
		}
		out[b.ID] = round1(sum / float64(len(b.Skills)))
	}
	return out
}

func (e *Engine) branchName(id string) string {
	for _, b := range e.Cat.Branches {
		if b.ID == id {
			return b.Name
		}
	}
	return id
}

// ReadinessFor computes readiness per certification blueprint and explains it.
func (e *Engine) ReadinessFor(skills []SkillScore, branches map[string]float64) []Readiness {
	touched := map[string]bool{}
	for _, s := range skills {
		touched[s.Skill] = true
	}
	var out []Readiness
	for _, c := range e.Cat.Certs {
		total, wsum, cov, covN := 0.0, 0.0, 0.0, 0.0
		type bw struct {
			b string
			s float64
		}
		var parts []bw
		for b, w := range c.Weights {
			total += w * branches[b]
			wsum += w
			parts = append(parts, bw{b, branches[b]})
			for _, br := range e.Cat.Branches {
				if br.ID == b {
					for _, s := range br.Skills {
						covN += w
						if touched[s.ID] {
							cov += w
						}
					}
				}
			}
		}
		sort.Slice(parts, func(i, j int) bool { return parts[i].s > parts[j].s })
		r := Readiness{Cert: c.ID, Name: c.Name}
		if wsum > 0 {
			r.Percent = round1(total / wsum)
		}
		if covN > 0 {
			r.Coverage = round1(cov / covN * 100)
		}
		for i, p := range parts {
			if i < 2 && p.s >= 60 {
				r.Strong = append(r.Strong, e.branchName(p.b))
			}
		}
		for i := len(parts) - 1; i >= 0 && len(r.Weak) < 2; i-- {
			if parts[i].s < 70 {
				r.Weak = append(r.Weak, e.branchName(parts[i].b))
			}
		}
		exp := fmt.Sprintf("%.0f%% practical readiness", r.Percent)
		if len(r.Strong) > 0 {
			exp += "; strong in " + strings.Join(r.Strong, " and ")
		}
		if len(r.Weak) > 0 {
			exp += "; weak in " + strings.Join(r.Weak, " and ")
		}
		exp += fmt.Sprintf(" (blueprint coverage %.0f%%)", r.Coverage)
		r.Explained = exp
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Cert < out[j].Cert })
	return out
}

// Streak counts consecutive days (ending today or yesterday) with practice.
func (e *Engine) Streak(attempts []Attempt) int {
	days := map[string]bool{}
	for _, a := range attempts {
		days[a.Started.UTC().Format("2006-01-02")] = true
	}
	d := e.now().UTC()
	if !days[d.Format("2006-01-02")] {
		d = d.Add(-24 * time.Hour)
	}
	n := 0
	for days[d.Format("2006-01-02")] {
		n++
		d = d.Add(-24 * time.Hour)
	}
	return n
}

// Badges evaluates badge rules.
func (e *Engine) Badges(skills []SkillScore, branches map[string]float64, attempts []Attempt) []Badge {
	passed := map[string]bool{}
	branchPassed := map[string]int{}
	for _, a := range submitted(attempts) {
		if a.Passed && !passed[a.LabID] {
			passed[a.LabID] = true
			if l := e.Cat.Lab(a.LabID); l != nil {
				branchPassed[l.Branch]++
			}
		}
	}
	skill := map[string]SkillScore{}
	for _, s := range skills {
		skill[s.Skill] = s
	}
	var out []Badge
	for _, b := range e.Cat.Badges {
		switch b.Kind {
		case "technical":
			if branches[b.Branch] >= b.MinMastery && branchPassed[b.Branch] >= b.MinLabs {
				out = append(out, Badge{ID: b.ID, Name: b.Name, Kind: b.Kind, Reason: fmt.Sprintf("%s mastery %.0f with %d labs", e.branchName(b.Branch), branches[b.Branch], branchPassed[b.Branch])})
			}
		case "mastery":
			if s, ok := skill[b.Skill]; ok && s.MasteryBadge && s.Mastery >= b.MinMastery {
				out = append(out, Badge{ID: b.ID, Name: b.Name, Kind: b.Kind, Reason: "challenge + incident + delayed re-test without help"})
			}
		case "special":
			ok := len(b.Labs) > 0
			for _, l := range b.Labs {
				if !passed[l] {
					ok = false
				}
			}
			if ok {
				out = append(out, Badge{ID: b.ID, Name: b.Name, Kind: b.Kind, Reason: b.Description})
			}
		}
	}
	return out
}

// Recommend suggests remediation packs, due re-tests and the next track step.
func (e *Engine) Recommend(skills []SkillScore, branches map[string]float64, attempts []Attempt, trackID string) []Recommendation {
	passed := map[string]bool{}
	firstPass := map[string]time.Time{}
	for _, a := range submitted(attempts) {
		if a.Passed {
			passed[a.LabID] = true
			if _, ok := firstPass[a.LabID]; !ok {
				firstPass[a.LabID] = *a.Finished
			}
		}
	}
	var recs []Recommendation
	// next step of the track
	if t := e.Cat.Track(trackID); t != nil {
		for _, id := range t.Labs {
			if !passed[id] {
				recs = append(recs, Recommendation{LabID: id, Title: e.Cat.Lab(id).Title, Reason: "next step in " + t.Title, Kind: "next"})
				break
			}
		}
	}
	// remediation on the two weakest branches that have been started
	type bs struct {
		id string
		s  float64
	}
	var bl []bs
	for b, s := range branches {
		bl = append(bl, bs{b, s})
	}
	sort.Slice(bl, func(i, j int) bool {
		if bl[i].s != bl[j].s {
			return bl[i].s < bl[j].s
		}
		return bl[i].id < bl[j].id
	})
	n := 0
	for _, b := range bl {
		if n >= 2 {
			break
		}
		var cands []string
		for _, id := range e.Cat.LabOrder {
			l := e.Cat.Lab(id)
			if l.Branch == b.id && !passed[id] && l.Type != "boss" {
				cands = append(cands, id)
			}
		}
		sort.SliceStable(cands, func(i, j int) bool { return e.Cat.Labs[cands[i]].Difficulty < e.Cat.Labs[cands[j]].Difficulty })
		if len(cands) > 0 {
			recs = append(recs, Recommendation{LabID: cands[0], Title: e.Cat.Labs[cands[0]].Title, Reason: fmt.Sprintf("%s remediation pack (branch score %.0f)", e.branchName(b.id), b.s), Kind: "remediation"})
			n++
		}
	}
	// retests due: equivalent scenario 7 days after first pass
	for _, id := range e.Cat.LabOrder {
		l := e.Cat.Lab(id)
		if l.RetestOf == "" || passed[id] {
			continue
		}
		if t, ok := firstPass[l.RetestOf]; ok && e.now().Sub(t) >= 7*24*time.Hour {
			recs = append(recs, Recommendation{LabID: id, Title: l.Title, Reason: "retention check: equivalent scenario without hints", Kind: "retest"})
		}
	}
	// mock scenario when ACE readiness is high
	for _, id := range e.Cat.LabOrder {
		l := e.Cat.Lab(id)
		if l.Type == "boss" && !passed[id] && branches["networking"] >= 60 && branches["iam"] >= 60 {
			recs = append(recs, Recommendation{LabID: id, Title: l.Title, Reason: "ACE mock scenario / boss battle", Kind: "mock"})
			break
		}
	}
	return recs
}

// League assigns a weekly cohort of learners at the same level.
func League(userID string, levelIdx int, now time.Time) string {
	y, w := now.ISOWeek()
	h := fnv.New32a()
	h.Write([]byte(fmt.Sprintf("%s-%d-%d", userID, y, w)))
	return fmt.Sprintf("%s-%d-W%02d-%c", Levels[levelIdx].Name, y, w, 'A'+rune(h.Sum32()%6))
}

// Profile builds the dashboard for a user.
func (e *Engine) Profile(u User, attempts []Attempt, trackID string) Profile {
	skills := e.Mastery(attempts)
	branches := e.BranchScores(skills)
	p := Profile{User: u, Skills: skills, Branches: branches}
	passed := map[string]bool{}
	for _, a := range attempts {
		if a.Status != "submitted" {
			continue
		}
		p.XP += a.XP + a.Bonus
		p.BonusXP += a.Bonus
		p.Attempts++
		if a.Passed {
			passed[a.LabID] = true
		}
	}
	p.LabsPassed = len(passed)
	p.Level, p.LevelIndex, p.LevelProgress = LevelFor(p.XP)
	p.Streak = e.Streak(attempts)
	p.Readiness = e.ReadinessFor(skills, branches)
	p.Badges = e.Badges(skills, branches, attempts)
	p.Recommendations = e.Recommend(skills, branches, attempts, trackID)
	p.League = League(u.ID, p.LevelIndex, e.now())
	p.Student = e.Student(attempts, skills)
	p.Career = e.Career(skills, branches, p.Student, attempts)
	p.Adaptive = e.Adaptive(attempts, skills, p.Student)
	p.User.PasswordHash = ""
	return p
}

// TrackReport is the end-of-track report (e.g. 30-day ACE).
type TrackReport struct {
	Track           string           `json:"track"`
	LabsCompleted   int              `json:"labsCompleted"`
	LabsTotal       int              `json:"labsTotal"`
	Points          int              `json:"points"`
	PointsMax       int              `json:"pointsMax"`
	BonusXP         int              `json:"bonusXp"`
	NoHints         int              `json:"labsWithoutHints"`
	IncidentsSolved int              `json:"incidentsSolved"`
	IncidentsTotal  int              `json:"incidentsTotal"`
	MedianMTTR      string           `json:"medianMttr"`
	Strongest       []BranchValue    `json:"strongest"`
	Weakest         []BranchValue    `json:"weakest"`
	Recommendations []Recommendation `json:"recommendations"`
	Readiness       []Readiness      `json:"readiness"`
}

// BranchValue is a named score.
type BranchValue struct {
	Branch string  `json:"branch"`
	Score  float64 `json:"score"`
}

// Report builds the track report shown at the end of a track.
func (e *Engine) Report(trackID string, attempts []Attempt) TrackReport {
	t := e.Cat.Track(trackID)
	r := TrackReport{Track: trackID}
	if t == nil {
		return r
	}
	inTrack := map[string]bool{}
	for _, id := range t.Labs {
		inTrack[id] = true
		if l := e.Cat.Lab(id); l != nil && (l.Type == "incident" || l.Type == "boss") {
			r.IncidentsTotal++
		}
	}
	r.LabsTotal = len(t.Labs)
	r.PointsMax = 100 * len(t.Labs)
	best := map[string]Attempt{}
	var mttr []float64
	for _, a := range submitted(attempts) {
		if !inTrack[a.LabID] {
			continue
		}
		r.BonusXP += a.Bonus
		if b, ok := best[a.LabID]; !ok || a.Score > b.Score {
			best[a.LabID] = a
		}
	}
	for id, a := range best {
		r.Points += a.Score
		if a.Passed {
			r.LabsCompleted++
			if len(a.HintsUsed) == 0 && !a.SolutionShown {
				r.NoHints++
			}
			if l := e.Cat.Lab(id); l != nil && (l.Type == "incident" || l.Type == "boss") {
				r.IncidentsSolved++
				mttr = append(mttr, a.DurationSec())
			}
		}
	}
	if len(mttr) > 0 {
		sort.Float64s(mttr)
		m := mttr[len(mttr)/2]
		r.MedianMTTR = fmt.Sprintf("%dm %02ds", int(m)/60, int(m)%60)
	}
	skills := e.Mastery(attempts)
	branches := e.BranchScores(skills)
	var bl []BranchValue
	for b, s := range branches {
		bl = append(bl, BranchValue{e.branchName(b), s})
	}
	sort.Slice(bl, func(i, j int) bool {
		return bl[i].Score > bl[j].Score || (bl[i].Score == bl[j].Score && bl[i].Branch < bl[j].Branch)
	})
	if len(bl) >= 3 {
		r.Strongest = bl[:3]
		r.Weakest = []BranchValue{bl[len(bl)-1], bl[len(bl)-2]}
	}
	r.Recommendations = e.Recommend(skills, branches, attempts, trackID)
	r.Readiness = e.ReadinessFor(skills, branches)
	return r
}
