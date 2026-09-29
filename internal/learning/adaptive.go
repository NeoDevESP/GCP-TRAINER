package learning

import (
	"fmt"
	"sort"

	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

// Adaptive Learning Engine (Blueprint §10, roadmap phase 7). It decides what
// to teach or re-assess next from mastery, forgetting, recurring errors and
// the skill graph:
//
//  1. Spaced repetition — skills whose estimated recall dropped below the
//     review threshold come back, preferably *embedded* in a generated
//     incident of another system (stealth assessment: the learner is not told
//     that IAM is being examined inside a Cloud Run outage).
//  2. Weakest dimension — a weak troubleshooting dimension schedules an
//     incident, weak architecture a case study, weak knowledge a guided lab.
//  3. Skill-graph frontier — skills whose prerequisites are mastered but that
//     have not been practised yet.
//  4. Recurring errors — remediation on the skills involved.

// AdaptiveThreshold is the mastery a prerequisite needs to unlock a skill.
const AdaptiveThreshold = 60

// Adaptive returns ordered, de-duplicated recommendations.
func (e *Engine) Adaptive(attempts []Attempt, skills []SkillScore, sm StudentModel) []Recommendation {
	passed := passedLabs(attempts)
	mastery := map[string]float64{}
	for _, s := range skills {
		mastery[s.Skill] = s.Mastery
	}
	var recs []Recommendation
	seen := map[string]bool{}
	add := func(r Recommendation) {
		key := r.LabID
		if r.Gen != nil {
			key = r.Gen.ID()
		}
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		recs = append(recs, r)
	}
	// 1. spaced repetition / stealth assessment
	for _, r := range sm.Retention {
		if !r.Due || len(recs) >= 3 {
			continue
		}
		if spec := e.stealthSpec(r.Skill, attempts); spec != nil {
			add(Recommendation{Kind: "stealth", Title: e.p("Incidente en producción", "Production incident"), Reason: fmt.Sprintf(e.p("comprobación de retención (recuerdo estimado %.0f%%) dentro de un incidente sin avisar", "retention check (estimated recall %.0f%%) embedded in an unannounced incident"), 100*r.Retention/max(1, r.Demonstrated)), Gen: spec, Skill: r.Skill})
			continue
		}
		if id := e.labForSkill(r.Skill, passed, true); id != "" {
			add(Recommendation{LabID: id, Title: e.Cat.Lab(id).Title, Kind: "review", Reason: fmt.Sprintf(e.p("repaso espaciado de %s (pendiente desde %s)", "spaced review of %s (due since %s)"), r.Skill, r.DueAt.Format("2006-01-02")), Skill: r.Skill})
		}
	}
	// 2. weakest dimension
	for _, d := range sm.Weakest {
		var want []string
		switch d {
		case DimTroubleshooting, DimOperations:
			want = []string{"incident"}
		case DimArchitecture, DimCost:
			want = []string{"case-study", "challenge", "capstone"}
		case DimKnowledge:
			want = []string{"guided", "quiz", "challenge"}
		case DimSecurity:
			want = []string{"incident", "challenge"}
		case DimAutonomy:
			want = []string{"incident", "boss"}
		default:
			continue
		}
		if id := e.labOfType(want, passed, d); id != "" {
			add(Recommendation{LabID: id, Title: e.Cat.Lab(id).Title, Kind: "dimension", Reason: e.p("refuerza tu dimensión más débil: ", "strengthen your weakest dimension: ") + e.dimName(d)})
		} else if e.Lib != nil && (d == DimTroubleshooting || d == DimAutonomy) {
			add(Recommendation{Kind: "dimension", Title: e.p("Incidente generado", "Generated incident"), Reason: fmt.Sprintf(e.p("refuerza %s con un incidente nuevo y sin etiquetar", "strengthen %s with a new, unlabelled incident"), e.dimName(d)), Gen: &scenario.GenSpec{System: "three-tier", Difficulty: 3, Seed: int64(len(attempts) + 11)}})
		}
	}
	// 3. frontier of the skill graph
	var frontier []string
	for _, n := range e.Cat.Graph() {
		if mastery[n.ID] > 0 {
			continue
		}
		if ok, _ := e.Cat.Unlocked(n.ID, mastery, AdaptiveThreshold); ok && len(n.Requires) > 0 {
			frontier = append(frontier, n.ID)
		}
	}
	sort.Strings(frontier)
	for _, sk := range frontier {
		if len(recs) >= 8 {
			break
		}
		if id := e.labForSkill(sk, passed, false); id != "" {
			add(Recommendation{LabID: id, Title: e.Cat.Lab(id).Title, Kind: "frontier", Reason: e.p("recién desbloqueada en el grafo de habilidades: ", "newly unlocked in the skill graph: ") + sk, Skill: sk})
		}
	}
	// 4. recurring errors
	for _, ep := range sm.Errors {
		for _, sk := range ep.Skills {
			if id := e.labForSkill(sk, passed, false); id != "" {
				add(Recommendation{LabID: id, Title: e.Cat.Lab(id).Title, Kind: "remediation", Reason: e.p("error recurrente: ", "recurring error: ") + ep.Pattern, Skill: sk})
				break
			}
		}
	}
	return recs
}

// stealthSpec builds a generated incident whose failure exercises the skill,
// preferring a system the learner has not used for that skill.
func (e *Engine) stealthSpec(skill string, attempts []Attempt) *scenario.GenSpec {
	if e.Lib == nil {
		return nil
	}
	var systems []string
	for id, sys := range e.Lib.Systems {
		for _, f := range sys.Failures {
			for _, s := range f.Skills {
				if s == skill {
					systems = append(systems, id)
				}
			}
		}
	}
	if len(systems) == 0 {
		return nil
	}
	sort.Strings(systems)
	systems = uniq(systems)
	sys := systems[len(attempts)%len(systems)]
	return &scenario.GenSpec{System: sys, Difficulty: 3, Mode: "production", Focus: []string{skill}, Seed: int64(len(attempts)*31 + 7)}
}

func uniq(xs []string) []string {
	var out []string
	for i, x := range xs {
		if i == 0 || x != xs[i-1] {
			out = append(out, x)
		}
	}
	return out
}

// labForSkill finds a catalogue lab practising the skill; retest prefers a
// passed lab's retest variant or any lab not passed yet.
func (e *Engine) labForSkill(skill string, passed map[string]bool, retest bool) string {
	var best string
	bestD := 99
	for _, id := range e.Cat.LabOrder {
		l := e.Cat.Lab(id)
		has := false
		for _, s := range l.Skills {
			if s == skill {
				has = true
			}
		}
		if !has || l.Type == "boss" {
			continue
		}
		if passed[id] && !retest {
			continue
		}
		d := l.Difficulty
		if passed[id] {
			d += 3 // prefer unseen equivalent labs for reviews
		}
		if d < bestD {
			best, bestD = id, d
		}
	}
	return best
}

func (e *Engine) labOfType(types []string, passed map[string]bool, dim string) string {
	for _, t := range types {
		for _, id := range e.Cat.LabOrder {
			l := e.Cat.Lab(id)
			if l.Type == t && !passed[id] {
				return id
			}
		}
	}
	return ""
}
