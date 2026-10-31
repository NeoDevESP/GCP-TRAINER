package sim

import (
	"fmt"
	"strings"
)

// VPC Service Controls: a service perimeter around projects. Calls to a
// restricted service (BigQuery, Cloud Storage…) of a project inside the
// perimeter are rejected when they come from outside it, unless an ingress
// rule lets that identity and source in. A dry-run configuration only logs
// what it would block.

// PerimeterConfig is the enforced (status) or dry-run (spec) configuration.
type PerimeterConfig struct {
	Resources          []string        `json:"resources" yaml:"resources"` // projects/NUMBER or projects/ID
	RestrictedServices []string        `json:"restrictedServices" yaml:"restrictedServices"`
	IngressPolicies    []IngressPolicy `json:"ingressPolicies,omitempty" yaml:"ingressPolicies"`
}

// IngressPolicy mirrors the gcloud YAML format.
type IngressPolicy struct {
	IngressFrom struct {
		Identities   []string `json:"identities,omitempty" yaml:"identities"`
		IdentityType string   `json:"identityType,omitempty" yaml:"identityType"`
		Sources      []struct {
			Resource    string `json:"resource,omitempty" yaml:"resource"`
			AccessLevel string `json:"accessLevel,omitempty" yaml:"accessLevel"`
		} `json:"sources,omitempty" yaml:"sources"`
	} `json:"ingressFrom" yaml:"ingressFrom"`
	IngressTo struct {
		Operations []struct {
			ServiceName     string `json:"serviceName" yaml:"serviceName"`
			MethodSelectors []struct {
				Method string `json:"method,omitempty" yaml:"method"`
			} `json:"methodSelectors,omitempty" yaml:"methodSelectors"`
		} `json:"operations,omitempty" yaml:"operations"`
		Resources []string `json:"resources,omitempty" yaml:"resources"`
	} `json:"ingressTo" yaml:"ingressTo"`
}

// APICaller is who calls a Google API and from where.
type APICaller struct {
	Principal string // serviceAccount:… / user:…
	Project   string // source project ID (outside the perimeter) or ""
	Number    string // source project number
	IP        string // public source IP when not from a project
}

// ParseFrom reads a TrafficSpec.From value.
func ParseFrom(principal, from string) APICaller {
	c := APICaller{Principal: principal}
	switch {
	case strings.HasPrefix(from, "project:"):
		id := strings.TrimPrefix(from, "project:")
		c.Project, c.Number, _ = strings.Cut(id, "/")
	case strings.HasPrefix(from, "ip:"):
		c.IP = strings.TrimPrefix(from, "ip:")
	}
	return c
}

// VPCSCDecision is the outcome of a perimeter check.
type VPCSCDecision struct {
	Blocked   bool   // enforced configuration rejects the call
	DryRun    bool   // dry-run configuration would reject it
	Perimeter string // the perimeter that decides
	Reason    string // RESOURCES_NOT_IN_SAME_SERVICE_PERIMETER / NO_MATCHING_ACCESS_LEVEL
	UID       string
}

func (s *State) projectRef(pid string) []string {
	refs := []string{"projects/" + pid}
	if p := s.Projects[pid]; p != nil && p.Number != "" {
		refs = append(refs, "projects/"+p.Number)
	}
	return refs
}

func inList(list []string, vals ...string) bool {
	for _, a := range list {
		for _, v := range vals {
			if a == v || a == "*" {
				return true
			}
		}
	}
	return false
}

func (cfg *PerimeterConfig) violation(s *State, target, service string, c APICaller) string {
	if cfg == nil || !inList(cfg.Resources, s.projectRef(target)...) || !inList(cfg.RestrictedServices, service) {
		return ""
	}
	if c.Project != "" && inList(cfg.Resources, "projects/"+c.Project, "projects/"+c.Number) {
		return ""
	}
	for _, in := range cfg.IngressPolicies {
		f, to := in.IngressFrom, in.IngressTo
		idOK := strings.EqualFold(f.IdentityType, "ANY_IDENTITY") ||
			(strings.EqualFold(f.IdentityType, "ANY_SERVICE_ACCOUNT") && strings.HasPrefix(c.Principal, "serviceAccount:")) ||
			(strings.EqualFold(f.IdentityType, "ANY_USER_ACCOUNT") && strings.HasPrefix(c.Principal, "user:")) ||
			inList(f.Identities, c.Principal)
		srcOK := false
		for _, src := range f.Sources {
			if src.AccessLevel == "*" || (c.Project != "" && (src.Resource == "projects/"+c.Project || src.Resource == "projects/"+c.Number)) {
				srcOK = true
			}
		}
		opOK := false
		for _, op := range to.Operations {
			if op.ServiceName == service || op.ServiceName == "*" {
				opOK = true
			}
		}
		resOK := len(to.Resources) == 0 || inList(to.Resources, s.projectRef(target)...)
		if idOK && srcOK && opOK && resOK {
			return ""
		}
	}
	if c.Project == "" {
		return "NO_MATCHING_ACCESS_LEVEL"
	}
	return "RESOURCES_NOT_IN_SAME_SERVICE_PERIMETER"
}

// VPCSC evaluates every perimeter for a call to service on target project.
func (s *State) VPCSC(target, service string, c APICaller) VPCSCDecision {
	var d VPCSCDecision
	p := s.Projects[target]
	if p == nil {
		return d
	}
	for _, n := range SortedKeys(p.Perimeters) {
		per := p.Perimeters[n]
		if !per.DryRun || per.Status != nil {
			if r := per.Status.violation(s, target, service, c); r != "" && !d.Blocked {
				d.Blocked, d.Perimeter, d.Reason = true, n, r
			}
		}
		if per.Spec != nil {
			if r := per.Spec.violation(s, target, service, c); r != "" && !d.DryRun {
				d.DryRun = true
				if !d.Blocked {
					d.Perimeter, d.Reason = n, r
				}
			}
		}
	}
	if d.Blocked || d.DryRun {
		d.UID = s.ID(16)
	}
	return d
}

// LogVPCSC writes the policy audit log entry of a perimeter decision.
func (s *State) LogVPCSC(target, service, method string, c APICaller, d VPCSCDecision, dryRun bool) {
	src := c.IP
	if c.Project != "" {
		src = "projects/" + firstNonEmptyStr(c.Number, c.Project)
	}
	s.Log(target, LogEntry{
		Severity: "ERROR",
		LogName:  "cloudaudit.googleapis.com%2Fpolicy",
		Resource: LogResource{Type: "audited_resource", Labels: map[string]string{"service": service, "method": method, "project_id": target}},
		Text:     fmt.Sprintf("Request is prohibited by organization's policy. vpcServiceControlsUniqueIdentifier: %s", d.UID),
		Proto: map[string]string{
			"@type": "type.googleapis.com/google.cloud.audit.AuditLog", "serviceName": service, "methodName": method,
			"authenticationInfo.principalEmail":                strings.TrimPrefix(strings.TrimPrefix(c.Principal, "serviceAccount:"), "user:"),
			"requestMetadata.callerIp":                         firstNonEmptyStr(c.IP, "private"),
			"metadata.@type":                                   "type.googleapis.com/google.cloud.audit.VpcServiceControlAuditMetadata",
			"metadata.violationReason":                         d.Reason,
			"metadata.dryRun":                                  fmt.Sprint(dryRun),
			"metadata.securityPolicyInfo.servicePerimeterName": "accessPolicies/zentra/servicePerimeters/" + d.Perimeter,
			"metadata.ingressViolations.source":                src,
			"metadata.ingressViolations.targetResource":        "projects/" + firstNonEmptyStr(s.Projects[target].Number, target),
			"metadata.vpcServiceControlsUniqueIdentifier":      d.UID,
		},
	})
}

// runAPITraffic simulates calls to a Google API from outside the project
// (target api:SERVICE, path = method), e.g. a BI tool querying BigQuery.
func (s *State) runAPITraffic(pid string, t TrafficSpec) {
	service := strings.TrimPrefix(t.Target, "api:")
	method := strings.TrimPrefix(t.Path, "/")
	if method == "" {
		method = "*"
	}
	c := ParseFrom(t.Principal, t.From)
	d := s.VPCSC(pid, service, c)
	name := firstNonEmptyStr(t.Name, t.Target)
	s.Metric("traffic/"+name+"/requests", float64(t.RPS*60))
	if d.Blocked {
		s.Metric("traffic/"+name+"/error_rate", 1)
		s.LogVPCSC(pid, service, method, c, d, false)
		return
	}
	if d.DryRun {
		s.LogVPCSC(pid, service, method, c, d, true)
	}
	s.Metric("traffic/"+name+"/error_rate", 0)
}

// APICall answers whether a call would get through (for checks and tools).
func (s *State) APICall(target, service string, c APICaller) (bool, string) {
	d := s.VPCSC(target, service, c)
	if d.Blocked {
		return false, fmt.Sprintf("403 Request is prohibited by organization's policy (perimeter %s: %s)", d.Perimeter, d.Reason)
	}
	return true, "allowed"
}
