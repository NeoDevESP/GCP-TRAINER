package sim

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// FinOps simulator (Blueprint §12): a simulated invoice grouped by service,
// resource or label, compared with the previous month to surface anomalies,
// and a recommender for idle and over-provisioned resources.
//
// Utilisation signals come from the simulation (listeners, traffic) and from
// the "sim-cpu" label that scenarios use to describe a workload's measured
// CPU (idle, low, normal, high).

// ServiceOf maps a cost line to a billing service.
func ServiceOf(resource string) string {
	kind := strings.SplitN(resource, "/", 2)[0]
	switch kind {
	case "instance", "disk", "snapshot":
		return "Compute Engine"
	case "address", "router", "forwarding-rule":
		return "Networking"
	case "bucket":
		return "Cloud Storage"
	case "sql":
		return "Cloud SQL"
	case "run":
		return "Cloud Run"
	case "gke":
		return "Kubernetes Engine"
	case "vertex-endpoint":
		return "Vertex AI"
	case "bigquery":
		return "BigQuery"
	}
	return "Other"
}

// BillLine is one row of a grouped invoice.
type BillLine struct {
	Key      string  `json:"key"`
	Current  float64 `json:"currentEur"`
	Previous float64 `json:"previousEur"`
	Delta    float64 `json:"deltaEur"`
	Anomaly  bool    `json:"anomaly"`
}

// SnapshotBill stores the current estimate as "previous month" per service.
func (s *State) SnapshotBill(project string) {
	lines, _ := s.CostEstimate(project)
	prev := map[string]float64{}
	for _, l := range lines {
		prev[ServiceOf(l.Resource)] += l.Monthly
	}
	b, _ := json.Marshal(prev)
	s.Extra["bill:prev:"+project] = string(b)
}

func (s *State) labelOf(project, resource, key string) string {
	p := s.Projects[project]
	if p == nil {
		return ""
	}
	kind, name, _ := strings.Cut(resource, "/")
	name = strings.SplitN(name, "/", 2)[0]
	switch kind {
	case "instance":
		if vm := p.Instances[name]; vm != nil {
			return vm.Labels[key]
		}
	case "bucket":
		if b := p.Buckets[name]; b != nil {
			return b.Labels[key]
		}
	case "run":
		if svc := p.RunServices[name]; svc != nil {
			return svc.Labels[key]
		}
	}
	return ""
}

// BillReport groups the current estimate by "service", "resource" or
// "label:KEY" and compares services with the previous month.
func (s *State) BillReport(project, groupBy string) ([]BillLine, float64, float64) {
	lines, total := s.CostEstimate(project)
	prev := map[string]float64{}
	_ = json.Unmarshal([]byte(s.Extra["bill:prev:"+project]), &prev)
	prevTotal := 0.0
	for _, v := range prev {
		prevTotal += v
	}
	agg := map[string]float64{}
	for _, l := range lines {
		key := ServiceOf(l.Resource)
		switch {
		case groupBy == "resource":
			key = l.Resource
		case strings.HasPrefix(groupBy, "label:"):
			key = s.labelOf(project, l.Resource, strings.TrimPrefix(groupBy, "label:"))
			if key == "" {
				key = "(unlabelled)"
			}
		}
		agg[key] += l.Monthly
	}
	var out []BillLine
	for k, v := range agg {
		bl := BillLine{Key: k, Current: round2(v)}
		if groupBy == "" || groupBy == "service" {
			bl.Previous = round2(prev[k])
			bl.Delta = round2(v - prev[k])
			bl.Anomaly = bl.Delta > 50 && (prev[k] == 0 || v > prev[k]*1.5)
		}
		out = append(out, bl)
	}
	if groupBy == "" || groupBy == "service" {
		for k, v := range prev {
			if _, ok := agg[k]; !ok {
				out = append(out, BillLine{Key: k, Previous: round2(v), Delta: -round2(v)})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Current > out[j].Current || (out[i].Current == out[j].Current && out[i].Key < out[j].Key)
	})
	return out, total, round2(prevTotal)
}

// Recommendation is a recommender result.
type Recommendation struct {
	ID          string  `json:"name"`
	Recommender string  `json:"recommenderSubtype"`
	Resource    string  `json:"resource"`
	Description string  `json:"description"`
	SavingsEur  float64 `json:"savingsEurMonthly"`
	Action      string  `json:"suggestedAction"`
}

var downsize = map[string]string{"n2-standard-32": "n2-standard-8", "n2-standard-16": "n2-standard-4", "n2-standard-8": "n2-standard-2", "e2-standard-8": "e2-standard-2", "e2-standard-4": "e2-standard-2", "n2-standard-4": "n2-standard-2", "c2-standard-60": "c2-standard-8"}

// Recommendations derives recommender output for a project.
func (s *State) Recommendations(project, recommender string) []Recommendation {
	p := s.Projects[project]
	if p == nil {
		return nil
	}
	const h = 730.0
	var out []Recommendation
	add := func(r Recommendation) {
		if recommender == "" || strings.Contains(recommender, r.Recommender) {
			r.ID = fmt.Sprintf("projects/%s/locations/global/recommenders/%s/recommendations/%08x", project, r.Recommender, len(out)+1)
			out = append(out, r)
		}
	}
	for _, n := range SortedKeys(p.Instances) {
		vm := p.Instances[n]
		if vm.Status != "RUNNING" {
			continue
		}
		cpu := vm.Labels["sim-cpu"]
		ls, _ := s.VMListeners(project, vm)
		idle := cpu == "idle" || (cpu == "" && len(ls) == 0 && vm.GPUs > 0)
		cost := MachineHourly(vm.MachineType)*h + float64(vm.GPUs)*2.48*h
		switch {
		case idle:
			add(Recommendation{Recommender: "google.compute.instance.IdleResourceRecommender", Resource: "instance/" + n, SavingsEur: round2(cost),
				Description: fmt.Sprintf("VM %s (%s%s) has had <2%% CPU for 14 days", n, vm.MachineType, gpuNote(vm.GPUs)), Action: "stop or delete the instance (snapshot the disk first if the data is needed)"})
		case cpu == "low":
			if to, ok := downsize[vm.MachineType]; ok {
				add(Recommendation{Recommender: "google.compute.instance.MachineTypeRecommender", Resource: "instance/" + n, SavingsEur: round2((MachineHourly(vm.MachineType) - MachineHourly(to)) * h),
					Description: fmt.Sprintf("VM %s averages 6%% CPU; %s fits the observed peak", n, to), Action: "change machine type to " + to})
			}
		}
	}
	for _, n := range SortedKeys(p.Disks) {
		d := p.Disks[n]
		if len(d.Users) == 0 && p.Instances[n] == nil {
			rate := 0.04
			if strings.Contains(d.Type, "ssd") {
				rate = 0.17
			} else if strings.Contains(d.Type, "balanced") {
				rate = 0.10
			}
			add(Recommendation{Recommender: "google.compute.disk.IdleResourceRecommender", Resource: "disk/" + n, SavingsEur: round2(float64(d.SizeGB) * rate),
				Description: fmt.Sprintf("Disk %s (%d GB %s) is not attached to any VM", n, d.SizeGB, d.Type), Action: "snapshot and delete the disk"})
		}
	}
	for _, n := range SortedKeys(p.Addresses) {
		a := p.Addresses[n]
		if a.Type == "EXTERNAL" && a.User == "" {
			add(Recommendation{Recommender: "google.compute.address.IdleResourceRecommender", Resource: "address/" + n, SavingsEur: round2(0.01 * h),
				Description: "Static external IP " + n + " is reserved but unused", Action: "release the address"})
		}
	}
	for _, n := range SortedKeys(p.SQLInstances) {
		in := p.SQLInstances[n]
		if in.Flags["sim-cpu"] == "low" {
			if to, ok := map[string]string{"db-custom-8-30720": "db-custom-2-7680", "db-custom-16-61440": "db-custom-4-15360", "db-custom-4-15360": "db-custom-2-7680"}[in.Tier]; ok {
				mult := 1.0
				if in.Availability == "REGIONAL" {
					mult = 2
				}
				add(Recommendation{Recommender: "google.cloudsql.instance.OverprovisionedRecommender", Resource: "sql/" + n, SavingsEur: round2((sqlHourly[in.Tier] - sqlHourly[to]) * h * mult),
					Description: fmt.Sprintf("Cloud SQL %s uses 8%% CPU and 20%% memory", n), Action: "change tier to " + to})
			}
		}
	}
	return out
}

func gpuNote(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(" + %d GPU", n)
}
