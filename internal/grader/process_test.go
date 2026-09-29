package grader

import (
	"testing"

	"github.com/neodevesp/gcp-trainer/internal/cli"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

func TestReadOnlyClassification(t *testing.T) {
	ro := []string{"gcloud compute instances list", "gcloud logging read 'severity>=ERROR'", "kubectl get pods", "curl -s http://x", "gcloud projects get-iam-policy p", "bq query --dry_run 'x'", "terraform plan",
		"gcloud compute ssh app-1 --zone=z --tunnel-through-iap --command='nc -zv license-1 27000'", `gcloud compute ssh web --command="curl -s localhost && df -h"`,
		"sudo journalctl -u nginx", "systemctl status nginx", "find /var/log -size +1G"}
	rw := []string{"gcloud compute instances delete vm", "gcloud projects add-iam-policy-binding p --member=user:a --role=roles/viewer", "kubectl delete pod x", "gcloud run services update s --region=r",
		"gcloud compute ssh web --command='sudo systemctl restart nginx'", "gcloud compute ssh web --zone=z", "sudo systemctl restart nginx", "find /var/log -name '*.gz' -delete"}
	for _, l := range ro {
		if !isReadOnly(l) {
			t.Errorf("%q should be read-only", l)
		}
	}
	for _, l := range rw {
		if isReadOnly(l) {
			t.Errorf("%q should be mutating", l)
		}
	}
}

func TestBlindRestartPenalised(t *testing.T) {
	lab := &scenario.Lab{Type: "incident", Solution: "gcloud logging read x\ngcloud run services update s", Constraints: []string{"no-downtime", "protect:legacy-batch"}}
	res := &Result{Validators: map[string][2]float64{"functional": {50, 50}}}
	careful := &cli.Session{Records: []cli.ExecRecord{{Line: "gcloud logging read 'x'"}, {Line: "gcloud run services describe s"}, {Line: "gcloud run services update s --update-env-vars=A=1"}}}
	blind := &cli.Session{Records: []cli.ExecRecord{{Line: "gcloud compute instances reset vm"}, {Line: "gcloud compute instances stop legacy-batch"}}}
	a := AssessProcess(lab, careful, res, Submission{}, 0)
	b := AssessProcess(lab, blind, res, Submission{}, 0)
	if a.Overall <= b.Overall {
		t.Fatalf("careful %.1f should beat blind %.1f", a.Overall, b.Overall)
	}
	if len(b.BlindFixes) == 0 || len(b.Violations) < 2 {
		t.Fatalf("expected blind fixes and violations, got %+v", b)
	}
}
