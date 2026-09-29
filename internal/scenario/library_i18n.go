package scenario

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/neodevesp/gcp-trainer/internal/desk"
	"github.com/neodevesp/gcp-trainer/internal/i18n"
)

// LibEN is the English overlay of a failure-library item (symptom, context,
// system or failure mode) written in Spanish. People, facts and check
// descriptions are translated through Text, keyed by the Spanish text.
type LibEN struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Title       string            `yaml:"title"`
	Summary     string            `yaml:"summary"`
	Service     string            `yaml:"service"`
	Impact      string            `yaml:"impact"`
	Reporter    string            `yaml:"reporter"`
	Topology    string            `yaml:"topology"`
	Sample      string            `yaml:"sample"`
	Vague       []string          `yaml:"vague"`
	Hints       []string          `yaml:"hints"`
	Dangerous   []string          `yaml:"dangerous"`
	Fixes       []string          `yaml:"fixes"`
	Misleading  map[string]string `yaml:"misleading"`
	Text        map[string]string `yaml:"text"`
}

func (e *LibEN) tr(s string) string {
	if e != nil {
		if v := e.Text[strings.TrimSpace(s)]; v != "" {
			return v
		}
	}
	return s
}

func pickStr(en, es string) string {
	if strings.TrimSpace(en) != "" {
		return en
	}
	return es
}

func overlayList(es, en []string) []string {
	if es == nil {
		return nil
	}
	out := append([]string{}, es...)
	for i := range out {
		if i < len(en) && en[i] != "" {
			out[i] = en[i]
		}
	}
	return out
}

func localizeChecks(cs []Check, e *LibEN) []Check {
	out := make([]Check, len(cs))
	for i, c := range cs {
		cp := Check{}
		for k, v := range c {
			cp[k] = v
		}
		if d, ok := cp["desc"].(string); ok {
			cp["desc"] = e.tr(d)
		}
		out[i] = cp
	}
	return out
}

func localizeFacts(fs []desk.Fact, e *LibEN) []desk.Fact {
	out := make([]desk.Fact, len(fs))
	for i, f := range fs {
		f.Answer = e.tr(f.Answer)
		out[i] = f
	}
	return out
}

var libViews sync.Map // *Library -> *Library (English view)

// Localized returns the library in a language: Spanish is the library
// itself; English is a cached copy with every overlay applied.
func (lib *Library) Localized(lang string) *Library {
	if i18n.Norm(lang) != i18n.EN {
		return lib
	}
	if v, ok := libViews.Load(lib); ok {
		return v.(*Library)
	}
	en := &Library{Symptoms: map[string]*Symptom{}, Contexts: map[string]*Context{}, Systems: map[string]*System{}, Failures: map[string]*FailureMode{}}
	for id, s := range lib.Symptoms {
		c := *s
		if e := s.EN; e != nil {
			c.Name, c.Description = pickStr(e.Name, s.Name), pickStr(e.Description, s.Description)
			c.Vague = overlayList(s.Vague, e.Vague)
			c.Misleading = map[string]string{}
			for k, v := range s.Misleading {
				c.Misleading[k] = pickStr(e.Misleading[k], v)
			}
		}
		en.Symptoms[id] = &c
	}
	for id, x := range lib.Contexts {
		c := *x
		if e := x.EN; e != nil {
			c.Service, c.Impact, c.Reporter = pickStr(e.Service, x.Service), pickStr(e.Impact, x.Impact), pickStr(e.Reporter, x.Reporter)
		}
		en.Contexts[id] = &c
	}
	for id, sys := range lib.Systems {
		c := *sys
		e := sys.EN
		if e != nil {
			c.Title, c.Topology = pickStr(e.Title, sys.Title), pickStr(e.Topology, sys.Topology)
		}
		c.Actors = make([]desk.Actor, len(sys.Actors))
		for i, a := range sys.Actors {
			a.Name, a.Persona, a.Fallback = e.tr(a.Name), e.tr(a.Persona), e.tr(a.Fallback)
			a.Facts = localizeFacts(a.Facts, e)
			c.Actors[i] = a
		}
		c.Healthy, c.Safety = localizeChecks(sys.Healthy, e), localizeChecks(sys.Safety, e)
		c.Failures = nil
		for _, f := range sys.Failures {
			fc := *f
			if fe := f.EN; fe != nil {
				fc.Title, fc.Summary = pickStr(fe.Title, f.Title), pickStr(fe.Summary, f.Summary)
				fc.Dangerous, fc.Fixes = overlayList(f.Dangerous, fe.Dangerous), overlayList(f.Fixes, fe.Fixes)
				fc.Evidence.Sample = pickStr(fe.Sample, f.Evidence.Sample)
				fc.Hints = make([]Hint, len(f.Hints))
				for i, h := range f.Hints {
					if i < len(fe.Hints) && fe.Hints[i] != "" {
						h.Text = fe.Hints[i]
					}
					fc.Hints[i] = h
				}
			}
			fc.Facts = map[string][]desk.Fact{}
			for r, fs := range f.Facts {
				fc.Facts[r] = localizeFacts(fs, f.EN)
			}
			fc.Checks = localizeChecks(f.Checks, f.EN)
			c.Failures = append(c.Failures, &fc)
			en.Failures[fc.ID] = &fc
		}
		en.Systems[id] = &c
	}
	v, _ := libViews.LoadOrStore(lib, en)
	return v.(*Library)
}

// TranslationProblems lists library texts without their English overlay.
func (lib *Library) TranslationProblems() []string {
	var p []string
	need := func(what string, e *LibEN, texts ...string) {
		if e == nil {
			p = append(p, fmt.Sprintf("failure library %s: missing `en:` translation", what))
			return
		}
		for _, t := range texts {
			if strings.TrimSpace(t) != "" && e.Text[strings.TrimSpace(t)] == "" {
				p = append(p, fmt.Sprintf("failure library %s: %q not translated", what, trunc60(t)))
			}
		}
	}
	checkDescs := func(cs []Check) []string {
		var out []string
		for _, c := range cs {
			if d, ok := c["desc"].(string); ok {
				out = append(out, d)
			}
		}
		return out
	}
	for id, s := range lib.Symptoms {
		need("symptom "+id, s.EN)
	}
	for id, c := range lib.Contexts {
		need("context "+id, c.EN)
	}
	for id, sys := range lib.Systems {
		var texts []string
		for _, a := range sys.Actors {
			texts = append(texts, a.Name, a.Persona, a.Fallback)
			for _, f := range a.Facts {
				texts = append(texts, f.Answer)
			}
		}
		texts = append(texts, checkDescs(sys.Healthy)...)
		texts = append(texts, checkDescs(sys.Safety)...)
		need("system "+id, sys.EN, texts...)
		for _, f := range sys.Failures {
			var ft []string
			for _, fs := range f.Facts {
				for _, x := range fs {
					ft = append(ft, x.Answer)
				}
			}
			ft = append(ft, checkDescs(f.Checks)...)
			need("failure "+f.ID, f.EN, ft...)
			if f.EN != nil && len(f.EN.Hints) != len(f.Hints) {
				p = append(p, fmt.Sprintf("failure library failure %s: en.hints has %d entries, Spanish has %d", f.ID, len(f.EN.Hints), len(f.Hints)))
			}
		}
	}
	sort.Strings(p)
	return p
}

// pairOverlay builds the English overlay of a lab generated in Spanish from
// the same lab generated in English (same spec and seed, so both have the
// same structure).
func pairOverlay(es, en *Lab) *LabEN {
	e := &LabEN{Title: en.Title, Summary: en.Summary, Story: en.Story, Instructions: en.Instructions,
		Objectives: append([]string{}, en.Objectives...), Text: map[string]string{}}
	for _, h := range en.Hints {
		e.Hints = append(e.Hints, h.Text)
	}
	if en.Evidence != nil {
		e.EvidencePrompt = en.Evidence.Prompt
	}
	if en.Ticket != nil {
		e.Ticket = &TicketEN{Summary: en.Ticket.Summary, Impact: en.Ticket.Impact}
	}
	for _, t := range en.Timeline {
		e.Timeline = append(e.Timeline, t.Text)
	}
	a, b := textSlots(es), textSlots(en)
	for i := range a {
		if i < len(b) && strings.TrimSpace(a[i]) != "" {
			e.Text[strings.TrimSpace(a[i])] = b[i]
		}
	}
	return e
}

// textSlots lists, in a fixed order, the strings a LabEN translates through Text.
func textSlots(l *Lab) []string {
	var out []string
	for _, r := range l.Rubric {
		out = append(out, r.Name)
		for _, ch := range r.Checks {
			if d, ok := ch["desc"].(string); ok {
				out = append(out, d)
			}
		}
	}
	for _, t := range l.Timeline {
		out = append(out, t.From)
	}
	if l.Ticket != nil {
		out = append(out, l.Ticket.Reporter)
		for _, c := range l.Ticket.Comments {
			out = append(out, c.From, c.Text)
		}
	}
	for _, a := range l.Actors {
		out = append(out, a.Name, a.Persona, a.Fallback)
		for _, f := range a.Facts {
			out = append(out, f.Answer)
		}
	}
	return out
}
