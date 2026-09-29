package cli

import (
	"strings"
	"testing"
)

// The commands built by the console pages for the platform services.
func TestPlatformConsoleCommands(t *testing.T) {
	s := newTestSession()
	for _, c := range []string{
		"gcloud monitoring uptime create shop-up --resource-type=uptime-url --resource-labels=host=example.com,project_id=lab-proj --path=/ --port=80 --period=5",
		"gcloud monitoring uptime delete shop-up --quiet",
		"gcloud storage buckets create gs://lab-proj-logs --location=europe-west1",
		"gcloud logging sinks create errors storage.googleapis.com/lab-proj-logs --log-filter='severity>=ERROR'",
		"gcloud logging sinks delete errors --quiet",
		"gcloud billing budgets create --billing-account=0X0X0X-0X0X0X-0X0X0X --display-name=monthly --budget-amount=100EUR --threshold-rule=percent=0.5 --threshold-rule=percent=0.9 --threshold-rule=percent=1.0",
		"gcloud billing budgets delete monthly --billing-account=0X0X0X-0X0X0X-0X0X0X --quiet",
		"gcloud compute security-policies create edge",
		"gcloud compute security-policies rules create 1000 --security-policy=edge --action=deny-403 --src-ip-ranges=1.2.3.0/24",
		"gcloud compute security-policies rules create 1100 --security-policy=edge --action=deny-403 --expression='origin.region_code == \"RU\"' --preview",
		"gcloud compute security-policies rules delete 1000 --security-policy=edge --quiet",
		"gcloud compute security-policies delete edge --quiet",
		"gcloud compute networks create a --subnet-mode=custom",
		"gcloud compute networks subnets create a-sub --network=a --range=10.50.0.0/24 --region=europe-west1",
		"gcloud compute networks peerings create a-to-default --network=a --peer-network=default",
		"gcloud compute networks peerings delete a-to-default --network=a --quiet",
		"gcloud source repos create app",
		"gcloud source repos delete app --quiet",
		"gcloud services enable aiplatform.googleapis.com",
		"gcloud ai endpoints create --display-name=churn --region=europe-west1",
		"gcloud scc findings list projects/lab-proj --filter='state=\"ACTIVE\"'",
		"gcloud recommender recommendations list --project=lab-proj --location=global --recommender=google.iam.policy.Recommender",
	} {
		if r := s.Exec(c); r.Exit != 0 {
			t.Errorf("%s\n%s", c, r.Output)
		}
	}
	if len(s.State.Projects["lab-proj"].SecurityPolicies) != 0 || !strings.Contains(s.Exec("gcloud ai endpoints list --region=europe-west1").Output, "churn") {
		t.Fatal("unexpected state")
	}
}
