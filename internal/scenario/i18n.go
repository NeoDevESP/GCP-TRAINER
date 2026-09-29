package scenario

import (
	"fmt"
	"sort"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/desk"
	"github.com/neodevesp/gcp-trainer/internal/i18n"
)

// LabEN is the English translation of a lab written in Spanish (the primary
// language). Long fields have their own keys; every other learner-facing
// string (rubric names, check descriptions, people, quiz, ticket comments,
// timeline senders, files) is translated through Text, keyed by the exact
// Spanish text. Commands, checks and identifiers are shared by both languages.
type LabEN struct {
	Title          string            `yaml:"title" json:"-"`
	Summary        string            `yaml:"summary" json:"-"`
	Story          string            `yaml:"story" json:"-"`
	Instructions   string            `yaml:"instructions" json:"-"`
	Objectives     []string          `yaml:"objectives" json:"-"`
	Hints          []string          `yaml:"hints" json:"-"`
	EvidencePrompt string            `yaml:"evidencePrompt" json:"-"`
	Ticket         *TicketEN         `yaml:"ticket" json:"-"`
	Timeline       []string          `yaml:"timeline" json:"-"`
	Text           map[string]string `yaml:"text" json:"-"`
}

// TicketEN translates the ticket header.
type TicketEN struct {
	Summary string `yaml:"summary"`
	Impact  string `yaml:"impact"`
}

// Localized returns the lab in the requested language. Spanish returns the
// lab itself; English returns a copy with the overlay applied (untranslated
// strings fall back to Spanish). Grading logic is identical in both.
func (l *Lab) Localized(lang string) *Lab {
	if l == nil || i18n.Norm(lang) != i18n.EN || l.EN == nil {
		return l
	}
	e := l.EN
	tr := func(s string) string {
		if v, ok := e.Text[strings.TrimSpace(s)]; ok && v != "" {
			return v
		}
		return s
	}
	pick := func(en, es string) string {
		if strings.TrimSpace(en) != "" {
			return en
		}
		return es
	}
	c := *l
	c.Title, c.Summary, c.Story, c.Instructions = pick(e.Title, l.Title), pick(e.Summary, l.Summary), pick(e.Story, l.Story), pick(e.Instructions, l.Instructions)
	c.Objectives = listOverlay(l.Objectives, e.Objectives, tr)
	c.Hints = make([]Hint, len(l.Hints))
	for i, h := range l.Hints {
		h.Text = tr(h.Text)
		if i < len(e.Hints) && e.Hints[i] != "" {
			h.Text = e.Hints[i]
		}
		c.Hints[i] = h
	}
	if l.Evidence != nil {
		ev := *l.Evidence
		ev.Prompt = pick(e.EvidencePrompt, tr(ev.Prompt))
		c.Evidence = &ev
	}
	c.Quiz = make([]Question, len(l.Quiz))
	for i, q := range l.Quiz {
		q.Question, q.Explanation, q.Probe = tr(q.Question), tr(q.Explanation), tr(q.Probe)
		q.Options = listOverlay(q.Options, nil, tr)
		c.Quiz[i] = q
	}
	c.Rubric = make([]RubricItem, len(l.Rubric))
	for i, r := range l.Rubric {
		r.Name = tr(r.Name)
		checks := make([]Check, len(r.Checks))
		for j, ch := range r.Checks {
			cp := Check{}
			for k, v := range ch {
				cp[k] = v
			}
			if d, ok := cp["desc"].(string); ok {
				cp["desc"] = tr(d)
			}
			checks[j] = cp
		}
		r.Checks = checks
		c.Rubric[i] = r
	}
	c.Timeline = make([]TimelineEvent, len(l.Timeline))
	for i, t := range l.Timeline {
		t.From, t.Text = tr(t.From), tr(t.Text)
		if i < len(e.Timeline) && e.Timeline[i] != "" {
			t.Text = e.Timeline[i]
		}
		c.Timeline[i] = t
	}
	if l.Ticket != nil {
		t := *l.Ticket
		t.Reporter = tr(t.Reporter)
		t.Summary, t.Impact = tr(t.Summary), tr(t.Impact)
		if e.Ticket != nil {
			t.Summary, t.Impact = pick(e.Ticket.Summary, t.Summary), pick(e.Ticket.Impact, t.Impact)
		}
		t.Comments = make([]desk.Comment, len(l.Ticket.Comments))
		for i, cm := range l.Ticket.Comments {
			cm.From, cm.Text = tr(cm.From), tr(cm.Text)
			t.Comments[i] = cm
		}
		t.Attachments = make([]desk.Attachment, len(l.Ticket.Attachments))
		for i, a := range l.Ticket.Attachments {
			a.Content = tr(a.Content)
			t.Attachments[i] = a
		}
		c.Ticket = &t
	}
	c.Actors = localizeActors(l.Actors, tr)
	c.Files = map[string]string{}
	for k, v := range l.Files {
		c.Files[k] = tr(v)
	}
	c.Shortcuts = make([]Shortcut, len(l.Shortcuts))
	for i, s := range l.Shortcuts {
		s.Name = tr(s.Name)
		c.Shortcuts[i] = s
	}
	return &c
}

func localizeActors(actors []desk.Actor, tr func(string) string) []desk.Actor {
	out := make([]desk.Actor, len(actors))
	for i, a := range actors {
		a.Name, a.Persona, a.Fallback = tr(a.Name), tr(a.Persona), tr(a.Fallback)
		facts := make([]desk.Fact, len(a.Facts))
		for j, f := range a.Facts {
			f.Answer, f.Content = tr(f.Answer), tr(f.Content)
			facts[j] = f
		}
		a.Facts = facts
		out[i] = a
	}
	return out
}

func listOverlay(es, en []string, tr func(string) string) []string {
	if es == nil {
		return nil
	}
	out := make([]string, len(es))
	for i, s := range es {
		out[i] = tr(s)
		if i < len(en) && en[i] != "" {
			out[i] = en[i]
		}
	}
	return out
}

// TranslationProblems checks the English overlay: it must exist, cover the
// long fields, keep list lengths, translate every learner-facing string and
// hold no stale entries. Content CI runs it for every lab.
func (l *Lab) TranslationProblems() []string {
	var p []string
	e := l.EN
	if e == nil {
		return []string{fmt.Sprintf("lab %s: missing `en:` translation", l.ID)}
	}
	if e.Title == "" || e.Summary == "" || (l.Story != "" && e.Story == "") || (l.Instructions != "" && e.Instructions == "") {
		p = append(p, fmt.Sprintf("lab %s: en.title/summary/story/instructions incomplete", l.ID))
	}
	if l.Evidence != nil && l.Evidence.Prompt != "" && e.EvidencePrompt == "" && e.Text[strings.TrimSpace(l.Evidence.Prompt)] == "" {
		p = append(p, fmt.Sprintf("lab %s: evidence prompt not translated", l.ID))
	}
	if l.Ticket != nil && e.Ticket == nil && e.Text[strings.TrimSpace(l.Ticket.Summary)] == "" {
		p = append(p, fmt.Sprintf("lab %s: ticket not translated", l.ID))
	}
	for name, pair := range map[string][2]int{"objectives": {len(l.Objectives), len(e.Objectives)}, "hints": {len(l.Hints), len(e.Hints)}, "timeline": {len(l.Timeline), len(e.Timeline)}} {
		if pair[1] != 0 && pair[1] != pair[0] {
			p = append(p, fmt.Sprintf("lab %s: en.%s has %d entries, Spanish has %d", l.ID, name, pair[1], pair[0]))
		}
	}
	covered := map[string]bool{}
	for k := range e.Text {
		covered[strings.TrimSpace(k)] = true
	}
	used := map[string]bool{}
	var missing []string
	for _, s := range l.translatableWithSlots() {
		if covered[s.text] {
			used[s.text] = true
			continue
		}
		if !s.dedicated {
			missing = append(missing, s.text)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		p = append(p, fmt.Sprintf("lab %s: %d strings without English translation (e.g. %q)", l.ID, len(missing), trunc60(missing[0])))
	}
	var stale []string
	for k := range covered {
		if !used[k] {
			stale = append(stale, k)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		p = append(p, fmt.Sprintf("lab %s: %d stale en.text entries (e.g. %q)", l.ID, len(stale), trunc60(stale[0])))
	}
	return p
}

type slot struct {
	text      string
	dedicated bool // translated by a dedicated en.* field
}

// translatableWithSlots marks strings that have a dedicated overlay field so
// they need no Text entry.
func (l *Lab) translatableWithSlots() []slot {
	e := l.EN
	var out []slot
	add := func(ded bool, s ...string) {
		for _, x := range s {
			if strings.TrimSpace(x) != "" {
				out = append(out, slot{strings.TrimSpace(x), ded})
			}
		}
	}
	for i, h := range l.Hints {
		add(i < len(e.Hints) && e.Hints[i] != "", h.Text)
	}
	for i, o := range l.Objectives {
		add(i < len(e.Objectives) && e.Objectives[i] != "", o)
	}
	if l.Evidence != nil {
		add(e.EvidencePrompt != "", l.Evidence.Prompt)
	}
	for _, q := range l.Quiz {
		add(false, q.Question, q.Explanation, q.Probe)
		add(false, q.Options...)
	}
	for _, r := range l.Rubric {
		add(false, r.Name)
		for _, ch := range r.Checks {
			if d, ok := ch["desc"].(string); ok {
				add(false, d)
			}
		}
	}
	for i, t := range l.Timeline {
		add(false, t.From)
		add(i < len(e.Timeline) && e.Timeline[i] != "", t.Text)
	}
	if l.Ticket != nil {
		add(false, l.Ticket.Reporter)
		add(e.Ticket != nil && e.Ticket.Summary != "", l.Ticket.Summary)
		add(e.Ticket != nil && e.Ticket.Impact != "", l.Ticket.Impact)
		for _, c := range l.Ticket.Comments {
			add(false, c.From, c.Text)
		}
		for _, a := range l.Ticket.Attachments {
			add(false, a.Content)
		}
	}
	for _, a := range l.Actors {
		add(false, a.Name, a.Persona, a.Fallback)
		for _, f := range a.Facts {
			add(false, f.Answer, f.Content)
		}
	}
	for _, v := range l.Files {
		add(false, v)
	}
	// shortcut names are only shown to authors: translation optional
	for _, sc := range l.Shortcuts {
		add(true, sc.Name)
	}
	return out
}

func trunc60(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 60 {
		return s[:57] + "..."
	}
	return s
}
