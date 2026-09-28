package sim

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
)

// Well-known source ranges used by Google front ends and IAP.
var (
	HealthCheckRanges = []string{"35.191.0.0/16", "130.211.0.0/22"}
	IAPRange          = "35.235.240.0/20"
	StudentIP         = "203.0.113.10" // the student's workstation on the internet
)

// ParseCIDR returns network and prefix, or nil.
func ParseCIDR(c string) *net.IPNet {
	if !strings.Contains(c, "/") {
		c += "/32"
	}
	_, n, err := net.ParseCIDR(c)
	if err != nil {
		return nil
	}
	return n
}

// IPInCIDR reports whether ip belongs to cidr.
func IPInCIDR(ip, cidr string) bool {
	n := ParseCIDR(cidr)
	p := net.ParseIP(ip)
	return n != nil && p != nil && n.Contains(p)
}

// CIDROverlap reports whether two CIDR ranges overlap.
func CIDROverlap(a, b string) bool {
	na, nb := ParseCIDR(a), ParseCIDR(b)
	if na == nil || nb == nil {
		return false
	}
	return na.Contains(nb.IP) || nb.Contains(na.IP)
}

// AllocIP returns the next free host address in a subnet range.
func (s *State) AllocIP(cidr string) string {
	s.init()
	n := ParseCIDR(cidr)
	if n == nil {
		return ""
	}
	ip := n.IP.To4()
	prefix := fmt.Sprintf("%d.%d.%d", ip[0], ip[1], ip[2])
	if s.ipSeq[prefix] < 1 {
		s.ipSeq[prefix] = 1 // .0 network, .1 gateway
	}
	s.ipSeq[prefix]++
	return fmt.Sprintf("%s.%d", prefix, s.ipSeq[prefix])
}

// ExternalIP allocates a public address.
func (s *State) ExternalIP() string {
	s.init()
	s.ipSeq["ext"]++
	n := s.ipSeq["ext"]
	return fmt.Sprintf("34.%d.%d.%d", 76+n/250, (n*37)%250+1, n%250+2)
}

// Endpoint is a network participant for flow evaluation.
type Endpoint struct {
	Kind    string   `json:"kind"` // vm, internet, gfe, iap, run, sql, pod, google-api
	Project string   `json:"project"`
	Network string   `json:"network"`
	Name    string   `json:"name"`
	IP      string   `json:"ip"`
	Tags    []string `json:"tags"`
	SA      string   `json:"sa"`
	Region  string   `json:"region"`
	Subnet  string   `json:"subnet"`
}

// FlowResult explains a connectivity decision.
type FlowResult struct {
	Allowed bool   `json:"allowed"`
	Stage   string `json:"stage"`  // route, egress-firewall, ingress-firewall, instance, listener
	Rule    string `json:"rule"`   // deciding rule
	Reason  string `json:"reason"` // human-readable
	Error   string `json:"error"`  // client-visible error (timeout/refused)
}

// VMEndpoint builds an endpoint for an instance.
func (s *State) VMEndpoint(project string, vm *Instance) Endpoint {
	return Endpoint{Kind: "vm", Project: project, Network: vm.Network, Name: vm.Name, IP: vm.InternalIP, Tags: vm.Tags, SA: vm.ServiceAccount, Region: RegionOf(vm.Zone), Subnet: vm.Subnet}
}

// InternetEndpoint is the student's machine or an arbitrary internet client.
func InternetEndpoint(ip string) Endpoint {
	if ip == "" {
		ip = StudentIP
	}
	return Endpoint{Kind: "internet", IP: ip}
}

func hasAny(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

func portMatches(rules []FWRule, proto string, port int) bool {
	for _, r := range rules {
		p := strings.ToLower(r.Protocol)
		if p != "all" && p != proto && !(p == "6" && proto == "tcp") && !(p == "17" && proto == "udp") {
			continue
		}
		if len(r.Ports) == 0 || proto == "icmp" {
			return true
		}
		for _, pr := range r.Ports {
			if strings.Contains(pr, "-") {
				ab := strings.SplitN(pr, "-", 2)
				lo, _ := strconv.Atoi(ab[0])
				hi, _ := strconv.Atoi(ab[1])
				if port >= lo && port <= hi {
					return true
				}
			} else if n, _ := strconv.Atoi(pr); n == port {
				return true
			}
		}
	}
	return false
}

// fwApplies checks a rule target selector against an endpoint.
func fwTargets(fw *Firewall, ep Endpoint) bool {
	if len(fw.TargetTags) > 0 {
		return hasAny(fw.TargetTags, ep.Tags)
	}
	if len(fw.TargetSAs) > 0 {
		for _, sa := range fw.TargetSAs {
			if sa == ep.SA {
				return true
			}
		}
		return false
	}
	return true
}

func fwSourceMatches(fw *Firewall, src Endpoint, sameNetwork bool) bool {
	for _, r := range fw.SourceRanges {
		if IPInCIDR(src.IP, r) {
			return true
		}
	}
	if sameNetwork && len(fw.SourceTags) > 0 && hasAny(fw.SourceTags, src.Tags) {
		return true
	}
	if sameNetwork && len(fw.SourceSAs) > 0 {
		for _, sa := range fw.SourceSAs {
			if sa == src.SA {
				return true
			}
		}
	}
	return false
}

// sortedFirewalls returns enabled rules of a network by priority, deny first on ties.
func (p *Project) sortedFirewalls(network, direction string) []*Firewall {
	var out []*Firewall
	for _, fw := range p.Firewalls {
		if fw.Network == network && strings.EqualFold(fw.Direction, direction) && !fw.Disabled {
			out = append(out, fw)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		if out[i].Action != out[j].Action {
			return out[i].Action == "DENY"
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// networksConnected reports whether two VPCs can route to each other.
func (s *State) networksConnected(pa, na, pb, nb string) bool {
	if pa == pb && na == nb {
		return true
	}
	a := s.network(pa, na)
	b := s.network(pb, nb)
	if a == nil || b == nil {
		return false
	}
	aToB, bToA := false, false
	for _, pe := range a.Peerings {
		if pe.Network == pb+"/"+nb && pe.State != "INACTIVE" {
			aToB = true
		}
	}
	for _, pe := range b.Peerings {
		if pe.Network == pa+"/"+na && pe.State != "INACTIVE" {
			bToA = true
		}
	}
	return aToB && bToA
}

func (s *State) network(project, name string) *Network {
	if p := s.Projects[project]; p != nil {
		return p.Networks[name]
	}
	return nil
}

// hasDefaultInternetRoute checks for a 0.0.0.0/0 route to the internet gateway
// (applicable to an endpoint's tags).
func (p *Project) hasDefaultInternetRoute(network string, tags []string) bool {
	for _, r := range p.Routes {
		if r.Network == network && r.DestRange == "0.0.0.0/0" && r.NextHopGateway != "" {
			if len(r.Tags) == 0 || hasAny(r.Tags, tags) {
				return true
			}
		}
	}
	return false
}

// evalFirewall returns the deciding rule for traffic, falling back to the
// implied rules (deny ingress / allow egress).
func (p *Project) evalFirewall(direction string, subject Endpoint, peer Endpoint, sameNet bool, proto string, port int) (bool, string) {
	for _, fw := range p.sortedFirewalls(subject.Network, direction) {
		if !fwTargets(fw, subject) || !portMatches(fw.Rules, proto, port) {
			continue
		}
		if direction == "INGRESS" {
			if !fwSourceMatches(fw, peer, sameNet) {
				continue
			}
		} else {
			if len(fw.DestRanges) > 0 {
				ok := false
				for _, r := range fw.DestRanges {
					if IPInCIDR(peer.IP, r) {
						ok = true
					}
				}
				if !ok {
					continue
				}
			}
		}
		return fw.Action == "ALLOW", fw.Name
	}
	if direction == "INGRESS" {
		return false, "implied-deny-ingress"
	}
	return true, "implied-allow-egress"
}

// CheckFlow evaluates whether src can open a connection to dst:port.
func (s *State) CheckFlow(src, dst Endpoint, proto string, port int) FlowResult {
	if proto == "" {
		proto = "tcp"
	}
	timeout := fmt.Sprintf("connection to %s port %d timed out", dst.IP, port)
	// Routing
	sameNet := src.Kind != "internet" && src.Kind != "gfe" && src.Kind != "iap" && dst.Kind != "internet" && src.Network != "" && dst.Network != "" && s.networksConnected(src.Project, src.Network, dst.Project, dst.Network)
	if src.Network != "" && dst.Network != "" && !sameNet && dst.Kind != "internet" {
		// Different VPCs without peering: only possible through public IPs.
		return FlowResult{Allowed: false, Stage: "route", Reason: fmt.Sprintf("no route from %s/%s to %s/%s (VPCs are not peered)", src.Project, src.Network, dst.Project, dst.Network), Error: timeout}
	}
	if dst.Kind == "vm" {
		dp := s.Projects[dst.Project]
		if dp == nil {
			return FlowResult{Stage: "route", Reason: "unknown project", Error: timeout}
		}
		vm := dp.Instances[dst.Name]
		if vm == nil || vm.Status != "RUNNING" {
			return FlowResult{Stage: "instance", Reason: "destination instance is not running", Error: timeout}
		}
		if src.Kind == "internet" {
			if vm.ExternalIP == "" {
				return FlowResult{Stage: "route", Reason: "instance has no external IP address", Error: timeout}
			}
			if !dp.hasDefaultInternetRoute(vm.Network, vm.Tags) {
				return FlowResult{Stage: "route", Reason: "no default route to the internet gateway for return traffic", Error: timeout}
			}
		}
	}
	// Egress firewall on the source side.
	if src.Kind == "vm" || src.Kind == "pod" || src.Kind == "run-vpc" {
		if sp := s.Projects[src.Project]; sp != nil {
			ok, rule := sp.evalFirewall("EGRESS", src, dst, sameNet, proto, port)
			if !ok {
				return FlowResult{Stage: "egress-firewall", Rule: rule, Reason: fmt.Sprintf("egress blocked by firewall rule %s", rule), Error: timeout}
			}
			if dst.Kind == "internet" || dst.Kind == "google-api" {
				if ok, why := s.internetPath(src); !ok {
					return FlowResult{Stage: "route", Reason: why, Error: timeout}
				}
			}
		}
	}
	// Ingress firewall on the destination side (only VPC endpoints).
	if dst.Kind == "vm" || dst.Kind == "pod" {
		dp := s.Projects[dst.Project]
		ok, rule := dp.evalFirewall("INGRESS", dst, src, sameNet, proto, port)
		if !ok {
			return FlowResult{Stage: "ingress-firewall", Rule: rule, Reason: fmt.Sprintf("ingress to %s:%d blocked by %s", dst.Name, port, rule), Error: timeout}
		}
		return FlowResult{Allowed: true, Stage: "ingress-firewall", Rule: rule, Reason: "allowed by " + rule}
	}
	return FlowResult{Allowed: true, Stage: "route", Reason: "reachable"}
}

// internetPath checks whether a VPC endpoint can reach the internet.
func (s *State) internetPath(src Endpoint) (bool, string) {
	p := s.Projects[src.Project]
	if p == nil {
		return false, "unknown project"
	}
	if !p.hasDefaultInternetRoute(src.Network, src.Tags) {
		return false, "no route to 0.0.0.0/0 via default-internet-gateway"
	}
	if src.Kind == "vm" {
		if vm := p.Instances[src.Name]; vm != nil && vm.ExternalIP != "" {
			return true, "external IP"
		}
	}
	if s.natCovers(src.Project, src.Network, src.Region, src.Subnet) {
		return true, "Cloud NAT"
	}
	return false, fmt.Sprintf("%s has no external IP and no Cloud NAT covers subnet %s in %s", src.Name, src.Subnet, src.Region)
}

func (s *State) natCovers(project, network, region, subnet string) bool {
	p := s.Projects[project]
	if p == nil {
		return false
	}
	for _, r := range p.Routers {
		if r.Network != network || r.Region != region {
			continue
		}
		for _, n := range r.NATs {
			if n.AllSubnets {
				return true
			}
			for _, sn := range n.Subnets {
				if sn == subnet {
					return true
				}
			}
		}
	}
	return false
}

// GoogleAPIAccess checks whether a VPC endpoint can reach *.googleapis.com.
func (s *State) GoogleAPIAccess(src Endpoint) (bool, string) {
	if src.Kind == "run" || src.Kind == "internet" {
		return true, "public path"
	}
	p := s.Projects[src.Project]
	if p == nil {
		return false, "unknown project"
	}
	if ok, why := s.internetPath(src); ok {
		return true, why
	}
	if sn := p.Subnets[src.Subnet]; sn != nil && sn.PrivateGoogleAccess {
		if p.hasDefaultInternetRoute(src.Network, src.Tags) {
			return true, "Private Google Access"
		}
		return false, "Private Google Access requires a route to the default internet gateway"
	}
	return false, fmt.Sprintf("%s cannot reach Google APIs: no external IP, no Cloud NAT and Private Google Access is disabled on %s", src.Name, src.Subnet)
}

// FindIP resolves an IP to an endpoint across projects.
func (s *State) FindIP(ip string) (Endpoint, bool) {
	for _, pid := range SortedKeys(s.Projects) {
		p := s.Projects[pid]
		for _, n := range SortedKeys(p.Instances) {
			vm := p.Instances[n]
			if vm.InternalIP == ip || (vm.ExternalIP != "" && vm.ExternalIP == ip) {
				ep := s.VMEndpoint(pid, vm)
				ep.IP = ip
				return ep, true
			}
		}
		for _, n := range SortedKeys(p.SQLInstances) {
			in := p.SQLInstances[n]
			if in.PrivateIP == ip || in.PublicIP == ip {
				return Endpoint{Kind: "sql", Project: pid, Name: in.Name, IP: ip, Network: in.Network, Region: in.Region}, true
			}
		}
	}
	return Endpoint{}, false
}

// SubnetForNetwork finds a subnet of a network in a region.
func (p *Project) SubnetForRegion(network, region string) *Subnet {
	for _, k := range SortedKeys(p.Subnets) {
		sn := p.Subnets[k]
		if sn.Network == network && sn.Region == region {
			return sn
		}
	}
	return nil
}

// ResolveHost resolves a hostname for a client in a given network context.
func (s *State) ResolveHost(host string, from Endpoint) (string, bool) {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if net.ParseIP(host) != nil {
		return host, true
	}
	// internal VM DNS: name, name.zone.c.project.internal, name.c.project.internal
	if from.Project != "" {
		if p := s.Projects[from.Project]; p != nil {
			short := strings.SplitN(host, ".", 2)[0]
			if vm := p.Instances[short]; vm != nil && (host == short || strings.HasSuffix(host, ".internal")) {
				return vm.InternalIP, true
			}
		}
	}
	defer func() {}()
	for _, pid := range SortedKeys(s.Projects) {
		p := s.Projects[pid]
		for _, zn := range SortedKeys(p.DNSZones) {
			z := p.DNSZones[zn]
			if z.Visibility == "private" {
				visible := false
				for _, n := range z.Networks {
					if pid == from.Project && n == from.Network {
						visible = true
					}
				}
				if !visible {
					continue
				}
			}
			for _, r := range z.Records {
				if strings.TrimSuffix(r.Name, ".") == host && (r.Type == "A") && len(r.Data) > 0 {
					return r.Data[0], true
				}
				if strings.TrimSuffix(r.Name, ".") == host && r.Type == "CNAME" && len(r.Data) > 0 {
					return s.ResolveHost(r.Data[0], from)
				}
			}
		}
	}
	for _, suffix := range publicInternet {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return "151.101.2.132", true
		}
	}
	return "", false
}

// publicInternet are well-known internet domains resolvable by public DNS.
var publicInternet = []string{"debian.org", "ubuntu.com", "google.com", "googleapis.com", "github.com", "pypi.org", "npmjs.org", "docker.io", "example.com", "exchangerate.example", "golang.org"}

// NetworksConnected is the exported form of networksConnected.
func (s *State) NetworksConnected(pa, na, pb, nb string) bool { return s.networksConnected(pa, na, pb, nb) }

// autoSubnets is the auto-mode subnet plan (subset of regions).
var autoSubnets = map[string]string{
	"us-central1": "10.128.0.0/20", "us-east1": "10.142.0.0/20", "us-west1": "10.138.0.0/20", "europe-west1": "10.132.0.0/20",
	"europe-west2": "10.154.0.0/20", "europe-west3": "10.156.0.0/20", "europe-west4": "10.164.0.0/20", "europe-southwest1": "10.204.0.0/20",
	"asia-east1": "10.140.0.0/20", "asia-northeast1": "10.146.0.0/20", "asia-southeast1": "10.148.0.0/20", "us-east4": "10.150.0.0/20",
	"europe-north1": "10.166.0.0/20", "southamerica-east1": "10.158.0.0/20", "australia-southeast1": "10.152.0.0/20",
	"northamerica-northeast1": "10.162.0.0/20", "us-west2": "10.168.0.0/20",
}

// CreateNetwork creates a VPC with its default internet route (and subnets in auto mode).
func (s *State) CreateNetwork(p *Project, name, mode string) *Network {
	n := &Network{Name: name, Mode: mode}
	p.Networks[name] = n
	rn := "default-route-" + s.ID(16)
	p.Routes[rn] = &Route{Name: rn, Network: name, DestRange: "0.0.0.0/0", Priority: 1000, NextHopGateway: "default-internet-gateway", System: true}
	if mode == "auto" {
		for region, cidr := range autoSubnets {
			p.Subnets[name+"-"+region] = &Subnet{Name: name + "-" + region, Region: region, Network: name, Range: cidr}
			if name == "default" {
				p.Subnets[name+"-"+region].Name = name + "-" + region
			}
		}
	}
	return n
}

// DefaultNetwork creates the classic "default" VPC with its default firewall rules.
func (s *State) DefaultNetwork(p *Project) {
	s.CreateNetwork(p, "default", "auto")
	p.Firewalls["default-allow-internal"] = &Firewall{Name: "default-allow-internal", Network: "default", Direction: "INGRESS", Priority: 65534, Action: "ALLOW", Rules: []FWRule{{Protocol: "tcp", Ports: []string{"0-65535"}}, {Protocol: "udp", Ports: []string{"0-65535"}}, {Protocol: "icmp"}}, SourceRanges: []string{"10.128.0.0/9"}}
	p.Firewalls["default-allow-ssh"] = &Firewall{Name: "default-allow-ssh", Network: "default", Direction: "INGRESS", Priority: 65534, Action: "ALLOW", Rules: []FWRule{{Protocol: "tcp", Ports: []string{"22"}}}, SourceRanges: []string{"0.0.0.0/0"}}
	p.Firewalls["default-allow-rdp"] = &Firewall{Name: "default-allow-rdp", Network: "default", Direction: "INGRESS", Priority: 65534, Action: "ALLOW", Rules: []FWRule{{Protocol: "tcp", Ports: []string{"3389"}}}, SourceRanges: []string{"0.0.0.0/0"}}
	p.Firewalls["default-allow-icmp"] = &Firewall{Name: "default-allow-icmp", Network: "default", Direction: "INGRESS", Priority: 65534, Action: "ALLOW", Rules: []FWRule{{Protocol: "icmp"}}, SourceRanges: []string{"0.0.0.0/0"}}
}
