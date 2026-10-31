package sim

// Google Workspace / Cloud Identity directory: the company's users,
// organizational units and Google groups. IAM bindings to group:EMAIL are
// resolved through it (with nested groups), and suspended users lose every
// permission, as a suspended account can no longer sign in.

import (
	"sort"
	"strings"
)

// Directory is the Workspace / Cloud Identity tenant of the organization.
type Directory struct {
	Domain string               `json:"domain"`
	Users  map[string]*DirUser  `json:"users"`
	Groups map[string]*DirGroup `json:"groups"`
	OUs    map[string]*DirOU    `json:"orgUnits"`
}

// DirUser is a Workspace user account.
type DirUser struct {
	Email       string   `json:"primaryEmail"`
	Name        string   `json:"name"`
	OU          string   `json:"orgUnitPath"`
	Title       string   `json:"title,omitempty"`
	Department  string   `json:"department,omitempty"`
	Manager     string   `json:"manager,omitempty"`
	Suspended   bool     `json:"suspended"`
	Archived    bool     `json:"archived,omitempty"`
	Admin       bool     `json:"isAdmin"`
	AdminRoles  []string `json:"adminRoles,omitempty"`
	TwoSV       bool     `json:"isEnrolledIn2Sv"`
	LastLogin   string   `json:"lastLoginTime,omitempty"`
	Created     string   `json:"creationTime,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	Licenses    []string `json:"licenses,omitempty"`
	SignedOut   bool     `json:"sessionsRevoked,omitempty"`
	Forwarding  string   `json:"forwardTo,omitempty"`
	ExternalTag string   `json:"-"`
}

// DirGroup is a Google group (Workspace or Cloud Identity).
type DirGroup struct {
	Email         string            `json:"email"`
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	Members       map[string]string `json:"members"` // email -> OWNER | MANAGER | MEMBER
	AllowExternal bool              `json:"allowExternalMembers"`
	WhoCanJoin    string            `json:"whoCanJoin"`
	Security      bool              `json:"securityLabel,omitempty"`
	AllMembers    []string          `json:"allMembers,omitempty"` // computed: transitive user/SA members
}

// DirOU is an organizational unit and the settings it enforces.
type DirOU struct {
	Path          string `json:"orgUnitPath"`
	Description   string `json:"description,omitempty"`
	Enforce2SV    bool   `json:"enforce2Sv"`
	ExternalShare string `json:"driveExternalSharing"` // ALLOWED | WHITELISTED | DISABLED
}

// Dir returns the directory, creating an empty one if needed.
func (s *State) Dir() *Directory {
	if s.Directory == nil {
		s.Directory = &Directory{Users: map[string]*DirUser{}, Groups: map[string]*DirGroup{}, OUs: map[string]*DirOU{"/": {Path: "/", ExternalShare: "ALLOWED"}}}
	}
	if s.Directory.OUs == nil {
		s.Directory.OUs = map[string]*DirOU{"/": {Path: "/", ExternalShare: "ALLOWED"}}
	}
	return s.Directory
}

// External reports whether an email is outside the directory's domain.
func (d *Directory) External(email string) bool {
	if d == nil || d.Domain == "" {
		return false
	}
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	dom := strings.ToLower(email[at+1:])
	return dom != d.Domain && !strings.HasSuffix(dom, "."+d.Domain) && !strings.HasSuffix(dom, ".gserviceaccount.com")
}

// Suspended reports whether a principal is a suspended directory user.
func (s *State) Suspended(principal string) bool {
	if s.Directory == nil || !strings.HasPrefix(principal, "user:") {
		return false
	}
	u := s.Directory.Users[strings.ToLower(strings.TrimPrefix(principal, "user:"))]
	return u != nil && (u.Suspended || u.Archived)
}

// directoryMembers returns the transitive principals of a group as
// "user:…", "serviceAccount:…" strings; ok is false if the group isn't in the directory.
func (s *State) directoryMembers(group string, seen map[string]bool) ([]string, bool) {
	if s.Directory == nil {
		return nil, false
	}
	g := s.Directory.Groups[strings.ToLower(group)]
	if g == nil {
		return nil, false
	}
	if seen[g.Email] {
		return nil, true
	}
	seen[g.Email] = true
	var out []string
	for _, m := range SortedKeys(g.Members) {
		if sub, ok := s.directoryMembers(m, seen); ok {
			out = append(out, sub...)
			continue
		}
		if strings.HasSuffix(m, ".gserviceaccount.com") {
			out = append(out, "serviceAccount:"+m)
		} else {
			out = append(out, "user:"+m)
		}
	}
	return out, true
}

// GroupsOf returns the groups (direct and nested) a member belongs to.
func (s *State) GroupsOf(email string) []string {
	if s.Directory == nil {
		return nil
	}
	email = strings.ToLower(email)
	var out []string
	for _, gn := range SortedKeys(s.Directory.Groups) {
		ms, _ := s.directoryMembers(gn, map[string]bool{})
		for _, m := range ms {
			if strings.EqualFold(m[strings.Index(m, ":")+1:], email) {
				out = append(out, gn)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// directoryView is the directory as graders see it, with transitive members.
func (s *State) directoryView() *Directory {
	if s.Directory == nil {
		return nil
	}
	d := *s.Directory
	d.Groups = map[string]*DirGroup{}
	for k, g := range s.Directory.Groups {
		cp := *g
		ms, _ := s.directoryMembers(k, map[string]bool{})
		cp.AllMembers = make([]string, 0, len(ms))
		for _, m := range ms {
			cp.AllMembers = append(cp.AllMembers, m[strings.Index(m, ":")+1:])
		}
		d.Groups[k] = &cp
	}
	return &d
}

// PermittedDomain reports whether a member belongs to the organization's
// customer for iam.allowedPolicyMemberDomains: the platform domain or the
// company's Workspace domain.
func (s *State) PermittedDomain(member string) bool {
	email := strings.ToLower(member[strings.Index(member, ":")+1:])
	if strings.HasSuffix(email, "@gcplab.dev") || strings.HasSuffix(email, ".gserviceaccount.com") {
		return true
	}
	return s.Directory != nil && s.Directory.Domain != "" && !s.Directory.External(email)
}

// ZoneStockout reports whether a zone has no capacity for a machine family
// right now (modelled per zone and family, e.g. "n2" or "a2").
func (s *State) ZoneStockout(zone, machineType string) bool {
	fam := machineType
	if i := strings.Index(fam, "-"); i > 0 {
		fam = fam[:i]
	}
	for _, f := range strings.Split(s.Extra["stockout:"+zone], ",") {
		if f == "*" || strings.TrimSpace(f) == fam {
			return true
		}
	}
	return false
}
