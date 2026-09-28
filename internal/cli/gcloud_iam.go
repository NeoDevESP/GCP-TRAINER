package cli

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
	"gopkg.in/yaml.v3"
)

var reSAName = regexp.MustCompile(`^[a-z]([-a-z0-9]{4,28}[a-z0-9])$`)

func (c *Cmd) findSA(ref string) (*sim.Project, *sim.ServiceAccount, error) {
	p, err := c.P()
	if err != nil {
		return nil, nil, err
	}
	email := ref
	if !strings.Contains(ref, "@") {
		email = ref + "@" + p.ID + ".iam.gserviceaccount.com"
	}
	for _, pp := range c.S.State.Projects {
		if sa := pp.ServiceAccounts[email]; sa != nil {
			return pp, sa, nil
		}
	}
	return nil, nil, fmt.Errorf("NOT_FOUND: Service account projects/%s/serviceAccounts/%s does not exist.", p.ID, email)
}

func saResource(p *sim.Project, sa *sim.ServiceAccount) sim.Resource {
	return sim.Resource{Project: p.ID, Type: "iam.googleapis.com/ServiceAccount", Name: "projects/" + p.ID + "/serviceAccounts/" + sa.Email, Service: "iam.googleapis.com", Policies: []*sim.Policy{&sa.IAM}}
}

func init() {
	reg("iam service-accounts create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		name, err := c.Arg(0, "NAME")
		if err != nil {
			return nil, err
		}
		if !reSAName.MatchString(name) {
			return nil, fmt.Errorf("argument NAME: Bad value [%s]: Service account name must be between 6 and 30 characters (inclusive), must begin with a lowercase letter, and consist of lowercase alphanumeric characters that can be separated by hyphens.", name)
		}
		if err := c.NeedProject("iam.serviceAccounts.create"); err != nil {
			return nil, err
		}
		email := name + "@" + p.ID + ".iam.gserviceaccount.com"
		if p.ServiceAccounts[email] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: Service account %s already exists within project projects/%s.", name, p.ID)
		}
		p.ServiceAccounts[email] = &sim.ServiceAccount{Email: email, DisplayName: c.Str("display-name", "")}
		c.Audit("iam.googleapis.com", "google.iam.admin.v1.CreateServiceAccount", "projects/"+p.ID+"/serviceAccounts/"+email)
		return "Created service account [" + name + "].\n", nil
	})
	reg("iam service-accounts list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("iam.serviceAccounts.list"); err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.ServiceAccounts) {
			sa := p.ServiceAccounts[k]
			rows = append(rows, map[string]any{"displayName": sa.DisplayName, "email": sa.Email, "disabled": sa.Disabled})
		}
		return Table{Cols: []Col{{"DISPLAY NAME", "displayName"}, {"EMAIL", "email"}, {"DISABLED", "disabled"}}, Rows: rows}, nil
	})
	reg("iam service-accounts describe", func(c *Cmd) (any, error) {
		ref, err := c.Arg(0, "SERVICE_ACCOUNT")
		if err != nil {
			return nil, err
		}
		p, sa, err := c.findSA(ref)
		if err != nil {
			return nil, err
		}
		return Obj{V: map[string]any{"email": sa.Email, "displayName": sa.DisplayName, "disabled": sa.Disabled, "projectId": p.ID, "name": "projects/" + p.ID + "/serviceAccounts/" + sa.Email}}, nil
	})
	reg("iam service-accounts delete", func(c *Cmd) (any, error) {
		ref, err := c.Arg(0, "SERVICE_ACCOUNT")
		if err != nil {
			return nil, err
		}
		p, sa, err := c.findSA(ref)
		if err != nil {
			return nil, err
		}
		if err := c.Need("iam.serviceAccounts.delete", saResource(p, sa)); err != nil {
			return nil, err
		}
		delete(p.ServiceAccounts, sa.Email)
		for i := range p.IAM.Bindings {
			for j, m := range p.IAM.Bindings[i].Members {
				if m == "serviceAccount:"+sa.Email {
					p.IAM.Bindings[i].Members[j] = "deleted:serviceAccount:" + sa.Email + "?uid=1"
				}
			}
		}
		c.Audit("iam.googleapis.com", "google.iam.admin.v1.DeleteServiceAccount", "projects/"+p.ID+"/serviceAccounts/"+sa.Email)
		return "deleted service account [" + sa.Email + "]\n", nil
	})
	toggle := func(disabled bool) handler {
		return func(c *Cmd) (any, error) {
			ref, err := c.Arg(0, "SERVICE_ACCOUNT")
			if err != nil {
				return nil, err
			}
			p, sa, err := c.findSA(ref)
			if err != nil {
				return nil, err
			}
			if err := c.Need("iam.serviceAccounts.disable", saResource(p, sa)); err != nil {
				return nil, err
			}
			sa.Disabled = disabled
			c.Audit("iam.googleapis.com", "google.iam.admin.v1.DisableServiceAccount", "projects/"+p.ID+"/serviceAccounts/"+sa.Email)
			if disabled {
				return "Disabled service account [" + sa.Email + "].\n", nil
			}
			return "Enabled service account [" + sa.Email + "].\n", nil
		}
	}
	reg("iam service-accounts disable", toggle(true))
	reg("iam service-accounts enable", toggle(false))
	reg("iam service-accounts get-iam-policy", func(c *Cmd) (any, error) {
		ref, err := c.Arg(0, "SERVICE_ACCOUNT")
		if err != nil {
			return nil, err
		}
		_, sa, err := c.findSA(ref)
		if err != nil {
			return nil, err
		}
		return policyView(&sa.IAM), nil
	})
	reg("iam service-accounts add-iam-policy-binding", func(c *Cmd) (any, error) {
		ref, err := c.Arg(0, "SERVICE_ACCOUNT")
		if err != nil {
			return nil, err
		}
		p, sa, err := c.findSA(ref)
		if err != nil {
			return nil, err
		}
		m, role, cond, err := c.bindingArgs()
		if err != nil {
			return nil, err
		}
		if err := c.Need("iam.serviceAccounts.setIamPolicy", saResource(p, sa)); err != nil {
			return nil, err
		}
		sa.IAM.AddBinding(role, m, cond)
		c.Audit("iam.googleapis.com", "google.iam.admin.v1.SetIAMPolicy", "projects/"+p.ID+"/serviceAccounts/"+sa.Email)
		r, _ := render(policyView(&sa.IAM), Flags{})
		return "Updated IAM policy for serviceAccount [" + sa.Email + "].\n" + r, nil
	})
	reg("iam service-accounts remove-iam-policy-binding", func(c *Cmd) (any, error) {
		ref, err := c.Arg(0, "SERVICE_ACCOUNT")
		if err != nil {
			return nil, err
		}
		p, sa, err := c.findSA(ref)
		if err != nil {
			return nil, err
		}
		if err := c.Need("iam.serviceAccounts.setIamPolicy", saResource(p, sa)); err != nil {
			return nil, err
		}
		role := c.Str("role", "")
		if !strings.HasPrefix(role, "roles/") {
			role = "roles/" + role
		}
		if !sa.IAM.RemoveBinding(role, c.Str("member", "")) {
			return nil, fmt.Errorf("Policy binding with the specified principal, role, and condition not found!")
		}
		return "Updated IAM policy for serviceAccount [" + sa.Email + "].\n", nil
	})
	reg("iam service-accounts keys create", func(c *Cmd) (any, error) {
		file, err := c.Arg(0, "OUTPUT-FILE")
		if err != nil {
			return nil, err
		}
		p, sa, err := c.findSA(c.Str("iam-account", ""))
		if err != nil {
			return nil, err
		}
		if c.S.State.OrgPolicyEnforced(p.ID, "iam.disableServiceAccountKeyCreation") || c.S.State.OrgPolicyEnforced(p.ID, "iam.managed.disableServiceAccountKeyCreation") {
			return nil, fmt.Errorf("FAILED_PRECONDITION: Key creation is not allowed on this service account (constraints/iam.disableServiceAccountKeyCreation). Prefer impersonation or Workload Identity.")
		}
		if err := c.Need("iam.serviceAccountKeys.create", saResource(p, sa)); err != nil {
			return nil, err
		}
		id := c.S.State.ID(40)
		sa.Keys = append(sa.Keys, sim.SAKey{ID: id, Created: c.S.State.Now()})
		c.S.Files[c.S.path(file)] = fmt.Sprintf("{\n  \"type\": \"service_account\",\n  \"project_id\": \"%s\",\n  \"private_key_id\": \"%s\",\n  \"private_key\": \"-----BEGIN PRIVATE KEY-----\\nSIMULATED\\n-----END PRIVATE KEY-----\\n\",\n  \"client_email\": \"%s\"\n}\n", p.ID, id, sa.Email)
		c.Audit("iam.googleapis.com", "google.iam.admin.v1.CreateServiceAccountKey", "projects/"+p.ID+"/serviceAccounts/"+sa.Email+"/keys/"+id)
		return fmt.Sprintf("created key [%s] of type [json] as [%s] for [%s]\n", id, file, sa.Email), nil
	})
	reg("iam service-accounts keys list", func(c *Cmd) (any, error) {
		_, sa, err := c.findSA(c.Str("iam-account", ""))
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sa.Keys {
			rows = append(rows, map[string]any{"name": k.ID, "validAfterTime": k.Created, "keyType": "USER_MANAGED"})
		}
		rows = append(rows, map[string]any{"name": "system-managed-" + sa.Email[:4], "validAfterTime": "2026-09-01T00:00:00Z", "keyType": "SYSTEM_MANAGED"})
		return Table{Cols: []Col{{"KEY_ID", "name"}, {"CREATED_AT", "validAfterTime"}, {"KEY_TYPE", "keyType"}}, Rows: rows}, nil
	})
	reg("iam service-accounts keys delete", func(c *Cmd) (any, error) {
		id, err := c.Arg(0, "KEY-ID")
		if err != nil {
			return nil, err
		}
		p, sa, err := c.findSA(c.Str("iam-account", ""))
		if err != nil {
			return nil, err
		}
		if err := c.Need("iam.serviceAccountKeys.delete", saResource(p, sa)); err != nil {
			return nil, err
		}
		var keep []sim.SAKey
		found := false
		for _, k := range sa.Keys {
			if k.ID == id {
				found = true
				continue
			}
			keep = append(keep, k)
		}
		if !found {
			return nil, fmt.Errorf("NOT_FOUND: key %s", id)
		}
		sa.Keys = keep
		c.Audit("iam.googleapis.com", "google.iam.admin.v1.DeleteServiceAccountKey", "projects/"+p.ID+"/serviceAccounts/"+sa.Email+"/keys/"+id)
		return "deleted key [" + id + "] for service account [" + sa.Email + "]\n", nil
	})

	// ---- roles ------------------------------------------------------------
	reg("iam roles describe", func(c *Cmd) (any, error) {
		role, err := c.Arg(0, "ROLE_ID")
		if err != nil {
			return nil, err
		}
		if c.Has("project") && !strings.HasPrefix(role, "projects/") && !strings.HasPrefix(role, "roles/") {
			role = "projects/" + c.ProjectID() + "/roles/" + role
		}
		if !strings.HasPrefix(role, "roles/") && !strings.HasPrefix(role, "projects/") {
			role = "roles/" + role
		}
		perms, ok := c.S.State.RolePermissions(role, c.ProjectID())
		if !ok {
			return nil, fmt.Errorf("NOT_FOUND: The role named %s was not found.", role)
		}
		stage := "GA"
		if strings.HasPrefix(role, "projects/") {
			parts := strings.Split(role, "/")
			if r := c.S.State.Projects[parts[1]].CustomRoles[parts[3]]; r != nil {
				stage = r.Stage
			}
		}
		if role == "roles/owner" {
			perms = []string{"(all permissions)"}
		} else if role == "roles/editor" {
			perms = []string{"(all permissions except IAM policy administration, role management, billing and org policy)"}
		}
		sorted := append([]string{}, perms...)
		sort.Strings(sorted)
		return Obj{V: map[string]any{"name": role, "includedPermissions": sorted, "stage": stage, "etag": "AA=="}}, nil
	})
	reg("iam roles list", func(c *Cmd) (any, error) {
		var rows []any
		if c.Has("project") {
			p, err := c.P()
			if err != nil {
				return nil, err
			}
			for _, k := range sim.SortedKeys(p.CustomRoles) {
				r := p.CustomRoles[k]
				rows = append(rows, map[string]any{"name": "projects/" + p.ID + "/roles/" + k, "title": r.Title, "stage": r.Stage})
			}
		} else {
			for _, k := range sim.SortedKeys(sim.PredefinedRoles) {
				rows = append(rows, map[string]any{"name": k, "title": k, "stage": "GA"})
			}
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"TITLE", "title"}, {"STAGE", "stage"}}, Rows: rows}, nil
	})
	roleCreateOrUpdate := func(update bool) handler {
		return func(c *Cmd) (any, error) {
			id, err := c.Arg(0, "ROLE_ID")
			if err != nil {
				return nil, err
			}
			p, err := c.P()
			if err != nil {
				return nil, err
			}
			if err := c.NeedProject("iam.roles.create"); err != nil {
				return nil, err
			}
			r := p.CustomRoles[id]
			if !update && r != nil {
				return nil, fmt.Errorf("ALREADY_EXISTS: A role named %s in projects/%s already exists.", id, p.ID)
			}
			if update && r == nil {
				return nil, fmt.Errorf("NOT_FOUND: role %s", id)
			}
			if r == nil {
				r = &sim.Role{Name: "projects/" + p.ID + "/roles/" + id, Stage: "GA"}
			}
			if f := c.Str("file", ""); f != "" {
				content, ok := c.S.Files[c.S.path(f)]
				if !ok {
					return nil, fmt.Errorf("Unable to read file [%s]", f)
				}
				var spec struct {
					Title       string   `yaml:"title"`
					Description string   `yaml:"description"`
					Stage       string   `yaml:"stage"`
					Permissions []string `yaml:"includedPermissions"`
				}
				if err := yaml.Unmarshal([]byte(content), &spec); err != nil {
					return nil, fmt.Errorf("Invalid role definition: %v", err)
				}
				r.Title, r.Description, r.Permissions = spec.Title, spec.Description, spec.Permissions
				if spec.Stage != "" {
					r.Stage = spec.Stage
				}
			}
			if v := c.Str("title", ""); v != "" {
				r.Title = v
			}
			if v := c.Str("description", ""); v != "" {
				r.Description = v
			}
			if v := c.Str("stage", ""); v != "" {
				r.Stage = v
			}
			if c.Has("permissions") {
				r.Permissions = c.List("permissions")
			}
			for _, a := range c.List("add-permissions") {
				r.Permissions = append(r.Permissions, a)
			}
			if rm := c.List("remove-permissions"); len(rm) > 0 {
				var keep []string
				for _, x := range r.Permissions {
					drop := false
					for _, y := range rm {
						if x == y {
							drop = true
						}
					}
					if !drop {
						keep = append(keep, x)
					}
				}
				r.Permissions = keep
			}
			for _, perm := range r.Permissions {
				if strings.Count(perm, ".") != 2 || strings.Contains(perm, "*") {
					return nil, fmt.Errorf("INVALID_ARGUMENT: Permission %s is not valid (wildcards are not allowed in custom roles).", perm)
				}
			}
			p.CustomRoles[id] = r
			c.Audit("iam.googleapis.com", "google.iam.admin.v1.CreateRole", r.Name)
			if update {
				return Obj{V: r}, nil
			}
			return "Created role [" + id + "].\n" + func() string { s, _ := render(Obj{V: r}, Flags{}); return s }(), nil
		}
	}
	reg("iam roles create", roleCreateOrUpdate(false))
	reg("iam roles update", roleCreateOrUpdate(true))
	reg("iam roles delete", func(c *Cmd) (any, error) {
		id, _ := c.Arg(0, "ROLE_ID")
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		r := p.CustomRoles[id]
		if r == nil {
			return nil, fmt.Errorf("NOT_FOUND: role %s", id)
		}
		r.Stage = "DELETED"
		return "deleted role [" + id + "]\n", nil
	})
	reg("iam list-grantable-roles", func(c *Cmd) (any, error) {
		var rows []any
		for _, k := range sim.SortedKeys(sim.PredefinedRoles) {
			rows = append(rows, map[string]any{"name": k, "title": k})
		}
		return Table{Cols: []Col{{"NAME", "name"}}, Rows: rows}, nil
	})
	reg("iam workload-identity-pools list", func(c *Cmd) (any, error) {
		return Table{Cols: []Col{{"NAME", "name"}}, Rows: []any{map[string]any{"name": c.ProjectID() + ".svc.id.goog"}}}, nil
	})
}
