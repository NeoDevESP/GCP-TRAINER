package cli

import (
	"strings"
	"testing"
)

func TestNATPortsStockoutQuotaAndBQLocation(t *testing.T) {
	s := newTestSession()
	run(t, s, "gcloud compute networks create vpc --subnet-mode=custom")
	run(t, s, "gcloud compute networks subnets create apps --network=vpc --range=10.1.0.0/24 --region=europe-west1")
	run(t, s, "gcloud compute routers create r --network=vpc --region=europe-west1")
	run(t, s, "gcloud compute routers nats create nat --router=r --region=europe-west1 --nat-all-subnet-ip-ranges --auto-allocate-nat-external-ips --enable-logging")
	run(t, s, "gcloud compute instances create fx --zone=europe-west1-b --machine-type=e2-small --subnet=apps --no-address --metadata=sim-egress-connections=900")
	vm := s.State.Projects["lab-proj"].Instances["fx"]
	ep := s.State.VMEndpoint("lab-proj", vm)
	if ok, why := s.State.InternetPath(ep); ok || !strings.Contains(why, "OUT_OF_RESOURCES") {
		t.Fatalf("expected NAT port exhaustion, got %v %s", ok, why)
	}
	if !strings.Contains(run(t, s, "gcloud logging read 'logName:nat_flows' --limit=1"), "OUT_OF_RESOURCES") {
		t.Fatal("NAT drop should be logged")
	}
	if !strings.Contains(run(t, s, "gcloud compute routers get-nat-mapping-info r --region=europe-west1"), "64") {
		t.Fatal("mapping info should show 64 ports")
	}
	run(t, s, "gcloud compute routers nats update nat --router=r --region=europe-west1 --min-ports-per-vm=1024")
	if ok, why := s.State.InternetPath(ep); !ok {
		t.Fatalf("1024 ports should be enough: %s", why)
	}

	// zone capacity vs quota
	s.State.Extra["stockout:europe-west1-b"] = "n2"
	if out := run(t, s, "gcloud compute instances create big --zone=europe-west1-b --machine-type=n2-standard-4 --subnet=apps --no-address"); !strings.Contains(out, "ZONE_RESOURCE_POOL_EXHAUSTED") {
		t.Fatal("expected stockout")
	}
	if out := run(t, s, "gcloud compute instances create big --zone=europe-west1-c --machine-type=n2-standard-4 --subnet=apps --no-address"); strings.Contains(out, "ERROR") {
		t.Fatal("another zone should work")
	}
	s.State.Projects["lab-proj"].Quotas = map[string]float64{"CPUS@europe-west1": 8}
	if out := run(t, s, "gcloud compute instances create big2 --zone=europe-west1-c --machine-type=n2-standard-8 --subnet=apps --no-address"); !strings.Contains(out, "Quota 'CPUS' exceeded") {
		t.Fatal("expected quota error")
	}
	if !strings.Contains(run(t, s, "gcloud compute regions describe europe-west1"), "CPUS") {
		t.Fatal("regions describe should show quotas")
	}
	if !strings.Contains(run(t, s, `gcloud beta quotas preferences create --service=compute.googleapis.com --quota-id=CPUS-per-project-region --preferred-value=64 --dimensions=region=europe-west1 --justification="campana" --email=a@b.c`), "reconciling") {
		t.Fatal("quota request should be accepted as pending")
	}

	// BigQuery location
	run(t, s, "bq mk --dataset --location=EU riesgo")
	run(t, s, "bq mk --table riesgo.saldos id:STRING,importe:NUMERIC")
	if out := run(t, s, "bq query --use_legacy_sql=false --location=US 'SELECT id FROM riesgo.saldos'"); !strings.Contains(out, "was not found in location US") {
		t.Fatal("expected location mismatch")
	}
	if out := run(t, s, "bq query --use_legacy_sql=false --location=EU 'SELECT id FROM riesgo.saldos'"); strings.Contains(out, "not found") {
		t.Fatal("same location should work")
	}
}
