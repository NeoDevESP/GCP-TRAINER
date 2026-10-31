package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
	"gopkg.in/yaml.v3"
)

// Commands that banks use every day: VPC Service Controls perimeters, IAM deny
// policies and the hybrid side of Cloud Router (BGP status, advertisements).

func init() {
	// ---- VPC Service Controls ---------------------------------------------------
	perimeter := func(c *Cmd) (*sim.Project, *sim.Perimeter, error) {
		p, err := c.P()
		if err != nil {
			return nil, nil, err
		}
		n, err := c.Arg(0, "PERIMETER")
		if err != nil {
			return nil, nil, err
		}
		n = n[strings.LastIndex(n, "/")+1:]
		per := p.Perimeters[n]
		if per == nil {
			return p, nil, fmt.Errorf("NOT_FOUND: Service perimeter accessPolicies/zentra/servicePerimeters/%s not found", n)
		}
		return p, per, nil
	}
	readIngress := func(c *Cmd, flag string) ([]sim.IngressPolicy, error) {
		f := c.Str(flag, "")
		raw, ok := c.S.Files[c.S.path(f)]
		if !ok {
			return nil, fmt.Errorf("Unable to read file [%s]", f)
		}
		var out []sim.IngressPolicy
		if err := yaml.Unmarshal([]byte(raw), &out); err != nil {
			return nil, fmt.Errorf("Invalid YAML in [%s]: %v", f, err)
		}
		for _, in := range out {
			if len(in.IngressFrom.Sources) == 0 || len(in.IngressTo.Operations) == 0 {
				return nil, fmt.Errorf("INVALID_ARGUMENT: every ingress policy needs ingressFrom.sources and ingressTo.operations")
			}
		}
		return out, nil
	}
	// applyConfig updates a perimeter configuration from the flags.
	applyConfig := func(c *Cmd, cfg *sim.PerimeterConfig) error {
		add := func(list []string, vals []string) []string {
			for _, v := range vals {
				if !inStrings(list, v) {
					list = append(list, v)
				}
			}
			return list
		}
		rm := func(list []string, vals []string) []string {
			var out []string
			for _, v := range list {
				if !inStrings(vals, v) {
					out = append(out, v)
				}
			}
			return out
		}
		if c.Has("resources") {
			cfg.Resources = c.List("resources")
		}
		cfg.Resources = rm(add(cfg.Resources, c.List("add-resources")), c.List("remove-resources"))
		if c.Has("restricted-services") {
			cfg.RestrictedServices = c.List("restricted-services")
		}
		cfg.RestrictedServices = rm(add(cfg.RestrictedServices, c.List("add-restricted-services")), c.List("remove-restricted-services"))
		if c.Has("ingress-policies") || c.Has("set-ingress-policies") {
			fl := "ingress-policies"
			if c.Has("set-ingress-policies") {
				fl = "set-ingress-policies"
			}
			in, err := readIngress(c, fl)
			if err != nil {
				return err
			}
			cfg.IngressPolicies = in
		}
		if c.Bool("clear-ingress-policies") {
			cfg.IngressPolicies = nil
		}
		return nil
	}
	view := func(per *sim.Perimeter, dry bool) any {
		cfg := per.Status
		if dry {
			cfg = per.Spec
		}
		m := map[string]any{"name": "accessPolicies/zentra/servicePerimeters/" + per.Name, "title": per.Title, "perimeterType": "PERIMETER_TYPE_REGULAR", "useExplicitDryRunSpec": per.DryRun}
		key := "status"
		if dry {
			key = "spec"
		}
		if cfg != nil {
			m[key] = cfg
		}
		return Obj{V: m}
	}
	reg("access-context-manager policies list", func(c *Cmd) (any, error) {
		return Table{Cols: []Col{{"NAME", "name"}, {"ORGANIZATION", "org"}, {"TITLE", "title"}}, Rows: []any{map[string]any{"name": "zentra", "org": "organizations/" + c.S.State.Org.ID, "title": "Zentra"}}}, nil
	})
	reg("access-context-manager perimeters list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, n := range sim.SortedKeys(p.Perimeters) {
			per := p.Perimeters[n]
			mode := "ENFORCED"
			if per.Status == nil {
				mode = "DRY_RUN"
			} else if per.Spec != nil {
				mode = "ENFORCED + DRY_RUN"
			}
			rows = append(rows, map[string]any{"name": n, "title": per.Title, "mode": mode})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"TITLE", "title"}, {"MODE", "mode"}}, Rows: rows}, nil
	})
	reg("access-context-manager perimeters describe", func(c *Cmd) (any, error) {
		_, per, err := perimeter(c)
		if err != nil {
			return nil, err
		}
		m := view(per, false).(Obj).V.(map[string]any)
		if per.Spec != nil {
			m["spec"] = per.Spec
		}
		return Obj{V: m}, nil
	})
	reg("access-context-manager perimeters create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.Arg(0, "PERIMETER")
		if err != nil {
			return nil, err
		}
		if p.Perimeters[n] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: perimeter %s already exists", n)
		}
		cfg := &sim.PerimeterConfig{}
		if err := applyConfig(c, cfg); err != nil {
			return nil, err
		}
		p.Perimeters[n] = &sim.Perimeter{Name: n, Title: c.Str("title", n), Status: cfg}
		c.Audit("accesscontextmanager.googleapis.com", "CreateServicePerimeter", "accessPolicies/zentra/servicePerimeters/"+n)
		return fmt.Sprintf("Created [%s].\n", n), nil
	})
	reg("access-context-manager perimeters update", func(c *Cmd) (any, error) {
		_, per, err := perimeter(c)
		if err != nil {
			return nil, err
		}
		if per.Status == nil {
			per.Status = &sim.PerimeterConfig{}
		}
		if err := applyConfig(c, per.Status); err != nil {
			return nil, err
		}
		c.Audit("accesscontextmanager.googleapis.com", "UpdateServicePerimeter", "accessPolicies/zentra/servicePerimeters/"+per.Name)
		return fmt.Sprintf("Updated [%s].\n", per.Name), nil
	})
	reg("access-context-manager perimeters delete", func(c *Cmd) (any, error) {
		p, per, err := perimeter(c)
		if err != nil {
			return nil, err
		}
		delete(p.Perimeters, per.Name)
		c.Audit("accesscontextmanager.googleapis.com", "DeleteServicePerimeter", "accessPolicies/zentra/servicePerimeters/"+per.Name)
		return fmt.Sprintf("Deleted [%s].\n", per.Name), nil
	})
	reg("access-context-manager perimeters dry-run create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.Arg(0, "PERIMETER")
		if err != nil {
			return nil, err
		}
		cfg := &sim.PerimeterConfig{}
		if err := applyConfig(c, cfg); err != nil {
			return nil, err
		}
		if per := p.Perimeters[n]; per != nil {
			per.Spec, per.DryRun = cfg, true
		} else {
			p.Perimeters[n] = &sim.Perimeter{Name: n, Title: c.Str("title", n), Spec: cfg, DryRun: true}
		}
		c.Audit("accesscontextmanager.googleapis.com", "UpdateServicePerimeter", "accessPolicies/zentra/servicePerimeters/"+n)
		return fmt.Sprintf("Created dry-run configuration for [%s].\n", n), nil
	})
	reg("access-context-manager perimeters dry-run update", func(c *Cmd) (any, error) {
		_, per, err := perimeter(c)
		if err != nil {
			return nil, err
		}
		if per.Spec == nil {
			cp := sim.PerimeterConfig{}
			if per.Status != nil {
				b, _ := json.Marshal(per.Status)
				json.Unmarshal(b, &cp)
			}
			per.Spec, per.DryRun = &cp, true
		}
		if err := applyConfig(c, per.Spec); err != nil {
			return nil, err
		}
		c.Audit("accesscontextmanager.googleapis.com", "UpdateServicePerimeter", "accessPolicies/zentra/servicePerimeters/"+per.Name)
		return fmt.Sprintf("Updated dry-run configuration for [%s].\n", per.Name), nil
	})
	reg("access-context-manager perimeters dry-run describe", func(c *Cmd) (any, error) {
		_, per, err := perimeter(c)
		if err != nil {
			return nil, err
		}
		if per.Spec == nil {
			return nil, fmt.Errorf("perimeter %s has no dry-run configuration", per.Name)
		}
		return view(per, true), nil
	})
	reg("access-context-manager perimeters dry-run enforce", func(c *Cmd) (any, error) {
		_, per, err := perimeter(c)
		if err != nil {
			return nil, err
		}
		if per.Spec == nil {
			return nil, fmt.Errorf("perimeter %s has no dry-run configuration to enforce", per.Name)
		}
		per.Status, per.Spec, per.DryRun = per.Spec, nil, false
		c.Audit("accesscontextmanager.googleapis.com", "UpdateServicePerimeter", "accessPolicies/zentra/servicePerimeters/"+per.Name)
		return fmt.Sprintf("Enforced the dry-run configuration of [%s].\n", per.Name), nil
	})
	reg("access-context-manager perimeters dry-run drop", func(c *Cmd) (any, error) {
		_, per, err := perimeter(c)
		if err != nil {
			return nil, err
		}
		per.Spec, per.DryRun = nil, false
		return fmt.Sprintf("Dropped the dry-run configuration of [%s].\n", per.Name), nil
	})

	// ---- IAM deny policies -------------------------------------------------------
	attach := func(c *Cmd) (*sim.Project, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		ap := c.Str("attachment-point", "")
		if ap == "" {
			return nil, fmt.Errorf("argument --attachment-point: Must be specified.")
		}
		ap, _ = urlUnescape(ap)
		pid := ap[strings.LastIndex(ap, "/")+1:]
		if pid != p.ID && pid != p.Number {
			return nil, fmt.Errorf("NOT_FOUND: attachment point %s not found", ap)
		}
		if k := c.Str("kind", "denypolicies"); k != "denypolicies" {
			return nil, fmt.Errorf("INVALID_ARGUMENT: only --kind=denypolicies is supported")
		}
		if p.DenyPolicies == nil {
			p.DenyPolicies = map[string]*sim.DenyPolicy{}
		}
		return p, nil
	}
	readDeny := func(c *Cmd, name string) (*sim.DenyPolicy, error) {
		f := c.Str("policy-file", "")
		raw, ok := c.S.Files[c.S.path(f)]
		if !ok {
			return nil, fmt.Errorf("Unable to read file [%s]", f)
		}
		dp := &sim.DenyPolicy{}
		if err := yaml.Unmarshal([]byte(raw), dp); err != nil {
			return nil, fmt.Errorf("Invalid policy file [%s]: %v", f, err)
		}
		dp.Name = name
		return dp, nil
	}
	denyRes := func(p *sim.Project) sim.Resource { return sim.ProjectResource(p.ID) }
	reg("iam policies list", func(c *Cmd) (any, error) {
		p, err := attach(c)
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, n := range sim.SortedKeys(p.DenyPolicies) {
			rows = append(rows, map[string]any{"name": n, "displayName": p.DenyPolicies[n].DisplayName, "kind": "DenyPolicy"})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"DISPLAY_NAME", "displayName"}, {"KIND", "kind"}}, Rows: rows}, nil
	})
	reg("iam policies get", func(c *Cmd) (any, error) {
		p, err := attach(c)
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "POLICY_ID")
		dp := p.DenyPolicies[n]
		if dp == nil {
			return nil, fmt.Errorf("NOT_FOUND: deny policy %s not found", n)
		}
		return Obj{V: map[string]any{"name": "policies/cloudresourcemanager.googleapis.com%2Fprojects%2F" + p.ID + "/denypolicies/" + n, "displayName": dp.DisplayName, "rules": dp.Rules, "kind": "DenyPolicy"}}, nil
	})
	upsert := func(create bool) handler {
		return func(c *Cmd) (any, error) {
			p, err := attach(c)
			if err != nil {
				return nil, err
			}
			n, err := c.Arg(0, "POLICY_ID")
			if err != nil {
				return nil, err
			}
			if err := c.Need("iam.denypolicies.create", denyRes(p)); err != nil {
				return nil, err
			}
			if create && p.DenyPolicies[n] != nil {
				return nil, fmt.Errorf("ALREADY_EXISTS: deny policy %s already exists", n)
			}
			if !create && p.DenyPolicies[n] == nil {
				return nil, fmt.Errorf("NOT_FOUND: deny policy %s not found", n)
			}
			dp, err := readDeny(c, n)
			if err != nil {
				return nil, err
			}
			p.DenyPolicies[n] = dp
			c.Audit("iam.googleapis.com", "google.iam.v2.Policies.CreatePolicy", "policies/cloudresourcemanager.googleapis.com/projects/"+p.ID+"/denypolicies/"+n)
			return fmt.Sprintf("Updated deny policy [%s].\n", n), nil
		}
	}
	reg("iam policies create", upsert(true))
	reg("iam policies update", upsert(false))
	reg("iam policies delete", func(c *Cmd) (any, error) {
		p, err := attach(c)
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "POLICY_ID")
		if p.DenyPolicies[n] == nil {
			return nil, fmt.Errorf("NOT_FOUND: deny policy %s not found", n)
		}
		if err := c.Need("iam.denypolicies.delete", denyRes(p)); err != nil {
			return nil, err
		}
		delete(p.DenyPolicies, n)
		c.Audit("iam.googleapis.com", "google.iam.v2.Policies.DeletePolicy", "policies/cloudresourcemanager.googleapis.com/projects/"+p.ID+"/denypolicies/"+n)
		return fmt.Sprintf("Deleted deny policy [%s].\n", n), nil
	})

	// ---- Cloud Router: BGP and advertisements --------------------------------------
	router := func(c *Cmd) (*sim.Project, *sim.Router, error) {
		p, err := c.P()
		if err != nil {
			return nil, nil, err
		}
		n, err := c.Arg(0, "NAME")
		if err != nil {
			return nil, nil, err
		}
		r := p.Routers[n]
		if r == nil {
			return nil, nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/regions/%s/routers/%s' was not found", p.ID, c.Str("region", c.S.Region), n)
		}
		return p, r, nil
	}
	reg("compute routers get-status", func(c *Cmd) (any, error) {
		p, r, err := router(c)
		if err != nil {
			return nil, err
		}
		peers := c.S.State.BGPStatus(p.ID, r)
		var best []any
		for _, ps := range peers {
			for _, l := range ps.Learned {
				best = append(best, map[string]any{"destRange": l, "nextHopIp": "169.254.0.2", "priority": 100, "routeType": "BGP"})
			}
		}
		return Obj{V: map[string]any{"kind": "compute#routerStatusResponse", "result": map[string]any{"network": r.Network, "bgpPeerStatus": peers, "bestRoutes": best}}}, nil
	})
	reg("compute routers update", func(c *Cmd) (any, error) {
		p, r, err := router(c)
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("compute.routers.update"); err != nil {
			return nil, err
		}
		if !routerAdvertisementFlags(c, r) {
			return nil, fmt.Errorf("at least one property must be modified")
		}
		if strings.EqualFold(r.AdvertiseMode, "CUSTOM") && len(r.AdvertiseGroups) == 0 && len(r.AdvertisedRanges) == 0 {
			return nil, fmt.Errorf("INVALID_ARGUMENT: custom advertisement mode needs groups or ranges")
		}
		c.Audit("compute.googleapis.com", "v1.compute.routers.patch", "projects/"+p.ID+"/regions/"+r.Region+"/routers/"+r.Name)
		return fmt.Sprintf("Updated [https://www.googleapis.com/compute/v1/projects/%s/regions/%s/routers/%s].\n", p.ID, r.Region, r.Name), nil
	})
	reg("compute routers update-bgp-peer", func(c *Cmd) (any, error) {
		p, r, err := router(c)
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("compute.routers.update"); err != nil {
			return nil, err
		}
		name := c.Str("peer-name", "")
		for i := range r.BGPPeers {
			if r.BGPPeers[i].Name == name {
				if c.Has("peer-asn") {
					r.BGPPeers[i].PeerASN = c.Int("peer-asn", r.BGPPeers[i].PeerASN)
				}
				if c.Has("interface") {
					r.BGPPeers[i].Interface = c.Str("interface", "")
				}
				c.Audit("compute.googleapis.com", "v1.compute.routers.patch", "projects/"+p.ID+"/regions/"+r.Region+"/routers/"+r.Name)
				return fmt.Sprintf("Updated [https://www.googleapis.com/compute/v1/projects/%s/regions/%s/routers/%s].\n", p.ID, r.Region, r.Name), nil
			}
		}
		return nil, fmt.Errorf("peer %q not found in router %s", name, r.Name)
	})
	reg("compute vpn-tunnels describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.Arg(0, "NAME")
		if err != nil {
			return nil, err
		}
		for _, gn := range sim.SortedKeys(p.VPNGateways) {
			gw := p.VPNGateways[gn]
			for _, t := range gw.Tunnels {
				if t.Name == n {
					detail := "Tunnel is up and running."
					if t.Status != "ESTABLISHED" {
						detail = "No incoming packets from peer."
					}
					return Obj{V: map[string]any{"name": t.Name, "region": gw.Region, "vpnGateway": gw.Name, "peerIp": t.PeerIP, "router": t.Router, "status": t.Status, "detailedStatus": detail, "vpnGatewayInterface": t.Interface}}, nil
				}
			}
		}
		return nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/regions/%s/vpnTunnels/%s' was not found", p.ID, c.Str("region", c.S.Region), n)
	})
}

// routerAdvertisementFlags applies `routers update` advertisement flags.
func routerAdvertisementFlags(c *Cmd, r *sim.Router) bool {
	changed := false
	if m := c.Str("advertisement-mode", ""); m != "" {
		r.AdvertiseMode, changed = strings.ToUpper(m), true
	}
	if c.Has("set-advertisement-groups") {
		r.AdvertiseGroups, changed = upperAll(c.List("set-advertisement-groups")), true
	}
	if c.Has("set-advertisement-ranges") {
		r.AdvertisedRanges, changed = rangesOnly(c.List("set-advertisement-ranges")), true
	}
	for _, v := range rangesOnly(c.List("add-advertisement-ranges")) {
		if !inStrings(r.AdvertisedRanges, v) {
			r.AdvertisedRanges = append(r.AdvertisedRanges, v)
		}
		changed = true
	}
	for _, g := range upperAll(c.List("add-advertisement-groups")) {
		if !inStrings(r.AdvertiseGroups, g) {
			r.AdvertiseGroups = append(r.AdvertiseGroups, g)
		}
		changed = true
	}
	if rm := rangesOnly(c.List("remove-advertisement-ranges")); len(rm) > 0 {
		var out []string
		for _, v := range r.AdvertisedRanges {
			if !inStrings(rm, v) {
				out = append(out, v)
			}
		}
		r.AdvertisedRanges, changed = out, true
	}
	return changed
}

// "10.0.0.0/8=description" -> "10.0.0.0/8"
func rangesOnly(list []string) []string {
	var out []string
	for _, v := range list {
		out = append(out, strings.SplitN(v, "=", 2)[0])
	}
	return out
}

func upperAll(list []string) []string {
	var out []string
	for _, v := range list {
		out = append(out, strings.ToUpper(v))
	}
	return out
}

func inStrings(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func urlUnescape(s string) (string, error) {
	return strings.NewReplacer("%2F", "/", "%2f", "/").Replace(s), nil
}
