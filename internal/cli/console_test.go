package cli

import (
	"strings"
	"testing"
)

// The graphical console edits VMs with these commands.
func TestInstanceLabelsAndDetachDisk(t *testing.T) {
	s := newTestSession()
	run(t, s, "gcloud compute instances create vm1 --zone=europe-west1-b")
	run(t, s, "gcloud compute instances add-labels vm1 --zone=europe-west1-b --labels=env=dev,team=web")
	vm := s.State.Projects["lab-proj"].Instances["vm1"]
	if vm.Labels["env"] != "dev" || vm.Labels["team"] != "web" {
		t.Fatalf("labels not added: %v", vm.Labels)
	}
	run(t, s, "gcloud compute instances remove-labels vm1 --zone=europe-west1-b --labels=team")
	if _, ok := vm.Labels["team"]; ok || vm.Labels["env"] != "dev" {
		t.Fatalf("label not removed: %v", vm.Labels)
	}
	if out := run(t, s, "gcloud compute instances add-labels vm1 --zone=europe-west1-b"); !strings.Contains(out, "KEY=VALUE") {
		t.Fatal("add-labels without --labels should fail")
	}

	run(t, s, "gcloud compute disks create data1 --zone=europe-west1-b --size=10GB")
	run(t, s, "gcloud compute instances attach-disk vm1 --zone=europe-west1-b --disk=data1")
	if out := run(t, s, "gcloud compute instances detach-disk vm1 --zone=europe-west1-b --disk=vm1"); !strings.Contains(out, "boot disk") {
		t.Fatal("detaching the boot disk should fail")
	}
	run(t, s, "gcloud compute instances detach-disk vm1 --zone=europe-west1-b --disk=data1")
	if contains(vm.Disks, "data1") || len(s.State.Projects["lab-proj"].Disks["data1"].Users) != 0 {
		t.Fatalf("disk still attached: %v", vm.Disks)
	}
	if out := run(t, s, "gcloud compute instances detach-disk vm1 --zone=europe-west1-b --disk=data1"); !strings.Contains(out, "not attached") {
		t.Fatal("second detach should fail")
	}
}

func TestLoadBalancerTeardownAndUpdates(t *testing.T) {
	s := newTestSession()
	for _, l := range []string{
		"gcloud compute instance-templates create tpl --machine-type=e2-small",
		"gcloud compute instance-groups managed create mig --template=tpl --size=1 --zone=europe-west1-b",
		"gcloud compute health-checks create http hc --port=80",
		"gcloud compute backend-services create be --protocol=HTTP --health-checks=hc --global",
		"gcloud compute url-maps create um --default-service=be",
		"gcloud compute target-http-proxies create px --url-map=um",
		"gcloud compute forwarding-rules create fr --global --target-http-proxy=px --ports=80",
	} {
		if r := s.Exec(l); r.Exit != 0 {
			t.Fatalf("%s: %s", l, r.Output)
		}
	}
	if out := run(t, s, "gcloud compute url-maps delete um --quiet"); !strings.Contains(out, "already being used") {
		t.Fatal("url map in use must not be deleted")
	}
	if out := run(t, s, "gcloud compute target-http-proxies delete px --quiet"); !strings.Contains(out, "already being used") {
		t.Fatal("proxy in use must not be deleted")
	}
	for _, l := range []string{"gcloud compute forwarding-rules delete fr --global --quiet", "gcloud compute target-http-proxies delete px --quiet", "gcloud compute url-maps delete um --quiet"} {
		if r := s.Exec(l); r.Exit != 0 {
			t.Fatalf("%s: %s", l, r.Output)
		}
	}
	p := s.State.Projects["lab-proj"]
	if p.URLMaps["um"] != nil || p.TargetProxies["px"] != nil {
		t.Fatal("load balancer pieces not deleted")
	}

	run(t, s, "gcloud iam service-accounts create app-svc")
	run(t, s, "gcloud iam service-accounts update app-svc@lab-proj.iam.gserviceaccount.com --display-name='App backend'")
	if p.ServiceAccounts["app-svc@lab-proj.iam.gserviceaccount.com"].DisplayName != "App backend" {
		t.Fatal("display name not updated")
	}

	run(t, s, "gcloud artifacts repositories create repo --repository-format=docker --location=europe-west1")
	if out := run(t, s, "gcloud artifacts repositories delete repo --location=us-central1 --quiet"); !strings.Contains(out, "NOT_FOUND") {
		t.Fatal("wrong location must not delete")
	}
	run(t, s, "gcloud artifacts repositories delete repo --location=europe-west1 --quiet")
	if p.ArtifactRepos["repo"] != nil {
		t.Fatal("repository not deleted")
	}
}
