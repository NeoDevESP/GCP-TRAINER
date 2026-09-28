package cli

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"sort"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

var regionCode = map[string]string{"europe-west1": "ew", "europe-west2": "nw", "europe-west3": "ey", "europe-west4": "ez", "us-central1": "uc", "us-east1": "ue", "us-west1": "uw", "asia-east1": "de", "europe-southwest1": "no", "asia-northeast1": "an"}

func runURL(p *sim.Project, name, region string) string {
	h := fnv.New32a()
	h.Write([]byte(p.Number + name))
	rc := regionCode[region]
	if rc == "" {
		rc = "xx"
	}
	return fmt.Sprintf("https://%s-%x-%s.a.run.app", name, h.Sum32()%0xfffffff, rc)
}

func runRes(p *sim.Project, svc *sim.RunService) sim.Resource {
	return sim.Resource{Project: p.ID, Type: "run.googleapis.com/Service", Name: "projects/" + p.ID + "/locations/" + svc.Region + "/services/" + svc.Name, Service: "run.googleapis.com", Policies: []*sim.Policy{&svc.IAM}}
}

func (c *Cmd) runService(name string) (*sim.Project, *sim.RunService, error) {
	p, err := c.P()
	if err != nil {
		return nil, nil, err
	}
	svc := p.RunServices[name]
	if svc == nil {
		return nil, nil, fmt.Errorf("Cannot find service [%s]", name)
	}
	return p, svc, nil
}

var reRunName = regexp.MustCompile(`^[a-z]([-a-z0-9]{0,47}[a-z0-9])?$`)

// applyRunFlags mutates a copy of a service configuration from deploy/update flags.
func (c *Cmd) applyRunFlags(p *sim.Project, svc *sim.RunService) error {
	if v := c.Str("image", ""); v != "" {
		svc.Image = v
	}
	if c.Has("set-env-vars") || c.Bool("clear-env-vars") {
		svc.Env = map[string]string{}
	}
	for k, v := range c.KV("set-env-vars") {
		svc.Env[k] = v
	}
	for k, v := range c.KV("update-env-vars") {
		svc.Env[k] = v
	}
	for _, k := range c.List("remove-env-vars") {
		delete(svc.Env, k)
	}
	if c.Has("set-secrets") || c.Bool("clear-secrets") {
		svc.Secrets = map[string]string{}
	}
	for k, v := range c.KV("set-secrets") {
		svc.Secrets[k] = v
	}
	for k, v := range c.KV("update-secrets") {
		svc.Secrets[k] = v
	}
	for _, k := range c.List("remove-secrets") {
		delete(svc.Secrets, k)
	}
	for k := range svc.Secrets {
		if _, dup := svc.Env[k]; dup {
			return fmt.Errorf("spec.template.spec.containers[0].env: duplicate environment variable %q defined both as literal and as secret; remove it with --remove-env-vars=%s", k, k)
		}
	}
	if v := c.Str("service-account", ""); v != "" {
		sa, err := c.resolveSA(p, v, false)
		if err != nil {
			return err
		}
		svc.SA = sa
	}
	if v := c.Str("ingress", ""); v != "" {
		switch v {
		case "all", "internal", "internal-and-cloud-load-balancing":
			svc.Ingress = v
		default:
			return fmt.Errorf("argument --ingress: Invalid choice: '%s'", v)
		}
	}
	if c.Has("min-instances") {
		svc.MinInstances = c.Int("min-instances", 0)
	}
	if c.Has("max-instances") {
		svc.MaxInstances = c.Int("max-instances", 100)
	}
	if c.Has("concurrency") {
		svc.Concurrency = c.Int("concurrency", 80)
	}
	if v := c.Str("memory", ""); v != "" {
		svc.Memory = v
	}
	if v := c.Str("cpu", ""); v != "" {
		svc.CPU = v
	}
	if c.Has("port") {
		svc.Port = c.Int("port", 8080)
	}
	if v := c.Str("network", ""); v != "" {
		v = v[strings.LastIndex(v, "/")+1:]
		if p.Networks[v] == nil {
			return fmt.Errorf("network %s not found", v)
		}
		svc.Network = v
		svc.Subnet = c.Str("subnet", "")
		if svc.Subnet == "" {
			if sn := p.SubnetForRegion(v, svc.Region); sn != nil {
				svc.Subnet = sn.Name
			} else {
				return fmt.Errorf("network %s has no subnet in %s; pass --subnet", v, svc.Region)
			}
		}
		if t := c.List("network-tags"); len(t) > 0 {
			svc.Labels["network-tags"] = strings.Join(t, ".")
		}
	}
	if c.Bool("clear-network") {
		svc.Network, svc.Subnet = "", ""
	}
	if v := c.Str("vpc-connector", ""); v != "" {
		if c.S.State.Extra["connector:"+p.ID+"/"+v] == "" {
			return fmt.Errorf("VPC connector %s not found", v)
		}
		svc.VPCConnector = v
	}
	if v := c.Str("vpc-egress", ""); v != "" {
		svc.VPCEgress = v
	}
	if c.Has("add-cloudsql-instances") || c.Has("set-cloudsql-instances") {
		if c.Has("set-cloudsql-instances") {
			svc.CloudSQL = nil
		}
		for _, x := range append(c.List("add-cloudsql-instances"), c.List("set-cloudsql-instances")...) {
			if !strings.Contains(x, ":") {
				if in := p.SQLInstances[x]; in != nil {
					x = p.ID + ":" + in.Region + ":" + x
				}
			}
			svc.CloudSQL = append(svc.CloudSQL, x)
		}
	}
	for k, v := range c.KV("labels") {
		svc.Labels[k] = v
	}
	for k, v := range c.KV("update-labels") {
		svc.Labels[k] = v
	}
	return nil
}

func cloneRun(s *sim.RunService) *sim.RunService {
	c := *s
	c.Env = copyStrMap(s.Env)
	c.Secrets = copyStrMap(s.Secrets)
	c.Labels = copyStrMap(s.Labels)
	c.CloudSQL = append([]string{}, s.CloudSQL...)
	c.Revisions = append([]sim.Revision{}, s.Revisions...)
	return &c
}

func copyStrMap(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (s *Session) deployRun(c *Cmd, name string, update bool) (string, error) {
	if err := c.API("run.googleapis.com"); err != nil {
		return "", err
	}
	p, _ := c.P()
	region, err := c.RegionFlag()
	if err != nil {
		if c.S.Region == "" && c.Str("region", "") == "" {
			return "", fmt.Errorf("No region specified. Pass --region or set run/region with `gcloud config set run/region REGION`.")
		}
		return "", err
	}
	if !reRunName.MatchString(name) {
		return "", fmt.Errorf("Invalid service name %q", name)
	}
	old := p.RunServices[name]
	if update && old == nil {
		return "", fmt.Errorf("Cannot find service [%s]", name)
	}
	perm := "run.services.create"
	var svc *sim.RunService
	if old != nil {
		perm = "run.services.update"
		svc = cloneRun(old)
	} else {
		svc = &sim.RunService{Name: name, Region: region, Env: map[string]string{}, Secrets: map[string]string{}, Labels: map[string]string{}, Ingress: "all", MaxInstances: 100, Concurrency: 80, Memory: "512Mi", CPU: "1", Port: 8080}
		svc.URL = runURL(p, name, region)
	}
	if err := c.Need(perm, sim.Resource{Project: p.ID, Type: "run.googleapis.com/Service", Name: "projects/" + p.ID + "/locations/" + region + "/services/" + name, Service: "run.googleapis.com"}); err != nil {
		return "", err
	}
	if src := c.Str("source", ""); src != "" {
		img, err := s.buildFromSource(c, p, region, name)
		if err != nil {
			return "", err
		}
		c.F["image"] = []string{img}
	}
	if !update && svc.Image == "" && c.Str("image", "") == "" {
		return "", fmt.Errorf("Missing required argument [--image]: Requires a container image to deploy (e.g. `gcr.io/cloudrun/hello:latest`) if no build source is provided.")
	}
	if err := c.applyRunFlags(p, svc); err != nil {
		return "", err
	}
	if svc.SA == "" {
		if _, err := c.resolveSA(p, p.Number+"-compute@developer.gserviceaccount.com", false); err != nil {
			return "", err
		}
	}
	revName := fmt.Sprintf("%s-%05d-%s", name, len(svc.Revisions)+1, s.State.ID(3))
	// Validate the revision can start before shifting traffic.
	p.RunServices[name] = svc
	w, why := s.State.RunWorkload(p.ID, svc)
	if w == nil {
		if old != nil {
			old.Revisions = append(old.Revisions, sim.Revision{Name: revName, Image: svc.Image, Env: svc.Env, Secrets: svc.Secrets, Traffic: 0, Tag: "FAILED"})
			p.RunServices[name] = old
		} else {
			svc.Revisions = append(svc.Revisions, sim.Revision{Name: revName, Image: svc.Image, Env: svc.Env, Secrets: svc.Secrets, Tag: "FAILED"})
			svc.Image = ""
		}
		c.Audit("run.googleapis.com", "google.cloud.run.v1.Services.ReplaceService", "namespaces/"+p.ID+"/services/"+name)
		msg := fmt.Sprintf("Revision '%s' is not ready and cannot serve traffic. %s", revName, why)
		if old != nil {
			msg += fmt.Sprintf("\nTraffic continues to be served by the previous revision (%s).", servingRevision(old))
		}
		return "", fmt.Errorf("%s", msg)
	}
	noTraffic := c.Bool("no-traffic")
	for i := range svc.Revisions {
		if !noTraffic {
			svc.Revisions[i].Traffic = 0
		}
	}
	traffic := 100
	if noTraffic && old != nil {
		traffic = 0
	}
	svc.Revisions = append(svc.Revisions, sim.Revision{Name: revName, Image: svc.Image, Env: copyStrMap(svc.Env), Secrets: copyStrMap(svc.Secrets), Traffic: traffic, Tag: c.Str("tag", "")})
	if noTraffic && old != nil {
		// keep serving config of old revision
		svc.Image, svc.Env, svc.Secrets = old.Image, old.Env, old.Secrets
	}
	out := fmt.Sprintf("Deploying container to Cloud Run service [%s] in project [%s] region [%s]\n", name, p.ID, region)
	if c.Bool("allow-unauthenticated") {
		if s.State.OrgPolicyEnforced(p.ID, "iam.allowedPolicyMemberDomains") {
			out += "WARNING: Setting IAM policy failed: FAILED_PRECONDITION: One or more users named in the policy do not belong to a permitted customer.\n"
		} else {
			svc.IAM.AddBinding("roles/run.invoker", "allUsers", nil)
		}
	}
	if c.Bool("no-allow-unauthenticated") {
		svc.IAM.RemoveBinding("roles/run.invoker", "allUsers")
	}
	p.RunServices[name] = svc
	c.Audit("run.googleapis.com", "google.cloud.run.v1.Services.ReplaceService", "namespaces/"+p.ID+"/services/"+name)
	out += "OK Deploying... Done.\n  OK Creating Revision...\n  OK Routing traffic...\n"
	if noTraffic {
		out += fmt.Sprintf("Done.\nService [%s] revision [%s] has been deployed and is serving 0 percent of traffic.\n", name, revName)
	} else {
		out += fmt.Sprintf("Done.\nService [%s] revision [%s] has been deployed and is serving 100 percent of traffic.\n", name, revName)
	}
	return out + "Service URL: " + svc.URL + "\n", nil
}

func servingRevision(s *sim.RunService) string {
	for _, r := range s.Revisions {
		if r.Traffic > 0 {
			return r.Name
		}
	}
	return "none"
}

func runView(svc *sim.RunService) map[string]any {
	return map[string]any{"metadata": map[string]any{"name": svc.Name, "labels": svc.Labels, "annotations": map[string]any{"run.googleapis.com/ingress": svc.Ingress}},
		"spec": map[string]any{"template": map[string]any{"spec": map[string]any{"serviceAccountName": svc.SA, "containerConcurrency": svc.Concurrency,
			"containers": []any{map[string]any{"image": svc.Image, "env": envList(svc.Env, svc.Secrets), "resources": map[string]any{"limits": map[string]any{"cpu": svc.CPU, "memory": svc.Memory}}}}},
			"metadata": map[string]any{"annotations": map[string]any{"autoscaling.knative.dev/minScale": svc.MinInstances, "autoscaling.knative.dev/maxScale": svc.MaxInstances, "run.googleapis.com/cloudsql-instances": strings.Join(svc.CloudSQL, ","), "run.googleapis.com/network-interfaces": svc.Network + "/" + svc.Subnet, "run.googleapis.com/vpc-access-connector": svc.VPCConnector}}}},
		"status": map[string]any{"url": svc.URL, "traffic": svc.Revisions, "latestReadyRevisionName": servingRevision(svc)},
		"region": svc.Region, "name": svc.Name, "url": svc.URL, "image": svc.Image, "serviceAccount": svc.SA, "ingress": svc.Ingress}
}

func envList(env, secrets map[string]string) []any {
	var out []any
	for _, k := range sim.SortedKeys(env) {
		out = append(out, map[string]any{"name": k, "value": env[k]})
	}
	for _, k := range sim.SortedKeys(secrets) {
		ref := secrets[k]
		name, ver, _ := strings.Cut(ref, ":")
		out = append(out, map[string]any{"name": k, "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": name, "key": ver}}})
	}
	return out
}

func init() {
	reg("run deploy", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "SERVICE")
		if err != nil {
			return nil, err
		}
		return c.S.deployRun(c, n, false)
	})
	reg("run services update", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "SERVICE")
		if err != nil {
			return nil, err
		}
		return c.S.deployRun(c, n, true)
	})
	reg("run services list", func(c *Cmd) (any, error) {
		if err := c.API("run.googleapis.com"); err != nil {
			return nil, err
		}
		p, _ := c.P()
		if err := c.NeedProject("run.services.list"); err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.RunServices) {
			svc := p.RunServices[k]
			ok := "✔"
			if svc.Image == "" {
				ok = "X"
			}
			rows = append(rows, map[string]any{"ok": ok, "name": svc.Name, "region": svc.Region, "url": svc.URL, "lastDeployer": "student", "image": svc.Image})
		}
		return Table{Cols: []Col{{"", "ok"}, {"SERVICE", "name"}, {"REGION", "region"}, {"URL", "url"}, {"LAST DEPLOYED BY", "lastDeployer"}}, Rows: rows}, nil
	})
	reg("run services describe", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "SERVICE")
		if err != nil {
			return nil, err
		}
		p, svc, err := c.runService(n)
		if err != nil {
			return nil, err
		}
		if err := c.Need("run.services.get", runRes(p, svc)); err != nil {
			return nil, err
		}
		if c.Str("format", "") == "" {
			var b strings.Builder
			fmt.Fprintf(&b, "✔ Service %s in region %s\n\nURL:     %s\nIngress: %s\nTraffic:\n", svc.Name, svc.Region, svc.URL, svc.Ingress)
			for _, r := range svc.Revisions {
				if r.Traffic > 0 {
					fmt.Fprintf(&b, "  %d%% %s\n", r.Traffic, r.Name)
				}
			}
			fmt.Fprintf(&b, "\nLast updated on %s:\n  Revision %s\n  Container None\n    Image:           %s\n    Port:            %d\n    Memory:          %s\n    CPU:             %s\n", c.S.State.Now(), servingRevision(svc), svc.Image, svc.Port, svc.Memory, svc.CPU)
			if len(svc.Env) > 0 {
				b.WriteString("    Env vars:\n")
				for _, k := range sim.SortedKeys(svc.Env) {
					fmt.Fprintf(&b, "      %-16s %s\n", k, svc.Env[k])
				}
			}
			if len(svc.Secrets) > 0 {
				b.WriteString("    Secrets:\n")
				for _, k := range sim.SortedKeys(svc.Secrets) {
					fmt.Fprintf(&b, "      %-16s %s\n", k, svc.Secrets[k])
				}
			}
			fmt.Fprintf(&b, "  Service account:   %s\n  Concurrency:       %d\n  Min instances:     %d\n  Max instances:     %d\n", svc.SA, svc.Concurrency, svc.MinInstances, svc.MaxInstances)
			if len(svc.CloudSQL) > 0 {
				fmt.Fprintf(&b, "  SQL connections:   %s\n", strings.Join(svc.CloudSQL, ","))
			}
			if svc.Network != "" || svc.VPCConnector != "" {
				fmt.Fprintf(&b, "  VPC access:        network=%s subnet=%s connector=%s egress=%s\n", svc.Network, svc.Subnet, svc.VPCConnector, svc.VPCEgress)
			}
			return b.String(), nil
		}
		return Obj{V: runView(svc)}, nil
	})
	reg("run services delete", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "SERVICE")
		if err != nil {
			return nil, err
		}
		p, svc, err := c.runService(n)
		if err != nil {
			return nil, err
		}
		if err := c.Need("run.services.delete", runRes(p, svc)); err != nil {
			return nil, err
		}
		delete(p.RunServices, n)
		c.Audit("run.googleapis.com", "google.cloud.run.v1.Services.DeleteService", "namespaces/"+p.ID+"/services/"+n)
		return "Deleted service [" + n + "].\n", nil
	})
	reg("run services get-iam-policy", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "SERVICE")
		if err != nil {
			return nil, err
		}
		_, svc, err := c.runService(n)
		if err != nil {
			return nil, err
		}
		return policyView(&svc.IAM), nil
	})
	runIAM := func(add bool) handler {
		return func(c *Cmd) (any, error) {
			n, err := c.Arg(0, "SERVICE")
			if err != nil {
				return nil, err
			}
			p, svc, err := c.runService(n)
			if err != nil {
				return nil, err
			}
			if err := c.Need("run.services.setIamPolicy", runRes(p, svc)); err != nil {
				return nil, err
			}
			m, err := member(c.Str("member", ""))
			if err != nil {
				return nil, err
			}
			role := c.Str("role", "")
			if !strings.HasPrefix(role, "roles/") {
				role = "roles/" + role
			}
			if add {
				if m == "allUsers" && c.S.State.OrgPolicyEnforced(p.ID, "iam.allowedPolicyMemberDomains") {
					return nil, fmt.Errorf("FAILED_PRECONDITION: One or more users named in the policy do not belong to a permitted customer.")
				}
				svc.IAM.AddBinding(role, m, nil)
			} else if !svc.IAM.RemoveBinding(role, m) {
				return nil, fmt.Errorf("Policy binding with the specified principal, role, and condition not found!")
			}
			c.Audit("run.googleapis.com", "google.cloud.run.v1.Services.SetIamPolicy", "projects/"+p.ID+"/locations/"+svc.Region+"/services/"+n)
			r, _ := render(policyView(&svc.IAM), Flags{})
			return "Updated IAM policy for service [" + n + "].\n" + r, nil
		}
	}
	reg("run services add-iam-policy-binding", runIAM(true))
	reg("run services remove-iam-policy-binding", runIAM(false))
	reg("run services update-traffic", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "SERVICE")
		if err != nil {
			return nil, err
		}
		p, svc, err := c.runService(n)
		if err != nil {
			return nil, err
		}
		if err := c.Need("run.services.update", runRes(p, svc)); err != nil {
			return nil, err
		}
		target := map[string]int{}
		if c.Bool("to-latest") {
			for i := len(svc.Revisions) - 1; i >= 0; i-- {
				if svc.Revisions[i].Tag != "FAILED" {
					target[svc.Revisions[i].Name] = 100
					break
				}
			}
		}
		for k, v := range c.KV("to-revisions") {
			var n int
			fmt.Sscanf(v, "%d", &n)
			if k == "LATEST" {
				k = svc.Revisions[len(svc.Revisions)-1].Name
			}
			target[k] = n
		}
		sum := 0
		for _, v := range target {
			sum += v
		}
		if sum != 100 {
			return nil, fmt.Errorf("Traffic percentages must sum to 100 (got %d).", sum)
		}
		best, bestPct := "", -1
		for i := range svc.Revisions {
			r := &svc.Revisions[i]
			r.Traffic = target[r.Name]
			if r.Tag == "FAILED" && r.Traffic > 0 {
				return nil, fmt.Errorf("Revision %s is not ready and cannot serve traffic.", r.Name)
			}
			if r.Traffic > bestPct {
				best, bestPct = r.Name, r.Traffic
			}
		}
		for _, r := range svc.Revisions {
			if r.Name == best {
				svc.Image, svc.Env, svc.Secrets = r.Image, copyStrMap(r.Env), copyStrMap(r.Secrets)
			}
		}
		c.Audit("run.googleapis.com", "google.cloud.run.v1.Services.ReplaceService", "namespaces/"+p.ID+"/services/"+n)
		return "OK Updating traffic... Done.\nTraffic: " + fmt.Sprint(target) + "\n", nil
	})
	reg("run revisions list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		names := sim.SortedKeys(p.RunServices)
		if s := c.Str("service", ""); s != "" {
			names = []string{s}
		}
		for _, n := range names {
			svc := p.RunServices[n]
			if svc == nil {
				continue
			}
			for i := len(svc.Revisions) - 1; i >= 0; i-- {
				r := svc.Revisions[i]
				ok := "✔"
				if r.Tag == "FAILED" {
					ok = "X"
				}
				rows = append(rows, map[string]any{"ok": ok, "name": r.Name, "active": map[bool]string{true: "yes", false: ""}[r.Traffic > 0], "service": n, "image": r.Image, "traffic": r.Traffic})
			}
		}
		return Table{Cols: []Col{{"", "ok"}, {"REVISION", "name"}, {"ACTIVE", "active"}, {"SERVICE", "service"}, {"IMAGE", "image"}}, Rows: rows}, nil
	})
	reg("run services logs read", func(c *Cmd) (any, error) {
		n, _ := c.Arg(0, "SERVICE")
		logs := c.S.State.QueryLogs(c.ProjectID(), `resource.type="cloud_run_revision" resource.labels.service_name="`+n+`"`, c.Int("limit", 20))
		var b strings.Builder
		for _, e := range logs {
			line := e.Text
			if e.HTTP != nil {
				line = fmt.Sprintf("%s %d %s", e.HTTP.Method, e.HTTP.Status, e.HTTP.URL)
			}
			fmt.Fprintf(&b, "%s %s %s\n", e.Timestamp, e.Severity, line)
		}
		return b.String(), nil
	})
}

// buildFromSource performs a Cloud Build of the workspace (Dockerfile) into
// Artifact Registry and returns the resulting image reference.
func (s *Session) buildFromSource(c *Cmd, p *sim.Project, region, name string) (string, error) {
	df, ok := s.Files["Dockerfile"]
	if !ok {
		return "", fmt.Errorf("Deploying from source requires a Dockerfile (or a supported buildpack project) in the source directory.")
	}
	repo := p.ArtifactRepos["cloud-run-source-deploy"]
	if repo == nil {
		repo = &sim.ArtifactRepo{Name: "cloud-run-source-deploy", Location: region, Format: "DOCKER", Images: map[string][]string{}}
		p.ArtifactRepos[repo.Name] = repo
	}
	img := fmt.Sprintf("%s-docker.pkg.dev/%s/cloud-run-source-deploy/%s", region, p.ID, name)
	tag := s.State.ID(7)
	repo.Images[name] = append(repo.Images[name], tag, "latest")
	behavior := behaviorFromDockerfile(df)
	s.State.Extra["image:"+img+":"+tag] = behavior
	s.State.Extra["image:"+img+":latest"] = behavior
	s.State.Extra["image:"+img] = behavior
	p.Builds = append(p.Builds, &sim.Build{ID: s.State.ID(8) + "-" + s.State.ID(4), Status: "SUCCESS", Source: "local", Images: []string{img + ":" + tag}, Created: s.State.Now()})
	return img + ":" + tag, nil
}

func behaviorFromDockerfile(df string) string {
	for _, line := range strings.Split(df, "\n") {
		if m := regexp.MustCompile(`(?i)LABEL\s+sim\.behavior=["']?([a-z0-9:.-]+)`).FindStringSubmatch(line); m != nil {
			return m[1]
		}
	}
	return "hello"
}

func sortedRevisions(s *sim.RunService) []sim.Revision {
	r := append([]sim.Revision{}, s.Revisions...)
	sort.Slice(r, func(i, j int) bool { return r[i].Name < r[j].Name })
	return r
}
