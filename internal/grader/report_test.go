package grader

import (
	"testing"

	"github.com/neodevesp/gcp-trainer/internal/cli"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

func partScore(r *Report, f string) float64 {
	for _, p := range r.Parts {
		if p.Field == f {
			return p.Score
		}
	}
	return -1
}

// The report is checked against the session: evidence must cite what the
// learner looked at, validation needs a check after the last change.
func TestPostIncidentReport(t *testing.T) {
	lab := &scenario.Lab{Type: "incident"}
	sess := &cli.Session{Records: []cli.ExecRecord{
		{Line: "gcloud compute backend-services get-health web-be --global"},
		{Line: "gcloud compute firewall-rules update fw-hc-probes --source-ranges=35.191.0.0/16,130.211.0.0/22"},
		{Line: "curl -s http://34.1.2.3"},
	}}
	ev := map[string]string{
		"symptom":      "La web de banca devuelve 502 a todos los clientes desde las 9:00",
		"evidenceSeen": "get-health de web-be marca todos los backends UNHEALTHY",
		"hypothesis":   "Creo que las sondas no llegan porque el cortafuegos no permite sus rangos",
		"rootCause":    "La regla fw-hc-probes solo permitía un rango que no es el de las sondas de Google",
		"action":       "Actualicé fw-hc-probes con los rangos 35.191.0.0/16 y 130.211.0.0/22",
		"validation":   "curl a la IP del balanceador devuelve 200",
		"prevention":   "Añadir una alerta de backends sin salud y revisar los cambios de cortafuegos con aprobación",
	}
	r := ScoreReport(lab, sess, Submission{Evidence: ev, Lang: "es"})
	if r == nil || r.Overall < 95 {
		t.Fatalf("a complete report backed by the session should score high: %+v", r)
	}
	// same text, but the session never looked or checked
	blind := &cli.Session{Records: []cli.ExecRecord{{Line: "gcloud compute firewall-rules update fw-hc-probes --source-ranges=0.0.0.0/0"}}}
	r2 := ScoreReport(lab, blind, Submission{Evidence: ev, Lang: "es"})
	if partScore(r2, "evidenceSeen") > 30 || partScore(r2, "validation") > 50 {
		t.Fatalf("claims not backed by the session must score low: %+v", r2.Parts)
	}
	if empty := ScoreReport(lab, sess, Submission{}); empty.Overall != 0 {
		t.Fatalf("an empty report scores 0: %+v", empty)
	}
	if ScoreReport(&scenario.Lab{Type: "guided"}, sess, Submission{}) != nil {
		t.Fatal("labs that are not incidents have no report unless the learner writes one")
	}
}
