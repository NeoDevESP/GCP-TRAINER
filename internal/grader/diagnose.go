package grader

import (
	"fmt"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/cli"
	"github.com/neodevesp/gcp-trainer/internal/i18n"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

// The diagnostic tutor: after a hand-in it names the *kind* of mistake, not
// just which check failed, and says what to do next. It reads the commands
// the learner ran, compares them with the reference solution and the process
// report, and never reveals values the learner has not already typed.

// Mistake kinds.
const (
	WrongIdentity        = "wrong_identity"        // permission given to (or changed for) another identity
	WrongResource        = "wrong_resource"        // the right kind of change on another resource
	UnsafeChange         = "unsafe_change"         // destructive or over-broad change the task did not need
	MissingEvidence      = "missing_evidence"      // changes made before looking
	IncompleteValidation = "incomplete_validation" // no check after the last change
)

// Mistake is one diagnosed error.
type Mistake struct {
	Kind     string   `json:"kind"`
	Title    string   `json:"title"`
	Why      string   `json:"why"`
	Next     string   `json:"next"`
	Commands []string `json:"commands,omitempty"` // the learner's own commands that show it
}

type cmdShape struct {
	group, verb, name string
	flags             map[string]string
}

// shape reads "gcloud <group…> <verb> <name> --flags".
func shape(line string) (cmdShape, bool) {
	ws := strings.Fields(line)
	if len(ws) < 3 || ws[0] != "gcloud" {
		return cmdShape{}, false
	}
	s := cmdShape{flags: map[string]string{}}
	var pos []string
	for i := 1; i < len(ws); i++ {
		w := ws[i]
		if strings.HasPrefix(w, "--") {
			k, v, ok := strings.Cut(w[2:], "=")
			if !ok && i+1 < len(ws) && !strings.HasPrefix(ws[i+1], "-") {
				v = ws[i+1]
				i++
			}
			s.flags[k] = strings.Trim(v, `'"`)
			continue
		}
		if w == "alpha" || w == "beta" {
			continue
		}
		pos = append(pos, w)
	}
	if len(pos) < 3 {
		return cmdShape{}, false
	}
	s.group = strings.Join(pos[:len(pos)-2], " ")
	s.verb = pos[len(pos)-2]
	s.name = pos[len(pos)-1]
	return s, true
}

func identityOf(s cmdShape) string {
	for _, k := range []string{"member", "member-email", "service-account"} {
		if v := s.flags[k]; v != "" {
			return strings.ToLower(v)
		}
	}
	return ""
}

func isIAMChange(s cmdShape) bool {
	return strings.Contains(s.verb, "iam-policy-binding") || strings.Contains(s.group, "memberships") || s.flags["member"] != "" || s.flags["member-email"] != ""
}

// Diagnose returns the mistakes found in an attempt.
func Diagnose(lab *scenario.Lab, sess *cli.Session, res *Result, pr *ProcessReport, lang string) []Mistake {
	p := func(es, en string) string { return i18n.P(lang, es, en) }
	var out []Mistake
	// reference changes
	var refs []cmdShape
	refIDs := map[string]bool{}
	for _, l := range strings.Split(lab.Solution, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") || isReadOnly(l) {
			continue
		}
		if s, ok := shape(l); ok {
			refs = append(refs, s)
			if id := identityOf(s); id != "" && !strings.Contains(id, "$") {
				refIDs[id] = true
			}
		}
	}
	failed := res != nil && !res.Passed
	var wrongID, wrongRes []string
	lastChange, lastRead := -1, -1
	for i, r := range sess.Records {
		line := strings.TrimSpace(r.Line)
		if line == "" {
			continue
		}
		if isReadOnly(line) {
			lastRead = i
			continue
		}
		if r.Exit == 0 {
			lastChange = i
		}
		s, ok := shape(line)
		if !ok || r.Exit != 0 {
			continue
		}
		// a permission change for an identity the task is not about
		if id := identityOf(s); id != "" && isIAMChange(s) && len(refIDs) > 0 && !refIDs[id] && !strings.HasPrefix(s.verb, "remove") {
			wrongID = append(wrongID, line)
			continue
		}
		// the same kind of change as the solution, on a resource the solution does not touch
		sameKind, sameName := false, false
		for _, ref := range refs {
			if ref.group == s.group && ref.verb == s.verb {
				sameKind = true
				if ref.name == s.name || strings.Contains(ref.name, "$") {
					sameName = true
				}
			}
		}
		if sameKind && !sameName && s.verb != "create" {
			wrongRes = append(wrongRes, line)
		}
	}
	if len(wrongID) > 0 && failed {
		out = append(out, Mistake{Kind: WrongIdentity,
			Title:    p("Cambiaste permisos de otra identidad", "You changed the permissions of another identity"),
			Why:      p("El acceso se le da a la identidad que hace la llamada: el usuario, el grupo o la cuenta de servicio con la que corre la aplicación. Si das el rol a otra, el error sigue.", "Access is granted to the identity that makes the call: the user, group or service account the application runs as. Granting the role to another one leaves the error in place."),
			Next:     p("Busca quién hace la llamada (registros de auditoría: principalEmail, o la cuenta de servicio de la VM / del servicio) y compara con la que usaste.", "Find who makes the call (audit logs: principalEmail, or the service account of the VM / service) and compare it with the one you used."),
			Commands: wrongID})
	}
	if len(wrongRes) > 0 && failed {
		out = append(out, Mistake{Kind: WrongResource,
			Title:    p("El cambio era del tipo correcto pero en otro recurso", "The change was the right kind, on another resource"),
			Why:      p("Cambiaste un recurso que no interviene en el fallo. Antes de tocar nada, confirma qué recurso exacto sirve el tráfico o recibe la llamada.", "You changed a resource that is not part of the fault. Before touching anything, confirm exactly which resource serves the traffic or receives the call."),
			Next:     p("Sigue el camino de la petición (balanceador → backend → VM / regla → red) y anota el nombre de cada pieza.", "Follow the request path (load balancer → backend → VM / rule → network) and write down the name of each piece."),
			Commands: wrongRes})
	}
	if pr != nil {
		var unsafe []string
		for _, d := range pr.Destructive {
			if !inSolution(lab.Solution, d) {
				unsafe = append(unsafe, d)
			}
		}
		for _, g := range pr.RiskyGrants {
			if !inSolution(lab.Solution, g) {
				unsafe = append(unsafe, g) // a public asset the task asks for is not a mistake
			}
		}
		// opening a rule to the whole internet when the task did not
		for _, r := range sess.Records {
			l := strings.TrimSpace(r.Line)
			if r.Exit == 0 && strings.Contains(l, "0.0.0.0/0") && !isReadOnly(l) && !strings.Contains(lab.Solution, "0.0.0.0/0") && !contains(unsafe, l) {
				unsafe = append(unsafe, l)
			}
		}
		if len(unsafe) > 0 {
			out = append(out, Mistake{Kind: UnsafeChange,
				Title:    p("Hiciste un cambio arriesgado que la tarea no necesitaba", "You made a risky change the task did not need"),
				Why:      p("Borrar, reiniciar o dar permisos amplios (owner, editor, allUsers, 0.0.0.0/0) puede arreglar el síntoma, pero en producción rompe otras cosas o abre un agujero de seguridad.", "Deleting, restarting or granting broad access (owner, editor, allUsers, 0.0.0.0/0) may fix the symptom, but in production it breaks other things or opens a security hole."),
				Next:     p("Busca el cambio mínimo: el rol más pequeño, el rango concreto, el recurso concreto. Si dudas, usa whatif antes de aplicar.", "Look for the smallest change: the narrowest role, the specific range, the specific resource. If unsure, use whatif before applying."),
				Commands: unsafe})
		}
		if len(pr.BlindFixes) > 0 {
			out = append(out, Mistake{Kind: MissingEvidence,
				Title:    p("Cambiaste cosas antes de mirar", "You changed things before looking"),
				Why:      p("Sin evidencias no sabes si tu cambio arregla la causa o solo coincide. En un banco cada cambio necesita una justificación.", "Without evidence you cannot tell whether your change fixes the cause or just happens to coincide. In a bank every change needs a justification."),
				Next:     p("Primero reproduce el síntoma y reúne pruebas (describe, logs, get-health); escribe tu hipótesis; luego cambia una sola cosa.", "First reproduce the symptom and gather evidence (describe, logs, get-health); write down your hypothesis; then change one thing."),
				Commands: pr.BlindFixes})
		}
	}
	if lastChange >= 0 && lastRead < lastChange {
		out = append(out, Mistake{Kind: IncompleteValidation,
			Title:    p("No comprobaste el resultado después del último cambio", "You did not check the result after the last change"),
			Why:      p("Un cambio aplicado no es un problema resuelto. Hay que demostrar que el servicio vuelve a funcionar para quien lo usa.", "An applied change is not a solved problem. You have to show the service works again for the people using it."),
			Next:     p("Repite la prueba del síntoma (curl, la consulta, el acceso del usuario) y guarda la salida como evidencia.", "Repeat the symptom test (curl, the query, the user's access) and keep the output as evidence."),
			Commands: []string{fmt.Sprint(sess.Records[lastChange].Line)}})
	}
	return out
}

func inSolution(solution, line string) bool {
	line = strings.TrimSpace(line)
	return line != "" && strings.Contains(solution, line)
}
