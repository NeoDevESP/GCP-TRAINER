package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

var reMachine = regexp.MustCompile(`^(e2-(micro|small|medium|standard-\d+|highmem-\d+|highcpu-\d+)|n1-(standard|highmem|highcpu)-\d+|n2d?-(standard|highmem|highcpu)-\d+|c2d?-standard-\d+|c3-standard-\d+|t2d-standard-\d+|a2-highgpu-\dg|g2-standard-\d+|custom-\d+-\d+)$`)
var reResName = regexp.MustCompile(`^[a-z]([-a-z0-9]{0,61}[a-z0-9])?$`)

func machineCPUs(mt string) int {
	switch {
	case strings.HasPrefix(mt, "e2-micro"), strings.HasPrefix(mt, "e2-small"), strings.HasPrefix(mt, "e2-medium"):
		return 2
	case strings.HasPrefix(mt, "custom-"):
		n, _ := strconv.Atoi(strings.Split(mt, "-")[1])
		return n
	case strings.HasPrefix(mt, "a2-highgpu-"):
		return 12
	}
	parts := strings.Split(mt, "-")
	n, _ := strconv.Atoi(parts[len(parts)-1])
	return n
}

func (c *Cmd) instance(name string) (*sim.Project, *sim.Instance, error) {
	p, err := c.P()
	if err != nil {
		return nil, nil, err
	}
	vm := p.Instances[name]
	if vm == nil {
		z := c.Str("zone", c.S.Zone)
		return nil, nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/zones/%s/instances/%s' was not found", p.ID, z, name)
	}
	if z := c.Str("zone", ""); z != "" && z != vm.Zone {
		return nil, nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/zones/%s/instances/%s' was not found", p.ID, z, name)
	}
	return p, vm, nil
}

func vmRes(p *sim.Project, vm *sim.Instance) sim.Resource {
	return sim.Resource{Project: p.ID, Type: "compute.googleapis.com/Instance", Name: "projects/" + p.ID + "/zones/" + vm.Zone + "/instances/" + vm.Name, Service: "compute.googleapis.com"}
}

func instanceRow(vm *sim.Instance) map[string]any {
	return map[string]any{"name": vm.Name, "zone": vm.Zone, "machineType": vm.MachineType, "status": vm.Status, "networkIP": vm.InternalIP, "natIP": vm.ExternalIP,
		"tags": map[string]any{"items": vm.Tags}, "labels": vm.Labels, "preemptible": vm.Spot,
		"networkInterfaces": []any{map[string]any{"network": vm.Network, "subnetwork": vm.Subnet, "networkIP": vm.InternalIP, "accessConfigs": func() []any {
			if vm.ExternalIP == "" {
				return nil
			}
			return []any{map[string]any{"name": "external-nat", "natIP": vm.ExternalIP, "type": "ONE_TO_ONE_NAT"}}
		}()}},
		"serviceAccounts": []any{map[string]any{"email": vm.ServiceAccount, "scopes": vm.Scopes}},
		"metadata":        map[string]any{"items": metaItems(vm.Metadata)}, "deletionProtection": vm.DeletionProtection,
		"guestAccelerators": vm.GPUs, "disks": vm.Disks, "shieldedInstanceConfig": map[string]any{"enableSecureBoot": vm.ShieldedVM}}
}

func metaItems(m map[string]string) []any {
	var out []any
	for _, k := range sim.SortedKeys(m) {
		out = append(out, map[string]any{"key": k, "value": m[k]})
	}
	return out
}

var instanceCols = []Col{{"NAME", "name"}, {"ZONE", "zone"}, {"MACHINE_TYPE", "machineType"}, {"PREEMPTIBLE", "preemptible"}, {"INTERNAL_IP", "networkIP"}, {"EXTERNAL_IP", "natIP"}, {"STATUS", "status"}}

// metadataFromFlags merges --metadata and --metadata-from-file.
func (c *Cmd) metadata() (map[string]string, error) {
	md := c.KV("metadata")
	for k, v := range c.KV("metadata-from-file") {
		content, ok := c.S.Files[c.S.path(v)]
		if !ok {
			return nil, fmt.Errorf("Unable to read file [%s]: [Errno 2] No such file or directory", v)
		}
		md[k] = content
	}
	return md, nil
}

func (c *Cmd) resolveSA(p *sim.Project, want string, noSA bool) (string, error) {
	if noSA {
		return "", nil
	}
	if want == "" {
		return p.Number + "-compute@developer.gserviceaccount.com", nil
	}
	var sa *sim.ServiceAccount
	var owner *sim.Project
	for _, pp := range c.S.State.Projects {
		if s := pp.ServiceAccounts[want]; s != nil {
			sa, owner = s, pp
		}
	}
	if sa == nil {
		return "", fmt.Errorf("Could not fetch resource:\n - The user does not have access to service account '%s'. User: '%s'. Ask a project owner to grant you the iam.serviceAccountUser role on the service account", want, c.S.Account)
	}
	if err := c.Need("iam.serviceAccounts.actAs", saResource(owner, sa)); err != nil {
		return "", fmt.Errorf("The user does not have access to service account '%s'. User: '%s'. Ask a project owner to grant you the iam.serviceAccountUser role on the service account (iam.serviceAccounts.actAs)", want, c.S.Account)
	}
	return want, nil
}

func normScopes(sc []string) []string {
	var out []string
	for _, s := range sc {
		s = strings.TrimPrefix(s, "https://www.googleapis.com/auth/")
		switch s {
		case "devstorage.read_only":
			s = "storage-ro"
		case "devstorage.read_write":
			s = "storage-rw"
		case "devstorage.full_control":
			s = "storage-full"
		}
		out = append(out, s)
	}
	return out
}

func (c *Cmd) pickSubnet(p *sim.Project, network, subnet, region string) (string, string, error) {
	if subnet != "" {
		subnet = subnet[strings.LastIndex(subnet, "/")+1:]
		sn := p.Subnets[subnet]
		if sn == nil {
			return "", "", fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/regions/%s/subnetworks/%s' was not found", p.ID, region, subnet)
		}
		if sn.Region != region {
			return "", "", fmt.Errorf("Could not fetch resource:\n - Invalid value for field 'resource.networkInterfaces[0].subnetwork': subnetwork %s is in region %s but the instance is in %s", subnet, sn.Region, region)
		}
		if network != "" && sn.Network != network {
			return "", "", fmt.Errorf("Invalid value for field 'resource.networkInterfaces[0]': subnetwork %s does not belong to network %s", subnet, network)
		}
		return sn.Network, subnet, nil
	}
	if network == "" {
		network = "default"
	}
	n := p.Networks[network]
	if n == nil {
		return "", "", fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/global/networks/%s' was not found", p.ID, network)
	}
	sn := p.SubnetForRegion(network, region)
	if sn == nil {
		return "", "", fmt.Errorf("Could not fetch resource:\n - Invalid value for field 'resource.networkInterfaces[0].network': network %s has no subnetwork in region %s; specify --subnet (custom-mode VPC)", network, region)
	}
	return network, sn.Name, nil
}

func (s *Session) createInstance(c *Cmd, p *sim.Project, name, zone string) (*sim.Instance, error) {
	if !reResName.MatchString(name) {
		return nil, fmt.Errorf("Invalid value for field 'resource.name': '%s'. Must be a match of regex '(?:[a-z](?:[-a-z0-9]{0,61}[a-z0-9])?)'", name)
	}
	if p.Instances[name] != nil {
		return nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/zones/%s/instances/%s' already exists", p.ID, zone, name)
	}
	tpl := &sim.InstanceTemplate{}
	if t := c.Str("source-instance-template", ""); t != "" {
		t = t[strings.LastIndex(t, "/")+1:]
		tpl = p.InstanceTemplates[t]
		if tpl == nil {
			return nil, fmt.Errorf("instance template %s not found", t)
		}
	}
	mt := c.Str("machine-type", tpl.MachineType)
	if mt == "" {
		mt = "e2-medium"
	}
	if !reMachine.MatchString(mt) {
		return nil, fmt.Errorf("Could not fetch resource:\n - Invalid value for field 'resource.machineType': 'zones/%s/machineTypes/%s'. Machine type with name '%s' does not exist in zone '%s'.", zone, mt, mt, zone)
	}
	if s.Policy.MaxMachineCPUs > 0 && machineCPUs(mt) > s.Policy.MaxMachineCPUs {
		return nil, fmt.Errorf("Quota 'CPUS' exceeded. Limit: %d.0 in region %s (lab quota).", s.Policy.MaxMachineCPUs, sim.RegionOf(zone))
	}
	gpus := 0
	if acc := c.KV("accelerator"); acc["count"] != "" || acc["type"] != "" {
		gpus, _ = strconv.Atoi(acc["count"])
		if gpus == 0 {
			gpus = 1
		}
	}
	if strings.HasPrefix(mt, "a2-") || strings.HasPrefix(mt, "g2-") {
		gpus = max(gpus, 1)
	}
	if gpus > 0 && !s.Policy.AllowGPUs {
		return nil, fmt.Errorf("Quota 'NVIDIA_T4_GPUS' exceeded. Limit: 0.0 in region %s. (GPUs are disabled in this lab.)", sim.RegionOf(zone))
	}
	if s.Policy.MaxInstances > 0 && len(p.Instances) >= s.Policy.MaxInstances {
		return nil, fmt.Errorf("Quota 'INSTANCES' exceeded. Limit: %d.0 in region %s (lab quota).", s.Policy.MaxInstances, sim.RegionOf(zone))
	}
	if err := c.NeedProject("compute.instances.create"); err != nil {
		return nil, err
	}
	network, subnet, err := c.pickSubnet(p, c.Str("network", tpl.Network), c.Str("subnet", tpl.Subnet), sim.RegionOf(zone))
	if err != nil {
		return nil, err
	}
	noAddr := c.Bool("no-address") || tpl.NoAddress
	if !noAddr && s.State.OrgPolicyEnforced(p.ID, "compute.vmExternalIpAccess") {
		return nil, fmt.Errorf("Could not fetch resource:\n - Constraint constraints/compute.vmExternalIpAccess violated for project %s. Add instance projects/%s/zones/%s/instances/%s to the constraint to use external IP with it.", p.Number, p.ID, zone, name)
	}
	if !c.Bool("shielded-secure-boot") && s.State.OrgPolicyEnforced(p.ID, "compute.requireShieldedVm") && !strings.Contains(c.Str("image-family", ""), "debian") {
		return nil, fmt.Errorf("Constraint constraints/compute.requireShieldedVm violated: image must support Shielded VM")
	}
	sa, err := c.resolveSA(p, c.Str("service-account", tpl.ServiceAccount), c.Bool("no-service-account"))
	if err != nil {
		return nil, err
	}
	scopes := normScopes(c.List("scopes"))
	if len(scopes) == 0 && !c.Bool("no-scopes") && sa != "" {
		scopes = []string{"default"}
	}
	md, err := c.metadata()
	if err != nil {
		return nil, err
	}
	for k, v := range tpl.Metadata {
		if _, ok := md[k]; !ok {
			md[k] = v
		}
	}
	tags := c.List("tags")
	if len(tags) == 0 {
		tags = append(tags, tpl.Tags...)
	}
	image := c.Str("image-family", c.Str("image", tpl.Image))
	if image == "" {
		image = "debian-12"
	}
	labels := c.KV("labels")
	vm := &sim.Instance{Name: name, Zone: zone, MachineType: mt, Status: "RUNNING", Tags: tags, Labels: labels, Metadata: md, Network: network, Subnet: subnet,
		ServiceAccount: sa, Scopes: scopes, Image: image, BootDisk: name, Disks: []string{name}, GPUs: gpus,
		Spot: c.Bool("spot") || c.Bool("preemptible") || c.Str("provisioning-model", "") == "SPOT", DeletionProtection: c.Bool("deletion-protection"),
		CreatedBy: c.Principal(), ShieldedVM: c.Bool("shielded-secure-boot"), OSLogin: md["enable-oslogin"] == "TRUE"}
	vm.InternalIP = s.State.AllocIP(p.Subnets[subnet].Range)
	if ip := c.Str("private-network-ip", ""); ip != "" {
		if !sim.IPInCIDR(ip, p.Subnets[subnet].Range) {
			return nil, fmt.Errorf("Requested internal IP address %s is outside the subnetwork range %s", ip, p.Subnets[subnet].Range)
		}
		vm.InternalIP = ip
	}
	if !noAddr {
		if a := c.Str("address", ""); a != "" {
			if addr := p.Addresses[a]; addr != nil {
				vm.ExternalIP = addr.Address
				addr.User = name
			} else {
				vm.ExternalIP = a
			}
		} else {
			vm.ExternalIP = s.State.ExternalIP()
		}
	}
	size := c.Int("boot-disk-size", 10)
	p.Disks[name] = &sim.Disk{Name: name, Zone: zone, SizeGB: size, Type: c.Str("boot-disk-type", "pd-balanced"), Image: image, Users: []string{name}}
	p.Instances[name] = vm
	s.State.Audit(p.ID, c.Principal(), "compute.googleapis.com", "v1.compute.instances.insert", "projects/"+p.ID+"/zones/"+zone+"/instances/"+name)
	return vm, nil
}

func init() {
	reg("compute instances create", func(c *Cmd) (any, error) {
		if err := c.API("compute.googleapis.com"); err != nil {
			return nil, err
		}
		p, _ := c.P()
		if len(c.Args) == 0 {
			return nil, fmt.Errorf("argument INSTANCE_NAMES [INSTANCE_NAMES ...]: Must be specified.")
		}
		zone, err := c.Zone()
		if err != nil {
			return nil, err
		}
		var rows []any
		out := ""
		for _, n := range c.Args {
			vm, err := c.S.createInstance(c, p, n, zone)
			if err != nil {
				return out, err
			}
			out += fmt.Sprintf("Created [https://www.googleapis.com/compute/v1/projects/%s/zones/%s/instances/%s].\n", p.ID, zone, n)
			rows = append(rows, instanceRow(vm))
		}
		t, _ := render(Table{Cols: instanceCols, Rows: rows}, c.F)
		return out + t, nil
	})
	reg("compute instances list", func(c *Cmd) (any, error) {
		if err := c.API("compute.googleapis.com"); err != nil {
			return nil, err
		}
		p, _ := c.P()
		if err := c.NeedProject("compute.instances.list"); err != nil {
			return nil, err
		}
		var rows []any
		zones := c.List("zones")
		for _, k := range sim.SortedKeys(p.Instances) {
			vm := p.Instances[k]
			if len(zones) > 0 && !contains(zones, vm.Zone) {
				continue
			}
			rows = append(rows, instanceRow(vm))
		}
		return Table{Cols: instanceCols, Rows: rows}, nil
	})
	reg("compute instances describe", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "INSTANCE_NAME")
		if err != nil {
			return nil, err
		}
		p, vm, err := c.instance(n)
		if err != nil {
			return nil, err
		}
		if err := c.Need("compute.instances.get", vmRes(p, vm)); err != nil {
			return nil, err
		}
		return Obj{V: instanceRow(vm)}, nil
	})
	reg("compute instances delete", func(c *Cmd) (any, error) {
		out := ""
		for _, n := range c.Args {
			p, vm, err := c.instance(n)
			if err != nil {
				return out, err
			}
			if vm.DeletionProtection {
				return out, fmt.Errorf("Could not fetch resource:\n - Invalid resource usage: 'Resource cannot be deleted if it's protected against deletion.'.")
			}
			if err := c.Need("compute.instances.delete", vmRes(p, vm)); err != nil {
				return out, err
			}
			if vm.Group != "" {
				if ig := p.InstanceGroups[vm.Group]; ig != nil && ig.Managed {
					return out, fmt.Errorf("The instance is managed by instance group %s; resize or delete the group instead (the MIG would recreate it).", vm.Group)
				}
			}
			delete(p.Instances, n)
			if !c.Bool("keep-disks") {
				delete(p.Disks, n)
			}
			for _, ig := range p.InstanceGroups {
				var keep []string
				for _, x := range ig.Instances {
					if x != n {
						keep = append(keep, x)
					}
				}
				ig.Instances = keep
			}
			for _, a := range p.Addresses {
				if a.User == n {
					a.User = ""
				}
			}
			c.Audit("compute.googleapis.com", "v1.compute.instances.delete", "projects/"+p.ID+"/zones/"+vm.Zone+"/instances/"+n)
			out += fmt.Sprintf("Deleted [https://www.googleapis.com/compute/v1/projects/%s/zones/%s/instances/%s].\n", p.ID, vm.Zone, n)
		}
		return out, nil
	})
	lifecycle := func(verb, status, perm string) handler {
		return func(c *Cmd) (any, error) {
			out := ""
			for _, n := range c.Args {
				p, vm, err := c.instance(n)
				if err != nil {
					return out, err
				}
				if err := c.Need(perm, vmRes(p, vm)); err != nil {
					return out, err
				}
				vm.Status = status
				if verb == "reset" {
					for k := range c.S.State.Extra {
						if strings.HasPrefix(k, "svc:"+p.ID+"/"+vm.Name+"/") && c.S.State.Extra[k] == "stopped" {
							delete(c.S.State.Extra, k)
						}
					}
				}
				c.Audit("compute.googleapis.com", "v1.compute.instances."+verb, "projects/"+p.ID+"/zones/"+vm.Zone+"/instances/"+n)
				out += fmt.Sprintf("Updated [https://compute.googleapis.com/compute/v1/projects/%s/zones/%s/instances/%s].\n", p.ID, vm.Zone, n)
			}
			return out, nil
		}
	}
	reg("compute instances start", lifecycle("start", "RUNNING", "compute.instances.start"))
	reg("compute instances stop", lifecycle("stop", "TERMINATED", "compute.instances.stop"))
	reg("compute instances reset", lifecycle("reset", "RUNNING", "compute.instances.reset"))
	reg("compute instances suspend", lifecycle("suspend", "SUSPENDED", "compute.instances.suspend"))
	reg("compute instances resume", lifecycle("resume", "RUNNING", "compute.instances.resume"))
	reg("compute instances add-tags", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "INSTANCE_NAME")
		if err != nil {
			return nil, err
		}
		p, vm, err := c.instance(n)
		if err != nil {
			return nil, err
		}
		if err := c.Need("compute.instances.setTags", vmRes(p, vm)); err != nil {
			return nil, err
		}
		for _, t := range c.List("tags") {
			if !contains(vm.Tags, t) {
				vm.Tags = append(vm.Tags, t)
			}
		}
		c.Audit("compute.googleapis.com", "v1.compute.instances.setTags", "projects/"+p.ID+"/zones/"+vm.Zone+"/instances/"+n)
		return fmt.Sprintf("Updated [https://www.googleapis.com/compute/v1/projects/%s/zones/%s/instances/%s].\n", p.ID, vm.Zone, n), nil
	})
	reg("compute instances remove-tags", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "INSTANCE_NAME")
		if err != nil {
			return nil, err
		}
		p, vm, err := c.instance(n)
		if err != nil {
			return nil, err
		}
		if err := c.Need("compute.instances.setTags", vmRes(p, vm)); err != nil {
			return nil, err
		}
		rm := c.List("tags")
		var keep []string
		for _, t := range vm.Tags {
			if !contains(rm, t) && !c.Bool("all") {
				keep = append(keep, t)
			}
		}
		vm.Tags = keep
		c.Audit("compute.googleapis.com", "v1.compute.instances.setTags", "projects/"+p.ID+"/zones/"+vm.Zone+"/instances/"+n)
		return fmt.Sprintf("Updated [https://www.googleapis.com/compute/v1/projects/%s/zones/%s/instances/%s].\n", p.ID, vm.Zone, n), nil
	})
	reg("compute instances add-metadata", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "INSTANCE_NAME")
		if err != nil {
			return nil, err
		}
		p, vm, err := c.instance(n)
		if err != nil {
			return nil, err
		}
		if err := c.Need("compute.instances.setMetadata", vmRes(p, vm)); err != nil {
			return nil, err
		}
		md, err := c.metadata()
		if err != nil {
			return nil, err
		}
		for k, v := range md {
			vm.Metadata[k] = v
		}
		c.Audit("compute.googleapis.com", "v1.compute.instances.setMetadata", "projects/"+p.ID+"/zones/"+vm.Zone+"/instances/"+n)
		return fmt.Sprintf("Updated [https://www.googleapis.com/compute/v1/projects/%s/zones/%s/instances/%s].\n", p.ID, vm.Zone, n), nil
	})
	reg("compute instances remove-metadata", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "INSTANCE_NAME")
		if err != nil {
			return nil, err
		}
		p, vm, err := c.instance(n)
		if err != nil {
			return nil, err
		}
		if err := c.Need("compute.instances.setMetadata", vmRes(p, vm)); err != nil {
			return nil, err
		}
		for _, k := range c.List("keys") {
			delete(vm.Metadata, k)
		}
		return fmt.Sprintf("Updated [https://www.googleapis.com/compute/v1/projects/%s/zones/%s/instances/%s].\n", p.ID, vm.Zone, n), nil
	})
	reg("compute instances set-service-account", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "INSTANCE_NAME")
		if err != nil {
			return nil, err
		}
		p, vm, err := c.instance(n)
		if err != nil {
			return nil, err
		}
		if vm.Status == "RUNNING" {
			return nil, fmt.Errorf("Could not fetch resource:\n - The instance must be stopped before changing its service account (current status RUNNING).")
		}
		if err := c.Need("compute.instances.setServiceAccount", vmRes(p, vm)); err != nil {
			return nil, err
		}
		sa, err := c.resolveSA(p, c.Str("service-account", ""), c.Bool("no-service-account"))
		if err != nil {
			return nil, err
		}
		vm.ServiceAccount = sa
		if c.Has("scopes") {
			vm.Scopes = normScopes(c.List("scopes"))
		}
		c.Audit("compute.googleapis.com", "v1.compute.instances.setServiceAccount", "projects/"+p.ID+"/zones/"+vm.Zone+"/instances/"+n)
		return fmt.Sprintf("Updated [https://www.googleapis.com/compute/v1/projects/%s/zones/%s/instances/%s].\n", p.ID, vm.Zone, n), nil
	})
	reg("compute instances set-machine-type", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "INSTANCE_NAME")
		if err != nil {
			return nil, err
		}
		p, vm, err := c.instance(n)
		if err != nil {
			return nil, err
		}
		if vm.Status == "RUNNING" {
			return nil, fmt.Errorf("The resource 'projects/%s/zones/%s/instances/%s' is not ready: instance must be stopped to change its machine type", p.ID, vm.Zone, n)
		}
		mt := c.Str("machine-type", "")
		if !reMachine.MatchString(mt) {
			return nil, fmt.Errorf("Invalid machine type %s", mt)
		}
		vm.MachineType = mt
		c.Audit("compute.googleapis.com", "v1.compute.instances.setMachineType", "projects/"+p.ID+"/zones/"+vm.Zone+"/instances/"+n)
		return "Updated.\n", nil
	})
	reg("compute instances update", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "INSTANCE_NAME")
		if err != nil {
			return nil, err
		}
		_, vm, err := c.instance(n)
		if err != nil {
			return nil, err
		}
		if c.Bool("deletion-protection") {
			vm.DeletionProtection = true
		}
		if c.Bool("no-deletion-protection") {
			vm.DeletionProtection = false
		}
		for k, v := range c.KV("update-labels") {
			vm.Labels[k] = v
		}
		if c.Bool("shielded-secure-boot") {
			vm.ShieldedVM = true
		}
		return "Updated.\n", nil
	})
	reg("compute instances delete-access-config", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "INSTANCE_NAME")
		if err != nil {
			return nil, err
		}
		p, vm, err := c.instance(n)
		if err != nil {
			return nil, err
		}
		if err := c.Need("compute.instances.deleteAccessConfig", vmRes(p, vm)); err != nil {
			return nil, err
		}
		vm.ExternalIP = ""
		c.Audit("compute.googleapis.com", "v1.compute.instances.deleteAccessConfig", "projects/"+p.ID+"/zones/"+vm.Zone+"/instances/"+n)
		return "Updated.\n", nil
	})
	reg("compute instances add-access-config", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "INSTANCE_NAME")
		if err != nil {
			return nil, err
		}
		p, vm, err := c.instance(n)
		if err != nil {
			return nil, err
		}
		if c.S.State.OrgPolicyEnforced(p.ID, "compute.vmExternalIpAccess") {
			return nil, fmt.Errorf("Constraint constraints/compute.vmExternalIpAccess violated for project %s.", p.Number)
		}
		vm.ExternalIP = c.S.State.ExternalIP()
		c.Audit("compute.googleapis.com", "v1.compute.instances.addAccessConfig", "projects/"+p.ID+"/zones/"+vm.Zone+"/instances/"+n)
		return "Updated.\n", nil
	})
	reg("compute instances get-serial-port-output", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "INSTANCE_NAME")
		if err != nil {
			return nil, err
		}
		p, vm, err := c.instance(n)
		if err != nil {
			return nil, err
		}
		if err := c.Need("compute.instances.getSerialPortOutput", vmRes(p, vm)); err != nil {
			return nil, err
		}
		_, console := c.S.State.VMListeners(p.ID, vm)
		return strings.Join(console, "\n") + "\n", nil
	})
	reg("compute ssh", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "INSTANCE")
		if err != nil {
			return nil, err
		}
		n = n[strings.LastIndex(n, "@")+1:]
		p, vm, err := c.instance(n)
		if err != nil {
			return nil, err
		}
		if vm.Status != "RUNNING" {
			return nil, fmt.Errorf("Instance %s is not running (status %s)", n, vm.Status)
		}
		if !c.S.State.Allowed(c.Principal(), "compute.instances.osLogin", vmRes(p, vm)) && !c.S.State.Allowed(c.Principal(), "compute.instances.setMetadata", vmRes(p, vm)) {
			return nil, fmt.Errorf("PERMISSION_DENIED: You need roles/compute.osLogin (or osAdminLogin) on the instance to log in.")
		}
		dst := c.S.State.VMEndpoint(p.ID, vm)
		viaIAP := c.Bool("tunnel-through-iap") || vm.ExternalIP == ""
		if viaIAP {
			if !c.S.State.Allowed(c.Principal(), "iap.tunnelInstances.accessViaIAP", vmRes(p, vm)) {
				return nil, fmt.Errorf("Error while connecting [4033: 'not authorized']. (roles/iap.tunnelResourceAccessor required)")
			}
			fr := c.S.State.CheckFlow(sim.Endpoint{Kind: "iap", IP: "35.235.240.10"}, dst, "tcp", 22)
			if !fr.Allowed {
				return nil, fmt.Errorf("Error while connecting [4003: 'failed to connect to backend']. Please ensure that the firewall allows ingress from 35.235.240.0/20 on tcp:22 (%s).", fr.Reason)
			}
		} else {
			dst.IP = vm.ExternalIP
			fr := c.S.State.CheckFlow(sim.InternetEndpoint(sim.StudentIP), dst, "tcp", 22)
			if !fr.Allowed {
				return nil, fmt.Errorf("ssh: connect to host %s port 22: Connection timed out\nRecommendation: To check for possible causes of SSH connectivity issues, re-run with --troubleshoot or use --tunnel-through-iap. (%s)", vm.ExternalIP, fr.Reason)
			}
		}
		cmd := c.Str("command", "")
		if cmd == "" && len(c.Args) > 1 {
			cmd = strings.Join(c.Args[1:], " ")
		}
		if cmd == "" {
			return fmt.Sprintf("Connected to %s. Interactive shells are not available in the simulator; use --command=\"...\".\n", n), nil
		}
		c.Audit("compute.googleapis.com", "ssh", "projects/"+p.ID+"/zones/"+vm.Zone+"/instances/"+n)
		return c.S.vmExec(p, vm, cmd)
	})

	// ---- templates and groups --------------------------------------------
	reg("compute instance-templates create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.Arg(0, "NAME")
		if err != nil {
			return nil, err
		}
		if p.InstanceTemplates[n] != nil {
			return nil, fmt.Errorf("The resource 'projects/%s/global/instanceTemplates/%s' already exists", p.ID, n)
		}
		if err := c.NeedProject("compute.instanceTemplates.create"); err != nil {
			return nil, err
		}
		md, err := c.metadata()
		if err != nil {
			return nil, err
		}
		mt := c.Str("machine-type", "e2-medium")
		if !reMachine.MatchString(mt) {
			return nil, fmt.Errorf("Invalid machine type %s", mt)
		}
		sa, err := c.resolveSA(p, c.Str("service-account", ""), c.Bool("no-service-account"))
		if err != nil {
			return nil, err
		}
		region := c.Str("region", c.S.Region)
		t := &sim.InstanceTemplate{Name: n, MachineType: mt, Tags: c.List("tags"), Metadata: md, Network: c.Str("network", "default"), Subnet: c.Str("subnet", ""), Region: region,
			NoAddress: c.Bool("no-address"), ServiceAccount: sa, Image: c.Str("image-family", "debian-12"), Labels: c.KV("labels")}
		if t.Subnet != "" {
			if sn := p.Subnets[t.Subnet]; sn != nil {
				t.Network = sn.Network
			} else {
				return nil, fmt.Errorf("subnetwork %s not found", t.Subnet)
			}
		} else if n := p.Networks[t.Network]; n == nil {
			return nil, fmt.Errorf("The resource 'projects/%s/global/networks/%s' was not found", p.ID, t.Network)
		}
		p.InstanceTemplates[n] = t
		c.Audit("compute.googleapis.com", "v1.compute.instanceTemplates.insert", "projects/"+p.ID+"/global/instanceTemplates/"+n)
		return fmt.Sprintf("Created [https://www.googleapis.com/compute/v1/projects/%s/global/instanceTemplates/%s].\n", p.ID, n), nil
	})
	reg("compute instance-templates list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.InstanceTemplates) {
			rows = append(rows, p.InstanceTemplates[k])
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"MACHINE_TYPE", "machineType"}, {"PREEMPTIBLE", "spot"}}, Rows: rows}, nil
	})
	reg("compute instance-templates describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		t := p.InstanceTemplates[n]
		if t == nil {
			return nil, fmt.Errorf("instance template %s not found", n)
		}
		return Obj{V: t}, nil
	})
	reg("compute instance-templates delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		for _, n := range c.Args {
			for _, ig := range p.InstanceGroups {
				if ig.Template == n {
					return nil, fmt.Errorf("The instance_template resource '%s' is already being used by '%s'", n, ig.Name)
				}
			}
			delete(p.InstanceTemplates, n)
		}
		return "Deleted.\n", nil
	})
	reg("compute instance-groups managed create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.Arg(0, "NAME")
		if err != nil {
			return nil, err
		}
		zone := c.Str("zone", c.S.Zone)
		region := c.Str("region", "")
		if region != "" {
			zone = region + "-b"
		}
		if zone == "" {
			return nil, fmt.Errorf("Specify --zone or --region")
		}
		tpl := c.Str("template", "")
		if p.InstanceTemplates[tpl] == nil {
			return nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/global/instanceTemplates/%s' was not found", p.ID, tpl)
		}
		if err := c.NeedProject("compute.instanceGroupManagers.create"); err != nil {
			return nil, err
		}
		size := c.Int("size", 0)
		if s := c.S.Policy.MaxInstances; s > 0 && len(p.Instances)+size > s {
			return nil, fmt.Errorf("Quota 'INSTANCES' exceeded. Limit: %d.0 (lab quota).", s)
		}
		ig := &sim.InstanceGroup{Name: n, Zone: zone, Region: region, Managed: true, Template: tpl, TargetSize: size, NamedPorts: map[string]int{}, BaseInstanceName: c.Str("base-instance-name", n)}
		if region != "" {
			ig.Zones = c.List("zones")
			if len(ig.Zones) == 0 {
				ig.Zones = []string{region + "-b", region + "-c", region + "-d"}
			}
		}
		if hc := c.Str("health-check", ""); hc != "" {
			ig.AutoHealing = &sim.AutoHealing{HealthCheck: hc, InitialDelay: c.Int("initial-delay", 300)}
		}
		p.InstanceGroups[n] = ig
		c.S.State.ResizeMIG(p.ID, ig)
		c.Audit("compute.googleapis.com", "v1.compute.instanceGroupManagers.insert", "projects/"+p.ID+"/zones/"+zone+"/instanceGroupManagers/"+n)
		return fmt.Sprintf("Created [https://www.googleapis.com/compute/v1/projects/%s/zones/%s/instanceGroupManagers/%s].\nNAME  LOCATION  SCOPE  BASE_INSTANCE_NAME  SIZE  TARGET_SIZE  INSTANCE_TEMPLATE  AUTOSCALED\n%s  %s  zone  %s  0  %d  %s  no\n", p.ID, zone, n, n, zone, ig.BaseInstanceName, size, tpl), nil
	})
	groupOf := func(c *Cmd) (*sim.Project, *sim.InstanceGroup, error) {
		p, err := c.P()
		if err != nil {
			return nil, nil, err
		}
		n, err := c.Arg(0, "NAME")
		if err != nil {
			return nil, nil, err
		}
		ig := p.InstanceGroups[n]
		if ig == nil {
			return nil, nil, fmt.Errorf("Could not fetch resource:\n - The resource 'projects/%s/zones/%s/instanceGroupManagers/%s' was not found", p.ID, c.Str("zone", c.S.Zone), n)
		}
		return p, ig, nil
	}
	reg("compute instance-groups managed set-autoscaling", func(c *Cmd) (any, error) {
		p, ig, err := groupOf(c)
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("compute.autoscalers.create"); err != nil {
			return nil, err
		}
		tgt, _ := strconv.ParseFloat(c.Str("target-cpu-utilization", "0.6"), 64)
		mx := c.Int("max-num-replicas", 0)
		if mx == 0 {
			return nil, fmt.Errorf("argument --max-num-replicas: Must be specified.")
		}
		if c.S.Policy.MaxInstances > 0 && mx > c.S.Policy.MaxInstances {
			return nil, fmt.Errorf("max-num-replicas %d exceeds the lab quota of %d instances", mx, c.S.Policy.MaxInstances)
		}
		ig.Autoscaler = &sim.Autoscaler{Min: c.Int("min-num-replicas", 1), Max: mx, TargetCPU: tgt, Cooldown: c.Int("cool-down-period", 60)}
		c.S.State.Audit(p.ID, c.Principal(), "compute.googleapis.com", "v1.compute.autoscalers.insert", "projects/"+p.ID+"/zones/"+ig.Zone+"/autoscalers/"+ig.Name)
		return fmt.Sprintf("Created [https://www.googleapis.com/compute/v1/projects/%s/zones/%s/autoscalers/%s].\n", p.ID, ig.Zone, ig.Name), nil
	})
	reg("compute instance-groups managed stop-autoscaling", func(c *Cmd) (any, error) {
		_, ig, err := groupOf(c)
		if err != nil {
			return nil, err
		}
		ig.Autoscaler = nil
		return "Updated.\n", nil
	})
	reg("compute instance-groups managed resize", func(c *Cmd) (any, error) {
		p, ig, err := groupOf(c)
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("compute.instanceGroupManagers.update"); err != nil {
			return nil, err
		}
		if ig.Autoscaler != nil {
			return nil, fmt.Errorf("The group is autoscaled; change the autoscaler (set-autoscaling / stop-autoscaling) instead of resizing it.")
		}
		ig.TargetSize = c.Int("size", ig.TargetSize)
		c.S.State.ResizeMIG(p.ID, ig)
		c.S.State.Audit(p.ID, c.Principal(), "compute.googleapis.com", "v1.compute.instanceGroupManagers.resize", "projects/"+p.ID+"/zones/"+ig.Zone+"/instanceGroupManagers/"+ig.Name)
		return "Updated.\n", nil
	})
	reg("compute instance-groups managed list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.InstanceGroups) {
			ig := p.InstanceGroups[k]
			if ig.Managed {
				rows = append(rows, map[string]any{"name": ig.Name, "location": ig.Zone, "size": len(ig.Instances), "targetSize": ig.TargetSize, "instanceTemplate": ig.Template, "autoscaled": ig.Autoscaler != nil})
			}
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"LOCATION", "location"}, {"SIZE", "size"}, {"TARGET_SIZE", "targetSize"}, {"INSTANCE_TEMPLATE", "instanceTemplate"}, {"AUTOSCALED", "autoscaled"}}, Rows: rows}, nil
	})
	reg("compute instance-groups managed describe", func(c *Cmd) (any, error) {
		_, ig, err := groupOf(c)
		if err != nil {
			return nil, err
		}
		return Obj{V: ig}, nil
	})
	reg("compute instance-groups managed list-instances", func(c *Cmd) (any, error) {
		p, ig, err := groupOf(c)
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, n := range ig.Instances {
			if vm := p.Instances[n]; vm != nil {
				rows = append(rows, map[string]any{"name": n, "zone": vm.Zone, "status": vm.Status, "action": "NONE"})
			}
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"ZONE", "zone"}, {"STATUS", "status"}, {"ACTION", "action"}}, Rows: rows}, nil
	})
	reg("compute instance-groups managed delete", func(c *Cmd) (any, error) {
		p, ig, err := groupOf(c)
		if err != nil {
			return nil, err
		}
		for _, bs := range p.BackendServices {
			for _, be := range bs.Backends {
				if be.Group == ig.Name {
					return nil, fmt.Errorf("The instance_group_manager resource is in use by backend service %s", bs.Name)
				}
			}
		}
		for _, n := range ig.Instances {
			delete(p.Instances, n)
		}
		delete(p.InstanceGroups, ig.Name)
		c.Audit("compute.googleapis.com", "v1.compute.instanceGroupManagers.delete", "projects/"+p.ID+"/zones/"+ig.Zone+"/instanceGroupManagers/"+ig.Name)
		return "Deleted.\n", nil
	})
	reg("compute instance-groups managed set-instance-template", func(c *Cmd) (any, error) {
		p, ig, err := groupOf(c)
		if err != nil {
			return nil, err
		}
		t := c.Str("template", "")
		if p.InstanceTemplates[t] == nil {
			return nil, fmt.Errorf("instance template %s not found", t)
		}
		ig.Template = t
		return "Updated.\n", nil
	})
	reg("compute instance-groups managed rolling-action start-update", func(c *Cmd) (any, error) {
		p, ig, err := groupOf(c)
		if err != nil {
			return nil, err
		}
		v := c.KV("version")
		t := v["template"]
		if p.InstanceTemplates[t] == nil {
			return nil, fmt.Errorf("instance template %s not found", t)
		}
		ig.Template = t
		size := ig.TargetSize
		for _, n := range ig.Instances {
			delete(p.Instances, n)
		}
		ig.Instances = nil
		ig.TargetSize = size
		c.S.State.ResizeMIG(p.ID, ig)
		c.Audit("compute.googleapis.com", "v1.compute.instanceGroupManagers.patch", "projects/"+p.ID+"/zones/"+ig.Zone+"/instanceGroupManagers/"+ig.Name)
		return "Updated. Rolling update started (maxSurge=1, maxUnavailable=0).\n", nil
	})
	reg("compute instance-groups managed rolling-action replace", func(c *Cmd) (any, error) {
		p, ig, err := groupOf(c)
		if err != nil {
			return nil, err
		}
		for _, n := range ig.Instances {
			delete(p.Instances, n)
		}
		ig.Instances = nil
		c.S.State.ResizeMIG(p.ID, ig)
		return "Updated.\n", nil
	})
	reg("compute instance-groups managed update", func(c *Cmd) (any, error) {
		_, ig, err := groupOf(c)
		if err != nil {
			return nil, err
		}
		if hc := c.Str("health-check", ""); hc != "" {
			ig.AutoHealing = &sim.AutoHealing{HealthCheck: hc, InitialDelay: c.Int("initial-delay", 300)}
		}
		return "Updated.\n", nil
	})
	setNamed := func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		ig := p.InstanceGroups[n]
		if ig == nil {
			return nil, fmt.Errorf("instance group %s not found", n)
		}
		ig.NamedPorts = map[string]int{}
		for _, np := range c.List("named-ports") {
			k, v, _ := strings.Cut(np, ":")
			port, _ := strconv.Atoi(v)
			ig.NamedPorts[k] = port
		}
		c.Audit("compute.googleapis.com", "v1.compute.instanceGroups.setNamedPorts", "projects/"+p.ID+"/zones/"+ig.Zone+"/instanceGroups/"+n)
		return "Updated.\n", nil
	}
	reg("compute instance-groups set-named-ports", setNamed)
	reg("compute instance-groups managed set-named-ports", setNamed)
	reg("compute instance-groups unmanaged set-named-ports", setNamed)
	reg("compute instance-groups get-named-ports", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		ig := p.InstanceGroups[n]
		if ig == nil {
			return nil, fmt.Errorf("instance group %s not found", n)
		}
		var rows []any
		for _, k := range sim.SortedKeys(ig.NamedPorts) {
			rows = append(rows, map[string]any{"name": k, "port": ig.NamedPorts[k]})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"PORT", "port"}}, Rows: rows}, nil
	})
	reg("compute instance-groups unmanaged create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.Arg(0, "NAME")
		if err != nil {
			return nil, err
		}
		zone, err := c.Zone()
		if err != nil {
			return nil, err
		}
		p.InstanceGroups[n] = &sim.InstanceGroup{Name: n, Zone: zone, NamedPorts: map[string]int{}}
		return fmt.Sprintf("Created [https://www.googleapis.com/compute/v1/projects/%s/zones/%s/instanceGroups/%s].\n", p.ID, zone, n), nil
	})
	reg("compute instance-groups unmanaged add-instances", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		ig := p.InstanceGroups[n]
		if ig == nil {
			return nil, fmt.Errorf("instance group %s not found", n)
		}
		for _, i := range c.List("instances") {
			vm := p.Instances[i]
			if vm == nil {
				return nil, fmt.Errorf("instance %s not found", i)
			}
			if vm.Zone != ig.Zone {
				return nil, fmt.Errorf("instance %s is in zone %s, group is in %s", i, vm.Zone, ig.Zone)
			}
			if !contains(ig.Instances, i) {
				ig.Instances = append(ig.Instances, i)
			}
			vm.Group = n
		}
		return "Updated.\n", nil
	})
	reg("compute instance-groups unmanaged remove-instances", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		ig := p.InstanceGroups[n]
		if ig == nil {
			return nil, fmt.Errorf("instance group %s not found", n)
		}
		rm := c.List("instances")
		var keep []string
		for _, i := range ig.Instances {
			if !contains(rm, i) {
				keep = append(keep, i)
			}
		}
		ig.Instances = keep
		return "Updated.\n", nil
	})
	reg("compute instance-groups list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.InstanceGroups) {
			ig := p.InstanceGroups[k]
			rows = append(rows, map[string]any{"name": ig.Name, "location": ig.Zone, "managed": ig.Managed, "instances": len(ig.Instances)})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"LOCATION", "location"}, {"MANAGED", "managed"}, {"INSTANCES", "instances"}}, Rows: rows}, nil
	})
	reg("compute instance-groups list-instances", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		ig := p.InstanceGroups[n]
		if ig == nil {
			return nil, fmt.Errorf("instance group %s not found", n)
		}
		var rows []any
		for _, i := range ig.Instances {
			if vm := p.Instances[i]; vm != nil {
				rows = append(rows, map[string]any{"name": i, "status": vm.Status})
			}
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"STATUS", "status"}}, Rows: rows}, nil
	})

	// ---- disks & snapshots -------------------------------------------------
	reg("compute disks create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		zone, err := c.Zone()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("compute.disks.create"); err != nil {
			return nil, err
		}
		for _, n := range c.Args {
			if p.Disks[n] != nil {
				return nil, fmt.Errorf("The resource 'projects/%s/zones/%s/disks/%s' already exists", p.ID, zone, n)
			}
			d := &sim.Disk{Name: n, Zone: zone, SizeGB: c.Int("size", 10), Type: c.Str("type", "pd-balanced"), Snapshot: c.Str("source-snapshot", ""), KMSKey: c.Str("kms-key", "")}
			if d.Snapshot != "" {
				sn := p.Snapshots[d.Snapshot]
				if sn == nil {
					return nil, fmt.Errorf("snapshot %s not found", d.Snapshot)
				}
				d.SizeGB = max(d.SizeGB, sn.SizeGB)
			}
			p.Disks[n] = d
			c.Audit("compute.googleapis.com", "v1.compute.disks.insert", "projects/"+p.ID+"/zones/"+zone+"/disks/"+n)
		}
		return "Created.\n", nil
	})
	reg("compute disks list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Disks) {
			d := p.Disks[k]
			rows = append(rows, map[string]any{"name": d.Name, "zone": d.Zone, "sizeGb": d.SizeGB, "type": d.Type, "status": "READY", "users": d.Users})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"LOCATION", "zone"}, {"SIZE_GB", "sizeGb"}, {"TYPE", "type"}, {"STATUS", "status"}}, Rows: rows}, nil
	})
	reg("compute disks delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		for _, n := range c.Args {
			d := p.Disks[n]
			if d == nil {
				return nil, fmt.Errorf("disk %s not found", n)
			}
			if vm := p.Instances[n]; vm != nil {
				return nil, fmt.Errorf("The disk resource 'projects/%s/zones/%s/disks/%s' is already being used by 'projects/%s/zones/%s/instances/%s'", p.ID, d.Zone, n, p.ID, d.Zone, n)
			}
			for _, vm := range p.Instances {
				if contains(vm.Disks, n) {
					return nil, fmt.Errorf("The disk resource is already being used by instance %s", vm.Name)
				}
			}
			delete(p.Disks, n)
			c.Audit("compute.googleapis.com", "v1.compute.disks.delete", "projects/"+p.ID+"/zones/"+d.Zone+"/disks/"+n)
		}
		return "Deleted.\n", nil
	})
	reg("compute disks snapshot", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.Arg(0, "DISK_NAME")
		if err != nil {
			return nil, err
		}
		d := p.Disks[n]
		if d == nil {
			return nil, fmt.Errorf("disk %s not found", n)
		}
		if err := c.NeedProject("compute.snapshots.create"); err != nil {
			return nil, err
		}
		names := c.List("snapshot-names")
		if len(names) == 0 {
			names = []string{n + "-" + c.S.State.ID(6)}
		}
		for _, sn := range names {
			p.Snapshots[sn] = &sim.Snapshot{Name: sn, SourceDisk: n, Location: c.Str("storage-location", sim.RegionOf(d.Zone)), SizeGB: d.SizeGB, Created: c.S.State.Now()}
			c.Audit("compute.googleapis.com", "v1.compute.disks.createSnapshot", "projects/"+p.ID+"/global/snapshots/"+sn)
		}
		return "Creating snapshot(s) " + strings.Join(names, ", ") + "...done.\n", nil
	})
	reg("compute disks resize", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "DISK_NAME")
		d := p.Disks[n]
		if d == nil {
			return nil, fmt.Errorf("disk %s not found", n)
		}
		sz := c.Int("size", d.SizeGB)
		if sz < d.SizeGB {
			return nil, fmt.Errorf("Disk size cannot be decreased (%d < %d GB)", sz, d.SizeGB)
		}
		d.SizeGB = sz
		return "Updated.\n", nil
	})
	reg("compute instances attach-disk", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "INSTANCE_NAME")
		if err != nil {
			return nil, err
		}
		p, vm, err := c.instance(n)
		if err != nil {
			return nil, err
		}
		dn := c.Str("disk", "")
		d := p.Disks[dn]
		if d == nil {
			return nil, fmt.Errorf("disk %s not found", dn)
		}
		if d.Zone != vm.Zone {
			return nil, fmt.Errorf("disk %s is in zone %s, instance in %s", dn, d.Zone, vm.Zone)
		}
		vm.Disks = append(vm.Disks, dn)
		d.Users = append(d.Users, n)
		return "Updated.\n", nil
	})
	reg("compute snapshots list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Snapshots) {
			rows = append(rows, p.Snapshots[k])
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"DISK_SIZE_GB", "diskSizeGb"}, {"SRC_DISK", "sourceDisk"}, {"STATUS", "status"}}, Rows: rows}, nil
	})
	reg("compute snapshots delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		for _, n := range c.Args {
			delete(p.Snapshots, n)
		}
		return "Deleted.\n", nil
	})
	reg("compute machine-types list", func(c *Cmd) (any, error) {
		var rows []any
		for _, mt := range []string{"e2-micro", "e2-small", "e2-medium", "e2-standard-2", "e2-standard-4", "n2-standard-2", "n2-standard-4", "n2-standard-8", "c2-standard-8"} {
			rows = append(rows, map[string]any{"name": mt, "cpus": machineCPUs(mt), "hourlyEur": sim.MachineHourly(mt)})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"CPUS", "cpus"}, {"EST_HOURLY_EUR", "hourlyEur"}}, Rows: rows}, nil
	})
	reg("compute zones list", func(c *Cmd) (any, error) {
		var rows []any
		for _, r := range sim.SortedKeys(sim.ValidRegions) {
			for _, s := range []string{"a", "b", "c"} {
				rows = append(rows, map[string]any{"name": r + "-" + s, "region": r, "status": "UP"})
			}
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"REGION", "region"}, {"STATUS", "status"}}, Rows: rows}, nil
	})
	reg("compute regions list", func(c *Cmd) (any, error) {
		var rows []any
		for _, r := range sim.SortedKeys(sim.ValidRegions) {
			rows = append(rows, map[string]any{"name": r, "status": "UP"})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"STATUS", "status"}}, Rows: rows}, nil
	})
	reg("compute images list", func(c *Cmd) (any, error) {
		var rows []any
		for _, i := range []string{"debian-12", "debian-11", "ubuntu-2204-lts", "ubuntu-2404-lts-amd64", "cos-stable", "rocky-linux-9"} {
			rows = append(rows, map[string]any{"name": i + "-v20260901", "family": i})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"FAMILY", "family"}}, Rows: rows}, nil
	})
	reg("compute project-info add-metadata", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		for k, v := range c.KV("metadata") {
			p.Labels["metadata."+k] = v
		}
		return "Updated.\n", nil
	})
}

func contains(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}
