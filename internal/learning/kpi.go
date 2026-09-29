package learning

import (
	"sort"
	"strings"
	"time"
)

// LabKPI are per-lab learning indicators (used to calibrate difficulty).
type LabKPI struct {
	LabID            string  `json:"labId"`
	Users            int     `json:"users"`
	FirstAttemptSucc float64 `json:"firstAttemptSuccess"`
	HintDependency   float64 `json:"hintDependency"`
	RetryToMastery   float64 `json:"retryToMastery"`
	AvgScore         float64 `json:"avgScore"`
	RootCauseRate    float64 `json:"rootCauseRate"`
	MedianMinutes    float64 `json:"medianMinutes"`
	Flag             string  `json:"flag,omitempty"`
}

// KPIs aggregates learning and platform indicators.
type KPIs struct {
	FirstAttemptSuccess float64           `json:"firstAttemptSuccess"`
	HintDependency      float64           `json:"hintDependency"`
	RetryToMastery      float64           `json:"retryToMastery"`
	Retention7          float64           `json:"retention7d"`
	Retention30         float64           `json:"retention30d"`
	IncidentRootCause   float64           `json:"incidentRootCauseRate"`
	CapstonePassRate    float64           `json:"capstonePassRate"`
	SkillCoverage       float64           `json:"skillCoverage"`
	ActiveUsers         int               `json:"activeUsers"`
	Attempts            int               `json:"attempts"`
	ProvisionP95Ms      int64             `json:"provisionP95Ms"`
	GraderErrorRate     float64           `json:"graderErrorRate"`
	AutoGradedLabs      float64           `json:"autoGradedLabs"`
	InfraFailureRate    float64           `json:"infraFailureRate"`
	PerLab              []LabKPI          `json:"perLab"`
	Targets             map[string]string `json:"targets"`
}

// ComputeKPIs derives KPIs from all attempts (optionally for a class).
func (e *Engine) ComputeKPIs(all []Attempt) KPIs {
	k := KPIs{Targets: map[string]string{
		"firstAttemptSuccess": "45–70% depending on difficulty", "hintDependency": "<30% after 2nd exposure",
		"retryToMastery": "1–3 retries", "retention7d": ">70%", "retention30d": ">60%", "incidentRootCauseRate": ">70% intermediate",
		"capstonePassRate": "50–70% first attempt", "skillCoverage": ">90% target skills", "provisionP95Ms": "F0/F1 <60s, F2 <4min",
		"graderErrorRate": "<1%", "autoGradedLabs": ">80%", "infraFailureRate": "<2% of sessions",
	}}
	byUserLab := map[string][]Attempt{}
	users := map[string]bool{}
	var provision []int64
	infraFail, graderErr, checks := 0, 0, 0
	for _, a := range all {
		users[a.UserID] = true
		if a.ProvisionMs > 0 {
			provision = append(provision, a.ProvisionMs)
		}
		if a.Status == "error" {
			infraFail++
		}
		if a.Status != "submitted" {
			continue
		}
		k.Attempts++
		byUserLab[a.UserID+"|"+a.LabID] = append(byUserLab[a.UserID+"|"+a.LabID], a)
		if a.Result != nil {
			for _, it := range a.Result.Items {
				for _, c := range it.Checks {
					checks++
					if strings.HasPrefix(c.Detail, "grader error") {
						graderErr++
					}
				}
			}
		}
	}
	k.ActiveUsers = len(users)
	if len(all) > 0 {
		k.InfraFailureRate = pct(infraFail, len(all))
	}
	if checks > 0 {
		k.GraderErrorRate = pct(graderErr, checks)
	}
	sort.Slice(provision, func(i, j int) bool { return provision[i] < provision[j] })
	if len(provision) > 0 {
		k.ProvisionP95Ms = provision[int(float64(len(provision)-1)*0.95)]
	}
	graded := 0
	for _, l := range e.Cat.Labs {
		if len(l.Rubric) > 0 {
			graded++
		}
	}
	if len(e.Cat.Labs) > 0 {
		k.AutoGradedLabs = pct(graded, len(e.Cat.Labs))
	}
	type labAgg struct {
		users, firstOK, hintUsers, masteredRetries, mastered int
		scores, minutes                                      []float64
		rcOK, rcN                                            int
	}
	per := map[string]*labAgg{}
	firstOK, firstN, hintN, hintYes, retriesSum, retriesN := 0, 0, 0, 0, 0, 0
	capOK, capN := 0, 0
	rcOK, rcN := 0, 0
	ret7ok, ret7n, ret30ok, ret30n := 0, 0, 0, 0
	for key, as := range byUserLab {
		labID := strings.SplitN(key, "|", 2)[1]
		sort.Slice(as, func(i, j int) bool { return as[i].Started.Before(as[j].Started) })
		la := per[labID]
		if la == nil {
			la = &labAgg{}
			per[labID] = la
		}
		la.users++
		firstN++
		if as[0].Passed {
			firstOK++
			la.firstOK++
		}
		if len(as) > 1 {
			hintN++
			if len(as[1].HintsUsed) > 0 {
				hintYes++
				la.hintUsers++
			}
		}
		for i, a := range as {
			la.scores = append(la.scores, float64(a.Score))
			la.minutes = append(la.minutes, a.DurationSec()/60)
			if a.Passed && len(a.HintsUsed) == 0 {
				retriesSum += i
				retriesN++
				la.masteredRetries += i
				la.mastered++
				break
			}
		}
		lab := e.Cat.Lab(labID)
		if lab != nil && lab.Type == "capstone" {
			capN++
			if as[0].Passed {
				capOK++
			}
		}
		if lab != nil && (lab.Type == "incident" || lab.Type == "boss") {
			for _, a := range as {
				if a.Result == nil {
					continue
				}
				for _, it := range a.Result.Items {
					if strings.Contains(strings.ToLower(it.Name), "root cause") {
						rcN++
						la.rcN++
						if it.Earned >= float64(it.Points)*0.7 {
							rcOK++
							la.rcOK++
						}
					}
				}
			}
		}
		// retention: any later attempt ≥7/30 days after first pass
		var first *Attempt
		for i := range as {
			if as[i].Passed {
				first = &as[i]
				break
			}
		}
		if first != nil {
			for _, a := range as {
				d := a.Started.Sub(*first.Finished)
				if d >= 30*24*time.Hour {
					ret30n++
					if a.Passed {
						ret30ok++
					}
				} else if d >= 7*24*time.Hour {
					ret7n++
					if a.Passed {
						ret7ok++
					}
				}
			}
		}
	}
	// retention via equivalent retest labs
	for _, a := range all {
		if a.Status == "submitted" && a.Retest {
			ret7n++
			if a.Passed {
				ret7ok++
			}
		}
	}
	k.FirstAttemptSuccess = pct(firstOK, firstN)
	k.HintDependency = pct(hintYes, hintN)
	if retriesN > 0 {
		k.RetryToMastery = round1(float64(retriesSum) / float64(retriesN))
	}
	k.Retention7 = pct(ret7ok, ret7n)
	k.Retention30 = pct(ret30ok, ret30n)
	k.IncidentRootCause = pct(rcOK, rcN)
	k.CapstonePassRate = pct(capOK, capN)
	covered := map[string]bool{}
	total := 0
	for _, b := range e.Cat.Branches {
		total += len(b.Skills)
	}
	for _, a := range all {
		if l := e.Cat.Lab(a.LabID); l != nil && a.Status == "submitted" {
			for _, s := range l.Skills {
				covered[s] = true
			}
		}
	}
	k.SkillCoverage = pct(len(covered), total)
	for id, la := range per {
		lk := LabKPI{LabID: id, Users: la.users, FirstAttemptSucc: pct(la.firstOK, la.users), HintDependency: pct(la.hintUsers, la.users), AvgScore: round1(avg(la.scores)), RootCauseRate: pct(la.rcOK, la.rcN)}
		if la.mastered > 0 {
			lk.RetryToMastery = round1(float64(la.masteredRetries) / float64(la.mastered))
		}
		sort.Float64s(la.minutes)
		if len(la.minutes) > 0 {
			lk.MedianMinutes = round1(la.minutes[len(la.minutes)/2])
		}
		if la.users >= 5 {
			switch {
			case lk.FirstAttemptSucc < 30:
				lk.Flag = "too hard or grader false-negatives: review"
			case lk.FirstAttemptSucc > 90:
				lk.Flag = "too easy: raise difficulty"
			}
		}
		k.PerLab = append(k.PerLab, lk)
	}
	sort.Slice(k.PerLab, func(i, j int) bool { return k.PerLab[i].LabID < k.PerLab[j].LabID })
	return k
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return round1(float64(a) / float64(b) * 100)
}

// LeaderboardEntry is one row of a league leaderboard.
type LeaderboardEntry struct {
	UserID string  `json:"userId"`
	Name   string  `json:"name"`
	Points float64 `json:"points"`
	Level  string  `json:"level"`
}

// Leaderboard ranks opted-in users of a league by XP normalised by difficulty
// over the current week.
func (e *Engine) Leaderboard(users []User, attempts []Attempt, league string) []LeaderboardEntry {
	now := e.now()
	y, w := now.ISOWeek()
	var out []LeaderboardEntry
	for _, u := range users {
		if !u.Leaderboard {
			continue
		}
		var mine []Attempt
		xp := 0
		for _, a := range attempts {
			if a.UserID == u.ID {
				mine = append(mine, a)
				if a.Status == "submitted" {
					xp += a.XP + a.Bonus
				}
			}
		}
		_, idx, _ := LevelFor(xp)
		if League(u.ID, idx, now) != league {
			continue
		}
		pts := 0.0
		for _, a := range mine {
			if a.Status != "submitted" || a.Finished == nil {
				continue
			}
			ay, aw := a.Finished.ISOWeek()
			if ay != y || aw != w {
				continue
			}
			d := 2.0
			if l := e.Cat.Lab(a.LabID); l != nil {
				d = float64(l.Difficulty)
			}
			pts += float64(a.XP) * d / 3
		}
		name, _, _ := LevelForLang(xp, e.Lang)
		out = append(out, LeaderboardEntry{UserID: u.ID, Name: u.Name, Points: round1(pts), Level: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Points > out[j].Points })
	return out
}
