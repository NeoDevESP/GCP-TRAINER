package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

// Mentor builtins (Blueprint §13):
//
//	why TARGET [--from=vm:NAME] [--path=/x] [--as=EMAIL]
//	    Teach me why: explain the causal chain of a request
//	    (DNS → IP → TCP → LB → firewall → process → application → dependencies).
//	whatif COMMAND...
//	    What happens if...? Run a command on a copy of the world and report
//	    its impact (resources, service health, security findings, cost)
//	    before doing it for real.

var layerAdvice = map[string]string{
	"dns":            "check Cloud DNS records and the hostname (gcloud dns record-sets list)",
	"load-balancer":  "check forwarding rule ports, target proxy and URL map (gcloud compute url-maps describe)",
	"cloud-armor":    "review the security policy rules (gcloud compute security-policies describe)",
	"backend-health": "check health-check path/port, named ports and firewall rules for 35.191.0.0/16 and 130.211.0.0/22 (gcloud compute backend-services get-health)",
	"network":        "check firewall rules (priority, direction, tags), routes and NAT (gcloud compute firewall-rules list)",
	"process":        "SSH in and check the service and its bind address (systemctl status, ss -ltnp)",
	"ingress":        "check the Cloud Run ingress setting",
	"iam":            "check who may invoke/access the resource (get-iam-policy, policy troubleshooter)",
	"workload":       "check the revision configuration: image, service account, secrets (gcloud run services describe)",
	"application":    "read the application logs and the deployed version (gcloud logging read)",
	"dependency":     "check the dependency itself and the permissions/network path of the workload's identity",
}

func (s *Session) whyCmd(args []string) (string, error) {
	pos, f := parseArgs(args[1:])
	if len(pos) == 0 {
		return "", fail(2, "usage: why URL|lb:NAME|run:NAME|vm:NAME:PORT|k8s:CLUSTER/SVC [--path=/x] [--from=vm:NAME] [--as=EMAIL]")
	}
	target := pos[0]
	path := firstOr(f["path"], "")
	url := target
	if strings.Contains(target, ":") && !strings.Contains(target, "://") && !strings.Contains(target, ".") {
		if path == "" {
			path = "/"
		}
		url = s.State.ResolveTrafficURL(s.Project, target, path)
		if url == "" {
			return "", fail(1, "unknown target %s in project %s", target, s.Project)
		}
	} else if path != "" {
		url = strings.TrimSuffix(url, "/") + path
	}
	req := sim.HTTPRequest{URL: url, SourceIP: sim.StudentIP}
	if from := firstOr(f["from"], ""); strings.HasPrefix(from, "vm:") {
		p := s.State.Projects[s.Project]
		vm := p.Instances[strings.TrimPrefix(from, "vm:")]
		if vm == nil {
			return "", fail(1, "instance %s not found", from)
		}
		req.From = s.State.VMEndpoint(s.Project, vm)
		req.Principal = "serviceAccount:" + vm.ServiceAccount
	}
	if as := firstOr(f["as"], ""); as != "" {
		req.Principal = "user:" + as
		if strings.HasSuffix(as, "gserviceaccount.com") {
			req.Principal = "serviceAccount:" + as
		}
	}
	st := s.State.Clone() // explaining never changes the world
	_, hops := st.ExplainHTTP(req)
	var b strings.Builder
	fmt.Fprintf(&b, "Causal chain for %s\n", url)
	var broken *sim.Hop
	for i := range hops {
		h := hops[i]
		mark := "✓"
		if h.Status == "fail" {
			mark = "✗"
			if broken == nil && h.Layer != "result" {
				broken = &hops[i]
			}
		}
		fmt.Fprintf(&b, "  %s %-15s %s\n", mark, h.Layer, h.Detail)
	}
	if broken != nil {
		fmt.Fprintf(&b, "\nFirst broken link: %s — %s\n", broken.Layer, broken.Detail)
		if a := layerAdvice[broken.Layer]; a != "" {
			fmt.Fprintf(&b, "Where to look: %s\n", a)
		}
		b.WriteString("Why this matters: every later hop depends on this one; fixing symptoms further down the chain will not help.\n")
	}
	return b.String(), nil
}

func firstOr(v []string, def string) string {
	if len(v) > 0 && v[0] != "" {
		return v[0]
	}
	return def
}

// probeTargets lists request targets to compare before/after a change.
func (s *Session) probeTargets() []string {
	seen := map[string]bool{}
	var out []string
	add := func(t string) {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for _, t := range s.State.Traffic {
		if t.Project == "" || t.Project == s.Project {
			add(t.Target + "|" + firstNonEmptyStr(t.Path, "/"))
		}
	}
	if p := s.State.Projects[s.Project]; p != nil {
		for _, n := range sim.SortedKeys(p.ForwardingRules) {
			add("lb:" + n + "|/")
		}
		for _, n := range sim.SortedKeys(p.RunServices) {
			add("run:" + n + "|/")
		}
	}
	sort.Strings(out)
	return out
}

func firstNonEmptyStr(v ...string) string {
	for _, x := range v {
		if x != "" {
			return x
		}
	}
	return ""
}

func probe(st *sim.State, project, target, path string) int {
	url := st.ResolveTrafficURL(project, target, path)
	if url == "" {
		return 0
	}
	r := st.HTTP(sim.HTTPRequest{URL: url, SourceIP: sim.StudentIP})
	return r.Status
}

func (s *Session) whatifCmd(args []string, stdin string) (string, error) {
	if len(args) < 2 {
		return "", fail(2, "usage: whatif COMMAND... (e.g. whatif gcloud compute firewall-rules delete allow-health)")
	}
	line := shellJoin(args[1:])
	before := s.State.Clone()
	after := s.State.Clone()
	cs := s.Clone(after)
	cs.Records = nil
	res := cs.Exec(line)
	after.Step(3) // let health checks, autoscalers and traffic react
	before.Step(3)
	var b strings.Builder
	fmt.Fprintf(&b, "What if you run: %s\n(simulated on a copy of the environment — nothing was changed)\n\n", line)
	if res.Exit != 0 {
		fmt.Fprintf(&b, "The command would FAIL (exit %d):\n%s\n", res.Exit, indent(res.Output))
		return b.String(), nil
	}
	// resource changes
	bv, av := before.ProjectView(s.Project), after.ProjectView(s.Project)
	var changes []string
	for _, k := range sim.SortedKeys(av) {
		if k == "cost" || k == "findings" || k == "backendHealth" || k == "pods" {
			continue
		}
		bm, ok1 := bv[k].(map[string]any)
		am, ok2 := av[k].(map[string]any)
		if !ok1 || !ok2 {
			continue
		}
		for _, n := range sim.SortedKeys(bm) {
			if _, ok := am[n]; !ok {
				changes = append(changes, fmt.Sprintf("  - %s %s would be DELETED", k, n))
			} else if fmt.Sprint(am[n]) != fmt.Sprint(bm[n]) {
				changes = append(changes, fmt.Sprintf("  ~ %s %s would be modified", k, n))
			}
		}
		for _, n := range sim.SortedKeys(am) {
			if _, ok := bm[n]; !ok {
				changes = append(changes, fmt.Sprintf("  + %s %s would be created", k, n))
			}
		}
	}
	if len(changes) == 0 {
		changes = []string{"  (no resource changes in " + s.Project + ")"}
	}
	b.WriteString("Resources:\n" + strings.Join(changes, "\n") + "\n\n")
	// service impact
	var impact []string
	for _, t := range s.probeTargets() {
		tp := strings.SplitN(t, "|", 2)
		x, y := probe(before, s.Project, tp[0], tp[1]), probe(after, s.Project, tp[0], tp[1])
		if x != y {
			verdict := "changes"
			if x < 400 && (y >= 400 || y == 0) {
				verdict = "BREAKS"
			} else if (x >= 400 || x == 0) && y < 400 && y != 0 {
				verdict = "FIXES"
			}
			impact = append(impact, fmt.Sprintf("  %s %s%s: HTTP %s → %s", verdict, tp[0], tp[1], code(x), code(y)))
		}
	}
	if len(impact) == 0 {
		impact = []string{"  no change in the availability of known services"}
	}
	b.WriteString("Service impact:\n" + strings.Join(impact, "\n") + "\n\n")
	// security findings
	bf, af := findingSet(before, s.Project), findingSet(after, s.Project)
	var sec []string
	for k := range af {
		if !bf[k] {
			sec = append(sec, "  + NEW finding "+k)
		}
	}
	for k := range bf {
		if !af[k] {
			sec = append(sec, "  - resolves finding "+k)
		}
	}
	sort.Strings(sec)
	if len(sec) == 0 {
		sec = []string{"  no change in security findings"}
	}
	b.WriteString("Security posture:\n" + strings.Join(sec, "\n") + "\n\n")
	_, c0 := before.CostEstimate(s.Project)
	_, c1 := after.CostEstimate(s.Project)
	fmt.Fprintf(&b, "Cost: %.2f → %.2f EUR/month (%+.2f)\n", c0, c1, c1-c0)
	if strings.Contains(strings.Join(impact, ""), "BREAKS") {
		b.WriteString("\n⚠ This change would break a running service. Consider a safer alternative or a rollback plan.\n")
	}
	return b.String(), nil
}

func code(c int) string {
	if c == 0 {
		return "no response"
	}
	return fmt.Sprint(c)
}

func findingSet(st *sim.State, project string) map[string]bool {
	out := map[string]bool{}
	for _, f := range st.Findings(project) {
		out[f.Category+" "+f.Resource[strings.LastIndex(f.Resource, "/")+1:]] = true
	}
	return out
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n  ")
}

func shellJoin(args []string) string {
	var out []string
	for _, a := range args {
		if strings.ContainsAny(a, " '\"$&|;<>*") {
			out = append(out, "'"+strings.ReplaceAll(a, "'", `'\''`)+"'")
		} else {
			out = append(out, a)
		}
	}
	return strings.Join(out, " ")
}
