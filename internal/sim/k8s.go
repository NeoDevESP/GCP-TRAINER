package sim

import (
	"encoding/base64"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// K8sState is the Kubernetes API state of a GKE cluster (F0 model; the F1
// layer can replace it with a real kind cluster).
type K8sState struct {
	Namespaces map[string]*Namespace `json:"namespaces"`
}

type Namespace struct {
	Deployments     map[string]*Deployment       `json:"deployments"`
	Services        map[string]*K8sService       `json:"services"`
	HPAs            map[string]*HPA              `json:"hpas"`
	ConfigMaps      map[string]map[string]string `json:"configMaps"`
	Secrets         map[string]map[string]string `json:"secrets"`
	ServiceAccounts map[string]*KSA              `json:"serviceAccounts"`
	Ingresses       map[string]*Ingress          `json:"ingresses"`
	Pods            map[string]*Pod              `json:"pods"` // standalone pods (kubectl run)
	NetworkPolicies map[string]*NetworkPolicy    `json:"networkPolicies"`
	PVCs            map[string]*PVC              `json:"persistentVolumeClaims,omitempty"`
	Roles           map[string]*K8sRole          `json:"roles,omitempty"`
	RoleBindings    map[string]*RoleBinding      `json:"roleBindings,omitempty"`
}

// PVC is a PersistentVolumeClaim; it binds only with an existing StorageClass.
type PVC struct {
	Name         string `json:"name"`
	StorageClass string `json:"storageClassName"`
	Size         string `json:"size"`
	Phase        string `json:"status"` // Bound, Pending
	Reason       string `json:"reason,omitempty"`
}

// K8sRole is a namespaced RBAC role.
type K8sRole struct {
	Name  string       `json:"name"`
	Rules []PolicyRule `json:"rules"`
}

// PolicyRule grants verbs on resources.
type PolicyRule struct {
	Resources []string `json:"resources"`
	Verbs     []string `json:"verbs"`
}

// RoleBinding binds a role to Kubernetes service accounts.
type RoleBinding struct {
	Name     string   `json:"name"`
	Role     string   `json:"roleRef"`
	Subjects []string `json:"subjects"` // service account names in the namespace (ns:name for other namespaces)
}

// StorageClasses available on GKE.
var StorageClasses = map[string]bool{"standard": true, "standard-rwo": true, "premium-rwo": true}

func (ns *Namespace) ensure() {
	if ns.PVCs == nil {
		ns.PVCs = map[string]*PVC{}
	}
	if ns.Roles == nil {
		ns.Roles = map[string]*K8sRole{}
	}
	if ns.RoleBindings == nil {
		ns.RoleBindings = map[string]*RoleBinding{}
	}
}

// RBACAllows checks whether a Kubernetes service account may perform verb on resource.
func (ns *Namespace) RBACAllows(nsName, sa, verb, resource string) bool {
	ns.ensure()
	for _, rb := range ns.RoleBindings {
		bound := false
		for _, sub := range rb.Subjects {
			if sub == sa || sub == nsName+":"+sa {
				bound = true
			}
		}
		if !bound {
			continue
		}
		r := ns.Roles[rb.Role]
		if r == nil {
			continue
		}
		for _, rule := range r.Rules {
			if (contains(rule.Resources, resource) || contains(rule.Resources, "*")) && (contains(rule.Verbs, verb) || contains(rule.Verbs, "*")) {
				return true
			}
		}
	}
	return false
}

// ParseMemMi parses Kubernetes memory quantities into MiB.
func ParseMemMi(v string) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	mult := map[string]float64{"Ki": 1.0 / 1024, "Mi": 1, "Gi": 1024, "K": 1.0 / 1000 / 1.048576, "M": 1 / 1.048576, "G": 1000 / 1.048576}
	for _, suf := range []string{"Ki", "Mi", "Gi", "K", "M", "G"} {
		if strings.HasSuffix(v, suf) {
			n, _ := strconv.ParseFloat(strings.TrimSuffix(v, suf), 64)
			return int(n * mult[suf])
		}
	}
	n, _ := strconv.ParseFloat(v, 64)
	return int(n / 1024 / 1024)
}

type Deployment struct {
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
	Selector  map[string]string `json:"selector"`
	Replicas  int               `json:"replicas"`
	Template  PodTemplate       `json:"template"`
	Revision  int               `json:"revision"`
	History   []PodTemplate     `json:"history"`
	Available int               `json:"availableReplicas"`
}

type PodTemplate struct {
	Labels         map[string]string `json:"labels"`
	Containers     []Container       `json:"containers"`
	ServiceAccount string            `json:"serviceAccountName"`
	Claims         []string          `json:"persistentVolumeClaims,omitempty"`
}

type Container struct {
	Name      string            `json:"name"`
	Image     string            `json:"image"`
	Ports     []int             `json:"ports"`
	Env       map[string]string `json:"env"`
	EnvFrom   []string          `json:"envFrom"` // configmap:name or secret:name
	EnvRefs   map[string]string `json:"envRefs"` // ENV -> secret:name/key or configmap:name/key
	Requests  Resources         `json:"requests"`
	Limits    Resources         `json:"limits"`
	Readiness *Probe            `json:"readinessProbe,omitempty"`
	Liveness  *Probe            `json:"livenessProbe,omitempty"`
	Command   string            `json:"command,omitempty"`
}

type Resources struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

type Probe struct {
	Path         string `json:"path"`
	Port         int    `json:"port"`
	TCP          bool   `json:"tcp"`
	InitialDelay int    `json:"initialDelaySeconds"`
}

type K8sService struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Selector    map[string]string `json:"selector"`
	Ports       []SvcPort         `json:"ports"`
	ClusterIP   string            `json:"clusterIP"`
	ExternalIP  string            `json:"externalIP,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type SvcPort struct {
	Name       string `json:"name,omitempty"`
	Port       int    `json:"port"`
	TargetPort int    `json:"targetPort"`
}

type HPA struct {
	Name       string `json:"name"`
	Target     string `json:"scaleTargetRef"`
	Min        int    `json:"minReplicas"`
	Max        int    `json:"maxReplicas"`
	TargetCPU  int    `json:"targetCPUUtilizationPercentage"`
	CurrentCPU int    `json:"currentCPUUtilizationPercentage"`
	Unknown    bool   `json:"metricsUnknown"`
	ScaledUpTo int    `json:"maxObservedReplicas"`
	ScaledDown bool   `json:"scaledDownAfterPeak"`
}

type KSA struct {
	Name        string            `json:"name"`
	Annotations map[string]string `json:"annotations"`
}

type Ingress struct {
	Name  string        `json:"name"`
	Rules []IngressRule `json:"rules"`
	IP    string        `json:"ip"`
	Class string        `json:"class"`
}

type IngressRule struct {
	Host    string `json:"host"`
	Path    string `json:"path"`
	Service string `json:"service"`
	Port    int    `json:"port"`
}

type NetworkPolicy struct {
	Name     string            `json:"name"`
	Selector map[string]string `json:"podSelector"`
	From     map[string]string `json:"fromPodSelector"`
	Ports    []int             `json:"ports"`
}

type Pod struct {
	Name     string            `json:"name"`
	Owner    string            `json:"owner"`
	Labels   map[string]string `json:"labels"`
	Phase    string            `json:"status"`
	Ready    bool              `json:"ready"`
	Restarts int               `json:"restarts"`
	Reason   string            `json:"reason,omitempty"`
	IP       string            `json:"ip"`
	Node     string            `json:"node"`
	Spec     Container         `json:"container"`
	SA       string            `json:"serviceAccountName"`
	CPUm     int               `json:"cpuMillicores"`
	Command  string            `json:"command,omitempty"`
	Claims   []string          `json:"persistentVolumeClaims,omitempty"`
}

// NewK8s creates an empty API state with default namespaces.
func NewK8s() *K8sState {
	k := &K8sState{Namespaces: map[string]*Namespace{}}
	k.NS("default")
	k.NS("kube-system")
	return k
}

// NS returns (creating) a namespace.
func (k *K8sState) NS(name string) *Namespace {
	if name == "" {
		name = "default"
	}
	ns := k.Namespaces[name]
	if ns == nil {
		ns = &Namespace{Deployments: map[string]*Deployment{}, Services: map[string]*K8sService{}, HPAs: map[string]*HPA{},
			ConfigMaps: map[string]map[string]string{}, Secrets: map[string]map[string]string{},
			ServiceAccounts: map[string]*KSA{"default": {Name: "default", Annotations: map[string]string{}}}, Ingresses: map[string]*Ingress{},
			Pods: map[string]*Pod{}, NetworkPolicies: map[string]*NetworkPolicy{}}
		k.Namespaces[name] = ns
	}
	ns.ensure()
	return ns
}

// ParseCPU converts "250m" or "1" to millicores.
func ParseCPU(v string) int {
	if v == "" {
		return 0
	}
	if strings.HasSuffix(v, "m") {
		n, _ := strconv.Atoi(strings.TrimSuffix(v, "m"))
		return n
	}
	f, _ := strconv.ParseFloat(v, 64)
	return int(f * 1000)
}

func machineCPU(mt string) int {
	switch {
	case strings.HasSuffix(mt, "-micro"):
		return 250
	case strings.HasSuffix(mt, "-small"):
		return 500
	case strings.HasSuffix(mt, "-medium"):
		return 940
	}
	parts := strings.Split(mt, "-")
	if n, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
		return n*1000 - 70
	}
	return 1930
}

// ClusterCapacity returns allocatable millicores.
func (c *Cluster) Capacity() int {
	if c.Autopilot {
		return 1 << 30
	}
	total := 0
	for _, np := range c.NodePools {
		total += np.Count * machineCPU(np.MachineType)
	}
	return total
}

func selectorMatches(sel, labels map[string]string) bool {
	if len(sel) == 0 {
		return false
	}
	for k, v := range sel {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// ComputePods derives the pods of all deployments in the cluster.
func (s *State) ComputePods(project string, c *Cluster) map[string][]*Pod {
	out := map[string][]*Pod{}
	if c.K8s == nil {
		return out
	}
	used := 0
	nodeSA := s.clusterNodeSA(project, c)
	for _, nsName := range SortedKeys(c.K8s.Namespaces) {
		ns := c.K8s.Namespaces[nsName]
		for _, dn := range SortedKeys(ns.Deployments) {
			d := ns.Deployments[dn]
			if len(d.Template.Containers) == 0 {
				continue
			}
			ct := d.Template.Containers[0]
			hash := fmt.Sprintf("%x", len(ct.Image)*7919+d.Revision*104729)[0:5]
			avail := 0
			for i := 0; i < d.Replicas; i++ {
				pod := &Pod{Name: fmt.Sprintf("%s-%s%d-%s", d.Name, hash, d.Revision, string(rune('a'+i%26))+fmt.Sprint(i/26)), Owner: d.Name,
					Labels: d.Template.Labels, Spec: ct, SA: d.Template.ServiceAccount, Claims: d.Template.Claims, Node: fmt.Sprintf("gke-%s-default-pool-%d", c.Name, i%max(1, len(c.NodePools)+1)),
					IP: fmt.Sprintf("10.108.%d.%d", 1+i/200, 10+i%200)}
				req := ParseCPU(ct.Requests.CPU)
				if req == 0 {
					req = 100
				}
				if used+req > c.Capacity() {
					pod.Phase, pod.Reason = "Pending", "0/3 nodes are available: 3 Insufficient cpu."
					out[nsName] = append(out[nsName], pod)
					continue
				}
				used += req
				s.evalPod(project, c, ns, pod, nodeSA)
				if pod.Ready {
					avail++
				}
				out[nsName] = append(out[nsName], pod)
			}
			d.Available = avail
		}
		for _, pn := range SortedKeys(ns.Pods) {
			p := ns.Pods[pn]
			p.Phase = "Running"
			p.Ready = true
			out[nsName] = append(out[nsName], p)
		}
	}
	return out
}

func (s *State) clusterNodeSA(project string, c *Cluster) string {
	for _, np := range c.NodePools {
		if np.SA != "" {
			return np.SA
		}
	}
	return s.Projects[project].Number + "-compute@developer.gserviceaccount.com"
}

func (s *State) evalPod(project string, c *Cluster, ns *Namespace, pod *Pod, nodeSA string) {
	ct := pod.Spec
	if ok, why := s.ImagePullable(ct.Image, "serviceAccount:"+nodeSA); !ok {
		pod.Phase, pod.Reason = "Pending", "ImagePullBackOff: "+why
		return
	}
	b, _ := s.LookupImage(ct.Image)
	for _, ef := range ct.EnvFrom {
		kind, name, _ := strings.Cut(ef, ":")
		if (kind == "configmap" && ns.ConfigMaps[name] == nil) || (kind == "secret" && ns.Secrets[name] == nil) {
			pod.Phase, pod.Reason = "Pending", fmt.Sprintf("CreateContainerConfigError: %s %q not found", kind, name)
			return
		}
	}
	for env, ref := range ct.EnvRefs {
		kind, rest, _ := strings.Cut(ref, ":")
		name, key, _ := strings.Cut(rest, "/")
		var m map[string]string
		if kind == "secret" {
			m = ns.Secrets[name]
		} else {
			m = ns.ConfigMaps[name]
		}
		if _, ok := m[key]; !ok {
			pod.Phase, pod.Reason = "Pending", fmt.Sprintf("CreateContainerConfigError: couldn't find key %s in %s %s (env %s)", key, kind, name, env)
			return
		}
	}
	ns.ensure()
	for _, claim := range pod.Claims {
		pvc := ns.PVCs[claim]
		if pvc == nil {
			pod.Phase, pod.Reason = "Pending", fmt.Sprintf("FailedScheduling: persistentvolumeclaim %q not found", claim)
			return
		}
		if pvc.Phase != "Bound" {
			pod.Phase, pod.Reason = "Pending", fmt.Sprintf("FailedScheduling: 0/3 nodes are available: pod has unbound immediate PersistentVolumeClaims (%s: %s)", claim, pvc.Reason)
			return
		}
	}
	if b != nil && b.Mode == "crash" {
		pod.Phase, pod.Reason, pod.Restarts = "Running", "CrashLoopBackOff", 5
		return
	}
	if b != nil && b.MemMB > 0 {
		if lim := ParseMemMi(ct.Limits.Memory); lim > 0 && lim < b.MemMB {
			pod.Phase, pod.Reason, pod.Restarts = "Running", fmt.Sprintf("CrashLoopBackOff (last state: OOMKilled, exit code 137 — memory limit %s, working set ~%dMi)", ct.Limits.Memory, b.MemMB), 7
			return
		}
	}
	pod.Phase = "Running"
	listen := 8080
	if b != nil {
		listen = b.Port
	}
	probeOK := func(p *Probe) bool {
		if p == nil {
			return true
		}
		if p.Port != listen {
			return false
		}
		if p.TCP || b == nil {
			return true
		}
		return p.Path == b.Health || pathMatch(b.Paths, p.Path) && p.Path != ""
	}
	if !probeOK(ct.Liveness) {
		pod.Reason, pod.Restarts = "CrashLoopBackOff (liveness probe failed: connection refused)", 4
		return
	}
	if !probeOK(ct.Readiness) {
		pod.Reason = fmt.Sprintf("Readiness probe failed: HTTP probe failed with statuscode: 404 (or connection refused on port %d)", ct.Readiness.Port)
		return
	}
	pod.Ready = true
}

// PodWorkload builds the workload view of a pod including Workload Identity.
func (s *State) PodWorkload(project string, c *Cluster, nsName string, pod *Pod) *Workload {
	ns := c.K8s.NS(nsName)
	env := map[string]string{}
	for _, ef := range pod.Spec.EnvFrom {
		kind, name, _ := strings.Cut(ef, ":")
		src := ns.ConfigMaps[name]
		if kind == "secret" {
			src = ns.Secrets[name]
		}
		for k, v := range src {
			env[k] = v
		}
	}
	for k, v := range pod.Spec.Env {
		env[k] = v
	}
	for env2, ref := range pod.Spec.EnvRefs {
		kind, rest, _ := strings.Cut(ref, ":")
		name, key, _ := strings.Cut(rest, "/")
		if kind == "secret" {
			env[env2] = ns.Secrets[name][key]
		} else {
			env[env2] = ns.ConfigMaps[name][key]
		}
	}
	b, _ := s.LookupImage(pod.Spec.Image)
	principal, scopes := s.podPrincipal(project, c, nsName, pod.SA)
	tags := []string{"gke-" + c.Name + "-node"}
	return &Workload{Kind: "pod", Project: project, Name: pod.Name, Env: env, Behavior: b, Principal: principal, Scopes: scopes, K8sNS: ns, K8sNSName: nsName, K8sSA: firstNonEmptyS(pod.SA, "default"),
		Endpoint: Endpoint{Kind: "pod", Project: project, Network: c.Network, Subnet: c.Subnet, Region: RegionOf(c.Location), Name: pod.Name, IP: pod.IP, Tags: tags, SA: s.clusterNodeSA(project, c)}}
}

func (s *State) podPrincipal(project string, c *Cluster, ns, ksaName string) (string, []string) {
	if ksaName == "" {
		ksaName = "default"
	}
	if c.WorkloadPool == "" && !c.Autopilot {
		return "serviceAccount:" + s.clusterNodeSA(project, c), []string{"default"}
	}
	pool := c.WorkloadPool
	if pool == "" {
		pool = project + ".svc.id.goog"
	}
	ksaMember := fmt.Sprintf("serviceAccount:%s[%s/%s]", pool, ns, ksaName)
	ksa := c.K8s.NS(ns).ServiceAccounts[ksaName]
	if ksa != nil {
		if gsa := ksa.Annotations["iam.gke.io/gcp-service-account"]; gsa != "" {
			for _, p := range s.Projects {
				if sa := p.ServiceAccounts[gsa]; sa != nil {
					if sa.IAM.HasMember("roles/iam.workloadIdentityUser", ksaMember) {
						return "serviceAccount:" + gsa, nil
					}
				}
			}
		}
	}
	return fmt.Sprintf("principal://iam.googleapis.com/projects/%s/locations/global/workloadIdentityPools/%s/subject/ns/%s/sa/%s", s.Projects[project].Number, pool, ns, ksaName), nil
}

// k8sResolveCall handles in-cluster service DNS from a pod.
func (s *State) k8sResolveCall(from Endpoint, host string, port int, path string) (HTTPResponse, bool) {
	p := s.Projects[from.Project]
	if p == nil {
		return HTTPResponse{}, false
	}
	for _, cn := range SortedKeys(p.Clusters) {
		c := p.Clusters[cn]
		if c.K8s == nil || c.Network != from.Network {
			continue
		}
		parts := strings.Split(host, ".")
		ns := "default"
		if len(parts) > 1 && parts[1] != "" && c.K8s.Namespaces[parts[1]] != nil {
			ns = parts[1]
		}
		svc := c.K8s.NS(ns).Services[parts[0]]
		if svc == nil {
			continue
		}
		return s.k8sServiceCall(from.Project, c, ns, svc, port, path, from), true
	}
	return HTTPResponse{}, false
}

func (s *State) k8sExternalCall(project, ip string, port int, path string, req HTTPRequest) (HTTPResponse, bool) {
	p := s.Projects[project]
	for _, cn := range SortedKeys(p.Clusters) {
		c := p.Clusters[cn]
		if c.K8s == nil {
			continue
		}
		for _, nsName := range SortedKeys(c.K8s.Namespaces) {
			ns := c.K8s.Namespaces[nsName]
			for _, sn := range SortedKeys(ns.Services) {
				svc := ns.Services[sn]
				if svc.ExternalIP == ip && svc.Type == "LoadBalancer" {
					return s.k8sServiceCall(project, c, nsName, svc, port, path, req.From), true
				}
			}
			for _, in := range SortedKeys(ns.Ingresses) {
				ing := ns.Ingresses[in]
				if ing.IP != ip {
					continue
				}
				for _, r := range ing.Rules {
					if r.Path == "" || r.Path == "/" || r.Path == "/*" || strings.HasPrefix(path, strings.TrimSuffix(r.Path, "*")) {
						if svc := ns.Services[r.Service]; svc != nil {
							return s.k8sServiceCall(project, c, nsName, svc, r.Port, path, req.From), true
						}
					}
				}
				return HTTPResponse{Status: 404, Body: "default backend - 404"}, true
			}
		}
	}
	return HTTPResponse{}, false
}

func (s *State) k8sServiceCall(project string, c *Cluster, nsName string, svc *K8sService, port int, path string, from Endpoint) HTTPResponse {
	res := HTTPResponse{Target: "k8s:" + svc.Name}
	var sp *SvcPort
	for i := range svc.Ports {
		if svc.Ports[i].Port == port || (port == 80 && len(svc.Ports) == 1) {
			sp = &svc.Ports[i]
		}
	}
	if sp == nil {
		res.Error = fmt.Sprintf("Failed to connect to %s port %d: Connection refused", svc.Name, port)
		return res
	}
	pods := s.ComputePods(project, c)[nsName]
	var ready []*Pod
	for _, pd := range pods {
		if pd.Ready && selectorMatches(svc.Selector, pd.Labels) {
			ready = append(ready, pd)
		}
	}
	if len(ready) == 0 {
		res.Status, res.Body = 503, "upstream connect error or disconnect/reset before headers. reset reason: connection failure (service has no ready endpoints)"
		return res
	}
	sort.Slice(ready, func(i, j int) bool { return ready[i].Name < ready[j].Name })
	pod := ready[s.Tick%len(ready)]
	// NetworkPolicy enforcement (ingress)
	ns := c.K8s.NS(nsName)
	for _, npn := range SortedKeys(ns.NetworkPolicies) {
		np := ns.NetworkPolicies[npn]
		if selectorMatches(np.Selector, pod.Labels) {
			allowed := false
			if from.Kind == "pod" {
				for _, src := range pods {
					if src.Name == from.Name && selectorMatches(np.From, src.Labels) {
						allowed = true
					}
				}
			}
			if !allowed {
				res.Error = "Connection timed out (blocked by NetworkPolicy " + np.Name + ")"
				return res
			}
		}
	}
	w := s.PodWorkload(project, c, nsName, pod)
	listen := 8080
	if w.Behavior != nil {
		listen = w.Behavior.Port
	}
	if sp.TargetPort != listen {
		res.Status, res.Body = 503, fmt.Sprintf("upstream connect error: connection refused (targetPort %d, container listens on %d)", sp.TargetPort, listen)
		return res
	}
	st, msg := s.Call(w, path)
	res.Status, res.Body = st, msg
	if st == 200 {
		res.Body = fmt.Sprintf("Hello from pod %s", pod.Name)
	}
	return res
}

// ---------------------------------------------------------------------------
// Manifest parsing (kubectl apply -f)

// ApplyManifest parses one or more YAML documents into the cluster state.
func (s *State) ApplyManifest(c *Cluster, defaultNS, doc string) ([]string, error) {
	var out []string
	dec := yaml.NewDecoder(strings.NewReader(doc))
	for {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			if err.Error() == "EOF" {
				break
			}
			return out, fmt.Errorf("error: error parsing manifest: %v", err)
		}
		if m == nil {
			continue
		}
		line, err := s.applyObject(c, defaultNS, m)
		if err != nil {
			return out, err
		}
		out = append(out, line)
	}
	return out, nil
}

func str(m map[string]any, path ...string) string {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = mm[p]
	}
	if cur == nil {
		return ""
	}
	return fmt.Sprint(cur)
}

func sub(m map[string]any, path ...string) map[string]any {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[p]
	}
	mm, _ := cur.(map[string]any)
	return mm
}

func list(m map[string]any, path ...string) []any {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[p]
	}
	l, _ := cur.([]any)
	return l
}

func strMap(m map[string]any) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = fmt.Sprint(v)
	}
	return out
}

func atoi(v string) int { n, _ := strconv.Atoi(v); return n }

func parseProbe(m map[string]any) *Probe {
	if m == nil {
		return nil
	}
	p := &Probe{InitialDelay: atoi(str(m, "initialDelaySeconds"))}
	if hg := sub(m, "httpGet"); hg != nil {
		p.Path = str(hg, "path")
		p.Port = atoi(str(hg, "port"))
	} else if tc := sub(m, "tcpSocket"); tc != nil {
		p.TCP = true
		p.Port = atoi(str(tc, "port"))
	}
	return p
}

func parseContainer(cm map[string]any) Container {
	c := Container{Name: str(cm, "name"), Image: str(cm, "image"), Env: map[string]string{}, EnvRefs: map[string]string{}}
	for _, p := range list(cm, "ports") {
		if pm, ok := p.(map[string]any); ok {
			c.Ports = append(c.Ports, atoi(str(pm, "containerPort")))
		}
	}
	for _, e := range list(cm, "env") {
		em, _ := e.(map[string]any)
		if em == nil {
			continue
		}
		if vf := sub(em, "valueFrom"); vf != nil {
			if sk := sub(vf, "secretKeyRef"); sk != nil {
				c.EnvRefs[str(em, "name")] = "secret:" + str(sk, "name") + "/" + str(sk, "key")
			} else if ck := sub(vf, "configMapKeyRef"); ck != nil {
				c.EnvRefs[str(em, "name")] = "configmap:" + str(ck, "name") + "/" + str(ck, "key")
			}
			continue
		}
		c.Env[str(em, "name")] = str(em, "value")
	}
	for _, e := range list(cm, "envFrom") {
		em, _ := e.(map[string]any)
		if r := sub(em, "configMapRef"); r != nil {
			c.EnvFrom = append(c.EnvFrom, "configmap:"+str(r, "name"))
		}
		if r := sub(em, "secretRef"); r != nil {
			c.EnvFrom = append(c.EnvFrom, "secret:"+str(r, "name"))
		}
	}
	c.Requests = Resources{CPU: str(cm, "resources", "requests", "cpu"), Memory: str(cm, "resources", "requests", "memory")}
	c.Limits = Resources{CPU: str(cm, "resources", "limits", "cpu"), Memory: str(cm, "resources", "limits", "memory")}
	c.Readiness = parseProbe(sub(cm, "readinessProbe"))
	c.Liveness = parseProbe(sub(cm, "livenessProbe"))
	var cmd []string
	for _, a := range append(list(cm, "command"), list(cm, "args")...) {
		cmd = append(cmd, fmt.Sprint(a))
	}
	c.Command = strings.Join(cmd, " ")
	return c
}

func (s *State) applyObject(c *Cluster, defaultNS string, m map[string]any) (string, error) {
	kind := str(m, "kind")
	name := str(m, "metadata", "name")
	nsName := str(m, "metadata", "namespace")
	if nsName == "" {
		nsName = defaultNS
	}
	if name == "" && kind != "List" {
		return "", fmt.Errorf("error: resource name may not be empty")
	}
	ns := c.K8s.NS(nsName)
	lk := strings.ToLower(kind)
	switch kind {
	case "Namespace":
		c.K8s.NS(name)
		return "namespace/" + name + " configured", nil
	case "Deployment":
		spec := sub(m, "spec")
		tpl := sub(spec, "template")
		d := ns.Deployments[name]
		created := d == nil
		if d == nil {
			d = &Deployment{Name: name}
			ns.Deployments[name] = d
		}
		d.Labels = strMap(sub(m, "metadata", "labels"))
		d.Selector = strMap(sub(spec, "selector", "matchLabels"))
		r := str(spec, "replicas")
		if r == "" {
			d.Replicas = 1
		} else {
			d.Replicas = atoi(r)
		}
		pt := PodTemplate{Labels: strMap(sub(tpl, "metadata", "labels")), ServiceAccount: str(tpl, "spec", "serviceAccountName")}
		for _, v := range list(tpl, "spec", "volumes") {
			if vm, ok := v.(map[string]any); ok {
				if c := str(vm, "persistentVolumeClaim", "claimName"); c != "" {
					pt.Claims = append(pt.Claims, c)
				}
			}
		}
		for _, cc := range list(tpl, "spec", "containers") {
			if cm, ok := cc.(map[string]any); ok {
				pt.Containers = append(pt.Containers, parseContainer(cm))
			}
		}
		if !selectorMatches(d.Selector, pt.Labels) {
			return "", fmt.Errorf("The Deployment %q is invalid: spec.template.metadata.labels: Invalid value: %v: `selector` does not match template `labels`", name, pt.Labels)
		}
		if created || !templateEq(d.Template, pt) {
			if !created {
				d.History = append(d.History, d.Template)
			}
			d.Template = pt
			d.Revision++
		}
		if hpa := findHPA(ns, name); hpa != nil && r == "" {
			d.Replicas = max(d.Replicas, hpa.Min)
		}
		if created {
			return "deployment.apps/" + name + " created", nil
		}
		return "deployment.apps/" + name + " configured", nil
	case "Service":
		spec := sub(m, "spec")
		svc := &K8sService{Name: name, Type: str(spec, "type"), Selector: strMap(sub(spec, "selector")), Annotations: strMap(sub(m, "metadata", "annotations"))}
		if svc.Type == "" {
			svc.Type = "ClusterIP"
		}
		for _, p := range list(spec, "ports") {
			pm, _ := p.(map[string]any)
			sp := SvcPort{Name: str(pm, "name"), Port: atoi(str(pm, "port")), TargetPort: atoi(str(pm, "targetPort"))}
			if sp.TargetPort == 0 {
				sp.TargetPort = sp.Port
			}
			svc.Ports = append(svc.Ports, sp)
		}
		old := ns.Services[name]
		svc.ClusterIP = fmt.Sprintf("10.112.%d.%d", 1+len(ns.Services)/200, 10+len(ns.Services)%200)
		if old != nil {
			svc.ClusterIP, svc.ExternalIP = old.ClusterIP, old.ExternalIP
		}
		if svc.Type == "LoadBalancer" && svc.ExternalIP == "" {
			svc.ExternalIP = s.ExternalIP()
		}
		ns.Services[name] = svc
		return "service/" + name + " " + createdOr(old == nil), nil
	case "HorizontalPodAutoscaler":
		spec := sub(m, "spec")
		h := &HPA{Name: name, Target: str(spec, "scaleTargetRef", "name"), Min: atoi(str(spec, "minReplicas")), Max: atoi(str(spec, "maxReplicas"))}
		if h.Min == 0 {
			h.Min = 1
		}
		if v := str(spec, "targetCPUUtilizationPercentage"); v != "" {
			h.TargetCPU = atoi(v)
		}
		for _, mt := range list(spec, "metrics") {
			mm, _ := mt.(map[string]any)
			if str(mm, "resource", "name") == "cpu" {
				h.TargetCPU = atoi(str(mm, "resource", "target", "averageUtilization"))
			}
		}
		old := ns.HPAs[name]
		ns.HPAs[name] = h
		return "horizontalpodautoscaler.autoscaling/" + name + " " + createdOr(old == nil), nil
	case "ConfigMap":
		old := ns.ConfigMaps[name]
		ns.ConfigMaps[name] = strMap(sub(m, "data"))
		return "configmap/" + name + " " + createdOr(old == nil), nil
	case "Secret":
		data := strMap(sub(m, "stringData"))
		for k, v := range strMap(sub(m, "data")) {
			if dec, err := base64.StdEncoding.DecodeString(v); err == nil {
				data[k] = string(dec)
			} else {
				data[k] = v
			}
		}
		old := ns.Secrets[name]
		ns.Secrets[name] = data
		return "secret/" + name + " " + createdOr(old == nil), nil
	case "ServiceAccount":
		old := ns.ServiceAccounts[name]
		ns.ServiceAccounts[name] = &KSA{Name: name, Annotations: strMap(sub(m, "metadata", "annotations"))}
		return "serviceaccount/" + name + " " + createdOr(old == nil), nil
	case "Ingress":
		ing := &Ingress{Name: name, Class: str(m, "metadata", "annotations", "kubernetes.io/ingress.class")}
		if old := ns.Ingresses[name]; old != nil {
			ing.IP = old.IP
		} else {
			ing.IP = s.ExternalIP()
		}
		if db := sub(m, "spec", "defaultBackend", "service"); db != nil {
			ing.Rules = append(ing.Rules, IngressRule{Path: "/*", Service: str(db, "name"), Port: atoi(str(db, "port", "number"))})
		}
		for _, r := range list(m, "spec", "rules") {
			rm, _ := r.(map[string]any)
			for _, p := range list(rm, "http", "paths") {
				pm, _ := p.(map[string]any)
				ing.Rules = append(ing.Rules, IngressRule{Host: str(rm, "host"), Path: str(pm, "path"), Service: str(pm, "backend", "service", "name"), Port: atoi(str(pm, "backend", "service", "port", "number"))})
			}
		}
		ns.Ingresses[name] = ing
		return "ingress.networking.k8s.io/" + name + " configured", nil
	case "PersistentVolumeClaim":
		ns.ensure()
		sc := str(m, "spec", "storageClassName")
		if sc == "" {
			sc = "standard-rwo"
		}
		pvc := &PVC{Name: name, StorageClass: sc, Size: str(m, "spec", "resources", "requests", "storage"), Phase: "Bound"}
		if !StorageClasses[sc] {
			pvc.Phase, pvc.Reason = "Pending", fmt.Sprintf("storageclass.storage.k8s.io %q not found", sc)
		}
		existed := ns.PVCs[name] != nil
		ns.PVCs[name] = pvc
		if existed {
			return "persistentvolumeclaim/" + name + " configured", nil
		}
		return "persistentvolumeclaim/" + name + " created", nil
	case "Role":
		ns.ensure()
		r := &K8sRole{Name: name}
		for _, x := range list(m, "rules") {
			xm, _ := x.(map[string]any)
			var rule PolicyRule
			for _, v := range list(xm, "resources") {
				rule.Resources = append(rule.Resources, fmt.Sprint(v))
			}
			for _, v := range list(xm, "verbs") {
				rule.Verbs = append(rule.Verbs, fmt.Sprint(v))
			}
			r.Rules = append(r.Rules, rule)
		}
		ns.Roles[name] = r
		return "role.rbac.authorization.k8s.io/" + name + " created", nil
	case "RoleBinding":
		ns.ensure()
		rb := &RoleBinding{Name: name, Role: str(m, "roleRef", "name")}
		for _, x := range list(m, "subjects") {
			xm, _ := x.(map[string]any)
			if str(xm, "kind") == "ServiceAccount" {
				sn := str(xm, "name")
				if n := str(xm, "namespace"); n != "" {
					sn = n + ":" + sn
				}
				rb.Subjects = append(rb.Subjects, sn)
			}
		}
		ns.RoleBindings[name] = rb
		return "rolebinding.rbac.authorization.k8s.io/" + name + " created", nil
	case "NetworkPolicy":
		np := &NetworkPolicy{Name: name, Selector: strMap(sub(m, "spec", "podSelector", "matchLabels"))}
		for _, in := range list(m, "spec", "ingress") {
			im, _ := in.(map[string]any)
			for _, f := range list(im, "from") {
				fm, _ := f.(map[string]any)
				np.From = strMap(sub(fm, "podSelector", "matchLabels"))
			}
		}
		ns.NetworkPolicies[name] = np
		return "networkpolicy.networking.k8s.io/" + name + " created", nil
	case "Pod":
		cs := list(m, "spec", "containers")
		if len(cs) == 0 {
			return "", fmt.Errorf("pod has no containers")
		}
		ct := parseContainer(cs[0].(map[string]any))
		ns.Pods[name] = &Pod{Name: name, Labels: strMap(sub(m, "metadata", "labels")), Spec: ct, Command: ct.Command, IP: fmt.Sprintf("10.108.99.%d", 10+len(ns.Pods))}
		return "pod/" + name + " created", nil
	}
	return "", fmt.Errorf("error: resource mapping not found for name: %q namespace: %q from \"STDIN\": no matches for kind %q (%s) in the simulator", name, nsName, kind, lk)
}

func createdOr(created bool) string {
	if created {
		return "created"
	}
	return "configured"
}

func templateEq(a, b PodTemplate) bool {
	ja, _ := yaml.Marshal(a)
	jb, _ := yaml.Marshal(b)
	return string(ja) == string(jb)
}

func findHPA(ns *Namespace, deploy string) *HPA {
	for _, h := range ns.HPAs {
		if h.Target == deploy {
			return h
		}
	}
	return nil
}

// tickK8s updates CPU usage, HPA decisions and node autoscaling.
func (s *State) tickK8s(project string, c *Cluster, rpsBySvc map[string]int) {
	if c.K8s == nil {
		return
	}
	for _, nsName := range SortedKeys(c.K8s.Namespaces) {
		ns := c.K8s.Namespaces[nsName]
		// Load generators: standalone pods whose command loops on a service.
		load := map[string]int{}
		for _, p := range ns.Pods {
			cmd := p.Command
			if strings.Contains(cmd, "while") && (strings.Contains(cmd, "wget") || strings.Contains(cmd, "curl")) {
				for sn := range ns.Services {
					if strings.Contains(cmd, "http://"+sn) {
						load[sn] += 400
					}
				}
			}
		}
		for sn, rps := range rpsBySvc {
			load[sn] += rps
		}
		for _, dn := range SortedKeys(ns.Deployments) {
			d := ns.Deployments[dn]
			rps := 0
			for sn, svc := range ns.Services {
				if selectorMatches(svc.Selector, d.Template.Labels) {
					rps += load[sn]
				}
			}
			if len(d.Template.Containers) == 0 {
				continue
			}
			ct := d.Template.Containers[0]
			b, _ := s.LookupImage(ct.Image)
			per := 0.01
			if b != nil && b.CPUPerRPS > 0 {
				per = b.CPUPerRPS
			}
			totalM := float64(rps)*per*1000 + float64(d.Replicas)*5
			reps := max(1, d.Replicas)
			usagePerPod := totalM / float64(reps)
			req := ParseCPU(ct.Requests.CPU)
			s.Metric(fmt.Sprintf("k8s/%s/%s/cpu_millicores", c.Name, d.Name), usagePerPod)
			h := findHPA(ns, d.Name)
			if h == nil {
				continue
			}
			if req == 0 || h.TargetCPU == 0 {
				h.Unknown = true
				h.CurrentCPU = 0
				continue
			}
			h.Unknown = false
			util := int(usagePerPod / float64(req) * 100)
			h.CurrentCPU = util
			desired := int(math.Ceil(float64(reps) * float64(util) / float64(h.TargetCPU)))
			if desired < h.Min {
				desired = h.Min
			}
			if desired > h.Max {
				desired = h.Max
			}
			if desired < d.Replicas && h.ScaledUpTo > h.Min {
				h.ScaledDown = true
			}
			d.Replicas = desired
			if desired > h.ScaledUpTo {
				h.ScaledUpTo = desired
			}
			s.Metric(fmt.Sprintf("k8s/%s/%s/replicas", c.Name, d.Name), float64(desired))
		}
	}
	// Cluster autoscaler: add nodes when pods are pending.
	pods := s.ComputePods(project, c)
	pending := 0
	for _, ps := range pods {
		for _, p := range ps {
			if p.Phase == "Pending" && strings.Contains(p.Reason, "Insufficient cpu") {
				pending++
			}
		}
	}
	if pending > 0 {
		for i := range c.NodePools {
			np := &c.NodePools[i]
			if np.Autoscaling && np.Count < np.Max {
				np.Count++
				break
			}
		}
	}
}

func firstNonEmptyS(v ...string) string {
	for _, x := range v {
		if x != "" {
			return x
		}
	}
	return ""
}
