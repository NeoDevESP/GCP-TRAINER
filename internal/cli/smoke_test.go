package cli

import (
	"strings"
	"testing"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

func newTestSession() *Session {
	st := sim.New(1, "lab-proj", "user:student@gcplab.dev")
	st.DefaultNetwork(st.Projects["lab-proj"])
	s := NewSession(st, "lab-proj", "student@gcplab.dev")
	return s
}

func run(t *testing.T, s *Session, line string) string {
	t.Helper()
	r := s.Exec(line)
	t.Logf("$ %s\n%s", line, r.Output)
	return r.Output
}

func TestWebVM(t *testing.T) {
	s := newTestSession()
	run(t, s, "gcloud config set compute/zone europe-west1-b")
	out := run(t, s, `gcloud compute instances create web-1 --machine-type=e2-small --tags=http-server --metadata=startup-script='apt-get install -y nginx'`)
	if !strings.Contains(out, "RUNNING") {
		t.Fatal("vm not created")
	}
	ip := strings.TrimSpace(run(t, s, `gcloud compute instances describe web-1 --format='value(networkInterfaces[0].accessConfigs[0].natIP)'`))
	out = run(t, s, "curl -s --max-time 5 http://"+ip)
	if !strings.Contains(out, "timed out") {
		t.Fatal("expected timeout before firewall")
	}
	run(t, s, "gcloud compute firewall-rules create allow-http --network default --allow tcp:80 --target-tags http-server --source-ranges 0.0.0.0/0")
	out = run(t, s, "curl -s http://"+ip)
	if !strings.Contains(out, "Hello from web-1") {
		t.Fatal("expected success")
	}
	run(t, s, "gcloud compute firewall-rules list --format='table(name,priority,sourceRanges.list())'")
	run(t, s, `gcloud projects get-iam-policy lab-proj --flatten="bindings[].members" --format="table(bindings.role,bindings.members)"`)
	run(t, s, "gcloud compute ssh web-1 --command 'sudo systemctl status nginx && curl -s localhost'")
	run(t, s, "gcloud logging read 'logName:cloudaudit' --limit 2 --format='value(protoPayload.methodName)'")
	run(t, s, "gcloud storage buckets create gs://my-docs-123 --location=europe-west1 --uniform-bucket-level-access --public-access-prevention")
	run(t, s, `echo '{"rule":[{"action":{"type":"Delete"},"condition":{"age":365}}]}' > lc.json && gcloud storage buckets update gs://my-docs-123 --lifecycle-file=lc.json && gcloud storage buckets describe gs://my-docs-123 --format=json | jq -r .lifecycle_config.rule[0].condition.age`)
}

func TestTerraform(t *testing.T) {
	s := newTestSession()
	s.Region, s.Zone = "europe-west1", "europe-west1-b"
	s.Files["main.tf"] = `
variable "region" { default = "europe-west1" }
provider "google" {
  project = "lab-proj"
  region  = var.region
  zone    = "europe-west1-b"
}
resource "google_compute_network" "vpc" {
  name                    = "app-vpc"
  auto_create_subnetworks = false
}
resource "google_compute_subnetwork" "web" {
  name          = "web-subnet"
  ip_cidr_range = "10.10.0.0/24"
  network       = google_compute_network.vpc.id
  region        = var.region
}
resource "google_compute_firewall" "http" {
  name    = "allow-http"
  network = google_compute_network.vpc.name
  allow {
    protocol = "tcp"
    ports    = ["80"]
  }
  source_ranges = ["0.0.0.0/0"]
  target_tags   = ["web"]
}
resource "google_compute_instance" "web" {
  name         = "tf-web"
  machine_type = "e2-small"
  tags         = ["web"]
  boot_disk {
    initialize_params { image = "debian-cloud/debian-12" }
  }
  network_interface {
    subnetwork = google_compute_subnetwork.web.self_link
    access_config {}
  }
  metadata_startup_script = "apt-get install -y nginx"
}
output "vm_name" { value = google_compute_instance.web.name }
`
	run(t, s, "terraform init")
	out := run(t, s, "terraform plan")
	if !strings.Contains(out, "4 to add") {
		t.Fatal("plan")
	}
	out = run(t, s, "terraform apply -auto-approve")
	if !strings.Contains(out, "Apply complete") {
		t.Fatal("apply")
	}
	out = run(t, s, "terraform plan")
	if !strings.Contains(out, "No changes") {
		t.Fatal("not idempotent")
	}
	run(t, s, "terraform output -raw vm_name")
	run(t, s, "terraform destroy -auto-approve")
}
