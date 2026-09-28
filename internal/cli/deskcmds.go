package cli

import (
	"fmt"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/desk"
)

const deskHelp = `ticket                     show the ticket, its comments and attachments
ticket comment "text"      post an internal update
ticket update "text"       post a stakeholder/customer-facing status update
ticket resolve "note"      resolve with a resolution note (what was wrong, what changed)
ticket escalate TEAM "why" hand over to another team
team                       list the people involved
ask WHO "question"         ask a person (developer, security, finance, manager, customer, sre)
`

// deskCmd implements the service-desk builtins.
func (s *Session) deskCmd(args []string, stdin string) (string, error) {
	at := s.State.Now()
	if args[0] == "team" {
		if s.Desk == nil || len(s.Desk.Actors) == 0 {
			return "Nobody else is involved in this lab.\n", nil
		}
		var b strings.Builder
		for _, a := range s.Desk.Actors {
			fmt.Fprintf(&b, "%-10s %-22s %s\n", a.Role, a.Name, a.Persona)
		}
		return b.String(), nil
	}
	if args[0] == "ask" {
		if s.Desk == nil {
			return "", fail(1, "nobody else is involved in this lab")
		}
		if len(args) < 3 {
			return "", fail(2, "usage: ask WHO \"question\"")
		}
		ans, file, content, err := s.Desk.Ask(args[1], strings.Join(args[2:], " "), at)
		if err != nil {
			return "", fail(1, "%v", err)
		}
		a := s.Desk.Find(args[1])
		out := fmt.Sprintf("%s (%s): %s\n", a.Name, a.Role, ans)
		if file != "" {
			s.Files[file] = content
			out += fmt.Sprintf("[%s shared %s — see the Files tab or cat %s]\n", a.Name, file, file)
		}
		return out, nil
	}
	// ticket ...
	if s.Desk == nil || s.Desk.Ticket == nil {
		return "", fail(1, "no ticket is attached to this lab")
	}
	if len(args) == 1 || args[1] == "show" {
		return s.Desk.Ticket.Render(), nil
	}
	text := strings.Join(args[2:], " ")
	if text == "" && stdin != "" {
		text = strings.TrimSpace(stdin)
	}
	var err error
	switch args[1] {
	case "comment":
		err = s.Desk.Comment("student", text, at, false)
	case "update":
		err = s.Desk.Comment("student", text, at, true)
	case "resolve":
		err = s.Desk.Resolve("student", text, at)
	case "escalate":
		if len(args) < 4 {
			return "", fail(2, "usage: ticket escalate TEAM \"reason\"")
		}
		err = s.Desk.Escalate("student", args[2], strings.Join(args[3:], " "), at)
	case "help", "--help":
		return deskHelp, nil
	default:
		return "", fail(2, "unknown ticket action %q\n%s", args[1], deskHelp)
	}
	if err != nil {
		return "", fail(1, "%v", err)
	}
	return fmt.Sprintf("%s updated (%s).\n", s.Desk.Ticket.ID, s.Desk.Ticket.Status), nil
}

var _ = desk.Kinds
