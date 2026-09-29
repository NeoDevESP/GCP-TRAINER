package archsim

import "testing"

// EU residency with US users: data stays in Europe, so US latency targets must
// allow one transatlantic round trip (a real trade-off the simulator exposes).
var req = Requirements{Availability: 99.95, RPOMinutes: 5, RTOMinutes: 30, PeakRPS: 2000, UserRegions: []string{"europe", "us"}, LatencyMs: 200, BudgetEur: 3500, Residency: "eu", PrivateData: true, WAF: true}

func TestNaiveDesignFailsWithReasons(t *testing.T) {
	d := &Design{Components: []Component{
		{Name: "lb", Type: "global-lb"},
		{Name: "web", Type: "mig", Regions: []string{"europe-west1"}, Zones: 1, MinInstances: 2, MaxInstances: 4, MachineType: "e2-standard-2"},
		{Name: "db", Type: "cloud-sql", Regions: []string{"europe-west1"}, Tier: "db-custom-2-7680", PublicIP: true},
	}, Path: []string{"lb", "web", "db"}}
	r := Evaluate(d, req)
	if r.Passed == r.Total {
		t.Fatalf("naive design should not meet the requirements: %+v", r.Findings)
	}
	failed := map[string]bool{}
	for _, f := range r.Findings {
		if !f.Pass {
			failed[f.Requirement] = true
			if f.Why == "" {
				t.Errorf("finding %s has no explanation", f.Requirement)
			}
		}
	}
	for _, want := range []string{"availability", "RPO (regional failure)", "peak capacity", "private data stores", "web application firewall"} {
		if !failed[want] {
			t.Errorf("expected %q to fail", want)
		}
	}
}

func TestGoodDesignMeetsRequirements(t *testing.T) {
	d := &Design{Components: []Component{
		{Name: "lb", Type: "global-lb", Armor: true},
		{Name: "cdn", Type: "cdn"},
		{Name: "web", Type: "cloud-run", Regions: []string{"europe-west1", "us-central1"}, MinInstances: 1, MaxInstances: 20},
		{Name: "db", Type: "spanner", Regions: []string{"eur3"}, MultiRegion: true, Nodes: 1, Private: true},
	}, Path: []string{"lb", "web", "db"}}
	d.Components[3].Regions = []string{"europe-west1", "europe-west4"}
	r := Evaluate(d, req)
	if r.Passed != r.Total {
		t.Fatalf("good design should pass everything, got %s", r.Render())
	}
}
