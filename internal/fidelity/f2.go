package fidelity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/cli"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
	"github.com/neodevesp/gcp-trainer/internal/sim"
)

// Project pool lifecycle (never create/delete projects per lab: project
// creation is quota-limited and deletion is a 30-day soft delete).
//
//	READY → LEASED → BASELINE → ACTIVE → REVOKING → CLEANING → VALIDATING → READY
//	                                         └────────── on failure ───────────→ QUARANTINE
const (
	StateReady      = "READY"
	StateLeased     = "LEASED"
	StateBaseline   = "BASELINE"
	StateActive     = "ACTIVE"
	StateRevoking   = "REVOKING"
	StateCleaning   = "CLEANING"
	StateValidating = "VALIDATING"
	StateQuarantine = "QUARANTINE"
)

// PoolProject is one project of the training pool.
type PoolProject struct {
	ID         string    `json:"id"`
	State      string    `json:"state"`
	LeaseID    string    `json:"leaseId,omitempty"`
	Recycled   int       `json:"recycled"`
	LastError  string    `json:"lastError,omitempty"`
	LastChange time.Time `json:"lastChange"`
	Orphans    []string  `json:"orphans,omitempty"`
}

// Lease grants temporary access to a pool project.
type Lease struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	UserID    string    `json:"userId"`
	Member    string    `json:"member"`
	LabID     string    `json:"labId"`
	Roles     []string  `json:"roles"`
	Expires   time.Time `json:"expires"`
	BudgetEur float64   `json:"budgetEur"`
}

// Driver performs the real cloud operations. GcloudDriver talks to Google
// Cloud; SimDriver backs the pool with the simulator for tests and demos.
type Driver interface {
	Name() string
	PrepareProject(ctx context.Context, project string, apis []string) error
	ApplyBaseline(ctx context.Context, project string, l *scenario.Lab) error
	GrantAccess(ctx context.Context, project, member string, roles []string, until time.Time) error
	RevokeAccess(ctx context.Context, project, member string) error
	Exec(ctx context.Context, project, member string, args []string, stdin string) (string, error)
	Snapshot(ctx context.Context, project string) (*sim.State, error)
	DestroyAll(ctx context.Context, project string) error
	ListResources(ctx context.Context, project string) ([]string, error)
}

// Pool manages leases over a fixed set of projects.
type Pool struct {
	mu       sync.Mutex
	Projects map[string]*PoolProject
	Leases   map[string]*Lease
	Driver   Driver
	Now      func() time.Time
	Events   []PoolEvent
}

// PoolEvent is an audit trail entry for the pool.
type PoolEvent struct {
	At      time.Time `json:"at"`
	Project string    `json:"project"`
	From    string    `json:"from"`
	To      string    `json:"to"`
	Note    string    `json:"note"`
}

// NewPool creates a pool of warm projects (already created by Terraform in
// deploy/terraform/sandbox-pool).
func NewPool(driver Driver, projectIDs []string) *Pool {
	p := &Pool{Projects: map[string]*PoolProject{}, Leases: map[string]*Lease{}, Driver: driver}
	for _, id := range projectIDs {
		p.Projects[id] = &PoolProject{ID: id, State: StateReady, LastChange: time.Now()}
	}
	return p
}

func (p *Pool) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *Pool) transition(pp *PoolProject, to, note string) {
	p.Events = append(p.Events, PoolEvent{At: p.now(), Project: pp.ID, From: pp.State, To: to, Note: note})
	if len(p.Events) > 2000 {
		p.Events = p.Events[len(p.Events)-2000:]
	}
	pp.State = to
	pp.LastChange = p.now()
}

// Stats summarises pool health (warm capacity, quarantine, orphans).
func (p *Pool) Stats() map[string]int {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := map[string]int{}
	for _, pp := range p.Projects {
		out[pp.State]++
		out["orphans"] += len(pp.Orphans)
	}
	out["total"] = len(p.Projects)
	return out
}

// Snapshot of the pool for the admin console.
func (p *Pool) List() []PoolProject {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []PoolProject
	for _, pp := range p.Projects {
		out = append(out, *pp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ErrPoolExhausted is returned when no warm project is available.
var ErrPoolExhausted = errors.New("no READY project in the sandbox pool")

// Acquire leases a project, applies the lab baseline and grants short-lived access.
func (p *Pool) Acquire(ctx context.Context, l *scenario.Lab, userID, member string) (*Lease, error) {
	p.mu.Lock()
	var pp *PoolProject
	ids := make([]string, 0, len(p.Projects))
	for id := range p.Projects {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if p.Projects[id].State == StateReady {
			pp = p.Projects[id]
			break
		}
	}
	if pp == nil {
		p.mu.Unlock()
		return nil, ErrPoolExhausted
	}
	ttl := 60 * time.Minute
	budget := 0.5
	var apis, roles []string
	if f2 := l.Fidelity.F2; f2 != nil {
		if f2.TTLMinutes > 0 {
			ttl = time.Duration(f2.TTLMinutes) * time.Minute
		}
		if f2.BudgetEur > 0 {
			budget = f2.BudgetEur
		}
		apis, roles = f2.APIs, f2.StudentRoles
	}
	if len(roles) == 0 {
		roles = []string{"roles/editor"}
	}
	lease := &Lease{ID: fmt.Sprintf("lease-%s-%d", pp.ID, pp.Recycled+1), ProjectID: pp.ID, UserID: userID, Member: member, LabID: l.ID, Roles: roles, Expires: p.now().Add(ttl), BudgetEur: budget}
	p.transition(pp, StateLeased, "lab "+l.ID)
	pp.LeaseID = lease.ID
	p.Leases[lease.ID] = lease
	p.mu.Unlock()

	fail := func(err error) (*Lease, error) {
		p.mu.Lock()
		p.transition(pp, StateQuarantine, err.Error())
		pp.LastError = err.Error()
		delete(p.Leases, lease.ID)
		p.mu.Unlock()
		return nil, err
	}
	if err := p.Driver.PrepareProject(ctx, pp.ID, apis); err != nil {
		return fail(fmt.Errorf("prepare: %w", err))
	}
	p.mu.Lock()
	p.transition(pp, StateBaseline, "")
	p.mu.Unlock()
	if err := p.Driver.ApplyBaseline(ctx, pp.ID, l); err != nil {
		return fail(fmt.Errorf("baseline: %w", err))
	}
	if err := p.Driver.GrantAccess(ctx, pp.ID, member, roles, lease.Expires); err != nil {
		return fail(fmt.Errorf("grant: %w", err))
	}
	p.mu.Lock()
	p.transition(pp, StateActive, "access until "+lease.Expires.Format(time.RFC3339))
	p.mu.Unlock()
	return lease, nil
}

// Release revokes access, destroys lab resources, validates the project is
// clean and returns it to READY (or QUARANTINE).
func (p *Pool) Release(ctx context.Context, leaseID, reason string) error {
	p.mu.Lock()
	lease := p.Leases[leaseID]
	if lease == nil {
		p.mu.Unlock()
		return fmt.Errorf("lease %s not found", leaseID)
	}
	pp := p.Projects[lease.ProjectID]
	p.transition(pp, StateRevoking, reason)
	p.mu.Unlock()
	errs := []string{}
	if err := p.Driver.RevokeAccess(ctx, pp.ID, lease.Member); err != nil {
		errs = append(errs, "revoke: "+err.Error())
	}
	p.mu.Lock()
	p.transition(pp, StateCleaning, "")
	p.mu.Unlock()
	if err := p.Driver.DestroyAll(ctx, pp.ID); err != nil {
		errs = append(errs, "destroy: "+err.Error())
	}
	p.mu.Lock()
	p.transition(pp, StateValidating, "")
	p.mu.Unlock()
	left, err := p.Driver.ListResources(ctx, pp.ID)
	if err != nil {
		errs = append(errs, "validate: "+err.Error())
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.Leases, leaseID)
	pp.LeaseID = ""
	pp.Orphans = left
	if len(errs) > 0 || len(left) > 0 {
		pp.LastError = strings.Join(append(errs, fmt.Sprintf("%d orphaned resources", len(left))), "; ")
		p.transition(pp, StateQuarantine, pp.LastError)
		return fmt.Errorf("%s", pp.LastError)
	}
	pp.Recycled++
	pp.LastError = ""
	p.transition(pp, StateReady, "clean")
	return nil
}

// Janitor releases every expired lease regardless of whether the student
// pressed "Finish" (TTL independent of the browser) and retries quarantined
// projects. It returns the number of leases reclaimed.
func (p *Pool) Janitor(ctx context.Context) (int, []error) {
	p.mu.Lock()
	var expired []string
	for id, l := range p.Leases {
		if p.now().After(l.Expires) {
			expired = append(expired, id)
		}
	}
	var quarantined []*PoolProject
	for _, pp := range p.Projects {
		if pp.State == StateQuarantine && pp.LeaseID == "" {
			quarantined = append(quarantined, pp)
		}
	}
	p.mu.Unlock()
	var errs []error
	for _, id := range expired {
		if err := p.Release(ctx, id, "TTL expired (janitor)"); err != nil {
			errs = append(errs, err)
		}
	}
	for _, pp := range quarantined {
		if err := p.Driver.DestroyAll(ctx, pp.ID); err != nil {
			errs = append(errs, err)
			continue
		}
		if left, err := p.Driver.ListResources(ctx, pp.ID); err == nil && len(left) == 0 {
			p.mu.Lock()
			pp.Orphans = nil
			pp.LastError = ""
			p.transition(pp, StateReady, "recovered by janitor")
			p.mu.Unlock()
		}
	}
	return len(expired), errs
}

// F2Runtime runs labs on real Google Cloud projects from the pool.
type F2Runtime struct {
	Pool *Pool
	// MemberFor maps a platform user to the short-lived lab identity
	// (never the student's own cloud credentials).
	MemberFor func(userID string) string
}

func (r *F2Runtime) Level() Level { return F2 }

func (r *F2Runtime) Available() (bool, string) {
	if r.Pool == nil {
		return false, "sandbox pool not configured"
	}
	st := r.Pool.Stats()
	if st[StateReady] == 0 {
		return false, "sandbox pool exhausted"
	}
	return true, ""
}

func (r *F2Runtime) Provision(l *scenario.Lab, seed int64, _ string, userID string) (*Env, error) {
	member := "user:lab-" + userID + "@gcplab.dev"
	if r.MemberFor != nil {
		member = r.MemberFor(userID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute) // F2 provisioning SLO p95 < 4 min
	defer cancel()
	lease, err := r.Pool.Acquire(ctx, l, userID, member)
	if err != nil {
		return nil, err
	}
	// Mirror world: the grader reads a snapshot of the real project.
	w, err := scenario.Provision(l, seed, lease.ProjectID)
	if err != nil {
		_ = r.Pool.Release(ctx, lease.ID, "provision mirror failed")
		return nil, err
	}
	driver := r.Pool.Driver
	w.Session.Interceptor = func(s *cli.Session, args []string, stdin string) (bool, string, error) {
		switch args[0] {
		case "gcloud", "gsutil", "bq", "kubectl", "terraform":
		default:
			return false, "", nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		out, err := driver.Exec(ctx, lease.ProjectID, member, args, stdin)
		if err != nil {
			return true, out, cli.Fail(1, "%v", err)
		}
		return true, out, nil
	}
	return &Env{Level: F2, World: w, Lease: lease, Cleanup: func() error {
		return r.Pool.Release(context.Background(), lease.ID, "session finished")
	}}, nil
}

// RefreshMirror replaces the grading state with a snapshot of the real project.
func (r *F2Runtime) RefreshMirror(env *Env) error {
	if env.Lease == nil {
		return nil
	}
	st, err := r.Pool.Driver.Snapshot(context.Background(), env.Lease.ProjectID)
	if err != nil {
		return err
	}
	env.World.State = st
	env.World.Session.State = st
	return nil
}

// ---------------------------------------------------------------------------
// SimDriver: pool backed by simulated projects (tests, demos, CI).

type SimDriver struct {
	mu     sync.Mutex
	Worlds map[string]*sim.State
}

func NewSimDriver() *SimDriver { return &SimDriver{Worlds: map[string]*sim.State{}} }

func (d *SimDriver) Name() string { return "sim" }

func (d *SimDriver) PrepareProject(ctx context.Context, project string, apis []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := sim.New(int64(len(project)), project, "user:"+scenario.AdminAccount)
	st.Org.IAM.AddBinding("roles/owner", "user:"+scenario.AdminAccount, nil)
	p := st.Projects[project]
	if len(apis) > 0 {
		p.Services = map[string]bool{}
		for _, a := range apis {
			p.Services[a] = true
		}
	}
	d.Worlds[project] = st
	return nil
}

func (d *SimDriver) ApplyBaseline(ctx context.Context, project string, l *scenario.Lab) error {
	w, err := scenario.Provision(l, 1, project)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.Worlds[project] = w.State
	d.mu.Unlock()
	return nil
}

func (d *SimDriver) GrantAccess(ctx context.Context, project, member string, roles []string, until time.Time) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.Worlds[project]
	cond := &sim.Condition{Title: "lab-lease", Expression: fmt.Sprintf(`request.time < timestamp("%s")`, until.UTC().Format(time.RFC3339))}
	for _, r := range roles {
		st.Projects[project].IAM.AddBinding(r, member, cond)
	}
	return nil
}

func (d *SimDriver) RevokeAccess(ctx context.Context, project, member string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.Worlds[project]
	if st == nil {
		return nil
	}
	p := st.Projects[project]
	var keep []sim.Binding
	for _, b := range p.IAM.Bindings {
		var ms []string
		for _, m := range b.Members {
			if m != member {
				ms = append(ms, m)
			}
		}
		if len(ms) > 0 {
			b.Members = ms
			keep = append(keep, b)
		}
	}
	p.IAM.Bindings = keep
	return nil
}

func (d *SimDriver) Exec(ctx context.Context, project, member string, args []string, stdin string) (string, error) {
	d.mu.Lock()
	st := d.Worlds[project]
	d.mu.Unlock()
	acct := strings.TrimPrefix(strings.TrimPrefix(member, "user:"), "serviceAccount:")
	s := cli.NewSession(st, project, acct)
	s.NoTick = true
	return s.RunArgs(args, stdin)
}

func (d *SimDriver) Snapshot(ctx context.Context, project string) (*sim.State, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.Worlds[project].Clone(), nil
}

func (d *SimDriver) DestroyAll(ctx context.Context, project string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := sim.New(int64(len(project)), project, "user:"+scenario.AdminAccount)
	d.Worlds[project] = st
	return nil
}

func (d *SimDriver) ListResources(ctx context.Context, project string) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.Worlds[project]
	if st == nil {
		return nil, nil
	}
	p := st.Projects[project]
	var out []string
	for n := range p.Instances {
		out = append(out, "instance/"+n)
	}
	for n := range p.Buckets {
		out = append(out, "bucket/"+n)
	}
	for n := range p.RunServices {
		out = append(out, "run/"+n)
	}
	for n := range p.SQLInstances {
		out = append(out, "sql/"+n)
	}
	for n := range p.Clusters {
		out = append(out, "gke/"+n)
	}
	sort.Strings(out)
	return out, nil
}

// ---------------------------------------------------------------------------
// GcloudDriver: real Google Cloud through the gcloud CLI, acting as the pool
// administrator service account via impersonation (no long-lived keys).

type GcloudDriver struct {
	AdminSA     string // pool-admin@<control-project>.iam.gserviceaccount.com
	Gcloud      string
	Terraform   string
	BaselineDir string // content/labs/<lab>/f2 terraform baselines
	DryRun      bool
	Log         func(string)
}

func (d *GcloudDriver) Name() string { return "gcloud" }

func (d *GcloudDriver) run(ctx context.Context, args ...string) (string, error) {
	bin := d.Gcloud
	if bin == "" {
		bin = "gcloud"
	}
	if d.AdminSA != "" {
		args = append(args, "--impersonate-service-account="+d.AdminSA)
	}
	args = append(args, "--quiet")
	if d.Log != nil {
		d.Log(bin + " " + strings.Join(args, " "))
	}
	if d.DryRun {
		return "", nil
	}
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %s", strings.Join(args[:min(3, len(args))], " "), strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (d *GcloudDriver) PrepareProject(ctx context.Context, project string, apis []string) error {
	if len(apis) == 0 {
		return nil
	}
	_, err := d.run(ctx, append([]string{"services", "enable", "--project=" + project}, apis...)...)
	return err
}

func (d *GcloudDriver) ApplyBaseline(ctx context.Context, project string, l *scenario.Lab) error {
	if l.Fidelity.F2 != nil && l.Fidelity.F2.TerraformBase != "" {
		tf := d.Terraform
		if tf == "" {
			tf = "terraform"
		}
		dir := l.Dir + "/" + l.Fidelity.F2.TerraformBase
		for _, args := range [][]string{{"init", "-input=false"}, {"apply", "-auto-approve", "-input=false", "-var=project_id=" + project}} {
			if d.DryRun {
				continue
			}
			cmd := exec.CommandContext(ctx, tf, append([]string{"-chdir=" + dir}, args...)...)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("terraform %s: %s", args[0], out)
			}
		}
		return nil
	}
	for _, line := range strings.Split(l.Setup, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "gcloud ") {
			continue
		}
		if _, err := d.run(ctx, append(strings.Fields(strings.TrimPrefix(line, "gcloud ")), "--project="+project)...); err != nil {
			return err
		}
	}
	return nil
}

func (d *GcloudDriver) GrantAccess(ctx context.Context, project, member string, roles []string, until time.Time) error {
	cond := fmt.Sprintf(`expression=request.time < timestamp("%s"),title=lab-lease`, until.UTC().Format(time.RFC3339))
	for _, r := range roles {
		if _, err := d.run(ctx, "projects", "add-iam-policy-binding", project, "--member="+member, "--role="+r, "--condition="+cond); err != nil {
			return err
		}
	}
	return nil
}

func (d *GcloudDriver) RevokeAccess(ctx context.Context, project, member string) error {
	out, err := d.run(ctx, "projects", "get-iam-policy", project, "--format=json")
	if err != nil || d.DryRun {
		return err
	}
	var pol sim.Policy
	if err := json.Unmarshal([]byte(out), &pol); err != nil {
		return err
	}
	for _, b := range pol.Bindings {
		for _, m := range b.Members {
			if m == member {
				args := []string{"projects", "remove-iam-policy-binding", project, "--member=" + member, "--role=" + b.Role, "--all"}
				if _, err := d.run(ctx, args...); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (d *GcloudDriver) Exec(ctx context.Context, project, member string, args []string, stdin string) (string, error) {
	if args[0] != "gcloud" {
		return "", fmt.Errorf("%s is only available in F0/F1 terminals; use gcloud in F2", args[0])
	}
	// Commands run as the lease identity (impersonated), scoped to the project.
	sa := strings.TrimPrefix(member, "serviceAccount:")
	a := append(append([]string{}, args[1:]...), "--project="+project)
	if strings.HasPrefix(member, "serviceAccount:") {
		a = append(a, "--impersonate-service-account="+sa)
	}
	bin := d.Gcloud
	if bin == "" {
		bin = "gcloud"
	}
	if d.DryRun {
		return "[dry-run] gcloud " + strings.Join(a, " ") + "\n", nil
	}
	cmd := exec.CommandContext(ctx, bin, a...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Snapshot reads the real project into the simulator model so the same
// state/security/cost validators can grade F2 attempts.
func (d *GcloudDriver) Snapshot(ctx context.Context, project string) (*sim.State, error) {
	st := sim.New(1, project, "user:"+scenario.AdminAccount)
	p := st.Projects[project]
	var raw []map[string]any
	get := func(args ...string) []map[string]any {
		out, err := d.run(ctx, append(args, "--project="+project, "--format=json")...)
		if err != nil || d.DryRun {
			return nil
		}
		raw = nil
		_ = json.Unmarshal([]byte(out), &raw)
		return raw
	}
	for _, fw := range get("compute", "firewall-rules", "list") {
		b, _ := json.Marshal(fw)
		var f struct {
			Name, Network, Direction             string
			Priority                             int
			Allowed, Denied                      []sim.FWRule
			SourceRanges, TargetTags, SourceTags []string
			Disabled                             bool
		}
		_ = json.Unmarshal(b, &f)
		rule := &sim.Firewall{Name: f.Name, Network: short(f.Network), Direction: f.Direction, Priority: f.Priority, SourceRanges: f.SourceRanges, TargetTags: f.TargetTags, SourceTags: f.SourceTags, Disabled: f.Disabled, Action: "ALLOW", Rules: f.Allowed}
		if len(f.Denied) > 0 {
			rule.Action, rule.Rules = "DENY", f.Denied
		}
		p.Firewalls[f.Name] = rule
	}
	for _, vm := range get("compute", "instances", "list") {
		b, _ := json.Marshal(vm)
		var v struct {
			Name, Zone, MachineType, Status string
			Tags                            struct{ Items []string }
			NetworkInterfaces               []struct {
				Network, Subnetwork, NetworkIP string
				AccessConfigs                  []struct{ NatIP string }
			}
			ServiceAccounts []struct {
				Email  string
				Scopes []string
			}
		}
		_ = json.Unmarshal(b, &v)
		in := &sim.Instance{Name: v.Name, Zone: short(v.Zone), MachineType: short(v.MachineType), Status: v.Status, Tags: v.Tags.Items, Metadata: map[string]string{}, Labels: map[string]string{}}
		if len(v.NetworkInterfaces) > 0 {
			ni := v.NetworkInterfaces[0]
			in.Network, in.Subnet, in.InternalIP = short(ni.Network), short(ni.Subnetwork), ni.NetworkIP
			if len(ni.AccessConfigs) > 0 {
				in.ExternalIP = ni.AccessConfigs[0].NatIP
			}
		}
		if len(v.ServiceAccounts) > 0 {
			in.ServiceAccount = v.ServiceAccounts[0].Email
			in.Scopes = v.ServiceAccounts[0].Scopes
		}
		p.Instances[v.Name] = in
	}
	if out, err := d.run(ctx, "projects", "get-iam-policy", project, "--format=json"); err == nil && !d.DryRun {
		_ = json.Unmarshal([]byte(out), &p.IAM)
	}
	for _, b := range get("storage", "buckets", "list") {
		name := fmt.Sprint(b["name"])
		p.Buckets[name] = &sim.Bucket{Name: name, Project: project, Location: fmt.Sprint(b["location"]), StorageClass: fmt.Sprint(b["default_storage_class"]), UBLA: b["uniform_bucket_level_access"] == true, PAP: fmt.Sprint(b["public_access_prevention"]), Objects: map[string]*sim.Object{}, Labels: map[string]string{}}
	}
	return st, nil
}

func (d *GcloudDriver) DestroyAll(ctx context.Context, project string) error {
	// Order matters: dependants first. Every step is idempotent.
	steps := [][]string{
		{"run", "services", "list", "--format=value(metadata.name,region)"},
		{"compute", "forwarding-rules", "list", "--format=value(name)"},
		{"compute", "instances", "list", "--format=value(name,zone)"},
		{"sql", "instances", "list", "--format=value(name)"},
		{"container", "clusters", "list", "--format=value(name,location)"},
		{"compute", "firewall-rules", "list", "--format=value(name)"},
		{"storage", "buckets", "list", "--format=value(name)"},
	}
	for _, s := range steps {
		out, err := d.run(ctx, append(s, "--project="+project)...)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			f := strings.Fields(line)
			if len(f) == 0 {
				continue
			}
			var del []string
			switch s[0] + " " + s[1] {
			case "run services":
				del = []string{"run", "services", "delete", f[0], "--region=" + f[len(f)-1]}
			case "compute forwarding-rules":
				del = []string{"compute", "forwarding-rules", "delete", f[0], "--global"}
			case "compute instances":
				del = []string{"compute", "instances", "delete", f[0], "--zone=" + short(f[len(f)-1])}
			case "sql instances":
				del = []string{"sql", "instances", "delete", f[0]}
			case "container clusters":
				del = []string{"container", "clusters", "delete", f[0], "--location=" + f[len(f)-1]}
			case "compute firewall-rules":
				del = []string{"compute", "firewall-rules", "delete", f[0]}
			case "storage buckets":
				del = []string{"storage", "rm", "--recursive", "gs://" + f[0]}
			}
			if _, err := d.run(ctx, append(del, "--project="+project)...); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *GcloudDriver) ListResources(ctx context.Context, project string) ([]string, error) {
	out, err := d.run(ctx, "asset", "search-all-resources", "--scope=projects/"+project, "--format=value(name)")
	if err != nil {
		return nil, err
	}
	var left []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l == "" || strings.Contains(l, "/serviceAccounts/") || strings.Contains(l, "/networks/default") || strings.Contains(l, "cloudresourcemanager") {
			continue
		}
		left = append(left, l)
	}
	return left, nil
}
