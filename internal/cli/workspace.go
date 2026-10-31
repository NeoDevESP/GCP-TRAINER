package cli

// Google Workspace / Cloud Identity administration: `gcloud identity groups`
// for Google groups and a subset of GAM (the admin CLI many Workspace teams
// use) for users, groups, organizational units and the admin audit report.
// Changes are written to the admin audit log so "who did it?" can be traced.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

var memberRoles = map[string]int{"MEMBER": 1, "MANAGER": 2, "OWNER": 3}

func (s *Session) dirAudit(method, resource string) {
	s.State.Audit(s.Project, s.Principal(), "admin.googleapis.com", method, resource)
}

func groupOf(s *Session, email string) (*sim.DirGroup, error) {
	g := s.State.Dir().Groups[strings.ToLower(email)]
	if g == nil {
		return nil, fmt.Errorf("NOT_FOUND: Group %s does not exist (Error 404: Resource Not Found: groupKey)", email)
	}
	return g, nil
}

// addMember adds a member with a role, applying the group's external-member setting.
func addMember(s *Session, g *sim.DirGroup, member, role string) error {
	member = strings.ToLower(member)
	d := s.State.Dir()
	if _, isGroup := d.Groups[member]; !isGroup && d.External(member) && !g.AllowExternal {
		return fmt.Errorf("FAILED_PRECONDITION: Member %s is outside the organization and group %s does not allow external members.", member, g.Email)
	}
	if u := d.Users[member]; u == nil && !d.External(member) && !strings.HasSuffix(member, ".gserviceaccount.com") {
		if _, isGroup := d.Groups[member]; !isGroup {
			return fmt.Errorf("NOT_FOUND: Member %s is not a user or group of the organization (Error 404: Resource Not Found: memberKey)", member)
		}
	}
	if _, ok := g.Members[member]; ok {
		return fmt.Errorf("ALREADY_EXISTS: Member %s already exists in group %s (Error 409: Member already exists)", member, g.Email)
	}
	if g.Members == nil {
		g.Members = map[string]string{}
	}
	g.Members[member] = role
	s.dirAudit("google.admin.AdminService.addGroupMember", "groups/"+g.Email+"/members/"+member)
	return nil
}

func removeMember(s *Session, g *sim.DirGroup, member string) error {
	member = strings.ToLower(member)
	if _, ok := g.Members[member]; !ok {
		return fmt.Errorf("NOT_FOUND: %s is not a member of %s (Error 404: Resource Not Found: memberKey)", member, g.Email)
	}
	delete(g.Members, member)
	s.dirAudit("google.admin.AdminService.removeGroupMember", "groups/"+g.Email+"/members/"+member)
	return nil
}

func groupView(s *Session, g *sim.DirGroup) map[string]any {
	return map[string]any{
		"groupKey": map[string]any{"id": g.Email}, "displayName": g.Name, "description": g.Description,
		"name": "groups/" + strings.SplitN(g.Email, "@", 2)[0], "labels": groupLabels(g),
		"settings": map[string]any{"allowExternalMembers": g.AllowExternal, "whoCanJoin": g.WhoCanJoin}, "directMembersCount": len(g.Members),
	}
}

func groupLabels(g *sim.DirGroup) map[string]string {
	l := map[string]string{"cloudidentity.googleapis.com/groups.discussion_forum": ""}
	if g.Security {
		l["cloudidentity.googleapis.com/groups.security"] = ""
	}
	return l
}

func init() {
	reg("identity groups create", func(c *Cmd) (any, error) {
		email, err := c.Arg(0, "EMAIL")
		if err != nil {
			return nil, err
		}
		email = strings.ToLower(email)
		d := c.S.State.Dir()
		if d.Domain != "" && d.External(email) {
			return nil, fmt.Errorf("INVALID_ARGUMENT: %s is not in a domain of this organization (%s)", email, d.Domain)
		}
		if d.Groups[email] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: Group %s already exists", email)
		}
		g := &sim.DirGroup{Email: email, Name: c.Str("display-name", email), Description: c.Str("description", ""), Members: map[string]string{}, WhoCanJoin: "INVITED_CAN_JOIN",
			Security: strings.Contains(c.Str("labels", ""), "groups.security")}
		if c.Has("with-initial-owner") {
			g.Members[strings.ToLower(strings.TrimPrefix(c.Principal(), "user:"))] = "OWNER"
		}
		d.Groups[email] = g
		c.S.dirAudit("google.apps.cloudidentity.groups.v1.GroupsService.CreateGroup", "groups/"+email)
		return map[string]any{"done": true, "response": groupView(c.S, g)}, nil
	})
	reg("identity groups describe", func(c *Cmd) (any, error) {
		email, err := c.Arg(0, "EMAIL")
		if err != nil {
			return nil, err
		}
		g, err := groupOf(c.S, email)
		if err != nil {
			return nil, err
		}
		return groupView(c.S, g), nil
	})
	reg("identity groups update", func(c *Cmd) (any, error) {
		email, err := c.Arg(0, "EMAIL")
		if err != nil {
			return nil, err
		}
		g, err := groupOf(c.S, email)
		if err != nil {
			return nil, err
		}
		if c.Has("display-name") {
			g.Name = c.Str("display-name", g.Name)
		}
		if c.Has("description") {
			g.Description = c.Str("description", "")
		}
		c.S.dirAudit("google.apps.cloudidentity.groups.v1.GroupsService.UpdateGroup", "groups/"+g.Email)
		return groupView(c.S, g), nil
	})
	reg("identity groups delete", func(c *Cmd) (any, error) {
		email, err := c.Arg(0, "EMAIL")
		if err != nil {
			return nil, err
		}
		if _, err := groupOf(c.S, email); err != nil {
			return nil, err
		}
		delete(c.S.State.Dir().Groups, strings.ToLower(email))
		c.S.dirAudit("google.apps.cloudidentity.groups.v1.GroupsService.DeleteGroup", "groups/"+email)
		return fmt.Sprintf("Deleted group [%s].\n", email), nil
	})
	reg("identity groups search", func(c *Cmd) (any, error) {
		d := c.S.State.Dir()
		var rows []any
		for _, k := range sim.SortedKeys(d.Groups) {
			g := d.Groups[k]
			if strings.Contains(c.Str("labels", ""), "groups.security") && !g.Security {
				continue
			}
			rows = append(rows, map[string]any{"email": g.Email, "name": g.Name, "members": len(g.Members), "external": g.AllowExternal})
		}
		return Table{Cols: []Col{{"EMAIL", "email"}, {"DISPLAY_NAME", "name"}, {"MEMBERS", "members"}, {"EXTERNAL_MEMBERS", "external"}}, Rows: rows}, nil
	})
	reg("identity groups memberships add", func(c *Cmd) (any, error) {
		g, err := groupOf(c.S, c.Str("group-email", ""))
		if err != nil {
			return nil, err
		}
		m := c.Str("member-email", "")
		if m == "" {
			return nil, fmt.Errorf("argument --member-email: Must be specified.")
		}
		role := "MEMBER"
		for _, r := range c.List("roles") {
			if memberRoles[strings.ToUpper(r)] > memberRoles[role] {
				role = strings.ToUpper(r)
			}
		}
		if err := addMember(c.S, g, m, role); err != nil {
			return nil, err
		}
		return map[string]any{"done": true, "response": map[string]any{"preferredMemberKey": map[string]any{"id": strings.ToLower(m)}, "roles": []any{map[string]any{"name": role}}}}, nil
	})
	reg("identity groups memberships delete", func(c *Cmd) (any, error) {
		g, err := groupOf(c.S, c.Str("group-email", ""))
		if err != nil {
			return nil, err
		}
		if err := removeMember(c.S, g, c.Str("member-email", "")); err != nil {
			return nil, err
		}
		return map[string]any{"done": true}, nil
	})
	reg("identity groups memberships list", func(c *Cmd) (any, error) {
		g, err := groupOf(c.S, c.Str("group-email", ""))
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, m := range sim.SortedKeys(g.Members) {
			kind := "USER"
			if c.S.State.Dir().Groups[m] != nil {
				kind = "GROUP"
			} else if strings.HasSuffix(m, ".gserviceaccount.com") {
				kind = "SERVICE_ACCOUNT"
			}
			rows = append(rows, map[string]any{"id": m, "roles": g.Members[m], "type": kind})
		}
		return Table{Cols: []Col{{"MEMBER", "id"}, {"ROLES", "roles"}, {"TYPE", "type"}}, Rows: rows}, nil
	})
	reg("identity groups memberships check-transitive-membership", func(c *Cmd) (any, error) {
		g, err := groupOf(c.S, c.Str("group-email", ""))
		if err != nil {
			return nil, err
		}
		m := strings.ToLower(c.Str("member-email", ""))
		for _, x := range c.S.State.GroupMembers(g.Email) {
			if strings.EqualFold(x, m) {
				return map[string]any{"hasMembership": true}, nil
			}
		}
		return map[string]any{"hasMembership": false}, nil
	})
	reg("identity groups memberships modify-membership-roles", func(c *Cmd) (any, error) {
		g, err := groupOf(c.S, c.Str("group-email", ""))
		if err != nil {
			return nil, err
		}
		m := strings.ToLower(c.Str("member-email", ""))
		if _, ok := g.Members[m]; !ok {
			return nil, fmt.Errorf("NOT_FOUND: %s is not a member of %s", m, g.Email)
		}
		role := "MEMBER"
		for _, r := range c.List("add-roles") {
			if memberRoles[strings.ToUpper(r)] > memberRoles[role] {
				role = strings.ToUpper(r)
			}
		}
		g.Members[m] = role
		c.S.dirAudit("google.admin.AdminService.updateGroupMember", "groups/"+g.Email+"/members/"+m)
		return map[string]any{"preferredMemberKey": map[string]any{"id": m}, "roles": role}, nil
	})
}

// ---------------------------------------------------------------- GAM

const gamUsage = `GAM (simulated subset)
  gam info user EMAIL | gam print users [query "orgUnitPath=/X"]
  gam create user EMAIL firstname F lastname L [org /OU] [title T]
  gam update user EMAIL [suspended on|off] [org /OU] [title T]
  gam delete user EMAIL
  gam user EMAIL signout | deprovision
  gam info group GROUP | gam print groups [member EMAIL] | gam print group-members group GROUP
  gam create group GROUP [name N] [description D]
  gam update group GROUP add member|manager|owner user EMAIL
  gam update group GROUP remove [member] user EMAIL
  gam update group GROUP allowexternalmembers true|false whocanjoin INVITED_CAN_JOIN|ALL_IN_DOMAIN_CAN_JOIN|ANYONE_CAN_JOIN
  gam print orgs | gam info org /PATH
  gam report admin [user EMAIL]
`

func (s *Session) gam(args []string) (string, error) {
	low := func(i int) string {
		if i < len(args) {
			return strings.ToLower(args[i])
		}
		return ""
	}
	d := s.State.Dir()
	user := func(email string) (*sim.DirUser, error) {
		u := d.Users[strings.ToLower(email)]
		if u == nil {
			return nil, fail(1, "ERROR: User: %s, Does not exist", email)
		}
		return u, nil
	}
	pairs := func(from int) map[string]string {
		m := map[string]string{}
		for i := from; i+1 < len(args); i += 2 {
			m[strings.ToLower(args[i])] = args[i+1]
		}
		return m
	}
	switch {
	case len(args) == 0 || low(0) == "help" || low(0) == "version":
		return gamUsage, nil

	case low(0) == "info" && low(1) == "user" && len(args) > 2:
		u, err := user(args[2])
		if err != nil {
			return "", err
		}
		var b strings.Builder
		fmt.Fprintf(&b, "User: %s\n  Full Name: %s\n  Is a Super Admin: %v\n  Admin Roles: %s\n  Account Suspended: %v\n  Archived: %v\n  2-step enrolled: %v\n  2-step enforced: %v\n  Org Unit Path: %s\n  Title: %s\n  Department: %s\n  Manager: %s\n  Creation Time: %s\n  Last login time: %s\n",
			u.Email, u.Name, u.Admin && containsStr(u.AdminRoles, "SUPER_ADMIN"), strings.Join(u.AdminRoles, ", "), u.Suspended, u.Archived, u.TwoSV, s.ou2sv(u.OU), u.OU, u.Title, u.Department, u.Manager, u.Created, orNever(u.LastLogin))
		if u.Forwarding != "" {
			fmt.Fprintf(&b, "  Forwarding to: %s\n", u.Forwarding)
		}
		gs := s.State.GroupsOf(u.Email)
		fmt.Fprintf(&b, "Groups: (%d)\n", len(gs))
		for _, g := range gs {
			role := d.Groups[g].Members[u.Email]
			if role == "" {
				role = "MEMBER (nested)"
			}
			fmt.Fprintf(&b, "  %s <%s> %s\n", d.Groups[g].Name, g, role)
		}
		fmt.Fprintf(&b, "Licenses: %s\n", strings.Join(u.Licenses, ", "))
		return b.String(), nil

	case low(0) == "print" && low(1) == "users":
		q := ""
		if low(2) == "query" && len(args) > 3 {
			q = args[3]
		}
		var b strings.Builder
		b.WriteString("primaryEmail,name.fullName,orgUnitPath,suspended,isEnrolledIn2Sv,isAdmin,lastLoginTime\n")
		for _, k := range sim.SortedKeys(d.Users) {
			u := d.Users[k]
			if q != "" && !userMatches(u, q) {
				continue
			}
			fmt.Fprintf(&b, "%s,%s,%s,%v,%v,%v,%s\n", u.Email, u.Name, u.OU, u.Suspended, u.TwoSV, u.Admin, orNever(u.LastLogin))
		}
		return b.String(), nil

	case low(0) == "create" && low(1) == "user" && len(args) > 2:
		email := strings.ToLower(args[2])
		if d.Domain == "" && strings.Contains(email, "@") {
			d.Domain = email[strings.LastIndex(email, "@")+1:]
		}
		if d.Users[email] != nil {
			return "", fail(1, "ERROR: User: %s, Duplicate, already exists", email)
		}
		if d.Domain != "" && d.External(email) {
			return "", fail(1, "ERROR: User: %s, Domain not verified as a secondary domain", email)
		}
		p := pairs(3)
		ou := p["org"]
		if ou == "" {
			ou = p["ou"]
		}
		if ou == "" {
			ou = "/"
		}
		if d.OUs[ou] == nil {
			return "", fail(1, "ERROR: Org Unit Path: %s, Does not exist", ou)
		}
		d.Users[email] = &sim.DirUser{Email: email, Name: strings.TrimSpace(p["firstname"] + " " + p["lastname"]), OU: ou, Title: p["title"], Department: p["department"], Manager: p["manager"], Created: s.State.Now()}
		s.dirAudit("google.admin.AdminService.createUser", "users/"+email)
		return fmt.Sprintf("User: %s, Created\n", email), nil

	case low(0) == "update" && low(1) == "user" && len(args) > 2:
		u, err := user(args[2])
		if err != nil {
			return "", err
		}
		p := pairs(3)
		var did []string
		if v, ok := p["suspended"]; ok {
			u.Suspended = v == "on" || v == "true"
			if u.Suspended {
				s.dirAudit("google.admin.AdminService.suspendUser", "users/"+u.Email)
			} else {
				s.dirAudit("google.admin.AdminService.unsuspendUser", "users/"+u.Email)
			}
			did = append(did, "suspended="+v)
		}
		for _, k := range []string{"org", "ou"} {
			if v, ok := p[k]; ok {
				if d.OUs[v] == nil {
					return "", fail(1, "ERROR: Org Unit Path: %s, Does not exist", v)
				}
				u.OU = v
				s.dirAudit("google.admin.AdminService.moveUserToOrgUnit", "users/"+u.Email)
				did = append(did, "org="+v)
			}
		}
		if v, ok := p["title"]; ok {
			u.Title = v
			did = append(did, "title")
		}
		if v, ok := p["manager"]; ok {
			u.Manager = v
			did = append(did, "manager")
		}
		if len(did) == 0 {
			return "", fail(2, "ERROR: nothing to update. Try: gam update user EMAIL suspended on")
		}
		return fmt.Sprintf("User: %s, Updated (%s)\n", u.Email, strings.Join(did, ", ")), nil

	case low(0) == "delete" && low(1) == "user" && len(args) > 2:
		u, err := user(args[2])
		if err != nil {
			return "", err
		}
		delete(d.Users, u.Email)
		for _, g := range d.Groups {
			delete(g.Members, u.Email)
		}
		s.dirAudit("google.admin.AdminService.deleteUser", "users/"+u.Email)
		return fmt.Sprintf("User: %s, Deleted (can be restored for 20 days from the Admin console)\n", u.Email), nil

	case low(0) == "user" && len(args) > 2 && (low(2) == "signout" || low(2) == "deprovision"):
		u, err := user(args[1])
		if err != nil {
			return "", err
		}
		u.SignedOut = true
		s.dirAudit("google.admin.AdminService.signOut", "users/"+u.Email)
		if low(2) == "deprovision" {
			s.dirAudit("google.admin.AdminService.revokeTokens", "users/"+u.Email)
			return fmt.Sprintf("User: %s, Signed out, OAuth tokens revoked, application-specific passwords and backup codes deleted\n", u.Email), nil
		}
		return fmt.Sprintf("User: %s, Signed out of all sessions\n", u.Email), nil

	case low(0) == "info" && low(1) == "group" && len(args) > 2:
		g, err := groupOf(s, args[2])
		if err != nil {
			return "", fail(1, "ERROR: Group: %s, Does not exist", args[2])
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Group: %s\n  name: %s\n  description: %s\n  allowExternalMembers: %v\n  whoCanJoin: %s\n  securityGroup: %v\nMembers: (%d)\n", g.Email, g.Name, g.Description, g.AllowExternal, g.WhoCanJoin, g.Security, len(g.Members))
		for _, m := range sim.SortedKeys(g.Members) {
			ext := ""
			if d.External(m) {
				ext = " (external)"
			}
			fmt.Fprintf(&b, "  %s: %s%s\n", strings.ToLower(g.Members[m]), m, ext)
		}
		return b.String(), nil

	case low(0) == "print" && low(1) == "groups":
		member := ""
		if low(2) == "member" && len(args) > 3 {
			member = strings.ToLower(args[3])
		}
		var b strings.Builder
		b.WriteString("email,name,directMembersCount,allowExternalMembers,whoCanJoin\n")
		var inGroups []string
		if member != "" {
			inGroups = s.State.GroupsOf(member)
		}
		for _, k := range sim.SortedKeys(d.Groups) {
			if member != "" && !containsStr(inGroups, k) {
				continue
			}
			g := d.Groups[k]
			fmt.Fprintf(&b, "%s,%s,%d,%v,%s\n", g.Email, g.Name, len(g.Members), g.AllowExternal, g.WhoCanJoin)
		}
		return b.String(), nil

	case low(0) == "print" && low(1) == "group-members":
		gn := ""
		if low(2) == "group" && len(args) > 3 {
			gn = args[3]
		}
		var b strings.Builder
		b.WriteString("group,role,email,type,status\n")
		for _, k := range sim.SortedKeys(d.Groups) {
			if gn != "" && !strings.EqualFold(k, gn) {
				continue
			}
			g := d.Groups[k]
			for _, m := range sim.SortedKeys(g.Members) {
				typ, st := "USER", "ACTIVE"
				if d.Groups[m] != nil {
					typ = "GROUP"
				} else if strings.HasSuffix(m, ".gserviceaccount.com") {
					typ = "SERVICE_ACCOUNT"
				} else if u := d.Users[m]; u != nil && u.Suspended {
					st = "SUSPENDED"
				} else if d.External(m) {
					typ = "EXTERNAL"
				}
				fmt.Fprintf(&b, "%s,%s,%s,%s,%s\n", g.Email, g.Members[m], m, typ, st)
			}
		}
		return b.String(), nil

	case low(0) == "create" && low(1) == "group" && len(args) > 2:
		email := strings.ToLower(args[2])
		if d.Groups[email] != nil {
			return "", fail(1, "ERROR: Group: %s, Duplicate, already exists", email)
		}
		p := pairs(3)
		d.Groups[email] = &sim.DirGroup{Email: email, Name: firstNonEmptyStr(p["name"], email), Description: p["description"], Members: map[string]string{}, WhoCanJoin: "INVITED_CAN_JOIN"}
		s.dirAudit("google.admin.AdminService.createGroup", "groups/"+email)
		return fmt.Sprintf("Group: %s, Created\n", email), nil

	case low(0) == "update" && low(1) == "group" && len(args) > 3:
		g, err := groupOf(s, args[2])
		if err != nil {
			return "", fail(1, "ERROR: Group: %s, Does not exist", args[2])
		}
		switch low(3) {
		case "add":
			// gam update group G add member|manager|owner [user] EMAIL
			role, i := "MEMBER", 4
			if r := strings.ToUpper(low(4)); memberRoles[r] > 0 {
				role, i = r, 5
			}
			if low(i) == "user" || low(i) == "group" {
				i++
			}
			if i >= len(args) {
				return "", fail(2, "ERROR: missing member email")
			}
			if err := addMember(s, g, args[i], role); err != nil {
				return "", fail(1, "ERROR: %s", err.Error())
			}
			return fmt.Sprintf("Group: %s, Member: %s, Added (%s)\n", g.Email, strings.ToLower(args[i]), strings.ToLower(role)), nil
		case "remove", "delete":
			i := 4
			for i < len(args) && (memberRoles[strings.ToUpper(low(i))] > 0 || low(i) == "user" || low(i) == "group") {
				i++
			}
			if i >= len(args) {
				return "", fail(2, "ERROR: missing member email")
			}
			if err := removeMember(s, g, args[i]); err != nil {
				return "", fail(1, "ERROR: %s", err.Error())
			}
			return fmt.Sprintf("Group: %s, Member: %s, Removed\n", g.Email, strings.ToLower(args[i])), nil
		default:
			p := pairs(3)
			var did []string
			if v, ok := p["allowexternalmembers"]; ok {
				g.AllowExternal = v == "true" || v == "on"
				did = append(did, "allowExternalMembers="+v)
			}
			if v, ok := p["whocanjoin"]; ok {
				g.WhoCanJoin = strings.ToUpper(v)
				did = append(did, "whoCanJoin="+g.WhoCanJoin)
			}
			if v, ok := p["name"]; ok {
				g.Name = v
				did = append(did, "name")
			}
			if v, ok := p["description"]; ok {
				g.Description = v
				did = append(did, "description")
			}
			if len(did) == 0 {
				return "", fail(2, "ERROR: unknown group update. Try: gam update group G allowexternalmembers false")
			}
			s.dirAudit("google.admin.AdminService.changeGroupSetting", "groups/"+g.Email)
			return fmt.Sprintf("Group: %s, Updated (%s)\n", g.Email, strings.Join(did, ", ")), nil
		}

	case low(0) == "create" && (low(1) == "org" || low(1) == "ou") && len(args) > 2:
		path := args[2]
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		if d.OUs[path] != nil {
			return "", fail(1, "ERROR: Org Unit Path: %s, Duplicate, already exists", path)
		}
		parent := path[:strings.LastIndex(path, "/")]
		if parent == "" {
			parent = "/"
		}
		if d.OUs[parent] == nil {
			return "", fail(1, "ERROR: Parent Org Unit Path: %s, Does not exist", parent)
		}
		p := pairs(3)
		d.OUs[path] = &sim.DirOU{Path: path, Description: p["description"], ExternalShare: d.OUs[parent].ExternalShare, Enforce2SV: d.OUs[parent].Enforce2SV}
		s.dirAudit("google.admin.AdminService.createOrgUnit", "orgunits"+path)
		return fmt.Sprintf("Org Unit: %s, Created\n", path), nil

	case low(0) == "print" && low(1) == "orgs":
		var b strings.Builder
		b.WriteString("orgUnitPath,description,enforce2Sv,driveExternalSharing\n")
		for _, k := range sim.SortedKeys(d.OUs) {
			o := d.OUs[k]
			fmt.Fprintf(&b, "%s,%s,%v,%s\n", o.Path, o.Description, o.Enforce2SV, o.ExternalShare)
		}
		return b.String(), nil

	case low(0) == "info" && (low(1) == "org" || low(1) == "ou") && len(args) > 2:
		o := d.OUs[args[2]]
		if o == nil {
			return "", fail(1, "ERROR: Org Unit Path: %s, Does not exist", args[2])
		}
		n := 0
		for _, u := range d.Users {
			if u.OU == o.Path {
				n++
			}
		}
		return fmt.Sprintf("Org Unit: %s\n  description: %s\n  2-step verification enforced: %v\n  Drive external sharing: %s\n  users: %d\n", o.Path, o.Description, o.Enforce2SV, o.ExternalShare, n), nil

	case low(0) == "report" && low(1) == "admin":
		who := ""
		if low(2) == "user" && len(args) > 3 {
			who = strings.ToLower(args[3])
		}
		var b strings.Builder
		b.WriteString("time,actor,event,target\n")
		for _, e := range s.State.Logs {
			if e.Proto["serviceName"] != "admin.googleapis.com" {
				continue
			}
			actor := e.Proto["authenticationInfo.principalEmail"]
			if who != "" && !strings.EqualFold(actor, who) && !strings.Contains(strings.ToLower(e.Proto["resourceName"]), who) {
				continue
			}
			ev := e.Proto["methodName"]
			ev = ev[strings.LastIndex(ev, ".")+1:]
			fmt.Fprintf(&b, "%s,%s,%s,%s\n", e.Timestamp, actor, ev, e.Proto["resourceName"])
		}
		return b.String(), nil
	}
	return "", fail(2, "ERROR: gam: command not supported by the simulator.\n%s", gamUsage)
}

func (s *Session) ou2sv(path string) bool {
	d := s.State.Dir()
	for p := path; ; {
		if o := d.OUs[p]; o != nil && o.Enforce2SV {
			return true
		}
		if p == "/" || p == "" {
			return false
		}
		i := strings.LastIndex(p, "/")
		if i <= 0 {
			p = "/"
		} else {
			p = p[:i]
		}
	}
}

func userMatches(u *sim.DirUser, q string) bool {
	for _, part := range strings.Fields(q) {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			k, v, _ = strings.Cut(part, ":")
		}
		v = strings.Trim(v, "'\"")
		switch strings.ToLower(k) {
		case "orgunitpath":
			if !strings.HasPrefix(u.OU, v) {
				return false
			}
		case "issuspended":
			if fmt.Sprint(u.Suspended) != strings.ToLower(v) {
				return false
			}
		case "isenrolledin2sv":
			if fmt.Sprint(u.TwoSV) != strings.ToLower(v) {
				return false
			}
		case "isadmin":
			if fmt.Sprint(u.Admin) != strings.ToLower(v) {
				return false
			}
		}
	}
	return true
}

func containsStr(a []string, s string) bool {
	for _, x := range a {
		if x == s {
			return true
		}
	}
	return false
}

func orNever(s string) string {
	if s == "" {
		return "Never"
	}
	return s
}

var _ = sort.Strings
