package scenario

import (
	"fmt"
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
	st := sim.New(seed, projectID, "user:"+AdminAccount)
	st.Clock = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	p := st.Projects[projectID]
	st.Org.IAM.AddBinding("roles/owner", "user:"+AdminAccount, nil)
	for _, r := range l.Student.Roles {
		p.IAM.AddBinding(r, "user:"+l.Student.Account, nil)
	}
	for _, b := range l.Baseline {
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
	st.Clock = time.Date(2026, 9, 1, 8, 50, 0, 0, time.UTC)
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
