package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

func (c *Cmd) name0() (string, error) {
	n, err := c.Arg(0, "NAME")
	if err != nil {
		return "", err
	}
	if !reResName.MatchString(n) {
		return "", fmt.Errorf("Invalid value for field 'resource.name': '%s'. Must be a match of regex '(?:[a-z](?:[-a-z0-9]{0,61}[a-z0-9])?)'", n)
	}
	return n, nil
}

func created(p, kind, loc, n string) string {
	return fmt.Sprintf("Created [https://www.googleapis.com/compute/v1/projects/%s/%s/%s/%s].\n", p, loc, kind, n)
}

func parseFWRules(v []string) ([]sim.FWRule, error) {
	var rules []sim.FWRule
	for _, spec := range v {
		for _, r := range strings.Split(spec, ",") {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			proto, port, _ := strings.Cut(r, ":")
			proto = strings.ToLower(proto)
			switch proto {
			case "tcp", "udp", "icmp", "all", "esp", "ah", "sctp":
			default:
				if _, err := strconv.Atoi(proto); err != nil {
					return nil, fmt.Errorf("Invalid value for [--rules]: %q (expected PROTOCOL[:PORT[-PORT]])", r)
				}
			}
			merged := false
			for i := range rules {
				if rules[i].Protocol == proto && port != "" {
					rules[i].Ports = append(rules[i].Ports, port)
					merged = true
				}
			}
			if !merged {
				fr := sim.FWRule{Protocol: proto}
				if port != "" {
					fr.Ports = []string{port}
				}
				rules = append(rules, fr)
			}
		}
	}
	return rules, nil
}

func fwView(fw *sim.Firewall) map[string]any {
	allow, deny := []sim.FWRule{}, []sim.FWRule{}
	if fw.Action == "ALLOW" {
		allow = fw.Rules
	} else {
		deny = fw.Rules
	}
	var rs []string
	for _, r := range fw.Rules {
		if len(r.Ports) == 0 {
			rs = append(rs, r.Protocol)
		}
		for _, p := range r.Ports {
			rs = append(rs, r.Protocol+":"+p)
		}
	}
	allowStr, denyStr := "", ""
	if fw.Action == "ALLOW" {
		allowStr = strings.Join(rs, ",")
	} else {
		denyStr = strings.Join(rs, ",")
	}
	return map[string]any{"name": fw.Name, "network": fw.Network, "direction": fw.Direction, "priority": fw.Priority, "allowed": allow, "denied": deny,
		"rules": strings.Join(rs, ","), "allowStr": allowStr, "denyStr": denyStr, "action": fw.Action, "sourceRanges": fw.SourceRanges, "destinationRanges": fw.DestRanges, "sourceTags": fw.SourceTags,
		"targetTags": fw.TargetTags, "sourceServiceAccounts": fw.SourceSAs, "targetServiceAccounts": fw.TargetSAs, "disabled": fw.Disabled,
		"logConfig": map[string]any{"enable": fw.Logging}, "description": fw.Description}
}

func (c *Cmd) applyFW(fw *sim.Firewall, create bool) error {
	if v := c.Str("direction", ""); v != "" {
		v = strings.ToUpper(v)
		if v == "IN" {
			v = "INGRESS"
		} else if v == "OUT" {
			v = "EGRESS"
		}
		fw.Direction = v
	}
	if c.Has("priority") {
		n, err := strconv.Atoi(c.Str("priority", ""))
		if err != nil || n < 0 || n > 65535 {
			return fmt.Errorf("Invalid value for [--priority]: must be an integer between 0 and 65535")
		}
		fw.Priority = n
	}
	if c.Has("allow") || c.Has("action") || c.Has("rules") {
		spec := c.F["rules"]
		if c.Has("allow") {
			spec = c.F["allow"]
			fw.Action = "ALLOW"
		}
		if a := c.Str("action", ""); a != "" {
			fw.Action = strings.ToUpper(a)
		}
		if len(spec) > 0 {
			r, err := parseFWRules(spec)
			if err != nil {
				return err
			}
			fw.Rules = r
		}
	}
	if c.Has("source-ranges") {
		fw.SourceRanges = c.List("source-ranges")
		for _, r := range fw.SourceRanges {
			if sim.ParseCIDR(r) == nil {
				return fmt.Errorf("Invalid value for field 'resource.sourceRanges': '%s'. Must be a CIDR address range.", r)
			}
		}
	}
	if c.Has("destination-ranges") {
		fw.DestRanges = c.List("destination-ranges")
	}
	if c.Has("source-tags") {
		fw.SourceTags = c.List("source-tags")
	}
	if c.Has("target-tags") {
		fw.TargetTags = c.List("target-tags")
	}
	if c.Has("source-service-accounts") {
		fw.SourceSAs = c.List("source-service-accounts")
	}
	if c.Has("target-service-accounts") {
		fw.TargetSAs = c.List("target-service-accounts")
	}
	if c.Bool("disabled") {
		fw.Disabled = true
	}
	if c.Bool("no-disabled") {
		fw.Disabled = false
	}
	if c.Bool("enable-logging") {
		fw.Logging = true
	}
	if c.Bool("no-enable-logging") {
		fw.Logging = false
	}
	if v := c.Str("description", ""); v != "" {
		fw.Description = v
	}
	if len(fw.Rules) == 0 {
		return fmt.Errorf("Exactly one of (--action | --allow) must be specified.")
	}
	if fw.Action == "" {
		return fmt.Errorf("--rules requires --action ALLOW|DENY")
	}
	if len(fw.TargetTags) > 0 && len(fw.TargetSAs) > 0 {
		return fmt.Errorf("Invalid value: target tags and target service accounts cannot be used together")
	}
	if create && fw.Direction == "INGRESS" && len(fw.SourceRanges) == 0 && len(fw.SourceTags) == 0 && len(fw.SourceSAs) == 0 {
		fw.SourceRanges = []string{"0.0.0.0/0"}
	}
	if fw.Direction == "EGRESS" && len(fw.DestRanges) == 0 {
		fw.DestRanges = []string{"0.0.0.0/0"}
	}
	return nil
}

func init() {
	reg("compute networks create", func(c *Cmd) (any, error) {
		if err := c.API("compute.googleapis.com"); err != nil {
			return nil, err
		}
		p, _ := c.P()
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		if p.Networks[n] != nil {
			return nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/global/networks/%s' already exists", p.ID, n)
		}
		if err := c.NeedProject("compute.networks.create"); err != nil {
			return nil, err
		}
		mode := strings.ToLower(c.Str("subnet-mode", "auto"))
		if mode != "auto" && mode != "custom" && mode != "legacy" {
			return nil, fmt.Errorf("argument --subnet-mode: Invalid choice: '%s'", mode)
		}
		c.S.State.CreateNetwork(p, n, mode)
		c.Audit("compute.googleapis.com", "v1.compute.networks.insert", "projects/"+p.ID+"/global/networks/"+n)
		out := created(p.ID, "networks", "global", n)
		out += fmt.Sprintf("NAME  SUBNET_MODE  BGP_ROUTING_MODE\n%s  %s  REGIONAL\n\nInstances on this network will not be reachable until firewall rules are created.\n", n, strings.ToUpper(mode))
		return out, nil
	})
	reg("compute networks list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("compute.networks.list"); err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Networks) {
			n := p.Networks[k]
			rows = append(rows, map[string]any{"name": n.Name, "x_gcloud_subnet_mode": strings.ToUpper(n.Mode), "peerings": n.Peerings})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"SUBNET_MODE", "x_gcloud_subnet_mode"}}, Rows: rows}, nil
	})
	reg("compute networks describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		nw := p.Networks[n]
		if nw == nil {
			return nil, fmt.Errorf("The resource 'projects/%s/global/networks/%s' was not found", p.ID, n)
		}
		return Obj{V: nw}, nil
	})
	reg("compute networks delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		for _, n := range c.Args {
			if p.Networks[n] == nil {
				return nil, fmt.Errorf("The resource 'projects/%s/global/networks/%s' was not found", p.ID, n)
			}
			for _, vm := range p.Instances {
				if vm.Network == n {
					return nil, fmt.Errorf("The network resource 'projects/%s/global/networks/%s' is already being used by 'projects/%s/zones/%s/instances/%s'", p.ID, n, p.ID, vm.Zone, vm.Name)
				}
			}
			for _, fw := range p.Firewalls {
				if fw.Network == n {
					return nil, fmt.Errorf("The network resource 'projects/%s/global/networks/%s' is already being used by 'projects/%s/global/firewalls/%s'", p.ID, n, p.ID, fw.Name)
				}
			}
			for k, sn := range p.Subnets {
				if sn.Network == n {
					delete(p.Subnets, k)
				}
			}
			for k, r := range p.Routes {
				if r.Network == n {
					delete(p.Routes, k)
				}
			}
			delete(p.Networks, n)
			c.Audit("compute.googleapis.com", "v1.compute.networks.delete", "projects/"+p.ID+"/global/networks/"+n)
		}
		return "Deleted.\n", nil
	})
	reg("compute networks peerings create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		local := p.Networks[c.Str("network", "")]
		if local == nil {
			return nil, fmt.Errorf("network %s not found", c.Str("network", ""))
		}
		peerProject := c.Str("peer-project", p.ID)
		peerNet := c.Str("peer-network", "")
		pp := c.S.State.Projects[peerProject]
		if pp == nil || pp.Networks[peerNet] == nil {
			return nil, fmt.Errorf("peer network %s/%s not found", peerProject, peerNet)
		}
		for _, sn := range p.Subnets {
			if sn.Network != c.Str("network", "") {
				continue
			}
			for _, psn := range pp.Subnets {
				if psn.Network == peerNet && sim.CIDROverlap(sn.Range, psn.Range) {
					return nil, fmt.Errorf("An IP range in the peer network (%s) overlaps with an IP range in the local network (%s) allocated by resource.", psn.Range, sn.Range)
				}
			}
		}
		local.Peerings = append(local.Peerings, sim.Peering{Name: n, Network: peerProject + "/" + peerNet, State: "ACTIVE"})
		c.Audit("compute.googleapis.com", "v1.compute.networks.addPeering", "projects/"+p.ID+"/global/networks/"+c.Str("network", ""))
		st := "ACTIVE"
		if !c.S.State.NetworksConnected(p.ID, c.Str("network", ""), peerProject, peerNet) {
			st = "INACTIVE (waiting for the peer network to create the matching peering)"
		}
		return "Updated. Peering state: " + st + "\n", nil
	})
	reg("compute networks peerings list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Networks) {
			for _, pe := range p.Networks[k].Peerings {
				rows = append(rows, map[string]any{"name": pe.Name, "network": k, "peerNetwork": pe.Network, "state": pe.State})
			}
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"NETWORK", "network"}, {"PEER_NETWORK", "peerNetwork"}, {"STATE", "state"}}, Rows: rows}, nil
	})
	reg("compute networks peerings delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		nw := p.Networks[c.Str("network", "")]
		if nw == nil {
			return nil, fmt.Errorf("network not found")
		}
		var keep []sim.Peering
		for _, pe := range nw.Peerings {
			if pe.Name != n {
				keep = append(keep, pe)
			}
		}
		nw.Peerings = keep
		return "Updated.\n", nil
	})
	reg("compute networks subnets create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		region, err := c.RegionFlag()
		if err != nil {
			return nil, err
		}
		netName := c.Str("network", "")
		nw := p.Networks[netName]
		if nw == nil {
			return nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/global/networks/%s' was not found", p.ID, netName)
		}
		if nw.Mode == "auto" {
			return nil, fmt.Errorf("Could not fetch resource:\n - Invalid resource usage: 'Subnetworks cannot be created in an auto-mode network. Convert it to custom mode first.'")
		}
		rng := c.Str("range", "")
		cidr := sim.ParseCIDR(rng)
		if cidr == nil || !strings.Contains(rng, "/") {
			return nil, fmt.Errorf("Invalid value for field 'resource.ipCidrRange': '%s'. Invalid IPCidrRange", rng)
		}
		if p.Subnets[n] != nil {
			return nil, fmt.Errorf("The resource 'projects/%s/regions/%s/subnetworks/%s' already exists", p.ID, region, n)
		}
		for _, sn := range p.Subnets {
			if sn.Network == netName && sim.CIDROverlap(sn.Range, rng) {
				return nil, fmt.Errorf("Could not fetch resource:\n - Invalid IPCidrRange: %s conflicts with existing subnetwork '%s' in region '%s'.", rng, sn.Name, sn.Region)
			}
		}
		if err := c.NeedProject("compute.subnetworks.create"); err != nil {
			return nil, err
		}
		p.Subnets[n] = &sim.Subnet{Name: n, Region: region, Network: netName, Range: cidr.String(), PrivateGoogleAccess: c.Bool("enable-private-ip-google-access"), FlowLogs: c.Bool("enable-flow-logs"), Purpose: c.Str("purpose", "")}
		c.Audit("compute.googleapis.com", "v1.compute.subnetworks.insert", "projects/"+p.ID+"/regions/"+region+"/subnetworks/"+n)
		return created(p.ID, "subnetworks", "regions/"+region, n) + fmt.Sprintf("NAME  REGION  NETWORK  RANGE\n%s  %s  %s  %s\n", n, region, netName, cidr.String()), nil
	})
	reg("compute networks subnets list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Subnets) {
			sn := p.Subnets[k]
			if nf := c.Str("network", ""); nf != "" && sn.Network != nf {
				continue
			}
			rows = append(rows, sn)
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"REGION", "region"}, {"NETWORK", "network"}, {"RANGE", "ipCidrRange"}, {"PRIVATE_GOOGLE_ACCESS", "privateIpGoogleAccess"}}, Rows: rows}, nil
	})
	reg("compute networks subnets describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		sn := p.Subnets[n]
		if sn == nil {
			return nil, fmt.Errorf("subnetwork %s not found", n)
		}
		return Obj{V: sn}, nil
	})
	reg("compute networks subnets update", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		sn := p.Subnets[n]
		if sn == nil {
			return nil, fmt.Errorf("subnetwork %s not found", n)
		}
		if err := c.NeedProject("compute.subnetworks.update"); err != nil {
			return nil, err
		}
		if c.Bool("enable-private-ip-google-access") {
			sn.PrivateGoogleAccess = true
		}
		if c.Bool("no-enable-private-ip-google-access") {
			sn.PrivateGoogleAccess = false
		}
		if c.Bool("enable-flow-logs") {
			sn.FlowLogs = true
		}
		c.Audit("compute.googleapis.com", "v1.compute.subnetworks.patch", "projects/"+p.ID+"/regions/"+sn.Region+"/subnetworks/"+n)
		return "Updated [https://www.googleapis.com/compute/v1/projects/" + p.ID + "/regions/" + sn.Region + "/subnetworks/" + n + "].\n", nil
	})
	reg("compute networks subnets expand-ip-range", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		sn := p.Subnets[n]
		if sn == nil {
			return nil, fmt.Errorf("subnetwork %s not found", n)
		}
		pl := c.Int("prefix-length", 0)
		cur, _ := strconv.Atoi(strings.Split(sn.Range, "/")[1])
		if pl >= cur || pl == 0 {
			return nil, fmt.Errorf("The new prefix length must be smaller than the current one (/%d); ranges can only be expanded.", cur)
		}
		base := strings.Split(sn.Range, "/")[0]
		nr := sim.ParseCIDR(fmt.Sprintf("%s/%d", base, pl)).String()
		for _, o := range p.Subnets {
			if o.Name != n && o.Network == sn.Network && sim.CIDROverlap(o.Range, nr) {
				return nil, fmt.Errorf("Expanded range %s overlaps subnetwork %s", nr, o.Name)
			}
		}
		sn.Range = nr
		return "Updated.\n", nil
	})
	reg("compute networks subnets delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		for _, n := range c.Args {
			for _, vm := range p.Instances {
				if vm.Subnet == n {
					return nil, fmt.Errorf("The subnetwork resource is already being used by instance %s", vm.Name)
				}
			}
			delete(p.Subnets, n)
		}
		return "Deleted.\n", nil
	})
	reg("compute firewall-rules create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		if p.Firewalls[n] != nil {
			return nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/global/firewalls/%s' already exists", p.ID, n)
		}
		netName := c.Str("network", "default")
		if p.Networks[netName] == nil {
			return nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/global/networks/%s' was not found", p.ID, netName)
		}
		if err := c.NeedProject("compute.firewalls.create"); err != nil {
			return nil, err
		}
		fw := &sim.Firewall{Name: n, Network: netName, Direction: "INGRESS", Priority: 1000}
		if err := c.applyFW(fw, true); err != nil {
			return nil, err
		}
		p.Firewalls[n] = fw
		c.Audit("compute.googleapis.com", "v1.compute.firewalls.insert", "projects/"+p.ID+"/global/firewalls/"+n)
		t, _ := render(Table{Cols: fwCols, Rows: []any{fwView(fw)}}, Flags{})
		return "Creating firewall...done.\n" + t, nil
	})
	reg("compute firewall-rules update", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		fw := p.Firewalls[n]
		if fw == nil {
			return nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/global/firewalls/%s' was not found", p.ID, n)
		}
		if err := c.NeedProject("compute.firewalls.update"); err != nil {
			return nil, err
		}
		if c.Has("direction") {
			return nil, fmt.Errorf("unrecognized arguments: --direction (direction cannot be changed; recreate the rule)")
		}
		if err := c.applyFW(fw, false); err != nil {
			return nil, err
		}
		c.Audit("compute.googleapis.com", "v1.compute.firewalls.patch", "projects/"+p.ID+"/global/firewalls/"+n)
		return "Updating firewall...done.\n", nil
	})
	reg("compute firewall-rules list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("compute.firewalls.list"); err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Firewalls) {
			rows = append(rows, fwView(p.Firewalls[k]))
		}
		out, _ := render(Table{Cols: fwCols, Rows: rows}, c.F)
		if c.Str("format", "") == "" {
			out += "\nTo show all fields of the firewall, please show in JSON format: --format=json\n"
		}
		return out, nil
	})
	reg("compute firewall-rules describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		fw := p.Firewalls[n]
		if fw == nil {
			return nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/global/firewalls/%s' was not found", p.ID, n)
		}
		return Obj{V: fwView(fw)}, nil
	})
	reg("compute firewall-rules delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		for _, n := range c.Args {
			if p.Firewalls[n] == nil {
				return nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/global/firewalls/%s' was not found", p.ID, n)
			}
			if err := c.NeedProject("compute.firewalls.delete"); err != nil {
				return nil, err
			}
			delete(p.Firewalls, n)
			c.Audit("compute.googleapis.com", "v1.compute.firewalls.delete", "projects/"+p.ID+"/global/firewalls/"+n)
		}
		return fmt.Sprintf("Deleted [https://www.googleapis.com/compute/v1/projects/%s/global/firewalls/%s].\n", p.ID, strings.Join(c.Args, ",")), nil
	})
	reg("compute routes create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		netName := c.Str("network", "default")
		if p.Networks[netName] == nil {
			return nil, fmt.Errorf("network %s not found", netName)
		}
		if err := c.NeedProject("compute.routes.create"); err != nil {
			return nil, err
		}
		r := &sim.Route{Name: n, Network: netName, DestRange: c.Str("destination-range", ""), Priority: c.Int("priority", 1000), Tags: c.List("tags")}
		if c.Has("next-hop-gateway") {
			r.NextHopGateway = c.Str("next-hop-gateway", "default-internet-gateway")
		}
		r.NextHopInstance = c.Str("next-hop-instance", "")
		r.NextHopIP = c.Str("next-hop-address", "")
		r.NextHopVPN = c.Str("next-hop-vpn-tunnel", "")
		if r.NextHopGateway == "" && r.NextHopInstance == "" && r.NextHopIP == "" && r.NextHopVPN == "" {
			return nil, fmt.Errorf("Exactly one of (--next-hop-address | --next-hop-gateway | --next-hop-ilb | --next-hop-instance | --next-hop-vpn-tunnel) must be specified.")
		}
		if sim.ParseCIDR(r.DestRange) == nil {
			return nil, fmt.Errorf("argument --destination-range: invalid CIDR")
		}
		p.Routes[n] = r
		c.Audit("compute.googleapis.com", "v1.compute.routes.insert", "projects/"+p.ID+"/global/routes/"+n)
		return created(p.ID, "routes", "global", n), nil
	})
	reg("compute routes list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Routes) {
			r := p.Routes[k]
			hop := r.NextHopGateway + r.NextHopInstance + r.NextHopIP + r.NextHopVPN
			if r.System {
				hop = r.NextHopGateway
			}
			rows = append(rows, map[string]any{"name": r.Name, "network": r.Network, "destRange": r.DestRange, "nextHop": hop, "priority": r.Priority, "tags": r.Tags})
		}
		for _, k := range sim.SortedKeys(p.Subnets) {
			sn := p.Subnets[k]
			rows = append(rows, map[string]any{"name": "default-route-" + sn.Name, "network": sn.Network, "destRange": sn.Range, "nextHop": sn.Network, "priority": 0})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"NETWORK", "network"}, {"DEST_RANGE", "destRange"}, {"NEXT_HOP", "nextHop"}, {"PRIORITY", "priority"}}, Rows: rows}, nil
	})
	reg("compute routes delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		for _, n := range c.Args {
			if p.Routes[n] == nil {
				return nil, fmt.Errorf("route %s not found", n)
			}
			delete(p.Routes, n)
			c.Audit("compute.googleapis.com", "v1.compute.routes.delete", "projects/"+p.ID+"/global/routes/"+n)
		}
		return "Deleted.\n", nil
	})
	reg("compute routers create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		region, err := c.RegionFlag()
		if err != nil {
			return nil, err
		}
		netName := c.Str("network", "")
		if p.Networks[netName] == nil {
			return nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/global/networks/%s' was not found", p.ID, netName)
		}
		if p.Routers[n] != nil {
			return nil, fmt.Errorf("router %s already exists", n)
		}
		if err := c.NeedProject("compute.routers.create"); err != nil {
			return nil, err
		}
		p.Routers[n] = &sim.Router{Name: n, Region: region, Network: netName, ASN: c.Int("asn", 64512)}
		c.Audit("compute.googleapis.com", "v1.compute.routers.insert", "projects/"+p.ID+"/regions/"+region+"/routers/"+n)
		return created(p.ID, "routers", "regions/"+region, n), nil
	})
	reg("compute routers list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Routers) {
			rows = append(rows, p.Routers[k])
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"REGION", "region"}, {"NETWORK", "network"}}, Rows: rows}, nil
	})
	reg("compute routers describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		r := p.Routers[n]
		if r == nil {
			return nil, fmt.Errorf("router %s not found", n)
		}
		return Obj{V: r}, nil
	})
	reg("compute routers delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		for _, n := range c.Args {
			delete(p.Routers, n)
		}
		return "Deleted.\n", nil
	})
	natUpsert := func(create bool) handler {
		return func(c *Cmd) (any, error) {
			p, err := c.P()
			if err != nil {
				return nil, err
			}
			n, err := c.name0()
			if err != nil {
				return nil, err
			}
			rn := c.Str("router", "")
			r := p.Routers[rn]
			if r == nil {
				return nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/regions/%s/routers/%s' was not found", p.ID, c.Str("region", c.S.Region), rn)
			}
			if reg := c.Str("region", c.S.Region); reg != "" && reg != r.Region {
				return nil, fmt.Errorf("router %s is in region %s, not %s", rn, r.Region, reg)
			}
			if err := c.NeedProject("compute.routers.update"); err != nil {
				return nil, err
			}
			idx := -1
			for i := range r.NATs {
				if r.NATs[i].Name == n {
					idx = i
				}
			}
			if create && idx >= 0 {
				return nil, fmt.Errorf("NAT %s already exists", n)
			}
			if !create && idx < 0 {
				return nil, fmt.Errorf("NAT %s not found", n)
			}
			nat := sim.NAT{Name: n}
			if idx >= 0 {
				nat = r.NATs[idx]
			}
			if c.Bool("nat-all-subnet-ip-ranges") {
				nat.AllSubnets, nat.Subnets = true, nil
			}
			if c.Has("nat-custom-subnet-ip-ranges") {
				nat.AllSubnets = false
				nat.Subnets = nil
				for _, s := range c.List("nat-custom-subnet-ip-ranges") {
					s = strings.Split(s, ":")[0]
					if sn := p.Subnets[s]; sn == nil || sn.Region != r.Region {
						return nil, fmt.Errorf("subnetwork %s not found in region %s", s, r.Region)
					}
					nat.Subnets = append(nat.Subnets, s)
				}
			}
			if c.Bool("auto-allocate-nat-external-ips") || c.Has("nat-external-ip-pool") {
				nat.AutoIPs = true
			}
			if c.Bool("enable-logging") {
				nat.LogErrors = true
			}
			if create && !nat.AllSubnets && len(nat.Subnets) == 0 {
				return nil, fmt.Errorf("Exactly one of (--nat-all-subnet-ip-ranges | --nat-custom-subnet-ip-ranges | --nat-primary-subnet-ip-ranges) must be specified.")
			}
			if create && !nat.AutoIPs {
				return nil, fmt.Errorf("Exactly one of (--auto-allocate-nat-external-ips | --nat-external-ip-pool) must be specified.")
			}
			if idx >= 0 {
				r.NATs[idx] = nat
			} else {
				r.NATs = append(r.NATs, nat)
			}
			c.Audit("compute.googleapis.com", "v1.compute.routers.patch", "projects/"+p.ID+"/regions/"+r.Region+"/routers/"+rn)
			return "Updating router [" + rn + "]...done.\n", nil
		}
	}
	reg("compute routers nats create", natUpsert(true))
	reg("compute routers nats update", natUpsert(false))
	reg("compute routers nats delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		r := p.Routers[c.Str("router", "")]
		if r == nil {
			return nil, fmt.Errorf("router not found")
		}
		var keep []sim.NAT
		for _, x := range r.NATs {
			if x.Name != n {
				keep = append(keep, x)
			}
		}
		r.NATs = keep
		return "Updated.\n", nil
	})
	reg("compute routers nats list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		r := p.Routers[c.Str("router", "")]
		if r == nil {
			return nil, fmt.Errorf("router not found")
		}
		var rows []any
		for _, x := range r.NATs {
			rows = append(rows, x)
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"ALL_SUBNETS", "allSubnets"}, {"SUBNETS", "subnets"}}, Rows: rows}, nil
	})
	reg("compute routers add-bgp-peer", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		r := p.Routers[n]
		if r == nil {
			return nil, fmt.Errorf("router not found")
		}
		r.BGPPeers = append(r.BGPPeers, sim.BGPPeer{Name: c.Str("peer-name", ""), PeerASN: c.Int("peer-asn", 65001), Interface: c.Str("interface", "")})
		return "Updated.\n", nil
	})
	reg("compute addresses create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("compute.addresses.create"); err != nil {
			return nil, err
		}
		a := &sim.Address{Name: n, Type: "EXTERNAL", Region: c.Str("region", "global")}
		if c.Bool("global") {
			a.Region = "global"
		}
		if pu := c.Str("purpose", ""); pu == "VPC_PEERING" {
			a.Purpose, a.Type = pu, "INTERNAL"
			a.Prefix = c.Int("prefix-length", 16)
			a.Network = c.Str("network", "default")
			a.Address = fmt.Sprintf("10.%d.0.0", 200+len(p.Addresses))
		} else if c.Has("subnet") {
			a.Type = "INTERNAL"
			sn := p.Subnets[c.Str("subnet", "")]
			if sn == nil {
				return nil, fmt.Errorf("subnetwork not found")
			}
			a.Address = c.Str("addresses", c.S.State.AllocIP(sn.Range))
		} else {
			a.Address = c.S.State.ExternalIP()
		}
		p.Addresses[n] = a
		c.Audit("compute.googleapis.com", "v1.compute.addresses.insert", "projects/"+p.ID+"/regions/"+a.Region+"/addresses/"+n)
		return created(p.ID, "addresses", a.Region, n), nil
	})
	reg("compute addresses list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Addresses) {
			a := p.Addresses[k]
			st := "RESERVED"
			if a.User != "" {
				st = "IN_USE"
			}
			rows = append(rows, map[string]any{"name": a.Name, "address": a.Address, "addressType": a.Type, "purpose": a.Purpose, "region": a.Region, "status": st, "users": a.User})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"ADDRESS/RANGE", "address"}, {"TYPE", "addressType"}, {"PURPOSE", "purpose"}, {"REGION", "region"}, {"STATUS", "status"}}, Rows: rows}, nil
	})
	reg("compute addresses describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		a := p.Addresses[n]
		if a == nil {
			return nil, fmt.Errorf("address %s not found", n)
		}
		return Obj{V: a}, nil
	})
	reg("compute addresses delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		for _, n := range c.Args {
			if a := p.Addresses[n]; a != nil && a.User != "" {
				return nil, fmt.Errorf("The address resource is in use by %s", a.User)
			}
			delete(p.Addresses, n)
		}
		return "Deleted.\n", nil
	})

	// ---- load balancing -----------------------------------------------------
	hcCreate := func(proto string) handler {
		return func(c *Cmd) (any, error) {
			p, err := c.P()
			if err != nil {
				return nil, err
			}
			n, err := c.name0()
			if err != nil {
				return nil, err
			}
			if err := c.NeedProject("compute.healthChecks.create"); err != nil {
				return nil, err
			}
			port := c.Int("port", 80)
			if proto == "HTTPS" && !c.Has("port") {
				port = 443
			}
			p.HealthChecks[n] = &sim.HealthCheck{Name: n, Protocol: proto, Port: port, RequestPath: c.Str("request-path", "/"), Interval: c.Int("check-interval", 5), Region: c.Str("region", "")}
			c.Audit("compute.googleapis.com", "v1.compute.healthChecks.insert", "projects/"+p.ID+"/global/healthChecks/"+n)
			return created(p.ID, "healthChecks", "global", n), nil
		}
	}
	reg("compute health-checks create http", hcCreate("HTTP"))
	reg("compute health-checks create https", hcCreate("HTTPS"))
	reg("compute health-checks create tcp", hcCreate("TCP"))
	reg("compute http-health-checks create", hcCreate("HTTP"))
	reg("compute health-checks update http", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		hc := p.HealthChecks[n]
		if hc == nil {
			return nil, fmt.Errorf("health check %s not found", n)
		}
		if c.Has("port") {
			hc.Port = c.Int("port", hc.Port)
		}
		if c.Has("request-path") {
			hc.RequestPath = c.Str("request-path", hc.RequestPath)
		}
		c.Audit("compute.googleapis.com", "v1.compute.healthChecks.patch", "projects/"+p.ID+"/global/healthChecks/"+n)
		return "Updated.\n", nil
	})
	reg("compute health-checks list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.HealthChecks) {
			rows = append(rows, p.HealthChecks[k])
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"PROTOCOL", "type"}, {"PORT", "port"}, {"PATH", "requestPath"}}, Rows: rows}, nil
	})
	reg("compute health-checks describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		if hc := p.HealthChecks[n]; hc != nil {
			return Obj{V: hc}, nil
		}
		return nil, fmt.Errorf("health check %s not found", n)
	})
	reg("compute health-checks delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		for _, n := range c.Args {
			delete(p.HealthChecks, n)
		}
		return "Deleted.\n", nil
	})
	reg("compute backend-services create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("compute.backendServices.create"); err != nil {
			return nil, err
		}
		hcs := c.List("health-checks")
		for _, h := range hcs {
			if p.HealthChecks[h] == nil {
				return nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/global/healthChecks/%s' was not found", p.ID, h)
			}
		}
		bs := &sim.BackendService{Name: n, Protocol: c.Str("protocol", "HTTP"), Global: c.Bool("global") || !c.Has("region"), Region: c.Str("region", ""), Scheme: c.Str("load-balancing-scheme", "EXTERNAL_MANAGED"),
			HealthChecks: hcs, PortName: c.Str("port-name", "http"), CDN: c.Bool("enable-cdn"), TimeoutSec: c.Int("timeout", 30), Logging: c.Bool("enable-logging")}
		p.BackendServices[n] = bs
		c.Audit("compute.googleapis.com", "v1.compute.backendServices.insert", "projects/"+p.ID+"/global/backendServices/"+n)
		return created(p.ID, "backendServices", "global", n), nil
	})
	reg("compute backend-services add-backend", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		bs := p.BackendServices[n]
		if bs == nil {
			return nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/global/backendServices/%s' was not found", p.ID, n)
		}
		if err := c.NeedProject("compute.backendServices.update"); err != nil {
			return nil, err
		}
		if neg := c.Str("network-endpoint-group", ""); neg != "" {
			if p.NEGs[neg] == nil {
				return nil, fmt.Errorf("network endpoint group %s not found", neg)
			}
			bs.Backends = append(bs.Backends, sim.Backend{NEG: neg})
		} else {
			g := c.Str("instance-group", "")
			ig := p.InstanceGroups[g]
			if ig == nil {
				return nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/zones/%s/instanceGroups/%s' was not found", p.ID, c.Str("instance-group-zone", ""), g)
			}
			bs.Backends = append(bs.Backends, sim.Backend{Group: g, Zone: ig.Zone, BalancingMode: c.Str("balancing-mode", "UTILIZATION")})
		}
		c.Audit("compute.googleapis.com", "v1.compute.backendServices.patch", "projects/"+p.ID+"/global/backendServices/"+n)
		return "Updated [https://www.googleapis.com/compute/v1/projects/" + p.ID + "/global/backendServices/" + n + "].\n", nil
	})
	reg("compute backend-services remove-backend", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		bs := p.BackendServices[n]
		if bs == nil {
			return nil, fmt.Errorf("backend service %s not found", n)
		}
		g := c.Str("instance-group", c.Str("network-endpoint-group", ""))
		var keep []sim.Backend
		for _, b := range bs.Backends {
			if b.Group != g && b.NEG != g {
				keep = append(keep, b)
			}
		}
		bs.Backends = keep
		return "Updated.\n", nil
	})
	reg("compute backend-services update", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		bs := p.BackendServices[n]
		if bs == nil {
			return nil, fmt.Errorf("backend service %s not found", n)
		}
		if err := c.NeedProject("compute.backendServices.update"); err != nil {
			return nil, err
		}
		if c.Has("security-policy") {
			sp := c.Str("security-policy", "")
			if sp != "" && sp != `""` && p.SecurityPolicies[sp] == nil {
				return nil, fmt.Errorf("security policy %s not found", sp)
			}
			if sp == `""` {
				sp = ""
			}
			bs.SecurityPolicy = sp
		}
		if c.Has("health-checks") {
			bs.HealthChecks = c.List("health-checks")
		}
		if c.Has("port-name") {
			bs.PortName = c.Str("port-name", "")
		}
		if c.Bool("enable-cdn") {
			bs.CDN = true
		}
		if c.Bool("no-enable-cdn") {
			bs.CDN = false
		}
		if c.Bool("enable-logging") {
			bs.Logging = true
		}
		c.Audit("compute.googleapis.com", "v1.compute.backendServices.patch", "projects/"+p.ID+"/global/backendServices/"+n)
		return "Updated [https://www.googleapis.com/compute/v1/projects/" + p.ID + "/global/backendServices/" + n + "].\n", nil
	})
	reg("compute backend-services list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.BackendServices) {
			bs := p.BackendServices[k]
			var bes []string
			for _, b := range bs.Backends {
				bes = append(bes, b.Group+b.NEG)
			}
			rows = append(rows, map[string]any{"name": bs.Name, "backends": bes, "protocol": bs.Protocol, "securityPolicy": bs.SecurityPolicy})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"BACKENDS", "backends"}, {"PROTOCOL", "protocol"}}, Rows: rows}, nil
	})
	reg("compute backend-services describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		if bs := p.BackendServices[n]; bs != nil {
			return Obj{V: bs}, nil
		}
		return nil, fmt.Errorf("backend service %s not found", n)
	})
	reg("compute backend-services get-health", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		bs := p.BackendServices[n]
		if bs == nil {
			return nil, fmt.Errorf("backend service %s not found", n)
		}
		var b strings.Builder
		for _, h := range c.S.State.BackendHealth(p.ID, bs) {
			st := "UNHEALTHY"
			if h.Healthy {
				st = "HEALTHY"
			}
			ip := ""
			if vm := p.Instances[h.Instance]; vm != nil {
				ip = vm.InternalIP
			}
			b.WriteString(fmt.Sprintf("---\nbackend: %s\nstatus:\n  healthStatus:\n  - healthState: %s\n    instance: projects/%s/instances/%s\n    ipAddress: %s\n", h.Instance, st, p.ID, h.Instance, ip))
			if !h.Healthy {
				b.WriteString("    # sim-diagnostic: " + h.Reason + "\n")
			}
		}
		if b.Len() == 0 {
			return "---\nbackend service has no backends\n", nil
		}
		return b.String(), nil
	})
	reg("compute backend-services delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		for _, n := range c.Args {
			delete(p.BackendServices, n)
		}
		return "Deleted.\n", nil
	})
	reg("compute network-endpoint-groups create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		svc := c.Str("cloud-run-service", "")
		if svc == "" {
			return nil, fmt.Errorf("only serverless NEGs (--network-endpoint-type=serverless --cloud-run-service) are supported by the simulator")
		}
		p.NEGs[n] = &sim.NEG{Name: n, Region: c.Str("region", c.S.Region), Type: "SERVERLESS", RunService: svc}
		return created(p.ID, "networkEndpointGroups", "regions/"+c.Str("region", c.S.Region), n), nil
	})
	reg("compute network-endpoint-groups list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.NEGs) {
			rows = append(rows, p.NEGs[k])
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"LOCATION", "region"}, {"ENDPOINT_TYPE", "networkEndpointType"}}, Rows: rows}, nil
	})
	reg("compute url-maps create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		ds := c.Str("default-service", "")
		if p.BackendServices[ds] == nil {
			return nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/global/backendServices/%s' was not found", p.ID, ds)
		}
		if err := c.NeedProject("compute.urlMaps.create"); err != nil {
			return nil, err
		}
		p.URLMaps[n] = &sim.URLMap{Name: n, DefaultService: ds}
		c.Audit("compute.googleapis.com", "v1.compute.urlMaps.insert", "projects/"+p.ID+"/global/urlMaps/"+n)
		return created(p.ID, "urlMaps", "global", n), nil
	})
	reg("compute url-maps add-path-matcher", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		um := p.URLMaps[n]
		if um == nil {
			return nil, fmt.Errorf("url map %s not found", n)
		}
		pm := sim.PathMatcher{Name: c.Str("path-matcher-name", ""), DefaultService: c.Str("default-service", um.DefaultService)}
		for _, rule := range c.List("path-rules") {
			paths, svc, ok := strings.Cut(rule, "=")
			if !ok {
				return nil, fmt.Errorf("Invalid --path-rules %q (PATH=SERVICE)", rule)
			}
			if p.BackendServices[svc] == nil {
				return nil, fmt.Errorf("backend service %s not found", svc)
			}
			pm.PathRules = append(pm.PathRules, sim.PathRule{Paths: []string{paths}, Service: svc})
		}
		um.PathMatchers = append(um.PathMatchers, pm)
		hosts := c.List("new-hosts")
		if len(hosts) == 0 {
			hosts = []string{"*"}
		}
		um.HostRules = append(um.HostRules, sim.HostRule{Hosts: hosts, PathMatcher: pm.Name})
		c.Audit("compute.googleapis.com", "v1.compute.urlMaps.patch", "projects/"+p.ID+"/global/urlMaps/"+n)
		return "Updated.\n", nil
	})
	reg("compute url-maps set-default-service", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		um := p.URLMaps[n]
		if um == nil {
			return nil, fmt.Errorf("url map %s not found", n)
		}
		um.DefaultService = c.Str("default-service", um.DefaultService)
		return "Updated.\n", nil
	})
	reg("compute url-maps list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.URLMaps) {
			rows = append(rows, p.URLMaps[k])
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"DEFAULT_SERVICE", "defaultService"}}, Rows: rows}, nil
	})
	reg("compute url-maps describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		if um := p.URLMaps[n]; um != nil {
			return Obj{V: um}, nil
		}
		return nil, fmt.Errorf("url map %s not found", n)
	})
	proxyCreate := func(kind string) handler {
		return func(c *Cmd) (any, error) {
			p, err := c.P()
			if err != nil {
				return nil, err
			}
			n, err := c.name0()
			if err != nil {
				return nil, err
			}
			um := c.Str("url-map", "")
			if p.URLMaps[um] == nil {
				return nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/global/urlMaps/%s' was not found", p.ID, um)
			}
			if kind == "https" && !c.Has("ssl-certificates") {
				return nil, fmt.Errorf("argument --ssl-certificates: Must be specified.")
			}
			p.TargetProxies[n] = &sim.TargetProxy{Name: n, Kind: kind, URLMap: um, SSLCerts: c.List("ssl-certificates")}
			c.Audit("compute.googleapis.com", "v1.compute.targetHttpProxies.insert", "projects/"+p.ID+"/global/targetHttpProxies/"+n)
			return created(p.ID, "target"+strings.Title(kind)+"Proxies", "global", n), nil
		}
	}
	reg("compute target-http-proxies create", proxyCreate("http"))
	reg("compute target-https-proxies create", proxyCreate("https"))
	reg("compute ssl-certificates create", func(c *Cmd) (any, error) {
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		return created(c.ProjectID(), "sslCertificates", "global", n), nil
	})
	reg("compute target-http-proxies list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.TargetProxies) {
			rows = append(rows, p.TargetProxies[k])
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"URL_MAP", "urlMap"}}, Rows: rows}, nil
	})
	reg("compute forwarding-rules create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("compute.forwardingRules.create"); err != nil {
			return nil, err
		}
		fr := &sim.ForwardingRule{Name: n, Global: c.Bool("global"), Region: c.Str("region", ""), Scheme: c.Str("load-balancing-scheme", "EXTERNAL_MANAGED")}
		tp := c.Str("target-http-proxy", c.Str("target-https-proxy", ""))
		if tp != "" {
			if p.TargetProxies[tp] == nil {
				return nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/global/targetHttpProxies/%s' was not found", p.ID, tp)
			}
			fr.Target = tp
		} else if bs := c.Str("backend-service", ""); bs != "" {
			if p.BackendServices[bs] == nil {
				return nil, fmt.Errorf("backend service %s not found", bs)
			}
			fr.BackendService = bs
		} else {
			return nil, fmt.Errorf("One of --target-http-proxy, --target-https-proxy or --backend-service must be specified.")
		}
		pr := c.Str("ports", c.Str("port-range", "80"))
		fr.Ports = strings.Split(pr, ",")
		if a := c.Str("address", ""); a != "" {
			if addr := p.Addresses[a]; addr != nil {
				fr.IP = addr.Address
				addr.User = n
			} else {
				fr.IP = a
			}
		} else {
			fr.IP = c.S.State.ExternalIP()
		}
		if fr.Scheme == "INTERNAL" || fr.Scheme == "INTERNAL_MANAGED" {
			fr.Network = c.Str("network", "default")
			fr.Subnet = c.Str("subnet", "")
		}
		p.ForwardingRules[n] = fr
		c.Audit("compute.googleapis.com", "v1.compute.globalForwardingRules.insert", "projects/"+p.ID+"/global/forwardingRules/"+n)
		return created(p.ID, "forwardingRules", "global", n), nil
	})
	reg("compute forwarding-rules list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.ForwardingRules) {
			fr := p.ForwardingRules[k]
			rows = append(rows, map[string]any{"name": fr.Name, "region": fr.Region, "IPAddress": fr.IP, "IPProtocol": "TCP", "target": fr.Target + fr.BackendService, "portRange": strings.Join(fr.Ports, ",")})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"REGION", "region"}, {"IP_ADDRESS", "IPAddress"}, {"IP_PROTOCOL", "IPProtocol"}, {"TARGET", "target"}}, Rows: rows}, nil
	})
	reg("compute forwarding-rules describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		if fr := p.ForwardingRules[n]; fr != nil {
			return Obj{V: fr}, nil
		}
		return nil, fmt.Errorf("forwarding rule %s not found", n)
	})
	reg("compute forwarding-rules delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		for _, n := range c.Args {
			delete(p.ForwardingRules, n)
		}
		return "Deleted.\n", nil
	})

	// ---- Cloud Armor ---------------------------------------------------------
	reg("compute security-policies create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("compute.securityPolicies.create"); err != nil {
			return nil, err
		}
		p.SecurityPolicies[n] = &sim.SecurityPolicy{Name: n, Rules: []sim.SPRule{{Priority: 2147483647, Action: "allow", SrcIPRanges: []string{"*"}, Description: "default rule"}}}
		c.Audit("compute.googleapis.com", "v1.compute.securityPolicies.insert", "projects/"+p.ID+"/global/securityPolicies/"+n)
		return created(p.ID, "securityPolicies", "global", n), nil
	})
	reg("compute security-policies rules create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		prio, err := c.Arg(0, "PRIORITY")
		if err != nil {
			return nil, err
		}
		pn := c.Str("security-policy", "")
		sp := p.SecurityPolicies[pn]
		if sp == nil {
			return nil, fmt.Errorf("security policy %s not found", pn)
		}
		n, _ := strconv.Atoi(prio)
		for _, r := range sp.Rules {
			if r.Priority == n {
				return nil, fmt.Errorf("A rule with priority %d already exists", n)
			}
		}
		act := strings.ToLower(c.Str("action", ""))
		if act == "" {
			return nil, fmt.Errorf("argument --action: Must be specified.")
		}
		sp.Rules = append(sp.Rules, sim.SPRule{Priority: n, Action: act, SrcIPRanges: c.List("src-ip-ranges"), Expression: c.Str("expression", ""), Preview: c.Bool("preview"), Description: c.Str("description", "")})
		c.Audit("compute.googleapis.com", "v1.compute.securityPolicies.addRule", "projects/"+p.ID+"/global/securityPolicies/"+pn)
		return "Updated.\n", nil
	})
	reg("compute security-policies rules update", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		prio, _ := c.Arg(0, "PRIORITY")
		sp := p.SecurityPolicies[c.Str("security-policy", "")]
		if sp == nil {
			return nil, fmt.Errorf("security policy not found")
		}
		n, _ := strconv.Atoi(prio)
		for i := range sp.Rules {
			if sp.Rules[i].Priority == n {
				if c.Has("action") {
					sp.Rules[i].Action = strings.ToLower(c.Str("action", ""))
				}
				if c.Bool("no-preview") {
					sp.Rules[i].Preview = false
				}
				if c.Bool("preview") {
					sp.Rules[i].Preview = true
				}
				if c.Has("src-ip-ranges") {
					sp.Rules[i].SrcIPRanges = c.List("src-ip-ranges")
				}
				return "Updated.\n", nil
			}
		}
		return nil, fmt.Errorf("rule %d not found", n)
	})
	reg("compute security-policies rules delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		prio, _ := c.Arg(0, "PRIORITY")
		sp := p.SecurityPolicies[c.Str("security-policy", "")]
		if sp == nil {
			return nil, fmt.Errorf("security policy not found")
		}
		n, _ := strconv.Atoi(prio)
		var keep []sim.SPRule
		for _, r := range sp.Rules {
			if r.Priority != n {
				keep = append(keep, r)
			}
		}
		sp.Rules = keep
		return "Deleted.\n", nil
	})
	reg("compute security-policies list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.SecurityPolicies) {
			rows = append(rows, p.SecurityPolicies[k])
		}
		return Table{Cols: []Col{{"NAME", "name"}}, Rows: rows}, nil
	})
	reg("compute security-policies describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		if sp := p.SecurityPolicies[n]; sp != nil {
			return Obj{V: sp}, nil
		}
		return nil, fmt.Errorf("security policy %s not found", n)
	})

	// ---- VPN (hybrid connectivity) -----------------------------------------
	reg("compute vpn-gateways create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		region, err := c.RegionFlag()
		if err != nil {
			return nil, err
		}
		p.VPNGateways[n] = &sim.VPNGateway{Name: n, Region: region, Network: c.Str("network", "default"), HA: true}
		return created(p.ID, "vpnGateways", "regions/"+region, n), nil
	})
	reg("compute vpn-tunnels create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		gw := p.VPNGateways[c.Str("vpn-gateway", "")]
		if gw == nil {
			return nil, fmt.Errorf("vpn gateway %s not found", c.Str("vpn-gateway", ""))
		}
		if p.Routers[c.Str("router", "")] == nil {
			return nil, fmt.Errorf("router %s not found (HA VPN requires a Cloud Router for BGP)", c.Str("router", ""))
		}
		gw.Tunnels = append(gw.Tunnels, sim.VPNTunnel{Name: n, PeerIP: c.Str("peer-address", c.Str("peer-external-gateway", "")), Router: c.Str("router", ""), Status: "ESTABLISHED", Interface: c.Int("interface", 0)})
		return created(p.ID, "vpnTunnels", "regions/"+gw.Region, n), nil
	})
	reg("compute vpn-tunnels list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.VPNGateways) {
			for _, t := range p.VPNGateways[k].Tunnels {
				rows = append(rows, map[string]any{"name": t.Name, "gateway": k, "peerIp": t.PeerIP, "status": t.Status})
			}
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"GATEWAY", "gateway"}, {"PEER_ADDRESS", "peerIp"}, {"STATUS", "status"}}, Rows: rows}, nil
	})
	reg("compute networks vpc-access connectors create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		netName := c.Str("network", "default")
		if p.Networks[netName] == nil {
			return nil, fmt.Errorf("network %s not found", netName)
		}
		c.S.State.Extra["connector:"+p.ID+"/"+n] = netName
		return "Create request issued for: [" + n + "]\nCreated connector [" + n + "].\n", nil
	})
	reg("compute networks vpc-access connectors list", func(c *Cmd) (any, error) {
		var rows []any
		for k, v := range c.S.State.Extra {
			if strings.HasPrefix(k, "connector:"+c.ProjectID()+"/") {
				rows = append(rows, map[string]any{"name": strings.TrimPrefix(k, "connector:"+c.ProjectID()+"/"), "network": v, "state": "READY"})
			}
		}
		return Table{Cols: []Col{{"CONNECTOR_ID", "name"}, {"NETWORK", "network"}, {"STATE", "state"}}, Rows: rows}, nil
	})

	// ---- Cloud DNS ---------------------------------------------------------
	reg("dns managed-zones create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		dn := c.Str("dns-name", "")
		if !strings.HasSuffix(dn, ".") {
			dn += "."
		}
		if dn == "." {
			return nil, fmt.Errorf("argument --dns-name: Must be specified.")
		}
		if err := c.NeedProject("dns.managedZones.create"); err != nil {
			return nil, err
		}
		vis := c.Str("visibility", "public")
		z := &sim.DNSZone{Name: n, DNSName: dn, Visibility: vis, Description: c.Str("description", ""), DNSSEC: c.Str("dnssec-state", "") == "on"}
		if vis == "private" {
			for _, nw := range c.List("networks") {
				nw = nw[strings.LastIndex(nw, "/")+1:]
				if p.Networks[nw] == nil {
					return nil, fmt.Errorf("network %s not found", nw)
				}
				z.Networks = append(z.Networks, nw)
			}
		}
		p.DNSZones[n] = z
		c.Audit("dns.googleapis.com", "dns.managedZones.create", "projects/"+p.ID+"/managedZones/"+n)
		return fmt.Sprintf("Created [https://dns.googleapis.com/dns/v1/projects/%s/managedZones/%s].\n", p.ID, n), nil
	})
	reg("dns managed-zones list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.DNSZones) {
			rows = append(rows, p.DNSZones[k])
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"DNS_NAME", "dnsName"}, {"DESCRIPTION", "description"}, {"VISIBILITY", "visibility"}}, Rows: rows}, nil
	})
	reg("dns managed-zones describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		if z := p.DNSZones[n]; z != nil {
			return Obj{V: map[string]any{"name": z.Name, "dnsName": z.DNSName, "visibility": z.Visibility, "nameServers": []string{"ns-cloud-a1.googledomains.com.", "ns-cloud-a2.googledomains.com."}}}, nil
		}
		return nil, fmt.Errorf("managed zone %s not found", n)
	})
	reg("dns managed-zones delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		z := p.DNSZones[n]
		if z == nil {
			return nil, fmt.Errorf("managed zone %s not found", n)
		}
		if len(z.Records) > 0 {
			return nil, fmt.Errorf("The resource named '%s' cannot be deleted because it is not empty", n)
		}
		delete(p.DNSZones, n)
		return "Deleted.\n", nil
	})
	rrUpsert := func(mode string) handler {
		return func(c *Cmd) (any, error) {
			p, err := c.P()
			if err != nil {
				return nil, err
			}
			name, err := c.Arg(0, "DNS_NAME")
			if err != nil {
				return nil, err
			}
			if !strings.HasSuffix(name, ".") {
				name += "."
			}
			zn := c.Str("zone", "")
			z := p.DNSZones[zn]
			if z == nil {
				return nil, fmt.Errorf("managed zone %s not found", zn)
			}
			if !strings.HasSuffix(name, z.DNSName) {
				return nil, fmt.Errorf("HTTPError 400: The resource 'entity.change.additions[%s]' named '%s' does not belong to zone %s (%s)", name, name, zn, z.DNSName)
			}
			typ := strings.ToUpper(c.Str("type", "A"))
			if err := c.NeedProject("dns.changes.create"); err != nil {
				return nil, err
			}
			idx := -1
			for i, r := range z.Records {
				if r.Name == name && r.Type == typ {
					idx = i
				}
			}
			switch mode {
			case "create":
				if idx >= 0 {
					return nil, fmt.Errorf("HTTPError 409: The resource '%s' of type %s already exists", name, typ)
				}
				z.Records = append(z.Records, sim.DNSRecord{Name: name, Type: typ, TTL: c.Int("ttl", 300), Data: c.List("rrdatas")})
			case "update":
				if idx < 0 {
					return nil, fmt.Errorf("HTTPError 404: record %s %s not found", name, typ)
				}
				z.Records[idx].Data = c.List("rrdatas")
				z.Records[idx].TTL = c.Int("ttl", z.Records[idx].TTL)
			case "delete":
				if idx < 0 {
					return nil, fmt.Errorf("HTTPError 404: record %s %s not found", name, typ)
				}
				z.Records = append(z.Records[:idx], z.Records[idx+1:]...)
			}
			c.Audit("dns.googleapis.com", "dns.changes.create", "projects/"+p.ID+"/managedZones/"+zn)
			return fmt.Sprintf("NAME  TYPE  TTL  DATA\n%s  %s  %d  %s\n", name, typ, c.Int("ttl", 300), strings.Join(c.List("rrdatas"), ",")), nil
		}
	}
	reg("dns record-sets create", rrUpsert("create"))
	reg("dns record-sets update", rrUpsert("update"))
	reg("dns record-sets delete", rrUpsert("delete"))
	reg("dns record-sets list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		z := p.DNSZones[c.Str("zone", "")]
		if z == nil {
			return nil, fmt.Errorf("managed zone %s not found", c.Str("zone", ""))
		}
		rows := []any{map[string]any{"name": z.DNSName, "type": "NS", "ttl": 21600, "rrdatas": []string{"ns-cloud-a1.googledomains.com."}}, map[string]any{"name": z.DNSName, "type": "SOA", "ttl": 21600, "rrdatas": []string{"ns-cloud-a1.googledomains.com. cloud-dns-hostmaster.google.com. 1 21600 3600 259200 300"}}}
		for _, r := range z.Records {
			rows = append(rows, r)
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"TYPE", "type"}, {"TTL", "ttl"}, {"DATA", "rrdatas"}}, Rows: rows}, nil
	})
}

var fwCols = []Col{{"NAME", "name"}, {"NETWORK", "network"}, {"DIRECTION", "direction"}, {"PRIORITY", "priority"}, {"ALLOW", "allowStr"}, {"DENY", "denyStr"}, {"SRC_RANGES", "sourceRanges"}, {"SRC_TAGS", "sourceTags"}, {"TARGET_TAGS", "targetTags"}, {"DISABLED", "disabled"}}
