package grader

import (
	"testing"

	"github.com/neodevesp/gcp-trainer/internal/cli"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

func kinds(ms []Mistake) map[string]bool {
	out := map[string]bool{}
	for _, m := range ms {
		out[m.Kind] = true
	}
	return out
}

func recs(lines ...string) *cli.Session {
	s := &cli.Session{}
	for _, l := range lines {
		s.Records = append(s.Records, cli.ExecRecord{Line: l})
	}
	return s
}

func TestDiagnoseNamesTheKindOfMistake(t *testing.T) {
	lab := &scenario.Lab{Type: "incident", Solution: "gcloud logging read x\ngcloud projects add-iam-policy-binding p --member=serviceAccount:app@p.iam.gserviceaccount.com --role=roles/storage.objectViewer\ngcloud compute firewall-rules update fw-hc --source-ranges=35.191.0.0/16"}
	failed := &Result{Passed: false}
	cases := []struct {
		name string
		sess *cli.Session
		want []string
		not  []string
	}{
		{"role given to the wrong identity", recs("gcloud logging read x", "gcloud projects add-iam-policy-binding p --member=user:ana@zentra.dev --role=roles/storage.objectViewer", "curl -s http://x"), []string{WrongIdentity}, []string{WrongResource, MissingEvidence, IncompleteValidation}},
		{"right change on the wrong rule", recs("gcloud compute firewall-rules list", "gcloud compute firewall-rules update fw-web --source-ranges=35.191.0.0/16", "curl -s http://x"), []string{WrongResource}, []string{WrongIdentity}},
		{"changes before looking, no check after", recs("gcloud compute instances reset vm"), []string{MissingEvidence, IncompleteValidation, UnsafeChange}, nil},
		{"owner granted", recs("gcloud logging read x", "gcloud projects add-iam-policy-binding p --member=serviceAccount:app@p.iam.gserviceaccount.com --role=roles/owner", "curl -s http://x"), []string{UnsafeChange}, []string{WrongIdentity}},
		{"careful and correct", recs("gcloud logging read x", "gcloud projects add-iam-policy-binding p --member=serviceAccount:app@p.iam.gserviceaccount.com --role=roles/storage.objectViewer", "gcloud compute firewall-rules update fw-hc --source-ranges=35.191.0.0/16", "curl -s http://x"), nil, []string{WrongIdentity, WrongResource, UnsafeChange, MissingEvidence, IncompleteValidation}},
	}
	for _, c := range cases {
		pr := AssessProcess(lab, c.sess, failed, Submission{}, 0)
		got := kinds(Diagnose(lab, c.sess, failed, pr, "es"))
		for _, k := range c.want {
			if !got[k] {
				t.Errorf("%s: want %s, got %v", c.name, k, got)
			}
		}
		for _, k := range c.not {
			if got[k] {
				t.Errorf("%s: did not expect %s", c.name, k)
			}
		}
	}
}
