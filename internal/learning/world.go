package learning

// Worlds are the fictional companies the practice routes happen in: the
// handbook a new employee would read (who they are, teams and people, how
// identity and the Google Cloud organization are laid out, the architecture,
// the house rules and how support works). Every text is a [es, en] pair.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// World is a company handbook (content/worlds/ID.yaml).
type World struct {
	ID       string         `yaml:"id"`
	Name     string         `yaml:"name"`
	Tagline  Pair           `yaml:"tagline"`
	Intro    Pair           `yaml:"intro"`
	Track    string         `yaml:"track"`
	Facts    []WorldFact    `yaml:"facts"`
	Sections []WorldSection `yaml:"sections"`
	People   []WorldPerson  `yaml:"people"`
	Glossary []WorldTerm    `yaml:"glossary"`
}

// WorldFact is a headline figure ("4.2 M customers").
type WorldFact struct {
	Value Pair `yaml:"value"`
	Label Pair `yaml:"label"`
}

// WorldSection is one chapter of the handbook.
type WorldSection struct {
	ID      string      `yaml:"id"`
	Title   Pair        `yaml:"title"`
	Body    Pair        `yaml:"body"`
	Diagram Pair        `yaml:"diagram"`
	Cards   []WorldCard `yaml:"cards"`
}

// WorldCard is a block inside a section (a team, a product, a rule…).
type WorldCard struct {
	Title Pair     `yaml:"title"`
	Text  Pair     `yaml:"text"`
	Meta  Pair     `yaml:"meta"`
	Tags  []string `yaml:"tags"`
}

// WorldPerson is a colleague who appears in the tickets.
type WorldPerson struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
	Role  Pair   `yaml:"role"`
	Team  Pair   `yaml:"team"`
	About Pair   `yaml:"about"`
}

// WorldTerm is a glossary entry.
type WorldTerm struct {
	Term Pair `yaml:"term"`
	Def  Pair `yaml:"def"`
}

func loadWorlds(dir string) (map[string]*World, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	sort.Strings(files)
	out := map[string]*World{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var w World
		if err := yaml.Unmarshal(b, &w); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if w.ID == "" {
			return nil, fmt.Errorf("%s: missing id", f)
		}
		out[w.ID] = &w
	}
	return out, nil
}

func (c *Catalog) validateWorlds() []string {
	var problems []string
	for _, w := range c.Worlds {
		if w.Track != "" && c.Track(w.Track) == nil {
			problems = append(problems, fmt.Sprintf("world %s: unknown track %s", w.ID, w.Track))
		}
		check := func(where string, p Pair) {
			if p[0] != "" && p[1] == "" {
				problems = append(problems, fmt.Sprintf("world %s: %s has no English text", w.ID, where))
			}
		}
		check("tagline", w.Tagline)
		check("intro", w.Intro)
		for _, s := range w.Sections {
			check(s.ID+".title", s.Title)
			check(s.ID+".body", s.Body)
			check(s.ID+".diagram", s.Diagram)
			for i, cd := range s.Cards {
				check(fmt.Sprintf("%s.cards[%d]", s.ID, i), cd.Title)
				check(fmt.Sprintf("%s.cards[%d].text", s.ID, i), cd.Text)
				check(fmt.Sprintf("%s.cards[%d].meta", s.ID, i), cd.Meta)
			}
		}
		for _, p := range w.People {
			check(p.Name+".role", p.Role)
			check(p.Name+".about", p.About)
		}
		for _, g := range w.Glossary {
			check("glossary "+g.Term[0], g.Def)
		}
	}
	return problems
}

// WorldList lists the worlds for an index.
func (c *Catalog) WorldList(lang string) []map[string]any {
	var out []map[string]any
	for _, id := range sortedWorldIDs(c.Worlds) {
		w := c.Worlds[id]
		out = append(out, map[string]any{"id": w.ID, "name": w.Name, "tagline": w.Tagline.In(lang), "track": w.Track})
	}
	return out
}

func sortedWorldIDs(m map[string]*World) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// WorldView returns a world in one language, or nil.
func (c *Catalog) WorldView(id, lang string) map[string]any {
	w := c.Worlds[id]
	if w == nil {
		return nil
	}
	facts := []map[string]string{}
	for _, f := range w.Facts {
		facts = append(facts, map[string]string{"value": f.Value.In(lang), "label": f.Label.In(lang)})
	}
	secs := []map[string]any{}
	for _, s := range w.Sections {
		cards := []map[string]any{}
		for _, cd := range s.Cards {
			cards = append(cards, map[string]any{"title": cd.Title.In(lang), "text": cd.Text.In(lang), "meta": cd.Meta.In(lang), "tags": cd.Tags})
		}
		secs = append(secs, map[string]any{"id": s.ID, "title": s.Title.In(lang), "body": s.Body.In(lang), "diagram": s.Diagram.In(lang), "cards": cards})
	}
	people := []map[string]string{}
	for _, p := range w.People {
		people = append(people, map[string]string{"name": p.Name, "email": p.Email, "role": p.Role.In(lang), "team": p.Team.In(lang), "about": p.About.In(lang)})
	}
	gl := []map[string]string{}
	for _, g := range w.Glossary {
		gl = append(gl, map[string]string{"term": g.Term.In(lang), "def": g.Def.In(lang)})
	}
	return map[string]any{"id": w.ID, "name": w.Name, "tagline": w.Tagline.In(lang), "intro": w.Intro.In(lang), "track": w.Track,
		"facts": facts, "sections": secs, "people": people, "glossary": gl}
}
