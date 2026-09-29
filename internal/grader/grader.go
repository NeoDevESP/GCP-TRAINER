// Package grader scores a lab attempt with several independent validators:
// resource state, functional probes, security policy (OPA/Rego), cost and
// hygiene, and evidence/explanation. It grades observable outcomes, not the
// sequence of clicks: any valid path (console, gcloud, Terraform, API) passes.
package grader

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/neodevesp/gcp-trainer/internal/archsim"
	"gopkg.in/yaml.v3"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/neodevesp/gcp-trainer/internal/cli"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
	"github.com/neodevesp/gcp-trainer/internal/sim"
	"github.com/open-policy-agent/opa/v1/rego"
)

// Submission carries the student's non-infrastructure answers.
type Submission struct {
	Evidence       map[string]string `json:"evidence"`       // rootCause, fix, prevention, postmortem, explanation...
	Answers        map[string][]int  `json:"answers"`        // quiz id -> selected options
	Justifications map[string]string `json:"justifications"` // quiz id -> text
	HintsUsed      int               `json:"hintsUsed"`      // set by the platform, never by the learner
}

// CheckResult is the outcome of one assertion.
type CheckResult struct {
	Type   string `json:"type"`
	Desc   string `json:"desc"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail,omitempty"`
}

// ItemResult is one rubric criterion.
type ItemResult struct {
	Name      string        `json:"name"`
	Validator string        `json:"validator"`
	Points    int           `json:"points"`
	Earned    float64       `json:"earned"`
	Critical  bool          `json:"critical"`
	Checks    []CheckResult `json:"checks"`
}

// Result is the full grading report.
type Result struct {
	LabID          string                `json:"labId"`
	Score          int                   `json:"score"`
	Max            int                   `json:"max"`
	Passed         bool                  `json:"passed"`
	CriticalFailed bool                  `json:"criticalFailed"`
	Items          []ItemResult          `json:"items"`
	Validators     map[string][2]float64 `json:"validators"` // earned, max
	Feedback       []string              `json:"feedback"`
	Process        *ProcessReport        `json:"process,omitempty"`
}

// Context is everything the grader can observe.
type Context struct {
	Lab     *scenario.Lab
	State   *sim.State // a clone; checks may step time
	Session *cli.Session
	Project string
	Sub     Submission
	view    map[string]any
}

// Grade evaluates the rubric. The state is cloned so grading never mutates
// the student's environment.
func Grade(lab *scenario.Lab, st *sim.State, sess *cli.Session, project string, sub Submission) *Result {
	clone := st.Clone()
	cs := sess.Clone(clone)
	ctx := &Context{Lab: lab, State: clone, Session: cs, Project: project, Sub: sub}
	res := &Result{LabID: lab.ID, Validators: map[string][2]float64{}}
	for _, item := range lab.Rubric {
		ir := ItemResult{Name: item.Name, Validator: item.Validator, Points: item.Points, Critical: item.Critical}
		if ir.Validator == "" {
			ir.Validator = "state"
		}
		passed := 0
		for _, ch := range item.Checks {
			cr := ctx.run(ch)
			ir.Checks = append(ir.Checks, cr)
			if cr.Pass {
				passed++
			}
		}
		if len(item.Checks) > 0 {
			frac := float64(passed) / float64(len(item.Checks))
			if item.All && passed < len(item.Checks) {
				frac = 0
			}
			ir.Earned = math.Round(float64(item.Points)*frac*10) / 10
		}
		if item.Critical && passed < len(item.Checks) {
			res.CriticalFailed = true
		}
		v := res.Validators[ir.Validator]
		v[0] += ir.Earned
		v[1] += float64(item.Points)
		res.Validators[ir.Validator] = v
		res.Items = append(res.Items, ir)
		res.Max += item.Points
	}
	total := 0.0
	for _, it := range res.Items {
		total += it.Earned
	}
	res.Score = int(math.Round(total))
	res.Passed = res.Score >= lab.PassScore && !res.CriticalFailed
	for _, it := range res.Items {
		for _, c := range it.Checks {
			if !c.Pass && c.Desc != "" {
				res.Feedback = append(res.Feedback, fmt.Sprintf("[%s] %s", it.Name, c.Desc))
			}
		}
	}
	res.Process = AssessProcess(lab, sess, res, sub, sub.HintsUsed)
	for _, v := range res.Process.Violations {
		res.Feedback = append(res.Feedback, "[Process] "+v)
	}
	if n := len(res.Process.BlindFixes); n > 0 {
		res.Feedback = append(res.Feedback, fmt.Sprintf("[Process] %d change(s) made before gathering evidence — form a hypothesis first", n))
	}
	return res
}

func (c *Context) View() map[string]any {
	if c.view == nil {
		c.view = c.State.ProjectView(c.Project)
	}
	return c.view
}

func str(ch scenario.Check, k string) string {
	if v, ok := ch[k]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

func num(ch scenario.Check, k string, def float64) float64 {
	if v, ok := ch[k]; ok && v != nil {
		switch x := v.(type) {
		case int:
			return float64(x)
		case float64:
			return x
		default:
			f, err := strconv.ParseFloat(fmt.Sprint(v), 64)
			if err == nil {
				return f
			}
		}
	}
	return def
}

func boolean(ch scenario.Check, k string, def bool) bool {
	if v, ok := ch[k].(bool); ok {
		return v
	}
	return def
}

func list(ch scenario.Check, k string) []string {
	var out []string
	switch v := ch[k].(type) {
	case []any:
		for _, e := range v {
			out = append(out, fmt.Sprint(e))
		}
	case string:
		out = []string{v}
	}
	return out
}

// normalizeCheck converts nested YAML maps into plain map[string]any.
func normalizeCheck(ch scenario.Check) scenario.Check {
	b, err := json.Marshal(ch)
	if err != nil {
		return ch
	}
	var out map[string]any
	if json.Unmarshal(b, &out) != nil {
		return ch
	}
	return scenario.Check(out)
}

func (c *Context) run(ch scenario.Check) (res CheckResult) {
	ch = normalizeCheck(ch)
	res.Type = str(ch, "type")
	res.Desc = str(ch, "desc")
	defer func() {
		if r := recover(); r != nil {
			res.Pass = false
			res.Detail = fmt.Sprintf("grader error: %v", r)
		}
	}()
	if n := int(num(ch, "settle", 0)); n > 0 {
		c.State.Step(n)
		c.view = nil
	}
	// "on" evaluates the check against another project of the same world
	// (multi-project company missions).
	if p := str(ch, "on"); p != "" && p != c.Project && c.State.Projects[p] != nil {
		savedP, savedV := c.Project, c.view
		c.Project, c.view = p, nil
		defer func() { c.Project, c.view = savedP, savedV }()
	}
	var ok bool
	var detail string
	switch res.Type {
	case "exists":
		ok, detail = c.exists(ch)
	case "absent":
		_, found := resolve(c.View(), str(ch, "path"))
		ok, detail = !found, "path "+str(ch, "path")
	case "count":
		ok, detail = c.count(ch)
	case "http":
		ok, detail = c.http(ch)
	case "tcp":
		ok, detail = c.tcp(ch)
	case "egress":
		ok, detail = c.egress(ch)
	case "google_api":
		ok, detail = c.googleAPI(ch)
	case "iam":
		ok, detail = c.iam(ch)
	case "no_basic_roles":
		ok, detail = c.noBasicRoles(ch)
	case "forbid_firewall":
		ok, detail = c.forbidFirewall(ch)
	case "finding_absent":
		ok, detail = c.finding(ch, false)
	case "finding_present":
		ok, detail = c.finding(ch, true)
	case "cost_max":
		_, total := c.State.CostEstimate(c.Project)
		ok, detail = total <= num(ch, "eur", 0), fmt.Sprintf("estimated %.2f EUR/month (limit %.2f)", total, num(ch, "eur", 0))
	case "cost_min":
		_, total := c.State.CostEstimate(c.Project)
		ok, detail = total >= num(ch, "eur", 0), fmt.Sprintf("estimated %.2f EUR/month", total)
	case "log_metric":
		ok, detail = c.logMetric(ch)
	case "alert_policy":
		ok, detail = c.alertPolicy(ch)
	case "evidence":
		ok, detail = c.evidence(ch)
	case "quiz":
		ok, detail = c.quiz(ch)
	case "command":
		ok, detail = c.command(ch)
	case "bq_bytes_max":
		ok, detail = c.bqBytes(ch)
	case "terraform_clean":
		ok, detail = c.terraformClean(ch)
	case "k8s_ready":
		ok, detail = c.k8sReady(ch)
	case "hpa_scaled":
		ok, detail = c.hpaScaled(ch)
	case "pubsub":
		ok, detail = c.pubsub(ch)
	case "build":
		ok, detail = c.build(ch)
	case "sql_healthy":
		ok, detail = c.sqlHealthy(ch)
	case "policy":
		ok, detail = c.policy(ch)
	case "rollout":
		ok, detail = c.rollout(ch)
	case "secret_rotated":
		sec := c.State.Projects[c.Project].Secrets[str(ch, "secret")]
		ok, detail = false, "secret not found"
		if sec != nil {
			enabled := 0
			ok, detail = true, "exposed value no longer served"
			for _, v := range sec.Versions {
				if v.State == "ENABLED" {
					enabled++
					if v.Data == str(ch, "exposedValue") {
						ok, detail = false, fmt.Sprintf("version %d still contains the exposed value", v.ID)
					}
				}
			}
			if enabled == 0 {
				ok, detail = false, "no enabled version"
			}
		}
	case "session_config":
		ok, detail = true, fmt.Sprintf("project=%s region=%s zone=%s", c.Session.Project, c.Session.Region, c.Session.Zone)
		if v := str(ch, "project"); v != "" && c.Session.Project != v {
			ok = false
		}
		if v := str(ch, "region"); v != "" && c.Session.Region != v {
			ok = false
		}
		if v := str(ch, "zone"); v != "" && c.Session.Zone != v {
			ok = false
		}
	case "log_contains":
		ok, detail = c.logContains(ch)
	case "k8s_rbac":
		cl, ns := c.cluster(ch)
		if cl == nil || cl.K8s == nil {
			ok, detail = false, "cluster not found"
			break
		}
		allowed := cl.K8s.NS(ns).RBACAllows(ns, str(ch, "serviceAccount"), str(ch, "verb"), str(ch, "resource"))
		ok = allowed == boolean(ch, "expect", true)
		detail = fmt.Sprintf("system:serviceaccount:%s:%s can %s %s: %v", ns, str(ch, "serviceAccount"), str(ch, "verb"), str(ch, "resource"), allowed)
	case "chaos":
		ok, detail = false, "no matching chaos experiment"
		for _, r := range c.Session.Chaos {
			if (str(ch, "experiment") == "" || r.Experiment == str(ch, "experiment")) && r.SLO >= num(ch, "minSlo", 0) && r.Minutes >= int(num(ch, "minMinutes", 1)) {
				if r.Held || !boolean(ch, "held", true) {
					ok, detail = true, fmt.Sprintf("%s held at %.2f%% (SLO %.1f%%)", r.Experiment, r.Availability, r.SLO)
					break
				}
				detail = fmt.Sprintf("%s failed: %.2f%% < SLO %.1f%%", r.Experiment, r.Availability, r.SLO)
			}
		}
	case "design":
		ok, detail = c.design(ch)
	case "tf_state":
		ok, detail = c.tfState(ch)
	case "file_contains":
		content, exists := c.Session.Files[str(ch, "file")]
		re, err := regexp.Compile(str(ch, "regex"))
		switch {
		case !exists:
			ok, detail = false, "file "+str(ch, "file")+" not found"
		case err != nil:
			ok, detail = false, "bad regex"
		default:
			ok, detail = re.MatchString(content), "file "+str(ch, "file")
		}
	case "org_policy":
		op := c.State.EffectiveOrgPolicy(c.Project, strings.TrimPrefix(str(ch, "constraint"), "constraints/"))
		ok, detail = op != nil && op.Enforce == boolean(ch, "enforced", true), "no effective policy"
		if op != nil {
			detail = fmt.Sprintf("effective policy enforce=%v", op.Enforce)
		}
	case "ticket_update", "ticket_resolved", "asked":
		ok, detail = c.deskCheck(res.Type, ch)
	default:
		return CheckResult{Type: res.Type, Desc: res.Desc, Detail: "unknown check type"}
	}
	if boolean(ch, "not", false) {
		ok = !ok
	}
	res.Pass, res.Detail = ok, detail
	return res
}

// ---- path resolution & expectations ---------------------------------------------

var rePathSeg = regexp.MustCompile(`\["([^"]+)"\]|\[(\d+)\]|([^.\[\]]+)`)

func resolve(v any, path string) (any, bool) {
	if path == "" {
		return v, true
	}
	cur := v
	for _, m := range rePathSeg.FindAllStringSubmatch(path, -1) {
		key := m[1] + m[3]
		switch x := cur.(type) {
		case map[string]any:
			n, ok := x[key]
			if !ok {
				return nil, false
			}
			cur = n
		case []any:
			if m[2] != "" {
				i, _ := strconv.Atoi(m[2])
				if i >= len(x) {
					return nil, false
				}
				cur = x[i]
			} else {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	return cur, cur != nil
}

func scalarStr(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case []any, map[string]any:
		b, _ := json.Marshal(x)
		return string(b)
	}
	return fmt.Sprint(v)
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	}
	f, err := strconv.ParseFloat(fmt.Sprint(v), 64)
	return f, err == nil
}

func asList(v any) []string {
	var out []string
	if l, ok := v.([]any); ok {
		for _, e := range l {
			out = append(out, scalarStr(e))
		}
	}
	return out
}

// expect evaluates an expectation against a value.
func expect(got any, want any) (bool, string) {
	if m, ok := want.(map[string]any); ok {
		for op, w := range m {
			ok, why := expectOp(got, op, w)
			if !ok {
				return false, why
			}
		}
		return true, ""
	}
	g, w := scalarStr(got), scalarStr(want)
	if strings.EqualFold(g, w) {
		return true, ""
	}
	if gf, ok1 := toFloat(got); ok1 {
		if wf, ok2 := toFloat(want); ok2 && gf == wf {
			return true, ""
		}
	}
	return false, fmt.Sprintf("got %s, want %s", trunc(g), w)
}

func trunc(s string) string {
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}

func expectOp(got any, op string, w any) (bool, string) {
	gs, ws := scalarStr(got), scalarStr(w)
	gf, gok := toFloat(got)
	wf, wok := toFloat(w)
	switch op {
	case "eq":
		return expect(got, w)
	case "ne":
		ok, _ := expect(got, w)
		return !ok, fmt.Sprintf("value must not be %s", ws)
	case "gt":
		return gok && wok && gf > wf, fmt.Sprintf("got %s, want > %s", gs, ws)
	case "gte":
		return gok && wok && gf >= wf, fmt.Sprintf("got %s, want >= %s", gs, ws)
	case "lt":
		return gok && wok && gf < wf, fmt.Sprintf("got %s, want < %s", gs, ws)
	case "lte":
		return gok && wok && gf <= wf, fmt.Sprintf("got %s, want <= %s", gs, ws)
	case "contains":
		if l := asList(got); l != nil {
			for _, x := range l {
				if strings.EqualFold(x, ws) {
					return true, ""
				}
			}
			return false, fmt.Sprintf("%s does not contain %s", trunc(gs), ws)
		}
		return strings.Contains(strings.ToLower(gs), strings.ToLower(ws)), fmt.Sprintf("%q does not contain %q", trunc(gs), ws)
	case "notContains":
		ok, _ := expectOp(got, "contains", w)
		return !ok, fmt.Sprintf("%s must not contain %s", trunc(gs), ws)
	case "in":
		for _, x := range asList(w) {
			if strings.EqualFold(x, gs) {
				return true, ""
			}
		}
		return false, fmt.Sprintf("%s not in %s", gs, ws)
	case "regex":
		re, err := regexp.Compile(ws)
		return err == nil && re.MatchString(gs), fmt.Sprintf("%q does not match /%s/", trunc(gs), ws)
	case "len":
		n := len(asList(got))
		if m, ok := got.(map[string]any); ok {
			n = len(m)
		}
		return expect(n, w)
	case "minLen":
		n := len(asList(got))
		if m, ok := got.(map[string]any); ok {
			n = len(m)
		}
		return float64(n) >= wf, fmt.Sprintf("has %d elements, want >= %s", n, ws)
	case "maxLen":
		n := len(asList(got))
		if m, ok := got.(map[string]any); ok {
			n = len(m)
		}
		return float64(n) <= wf, fmt.Sprintf("has %d elements, want <= %s", n, ws)
	case "empty":
		empty := gs == "" || gs == "[]" || gs == "{}" || gs == "null" || gs == "false" || gs == "0"
		want := ws == "true"
		return empty == want, fmt.Sprintf("emptiness of %s", trunc(gs))
	}
	return false, "unknown operator " + op
}

func (c *Context) exists(ch scenario.Check) (bool, string) {
	v, ok := resolve(c.View(), str(ch, "path"))
	if !ok {
		return false, "resource " + str(ch, "path") + " not found"
	}
	if exp, ok := ch["expect"].(map[string]any); ok {
		for _, k := range sortedKeys(exp) {
			got, _ := resolve(v, k)
			if ok, why := expect(got, exp[k]); !ok {
				return false, k + ": " + why
			}
		}
	}
	return true, "ok"
}

func sortedKeys(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (c *Context) count(ch scenario.Check) (bool, string) {
	coll, ok := resolve(c.View(), str(ch, "collection"))
	if !ok {
		coll = map[string]any{}
	}
	var items []any
	switch x := coll.(type) {
	case map[string]any:
		for _, k := range sortedKeys(x) {
			items = append(items, x[k])
		}
	case []any:
		items = x
	}
	n := 0
	where, _ := ch["where"].(map[string]any)
	for _, it := range items {
		match := true
		for _, k := range sortedKeys(where) {
			got, _ := resolve(it, k)
			if ok, _ := expect(got, where[k]); !ok {
				match = false
				break
			}
		}
		if match {
			n++
		}
	}
	mn, mx := num(ch, "min", 0), num(ch, "max", math.MaxInt32)
	return float64(n) >= mn && float64(n) <= mx, fmt.Sprintf("%d matching %s (want %v..%v)", n, str(ch, "collection"), mn, mx)
}

// endpoint parses "internet", "vm:NAME", "pod:CLUSTER/NS/DEPLOY", "run:SVC".
func (c *Context) endpoint(ref string) (sim.Endpoint, string, error) {
	p := c.State.Projects[c.Project]
	switch {
	case ref == "" || ref == "internet":
		return sim.InternetEndpoint(""), "", nil
	case strings.HasPrefix(ref, "vm:"):
		vm := p.Instances[strings.TrimPrefix(ref, "vm:")]
		if vm == nil {
			return sim.Endpoint{}, "", fmt.Errorf("vm %s not found", ref)
		}
		return c.State.VMEndpoint(c.Project, vm), "serviceAccount:" + vm.ServiceAccount, nil
	case strings.HasPrefix(ref, "pod:"):
		parts := strings.Split(strings.TrimPrefix(ref, "pod:"), "/")
		if len(parts) != 3 {
			return sim.Endpoint{}, "", fmt.Errorf("pod ref must be pod:CLUSTER/NS/DEPLOYMENT")
		}
		cl := p.Clusters[parts[0]]
		if cl == nil {
			return sim.Endpoint{}, "", fmt.Errorf("cluster %s not found", parts[0])
		}
		for _, pd := range c.State.ComputePods(c.Project, cl)[parts[1]] {
			if pd.Owner == parts[2] && pd.Ready {
				w := c.State.PodWorkload(c.Project, cl, parts[1], pd)
				return w.Endpoint, w.Principal, nil
			}
		}
		return sim.Endpoint{}, "", fmt.Errorf("no ready pod for %s", ref)
	case strings.HasPrefix(ref, "run:"):
		svc := p.RunServices[strings.TrimPrefix(ref, "run:")]
		if svc == nil {
			return sim.Endpoint{}, "", fmt.Errorf("service %s not found", ref)
		}
		w, why := c.State.RunWorkload(c.Project, svc)
		if w == nil {
			return sim.Endpoint{}, "", fmt.Errorf("%s", why)
		}
		return w.Endpoint, w.Principal, nil
	}
	return sim.Endpoint{}, "", fmt.Errorf("unknown endpoint %q", ref)
}

func (c *Context) http(ch scenario.Check) (bool, string) {
	target := str(ch, "target")
	url := c.State.ResolveTrafficURL(c.Project, target, str(ch, "path"))
	if url == "" {
		return false, "target " + target + " does not exist"
	}
	from, fromPrincipal, err := c.endpoint(str(ch, "from"))
	if err != nil {
		return false, err.Error()
	}
	principal := str(ch, "principal")
	if principal == "from" {
		principal = fromPrincipal
	}
	r := c.State.HTTP(sim.HTTPRequest{From: from, Principal: principal, URL: url, SourceIP: from.IP})
	want := int(num(ch, "status", 200))
	detail := fmt.Sprintf("GET %s -> %d %s", url, r.Status, trunc(firstNonEmpty(r.Error, r.Body)))
	if ns := int(num(ch, "notStatus", 0)); ns != 0 {
		return r.Status != ns && r.Status != 0, detail
	}
	if str(ch, "status") == "blocked" {
		return r.Status == 0 || r.Status == 403 || r.Status == 404, detail
	}
	return r.Status == want, detail
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}

func (c *Context) tcp(ch scenario.Check) (bool, string) {
	from, _, err := c.endpoint(str(ch, "from"))
	if err != nil {
		return false, err.Error()
	}
	host := str(ch, "to")
	p := c.State.Projects[c.Project]
	if strings.HasPrefix(host, "vm:") {
		if vm := p.Instances[strings.TrimPrefix(host, "vm:")]; vm != nil {
			host = vm.InternalIP
		}
	} else if strings.HasPrefix(host, "sql:") {
		if in := p.SQLInstances[strings.TrimPrefix(host, "sql:")]; in != nil {
			host = firstNonEmpty(in.PrivateIP, in.PublicIP)
		}
	}
	ok, _, msg := c.State.TCPConnect(from, host, int(num(ch, "port", 80)))
	want := boolean(ch, "expect", true)
	return ok == want, msg
}

func (c *Context) egress(ch scenario.Check) (bool, string) {
	from, _, err := c.endpoint(str(ch, "from"))
	if err != nil {
		return false, err.Error()
	}
	ok, why := c.State.InternetPath(from)
	return ok == boolean(ch, "expect", true), why
}

func (c *Context) googleAPI(ch scenario.Check) (bool, string) {
	from, _, err := c.endpoint(str(ch, "from"))
	if err != nil {
		return false, err.Error()
	}
	ok, why := c.State.GoogleAPIAccess(from)
	return ok == boolean(ch, "expect", true), why
}

func (c *Context) resource(ref string) (sim.Resource, error) {
	p := c.State.Projects[c.Project]
	kind, name, _ := strings.Cut(ref, ":")
	switch kind {
	case "", "project":
		return sim.ProjectResource(c.Project), nil
	case "bucket":
		bn, obj, _ := strings.Cut(name, "/")
		b, _ := c.State.FindBucket(bn)
		if b == nil {
			return sim.Resource{}, fmt.Errorf("bucket %s not found", bn)
		}
		return c.State.BucketResource(b, obj), nil
	case "secret":
		s := p.Secrets[name]
		if s == nil {
			return sim.Resource{}, fmt.Errorf("secret %s not found", name)
		}
		return sim.Resource{Project: c.Project, Type: "secretmanager.googleapis.com/Secret", Name: "projects/" + c.Project + "/secrets/" + name, Service: "secretmanager.googleapis.com", Policies: []*sim.Policy{&s.IAM}}, nil
	case "sa":
		s := p.ServiceAccounts[name]
		if s == nil {
			return sim.Resource{}, fmt.Errorf("service account %s not found", name)
		}
		return sim.Resource{Project: c.Project, Type: "iam.googleapis.com/ServiceAccount", Name: "projects/" + c.Project + "/serviceAccounts/" + name, Service: "iam.googleapis.com", Policies: []*sim.Policy{&s.IAM}}, nil
	case "run":
		s := p.RunServices[name]
		if s == nil {
			return sim.Resource{}, fmt.Errorf("service %s not found", name)
		}
		return sim.Resource{Project: c.Project, Type: "run.googleapis.com/Service", Name: "projects/" + c.Project + "/locations/" + s.Region + "/services/" + name, Service: "run.googleapis.com", Policies: []*sim.Policy{&s.IAM}}, nil
	case "topic":
		return sim.Resource{Project: c.Project, Type: "pubsub.googleapis.com/Topic", Name: "projects/" + c.Project + "/topics/" + name, Service: "pubsub.googleapis.com"}, nil
	case "key":
		parts := strings.Split(name, "/")
		if len(parts) == 2 {
			if kr := p.KeyRings[parts[0]]; kr != nil && kr.Keys[parts[1]] != nil {
				k := kr.Keys[parts[1]]
				return sim.Resource{Project: c.Project, Type: "cloudkms.googleapis.com/CryptoKey", Name: "projects/" + c.Project + "/locations/" + kr.Location + "/keyRings/" + kr.Name + "/cryptoKeys/" + k.Name, Service: "cloudkms.googleapis.com", Policies: []*sim.Policy{&k.IAM}}, nil
			}
		}
		return sim.Resource{}, fmt.Errorf("key %s not found", name)
	case "dataset":
		d := p.Datasets[name]
		if d == nil {
			return sim.Resource{}, fmt.Errorf("dataset %s not found", name)
		}
		return sim.Resource{Project: c.Project, Type: "bigquery.googleapis.com/Dataset", Name: "projects/" + c.Project + "/datasets/" + name, Service: "bigquery.googleapis.com", Policies: []*sim.Policy{&d.IAM}}, nil
	}
	return sim.Resource{}, fmt.Errorf("unknown resource %q", ref)
}

func (c *Context) principal(ref string) string {
	if strings.HasPrefix(ref, "vm:") {
		if vm := c.State.Projects[c.Project].Instances[strings.TrimPrefix(ref, "vm:")]; vm != nil {
			return "serviceAccount:" + vm.ServiceAccount
		}
	}
	if strings.HasPrefix(ref, "run:") {
		if svc := c.State.Projects[c.Project].RunServices[strings.TrimPrefix(ref, "run:")]; svc != nil {
			sa := svc.SA
			if sa == "" {
				sa = c.State.Projects[c.Project].Number + "-compute@developer.gserviceaccount.com"
			}
			return "serviceAccount:" + sa
		}
	}
	return ref
}

func (c *Context) iam(ch scenario.Check) (bool, string) {
	r, err := c.resource(str(ch, "resource"))
	if err != nil {
		return false, err.Error()
	}
	pr := c.principal(str(ch, "principal"))
	allowed, role := c.State.Explain(pr, str(ch, "permission"), r)
	want := boolean(ch, "expect", true)
	d := fmt.Sprintf("%s %s on %s: allowed=%v", pr, str(ch, "permission"), r.Name, allowed)
	if role != "" {
		d += " via " + role
	}
	return allowed == want, d
}

func (c *Context) noBasicRoles(ch scenario.Check) (bool, string) {
	p := c.State.Projects[c.Project]
	except := list(ch, "except")
	only := list(ch, "members")
	for _, b := range p.IAM.Bindings {
		if b.Role != "roles/owner" && b.Role != "roles/editor" && b.Role != "roles/viewer" {
			continue
		}
		for _, m := range b.Members {
			skip := false
			for _, e := range except {
				if m == e || (strings.HasPrefix(e, "*") && strings.HasSuffix(m, strings.TrimPrefix(e, "*"))) {
					skip = true
				}
			}
			if len(only) > 0 {
				skip = true
				for _, o := range only {
					if o == m {
						skip = false
					}
				}
			}
			if !skip {
				return false, fmt.Sprintf("%s holds basic role %s", m, b.Role)
			}
		}
	}
	return true, "no unexpected basic roles"
}

func (c *Context) forbidFirewall(ch scenario.Check) (bool, string) {
	p := c.State.Projects[c.Project]
	src := str(ch, "sourceRange")
	if src == "" {
		src = "0.0.0.0/0"
	}
	var ports []int
	for _, x := range list(ch, "ports") {
		n, _ := strconv.Atoi(x)
		ports = append(ports, n)
	}
	if n := int(num(ch, "port", 0)); n > 0 {
		ports = append(ports, n)
	}
	for _, name := range sim.SortedKeys(p.Firewalls) {
		fw := p.Firewalls[name]
		if fw.Direction != "INGRESS" || fw.Action != "ALLOW" || fw.Disabled {
			continue
		}
		if nw := str(ch, "network"); nw != "" && fw.Network != nw {
			continue
		}
		hasSrc := false
		for _, r := range fw.SourceRanges {
			if r == src || (src == "0.0.0.0/0" && strings.HasSuffix(r, "/0")) {
				hasSrc = true
			}
		}
		if !hasSrc {
			continue
		}
		if len(ports) == 0 {
			return false, "rule " + name + " allows " + src
		}
		for _, port := range ports {
			for _, r := range fw.Rules {
				if matchPort(r, port) {
					return false, fmt.Sprintf("rule %s allows %s on port %d", name, src, port)
				}
			}
		}
	}
	return true, "no overly permissive firewall rule"
}

func matchPort(r sim.FWRule, port int) bool {
	if strings.EqualFold(r.Protocol, "all") {
		return true
	}
	if r.Protocol != "tcp" {
		return false
	}
	if len(r.Ports) == 0 {
		return true
	}
	return sim.PortInList(r.Ports, port)
}

func (c *Context) finding(ch scenario.Check, want bool) (bool, string) {
	cat := str(ch, "category")
	for _, f := range c.State.Findings(c.Project) {
		if f.Category == cat && (str(ch, "resourceContains") == "" || strings.Contains(f.Resource, str(ch, "resourceContains"))) {
			return want, "finding " + cat + " present on " + f.Resource
		}
	}
	return !want, "finding " + cat + " absent"
}

func (c *Context) logMetric(ch scenario.Check) (bool, string) {
	for _, n := range sim.SortedKeys(c.State.Projects[c.Project].LogMetrics) {
		m := c.State.Projects[c.Project].LogMetrics[n]
		ok := true
		for _, frag := range list(ch, "filterContains") {
			if !strings.Contains(strings.ToLower(strings.ReplaceAll(m.Filter, " ", "")), strings.ToLower(strings.ReplaceAll(frag, " ", ""))) {
				ok = false
			}
		}
		if name := str(ch, "name"); name != "" && n != name {
			ok = false
		}
		if ok && boolean(ch, "matchesErrors", false) {
			// the metric must actually count the error logs produced by the scenario
			found := false
			for _, e := range c.State.Logs {
				if e.Project == c.Project && sim.MatchFilter(m.Filter, e) && (e.Severity == "ERROR" || (e.HTTP != nil && e.HTTP.Status >= 500)) {
					found = true
					break
				}
			}
			ok = found
		}
		if ok {
			return true, "log-based metric " + n
		}
	}
	return false, "no log-based metric matching the requirement"
}

func (c *Context) alertPolicy(ch scenario.Check) (bool, string) {
	p := c.State.Projects[c.Project]
	for _, n := range sim.SortedKeys(p.AlertPolicies) {
		ap := p.AlertPolicies[n]
		ok := ap.Enabled
		if frag := str(ch, "filterContains"); frag != "" {
			found := false
			for _, cd := range ap.Conditions {
				if strings.Contains(cd.Filter, frag) {
					found = true
				}
			}
			ok = ok && found
		}
		if boolean(ch, "channel", false) && len(ap.Channels) == 0 {
			ok = false
		}
		if boolean(ch, "firing", false) && !ap.Firing {
			ok = false
		}
		if ok {
			return true, "alert policy " + ap.DisplayName
		}
	}
	return false, "no matching alert policy"
}

// ---- evidence, quiz, commands ----------------------------------------------------

func normalize(s string) string {
	s = strings.ToLower(s)
	r := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n")
	return r.Replace(s)
}

func words(s string) int {
	return len(strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }))
}

func (c *Context) evidenceText(fields []string) string {
	var parts []string
	if len(fields) == 0 {
		for _, k := range sortedStrKeys(c.Sub.Evidence) {
			parts = append(parts, c.Sub.Evidence[k])
		}
	} else {
		for _, f := range fields {
			parts = append(parts, c.Sub.Evidence[f])
		}
	}
	return strings.Join(parts, "\n")
}

func sortedStrKeys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (c *Context) evidence(ch scenario.Check) (bool, string) {
	text := normalize(c.evidenceText(list(ch, "fields")))
	minW := int(num(ch, "minWords", 3))
	if n := words(text); n < minW {
		return false, fmt.Sprintf("explanation has %d words (minimum %d)", n, minW)
	}
	if c.Lab.Evidence != nil && c.Lab.Evidence.MinWords > 0 {
		if n := words(c.evidenceText(nil)); n < c.Lab.Evidence.MinWords {
			return false, fmt.Sprintf("the whole explanation has %d words (minimum %d)", n, c.Lab.Evidence.MinWords)
		}
	}
	var groups [][]string
	if raw, ok := ch["keywords"].([]any); ok {
		for _, g := range raw {
			var grp []string
			switch x := g.(type) {
			case []any:
				for _, w := range x {
					grp = append(grp, fmt.Sprint(w))
				}
			default:
				grp = []string{fmt.Sprint(x)}
			}
			groups = append(groups, grp)
		}
	} else if c.Lab.Evidence != nil {
		groups = c.Lab.Evidence.Keywords
	}
	var missing []string
	for _, g := range groups {
		hit := false
		for _, w := range g {
			if strings.Contains(text, normalize(w)) {
				hit = true
			}
		}
		if !hit {
			missing = append(missing, g[0])
		}
	}
	need := len(groups)
	if r := num(ch, "ratio", 1); r < 1 {
		need = int(math.Ceil(float64(len(groups)) * r))
	}
	if len(groups)-len(missing) < need {
		return false, fmt.Sprintf("explanation does not yet cover %d key concept(s)", len(missing))
	}
	return true, "explanation covers the key concepts"
}

func (c *Context) quiz(ch scenario.Check) (bool, string) {
	ids := list(ch, "ids")
	correct, total := 0, 0
	for _, q := range c.Lab.Quiz {
		if len(ids) > 0 && !contains(ids, q.ID) {
			continue
		}
		total++
		got := append([]int{}, c.Sub.Answers[q.ID]...)
		want := append([]int{}, q.Answer...)
		sort.Ints(got)
		sort.Ints(want)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			continue
		}
		if q.Justify && len(q.Keywords) > 0 {
			j := normalize(c.Sub.Justifications[q.ID])
			hits := 0
			for _, g := range q.Keywords {
				for _, w := range g {
					if strings.Contains(j, normalize(w)) {
						hits++
						break
					}
				}
			}
			if hits*2 < len(q.Keywords) {
				continue
			}
		}
		correct++
	}
	if total == 0 {
		return false, "no quiz questions"
	}
	ratio := num(ch, "ratio", 1)
	return float64(correct) >= math.Ceil(float64(total)*ratio), fmt.Sprintf("%d/%d correct", correct, total)
}

func contains(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

func (c *Context) command(ch scenario.Check) (bool, string) {
	re, err := regexp.Compile(str(ch, "regex"))
	if err != nil {
		return false, "invalid regex"
	}
	n := 0
	for _, r := range c.Session.Records {
		if re.MatchString(r.Line) && (r.Exit == 0 || boolean(ch, "anyExit", false)) {
			n++
		}
	}
	return n >= int(num(ch, "min", 1)), fmt.Sprintf("%d matching commands", n)
}

func (c *Context) bqBytes(ch scenario.Check) (bool, string) {
	p := c.State.Projects[c.Project]
	limit := int64(num(ch, "bytes", 0))
	best := int64(-1)
	for _, j := range p.BQJobs {
		if j.DryRun && !boolean(ch, "allowDryRun", true) {
			continue
		}
		if frag := str(ch, "queryContains"); frag != "" && !strings.Contains(strings.ToLower(j.Query), strings.ToLower(frag)) {
			continue
		}
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(j.Query)), "CREATE") {
			continue
		}
		if best < 0 || j.BytesProcessed < best {
			best = j.BytesProcessed
		}
	}
	if best < 0 {
		return false, "no qualifying query found"
	}
	return best <= limit, fmt.Sprintf("best query processed %s (limit %s)", sim.FormatBytes(best), sim.FormatBytes(limit))
}

func (c *Context) terraformClean(ch scenario.Check) (bool, string) {
	s := c.Session
	st, ok := s.Files["terraform.tfstate"]
	if !ok {
		// maybe remote state in a bucket
		for _, b := range c.State.Projects[c.Project].Buckets {
			for k, o := range b.Objects {
				if strings.HasSuffix(k, "default.tfstate") {
					st, ok = o.Content, true
				}
			}
		}
	}
	if !ok {
		return false, "no Terraform state found (run terraform apply)"
	}
	var parsed struct {
		Resources []any `json:"resources"`
	}
	_ = json.Unmarshal([]byte(st), &parsed)
	if len(parsed.Resources) < int(num(ch, "minResources", 1)) {
		return false, fmt.Sprintf("state manages %d resources", len(parsed.Resources))
	}
	s.Env["TF_INITIALIZED"] = "1"
	r := s.Exec("terraform plan")
	if !strings.Contains(r.Output, "No changes") {
		return false, "terraform plan is not empty (configuration drift or non-idempotent code)"
	}
	for _, v := range list(ch, "requireVariables") {
		found := false
		for k, f := range s.Files {
			if strings.HasSuffix(k, ".tf") && strings.Contains(f, `variable "`+v+`"`) {
				found = true
			}
		}
		if !found {
			return false, "variable " + v + " not declared"
		}
	}
	if boolean(ch, "requireOutputs", false) {
		found := false
		for k, f := range s.Files {
			if strings.HasSuffix(k, ".tf") && strings.Contains(f, "output \"") {
				found = true
			}
		}
		if !found {
			return false, "no outputs declared"
		}
	}
	return true, fmt.Sprintf("%d resources, plan clean", len(parsed.Resources))
}

func (c *Context) cluster(ch scenario.Check) (*sim.Cluster, string) {
	p := c.State.Projects[c.Project]
	cl := p.Clusters[str(ch, "cluster")]
	ns := str(ch, "namespace")
	if ns == "" {
		ns = "default"
	}
	return cl, ns
}

func (c *Context) k8sReady(ch scenario.Check) (bool, string) {
	cl, ns := c.cluster(ch)
	if cl == nil || cl.K8s == nil {
		return false, "cluster not found"
	}
	ready := 0
	for _, pd := range c.State.ComputePods(c.Project, cl)[ns] {
		if pd.Owner == str(ch, "deployment") && pd.Ready {
			ready++
		}
	}
	return float64(ready) >= num(ch, "min", 1), fmt.Sprintf("%d ready pods for %s", ready, str(ch, "deployment"))
}

func (c *Context) hpaScaled(ch scenario.Check) (bool, string) {
	cl, ns := c.cluster(ch)
	if cl == nil || cl.K8s == nil {
		return false, "cluster not found"
	}
	for _, h := range cl.K8s.NS(ns).HPAs {
		if h.Target == str(ch, "deployment") {
			if h.Unknown {
				return false, "HPA cannot read CPU metrics (missing resource requests)"
			}
			if h.ScaledUpTo <= h.Min {
				return false, fmt.Sprintf("HPA never scaled out (max observed %d)", h.ScaledUpTo)
			}
			if boolean(ch, "requireScaleIn", false) && !h.ScaledDown {
				return false, "HPA scaled out but scale-in was not demonstrated"
			}
			return true, fmt.Sprintf("scaled up to %d replicas", h.ScaledUpTo)
		}
	}
	return false, "no HPA targeting " + str(ch, "deployment")
}

func (c *Context) pubsub(ch scenario.Check) (bool, string) {
	sub := c.State.Projects[c.Project].Subs[str(ch, "subscription")]
	if sub == nil {
		return false, "subscription not found"
	}
	if v := num(ch, "ackedMin", -1); v >= 0 && float64(sub.Acked) < v {
		return false, fmt.Sprintf("%d messages acknowledged", sub.Acked)
	}
	if v := num(ch, "deadLetteredMin", -1); v >= 0 && float64(sub.DeadLettered) < v {
		return false, fmt.Sprintf("%d messages dead-lettered", sub.DeadLettered)
	}
	if v := num(ch, "backlogMax", -1); v >= 0 && float64(len(sub.Backlog)) > v {
		return false, fmt.Sprintf("backlog is %d messages", len(sub.Backlog))
	}
	return true, fmt.Sprintf("acked=%d deadLettered=%d backlog=%d", sub.Acked, sub.DeadLettered, len(sub.Backlog))
}

func (c *Context) build(ch scenario.Check) (bool, string) {
	re := regexp.MustCompile(firstNonEmpty(str(ch, "imageRegex"), ".*"))
	for i := len(c.State.Projects[c.Project].Builds) - 1; i >= 0; i-- {
		b := c.State.Projects[c.Project].Builds[i]
		if b.Status != "SUCCESS" {
			continue
		}
		if boolean(ch, "trigger", false) && b.Trigger == "" {
			continue
		}
		for _, img := range b.Images {
			if re.MatchString(img) {
				if boolean(ch, "shaTag", false) && !strings.Contains(img, b.Commit[:7]) && !strings.Contains(img, b.Commit) {
					continue
				}
				return true, "build " + b.ID + " produced " + img
			}
		}
	}
	return false, "no successful build producing a matching image"
}

func (c *Context) sqlHealthy(ch scenario.Check) (bool, string) {
	in := c.State.Projects[c.Project].SQLInstances[str(ch, "instance")]
	if in == nil {
		return false, "instance not found"
	}
	maxc := sim.DBMaxConnections(in)
	if in.Connections > maxc {
		return false, fmt.Sprintf("%d connections > max_connections %d", in.Connections, maxc)
	}
	if boolean(ch, "noSlowQueries", false) && len(in.SlowQueries) > 0 {
		return false, fmt.Sprintf("%d slow queries still present", len(in.SlowQueries))
	}
	if in.CPU > num(ch, "maxCpu", 90) {
		return false, fmt.Sprintf("CPU %.0f%%", in.CPU)
	}
	return true, fmt.Sprintf("connections %d/%d, cpu %.0f%%", in.Connections, maxc, in.CPU)
}

func (c *Context) rollout(ch scenario.Check) (bool, string) {
	p := c.State.Projects[c.Project]
	for _, r := range p.Releases {
		if frag := str(ch, "imageContains"); frag != "" && !strings.Contains(r.Image, frag) {
			continue
		}
		for _, ro := range r.Rollouts {
			if ro.Target == str(ch, "target") && strings.EqualFold(ro.State, firstNonEmpty(str(ch, "state"), "SUCCEEDED")) {
				if frag := str(ch, "detailContains"); frag != "" && !strings.Contains(ro.Detail, frag) {
					continue
				}
				return true, "release " + r.Name + " " + ro.State
			}
		}
	}
	return false, "no matching rollout"
}

func (c *Context) logContains(ch scenario.Check) (bool, string) {
	n := len(c.State.QueryLogs(c.Project, str(ch, "filter"), 0))
	return float64(n) >= num(ch, "min", 1), fmt.Sprintf("%d matching log entries", n)
}

// ---- OPA / Rego policy validator -------------------------------------------------

// PolicyDir holds Rego modules (content/policies).
var PolicyDir = "content/policies"

var (
	policyOnce  sync.Once
	policyMods  map[string]string
	policyError error
)

func loadPolicies() {
	policyMods = map[string]string{}
	entries, err := os.ReadDir(PolicyDir)
	if err != nil {
		policyError = err
		return
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".rego") && !strings.HasSuffix(e.Name(), "_test.rego") {
			b, err := os.ReadFile(filepath.Join(PolicyDir, e.Name()))
			if err == nil {
				policyMods[e.Name()] = string(b)
			}
		}
	}
}

// EvalPolicy evaluates `data.<pkg>.deny` against a project view.
func EvalPolicy(pkg string, input map[string]any) ([]string, error) {
	policyOnce.Do(loadPolicies)
	if policyError != nil {
		return nil, policyError
	}
	opts := []func(*rego.Rego){rego.Query("data." + pkg + ".deny"), rego.Input(input)}
	for n, m := range policyMods {
		opts = append(opts, rego.Module(n, m))
	}
	rs, err := rego.New(opts...).Eval(context.Background())
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range rs {
		for _, e := range r.Expressions {
			if set, ok := e.Value.([]any); ok {
				for _, v := range set {
					out = append(out, fmt.Sprint(v))
				}
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

func (c *Context) policy(ch scenario.Check) (bool, string) {
	pkg := firstNonEmpty(str(ch, "package"), "gcplab.security")
	input := map[string]any{"project": c.View(), "projectId": c.Project, "params": ch["params"]}
	msgs, err := EvalPolicy(pkg, input)
	if err != nil {
		return false, "policy evaluation error: " + err.Error()
	}
	ids := list(ch, "ids")
	var hit []string
	for _, m := range msgs {
		if len(ids) == 0 {
			hit = append(hit, m)
			continue
		}
		for _, id := range ids {
			if strings.HasPrefix(m, id+":") || strings.HasPrefix(m, "["+id+"]") {
				hit = append(hit, m)
			}
		}
	}
	if len(hit) > 0 {
		return false, strings.Join(hit, "; ")
	}
	return true, "policy " + pkg + " satisfied"
}

// deskCheck validates service-desk communication: stakeholder updates, the
// resolution note and questions asked to simulated actors.
func (c *Context) deskCheck(typ string, ch scenario.Check) (bool, string) {
	d := c.Session.Desk
	if d == nil {
		return false, "no service desk in this lab"
	}
	match := func(text string) bool {
		groups := ch["keywords"]
		gs, _ := groups.([]any)
		t := normalize(text)
		for _, g := range gs {
			hit := false
			alts, _ := g.([]any)
			for _, a := range alts {
				if strings.Contains(t, normalize(fmt.Sprint(a))) {
					hit = true
				}
			}
			if !hit {
				return false
			}
		}
		return true
	}
	switch typ {
	case "ticket_update":
		n := 0
		for _, cm := range d.StudentComments(c.Session.Account) {
			if (cm.Public || !boolean(ch, "public", true)) && len(strings.Fields(cm.Text)) >= 6 && match(cm.Text) {
				n++
			}
		}
		for _, cm := range d.StudentComments("student") {
			if (cm.Public || !boolean(ch, "public", true)) && len(strings.Fields(cm.Text)) >= 6 && match(cm.Text) {
				n++
			}
		}
		return float64(n) >= num(ch, "min", 1), fmt.Sprintf("%d useful update(s)", n)
	case "ticket_resolved":
		if d.Ticket == nil || d.Ticket.Status != "RESOLVED" {
			return false, "ticket not resolved"
		}
		return match(d.Ticket.Resolution), "resolved: " + d.Ticket.Resolution
	case "asked":
		n := 0
		for _, q := range d.Questions {
			if (str(ch, "actor") == "" || strings.EqualFold(q.Actor, str(ch, "actor"))) && (q.Useful || !boolean(ch, "useful", true)) {
				n++
			}
		}
		return float64(n) >= num(ch, "min", 1), fmt.Sprintf("%d relevant question(s)", n)
	}
	return false, "unknown desk check"
}

// tfState checks that a resource address is (or is not) in the Terraform state.
func (c *Context) tfState(ch scenario.Check) (bool, string) {
	raw, ok := c.Session.Files["terraform.tfstate"]
	if !ok {
		for _, b := range c.State.Projects[c.Project].Buckets {
			for k, o := range b.Objects {
				if strings.HasSuffix(k, "default.tfstate") {
					raw, ok = o.Content, true
				}
			}
		}
	}
	if !ok {
		return false, "no Terraform state"
	}
	var st struct {
		Resources []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"resources"`
	}
	_ = json.Unmarshal([]byte(raw), &st)
	want := str(ch, "address")
	found := false
	for _, r := range st.Resources {
		if r.Type+"."+r.Name == want {
			found = true
		}
	}
	return found == boolean(ch, "present", true), fmt.Sprintf("%s in state: %v", want, found)
}

// design evaluates the learner's architecture file against the requirements
// stated in the check (Architecture Simulator). "requirements" filters which
// findings must pass (all by default).
func (c *Context) design(ch scenario.Check) (bool, string) {
	raw, ok := c.Session.Files[firstNonEmpty(str(ch, "file"), "design.yaml")]
	if !ok {
		return false, "design file not found"
	}
	d, err := archsim.Parse([]byte(raw))
	if err != nil {
		return false, err.Error()
	}
	var req archsim.Requirements
	b, _ := yaml.Marshal(ch["requirements"])
	if err := yaml.Unmarshal(b, &req); err != nil {
		return false, "bad requirements: " + err.Error()
	}
	rep := archsim.Evaluate(d, req)
	if len(rep.Problems) > 0 {
		return false, strings.Join(rep.Problems, "; ")
	}
	only := list(ch, "only")
	var failed []string
	for _, f := range rep.Findings {
		if len(only) > 0 && !contains(only, f.Requirement) {
			continue
		}
		if !f.Pass {
			failed = append(failed, fmt.Sprintf("%s (%s, target %s)", f.Requirement, f.Achieved, f.Target))
		}
	}
	if len(failed) > 0 {
		return false, "not met: " + strings.Join(failed, "; ")
	}
	return true, fmt.Sprintf("%d/%d requirements met, %.0f EUR/month", rep.Passed, rep.Total, rep.CostEur)
}
