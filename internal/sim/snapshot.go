package sim

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Clone deep-copies the state (used for grading without side effects).
func (s *State) Clone() *State {
	b, _ := json.Marshal(s)
	var c State
	_ = json.Unmarshal(b, &c)
	c.Seed = s.Seed
	c.Rehydrate()
	for k, v := range s.ipSeq {
		c.ipSeq[k] = v
	}
	return &c
}

// Marshal serialises the state.
func (s *State) Marshal() ([]byte, error) { return json.Marshal(s) }

// Unmarshal restores a state.
func Unmarshal(b []byte) (*State, error) {
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	s.Rehydrate()
	return &s, nil
}

// ProjectView returns a generic JSON representation of a project, used by the
// grader's resource assertions and by the Console view.
func (s *State) ProjectView(project string) map[string]any {
	p := s.Projects[project]
	if p == nil {
		return nil
	}
	b, _ := json.Marshal(p)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	// computed views
	var pods []any
	for _, cn := range SortedKeys(p.Clusters) {
		c := p.Clusters[cn]
		for ns, ps := range s.ComputePods(project, c) {
			for _, pd := range ps {
				pb, _ := json.Marshal(pd)
				var pm map[string]any
				_ = json.Unmarshal(pb, &pm)
				pm["cluster"] = cn
				pm["namespace"] = ns
				pods = append(pods, pm)
			}
		}
	}
	m["pods"] = pods
	var health []any
	for _, bn := range SortedKeys(p.BackendServices) {
		for _, h := range s.BackendHealth(project, p.BackendServices[bn]) {
			health = append(health, map[string]any{"backendService": bn, "instance": h.Instance, "healthy": h.Healthy, "reason": h.Reason})
		}
	}
	m["backendHealth"] = health
	var findings []any
	for _, f := range s.Findings(project) {
		findings = append(findings, map[string]any{"category": f.Category, "severity": f.Severity, "resource": f.Resource})
	}
	m["findings"] = findings
	lines, total := s.CostEstimate(project)
	m["cost"] = map[string]any{"monthlyEur": total, "lines": lines}
	return m
}

// TopoNode and TopoEdge describe the architecture diagram.
type TopoNode struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Kind   string `json:"kind"`
	Group  string `json:"group"`
	Status string `json:"status"`
}

type TopoEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
	OK    bool   `json:"ok"`
}

// Topology builds a graph of the project's main resources.
func (s *State) Topology(project string) ([]TopoNode, []TopoEdge) {
	p := s.Projects[project]
	if p == nil {
		return nil, nil
	}
	var nodes []TopoNode
	var edges []TopoEdge
	for _, n := range SortedKeys(p.Networks) {
		nodes = append(nodes, TopoNode{ID: "net:" + n, Label: "VPC " + n, Kind: "network"})
	}
	for _, n := range SortedKeys(p.Subnets) {
		sn := p.Subnets[n]
		nodes = append(nodes, TopoNode{ID: "subnet:" + n, Label: fmt.Sprintf("%s\\n%s %s", n, sn.Region, sn.Range), Kind: "subnet", Group: "net:" + sn.Network})
	}
	for _, n := range SortedKeys(p.Instances) {
		vm := p.Instances[n]
		st := "ok"
		if vm.Status != "RUNNING" {
			st = "down"
		}
		nodes = append(nodes, TopoNode{ID: "vm:" + n, Label: fmt.Sprintf("VM %s\\n%s", n, vm.InternalIP), Kind: "vm", Group: "subnet:" + vm.Subnet, Status: st})
	}
	for _, n := range SortedKeys(p.SQLInstances) {
		in := p.SQLInstances[n]
		nodes = append(nodes, TopoNode{ID: "sql:" + n, Label: "Cloud SQL " + n, Kind: "sql", Status: strings.ToLower(in.State)})
	}
	for _, n := range SortedKeys(p.RunServices) {
		nodes = append(nodes, TopoNode{ID: "run:" + n, Label: "Cloud Run " + n, Kind: "run"})
		svc := p.RunServices[n]
		if h := svc.Env["DB_HOST"]; h != "" {
			for sn, in := range p.SQLInstances {
				if h == in.PrivateIP || h == in.PublicIP || strings.HasSuffix(h, ":"+sn) {
					edges = append(edges, TopoEdge{From: "run:" + n, To: "sql:" + sn, Label: "5432"})
				}
			}
			if ep, ok := s.FindIP(h); ok && ep.Kind == "vm" {
				edges = append(edges, TopoEdge{From: "run:" + n, To: "vm:" + ep.Name, Label: "5432"})
			}
		}
		if t := svc.Env["TOPIC"]; t != "" {
			edges = append(edges, TopoEdge{From: "run:" + n, To: "topic:" + t, Label: "publish"})
		}
		if b := svc.Env["BUCKET"]; b != "" {
			edges = append(edges, TopoEdge{From: "run:" + n, To: "bucket:" + strings.TrimPrefix(b, "gs://"), Label: "read"})
		}
	}
	for _, n := range SortedKeys(p.Buckets) {
		nodes = append(nodes, TopoNode{ID: "bucket:" + n, Label: "gs://" + n, Kind: "bucket"})
	}
	for _, n := range SortedKeys(p.Topics) {
		nodes = append(nodes, TopoNode{ID: "topic:" + n, Label: "Topic " + n, Kind: "topic"})
	}
	for _, n := range SortedKeys(p.Subs) {
		sub := p.Subs[n]
		nodes = append(nodes, TopoNode{ID: "sub:" + n, Label: "Sub " + n, Kind: "subscription"})
		edges = append(edges, TopoEdge{From: "topic:" + sub.Topic, To: "sub:" + n, OK: true})
	}
	for _, n := range SortedKeys(p.ForwardingRules) {
		fr := p.ForwardingRules[n]
		nodes = append(nodes, TopoNode{ID: "lb:" + n, Label: "LB " + n + "\\n" + fr.IP, Kind: "lb"})
		for _, bn := range SortedKeys(p.BackendServices) {
			for _, h := range s.BackendHealth(project, p.BackendServices[bn]) {
				edges = append(edges, TopoEdge{From: "lb:" + n, To: "vm:" + h.Instance, Label: bn, OK: h.Healthy})
			}
		}
	}
	for _, n := range SortedKeys(p.Clusters) {
		nodes = append(nodes, TopoNode{ID: "gke:" + n, Label: "GKE " + n, Kind: "gke"})
	}
	// VM-to-VM application dependencies (from startup scripts)
	for _, n := range SortedKeys(p.Instances) {
		vm := p.Instances[n]
		ls, _ := s.VMListeners(project, vm)
		for _, l := range ls {
			if h := l.Env["DB_HOST"]; h != "" {
				if ep, ok := s.FindIP(h); ok {
					port := 5432
					fr := s.CheckFlow(s.VMEndpoint(project, vm), func() Endpoint { e := ep; return e }(), "tcp", port)
					to := ep.Kind + ":" + ep.Name
					edges = append(edges, TopoEdge{From: "vm:" + n, To: to, Label: fmt.Sprintf("tcp:%d", port), OK: fr.Allowed || ep.Kind == "sql"})
				}
			}
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes, edges
}

// Mermaid renders the topology as a Mermaid flowchart.
func (s *State) Mermaid(project string) string {
	nodes, edges := s.Topology(project)
	var b strings.Builder
	b.WriteString("flowchart LR\n")
	id := func(x string) string {
		r := strings.NewReplacer(":", "_", "-", "_", ".", "_", "/", "_")
		return r.Replace(x)
	}
	for _, n := range nodes {
		shape := fmt.Sprintf("[\"%s\"]", strings.ReplaceAll(n.Label, "\\n", "<br/>"))
		switch n.Kind {
		case "sql", "bucket":
			shape = fmt.Sprintf("[(\"%s\")]", strings.ReplaceAll(n.Label, "\\n", "<br/>"))
		case "lb":
			shape = fmt.Sprintf("{{\"%s\"}}", strings.ReplaceAll(n.Label, "\\n", "<br/>"))
		}
		b.WriteString("  " + id(n.ID) + shape + "\n")
	}
	for _, e := range edges {
		arrow := "-->"
		if !e.OK && e.Label != "publish" && e.Label != "read" {
			arrow = "-.-x"
		}
		if e.Label != "" {
			b.WriteString(fmt.Sprintf("  %s %s|%s| %s\n", id(e.From), arrow, e.Label, id(e.To)))
		} else {
			b.WriteString(fmt.Sprintf("  %s %s %s\n", id(e.From), arrow, id(e.To)))
		}
	}
	return b.String()
}
