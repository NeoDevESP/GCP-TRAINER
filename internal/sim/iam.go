package sim

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// PredefinedRoles maps a predefined role to its permissions. Entries ending in
// ".*" are prefix wildcards. It is a curated subset of the real catalogue that
// is sufficient for practice scenarios; `gcloud iam roles describe` exposes it.
var PredefinedRoles = map[string][]string{
	"roles/browser": {"resourcemanager.projects.get", "resourcemanager.folders.get", "resourcemanager.folders.list", "resourcemanager.organizations.get"},

	"roles/storage.objectViewer":  {"storage.objects.get", "storage.objects.list", "resourcemanager.projects.get"},
	"roles/storage.objectCreator": {"storage.objects.create"},
	"roles/storage.objectUser":    {"storage.objects.create", "storage.objects.delete", "storage.objects.get", "storage.objects.list", "storage.objects.update", "storage.objects.restore"},
	"roles/storage.objectAdmin":   {"storage.objects.*", "storage.buckets.get", "storage.buckets.list"},
	"roles/storage.admin":         {"storage.*", "resourcemanager.projects.get"},
	"roles/storage.legacyBucketReader": {"storage.buckets.get", "storage.objects.list"},
	"roles/storage.bucketViewer":  {"storage.buckets.get", "storage.buckets.list"},

	"roles/compute.viewer":         {"compute.*.get", "compute.*.list", "compute.*.getIamPolicy"},
	"roles/compute.instanceAdmin.v1": {"compute.instances.*", "compute.disks.*", "compute.instanceGroups.*", "compute.instanceGroupManagers.*", "compute.instanceTemplates.*", "compute.autoscalers.*", "compute.snapshots.*", "compute.images.useReadOnly", "compute.subnetworks.use", "compute.subnetworks.useExternalIp", "compute.networks.get", "compute.networks.list", "compute.subnetworks.get", "compute.subnetworks.list", "compute.zones.list", "compute.machineTypes.list"},
	"roles/compute.networkAdmin":   {"compute.networks.*", "compute.subnetworks.*", "compute.routes.*", "compute.routers.*", "compute.addresses.*", "compute.globalAddresses.*", "compute.forwardingRules.*", "compute.globalForwardingRules.*", "compute.backendServices.*", "compute.healthChecks.*", "compute.urlMaps.*", "compute.targetHttpProxies.*", "compute.targetHttpsProxies.*", "compute.networkEndpointGroups.*", "compute.vpnGateways.*", "compute.vpnTunnels.*", "compute.instances.get", "compute.instances.list", "compute.firewalls.get", "compute.firewalls.list", "dns.*"},
	"roles/compute.securityAdmin":  {"compute.firewalls.*", "compute.securityPolicies.*", "compute.sslCertificates.*", "compute.networks.updatePolicy", "compute.networks.get", "compute.networks.list"},
	"roles/compute.loadBalancerAdmin": {"compute.backendServices.*", "compute.forwardingRules.*", "compute.globalForwardingRules.*", "compute.healthChecks.*", "compute.urlMaps.*", "compute.targetHttpProxies.*", "compute.targetHttpsProxies.*", "compute.networkEndpointGroups.*", "compute.instanceGroups.use", "compute.instanceGroups.get", "compute.instanceGroups.list", "compute.globalAddresses.*", "compute.addresses.*"},
	"roles/compute.osLogin":        {"compute.instances.osLogin", "compute.instances.get", "compute.instances.list"},
	"roles/compute.osAdminLogin":   {"compute.instances.osAdminLogin", "compute.instances.osLogin", "compute.instances.get", "compute.instances.list"},

	"roles/iam.serviceAccountUser":         {"iam.serviceAccounts.actAs", "iam.serviceAccounts.get", "iam.serviceAccounts.list"},
	"roles/iam.serviceAccountTokenCreator": {"iam.serviceAccounts.getAccessToken", "iam.serviceAccounts.getOpenIdToken", "iam.serviceAccounts.signBlob", "iam.serviceAccounts.signJwt", "iam.serviceAccounts.implicitDelegation"},
	"roles/iam.serviceAccountAdmin":        {"iam.serviceAccounts.create", "iam.serviceAccounts.delete", "iam.serviceAccounts.get", "iam.serviceAccounts.list", "iam.serviceAccounts.update", "iam.serviceAccounts.setIamPolicy", "iam.serviceAccounts.getIamPolicy", "iam.serviceAccounts.disable", "iam.serviceAccounts.enable"},
	"roles/iam.serviceAccountKeyAdmin":     {"iam.serviceAccountKeys.*"},
	"roles/iam.roleAdmin":                  {"iam.roles.*"},
	"roles/iap.tunnelResourceAccessor":     {"iap.tunnelInstances.accessViaIAP", "iap.tunnelDestGroups.accessViaIAP"},
	"roles/iap.httpsResourceAccessor":      {"iap.webServiceVersions.accessViaIAP"},
	"roles/iam.roleViewer":                 {"iam.roles.get", "iam.roles.list"},
	"roles/iam.securityReviewer":           {"*.getIamPolicy", "iam.roles.get", "iam.roles.list", "resourcemanager.projects.get"},
	"roles/iam.securityAdmin":              {"*.getIamPolicy", "*.setIamPolicy", "iam.roles.*", "iam.serviceAccountKeys.*"},
	"roles/resourcemanager.projectIamAdmin": {"resourcemanager.projects.getIamPolicy", "resourcemanager.projects.setIamPolicy"},
	"roles/orgpolicy.policyAdmin":          {"orgpolicy.*"},
	"roles/serviceusage.serviceUsageAdmin": {"serviceusage.*"},
	"roles/serviceusage.serviceUsageConsumer": {"serviceusage.services.use", "serviceusage.services.get", "serviceusage.services.list"},

	"roles/run.invoker":   {"run.routes.invoke"},
	"roles/run.viewer":    {"run.services.get", "run.services.list", "run.revisions.get", "run.revisions.list", "run.services.getIamPolicy"},
	"roles/run.developer": {"run.services.create", "run.services.update", "run.services.get", "run.services.list", "run.services.delete", "run.revisions.*", "run.routes.invoke", "run.services.getIamPolicy"},
	"roles/run.admin":     {"run.*"},

	"roles/cloudsql.client": {"cloudsql.instances.connect", "cloudsql.instances.get"},
	"roles/cloudsql.instanceUser": {"cloudsql.instances.login", "cloudsql.instances.get"},
	"roles/cloudsql.viewer": {"cloudsql.*.get", "cloudsql.*.list"},
	"roles/cloudsql.editor": {"cloudsql.instances.get", "cloudsql.instances.list", "cloudsql.instances.update", "cloudsql.instances.restart", "cloudsql.databases.*", "cloudsql.users.*", "cloudsql.backupRuns.*", "cloudsql.instances.connect"},
	"roles/cloudsql.admin":  {"cloudsql.*"},

	"roles/pubsub.publisher":  {"pubsub.topics.publish"},
	"roles/pubsub.subscriber": {"pubsub.subscriptions.consume", "pubsub.topics.attachSubscription", "pubsub.snapshots.seek"},
	"roles/pubsub.viewer":     {"pubsub.*.get", "pubsub.*.list"},
	"roles/pubsub.editor":     {"pubsub.topics.*", "pubsub.subscriptions.*", "pubsub.snapshots.*"},
	"roles/pubsub.admin":      {"pubsub.*"},

	"roles/secretmanager.secretAccessor":       {"secretmanager.versions.access"},
	"roles/secretmanager.viewer":               {"secretmanager.secrets.get", "secretmanager.secrets.list", "secretmanager.versions.get", "secretmanager.versions.list"},
	"roles/secretmanager.secretVersionAdder":   {"secretmanager.versions.add"},
	"roles/secretmanager.secretVersionManager": {"secretmanager.versions.add", "secretmanager.versions.enable", "secretmanager.versions.disable", "secretmanager.versions.destroy", "secretmanager.versions.get", "secretmanager.versions.list"},
	"roles/secretmanager.admin":                {"secretmanager.*"},

	"roles/cloudkms.cryptoKeyEncrypterDecrypter": {"cloudkms.cryptoKeyVersions.useToEncrypt", "cloudkms.cryptoKeyVersions.useToDecrypt"},
	"roles/cloudkms.cryptoKeyEncrypter":          {"cloudkms.cryptoKeyVersions.useToEncrypt"},
	"roles/cloudkms.cryptoKeyDecrypter":          {"cloudkms.cryptoKeyVersions.useToDecrypt"},
	"roles/cloudkms.admin":                       {"cloudkms.keyRings.*", "cloudkms.cryptoKeys.*", "cloudkms.cryptoKeyVersions.create", "cloudkms.cryptoKeyVersions.destroy", "cloudkms.cryptoKeyVersions.get", "cloudkms.cryptoKeyVersions.list", "cloudkms.cryptoKeyVersions.update"},
	"roles/cloudkms.viewer":                      {"cloudkms.*.get", "cloudkms.*.list"},

	"roles/container.viewer":        {"container.*.get", "container.*.list"},
	"roles/container.clusterViewer": {"container.clusters.get", "container.clusters.list"},
	"roles/container.developer":     {"container.clusters.get", "container.clusters.list", "container.deployments.*", "container.services.*", "container.pods.*", "container.configMaps.*", "container.secrets.*", "container.horizontalPodAutoscalers.*", "container.ingresses.*", "container.jobs.*", "container.serviceAccounts.*"},
	"roles/container.admin":         {"container.*"},
	"roles/container.clusterAdmin":  {"container.clusters.*", "container.operations.*"},
	"roles/container.defaultNodeServiceAccount": {"logging.logEntries.create", "monitoring.timeSeries.create", "monitoring.metricDescriptors.create", "autoscaling.sites.writeMetrics", "artifactregistry.repositories.downloadArtifacts"},

	"roles/artifactregistry.reader": {"artifactregistry.repositories.downloadArtifacts", "artifactregistry.repositories.get", "artifactregistry.repositories.list", "artifactregistry.dockerimages.*"},
	"roles/artifactregistry.writer": {"artifactregistry.repositories.downloadArtifacts", "artifactregistry.repositories.uploadArtifacts", "artifactregistry.repositories.get", "artifactregistry.repositories.list", "artifactregistry.dockerimages.*", "artifactregistry.tags.*"},
	"roles/artifactregistry.admin":  {"artifactregistry.*"},

	"roles/cloudbuild.builds.editor":  {"cloudbuild.builds.*"},
	"roles/cloudbuild.builds.viewer":  {"cloudbuild.builds.get", "cloudbuild.builds.list"},
	"roles/cloudbuild.builds.builder": {"cloudbuild.builds.*", "artifactregistry.repositories.uploadArtifacts", "artifactregistry.repositories.downloadArtifacts", "logging.logEntries.create", "storage.objects.*"},
	"roles/source.admin":              {"source.*"},
	"roles/source.writer":             {"source.repos.get", "source.repos.list", "source.repos.update"},
	"roles/clouddeploy.releaser":      {"clouddeploy.releases.*", "clouddeploy.rollouts.*", "clouddeploy.deliveryPipelines.get", "clouddeploy.deliveryPipelines.list"},
	"roles/clouddeploy.operator":      {"clouddeploy.*"},
	"roles/clouddeploy.admin":         {"clouddeploy.*"},
	"roles/clouddeploy.jobRunner":     {"clouddeploy.jobRuns.*", "logging.logEntries.create", "storage.objects.*"},

	"roles/bigquery.dataViewer": {"bigquery.tables.get", "bigquery.tables.list", "bigquery.tables.getData", "bigquery.datasets.get"},
	"roles/bigquery.dataEditor": {"bigquery.tables.*", "bigquery.datasets.get", "bigquery.models.*"},
	"roles/bigquery.dataOwner":  {"bigquery.tables.*", "bigquery.datasets.*", "bigquery.models.*"},
	"roles/bigquery.jobUser":    {"bigquery.jobs.create"},
	"roles/bigquery.user":       {"bigquery.jobs.create", "bigquery.datasets.create", "bigquery.datasets.get", "bigquery.tables.list"},
	"roles/bigquery.admin":      {"bigquery.*"},

	"roles/aiplatform.user":   {"aiplatform.*"},
	"roles/aiplatform.viewer": {"aiplatform.*.get", "aiplatform.*.list"},
	"roles/aiplatform.admin":  {"aiplatform.*"},

	"roles/logging.viewer":        {"logging.logEntries.list", "logging.logs.list", "logging.logMetrics.get", "logging.logMetrics.list", "logging.sinks.get", "logging.sinks.list"},
	"roles/logging.privateLogViewer": {"logging.logEntries.list", "logging.privateLogEntries.list", "logging.logs.list"},
	"roles/logging.logWriter":     {"logging.logEntries.create"},
	"roles/logging.configWriter":  {"logging.logMetrics.*", "logging.sinks.*", "logging.exclusions.*"},
	"roles/logging.admin":         {"logging.*"},
	"roles/monitoring.viewer":     {"monitoring.*.get", "monitoring.*.list"},
	"roles/monitoring.metricWriter": {"monitoring.timeSeries.create", "monitoring.metricDescriptors.create"},
	"roles/monitoring.alertPolicyEditor": {"monitoring.alertPolicies.*"},
	"roles/monitoring.editor":     {"monitoring.*"},
	"roles/monitoring.admin":      {"monitoring.*"},

	"roles/securitycenter.findingsViewer": {"securitycenter.findings.list", "securitycenter.findings.get"},
	"roles/securitycenter.admin":          {"securitycenter.*"},
	"roles/dns.admin":  {"dns.*"},
	"roles/dns.reader": {"dns.*.get", "dns.*.list"},
	"roles/accesscontextmanager.policyAdmin": {"accesscontextmanager.*"},
	"roles/billing.viewer": {"billing.*.get", "billing.*.list"},
	"roles/billing.costsManager": {"billing.budgets.*"},
}

// adminPermsExcludedFromEditor are permissions Editor does not include.
var editorExcluded = []string{"*.setIamPolicy", "iam.roles.*", "resourcemanager.projects.delete", "orgpolicy.*", "billing.*", "securitycenter.*", "accesscontextmanager.*"}

// RolePermissions resolves permissions for a predefined, basic or custom role.
func (s *State) RolePermissions(role, project string) ([]string, bool) {
	switch role {
	case "roles/owner":
		return []string{"*"}, true
	case "roles/editor":
		return []string{"*!editor"}, true
	case "roles/viewer":
		return []string{"*.get", "*.list", "resourcemanager.projects.get", "logging.logEntries.list"}, true
	}
	if p, ok := PredefinedRoles[role]; ok {
		return p, true
	}
	if strings.HasPrefix(role, "projects/") {
		parts := strings.Split(role, "/")
		if len(parts) == 4 {
			if pr := s.Projects[parts[1]]; pr != nil {
				if r := pr.CustomRoles[parts[3]]; r != nil && r.Stage != "DISABLED" && r.Stage != "DELETED" {
					return r.Permissions, true
				}
			}
		}
	}
	return nil, false
}

// RoleExists reports whether a role name can be granted.
func (s *State) RoleExists(role string) bool {
	_, ok := s.RolePermissions(role, "")
	return ok
}

func permMatch(pattern, perm string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == "*!editor" {
		for _, ex := range editorExcluded {
			if permMatch(ex, perm) {
				return false
			}
		}
		return true
	}
	if pattern == perm {
		return true
	}
	if strings.HasSuffix(pattern, ".*") && strings.HasPrefix(perm, strings.TrimSuffix(pattern, "*")) {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		return strings.HasSuffix(perm, pattern[1:])
	}
	// service.*.verb
	if strings.Contains(pattern, ".*.") {
		i := strings.Index(pattern, ".*.")
		return strings.HasPrefix(perm, pattern[:i+1]) && strings.HasSuffix(perm, pattern[i+2:])
	}
	return false
}

// RoleHasPermission reports whether a role grants a permission.
func (s *State) RoleHasPermission(role, perm string) bool {
	perms, ok := s.RolePermissions(role, "")
	if !ok {
		return false
	}
	for _, p := range perms {
		if permMatch(p, perm) {
			return true
		}
	}
	return false
}

// Resource identifies an IAM-protected resource for policy evaluation.
type Resource struct {
	Project string // owning project
	Type    string // e.g. storage.googleapis.com/Bucket
	Name    string // full resource name used in conditions
	Service string
	Policies []*Policy // resource-level policies (bucket, SA, secret...)
}

// ProjectResource is the project itself.
func ProjectResource(p string) Resource {
	return Resource{Project: p, Type: "cloudresourcemanager.googleapis.com/Project", Name: "projects/" + p, Service: "cloudresourcemanager.googleapis.com"}
}

// memberMatches evaluates a binding member against a principal.
func (s *State) memberMatches(member, principal string) bool {
	if member == principal || member == "allUsers" {
		return true
	}
	if member == "allAuthenticatedUsers" && principal != "anonymous" {
		return true
	}
	if strings.HasPrefix(member, "group:") {
		for _, m := range s.GroupMembers(strings.TrimPrefix(member, "group:")) {
			if m == principal {
				return true
			}
		}
	}
	if strings.HasPrefix(member, "domain:") {
		d := strings.TrimPrefix(member, "domain:")
		return strings.HasSuffix(principal, "@"+d)
	}
	if strings.HasPrefix(member, "principalSet://") && strings.Contains(member, "/namespace/") {
		// workload identity principal sets: match by namespace
		ns := member[strings.Index(member, "/namespace/")+len("/namespace/"):]
		return strings.Contains(principal, "["+ns+"/")
	}
	return false
}

// GroupMembers returns members of a Google group modelled in Extra.
func (s *State) GroupMembers(group string) []string {
	v := s.Extra["group:"+group]
	if v == "" {
		return nil
	}
	return strings.Split(v, ",")
}

// hierarchy returns project, folders and org policies for a project.
func (s *State) hierarchyPolicies(project string) []*Policy {
	var out []*Policy
	p := s.Projects[project]
	if p == nil {
		return out
	}
	out = append(out, &p.IAM)
	parent := p.Parent
	for i := 0; i < 10 && parent != ""; i++ {
		if strings.HasPrefix(parent, "folders/") {
			f := s.Folders[parent]
			if f == nil {
				break
			}
			out = append(out, &f.IAM)
			parent = f.Parent
		} else if strings.HasPrefix(parent, "organizations/") && s.Org != nil {
			out = append(out, &s.Org.IAM)
			break
		} else {
			break
		}
	}
	return out
}

// Allowed evaluates whether the principal holds perm on the resource, taking
// the resource hierarchy and IAM Conditions into account.
func (s *State) Allowed(principal, perm string, r Resource) bool {
	ok, _ := s.Explain(principal, perm, r)
	return ok
}

// Explain returns the decision and the binding that granted access (for the
// policy troubleshooter).
func (s *State) Explain(principal, perm string, r Resource) (bool, string) {
	pols := append([]*Policy{}, r.Policies...)
	if r.Project != "" {
		pols = append(pols, s.hierarchyPolicies(r.Project)...)
	}
	for _, pol := range pols {
		for _, b := range pol.Bindings {
			matched := false
			for _, m := range b.Members {
				if s.memberMatches(m, principal) {
					matched = true
					break
				}
			}
			if !matched || !s.RoleHasPermission(b.Role, perm) {
				continue
			}
			if b.Condition != nil {
				ok, err := EvalCondition(b.Condition.Expression, CondContext{Time: s.Clock, ResourceName: r.Name, ResourceType: r.Type, Service: r.Service})
				if err != nil || !ok {
					continue
				}
			}
			return true, b.Role
		}
	}
	return false, ""
}

// AddBinding adds member to role (merging into an existing binding).
func (p *Policy) AddBinding(role, member string, cond *Condition) {
	for i := range p.Bindings {
		b := &p.Bindings[i]
		if b.Role == role && condEq(b.Condition, cond) {
			for _, m := range b.Members {
				if m == member {
					return
				}
			}
			b.Members = append(b.Members, member)
			sort.Strings(b.Members)
			return
		}
	}
	p.Bindings = append(p.Bindings, Binding{Role: role, Members: []string{member}, Condition: cond})
	if cond != nil {
		p.Version = 3
	}
}

// RemoveBinding removes a member from a role; returns false if not present.
func (p *Policy) RemoveBinding(role, member string) bool {
	found := false
	var out []Binding
	for _, b := range p.Bindings {
		if b.Role == role {
			var ms []string
			for _, m := range b.Members {
				if m == member {
					found = true
					continue
				}
				ms = append(ms, m)
			}
			b.Members = ms
		}
		if len(b.Members) > 0 {
			out = append(out, b)
		}
	}
	p.Bindings = out
	return found
}

// HasMember reports whether member holds role (unconditionally or not).
func (p *Policy) HasMember(role, member string) bool {
	for _, b := range p.Bindings {
		if b.Role == role {
			for _, m := range b.Members {
				if m == member {
					return true
				}
			}
		}
	}
	return false
}

// RolesOf lists roles held by a member.
func (p *Policy) RolesOf(member string) []string {
	var out []string
	for _, b := range p.Bindings {
		for _, m := range b.Members {
			if m == member {
				out = append(out, b.Role)
			}
		}
	}
	return out
}

func condEq(a, b *Condition) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Expression == b.Expression && a.Title == b.Title
}

// ---------------------------------------------------------------------------
// IAM Conditions: a small CEL subset.

// CondContext carries attributes available to condition expressions.
type CondContext struct {
	Time         time.Time
	ResourceName string
	ResourceType string
	Service      string
}

// EvalCondition evaluates a CEL-subset expression.
func EvalCondition(expr string, ctx CondContext) (bool, error) {
	p := &celParser{toks: celLex(expr), ctx: ctx}
	v, err := p.or()
	if err != nil {
		return false, err
	}
	if p.pos < len(p.toks) {
		return false, fmt.Errorf("unexpected token %q", p.toks[p.pos])
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("condition does not evaluate to bool")
	}
	return b, nil
}

func celLex(s string) []string {
	var toks []string
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case unicode.IsSpace(rune(c)):
			i++
		case c == '"' || c == '\'':
			j := i + 1
			for j < len(s) && s[j] != c {
				j++
			}
			toks = append(toks, "\""+s[i+1:min(j, len(s))])
			i = j + 1
		case strings.HasPrefix(s[i:], "&&") || strings.HasPrefix(s[i:], "||") || strings.HasPrefix(s[i:], "==") || strings.HasPrefix(s[i:], "!=") || strings.HasPrefix(s[i:], "<=") || strings.HasPrefix(s[i:], ">="):
			toks = append(toks, s[i:i+2])
			i += 2
		case strings.ContainsRune("()<>!,", rune(c)):
			toks = append(toks, string(c))
			i++
		default:
			j := i
			for j < len(s) && (unicode.IsLetter(rune(s[j])) || unicode.IsDigit(rune(s[j])) || s[j] == '.' || s[j] == '_' || s[j] == '-' || s[j] == ':') {
				j++
			}
			if j == i {
				j = i + 1
			}
			toks = append(toks, s[i:j])
			i = j
		}
	}
	return toks
}

type celParser struct {
	toks []string
	pos  int
	ctx  CondContext
}

func (p *celParser) peek() string {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return ""
}
func (p *celParser) next() string { t := p.peek(); p.pos++; return t }

func (p *celParser) or() (any, error) {
	l, err := p.and()
	if err != nil {
		return nil, err
	}
	for p.peek() == "||" {
		p.next()
		r, err := p.and()
		if err != nil {
			return nil, err
		}
		l = truthy(l) || truthy(r)
	}
	return l, nil
}

func (p *celParser) and() (any, error) {
	l, err := p.unary()
	if err != nil {
		return nil, err
	}
	for p.peek() == "&&" {
		p.next()
		r, err := p.unary()
		if err != nil {
			return nil, err
		}
		l = truthy(l) && truthy(r)
	}
	return l, nil
}

func (p *celParser) unary() (any, error) {
	if p.peek() == "!" {
		p.next()
		v, err := p.unary()
		return !truthy(v), err
	}
	return p.cmp()
}

func (p *celParser) cmp() (any, error) {
	l, err := p.primary()
	if err != nil {
		return nil, err
	}
	switch op := p.peek(); op {
	case "==", "!=", "<", ">", "<=", ">=":
		p.next()
		r, err := p.primary()
		if err != nil {
			return nil, err
		}
		return compare(l, r, op)
	}
	return l, nil
}

func (p *celParser) primary() (any, error) {
	t := p.next()
	switch {
	case t == "(":
		v, err := p.or()
		if err != nil {
			return nil, err
		}
		if p.next() != ")" {
			return nil, fmt.Errorf("missing )")
		}
		return v, nil
	case strings.HasPrefix(t, "\""):
		return t[1:], nil
	case t == "true":
		return true, nil
	case t == "false":
		return false, nil
	case t == "timestamp" || t == "duration":
		if p.next() != "(" {
			return nil, fmt.Errorf("expected (")
		}
		arg := p.next()
		if p.next() != ")" {
			return nil, fmt.Errorf("expected )")
		}
		if t == "duration" {
			d, err := time.ParseDuration(strings.TrimPrefix(arg, "\""))
			return float64(d.Seconds()), err
		}
		tm, err := time.Parse(time.RFC3339, strings.TrimPrefix(arg, "\""))
		return tm, err
	}
	if n, err := strconv.ParseFloat(t, 64); err == nil {
		return n, nil
	}
	// identifiers, possibly with a method call
	var val any
	method := ""
	ident := t
	for _, m := range []string{".startsWith", ".endsWith", ".contains", ".getHours", ".getDayOfWeek", ".matches"} {
		if strings.HasSuffix(ident, m) {
			method = m[1:]
			ident = strings.TrimSuffix(ident, m)
		}
	}
	switch ident {
	case "request.time":
		val = p.ctx.Time
	case "resource.name":
		val = p.ctx.ResourceName
	case "resource.type":
		val = p.ctx.ResourceType
	case "resource.service":
		val = p.ctx.Service
	default:
		return nil, fmt.Errorf("unknown attribute %q", ident)
	}
	if method == "" {
		return val, nil
	}
	if p.next() != "(" {
		return nil, fmt.Errorf("expected ( after %s", method)
	}
	arg := ""
	if p.peek() != ")" {
		arg = strings.TrimPrefix(p.next(), "\"")
	}
	if p.next() != ")" {
		return nil, fmt.Errorf("expected )")
	}
	switch method {
	case "startsWith":
		return strings.HasPrefix(fmt.Sprint(val), arg), nil
	case "endsWith":
		return strings.HasSuffix(fmt.Sprint(val), arg), nil
	case "contains":
		return strings.Contains(fmt.Sprint(val), arg), nil
	case "matches":
		return strings.Contains(fmt.Sprint(val), strings.Trim(arg, "^$.*")), nil
	case "getHours":
		tm, _ := val.(time.Time)
		if loc, err := time.LoadLocation(arg); err == nil && arg != "" {
			tm = tm.In(loc)
		}
		return float64(tm.Hour()), nil
	case "getDayOfWeek":
		tm, _ := val.(time.Time)
		return float64(tm.Weekday()), nil
	}
	return nil, fmt.Errorf("unsupported method")
}

func truthy(v any) bool { b, _ := v.(bool); return b }

func compare(l, r any, op string) (any, error) {
	switch lv := l.(type) {
	case time.Time:
		rv, ok := r.(time.Time)
		if !ok {
			return nil, fmt.Errorf("type mismatch")
		}
		switch op {
		case "<":
			return lv.Before(rv), nil
		case ">":
			return lv.After(rv), nil
		case "<=":
			return !lv.After(rv), nil
		case ">=":
			return !lv.Before(rv), nil
		case "==":
			return lv.Equal(rv), nil
		case "!=":
			return !lv.Equal(rv), nil
		}
	case float64:
		rv, ok := r.(float64)
		if !ok {
			return nil, fmt.Errorf("type mismatch")
		}
		switch op {
		case "<":
			return lv < rv, nil
		case ">":
			return lv > rv, nil
		case "<=":
			return lv <= rv, nil
		case ">=":
			return lv >= rv, nil
		case "==":
			return lv == rv, nil
		case "!=":
			return lv != rv, nil
		}
	default:
		ls, rs := fmt.Sprint(l), fmt.Sprint(r)
		switch op {
		case "==":
			return ls == rs, nil
		case "!=":
			return ls != rs, nil
		}
	}
	return nil, fmt.Errorf("unsupported comparison")
}

// OrgPolicyEnforced checks a boolean constraint along the hierarchy.
func (s *State) OrgPolicyEnforced(project, constraint string) bool {
	if p := s.Projects[project]; p != nil {
		if op := p.OrgPolicies[constraint]; op != nil {
			return op.Enforce
		}
	}
	if s.Org != nil {
		if op := s.Org.OrgPolicies[constraint]; op != nil {
			return op.Enforce
		}
	}
	return false
}

// OrgPolicyAllows checks a list constraint (e.g. gcp.resourceLocations).
func (s *State) OrgPolicyAllows(project, constraint, value string) bool {
	var op *OrgPolicy
	if p := s.Projects[project]; p != nil {
		op = p.OrgPolicies[constraint]
	}
	if op == nil && s.Org != nil {
		op = s.Org.OrgPolicies[constraint]
	}
	if op == nil {
		return true
	}
	for _, d := range op.DeniedValues {
		if matchListValue(d, value) {
			return false
		}
	}
	if len(op.AllowedValues) == 0 {
		return true
	}
	for _, a := range op.AllowedValues {
		if matchListValue(a, value) {
			return true
		}
	}
	return false
}

func matchListValue(pattern, v string) bool {
	pattern = strings.TrimPrefix(pattern, "in:")
	switch pattern {
	case "eu-locations":
		return strings.HasPrefix(v, "europe-") || v == "EU" || v == "eu" || v == "EUR4"
	case "us-locations":
		return strings.HasPrefix(v, "us-") || v == "US" || v == "NAM4"
	}
	return pattern == v || strings.HasPrefix(v, pattern+"-")
}
