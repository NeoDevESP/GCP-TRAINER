package learning

import (
	"fmt"

	"github.com/neodevesp/gcp-trainer/internal/i18n"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

// TextEN is the English translation of a curriculum item written in Spanish
// (the platform's primary language). Only the fields the item has are used.
type TextEN struct {
	Name        string `yaml:"name"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Observable  string `yaml:"observable"`
	Scenario    string `yaml:"scenario"`
	Guide       string `yaml:"guide"`
}

func pick(en, es string) string {
	if en != "" {
		return en
	}
	return es
}

// ForLang returns the catalog in a language. Spanish is the catalog itself;
// other languages are cached, read-only views with every name, description
// and lab translated. Generated labs are shared by all views.
func (c *Catalog) ForLang(lang string) *Catalog {
	src := c
	if c.base != nil {
		src = c.base
	}
	lang = i18n.Norm(lang)
	if lang == i18n.ES {
		return src
	}
	src.gen.mu.Lock()
	defer src.gen.mu.Unlock()
	if v := src.gen.views[lang]; v != nil {
		return v
	}
	v := &Catalog{Labs: map[string]*scenario.Lab{}, LabOrder: src.LabOrder, skillBranch: src.skillBranch, gen: src.gen, lang: lang, base: src}
	for id, l := range src.Labs {
		v.Labs[id] = l.Localized(lang)
	}
	for _, b := range src.Branches {
		nb := b
		if b.EN != nil {
			nb.Name = pick(b.EN.Name, b.Name)
		}
		nb.Skills = make([]Skill, len(b.Skills))
		for i, s := range b.Skills {
			if s.EN != nil {
				s.Name, s.Observable = pick(s.EN.Name, s.Name), pick(s.EN.Observable, s.Observable)
			}
			nb.Skills[i] = s
		}
		v.Branches = append(v.Branches, nb)
	}
	for _, t := range src.Tracks {
		if t.EN != nil {
			t.Title, t.Description = pick(t.EN.Title, t.Title), pick(t.EN.Description, t.Description)
		}
		v.Tracks = append(v.Tracks, t)
	}
	for _, b := range src.Badges {
		if b.EN != nil {
			b.Name, b.Description = pick(b.EN.Name, b.Name), pick(b.EN.Description, b.Description)
		}
		v.Badges = append(v.Badges, b)
	}
	for _, ce := range src.Certs {
		if ce.EN != nil {
			ce.Name, ce.Guide = pick(ce.EN.Name, ce.Name), pick(ce.EN.Guide, ce.Guide)
		}
		v.Certs = append(v.Certs, ce)
	}
	for _, st := range src.Career.Stages {
		if st.EN != nil {
			st.Name, st.Scenario = pick(st.EN.Name, st.Name), pick(st.EN.Scenario, st.Scenario)
		}
		v.Career.Stages = append(v.Career.Stages, st)
	}
	for _, sp := range src.Career.Specializations {
		if sp.EN != nil {
			sp.Name = pick(sp.EN.Name, sp.Name)
		}
		v.Career.Specializations = append(v.Career.Specializations, sp)
	}
	src.gen.views[lang] = v
	return v
}

// Lang is the language of this catalog view.
func (c *Catalog) Lang() string {
	if c.lang == "" {
		return i18n.ES
	}
	return c.lang
}

// TranslationProblems reports curriculum items and labs without an English
// translation (content CI).
func (c *Catalog) TranslationProblems() []string {
	var p []string
	miss := func(kind, id string, en *TextEN) {
		if en == nil {
			p = append(p, fmt.Sprintf("%s %s: missing `en:` translation", kind, id))
		}
	}
	for _, b := range c.Branches {
		miss("branch", b.ID, b.EN)
		for _, s := range b.Skills {
			miss("skill", s.ID, s.EN)
		}
	}
	for _, t := range c.Tracks {
		miss("track", t.ID, t.EN)
	}
	for _, b := range c.Badges {
		miss("badge", b.ID, b.EN)
	}
	for _, st := range c.Career.Stages {
		miss("career stage", st.ID, st.EN)
	}
	for _, sp := range c.Career.Specializations {
		miss("specialization", sp.ID, sp.EN)
	}
	for _, id := range c.LabOrder {
		p = append(p, c.Labs[id].TranslationProblems()...)
	}
	c.gen.mu.RLock()
	for _, l := range c.gen.labs {
		if l.Company != nil { // company missions ship with the content
			p = append(p, l.TranslationProblems()...)
		}
	}
	c.gen.mu.RUnlock()
	return p
}
