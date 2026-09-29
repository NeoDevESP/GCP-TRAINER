// Package desk simulates the service desk around a lab (Blueprint §8): a
// ticket (INC, REQ, CHG, PRB, SEC, COST, MIG) with priority, SLA, impact,
// comments and attachments, plus simulated actors (Developer, Security,
// Finance, Manager, Customer, SRE) the learner must question to obtain
// evidence. Initial information can be imperfect: a ticket may say "looks
// like the network" while the real cause is IAM.
package desk

import (
	"fmt"
	"github.com/neodevesp/gcp-trainer/internal/i18n"
	"sort"
	"strings"
	"time"
)

// Ticket kinds.
var Kinds = []string{"INC", "REQ", "CHG", "PRB", "SEC", "COST", "MIG"}

// Comment is a ticket update.
type Comment struct {
	At     string `yaml:"at" json:"at"`
	From   string `yaml:"from" json:"from"`
	Text   string `yaml:"text" json:"text"`
	Public bool   `yaml:"public" json:"public,omitempty"` // customer-visible update
}

// Attachment is a file attached to the ticket (screenshot text, log excerpt...).
type Attachment struct {
	Name    string `yaml:"name" json:"name"`
	Content string `yaml:"content" json:"content"`
}

// Ticket is the work item that frames a lab.
type Ticket struct {
	ID          string       `yaml:"id" json:"id"`
	Kind        string       `yaml:"kind" json:"kind"`
	Priority    string       `yaml:"priority" json:"priority"` // P1..P4
	SLA         string       `yaml:"sla" json:"sla"`           // e.g. 30m, 4h, 2d
	Summary     string       `yaml:"summary" json:"summary"`
	Impact      string       `yaml:"impact" json:"impact"`
	Reporter    string       `yaml:"reporter" json:"reporter"`
	Service     string       `yaml:"service" json:"service,omitempty"`
	Comments    []Comment    `yaml:"comments" json:"comments"`
	Attachments []Attachment `yaml:"attachments" json:"attachments,omitempty"`
	Status      string       `yaml:"status" json:"status"` // NEW, IN_PROGRESS, RESOLVED, ESCALATED
	Resolution  string       `yaml:"resolution" json:"resolution,omitempty"`
	EscalatedTo string       `yaml:"escalatedTo" json:"escalatedTo,omitempty"`
	// Misleading marks that the initial summary deliberately points to the
	// wrong layer (used by the generator at higher difficulty).
	Misleading bool `yaml:"misleading" json:"-"`
}

// Fact is something an actor knows and reveals when asked about it.
type Fact struct {
	Keywords [][]string `yaml:"keywords" json:"-"` // every group must match (OR inside a group)
	Answer   string     `yaml:"answer" json:"-"`
	Evidence string     `yaml:"evidence" json:"-"` // optional file name added to the workspace
	Content  string     `yaml:"content" json:"-"`
}

// Actor is a simulated person.
type Actor struct {
	Role     string `yaml:"role" json:"role"` // developer, security, finance, manager, customer, sre
	Name     string `yaml:"name" json:"name"`
	Persona  string `yaml:"persona" json:"persona,omitempty"`
	Facts    []Fact `yaml:"facts" json:"-"`
	Fallback string `yaml:"fallback" json:"-"`
}

// Question is a logged interaction with an actor.
type Question struct {
	At       string `json:"at"`
	Actor    string `json:"actor"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Useful   bool   `json:"useful"`
}

// Desk is the per-session service desk state.
type Desk struct {
	Ticket    *Ticket    `json:"ticket,omitempty"`
	Actors    []Actor    `json:"actors,omitempty"`
	Questions []Question `json:"questions,omitempty"`
}

// Clone deep-copies the desk.
func (d *Desk) Clone() *Desk {
	if d == nil {
		return nil
	}
	n := &Desk{Actors: d.Actors, Questions: append([]Question{}, d.Questions...)}
	if d.Ticket != nil {
		t := *d.Ticket
		t.Comments = append([]Comment{}, d.Ticket.Comments...)
		n.Ticket = &t
	}
	return n
}

// SLADuration parses the ticket SLA.
func (t *Ticket) SLADuration() time.Duration {
	s := strings.TrimSpace(t.SLA)
	if s == "" {
		switch t.Priority {
		case "P1":
			return 30 * time.Minute
		case "P2":
			return 4 * time.Hour
		case "P3":
			return 24 * time.Hour
		}
		return 72 * time.Hour
	}
	if strings.HasSuffix(s, "d") {
		var n int
		fmt.Sscan(strings.TrimSuffix(s, "d"), &n)
		return time.Duration(n) * 24 * time.Hour
	}
	d, _ := time.ParseDuration(s)
	return d
}

func normalize(s string) string {
	s = strings.ToLower(s)
	r := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n")
	return r.Replace(s)
}

// Find returns an actor by role or name (case-insensitive).
func (d *Desk) Find(who string) *Actor {
	w := normalize(who)
	for i := range d.Actors {
		if normalize(d.Actors[i].Role) == w || normalize(d.Actors[i].Name) == w || strings.HasPrefix(normalize(d.Actors[i].Name), w) {
			return &d.Actors[i]
		}
	}
	return nil
}

// Ask asks an actor a question. Deterministic: the first fact whose keyword
// groups all match the question answers it. It returns the answer and an
// optional evidence file to add to the workspace.
func (d *Desk) Ask(who, question, at string) (string, string, string, error) {
	a := d.Find(who)
	if a == nil {
		var roles []string
		for _, x := range d.Actors {
			roles = append(roles, x.Role)
		}
		sort.Strings(roles)
		if len(roles) == 0 {
			return "", "", "", fmt.Errorf("nobody else is involved in this ticket")
		}
		return "", "", "", fmt.Errorf("unknown person %q (available: %s)", who, strings.Join(roles, ", "))
	}
	q := normalize(question)
	for _, f := range a.Facts {
		ok := len(f.Keywords) > 0
		for _, g := range f.Keywords {
			hit := false
			for _, k := range g {
				if i18n.ContainsAny(q, k) {
					hit = true
					break
				}
			}
			if !hit {
				ok = false
				break
			}
		}
		if ok {
			d.Questions = append(d.Questions, Question{At: at, Actor: a.Role, Question: question, Answer: f.Answer, Useful: true})
			return f.Answer, f.Evidence, f.Content, nil
		}
	}
	fb := a.Fallback
	if fb == "" {
		fb = "I'm not sure — can you be more specific? I can tell you what I saw or what changed."
	}
	d.Questions = append(d.Questions, Question{At: at, Actor: a.Role, Question: question, Answer: fb})
	return fb, "", "", nil
}

// Comment adds an update to the ticket.
func (d *Desk) Comment(from, text, at string, public bool) error {
	if d.Ticket == nil {
		return fmt.Errorf("no ticket is attached to this lab")
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("empty comment")
	}
	d.Ticket.Comments = append(d.Ticket.Comments, Comment{At: at, From: from, Text: text, Public: public})
	if d.Ticket.Status == "" || d.Ticket.Status == "NEW" {
		d.Ticket.Status = "IN_PROGRESS"
	}
	return nil
}

// Resolve closes the ticket with a resolution note.
func (d *Desk) Resolve(from, note, at string) error {
	if d.Ticket == nil {
		return fmt.Errorf("no ticket is attached to this lab")
	}
	if len(strings.Fields(note)) < 5 {
		return fmt.Errorf("a resolution note needs at least five words (what was wrong, what you changed)")
	}
	d.Ticket.Status, d.Ticket.Resolution = "RESOLVED", note
	d.Ticket.Comments = append(d.Ticket.Comments, Comment{At: at, From: from, Text: "RESOLVED: " + note})
	return nil
}

// Escalate hands the ticket to another team.
func (d *Desk) Escalate(from, team, reason, at string) error {
	if d.Ticket == nil {
		return fmt.Errorf("no ticket is attached to this lab")
	}
	d.Ticket.Status, d.Ticket.EscalatedTo = "ESCALATED", team
	d.Ticket.Comments = append(d.Ticket.Comments, Comment{At: at, From: from, Text: "ESCALATED to " + team + ": " + reason})
	return nil
}

// StudentComments returns the comments written by the learner.
func (d *Desk) StudentComments(student string) []Comment {
	if d == nil || d.Ticket == nil {
		return nil
	}
	var out []Comment
	for _, c := range d.Ticket.Comments {
		if c.From == student {
			out = append(out, c)
		}
	}
	return out
}

// Render formats the ticket for the terminal.
func (t *Ticket) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  [%s %s]  SLA %s  status %s\n", t.ID, t.Kind, t.Priority, t.SLA, firstNonEmpty(t.Status, "NEW"))
	fmt.Fprintf(&b, "Summary:  %s\n", t.Summary)
	if t.Impact != "" {
		fmt.Fprintf(&b, "Impact:   %s\n", t.Impact)
	}
	if t.Reporter != "" {
		fmt.Fprintf(&b, "Reporter: %s\n", t.Reporter)
	}
	for _, a := range t.Attachments {
		fmt.Fprintf(&b, "Attachment: %s (cat %s)\n", a.Name, a.Name)
	}
	if len(t.Comments) > 0 {
		b.WriteString("\n")
		for _, c := range t.Comments {
			fmt.Fprintf(&b, "  %s  %-22s %s\n", c.At, c.From, c.Text)
		}
	}
	return b.String()
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
