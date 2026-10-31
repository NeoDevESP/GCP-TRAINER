package sim

import (
	"fmt"
	"sort"
	"strings"
)

// Hybrid connectivity: the bank's own data centre ("on-prem") reached over
// Cloud VPN with BGP through a Cloud Router. Reachability needs a tunnel that
// is up, a BGP session that is established (both sides agree on the ASNs) and
// routes in both directions: the VPC learns the on-prem ranges and on-prem
// learns whatever the Cloud Router advertises.

// OnPrem is the bank's data centre at the other end of the VPN.
type OnPrem struct {
	Name      string       `json:"name"`
	ASN       int          `json:"asn"`       // the on-prem router's ASN
	GoogleASN int          `json:"googleAsn"` // the ASN on-prem expects from Google (0 = any)
	Ranges    []string     `json:"ranges"`
	DNSServer string       `json:"dnsServer,omitempty"`
	Hosts     []OnPremHost `json:"hosts,omitempty"`
}

// OnPremHost is a server in the data centre.
type OnPremHost struct {
	Name  string `json:"name"` // FQDN, e.g. core.zentra.corp
	IP    string `json:"ip"`
	Ports []int  `json:"ports"`
	Body  string `json:"body,omitempty"`
}

// CloudDNSForwarders is the range Cloud DNS uses to forward queries; on-prem
// only answers if it has a route back (a custom advertisement).
const CloudDNSForwarders = "35.199.192.0/19"

// PeerStatus is what `routers get-status` shows for a BGP peer.
type PeerStatus struct {
	Name       string   `json:"name"`
	PeerASN    int      `json:"peerAsn"`
	Tunnel     string   `json:"linkedVpnTunnel,omitempty"`
	Status     string   `json:"status"` // UP / DOWN
	State      string   `json:"state"`  // Established / Idle / Active / Connect
	Reason     string   `json:"statusReason,omitempty"`
	Learned    []string `json:"learnedRoutes,omitempty"`
	Advertised []string `json:"advertisedRoutes,omitempty"`
}

func (s *State) routerTunnels(p *Project, r *Router) []VPNTunnel {
	var out []VPNTunnel
	for _, gn := range SortedKeys(p.VPNGateways) {
		gw := p.VPNGateways[gn]
		for _, t := range gw.Tunnels {
			if t.Router == r.Name {
				out = append(out, t)
			}
		}
	}
	return out
}

// Advertised returns the prefixes a Cloud Router announces to its peers.
func (s *State) Advertised(p *Project, r *Router) []string {
	var out []string
	allSubnets := !strings.EqualFold(r.AdvertiseMode, "CUSTOM")
	for _, g := range r.AdvertiseGroups {
		if strings.EqualFold(g, "ALL_SUBNETS") {
			allSubnets = true
		}
	}
	if allSubnets {
		for _, sn := range SortedKeys(p.Subnets) {
			if sub := p.Subnets[sn]; sub.Network == r.Network {
				out = append(out, sub.Range)
			}
		}
	}
	if strings.EqualFold(r.AdvertiseMode, "CUSTOM") {
		out = append(out, r.AdvertisedRanges...)
	}
	return out
}

// BGPStatus evaluates every BGP session of a router.
func (s *State) BGPStatus(project string, r *Router) []PeerStatus {
	p := s.Projects[project]
	var out []PeerStatus
	tunnels := s.routerTunnels(p, r)
	for _, peer := range r.BGPPeers {
		ps := PeerStatus{Name: peer.Name, PeerASN: peer.PeerASN, Status: "DOWN", State: "Idle"}
		var tun *VPNTunnel
		for i := range tunnels {
			if peer.Interface == "" || strings.Contains(peer.Interface, tunnels[i].Name) || len(tunnels) == 1 {
				tun = &tunnels[i]
				break
			}
		}
		switch {
		case tun == nil:
			ps.Reason = "no VPN tunnel is attached to this BGP interface"
		case tun.Status != "ESTABLISHED":
			ps.Tunnel, ps.State = tun.Name, "Active"
			ps.Reason = fmt.Sprintf("VPN tunnel %s is %s: BGP cannot connect", tun.Name, tun.Status)
		case s.OnPrem == nil:
			ps.Tunnel, ps.State = tun.Name, "Active"
			ps.Reason = "no BGP speaker answers at the other end"
		case peer.PeerASN != s.OnPrem.ASN:
			ps.Tunnel = tun.Name
			ps.Reason = fmt.Sprintf("BGP OPEN rejected (Bad Peer AS): configured peer ASN %d, the peer announced ASN %d", peer.PeerASN, s.OnPrem.ASN)
		case s.OnPrem.GoogleASN != 0 && r.ASN != s.OnPrem.GoogleASN:
			ps.Tunnel = tun.Name
			ps.Reason = fmt.Sprintf("BGP OPEN rejected by the peer (Bad Peer AS): the peer expects Google ASN %d, the Cloud Router uses %d", s.OnPrem.GoogleASN, r.ASN)
		default:
			ps.Tunnel, ps.Status, ps.State = tun.Name, "UP", "Established"
			ps.Learned = append([]string{}, s.OnPrem.Ranges...)
			ps.Advertised = s.Advertised(p, r)
		}
		out = append(out, ps)
	}
	return out
}

// hybridSession returns the established session of a network, if any.
func (s *State) hybridSession(project, network string) (*Router, *PeerStatus) {
	p := s.Projects[project]
	if p == nil {
		return nil, nil
	}
	for _, rn := range SortedKeys(p.Routers) {
		r := p.Routers[rn]
		if r.Network != network {
			continue
		}
		for _, ps := range s.BGPStatus(project, r) {
			if ps.Status == "UP" {
				ps := ps
				return r, &ps
			}
		}
	}
	return nil, nil
}

// HybridPath checks traffic between a VPC address and an on-prem address
// (both directions need the same routes: the reply must come back).
func (s *State) HybridPath(project, network, vpcIP, onpremIP string) (bool, string) {
	if s.OnPrem == nil {
		return false, "there is no hybrid connection to the data centre"
	}
	in := false
	for _, r := range s.OnPrem.Ranges {
		if IPInCIDR(onpremIP, r) {
			in = true
		}
	}
	if !in {
		return false, fmt.Sprintf("%s is not in the on-prem ranges %v", onpremIP, s.OnPrem.Ranges)
	}
	r, ps := s.hybridSession(project, network)
	if r == nil {
		reason := "no established BGP session on network " + network
		if p := s.Projects[project]; p != nil {
			for _, rn := range SortedKeys(p.Routers) {
				if p.Routers[rn].Network == network {
					for _, st := range s.BGPStatus(project, p.Routers[rn]) {
						reason = fmt.Sprintf("BGP session %s on router %s is %s: %s", st.Name, rn, st.State, st.Reason)
					}
				}
			}
		}
		return false, reason + " (the VPC has no route to " + onpremIP + ")"
	}
	for _, a := range ps.Advertised {
		if IPInCIDR(vpcIP, a) {
			return true, fmt.Sprintf("via VPN tunnel %s, BGP session %s established", ps.Tunnel, ps.Name)
		}
	}
	return false, fmt.Sprintf("router %s does not advertise a range containing %s: the data centre has no route back (advertised %v)", r.Name, vpcIP, ps.Advertised)
}

// OnPremHostByIP finds a data-centre server.
func (s *State) OnPremHostByIP(ip string) *OnPremHost {
	if s.OnPrem == nil {
		return nil
	}
	for i := range s.OnPrem.Hosts {
		if s.OnPrem.Hosts[i].IP == ip {
			return &s.OnPrem.Hosts[i]
		}
	}
	return nil
}

// OnPremEndpoint is a data-centre server as a traffic source.
func (s *State) OnPremEndpoint(name string) (Endpoint, bool) {
	if s.OnPrem == nil {
		return Endpoint{}, false
	}
	for _, h := range s.OnPrem.Hosts {
		if h.Name == name || strings.SplitN(h.Name, ".", 2)[0] == name {
			return Endpoint{Kind: "onprem", Name: h.Name, IP: h.IP}, true
		}
	}
	return Endpoint{}, false
}

// resolveForwarded answers a query sent by a Cloud DNS forwarding zone.
func (s *State) resolveForwarded(z *DNSZone, host string, from Endpoint) (string, bool, string) {
	if s.OnPrem == nil {
		return "", false, "no DNS server answers at the forwarding target"
	}
	target := ""
	for _, t := range z.ForwardingTargets {
		if t == s.OnPrem.DNSServer {
			target = t
		}
	}
	if target == "" {
		return "", false, fmt.Sprintf("forwarding targets %v do not include the data-centre DNS server", z.ForwardingTargets)
	}
	r, ps := s.hybridSession(from.Project, from.Network)
	if r == nil {
		return "", false, "the VPC has no route to the data-centre DNS server (BGP down)"
	}
	back := false
	for _, a := range ps.Advertised {
		if a == CloudDNSForwarders {
			back = true
		}
	}
	if !back {
		return "", false, fmt.Sprintf("queries leave from %s but router %s does not advertise it: the data-centre DNS server has no route back", CloudDNSForwarders, r.Name)
	}
	for _, h := range s.OnPrem.Hosts {
		if strings.EqualFold(h.Name, host) {
			return h.IP, true, ""
		}
	}
	return "", false, "NXDOMAIN from the data-centre DNS server"
}

// OnPremSummary lists the data centre for the UI/CLI.
func (o *OnPrem) Summary() string {
	if o == nil {
		return ""
	}
	names := []string{}
	for _, h := range o.Hosts {
		names = append(names, h.Name+"="+h.IP)
	}
	sort.Strings(names)
	return fmt.Sprintf("%s ASN %d ranges %v hosts %v", o.Name, o.ASN, o.Ranges, names)
}
