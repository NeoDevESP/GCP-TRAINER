package sim

import "strings"

// IAM deny policies: rules attached to a project that deny permissions to
// principals regardless of their roles, with exceptions (for example a
// break-glass group). Deny is evaluated before allow.

// DenyPolicy mirrors iam.googleapis.com/v2 policies of kind denypolicies.
type DenyPolicy struct {
	Name        string     `json:"name" yaml:"name"`
	DisplayName string     `json:"displayName,omitempty" yaml:"displayName"`
	Rules       []DenyRule `json:"rules" yaml:"rules"`
}

// DenyRule is one rule of a deny policy.
type DenyRule struct {
	Description string `json:"description,omitempty" yaml:"description"`
	DenyRule    struct {
		DeniedPrincipals    []string `json:"deniedPrincipals" yaml:"deniedPrincipals"`
		ExceptionPrincipals []string `json:"exceptionPrincipals,omitempty" yaml:"exceptionPrincipals"`
		DeniedPermissions   []string `json:"deniedPermissions" yaml:"deniedPermissions"`
	} `json:"denyRule" yaml:"denyRule"`
}

// v2 principal identifiers to policy member strings.
func denyMember(p string) string {
	switch {
	case p == "principalSet://goog/public:all":
		return "*"
	case strings.HasPrefix(p, "principal://goog/subject/"):
		id := strings.TrimPrefix(p, "principal://goog/subject/")
		if strings.HasSuffix(id, ".gserviceaccount.com") {
			return "serviceAccount:" + id
		}
		return "user:" + id
	case strings.HasPrefix(p, "principal://iam.googleapis.com/projects/-/serviceAccounts/"):
		return "serviceAccount:" + strings.TrimPrefix(p, "principal://iam.googleapis.com/projects/-/serviceAccounts/")
	case strings.HasPrefix(p, "principalSet://goog/group/"):
		return "group:" + strings.TrimPrefix(p, "principalSet://goog/group/")
	}
	return p
}

// v2 permission names (storage.googleapis.com/objects.delete) to v1.
func denyPerm(p string) string {
	if svc, rest, ok := strings.Cut(p, ".googleapis.com/"); ok {
		return svc + "." + rest
	}
	return p
}

// DeniedBy returns the deny policy that blocks perm for principal, if any.
func (s *State) DeniedBy(principal, perm string, r Resource) string {
	if r.Project == "" {
		return ""
	}
	p := s.Projects[r.Project]
	if p == nil {
		return ""
	}
	matches := func(list []string) bool {
		for _, m := range list {
			mm := denyMember(m)
			if mm == "*" || s.memberMatches(mm, principal) {
				return true
			}
		}
		return false
	}
	for _, n := range SortedKeys(p.DenyPolicies) {
		for _, rule := range p.DenyPolicies[n].Rules {
			dr := rule.DenyRule
			hit := false
			for _, dp := range dr.DeniedPermissions {
				if permMatch(denyPerm(dp), perm) {
					hit = true
				}
			}
			if hit && matches(dr.DeniedPrincipals) && !matches(dr.ExceptionPrincipals) {
				return n
			}
		}
	}
	return ""
}
