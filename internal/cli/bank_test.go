package cli

import (
	"strings"
	"testing"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

func TestHybridBGPAdvertisementAndDNSForwarding(t *testing.T) {
	s := newTestSession()
	run(t, s, "gcloud compute networks create vpc --subnet-mode=custom")
	run(t, s, "gcloud compute networks subnets create apps --network=vpc --range=10.1.0.0/24 --region=europe-west1")
	run(t, s, "gcloud compute instances create app --zone=europe-west1-b --machine-type=e2-small --subnet=apps --no-address")
	run(t, s, "gcloud compute routers create r1 --network=vpc --region=europe-west1 --asn=64512")
	run(t, s, "gcloud compute vpn-gateways create gw --network=vpc --region=europe-west1")
	run(t, s, "gcloud compute vpn-tunnels create t0 --vpn-gateway=gw --region=europe-west1 --peer-address=203.0.113.5 --router=r1 --interface=0")
	run(t, s, "gcloud compute routers add-bgp-peer r1 --region=europe-west1 --peer-name=cpd --peer-asn=65001 --interface=if-t0")
	s.State.OnPrem = &sim.OnPrem{Name: "cpd", ASN: 65010, Ranges: []string{"192.168.0.0/16"}, DNSServer: "192.168.10.53",
		Hosts: []sim.OnPremHost{{Name: "core.zentra.corp", IP: "192.168.20.10", Ports: []int{443, 80}}}}
	vm := s.State.Projects["lab-proj"].Instances["app"]
	from := s.State.VMEndpoint("lab-proj", vm)
	if r := s.State.HTTP(sim.HTTPRequest{From: from, URL: "http://192.168.20.10/"}); r.Status == 200 {
		t.Fatal("BGP with the wrong peer ASN must not route")
	}
	if out := run(t, s, "gcloud compute routers get-status r1 --region=europe-west1"); !strings.Contains(out, "Bad Peer AS") {
		t.Fatalf("get-status should explain the ASN mismatch:\n%s", out)
	}
	run(t, s, "gcloud compute routers update-bgp-peer r1 --region=europe-west1 --peer-name=cpd --peer-asn=65010")
	if r := s.State.HTTP(sim.HTTPRequest{From: from, URL: "http://192.168.20.10/"}); r.Status != 200 {
		t.Fatalf("VM should reach the data centre: %+v", r)
	}
	// custom advertisement without the subnet: no route back
	run(t, s, "gcloud compute routers update r1 --region=europe-west1 --advertisement-mode=custom --set-advertisement-ranges=10.9.0.0/24")
	if r := s.State.HTTP(sim.HTTPRequest{From: from, URL: "http://192.168.20.10/"}); r.Status == 200 {
		t.Fatal("subnet not advertised: the data centre has no route back")
	}
	run(t, s, "gcloud compute routers update r1 --region=europe-west1 --set-advertisement-groups=all_subnets")
	// DNS forwarding needs 35.199.192.0/19 advertised
	run(t, s, "gcloud dns managed-zones create corp --dns-name=zentra.corp. --visibility=private --networks=vpc --forwarding-targets=192.168.10.53 --description=cpd")
	if _, ok := s.State.ResolveHost("core.zentra.corp", from); ok {
		t.Fatal("forwarding without a route back for 35.199.192.0/19 must fail")
	}
	run(t, s, "gcloud compute routers update r1 --region=europe-west1 --add-advertisement-ranges=35.199.192.0/19")
	if ip, ok := s.State.ResolveHost("core.zentra.corp", from); !ok || ip != "192.168.20.10" {
		t.Fatalf("forwarding should resolve core.zentra.corp, got %q", ip)
	}
	// from the data centre to the VM needs an ingress rule
	src, _ := s.State.OnPremEndpoint("core")
	dst := s.State.VMEndpoint("lab-proj", vm)
	if fl := s.State.CheckFlow(src, dst, "tcp", 8080); fl.Allowed {
		t.Fatal("no ingress rule from the data centre yet")
	}
	run(t, s, "gcloud compute firewall-rules create allow-cpd --network=vpc --allow=tcp:8080 --source-ranges=192.168.0.0/16")
	if fl := s.State.CheckFlow(src, dst, "tcp", 8080); !fl.Allowed {
		t.Fatalf("data centre should reach the VM: %+v", fl)
	}
}

func TestVPCSCDenyAndCMEK(t *testing.T) {
	s := newTestSession()
	p := s.State.Projects["lab-proj"]
	run(t, s, "gcloud access-context-manager perimeters create datos --title=Datos --resources=projects/"+p.Number+" --restricted-services=bigquery.googleapis.com,storage.googleapis.com")
	bi := sim.ParseFrom("serviceAccount:looker@bi.iam.gserviceaccount.com", "project:zentra-bi/418230557192")
	if ok, _ := s.State.APICall("lab-proj", "bigquery.googleapis.com", bi); ok {
		t.Fatal("outside caller must be blocked")
	}
	s.Files["ingress.yaml"] = `- ingressFrom:
    identities:
    - serviceAccount:looker@bi.iam.gserviceaccount.com
    sources:
    - resource: projects/418230557192
  ingressTo:
    operations:
    - serviceName: bigquery.googleapis.com
      methodSelectors:
      - method: "*"
    resources:
    - "*"
`
	run(t, s, "gcloud access-context-manager perimeters update datos --set-ingress-policies=ingress.yaml")
	if ok, why := s.State.APICall("lab-proj", "bigquery.googleapis.com", bi); !ok {
		t.Fatal(why)
	}
	if ok, _ := s.State.APICall("lab-proj", "storage.googleapis.com", bi); ok {
		t.Fatal("ingress rule only covers BigQuery")
	}
	other := sim.ParseFrom("serviceAccount:x@bi.iam.gserviceaccount.com", "project:zentra-bi/418230557192")
	if ok, _ := s.State.APICall("lab-proj", "bigquery.googleapis.com", other); ok {
		t.Fatal("ingress rule only covers the BI service account")
	}
	// traffic logs violations
	s.State.Traffic = []sim.TrafficSpec{{Name: "bi", Target: "api:storage.googleapis.com", Path: "storage.objects.get", RPS: 5, Principal: bi.Principal, From: "project:zentra-bi/418230557192"}}
	s.State.Step(1)
	if out := run(t, s, "gcloud logging read 'protoPayload.metadata.violationReason=\"RESOURCES_NOT_IN_SAME_SERVICE_PERIMETER\"' --limit=1"); !strings.Contains(out, "vpcServiceControlsUniqueIdentifier") {
		t.Fatalf("violation should be logged:\n%s", out)
	}
	// deny policy beats Owner
	run(t, s, "gcloud storage buckets create gs://audit-lab --location=EU")
	s.Files["deny.json"] = `{"displayName":"no borrar","rules":[{"denyRule":{"deniedPrincipals":["principalSet://goog/public:all"],"exceptionPrincipals":["principalSet://goog/group/breakglass@zentra.dev"],"deniedPermissions":["storage.googleapis.com/objects.delete"]}}]}`
	run(t, s, "gcloud iam policies create no-borrar --attachment-point=cloudresourcemanager.googleapis.com/projects/lab-proj --kind=denypolicies --policy-file=deny.json")
	b, _ := s.State.FindBucket("audit-lab")
	if ok, why := s.State.Explain(s.Principal(), "storage.objects.delete", s.State.BucketResource(b, "x")); ok || !strings.Contains(why, "deny") {
		t.Fatalf("deny should win over owner: %v %s", ok, why)
	}
	// CMEK: disabled version breaks writes, enabling restores
	run(t, s, "gcloud kms keyrings create k --location=europe-west1")
	run(t, s, "gcloud kms keys create d --keyring=k --location=europe-west1 --purpose=encryption")
	run(t, s, "gcloud kms keys add-iam-policy-binding d --keyring=k --location=europe-west1 --member=serviceAccount:"+s.State.StorageAgent("lab-proj")+" --role=roles/cloudkms.cryptoKeyEncrypterDecrypter")
	run(t, s, "gcloud storage buckets create gs://cmek-lab --location=europe-west1 --default-encryption-key=projects/lab-proj/locations/europe-west1/keyRings/k/cryptoKeys/d")
	s.Files["a.txt"] = "x"
	run(t, s, "gcloud kms keys versions disable 1 --key=d --keyring=k --location=europe-west1")
	if r := s.Exec("gcloud storage cp a.txt gs://cmek-lab/a.txt"); r.Exit == 0 {
		t.Fatal("write with a disabled key version must fail")
	}
	run(t, s, "gcloud kms keys versions enable 1 --key=d --keyring=k --location=europe-west1")
	run(t, s, "gcloud storage cp a.txt gs://cmek-lab/a.txt")
}
