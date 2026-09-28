package learning

import (
	"fmt"
	"sort"
)

// Skill graph (Blueprint §4): prerequisites form a DAG. The graph drives which
// skills are unlocked, what the adaptive engine may schedule next and the
// dependency view in the UI.

// SkillNode is a skill with its graph relations and a learner's state.
type SkillNode struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Branch   string   `json:"branch"`
	Requires []string `json:"requires,omitempty"`
	Unlocks  []string `json:"unlocks,omitempty"`
	Depth    int      `json:"depth"`
}

// SkillMap returns every skill by id.
func (c *Catalog) SkillMap() map[string]Skill {
	out := map[string]Skill{}
	for _, b := range c.Branches {
		for _, s := range b.Skills {
			out[s.ID] = s
		}
	}
	return out
}

// Graph returns the skill graph with reverse edges and depth (longest
// prerequisite chain), sorted by depth then id.
func (c *Catalog) Graph() []SkillNode {
	skills := c.SkillMap()
	unlocks := map[string][]string{}
	for id, s := range skills {
		for _, r := range s.Requires {
			unlocks[r] = append(unlocks[r], id)
		}
	}
	depth := map[string]int{}
	var d func(id string, seen map[string]bool) int
	d = func(id string, seen map[string]bool) int {
		if v, ok := depth[id]; ok {
			return v
		}
		if seen[id] {
			return 0
		}
		seen[id] = true
		best := 0
		for _, r := range skills[id].Requires {
			if x := d(r, seen) + 1; x > best {
				best = x
			}
		}
		depth[id] = best
		return best
	}
	var out []SkillNode
	for id, s := range skills {
		u := unlocks[id]
		sort.Strings(u)
		out = append(out, SkillNode{ID: id, Name: s.Name, Branch: c.BranchOf(id), Requires: s.Requires, Unlocks: u, Depth: d(id, map[string]bool{})})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Depth != out[j].Depth {
			return out[i].Depth < out[j].Depth
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Prerequisites returns the transitive prerequisites of a skill.
func (c *Catalog) Prerequisites(skill string) []string {
	skills := c.SkillMap()
	seen := map[string]bool{}
	var walk func(string)
	walk = func(id string) {
		for _, r := range skills[id].Requires {
			if !seen[r] {
				seen[r] = true
				walk(r)
			}
		}
	}
	walk(skill)
	var out []string
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Unlocked reports whether every direct prerequisite of skill reaches the
// threshold in the given per-skill mastery map (0..100). Skills without
// prerequisites are always unlocked.
func (c *Catalog) Unlocked(skill string, mastery map[string]float64, threshold float64) (bool, []string) {
	var missing []string
	for _, r := range c.SkillMap()[skill].Requires {
		if mastery[r] < threshold {
			missing = append(missing, r)
		}
	}
	return len(missing) == 0, missing
}

func (c *Catalog) validateGraph() []string {
	var problems []string
	skills := c.SkillMap()
	for id, s := range skills {
		for _, r := range s.Requires {
			if _, ok := skills[r]; !ok {
				problems = append(problems, fmt.Sprintf("skill %s: unknown prerequisite %s", id, r))
			}
		}
	}
	// cycle detection (DFS colouring)
	color := map[string]int{}
	var visit func(id string, path []string) bool
	visit = func(id string, path []string) bool {
		switch color[id] {
		case 1:
			problems = append(problems, fmt.Sprintf("skill graph cycle: %v -> %s", path, id))
			return false
		case 2:
			return true
		}
		color[id] = 1
		for _, r := range skills[id].Requires {
			if !visit(r, append(path, id)) {
				return false
			}
		}
		color[id] = 2
		return true
	}
	ids := make([]string, 0, len(skills))
	for id := range skills {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		visit(id, nil)
	}
	return problems
}
