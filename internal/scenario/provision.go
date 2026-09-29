package scenario

import (
	"fmt"
	"github.com/neodevesp/gcp-trainer/internal/desk"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/cli"
	"github.com/neodevesp/gcp-trainer/internal/sim"
)

// AdminAccount performs baseline setup (the platform, not the student).
const AdminAccount = "platform-admin@gcplab.dev"

// BaselineDir holds reusable baseline scripts (content/baselines/*.sh).
var BaselineDir = "content/baselines"

// PlatformPolicy applies to platform sessions (baselines, setup, faults):
// learner guardrails such as "no GPUs" must not stop the platform from
// building a scenario (e.g. planting an idle GPU VM in a FinOps lab).
var PlatformPolicy = cli.Policy{AllowGPUs: true}

// World is a provisioned lab environment.
type World struct {
	Lab     *Lab              `json:"lab"`
	Params  map[string]string `json:"params"`
	Project string            `json:"project"`
	State   *sim.State        `json:"-"`
	Session *cli.Session      `json:"-"`
	Seed    int64             `json:"seed"`
}

// builtinBaselines are Go-defined starting points.
var builtinBaselines = map[string]func(st *sim.State, p *sim.Project){
	"empty":           func(st *sim.State, p *sim.Project) {},
	"default-network": func(st *sim.State, p *sim.Project) { st.DefaultNetwork(p) },
	"decoy-projects": func(st *sim.State, p *sim.Project) {
		for _, suffix := range []string{"-prod", "-legacy"} {
			d := st.NewProject(p.ID+suffix, "folders/771300")
			d.Labels["purpose"] = "production"
			d.IAM.AddBinding("roles/viewer", "user:student@gcplab.dev", nil)
		}
		st.Folders["folders/771300"] = &sim.Folder{ID: "771300", DisplayName: "production", Parent: "organizations/240158832107"}
		p.Labels["purpose"] = "training-lab"
	},
	"restricted-org": func(st *sim.State, p *sim.Project) {
		st.Org.OrgPolicies["iam.allowedPolicyMemberDomains"] = &sim.OrgPolicy{Constraint: "iam.allowedPolicyMemberDomains", Enforce: true}
		st.Org.OrgPolicies["storage.publicAccessPrevention"] = &sim.OrgPolicy{Constraint: "storage.publicAccessPrevention", Enforce: true}
	},
}

// AdminSession returns a platform session on the world (used by setup, faults
// and the official solution runner in "admin" mode).
func AdminSession(st *sim.State, project string, l *Lab) *cli.Session {
	s := cli.NewSession(st, project, AdminAccount)
	s.Region, s.Zone, s.NoTick = l.Region, l.Zone, true
	s.Policy = PlatformPolicy
	for k, v := range l.Files {
		s.Files[k] = v
	}
	return s
}

// Provision builds a world for a lab variant.
func Provision(base *Lab, seed int64, projectID string) (*World, error) {
	l, params, err := base.Variant(seed, projectID)
	if err != nil {
		return nil, err
	}
	var st *sim.State
	if len(l.InitialState) > 0 {
		// Persistent world (company simulation): continue from the saved state.
		st, err = sim.Unmarshal(l.InitialState)
		if err != nil {
			return nil, fmt.Errorf("lab %s: initial state: %w", l.ID, err)
		}
		if st.Projects[projectID] == nil {
			return nil, fmt.Errorf("lab %s: project %s not in the saved world", l.ID, projectID)
		}
		d := st.Clock
		st.Clock = time.Date(d.Year(), d.Month(), d.Day(), 8, 0, 0, 0, time.UTC)
	} else {
		st = sim.New(seed, projectID, "user:"+AdminAccount)
		st.Clock = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	}
	p := st.Projects[projectID]
	st.Org.IAM.AddBinding("roles/owner", "user:"+AdminAccount, nil)
	for _, r := range l.Student.Roles {
		p.IAM.AddBinding(r, "user:"+l.Student.Account, nil)
	}
	for _, b := range l.Baseline {
		if len(l.InitialState) > 0 {
			break
		}
		if fn, ok := builtinBaselines[b]; ok {
			fn(st, p)
			continue
		}
		script, err := os.ReadFile(filepath.Join(BaselineDir, b+".sh"))
		if err != nil {
			return nil, fmt.Errorf("lab %s: unknown baseline %q", l.ID, b)
		}
		rendered, _, err := renderWith(string(script), params)
		if err != nil {
			return nil, err
		}
		if err := runScript(AdminSession(st, projectID, l), rendered, "baseline "+b); err != nil {
			return nil, fmt.Errorf("lab %s: %w", l.ID, err)
		}
	}
	admin := AdminSession(st, projectID, l)
	if strings.TrimSpace(l.Setup) != "" {
		if err := runScript(admin, l.Setup, "setup"); err != nil {
			return nil, fmt.Errorf("lab %s: %w", l.ID, err)
		}
	}
	st.Clock = time.Date(st.Clock.Year(), st.Clock.Month(), st.Clock.Day(), 8, 50, 0, 0, time.UTC)
	for i, f := range l.Faults {
		if err := ApplyFault(st, projectID, l, f); err != nil {
			return nil, fmt.Errorf("lab %s: fault %d: %w", l.ID, i, err)
		}
	}
	st.Traffic = nil
	for _, t := range l.Traffic {
		if t.Project == "" {
			t.Project = projectID
		}
		if t.RPS == 0 {
			t.RPS = 20
		}
		st.Traffic = append(st.Traffic, t)
	}
	st.Step(10)
	sess := cli.NewSession(st, projectID, l.Student.Account)
	sess.Policy = l.Policy
	AttachInterview(sess, l)
	if l.Ticket != nil || len(l.Actors) > 0 {
		d := &desk.Desk{Actors: l.Actors}
		if l.Ticket != nil {
			t := *l.Ticket
			t.Comments = append([]desk.Comment{}, l.Ticket.Comments...)
			if t.Status == "" {
				t.Status = "NEW"
			}
			d.Ticket = &t
			for _, a := range t.Attachments {
				sess.Files[a.Name] = a.Content
			}
		}
		sess.Desk = d
	}
	if l.Student.Unconfigured {
		sess.Project = ""
	}
	for k, v := range l.Files {
		sess.Files[k] = v
	}
	return &World{Lab: l, Params: params, Project: projectID, State: st, Session: sess, Seed: seed}, nil
}

func renderWith(s string, params map[string]string) (string, map[string]string, error) {
	l := &Lab{Setup: s, Params: map[string][]string{}}
	for k, v := range params {
		l.Params[k] = []string{v}
	}
	l.Region, l.Zone = params["region"], params["zone"]
	v, p, err := l.Variant(0, params["project"])
	if err != nil {
		return "", nil, err
	}
	return v.Setup, p, nil
}

// runScript executes a multi-line script, failing on the first error.
func runScript(s *cli.Session, script, what string) error {
	// Execute as one input so heredocs work, but detect failures per line by
	// running line groups individually.
	for _, chunk := range splitScript(script) {
		r := s.Exec(chunk)
		if r.Exit != 0 {
			return fmt.Errorf("%s failed at `%s`: %s", what, firstLine(chunk), strings.TrimSpace(r.Output))
		}
	}
	return nil
}

func firstLine(s string) string {
	return strings.SplitN(strings.TrimSpace(s), "\n", 2)[0]
}

// splitScript groups lines into executable chunks keeping heredocs and
// continuation lines together. Lines starting with "!" are allowed to fail.
func splitScript(script string) []string {
	var out []string
	lines := strings.Split(strings.ReplaceAll(script, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		chunk := line
		for strings.HasSuffix(strings.TrimRight(chunk, " "), "\\") && i+1 < len(lines) {
			i++
			chunk += "\n" + lines[i]
		}
		if m := heredocDelim(chunk); m != "" {
			for i+1 < len(lines) {
				i++
				chunk += "\n" + lines[i]
				if strings.TrimSpace(lines[i]) == m {
					break
				}
			}
		}
		// keep for/if blocks together
		for cli.OpenBlocks(chunk) > 0 && i+1 < len(lines) {
			i++
			chunk += "\n" + lines[i]
		}
		out = append(out, chunk)
	}
	return out
}

func heredocDelim(line string) string {
	i := strings.Index(line, "<<")
	if i < 0 {
		return ""
	}
	rest := strings.TrimLeft(line[i+2:], "-")
	rest = strings.TrimSpace(rest)
	rest = strings.Trim(strings.Fields(rest + " ")[0], `'"`)
	return rest
}

// RunSolution executes the official solution as the student and reports failures.
func RunSolution(w *World) (string, error) {
	var log strings.Builder
	for _, chunk := range splitScript(w.Lab.Solution) {
		allowFail := strings.HasPrefix(strings.TrimSpace(chunk), "!")
		if allowFail {
			chunk = strings.TrimPrefix(strings.TrimSpace(chunk), "!")
		}
		r := w.Session.Exec(chunk)
		log.WriteString("$ " + firstLine(chunk) + "\n" + r.Output)
		if r.Exit != 0 && !allowFail {
			return log.String(), fmt.Errorf("solution step failed: `%s`: %s", firstLine(chunk), strings.TrimSpace(r.Output))
		}
	}
	return log.String(), nil
}

// RunBaseline applies a builtin or scripted baseline to a project of an
// existing world (used to build persistent company worlds).
func RunBaseline(st *sim.State, projectID, name string, extra map[string]string) error {
	p := st.Projects[projectID]
	if p == nil {
		return fmt.Errorf("project %s not found", projectID)
	}
	if fn, ok := builtinBaselines[name]; ok {
		fn(st, p)
		return nil
	}
	script, err := os.ReadFile(filepath.Join(BaselineDir, name+".sh"))
	if err != nil {
		return fmt.Errorf("unknown baseline %q", name)
	}
	params := map[string]string{"project": projectID, "region": "europe-west1", "zone": "europe-west1-b"}
	for k, v := range extra {
		params[k] = v
	}
	rendered, _, err := renderWith(string(script), params)
	if err != nil {
		return err
	}
	s := cli.NewSession(st, projectID, AdminAccount)
	s.Region, s.Zone, s.NoTick = "europe-west1", "europe-west1-b", true
	s.Policy = PlatformPolicy
	return runScript(s, rendered, "baseline "+name)
}

// AttachInterview loads interview questions into a session (interview mode).
// Answers already given (restored sessions) are preserved.
func AttachInterview(sess *cli.Session, l *Lab) {
	if l.Mode != "interview" || len(l.Quiz) == 0 {
		return
	}
	if sess.Interview == nil {
		sess.Interview = &cli.Interview{Answers: map[string][]int{}, Justifications: map[string]string{}}
	}
	sess.Interview.Questions = nil
	for _, q := range l.Quiz {
		sess.Interview.Questions = append(sess.Interview.Questions, cli.InterviewQuestion{ID: q.ID, Question: q.Question, Options: q.Options, Answer: q.Answer, After: q.After, When: q.When, Probe: q.Probe, Justify: q.Justify})
	}
}
