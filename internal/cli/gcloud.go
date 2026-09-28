package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

// Cmd is the context passed to gcloud handlers.
type Cmd struct {
	S     *Session
	Path  string
	Args  []string
	F     Flags
	Stdin string
}

type handler func(c *Cmd) (any, error)

var registry = map[string]handler{}

func reg(path string, h handler) { registry[path] = h }

// gcloudErr wraps an error with the gcloud command prefix.
func (s *Session) gcloud(args []string, stdin string) (string, error) {
	pos, f := parseArgs(args)
	if len(pos) == 0 || pos[0] == "help" || f["help"] != nil {
		return "Usage: gcloud GROUP [GROUP...] COMMAND [ARGS] [--flags]\nRun `help` to list supported tools.\n", nil
	}
	var path string
	var h handler
	var rest []string
	for n := min(5, len(pos)); n >= 1; n-- {
		p := strings.Join(pos[:n], " ")
		if hh, ok := registry[p]; ok {
			path, h, rest = p, hh, pos[n:]
			break
		}
	}
	// "alpha" and "beta" tracks map to GA commands
	if h == nil && (pos[0] == "alpha" || pos[0] == "beta") {
		return s.gcloud(args[1:], stdin)
	}
	if h == nil {
		return "", fail(2, "ERROR: (gcloud) Invalid choice: '%s'.\nThis command is not available in the simulator. Maybe you meant:\n%s", strings.Join(pos, " "), suggest(pos))
	}
	c := &Cmd{S: s, Path: path, Args: rest, F: f, Stdin: stdin}
	prev := s.Impersonate
	if v := last(f["impersonate-service-account"]); v != "" {
		s.Impersonate = v
		defer func() { s.Impersonate = prev }()
	}
	if s.Impersonate != "" {
		if err := s.checkImpersonation(); err != nil {
			return "", fail(1, "ERROR: (gcloud.%s) %s", strings.ReplaceAll(path, " ", "."), err.Error())
		}
	}
	out, err := h(c)
	if err != nil {
		if ee, ok := err.(*exitErr); ok && strings.HasPrefix(ee.msg, "ERROR:") {
			return "", err
		}
		return "", fail(1, "ERROR: (gcloud.%s) %s", strings.ReplaceAll(path, " ", "."), err.Error())
	}
	text, err := render(out, f)
	if err != nil {
		return "", err
	}
	return text, nil
}

func suggest(pos []string) string {
	var cands []string
	for k := range registry {
		if strings.HasPrefix(k, pos[0]) {
			cands = append(cands, "  gcloud "+k)
		}
	}
	sort.Strings(cands)
	if len(cands) > 12 {
		cands = cands[:12]
	}
	return strings.Join(cands, "\n")
}

func (s *Session) checkImpersonation() error {
	caller := "user:" + s.Account
	if strings.HasSuffix(s.Account, ".gserviceaccount.com") {
		caller = "serviceAccount:" + s.Account
	}
	for _, p := range s.State.Projects {
		if sa := p.ServiceAccounts[s.Impersonate]; sa != nil {
			if s.State.Allowed(caller, "iam.serviceAccounts.getAccessToken", sim.Resource{Project: p.ID, Type: "iam.googleapis.com/ServiceAccount", Name: "projects/" + p.ID + "/serviceAccounts/" + sa.Email, Service: "iam.googleapis.com", Policies: []*sim.Policy{&sa.IAM}}) {
				return nil
			}
			return fmt.Errorf("PERMISSION_DENIED: Failed to impersonate [%s]. Make sure the account that's trying to impersonate it has access to the service account itself and the \"roles/iam.serviceAccountTokenCreator\" role.", s.Impersonate)
		}
	}
	return fmt.Errorf("NOT_FOUND: service account [%s] does not exist", s.Impersonate)
}

// --- helpers --------------------------------------------------------------

func (c *Cmd) Str(name, def string) string {
	if v := last(c.F[name]); v != "" {
		return v
	}
	return def
}

func (c *Cmd) Has(name string) bool { return c.F[name] != nil }

func (c *Cmd) Bool(name string) bool {
	v := last(c.F[name])
	return v == "true" || v == "True" || v == "1" || v == "yes"
}

func (c *Cmd) Int(name string, def int) int {
	if v := last(c.F[name]); v != "" {
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSuffix(v, "s"), "GB"))
		if err == nil {
			return n
		}
	}
	return def
}

// List returns a comma-separated flag as a list (merging repeats).
func (c *Cmd) List(name string) []string {
	var out []string
	for _, v := range c.F[name] {
		for _, p := range splitComma(v) {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// splitComma splits on commas not inside brackets or quotes.
func splitComma(s string) []string {
	var out []string
	depth := 0
	start := 0
	for i, ch := range s {
		switch ch {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// KV parses KEY=VALUE lists.
func (c *Cmd) KV(name string) map[string]string {
	out := map[string]string{}
	for _, v := range c.F[name] {
		sep := ","
		if strings.HasPrefix(v, "^") {
			if i := strings.Index(v[1:], "^"); i >= 0 {
				sep = v[1 : i+1]
				v = v[i+2:]
			}
		}
		for _, p := range strings.Split(v, sep) {
			if k, val, ok := strings.Cut(p, "="); ok {
				out[strings.TrimSpace(k)] = val
			}
		}
	}
	return out
}

func (c *Cmd) ProjectID() string { return c.Str("project", c.S.Project) }

func (c *Cmd) P() (*sim.Project, error) {
	id := c.ProjectID()
	if id == "" {
		return nil, fmt.Errorf("The required property [project] is not currently set.\nIt can be set on a per-command basis by re-running your command with the [--project] flag.\n\nYou may set it for your current workspace by running:\n\n  $ gcloud config set project VALUE")
	}
	p := c.S.State.Projects[id]
	if p == nil || p.State != "ACTIVE" {
		return nil, fmt.Errorf("PERMISSION_DENIED: The caller does not have permission, or project [%s] does not exist (it may have been deleted).", id)
	}
	return p, nil
}

func (c *Cmd) Principal() string { return c.S.Principal() }

func (c *Cmd) Need(perm string, r sim.Resource) error {
	if r.Project == "" {
		r.Project = c.ProjectID()
	}
	if c.S.State.Allowed(c.Principal(), perm, r) {
		return nil
	}
	who := strings.TrimPrefix(strings.TrimPrefix(c.Principal(), "user:"), "serviceAccount:")
	return fmt.Errorf("PERMISSION_DENIED: Permission '%s' denied on resource '%s' (or it may not exist).\n  principal: %s\n  Hint: grant a role that contains %s, following least privilege.", perm, r.Name, who, perm)
}

// NeedProject checks a project-level permission.
func (c *Cmd) NeedProject(perm string) error {
	return c.Need(perm, sim.ProjectResource(c.ProjectID()))
}

func (c *Cmd) API(svc string) error {
	p, err := c.P()
	if err != nil {
		return err
	}
	if !p.Services[svc] {
		return fmt.Errorf("API [%s] not enabled on project [%s]. Enable it with:\n  gcloud services enable %s --project=%s", svc, p.Number, svc, p.ID)
	}
	return nil
}

func (c *Cmd) Audit(service, method, resource string) {
	c.S.State.Audit(c.ProjectID(), c.Principal(), service, method, resource)
}

func (c *Cmd) Arg(i int, what string) (string, error) {
	if i < len(c.Args) {
		return c.Args[i], nil
	}
	return "", fmt.Errorf("argument %s: Must be specified.", what)
}

func (c *Cmd) Zone() (string, error) {
	z := c.Str("zone", c.S.Zone)
	if z == "" {
		return "", fmt.Errorf("Underspecified resource [%s]. Specify the [--zone] flag or set the [compute/zone] property.", strings.Join(c.Args, " "))
	}
	if !sim.ValidZone(z) {
		return "", fmt.Errorf("Could not fetch resource:\n - Invalid value for field 'zone': '%s'. Unknown zone.", z)
	}
	if err := c.S.checkRegion(sim.RegionOf(z), c.ProjectID()); err != nil {
		return "", err
	}
	return z, nil
}

func (c *Cmd) RegionFlag() (string, error) {
	r := c.Str("region", c.S.Region)
	if r == "" {
		return "", fmt.Errorf("Underspecified resource. Specify the [--region] flag or set the [compute/region] property.")
	}
	if !sim.ValidRegions[r] {
		return "", fmt.Errorf("Invalid value for field 'region': '%s'. Unknown region.", r)
	}
	if err := c.S.checkRegion(r, c.ProjectID()); err != nil {
		return "", err
	}
	return r, nil
}

func (s *Session) checkRegion(region, project string) error {
	if len(s.Policy.AllowedRegions) > 0 {
		ok := false
		for _, r := range s.Policy.AllowedRegions {
			if r == region {
				ok = true
			}
		}
		if !ok {
			return fmt.Errorf("Quota exceeded or region not allowed in this lab: region '%s' is outside the lab allowlist %v", region, s.Policy.AllowedRegions)
		}
	}
	if !s.State.OrgPolicyAllows(project, "gcp.resourceLocations", region) {
		return fmt.Errorf("FAILED_PRECONDITION: Location %s violates constraint constraints/gcp.resourceLocations on the resource projects/%s.", region, project)
	}
	return nil
}

func (c *Cmd) Created(kind, url string) string {
	return fmt.Sprintf("Created [https://www.googleapis.com/%s].\n", url)
}

func (c *Cmd) Quiet() bool { return c.Bool("quiet") || c.Bool("q") }

// member normalises IAM members.
func member(m string) (string, error) {
	if m == "allUsers" || m == "allAuthenticatedUsers" {
		return m, nil
	}
	for _, p := range []string{"user:", "serviceAccount:", "group:", "domain:", "principal:", "principalSet:", "deleted:"} {
		if strings.HasPrefix(m, p) {
			return m, nil
		}
	}
	return "", fmt.Errorf("Invalid value for [--member]: '%s'. Must be prefixed with user:, serviceAccount:, group:, domain: or be allUsers/allAuthenticatedUsers.", m)
}

func parseCondition(v string) (*sim.Condition, error) {
	if v == "" || v == "None" {
		return nil, nil
	}
	cond := &sim.Condition{}
	for _, part := range splitComma(v) {
		k, val, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "expression":
			cond.Expression = val
		case "title":
			cond.Title = val
		case "description":
			cond.Description = val
		}
	}
	if cond.Expression == "" || cond.Title == "" {
		return nil, fmt.Errorf("Invalid value for [--condition]: condition must include expression= and title=")
	}
	if _, err := sim.EvalCondition(cond.Expression, sim.CondContext{}); err != nil && !strings.Contains(err.Error(), "type mismatch") {
		return nil, fmt.Errorf("INVALID_ARGUMENT: Condition expression is invalid: %v", err)
	}
	return cond, nil
}

func (c *Cmd) bindingArgs() (string, string, *sim.Condition, error) {
	m, err := member(c.Str("member", ""))
	if err != nil {
		return "", "", nil, err
	}
	role := c.Str("role", "")
	if role == "" {
		return "", "", nil, fmt.Errorf("argument --role: Must be specified.")
	}
	if !strings.HasPrefix(role, "roles/") && !strings.HasPrefix(role, "projects/") && !strings.HasPrefix(role, "organizations/") {
		role = "roles/" + role
	}
	if !c.S.State.RoleExists(role) {
		return "", "", nil, fmt.Errorf("INVALID_ARGUMENT: Role %s is not supported for this resource.", role)
	}
	cond, err := parseCondition(c.Str("condition", ""))
	if err != nil {
		return "", "", nil, err
	}
	if strings.HasPrefix(m, "serviceAccount:") && !strings.Contains(m, ".svc.id.goog[") {
		email := strings.TrimPrefix(m, "serviceAccount:")
		found := strings.HasSuffix(email, "gserviceaccount.com") && (strings.Contains(email, "@cloudservices") || strings.Contains(email, "@gcp-sa-") || strings.Contains(email, "-compute@") || strings.Contains(email, "@cloudbuild") || strings.Contains(email, "service-"))
		for _, p := range c.S.State.Projects {
			if p.ServiceAccounts[email] != nil {
				found = true
			}
		}
		if !found {
			return "", "", nil, fmt.Errorf("INVALID_ARGUMENT: Service account %s does not exist.", email)
		}
	}
	return m, role, cond, nil
}

func policyView(p *sim.Policy) Obj {
	return Obj{V: map[string]any{"bindings": p.Bindings, "etag": "BwY" + strconv.Itoa(len(p.Bindings)*7919), "version": max(1, p.Version)}}
}

func init() {
	// ---- config / auth / projects -----------------------------------------
	reg("config set", func(c *Cmd) (any, error) {
		k, _ := c.Arg(0, "SECTION/PROPERTY")
		v, err := c.Arg(1, "VALUE")
		if err != nil {
			return nil, err
		}
		switch k {
		case "project", "core/project":
			if c.S.State.Projects[v] == nil {
				c.S.Project = v
				return "WARNING: You do not appear to have access to project [" + v + "] or it does not exist.\nUpdated property [core/project].\n", nil
			}
			c.S.Project = v
			c.S.Env["GOOGLE_CLOUD_PROJECT"] = v
		case "compute/region":
			if !sim.ValidRegions[v] {
				return nil, fmt.Errorf("Invalid value for [compute/region]: %s is not a valid region", v)
			}
			c.S.Region = v
		case "compute/zone":
			if !sim.ValidZone(v) {
				return nil, fmt.Errorf("Invalid value for [compute/zone]: %s is not a valid zone", v)
			}
			c.S.Zone = v
		case "account", "core/account":
			c.S.Account = v
		case "auth/impersonate_service_account":
			c.S.Impersonate = v
		default:
			return "Updated property [" + k + "] (ignored by the simulator).\n", nil
		}
		return "Updated property [" + k + "].\n", nil
	})
	reg("config unset", func(c *Cmd) (any, error) {
		k, _ := c.Arg(0, "PROPERTY")
		switch k {
		case "project", "core/project":
			c.S.Project = ""
		case "compute/region":
			c.S.Region = ""
		case "compute/zone":
			c.S.Zone = ""
		case "auth/impersonate_service_account":
			c.S.Impersonate = ""
		}
		return "Unset property [" + k + "].\n", nil
	})
	reg("config get", func(c *Cmd) (any, error) {
		k, _ := c.Arg(0, "PROPERTY")
		switch k {
		case "project", "core/project":
			return c.S.Project + "\n", nil
		case "compute/region":
			return c.S.Region + "\n", nil
		case "compute/zone":
			return c.S.Zone + "\n", nil
		case "account", "core/account":
			return c.S.Account + "\n", nil
		}
		return "(unset)\n", nil
	})
	reg("config get-value", registry["config get"])
	reg("config list", func(c *Cmd) (any, error) {
		out := "[compute]\n"
		if c.S.Region != "" {
			out += "region = " + c.S.Region + "\n"
		}
		if c.S.Zone != "" {
			out += "zone = " + c.S.Zone + "\n"
		}
		out += "[core]\naccount = " + c.S.Account + "\n"
		if c.S.Project != "" {
			out += "project = " + c.S.Project + "\n"
		}
		if c.S.Impersonate != "" {
			out += "[auth]\nimpersonate_service_account = " + c.S.Impersonate + "\n"
		}
		return out + "\nYour active configuration is: [default]\n", nil
	})
	reg("config configurations list", func(c *Cmd) (any, error) {
		return Table{Cols: []Col{{"NAME", "name"}, {"IS_ACTIVE", "active"}, {"ACCOUNT", "account"}, {"PROJECT", "project"}, {"COMPUTE_DEFAULT_ZONE", "zone"}, {"COMPUTE_DEFAULT_REGION", "region"}},
			Rows: []any{map[string]any{"name": "default", "active": "True", "account": c.S.Account, "project": c.S.Project, "zone": c.S.Zone, "region": c.S.Region}}}, nil
	})
	reg("auth list", func(c *Cmd) (any, error) {
		rows := []any{map[string]any{"active": "*", "account": c.S.Account}}
		for k := range c.S.Credentials {
			rows = append(rows, map[string]any{"active": "", "account": k})
		}
		return Table{Cols: []Col{{"ACTIVE", "active"}, {"ACCOUNT", "account"}}, Rows: rows}, nil
	})
	reg("auth print-access-token", func(c *Cmd) (any, error) { return "ya29.sim-" + c.S.State.ID(40) + "\n", nil })
	reg("auth print-identity-token", func(c *Cmd) (any, error) {
		return IdentityToken(c.Principal()) + "\n", nil
	})
	reg("auth login", func(c *Cmd) (any, error) { return "You are already authenticated as [" + c.S.Account + "] in the lab.\n", nil })
	reg("auth application-default login", registry["auth login"])
	reg("auth configure-docker", func(c *Cmd) (any, error) {
		for _, h := range append(c.Args, "gcr.io") {
			for _, x := range strings.Split(h, ",") {
				c.S.DockerAuth[x] = true
			}
		}
		return "Adding credentials for: " + strings.Join(c.Args, ",") + "\nDocker configuration file updated.\n", nil
	})
	reg("auth activate-service-account", func(c *Cmd) (any, error) {
		kf := c.Str("key-file", "")
		content, ok := c.S.Files[c.S.path(kf)]
		if !ok {
			return nil, fmt.Errorf("Could not read json file %s: No such file or directory", kf)
		}
		email := ""
		if i := strings.Index(content, "\"client_email\": \""); i >= 0 {
			rest := content[i+17:]
			email = rest[:strings.Index(rest, "\"")]
		}
		if email == "" {
			return nil, fmt.Errorf("invalid key file")
		}
		c.S.Credentials[email] = kf
		c.S.Account = email
		return "Activated service account credentials for: [" + email + "]\n", nil
	})
	reg("auth revoke", func(c *Cmd) (any, error) { return "Revoked credentials.\n", nil })

	reg("projects list", func(c *Cmd) (any, error) {
		var rows []any
		for _, id := range sim.SortedKeys(c.S.State.Projects) {
			p := c.S.State.Projects[id]
			if p.State != "ACTIVE" {
				continue
			}
			if c.S.State.Allowed(c.Principal(), "resourcemanager.projects.get", sim.ProjectResource(id)) {
				rows = append(rows, map[string]any{"projectId": p.ID, "name": p.Name, "projectNumber": p.Number, "parent": p.Parent, "labels": p.Labels})
			}
		}
		return Table{Cols: []Col{{"PROJECT_ID", "projectId"}, {"NAME", "name"}, {"PROJECT_NUMBER", "projectNumber"}}, Rows: rows}, nil
	})
	reg("projects describe", func(c *Cmd) (any, error) {
		id, err := c.Arg(0, "PROJECT_ID")
		if err != nil {
			return nil, err
		}
		p := c.S.State.Projects[id]
		if p == nil || !c.S.State.Allowed(c.Principal(), "resourcemanager.projects.get", sim.ProjectResource(id)) {
			return nil, fmt.Errorf("PERMISSION_DENIED: The caller does not have permission")
		}
		parentType, parentID, _ := strings.Cut(p.Parent, "/")
		return Obj{V: map[string]any{"createTime": "2026-08-30T08:00:00.000Z", "lifecycleState": p.State, "name": p.Name, "parent": map[string]any{"id": parentID, "type": strings.TrimSuffix(parentType, "s")}, "projectId": p.ID, "projectNumber": p.Number, "labels": p.Labels}}, nil
	})
	reg("projects get-iam-policy", func(c *Cmd) (any, error) {
		id, err := c.Arg(0, "PROJECT_ID")
		if err != nil {
			return nil, err
		}
		p := c.S.State.Projects[id]
		if p == nil {
			return nil, fmt.Errorf("project %s not found", id)
		}
		if err := c.Need("resourcemanager.projects.getIamPolicy", sim.ProjectResource(id)); err != nil {
			return nil, err
		}
		if c.Has("flatten") || strings.HasPrefix(c.Str("format", ""), "table") || strings.HasPrefix(c.Str("format", ""), "value") || strings.HasPrefix(c.Str("format", ""), "csv") {
			return Table{Cols: []Col{{"ROLE", "bindings.role"}, {"MEMBERS", "bindings.members"}}, Rows: []any{map[string]any{"bindings": p.IAM.Bindings}}}, nil
		}
		return policyView(&p.IAM), nil
	})
	reg("projects add-iam-policy-binding", func(c *Cmd) (any, error) {
		id, err := c.Arg(0, "PROJECT_ID")
		if err != nil {
			return nil, err
		}
		p := c.S.State.Projects[id]
		if p == nil {
			return nil, fmt.Errorf("project %s not found", id)
		}
		m, role, cond, err := c.bindingArgs()
		if err != nil {
			return nil, err
		}
		if err := c.Need("resourcemanager.projects.setIamPolicy", sim.ProjectResource(id)); err != nil {
			return nil, err
		}
		if (m == "allUsers" || m == "allAuthenticatedUsers") && c.S.State.OrgPolicyEnforced(id, "iam.allowedPolicyMemberDomains") {
			return nil, fmt.Errorf("FAILED_PRECONDITION: One or more users named in the policy do not belong to a permitted customer (constraints/iam.allowedPolicyMemberDomains).")
		}
		if strings.HasPrefix(m, "user:") && !strings.HasSuffix(m, "@gcplab.dev") && c.S.State.OrgPolicyEnforced(id, "iam.allowedPolicyMemberDomains") {
			return nil, fmt.Errorf("FAILED_PRECONDITION: One or more users named in the policy do not belong to a permitted customer.")
		}
		p.IAM.AddBinding(role, m, cond)
		c.S.State.Audit(id, c.Principal(), "cloudresourcemanager.googleapis.com", "SetIamPolicy", "projects/"+id)
		out := "Updated IAM policy for project [" + id + "].\n"
		r, _ := render(policyView(&p.IAM), Flags{"format": c.F["format"]})
		return out + r, nil
	})
	reg("projects remove-iam-policy-binding", func(c *Cmd) (any, error) {
		id, err := c.Arg(0, "PROJECT_ID")
		if err != nil {
			return nil, err
		}
		p := c.S.State.Projects[id]
		if p == nil {
			return nil, fmt.Errorf("project %s not found", id)
		}
		role := c.Str("role", "")
		if !strings.HasPrefix(role, "roles/") && !strings.HasPrefix(role, "projects/") {
			role = "roles/" + role
		}
		if err := c.Need("resourcemanager.projects.setIamPolicy", sim.ProjectResource(id)); err != nil {
			return nil, err
		}
		if !p.IAM.RemoveBinding(role, c.Str("member", "")) {
			return nil, fmt.Errorf("Policy binding with the specified principal, role, and condition not found!")
		}
		c.S.State.Audit(id, c.Principal(), "cloudresourcemanager.googleapis.com", "SetIamPolicy", "projects/"+id)
		return "Updated IAM policy for project [" + id + "].\n", nil
	})
	reg("projects create", func(c *Cmd) (any, error) {
		return nil, fmt.Errorf("PERMISSION_DENIED: Project creation is disabled in lab sandboxes (projects are leased from the training pool).")
	})
	reg("projects delete", func(c *Cmd) (any, error) {
		return nil, fmt.Errorf("PERMISSION_DENIED: Deleting the lab project is not allowed.")
	})
	reg("organizations list", func(c *Cmd) (any, error) {
		o := c.S.State.Org
		return Table{Cols: []Col{{"DISPLAY_NAME", "displayName"}, {"ID", "id"}, {"DIRECTORY_CUSTOMER_ID", "customer"}}, Rows: []any{map[string]any{"displayName": o.DisplayName, "id": o.ID, "customer": "C03xgjk2a"}}}, nil
	})
	reg("resource-manager folders list", func(c *Cmd) (any, error) {
		var rows []any
		for _, k := range sim.SortedKeys(c.S.State.Folders) {
			f := c.S.State.Folders[k]
			rows = append(rows, map[string]any{"displayName": f.DisplayName, "parent": f.Parent, "id": f.ID})
		}
		return Table{Cols: []Col{{"DISPLAY_NAME", "displayName"}, {"PARENT_NAME", "parent"}, {"ID", "id"}}, Rows: rows}, nil
	})
	reg("projects get-ancestors", func(c *Cmd) (any, error) {
		id, _ := c.Arg(0, "PROJECT_ID")
		p := c.S.State.Projects[id]
		if p == nil {
			return nil, fmt.Errorf("project %s not found", id)
		}
		rows := []any{map[string]any{"id": id, "type": "project"}}
		parent := p.Parent
		for parent != "" {
			t, pid, _ := strings.Cut(parent, "/")
			rows = append(rows, map[string]any{"id": pid, "type": strings.TrimSuffix(t, "s")})
			if f := c.S.State.Folders[parent]; f != nil {
				parent = f.Parent
			} else {
				parent = ""
			}
		}
		return Table{Cols: []Col{{"ID", "id"}, {"TYPE", "type"}}, Rows: rows}, nil
	})
	reg("info", func(c *Cmd) (any, error) {
		return fmt.Sprintf("Google Cloud SDK [simulated 520.0.0]\n\nAccount: [%s]\nProject: [%s]\nCurrent Properties:\n  [compute]\n    region: [%s]\n    zone: [%s]\n", c.S.Account, c.S.Project, c.S.Region, c.S.Zone), nil
	})
	reg("version", func(c *Cmd) (any, error) {
		return "Google Cloud SDK 520.0.0 (GCP Lab Simulator F0)\nbq 2.1.14\ncore 2026.09.01\ngsutil 5.33\nkubectl 1.31.1\n", nil
	})

	// ---- services ---------------------------------------------------------
	reg("services list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		avail := c.Bool("available")
		for _, s := range sim.DefaultEnabledAPIs {
			if p.Services[s] != avail || avail {
				if !avail && !p.Services[s] {
					continue
				}
				rows = append(rows, map[string]any{"config": map[string]any{"name": s, "title": s}, "state": map[bool]string{true: "ENABLED", false: "DISABLED"}[p.Services[s]]})
			}
		}
		for _, s := range sim.SortedKeys(p.Services) {
			known := false
			for _, d := range sim.DefaultEnabledAPIs {
				if d == s {
					known = true
				}
			}
			if !known && p.Services[s] {
				rows = append(rows, map[string]any{"config": map[string]any{"name": s, "title": s}, "state": "ENABLED"})
			}
		}
		return Table{Cols: []Col{{"NAME", "config.name"}, {"TITLE", "config.title"}}, Rows: rows}, nil
	})
	reg("services enable", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		if len(c.Args) == 0 {
			return nil, fmt.Errorf("argument SERVICE: Must be specified.")
		}
		if err := c.NeedProject("serviceusage.services.enable"); err != nil {
			return nil, err
		}
		for _, s := range c.Args {
			if !strings.HasSuffix(s, ".googleapis.com") {
				return nil, fmt.Errorf("Invalid service name %q", s)
			}
			if blocked := c.S.State.Extra["api-blocked:"+s]; blocked != "" {
				return nil, fmt.Errorf("FAILED_PRECONDITION: Service %s is not in the API allowlist of this lab (%s).", s, blocked)
			}
			p.Services[s] = true
			c.Audit("serviceusage.googleapis.com", "google.api.serviceusage.v1.ServiceUsage.EnableService", "projects/"+p.Number+"/services/"+s)
		}
		return "Operation \"operations/acf.p2-" + c.S.State.ID(12) + "\" finished successfully.\n", nil
	})
	reg("services disable", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("serviceusage.services.disable"); err != nil {
			return nil, err
		}
		for _, s := range c.Args {
			p.Services[s] = false
		}
		return "Operation finished successfully.\n", nil
	})
	reg("services vpc-peerings connect", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		netName := c.Str("network", "default")
		n := p.Networks[netName]
		if n == nil {
			return nil, fmt.Errorf("network %s not found", netName)
		}
		ranges := c.List("ranges")
		for _, r := range ranges {
			a := p.Addresses[r]
			if a == nil || a.Purpose != "VPC_PEERING" {
				return nil, fmt.Errorf("FAILED_PRECONDITION: Allocated IP range '%s' not found in network (create it with `gcloud compute addresses create %s --global --purpose=VPC_PEERING --prefix-length=16 --network=%s`).", r, r, netName)
			}
		}
		if len(ranges) == 0 {
			return nil, fmt.Errorf("argument --ranges: Must be specified.")
		}
		n.PSAConnected = true
		n.PSARanges = ranges
		c.Audit("servicenetworking.googleapis.com", "google.cloud.servicenetworking.v1.ServicesConnect", "services/servicenetworking.googleapis.com/connections/servicenetworking-googleapis-com")
		return "Operation \"operations/pssn." + c.S.State.ID(10) + "\" finished successfully.\n", nil
	})
	reg("services vpc-peerings list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Networks) {
			n := p.Networks[k]
			if n.PSAConnected {
				rows = append(rows, map[string]any{"network": k, "peering": "servicenetworking-googleapis-com", "reservedPeeringRanges": n.PSARanges})
			}
		}
		return Table{Cols: []Col{{"NETWORK", "network"}, {"PEERING", "peering"}, {"RESERVED_PEERING_RANGES", "reservedPeeringRanges"}}, Rows: rows}, nil
	})

	// ---- org policy -------------------------------------------------------
	orgPolicyToggle := func(enforce bool) handler {
		return func(c *Cmd) (any, error) {
			cons, err := c.Arg(0, "CONSTRAINT")
			if err != nil {
				return nil, err
			}
			cons = strings.TrimPrefix(cons, "constraints/")
			p, err := c.P()
			if err != nil {
				return nil, err
			}
			if err := c.NeedProject("orgpolicy.policy.set"); err != nil {
				return nil, err
			}
			p.OrgPolicies[cons] = &sim.OrgPolicy{Constraint: cons, Enforce: enforce}
			c.Audit("orgpolicy.googleapis.com", "SetOrgPolicy", "projects/"+p.ID+"/policies/"+cons)
			return Obj{V: map[string]any{"constraint": "constraints/" + cons, "booleanPolicy": map[string]any{"enforced": enforce}}}, nil
		}
	}
	reg("resource-manager org-policies enable-enforce", orgPolicyToggle(true))
	reg("resource-manager org-policies disable-enforce", orgPolicyToggle(false))
	reg("resource-manager org-policies allow", func(c *Cmd) (any, error) {
		cons, err := c.Arg(0, "CONSTRAINT")
		if err != nil {
			return nil, err
		}
		cons = strings.TrimPrefix(cons, "constraints/")
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("orgpolicy.policy.set"); err != nil {
			return nil, err
		}
		p.OrgPolicies[cons] = &sim.OrgPolicy{Constraint: cons, AllowedValues: c.Args[1:]}
		return Obj{V: map[string]any{"constraint": "constraints/" + cons, "listPolicy": map[string]any{"allowedValues": c.Args[1:]}}}, nil
	})
	reg("resource-manager org-policies describe", func(c *Cmd) (any, error) {
		cons, err := c.Arg(0, "CONSTRAINT")
		if err != nil {
			return nil, err
		}
		cons = strings.TrimPrefix(cons, "constraints/")
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		op := p.OrgPolicies[cons]
		if op == nil {
			op = c.S.State.Org.OrgPolicies[cons]
		}
		if op == nil {
			return Obj{V: map[string]any{"constraint": "constraints/" + cons}}, nil
		}
		return Obj{V: op}, nil
	})
	reg("resource-manager org-policies list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.OrgPolicies) {
			rows = append(rows, p.OrgPolicies[k])
		}
		for _, k := range sim.SortedKeys(c.S.State.Org.OrgPolicies) {
			rows = append(rows, c.S.State.Org.OrgPolicies[k])
		}
		return Table{Cols: []Col{{"CONSTRAINT", "constraint"}, {"ENFORCE", "enforce"}, {"ALLOWED", "allowedValues"}}, Rows: rows}, nil
	})
	reg("org-policies describe", registry["resource-manager org-policies describe"])
	reg("org-policies list", registry["resource-manager org-policies list"])

	// ---- policy troubleshooter / asset inventory ---------------------------
	reg("policy-intelligence troubleshoot-policy iam", func(c *Cmd) (any, error) {
		res, err := c.Arg(0, "RESOURCE")
		if err != nil {
			return nil, err
		}
		principal := c.Str("principal-email", "")
		perm := c.Str("permission", "")
		if principal == "" || perm == "" {
			return nil, fmt.Errorf("--principal-email and --permission are required")
		}
		pm := "user:" + principal
		if strings.HasSuffix(principal, "gserviceaccount.com") {
			pm = "serviceAccount:" + principal
		}
		r := c.S.resourceFromFullName(res)
		ok, role := c.S.State.Explain(pm, perm, r)
		access := "NOT_GRANTED"
		if ok {
			access = "GRANTED"
		}
		return Obj{V: map[string]any{"access": access, "principal": pm, "permission": perm, "resource": res, "grantingRole": role,
			"explainedPolicies": len(r.Policies) + 3}}, nil
	})
	reg("asset search-all-resources", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("resourcemanager.projects.get"); err != nil {
			return nil, err
		}
		var rows []any
		add := func(t, name, loc string) {
			rows = append(rows, map[string]any{"assetType": t, "name": name, "location": loc})
		}
		for _, n := range sim.SortedKeys(p.Instances) {
			add("compute.googleapis.com/Instance", "//compute.googleapis.com/projects/"+p.ID+"/zones/"+p.Instances[n].Zone+"/instances/"+n, p.Instances[n].Zone)
		}
		for _, n := range sim.SortedKeys(p.Buckets) {
			add("storage.googleapis.com/Bucket", "//storage.googleapis.com/"+n, p.Buckets[n].Location)
		}
		for _, n := range sim.SortedKeys(p.Networks) {
			add("compute.googleapis.com/Network", "//compute.googleapis.com/projects/"+p.ID+"/global/networks/"+n, "global")
		}
		for _, n := range sim.SortedKeys(p.Firewalls) {
			add("compute.googleapis.com/Firewall", "//compute.googleapis.com/projects/"+p.ID+"/global/firewalls/"+n, "global")
		}
		for _, n := range sim.SortedKeys(p.SQLInstances) {
			add("sqladmin.googleapis.com/Instance", "//cloudsql.googleapis.com/projects/"+p.ID+"/instances/"+n, p.SQLInstances[n].Region)
		}
		for _, n := range sim.SortedKeys(p.RunServices) {
			add("run.googleapis.com/Service", "//run.googleapis.com/projects/"+p.ID+"/locations/"+p.RunServices[n].Region+"/services/"+n, p.RunServices[n].Region)
		}
		for _, n := range sim.SortedKeys(p.ServiceAccounts) {
			add("iam.googleapis.com/ServiceAccount", "//iam.googleapis.com/projects/"+p.ID+"/serviceAccounts/"+n, "global")
		}
		for _, n := range sim.SortedKeys(p.Clusters) {
			add("container.googleapis.com/Cluster", "//container.googleapis.com/projects/"+p.ID+"/locations/"+p.Clusters[n].Location+"/clusters/"+n, p.Clusters[n].Location)
		}
		if t := c.Str("asset-types", ""); t != "" {
			var kept []any
			for _, r := range rows {
				if strings.Contains(t, r.(map[string]any)["assetType"].(string)) {
					kept = append(kept, r)
				}
			}
			rows = kept
		}
		return Table{Cols: []Col{{"ASSET_TYPE", "assetType"}, {"NAME", "name"}, {"LOCATION", "location"}}, Rows: rows}, nil
	})

	// ---- SCC / billing ---------------------------------------------------
	reg("scc findings list", func(c *Cmd) (any, error) {
		pid := c.ProjectID()
		if len(c.Args) > 0 && strings.HasPrefix(c.Args[0], "projects/") {
			pid = strings.TrimPrefix(c.Args[0], "projects/")
		}
		if err := c.Need("securitycenter.findings.list", sim.ProjectResource(pid)); err != nil {
			return nil, err
		}
		var rows []any
		for _, f := range c.S.State.Findings(pid) {
			rows = append(rows, map[string]any{"finding": map[string]any{"category": f.Category, "severity": f.Severity, "resourceName": f.Resource, "state": f.State, "description": f.Detail}})
		}
		return Table{Cols: []Col{{"CATEGORY", "finding.category"}, {"SEVERITY", "finding.severity"}, {"RESOURCE_NAME", "finding.resourceName"}}, Rows: rows}, nil
	})
	reg("billing budgets create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		name := c.Str("display-name", "budget")
		amt := strings.TrimSuffix(strings.TrimSuffix(c.Str("budget-amount", "100"), "EUR"), "USD")
		f, _ := strconv.ParseFloat(amt, 64)
		var th []float64
		for _, t := range c.F["threshold-rule"] {
			if v := strings.TrimPrefix(t, "percent="); v != t {
				x, _ := strconv.ParseFloat(strings.Split(v, ",")[0], 64)
				th = append(th, x)
			}
		}
		p.Budgets[name] = &sim.Budget{Name: name, Amount: f, Thresholds: th, PubSubTopic: c.Str("notifications-rule-pubsub-topic", "")}
		return "Created budget [" + name + "].\nNOTE: budgets send alerts; they do not cap spending.\n", nil
	})
	reg("billing budgets list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Budgets) {
			rows = append(rows, p.Budgets[k])
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"AMOUNT", "amount"}, {"THRESHOLDS", "thresholds"}}, Rows: rows}, nil
	})
	reg("billing projects describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		return Obj{V: map[string]any{"billingAccountName": "billingAccounts/0X0X0X-TRAIN0-000000", "billingEnabled": p.BillingEnabled, "projectId": p.ID}}, nil
	})
	reg("billing estimate", func(c *Cmd) (any, error) {
		lines, total := c.S.State.CostEstimate(c.ProjectID())
		rows := []any{}
		for _, l := range lines {
			rows = append(rows, l)
		}
		rows = append(rows, map[string]any{"resource": "TOTAL", "sku": "", "monthlyEur": total})
		return Table{Cols: []Col{{"RESOURCE", "resource"}, {"SKU", "sku"}, {"MONTHLY_EUR", "monthlyEur"}}, Rows: rows}, nil
	})
}

// resourceFromFullName maps //service/... names to IAM resources.
func (s *Session) resourceFromFullName(full string) sim.Resource {
	st := s.State
	full = strings.TrimPrefix(full, "//")
	switch {
	case strings.HasPrefix(full, "storage.googleapis.com/"):
		rest := strings.TrimPrefix(full, "storage.googleapis.com/")
		rest = strings.TrimPrefix(rest, "projects/_/buckets/")
		bn, obj, _ := strings.Cut(rest, "/objects/")
		if b, _ := st.FindBucket(bn); b != nil {
			return st.BucketResource(b, obj)
		}
	case strings.HasPrefix(full, "cloudresourcemanager.googleapis.com/projects/"):
		return sim.ProjectResource(strings.TrimPrefix(full, "cloudresourcemanager.googleapis.com/projects/"))
	case strings.HasPrefix(full, "iam.googleapis.com/projects/"):
		parts := strings.Split(full, "/")
		if len(parts) >= 5 {
			if p := st.Projects[parts[2]]; p != nil {
				if sa := p.ServiceAccounts[parts[4]]; sa != nil {
					return sim.Resource{Project: p.ID, Type: "iam.googleapis.com/ServiceAccount", Name: "projects/" + p.ID + "/serviceAccounts/" + sa.Email, Service: "iam.googleapis.com", Policies: []*sim.Policy{&sa.IAM}}
				}
			}
		}
	case strings.HasPrefix(full, "secretmanager.googleapis.com/projects/"):
		parts := strings.Split(full, "/")
		if len(parts) >= 5 {
			if p := st.Projects[parts[2]]; p != nil {
				if sec := p.Secrets[parts[4]]; sec != nil {
					return sim.Resource{Project: p.ID, Type: "secretmanager.googleapis.com/Secret", Name: "projects/" + p.ID + "/secrets/" + parts[4], Service: "secretmanager.googleapis.com", Policies: []*sim.Policy{&sec.IAM}}
				}
			}
		}
	}
	parts := strings.Split(full, "/")
	for i, x := range parts {
		if x == "projects" && i+1 < len(parts) {
			return sim.ProjectResource(parts[i+1])
		}
	}
	return sim.ProjectResource(s.Project)
}
