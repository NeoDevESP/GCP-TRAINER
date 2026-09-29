package sim

import (
	"fmt"
	"sort"
	"strings"
)

// Fictitious training price list (EUR). These are planning values for the
// simulator, not Google's official prices.
var machineHourly = map[string]float64{
	"e2-micro": 0.0084, "e2-small": 0.0168, "e2-medium": 0.0335, "e2-standard-2": 0.067, "e2-standard-4": 0.134,
	"e2-standard-8": 0.268, "e2-highmem-2": 0.090, "n2-standard-2": 0.097, "n2-standard-4": 0.194, "n2-standard-8": 0.388,
	"n2-standard-16": 0.777, "n2-standard-32": 1.554, "c2-standard-8": 0.417, "c2-standard-60": 3.13, "n1-standard-1": 0.0475,
	"n1-standard-4": 0.19, "a2-highgpu-1g": 3.67, "g2-standard-4": 0.71, "n1-highmem-8": 0.473,
}

var sqlHourly = map[string]float64{"db-f1-micro": 0.0105, "db-g1-small": 0.035, "db-custom-1-3840": 0.064, "db-custom-2-7680": 0.13, "db-custom-4-15360": 0.26, "db-custom-8-30720": 0.52, "db-custom-16-61440": 1.04}

var storageMonthly = map[string]float64{"STANDARD": 0.020, "NEARLINE": 0.010, "COLDLINE": 0.004, "ARCHIVE": 0.0012}

// CostLine is one item of the cost estimate.
type CostLine struct {
	Resource string  `json:"resource"`
	SKU      string  `json:"sku"`
	Monthly  float64 `json:"monthlyEur"`
}

// MachineHourly returns the hourly price of a machine type.
func MachineHourly(mt string) float64 {
	if v, ok := machineHourly[mt]; ok {
		return v
	}
	if strings.HasPrefix(mt, "custom-") {
		return 0.08
	}
	return 0.067
}

// CostEstimate computes an estimated monthly bill for a project.
func (s *State) CostEstimate(project string) ([]CostLine, float64) {
	p := s.Projects[project]
	if p == nil {
		return nil, 0
	}
	const h = 730.0
	var lines []CostLine
	add := func(res, sku string, m float64) {
		if m > 0 {
			lines = append(lines, CostLine{Resource: res, SKU: sku, Monthly: round2(m)})
		}
	}
	for _, n := range SortedKeys(p.Instances) {
		vm := p.Instances[n]
		if vm.Status == "RUNNING" {
			rate := MachineHourly(vm.MachineType)
			if vm.Spot {
				rate *= 0.3
			}
			add("instance/"+n, vm.MachineType, rate*h)
			if vm.GPUs > 0 {
				add("instance/"+n, fmt.Sprintf("%d x GPU", vm.GPUs), float64(vm.GPUs)*2.48*h)
			}
			if vm.ExternalIP != "" {
				add("instance/"+n, "external IPv4", 0.005*h)
			}
		}
	}
	for _, n := range SortedKeys(p.Disks) {
		d := p.Disks[n]
		rate := 0.04
		if strings.Contains(d.Type, "ssd") {
			rate = 0.17
		} else if strings.Contains(d.Type, "balanced") {
			rate = 0.10
		}
		add("disk/"+n, d.Type, float64(d.SizeGB)*rate)
	}
	for _, n := range SortedKeys(p.Snapshots) {
		add("snapshot/"+n, "snapshot storage", float64(p.Snapshots[n].SizeGB)*0.026)
	}
	for _, n := range SortedKeys(p.Addresses) {
		a := p.Addresses[n]
		if a.Type == "EXTERNAL" && a.User == "" {
			add("address/"+n, "unused static IP", 0.01*h)
		}
	}
	for _, n := range SortedKeys(p.Routers) {
		if len(p.Routers[n].NATs) > 0 {
			add("router/"+n, "Cloud NAT gateway", 0.045*h)
		}
	}
	for _, n := range SortedKeys(p.ForwardingRules) {
		add("forwarding-rule/"+n, "LB forwarding rule", 0.025*h)
	}
	for _, n := range SortedKeys(p.Buckets) {
		b := p.Buckets[n]
		size := 0
		for _, o := range b.Objects {
			size += o.Size
		}
		gb := float64(size) / 1e9
		if v := b.Labels["sim-size-gb"]; v != "" {
			fmt.Sscanf(v, "%g", &gb)
			if len(b.Lifecycle) > 0 {
				gb *= 0.45 // lifecycle moves or deletes old data
			}
		}
		add("bucket/"+n, b.StorageClass, gb*storageMonthly[b.StorageClass])
	}
	for _, n := range SortedKeys(p.SQLInstances) {
		in := p.SQLInstances[n]
		if in.State == "STOPPED" {
			continue
		}
		rate := sqlHourly[in.Tier]
		if rate == 0 {
			rate = 0.13
		}
		if in.Availability == "REGIONAL" {
			rate *= 2
		}
		add("sql/"+n, in.Tier+" "+in.Availability, rate*h)
	}
	for _, n := range SortedKeys(p.RunServices) {
		svc := p.RunServices[n]
		add("run/"+n, "min instances", float64(svc.MinInstances)*0.045*h)
		add("run/"+n, "requests", float64(svc.Instances)*3)
	}
	for _, n := range SortedKeys(p.Clusters) {
		c := p.Clusters[n]
		add("gke/"+n, "cluster management fee", 0.10*h)
		for _, np := range c.NodePools {
			rate := MachineHourly(np.MachineType)
			if np.Spot {
				rate *= 0.3
			}
			add("gke/"+n+"/"+np.Name, fmt.Sprintf("%d x %s", np.Count, np.MachineType), float64(np.Count)*rate*h)
		}
	}
	for _, n := range SortedKeys(p.AIEndpoints) {
		for _, d := range p.AIEndpoints[n].Deployed {
			add("vertex-endpoint/"+n, d.MachineType, float64(max(1, d.MinReplicas))*MachineHourly(d.MachineType)*1.15*h)
			if d.GPUs > 0 {
				add("vertex-endpoint/"+n, "GPU", float64(d.GPUs)*2.48*h)
			}
		}
	}
	p.EnsureServices()
	for _, n := range SortedKeys(p.Functions) {
		fn := p.Functions[n]
		add("functions/"+n, "invocations + min instances", float64(fn.Invocations)*30*0.0000004+float64(fn.MinInstances)*0.03*h)
	}
	for _, n := range SortedKeys(p.SchedulerJobs) {
		add("scheduler/"+n, "job", 0.10)
	}
	if app := p.AppEngine; app != nil {
		for _, sn := range SortedKeys(app.Services) {
			for _, vn := range SortedKeys(app.Services[sn].Versions) {
				if v := app.Services[sn].Versions[vn]; v.Status == "SERVING" && (v.Scaling == "manual" || v.Env == "flexible") {
					add("appengine/"+sn+"/"+vn, v.Env+" "+v.Scaling, 0.07*h)
				}
			}
		}
	}
	for _, n := range SortedKeys(p.Redis) {
		r := p.Redis[n]
		rate := 0.049
		if r.Tier == "STANDARD" {
			rate = 0.064
		}
		add("redis/"+n, fmt.Sprintf("%s %d GB", r.Tier, r.SizeGB), float64(r.SizeGB)*rate*h)
	}
	for _, n := range SortedKeys(p.Spanner) {
		in := p.Spanner[n]
		add("spanner/"+n, fmt.Sprintf("%d processing units", in.ProcessingUnits), float64(in.ProcessingUnits)/1000*0.90*h)
	}
	for _, j := range p.DataflowJobs {
		if j.State == "JOB_STATE_RUNNING" {
			add("dataflow/"+j.Name, fmt.Sprintf("%d streaming workers", max(1, j.Workers)), float64(max(1, j.Workers))*0.27*h)
		}
	}
	for _, n := range SortedKeys(p.DataprocClusters) {
		cl := p.DataprocClusters[n]
		if cl.State != "RUNNING" {
			continue
		}
		vms := 1 + cl.Workers
		rate := MachineHourly(cl.MasterType)
		add("dataproc/"+n, fmt.Sprintf("%d VMs + Dataproc fee", vms), (float64(vms)*(rate+0.04)+float64(cl.Preemptible)*rate*0.3)*h)
	}
	for _, n := range SortedKeys(p.Composer) {
		add("composer/"+n, "environment "+p.Composer[n].Size, 0.52*h)
	}
	for _, n := range SortedKeys(p.Filestore) {
		f := p.Filestore[n]
		rate := 0.16
		if f.Tier != "BASIC_HDD" {
			rate = 0.30
		}
		add("filestore/"+n, fmt.Sprintf("%s %d GB", f.Tier, f.CapacityGB), float64(f.CapacityGB)*rate)
	}
	var qbytes int64
	for _, j := range p.BQJobs {
		if !j.DryRun {
			qbytes += j.BytesProcessed
		}
	}
	// assume the recorded query pattern runs 30 times per day
	add("bigquery", "on-demand analysis", float64(qbytes)/1e12*6.25*30*30)
	for _, n := range SortedKeys(p.InstanceGroups) {
		_ = n
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].Monthly > lines[j].Monthly })
	total := 0.0
	for _, l := range lines {
		total += l.Monthly
	}
	return lines, round2(total)
}

func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }
