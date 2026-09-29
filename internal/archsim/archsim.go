// Package archsim is the Architecture Simulator (Blueprint §12): the learner
// writes a design (components, regions, flows) and the engine evaluates it
// against requirements — availability, RPO/RTO under a regional failure,
// capacity at peak, latency per user region, data residency, security and
// monthly cost — explaining every number so trade-offs can be discussed.
//
// The model is intentionally simple and transparent: published SLA figures
// per component and topology, series composition along the request path, and
// list prices rounded for teaching. It teaches reasoning, not exact billing.
package archsim

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Component is one building block of a design.
type Component struct {
	Name         string   `yaml:"name" json:"name"`
	Type         string   `yaml:"type" json:"type"` // global-lb, cdn, cloud-run, mig, gke, cloud-sql, spanner, firestore, memorystore, pubsub, gcs, bigquery
	Regions      []string `yaml:"regions" json:"regions"`
	Zones        int      `yaml:"zones" json:"zones"` // MIG / GKE: zones per region (1 = zonal)
	MinInstances int      `yaml:"minInstances" json:"minInstances"`
	MaxInstances int      `yaml:"maxInstances" json:"maxInstances"`
	MachineType  string   `yaml:"machineType" json:"machineType"`
	HA           bool     `yaml:"ha" json:"ha"`                     // Cloud SQL regional HA
	Replicas     []string `yaml:"replicas" json:"replicas"`         // Cloud SQL cross-region read replicas
	Backups      bool     `yaml:"backups" json:"backups"`           // automated backups
	PITR         bool     `yaml:"pitr" json:"pitr"`                 // point-in-time recovery
	MultiRegion  bool     `yaml:"multiRegion" json:"multiRegion"`   // Spanner/Firestore/GCS multi-region config
	PublicIP     bool     `yaml:"publicIp" json:"publicIp"`         // databases/VMs exposed with public IPs
	Private      bool     `yaml:"private" json:"private"`           // private connectivity only
	CMEK         bool     `yaml:"cmek" json:"cmek"`                 // customer-managed encryption keys
	Armor        bool     `yaml:"armor" json:"armor"`               // Cloud Armor on the load balancer
	Tier         string   `yaml:"tier" json:"tier"`                 // SQL tier / memorystore size
	Nodes        int      `yaml:"nodes" json:"nodes"`               // Spanner nodes, GKE nodes per zone
	RPSPerUnit   int      `yaml:"rpsPerInstance" json:"rpsPerUnit"` // optional override
}

// Design is what the learner submits (design.yaml).
type Design struct {
	Name       string      `yaml:"name" json:"name"`
	Components []Component `yaml:"components" json:"components"`
	// Path is the synchronous request path from the user, e.g. [lb, web, db].
	Path []string `yaml:"path" json:"path"`
}

// Requirements are the non-functional requirements of the case study.
type Requirements struct {
	Availability float64  `yaml:"availability" json:"availability"` // e.g. 99.95
	RPOMinutes   float64  `yaml:"rpoMinutes" json:"rpoMinutes"`     // tolerated data loss on regional failure
	RTOMinutes   float64  `yaml:"rtoMinutes" json:"rtoMinutes"`     // tolerated downtime on regional failure
	PeakRPS      int      `yaml:"peakRps" json:"peakRps"`
	UserRegions  []string `yaml:"userRegions" json:"userRegions"` // europe, us, asia
	LatencyMs    int      `yaml:"latencyMs" json:"latencyMs"`     // p95 target for every user region
	BudgetEur    float64  `yaml:"budgetEur" json:"budgetEur"`     // monthly
	Residency    string   `yaml:"residency" json:"residency"`     // "eu" → all data stores in EU
	PrivateData  bool     `yaml:"privateData" json:"privateData"` // no public IPs on data stores
	CMEK         bool     `yaml:"cmek" json:"cmek"`
	WAF          bool     `yaml:"waf" json:"waf"`
}

// Finding is one requirement verdict.
type Finding struct {
	Requirement string `json:"requirement"`
	Target      string `json:"target"`
	Achieved    string `json:"achieved"`
	Pass        bool   `json:"pass"`
	Why         string `json:"why"`
}

// Report is the evaluation of a design.
type Report struct {
	Availability float64            `json:"availability"`
	RPOMinutes   float64            `json:"rpoMinutes"`
	RTOMinutes   float64            `json:"rtoMinutes"`
	CapacityRPS  int                `json:"capacityRps"`
	LatencyMs    map[string]int     `json:"latencyMs"`
	CostEur      float64            `json:"monthlyCostEur"`
	CostLines    map[string]float64 `json:"costLines"`
	Findings     []Finding          `json:"findings"`
	Passed       int                `json:"passed"`
	Total        int                `json:"total"`
	Problems     []string           `json:"problems,omitempty"` // invalid design
}

// Parse reads a YAML design.
func Parse(b []byte) (*Design, error) {
	var d Design
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("design.yaml: %w", err)
	}
	return &d, nil
}

func continent(region string) string {
	switch {
	case strings.HasPrefix(region, "europe"):
		return "europe"
	case strings.HasPrefix(region, "us"), strings.HasPrefix(region, "northamerica"), strings.HasPrefix(region, "southamerica"):
		return "us"
	case strings.HasPrefix(region, "asia"), strings.HasPrefix(region, "australia"):
		return "asia"
	}
	return region
}

// componentAvailability returns the availability (%) of a component and why.
func componentAvailability(c Component) (float64, string) {
	regions := len(c.Regions)
	switch c.Type {
	case "global-lb":
		return 99.99, "global external load balancer SLA 99.99%"
	case "cdn":
		return 99.99, "Cloud CDN at the edge"
	case "cloud-run":
		if regions >= 2 {
			return 99.99, "Cloud Run in 2+ regions behind a global LB (regional failure tolerated)"
		}
		return 99.95, "Cloud Run single region (99.95%)"
	case "mig", "gke":
		switch {
		case regions >= 2:
			return 99.99, "regional " + c.Type + " in 2+ regions"
		case c.Zones >= 2:
			return 99.95, c.Type + " spread over several zones of one region"
		default:
			return 99.5, "zonal " + c.Type + " (single zone: a zone failure takes it down)"
		}
	case "cloud-sql":
		if c.HA {
			return 99.95, "Cloud SQL regional HA (standby in another zone)"
		}
		return 99.5, "Cloud SQL zonal instance (no standby)"
	case "spanner":
		if c.MultiRegion {
			return 99.999, "Spanner multi-region configuration"
		}
		return 99.99, "Spanner regional configuration"
	case "firestore":
		if c.MultiRegion {
			return 99.999, "Firestore multi-region"
		}
		return 99.99, "Firestore regional"
	case "memorystore":
		if c.HA {
			return 99.9, "Memorystore Standard tier (replica)"
		}
		return 99.5, "Memorystore Basic tier (no replica)"
	case "pubsub", "gcs", "bigquery":
		return 99.95, c.Type + " regional/multi-regional managed service"
	}
	return 99.0, "unknown component type (assumed 99%)"
}

// capacity in requests per second per instance/unit.
func unitRPS(c Component) int {
	if c.RPSPerUnit > 0 {
		return c.RPSPerUnit
	}
	switch c.Type {
	case "cloud-run":
		return 80
	case "mig":
		switch {
		case strings.Contains(c.MachineType, "standard-8"):
			return 800
		case strings.Contains(c.MachineType, "standard-4"):
			return 400
		case strings.Contains(c.MachineType, "standard-2"), c.MachineType == "":
			return 200
		default:
			return 100
		}
	case "gke":
		return 300
	}
	return 0
}

var (
	hourlyVM = map[string]float64{"e2-small": 0.018, "e2-medium": 0.036, "e2-standard-2": 0.072, "e2-standard-4": 0.144, "e2-standard-8": 0.288, "n2-standard-2": 0.105, "n2-standard-4": 0.21, "n2-standard-8": 0.42}
	sqlTier  = map[string]float64{"db-f1-micro": 9, "db-g1-small": 27, "db-custom-2-7680": 120, "db-custom-4-15360": 240, "db-custom-8-30720": 480}
)

func componentCost(c Component) (float64, string) {
	regions := max(1, len(c.Regions))
	switch c.Type {
	case "global-lb":
		if c.Armor {
			return 18 + 5, "forwarding rules + Cloud Armor policy"
		}
		return 18, "forwarding rules and data processing"
	case "cdn":
		return 40, "cache egress (estimate)"
	case "cloud-run":
		minI := max(0, c.MinInstances)
		return float64(regions) * (15 + float64(minI)*40), fmt.Sprintf("%d region(s), %d always-on instance(s) each", regions, minI)
	case "mig", "gke":
		h := hourlyVM[c.MachineType]
		if h == 0 {
			h = 0.072
		}
		n := max(1, c.MinInstances)
		cost := float64(regions*n) * h * 730
		if c.Type == "gke" {
			cost += 73 * float64(regions)
		}
		return cost, fmt.Sprintf("%d × %s × %d region(s)", n, firstNonEmpty(c.MachineType, "e2-standard-2"), regions)
	case "cloud-sql":
		base := sqlTier[c.Tier]
		if base == 0 {
			base = 120
		}
		cost := base
		why := firstNonEmpty(c.Tier, "db-custom-2-7680")
		if c.HA {
			cost += base
			why += " + HA standby"
		}
		cost += base * float64(len(c.Replicas))
		if len(c.Replicas) > 0 {
			why += fmt.Sprintf(" + %d cross-region replica(s)", len(c.Replicas))
		}
		if c.Backups {
			cost += 10
		}
		return cost, why
	case "spanner":
		n := max(1, c.Nodes)
		per := 650.0
		if c.MultiRegion {
			per = 2900
		}
		return float64(n) * per, fmt.Sprintf("%d node(s) %s", n, map[bool]string{true: "multi-region", false: "regional"}[c.MultiRegion])
	case "firestore":
		return 60, "document reads/writes (estimate)"
	case "memorystore":
		if c.HA {
			return 110, "Standard tier 5 GB"
		}
		return 55, "Basic tier 5 GB"
	case "pubsub":
		return 20, "throughput (estimate)"
	case "gcs":
		return 25, "storage (estimate)"
	case "bigquery":
		return 50, "storage + queries (estimate)"
	}
	return 0, ""
}

var dataStores = map[string]bool{"cloud-sql": true, "spanner": true, "firestore": true, "memorystore": true, "gcs": true, "bigquery": true}

// Evaluate scores a design against requirements.
func Evaluate(d *Design, req Requirements) Report {
	r := Report{LatencyMs: map[string]int{}, CostLines: map[string]float64{}}
	byName := map[string]Component{}
	for _, c := range d.Components {
		if c.Name == "" {
			r.Problems = append(r.Problems, "component without name")
			continue
		}
		byName[c.Name] = c
	}
	if len(d.Path) == 0 {
		r.Problems = append(r.Problems, "design has no request path (path: [lb, app, db])")
	}
	// availability: series composition along the synchronous path
	avail := 1.0
	var availWhy []string
	for _, n := range d.Path {
		c, ok := byName[n]
		if !ok {
			r.Problems = append(r.Problems, "path references unknown component "+n)
			continue
		}
		a, why := componentAvailability(c)
		avail *= a / 100
		availWhy = append(availWhy, fmt.Sprintf("%s %.3f%% (%s)", n, a, why))
	}
	r.Availability = math.Floor(avail*100000) / 1000
	// regional failure: RPO/RTO driven by the stateful store on the path
	r.RPOMinutes, r.RTOMinutes = 0, 0
	var drWhy []string
	computeMulti := true
	for _, n := range d.Path {
		c := byName[n]
		switch c.Type {
		case "cloud-run", "mig", "gke":
			if len(c.Regions) < 2 {
				computeMulti = false
				drWhy = append(drWhy, n+" runs in a single region: redeploying elsewhere takes ~60 min")
				r.RTOMinutes = math.Max(r.RTOMinutes, 60)
			}
		case "cloud-sql":
			switch {
			case len(c.Replicas) > 0:
				r.RPOMinutes = math.Max(r.RPOMinutes, 1)
				r.RTOMinutes = math.Max(r.RTOMinutes, 15)
				drWhy = append(drWhy, n+": promote the cross-region replica (async: seconds of data loss, ~15 min manual failover)")
			case c.PITR && c.Backups:
				r.RPOMinutes = math.Max(r.RPOMinutes, 60)
				r.RTOMinutes = math.Max(r.RTOMinutes, 120)
				drWhy = append(drWhy, n+": restore backups/PITR into another region (~1 h of log shipping lag, ~2 h restore)")
			case c.Backups:
				r.RPOMinutes = math.Max(r.RPOMinutes, 1440)
				r.RTOMinutes = math.Max(r.RTOMinutes, 180)
				drWhy = append(drWhy, n+": only daily backups (up to 24 h of data lost, ~3 h restore)")
			default:
				r.RPOMinutes = math.Max(r.RPOMinutes, 1e6)
				r.RTOMinutes = math.Max(r.RTOMinutes, 1e6)
				drWhy = append(drWhy, n+": no backups — a regional disaster loses the data")
			}
		case "spanner", "firestore":
			if !c.MultiRegion {
				r.RPOMinutes = math.Max(r.RPOMinutes, 60)
				r.RTOMinutes = math.Max(r.RTOMinutes, 120)
				drWhy = append(drWhy, n+": regional configuration — restore from backup in another region")
			} else {
				drWhy = append(drWhy, n+": multi-region, synchronous replication (RPO 0, automatic failover)")
			}
		}
	}
	_ = computeMulti
	// capacity: minimum over compute components on the path
	cap := -1
	var capWhy string
	for _, n := range d.Path {
		c := byName[n]
		u := unitRPS(c)
		if u == 0 {
			continue
		}
		maxI := c.MaxInstances
		if maxI == 0 {
			maxI = max(1, c.MinInstances)
		}
		total := u * maxI * max(1, len(c.Regions))
		if cap < 0 || total < cap {
			cap = total
			capWhy = fmt.Sprintf("%s: %d rps/instance × max %d × %d region(s)", n, u, maxI, max(1, len(c.Regions)))
		}
	}
	if cap < 0 {
		cap = 0
	}
	r.CapacityRPS = cap
	// latency per user region
	hasCDN := false
	serving := map[string]bool{}
	for _, c := range d.Components {
		if c.Type == "cdn" {
			hasCDN = true
		}
	}
	for _, n := range d.Path {
		c := byName[n]
		if c.Type == "cloud-run" || c.Type == "mig" || c.Type == "gke" {
			for _, rg := range c.Regions {
				serving[continent(rg)] = true
			}
		}
	}
	for _, ur := range req.UserRegions {
		lat := 40
		if !serving[ur] {
			lat = 160
			if ur == "asia" {
				lat = 230
			}
		}
		if hasCDN {
			lat -= 20
		}
		// a single-region database behind multi-region compute adds a cross-region hop
		for _, n := range d.Path {
			c := byName[n]
			if (c.Type == "cloud-sql" || c.Type == "spanner" || c.Type == "firestore") && serving[ur] {
				home := ""
				if len(c.Regions) > 0 {
					home = continent(c.Regions[0])
				}
				if home != "" && home != ur {
					lat += 110
				}
			}
		}
		r.LatencyMs[ur] = lat
	}
	// cost
	for _, c := range d.Components {
		v, _ := componentCost(c)
		r.CostLines[c.Name] = math.Round(v*100) / 100
		r.CostEur += v
	}
	r.CostEur = math.Round(r.CostEur*100) / 100

	add := func(name, target, achieved string, pass bool, why string) {
		r.Findings = append(r.Findings, Finding{Requirement: name, Target: target, Achieved: achieved, Pass: pass, Why: why})
		r.Total++
		if pass {
			r.Passed++
		}
	}
	if req.Availability > 0 {
		add("availability", fmt.Sprintf("≥ %.3f%%", req.Availability), fmt.Sprintf("%.3f%%", r.Availability), r.Availability >= req.Availability, "series composition: "+strings.Join(availWhy, " × "))
	}
	if req.RPOMinutes > 0 || req.RTOMinutes > 0 {
		sort.Strings(drWhy)
		why := strings.Join(drWhy, "; ")
		if why == "" {
			why = "no stateful component on the request path"
		}
		if req.RPOMinutes > 0 {
			add("RPO (regional failure)", fmt.Sprintf("≤ %.0f min", req.RPOMinutes), fmtMin(r.RPOMinutes), r.RPOMinutes <= req.RPOMinutes, why)
		}
		if req.RTOMinutes > 0 {
			add("RTO (regional failure)", fmt.Sprintf("≤ %.0f min", req.RTOMinutes), fmtMin(r.RTOMinutes), r.RTOMinutes <= req.RTOMinutes, why)
		}
	}
	if req.PeakRPS > 0 {
		add("peak capacity", fmt.Sprintf("≥ %d rps", req.PeakRPS), fmt.Sprintf("%d rps", r.CapacityRPS), r.CapacityRPS >= req.PeakRPS, capWhy)
	}
	if req.LatencyMs > 0 {
		for _, ur := range req.UserRegions {
			l := r.LatencyMs[ur]
			why := "served from the same continent"
			if !serving[ur] {
				why = "no serving region on this continent (cross-ocean round trip)"
			}
			add("latency "+ur, fmt.Sprintf("≤ %d ms p95", req.LatencyMs), fmt.Sprintf("%d ms", l), l <= req.LatencyMs, why)
		}
	}
	if req.Residency == "eu" {
		ok := true
		var bad []string
		for _, c := range d.Components {
			if !dataStores[c.Type] {
				continue
			}
			for _, rg := range append(append([]string{}, c.Regions...), c.Replicas...) {
				if continent(rg) != "europe" {
					ok = false
					bad = append(bad, c.Name+"@"+rg)
				}
			}
		}
		why := "all data stores in EU regions"
		if !ok {
			why = "data stored outside the EU: " + strings.Join(bad, ", ")
		}
		add("data residency (EU)", "EU only", map[bool]string{true: "EU", false: "violated"}[ok], ok, why)
	}
	if req.PrivateData {
		ok := true
		var bad []string
		for _, c := range d.Components {
			if dataStores[c.Type] && c.PublicIP {
				ok = false
				bad = append(bad, c.Name)
			}
		}
		add("private data stores", "no public IPs", map[bool]string{true: "private", false: "public: " + strings.Join(bad, ", ")}[ok], ok, "databases reachable only through private networking")
	}
	if req.CMEK {
		ok := true
		for _, c := range d.Components {
			if dataStores[c.Type] && !c.CMEK {
				ok = false
			}
		}
		add("customer-managed keys", "CMEK on data stores", map[bool]string{true: "yes", false: "missing"}[ok], ok, "compliance requires keys the company controls")
	}
	if req.WAF {
		ok := false
		for _, c := range d.Components {
			if c.Type == "global-lb" && c.Armor {
				ok = true
			}
		}
		add("web application firewall", "Cloud Armor", map[bool]string{true: "yes", false: "missing"}[ok], ok, "OWASP rules and rate limiting at the edge")
	}
	if req.BudgetEur > 0 {
		add("monthly budget", fmt.Sprintf("≤ %.0f EUR", req.BudgetEur), fmt.Sprintf("%.0f EUR", r.CostEur), r.CostEur <= req.BudgetEur, costWhy(r.CostLines))
	}
	return r
}

func fmtMin(m float64) string {
	if m >= 1e6 {
		return "data lost / unbounded"
	}
	if m == 0 {
		return "0 min"
	}
	return fmt.Sprintf("%.0f min", m)
}

func costWhy(lines map[string]float64) string {
	type kv struct {
		k string
		v float64
	}
	var l []kv
	for k, v := range lines {
		l = append(l, kv{k, v})
	}
	sort.Slice(l, func(i, j int) bool { return l[i].v > l[j].v })
	var parts []string
	for i, x := range l {
		if i >= 3 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s %.0f", x.k, x.v))
	}
	return "largest items: " + strings.Join(parts, ", ")
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// Render formats a report for the terminal.
func (r Report) Render() string {
	var b strings.Builder
	if len(r.Problems) > 0 {
		b.WriteString("Design problems:\n")
		for _, p := range r.Problems {
			b.WriteString("  ✗ " + p + "\n")
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Requirement                  Target            Achieved\n")
	for _, f := range r.Findings {
		mark := "✓"
		if !f.Pass {
			mark = "✗"
		}
		fmt.Fprintf(&b, "%s %-26s %-17s %s\n    %s\n", mark, f.Requirement, f.Target, f.Achieved, f.Why)
	}
	fmt.Fprintf(&b, "\n%d/%d requirements met · availability %.3f%% · %d rps · %.0f EUR/month\n", r.Passed, r.Total, r.Availability, r.CapacityRPS, r.CostEur)
	return b.String()
}
