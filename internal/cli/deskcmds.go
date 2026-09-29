package cli

import (
	"fmt"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/desk"
)

const deskHelpES = `ticket                        muestra el ticket, sus comentarios y adjuntos
ticket comment "texto"        añade una nota interna
ticket update "texto"         publica una actualización para el cliente o las partes interesadas
ticket resolve "nota"         resuelve con una nota (qué fallaba y qué has cambiado)
ticket escalate EQUIPO "por qué"  pasa el ticket a otro equipo
team                          lista las personas implicadas
ask QUIÉN "pregunta"          pregunta a una persona (developer, security, finance, manager, customer, sre)
`

const deskHelpEN = `ticket                     show the ticket, its comments and attachments
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
	deskHelp := s.tr(deskHelpES, deskHelpEN)
	if s.Desk != nil {
		s.Desk.Lang = s.Lang
	}
	if args[0] == "team" {
		if s.Desk == nil || len(s.Desk.Actors) == 0 {
			return s.tr("No hay nadie más implicado en este laboratorio.\n", "Nobody else is involved in this lab.\n"), nil
		}
		var b strings.Builder
		for _, a := range s.Desk.Actors {
			fmt.Fprintf(&b, "%-10s %-22s %s\n", a.Role, a.Name, a.Persona)
		}
		return b.String(), nil
	}
	if args[0] == "ask" {
		if s.Desk == nil {
			return "", fail(1, "%s", s.tr("no hay nadie más implicado en este laboratorio", "nobody else is involved in this lab"))
		}
		if len(args) < 3 {
			return "", fail(2, "%s", s.tr("uso: ask QUIÉN \"pregunta\"", "usage: ask WHO \"question\""))
		}
		ans, file, content, err := s.Desk.Ask(args[1], strings.Join(args[2:], " "), at)
		if err != nil {
			return "", fail(1, "%v", err)
		}
		a := s.Desk.Find(args[1])
		out := fmt.Sprintf("%s (%s): %s\n", a.Name, a.Role, ans)
		if file != "" {
			s.Files[file] = content
			out += fmt.Sprintf(s.tr("[%s ha compartido %s — míralo en la pestaña Archivos o con cat %s]\n", "[%s shared %s — see the Files tab or cat %s]\n"), a.Name, file, file)
		}
		return out, nil
	}
	// ticket ...
	if s.Desk == nil || s.Desk.Ticket == nil {
		return "", fail(1, "%s", s.tr("este laboratorio no tiene ningún ticket", "no ticket is attached to this lab"))
	}
	if len(args) == 1 || args[1] == "show" {
		return s.Desk.Ticket.Render(s.Lang), nil
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
			return "", fail(2, "%s", s.tr("uso: ticket escalate EQUIPO \"motivo\"", "usage: ticket escalate TEAM \"reason\""))
		}
		err = s.Desk.Escalate("student", args[2], strings.Join(args[3:], " "), at)
	case "help", "--help":
		return deskHelp, nil
	default:
		return "", fail(2, s.tr("acción de ticket desconocida %q\n%s", "unknown ticket action %q\n%s"), args[1], deskHelp)
	}
	if err != nil {
		return "", fail(1, "%v", err)
	}
	return fmt.Sprintf(s.tr("%s actualizado (%s).\n", "%s updated (%s).\n"), s.Desk.Ticket.ID, s.Desk.Ticket.Status), nil
}

var _ = desk.Kinds
