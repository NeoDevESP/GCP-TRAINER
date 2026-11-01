package grader

import (
	"regexp"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/cli"
	"github.com/neodevesp/gcp-trainer/internal/i18n"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

// The post-incident report: Symptom → Evidence → Hypothesis → Root cause →
// Action → Validation → Prevention. Each part is scored on its own and
// checked against what the learner actually did in the session (the evidence
// must cite something they looked at, the action something they changed, the
// validation a check made after the last change). The report never changes
// the rubric score: it is its own result, shown next to it.

// ReportFields are the parts of the report, in order. rootCause and
// prevention are the same keys labs already ask for as evidence.
var ReportFields = []string{"symptom", "evidenceSeen", "hypothesis", "rootCause", "action", "validation", "prevention"}

// ReportPart is the score of one part.
type ReportPart struct {
	Field string  `json:"field"`
	Score float64 `json:"score"` // 0..100
	Note  string  `json:"note"`
}

// Report is the scored post-incident report.
type Report struct {
	Parts   []ReportPart `json:"parts"`
	Overall float64      `json:"overall"`
}

var (
	reCausal     = regexp.MustCompile(`(?i)\b(porque|ya que|debido|por eso|si |creo|parece|podría|puede que|hipótesis|because|since|due to|if |i think|seems|could|might|hypothesis)`)
	rePrevention = regexp.MustCompile(`(?i)(alert|monitor|polític|policy|runbook|revis|review|automat|prueba|test|terraform|iac|mínimo privilegio|least privilege|org policy|presupuesto|budget|documenta|document|checklist|aprobación|approval|cambio controlado|change control)`)
	reToken      = regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9._@:/-]{3,}`)
)

// WantsReport says whether a lab asks for the post-incident report.
func WantsReport(lab *scenario.Lab) bool {
	if lab.Type == "incident" || lab.Type == "boss" {
		return true
	}
	if lab.Ticket != nil {
		switch lab.Ticket.Kind {
		case "INC", "PRB", "SEC":
			return true
		}
	}
	return false
}

// names the learner touched: resource names and values in their commands
func sessionTokens(sess *cli.Session, readOnly bool) map[string]bool {
	out := map[string]bool{}
	skip := map[string]bool{"gcloud": true, "kubectl": true, "list": true, "describe": true, "update": true, "create": true, "delete": true, "compute": true, "projects": true, "--format": true, "--zone": true, "--region": true, "--project": true}
	for _, r := range sess.Records {
		if r.Line == "" || isReadOnly(r.Line) != readOnly {
			continue
		}
		for _, w := range strings.Fields(r.Line) {
			_, v, ok := strings.Cut(w, "=")
			if ok {
				w = v
			}
			w = strings.Trim(w, `'"`)
			if len(w) >= 4 && !skip[w] && !strings.HasPrefix(w, "-") {
				out[strings.ToLower(w)] = true
			}
		}
	}
	return out
}

func cites(text string, tokens map[string]bool) bool {
	for _, t := range reToken.FindAllString(strings.ToLower(text), -1) {
		if tokens[strings.Trim(t, ".,;:")] {
			return true
		}
	}
	return false
}

// ScoreReport scores the report in sub.Evidence. It returns nil when the lab
// does not ask for one and the learner did not write one.
func ScoreReport(lab *scenario.Lab, sess *cli.Session, sub Submission) *Report {
	p := func(es, en string) string { return i18n.P(sub.Lang, es, en) }
	written := 0
	for _, f := range ReportFields {
		if strings.TrimSpace(sub.Evidence[f]) != "" {
			written++
		}
	}
	if !WantsReport(lab) && written < 2 {
		return nil
	}
	read, changed := sessionTokens(sess, true), sessionTokens(sess, false)
	// a check after the last change
	lastChange, lastRead := -1, -1
	for i, r := range sess.Records {
		if r.Line == "" {
			continue
		}
		if isReadOnly(r.Line) {
			lastRead = i
		} else if r.Exit == 0 {
			lastChange = i
		}
	}
	rep := &Report{}
	part := func(f string, score float64, note string) {
		rep.Parts = append(rep.Parts, ReportPart{Field: f, Score: score, Note: note})
	}
	for _, f := range ReportFields {
		t := strings.TrimSpace(sub.Evidence[f])
		n := words(t)
		if n == 0 {
			part(f, 0, p("sin escribir", "not written"))
			continue
		}
		short := n < 6
		switch f {
		case "symptom":
			if short {
				part(f, 50, p("describe qué falla, para quién y desde cuándo", "describe what fails, for whom and since when"))
			} else {
				part(f, 100, p("síntoma descrito", "symptom described"))
			}
		case "evidenceSeen":
			switch {
			case cites(t, read):
				part(f, 100, p("cita lo que consultaste en la sesión", "cites what you looked at in the session"))
			case len(read) == 0:
				part(f, 20, p("no consultaste nada antes de concluir", "you looked at nothing before concluding"))
			default:
				part(f, 50, p("nombra el recurso, comando o registro concreto que lo muestra", "name the specific resource, command or log line that shows it"))
			}
		case "hypothesis":
			if reCausal.MatchString(t) && !short {
				part(f, 100, p("hipótesis razonada", "reasoned hypothesis"))
			} else {
				part(f, 50, p("formúlala como causa posible y cómo la comprobarías", "state it as a possible cause and how you would test it"))
			}
		case "rootCause":
			if lab.Evidence != nil && len(lab.Evidence.Keywords) > 0 {
				hit := 0
				for _, g := range lab.Evidence.Keywords {
					for _, w := range g {
						if i18n.ContainsAny(normalize(t), w) {
							hit++
							break
						}
					}
				}
				sc := 100 * float64(hit) / float64(len(lab.Evidence.Keywords))
				part(f, clamp(sc), p("conceptos clave de la causa cubiertos", "key concepts of the cause covered"))
			} else if short {
				part(f, 50, p("explica el mecanismo: qué estaba mal y por qué producía el síntoma", "explain the mechanism: what was wrong and why it caused the symptom"))
			} else {
				part(f, 100, p("causa explicada", "cause explained"))
			}
		case "action":
			switch {
			case cites(t, changed):
				part(f, 100, p("describe el cambio que hiciste", "describes the change you made"))
			case len(changed) == 0:
				part(f, 30, p("en la sesión no hay ningún cambio aplicado", "the session has no applied change"))
			default:
				part(f, 60, p("nombra el recurso y el valor que cambiaste", "name the resource and the value you changed"))
			}
		case "validation":
			switch {
			case lastChange >= 0 && lastRead > lastChange:
				part(f, 100, p("comprobaste el resultado después del cambio", "you checked the result after the change"))
			case lastChange < 0:
				part(f, 60, p("sin cambios que validar", "no change to validate"))
			default:
				part(f, 40, p("lo cuentas, pero no hay ninguna comprobación después del último cambio", "you describe it, but there is no check after the last change"))
			}
		case "prevention":
			if rePrevention.MatchString(t) && !short {
				part(f, 100, p("medida concreta para que no se repita", "concrete measure so it does not happen again"))
			} else {
				part(f, 50, p("propón algo concreto: una alerta, una política, una revisión, una prueba", "propose something concrete: an alert, a policy, a review, a test"))
			}
		}
	}
	var sum float64
	for _, pt := range rep.Parts {
		sum += pt.Score
	}
	rep.Overall = clamp(sum / float64(len(rep.Parts)))
	return rep
}
