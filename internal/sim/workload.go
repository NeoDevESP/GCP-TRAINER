package sim

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Behavior describes how a container image or installed package behaves. It
// is the simulator's substitute for actually running application code.
type Behavior struct {
	Name        string   `json:"name" yaml:"name"`
	Port        int      `json:"port" yaml:"port"`
	Health      string   `json:"health" yaml:"health"`
	Paths       []string `json:"paths,omitempty" yaml:"paths"`
	Deps        []Dep    `json:"deps,omitempty" yaml:"deps"`
	Mode        string   `json:"mode,omitempty" yaml:"mode"` // crash, error500, slow
	ErrorPaths  []string `json:"errorPaths,omitempty" yaml:"errorPaths"`
	PoolEnv     string   `json:"poolEnv,omitempty" yaml:"poolEnv"`
	CPUPerRPS   float64  `json:"cpuPerRps,omitempty" yaml:"cpuPerRps"`
	Consumer    bool     `json:"consumer,omitempty" yaml:"consumer"`
	Public      bool     `json:"public,omitempty" yaml:"public"`
	Description string   `json:"description,omitempty" yaml:"description"`
	LatencyMs   int      `json:"latencyMs,omitempty" yaml:"latencyMs"`
	MemMB       int      `json:"memMb,omitempty" yaml:"memMb"` // working set; exceeding a memory limit means OOMKilled
}

// Dep is a runtime dependency resolved from environment variables.
type Dep struct {
	Kind  string   `json:"kind" yaml:"kind"` // postgres, mysql, tcp, gcs-read, gcs-write, pubsub-publish, secret, http, egress, bigquery, vertex
	Env   string   `json:"env" yaml:"env"`
	Port  int      `json:"port,omitempty" yaml:"port"`
	Paths []string `json:"paths,omitempty" yaml:"paths"`
}

// DefaultImages is the built-in image catalogue.
func DefaultImages() map[string]*Behavior {
	pg := []Dep{{Kind: "postgres", Env: "DB_HOST", Port: 5432, Paths: []string{"/checkout", "/cart", "/orders", "/stock"}}}
	return map[string]*Behavior{
		"nginx":    {Name: "nginx", Port: 80, Health: "/", Public: true, CPUPerRPS: 0.002},
		"httpd":    {Name: "httpd", Port: 80, Health: "/", Public: true, CPUPerRPS: 0.002},
		"static":   {Name: "static", Port: 8000, Health: "/", Public: true},
		"postgres": {Name: "postgres", Port: 5432, Public: true},
		"mysql":    {Name: "mysql", Port: 3306, Public: true},
		"redis":    {Name: "redis", Port: 6379, Public: true},
		"hello":    {Name: "hello", Port: 8080, Health: "/", Public: true, CPUPerRPS: 0.003},
		"busybox":  {Name: "busybox", Port: 0, Public: true},
		"checkout-api": {Name: "checkout-api", Port: 8080, Health: "/healthz", Paths: []string{"/", "/healthz", "/checkout", "/cart"}, Deps: pg, PoolEnv: "DB_POOL_SIZE", CPUPerRPS: 0.01, Public: true,
			Description: "Checkout API; needs PostgreSQL at $DB_HOST:5432 with $DB_USER/$DB_PASSWORD"},
		"checkout-api:2.0-bad":     {Name: "checkout-api", Port: 8080, Health: "/healthz", Paths: []string{"/", "/healthz", "/checkout"}, Deps: pg, ErrorPaths: []string{"/checkout"}, Public: true},
		"orders-api":               {Name: "orders-api", Port: 8080, Health: "/healthz", Paths: []string{"/", "/healthz", "/orders"}, Deps: append(append([]Dep{}, pg...), Dep{Kind: "pubsub-publish", Env: "TOPIC", Paths: []string{"/orders"}}), PoolEnv: "DB_POOL_SIZE", CPUPerRPS: 0.01, Public: true},
		"orders-worker":            {Name: "orders-worker", Port: 8080, Health: "/healthz", Paths: []string{"/", "/healthz", "/push"}, Consumer: true, Deps: []Dep{{Kind: "bigquery", Env: "BQ_TABLE", Paths: []string{"/push"}}}, Public: true},
		"events-processor":         {Name: "events-processor", Port: 8080, Health: "/healthz", Paths: []string{"/", "/healthz", "/push"}, Consumer: true, Deps: []Dep{{Kind: "gcs-write", Env: "SINK_BUCKET", Paths: []string{"/push"}}}, Public: true},
		"events-processor:1.1-bad": {Name: "events-processor", Port: 8080, Health: "/healthz", Paths: []string{"/", "/healthz", "/push"}, Consumer: true, ErrorPaths: []string{"/push"}, Public: true},
		"reports-api":              {Name: "reports-api", Port: 8080, Health: "/healthz", Paths: []string{"/", "/healthz", "/reports"}, Deps: []Dep{{Kind: "gcs-read", Env: "BUCKET", Paths: []string{"/reports"}}}, Public: true},
		"uploader":                 {Name: "uploader", Port: 8080, Health: "/healthz", Paths: []string{"/", "/healthz", "/upload"}, Deps: []Dep{{Kind: "gcs-write", Env: "BUCKET", Paths: []string{"/upload"}}}, Public: true},
		"web-frontend":             {Name: "web-frontend", Port: 8080, Health: "/healthz", Paths: []string{"/", "/healthz", "/shop"}, Deps: []Dep{{Kind: "http", Env: "BACKEND_URL", Paths: []string{"/shop"}}}, Public: true, CPUPerRPS: 0.004},
		"catalog-api":              {Name: "catalog-api", Port: 8080, Health: "/healthz", Paths: []string{"/", "/healthz", "/products"}, Public: true, CPUPerRPS: 0.02},
		"payments-api":             {Name: "payments-api", Port: 8080, Health: "/healthz", Paths: []string{"/", "/healthz", "/pay"}, Deps: []Dep{{Kind: "secret", Env: "API_KEY"}}, Public: true},
		"payments-api:3.0-bad":     {Name: "payments-api", Port: 8080, Mode: "crash", Public: true},
		"inventory-api":            {Name: "inventory-api", Port: 8080, Health: "/healthz", Paths: []string{"/", "/healthz", "/stock"}, Deps: pg, PoolEnv: "DB_POOL_SIZE", Public: true},
		"admin-portal":             {Name: "admin-portal", Port: 8080, Health: "/", Paths: []string{"/", "/admin", "/healthz"}, Public: true},
		"model-server":             {Name: "model-server", Port: 8080, Health: "/health", Paths: []string{"/health", "/predict"}, Public: true},
		"shop-api":                 {Name: "shop-api", Port: 8080, Health: "/healthz", Paths: []string{"/", "/healthz", "/products", "/checkout", "/search"}, ErrorPaths: []string{"/search"}, Public: true, CPUPerRPS: 0.01},
		"shop-api:1.4":             {Name: "shop-api", Port: 8080, Health: "/healthz", Paths: []string{"/", "/healthz", "/products", "/checkout", "/search"}, Public: true, CPUPerRPS: 0.01},
		"cpu-burner":               {Name: "cpu-burner", Port: 8080, Health: "/", CPUPerRPS: 0.08, Public: true},
		"report-generator": {Name: "report-generator", Port: 8080, Health: "/healthz", Paths: []string{"/", "/healthz", "/report"}, MemMB: 700, Public: true,
			Description: "Builds PDF reports in memory; needs ~700Mi"},
		"pod-inspector": {Name: "pod-inspector", Port: 8080, Health: "/healthz", Paths: []string{"/", "/healthz", "/pods"}, Deps: []Dep{{Kind: "k8s-api", Paths: []string{"/pods"}}}, Public: true,
			Description: "Lists pods of its namespace through the Kubernetes API (needs RBAC list on pods)"},
		"media-store": {Name: "media-store", Port: 8080, Health: "/healthz", Paths: []string{"/", "/healthz", "/media"}, Public: true, MemMB: 200,
			Description: "Stores uploads on a persistent volume mounted at /data"},
		"egress-app": {Name: "egress-app", Port: 8080, Health: "/healthz", Paths: []string{"/", "/healthz", "/fx"}, Deps: []Dep{{Kind: "egress", Paths: []string{"/fx"}}}, Public: true},
	}
}

// LookupImage finds the behaviour of an image reference like
// europe-docker.pkg.dev/proj/repo/checkout-api:2.0-bad.
func (s *State) LookupImage(ref string) (*Behavior, string) {
	name := ref
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.Index(name, "@"); i >= 0 {
		name = name[:i]
	}
	base, tag := name, "latest"
	if i := strings.Index(name, ":"); i >= 0 {
		base, tag = name[:i], name[i+1:]
	}
	if b := s.Images[base+":"+tag]; b != nil {
		return b, base
	}
	if b := s.Images[base]; b != nil {
		return b, base
	}
	// Student-built images registered by Cloud Build carry an explicit behaviour.
	if v := s.Extra["image:"+ref]; v != "" {
		if b := s.Images[v]; b != nil {
			return b, base
		}
	}
	return nil, base
}

// ImagePullable checks whether an image reference exists and can be pulled by
// a principal from a given network context.
func (s *State) ImagePullable(ref, principal string) (bool, string) {
	if strings.Contains(ref, "-docker.pkg.dev/") {
		parts := strings.Split(ref, "/")
		if len(parts) >= 4 {
			proj, repo := parts[1], parts[2]
			if proj == "gcplab-public" || proj == "cloudrun" {
				return true, ""
			}
			p := s.Projects[proj]
			if p == nil {
				return false, fmt.Sprintf("Image '%s' not found (project %s does not exist)", ref, proj)
			}
			r := p.ArtifactRepos[repo]
			if r == nil {
				return false, fmt.Sprintf("Image '%s' not found: repository %s does not exist", ref, repo)
			}
			img := strings.Join(parts[3:], "/")
			tag := "latest"
			if i := strings.LastIndex(img, ":"); i >= 0 {
				img, tag = img[:i], img[i+1:]
			}
			found := false
			for _, t := range r.Images[img] {
				if t == tag {
					found = true
				}
			}
			if !found {
				return false, fmt.Sprintf("Image '%s' not found.", ref)
			}
			if principal != "" && !s.Allowed(principal, "artifactregistry.repositories.downloadArtifacts", Resource{Project: proj, Type: "artifactregistry.googleapis.com/Repository", Name: "projects/" + proj + "/locations/" + r.Location + "/repositories/" + repo, Service: "artifactregistry.googleapis.com"}) {
				return false, fmt.Sprintf("Permission 'artifactregistry.repositories.downloadArtifacts' denied on resource (or it may not exist) for %s", strings.TrimPrefix(principal, "serviceAccount:"))
			}
			return true, ""
		}
	}
	b, _ := s.LookupImage(ref)
	if b == nil {
		return false, fmt.Sprintf("Image '%s' not found.", ref)
	}
	return true, ""
}

// Listener is a process listening on a VM port.
type Listener struct {
	Name     string            `json:"name"`
	Port     int               `json:"port"`
	Bind     string            `json:"bind"`
	Behavior *Behavior         `json:"-"`
	Env      map[string]string `json:"env,omitempty"`
	Running  bool              `json:"running"`
	Error    string            `json:"error,omitempty"`
}

var (
	reAptInstall = regexp.MustCompile(`(?:apt-get|apt|yum|dnf)\s+install\s+(?:-y\s+)?([a-zA-Z0-9 ._+-]+)`)
	reHTTPServer = regexp.MustCompile(`python3?\s+-m\s+http\.server\s+(\d+)`)
	reDocker     = regexp.MustCompile(`docker\s+run\s+(.+)`)
)

var pkgBehaviors = map[string]string{"nginx": "nginx", "apache2": "httpd", "httpd": "httpd", "postgresql": "postgres", "mysql-server": "mysql", "redis-server": "redis"}

// VMListeners derives the processes running on a VM from its startup script.
// It returns listeners and serial console lines describing boot.
func (s *State) VMListeners(project string, vm *Instance) ([]Listener, []string) {
	var out []Listener
	var console []string
	if vm.Status != "RUNNING" {
		return nil, []string{"Instance is " + vm.Status}
	}
	console = append(console, fmt.Sprintf("[    0.000000] Linux version 6.1.0-cloud-amd64 (%s)", vm.Image), "Starting google-guest-agent...", "google_metadata_script_runner: Starting startup scripts.")
	script := vm.Metadata["startup-script"]
	pre := map[string]bool{}
	for _, p := range strings.FieldsFunc(vm.Metadata["sim-preinstalled"], func(r rune) bool { return r == ',' || r == ' ' }) {
		pre[p] = true
	}
	for pkg := range pkgBehaviors {
		if strings.Contains(vm.Image, pkg) {
			pre[pkg] = true
		}
	}
	ep := s.VMEndpoint(project, vm)
	netOK, netWhy := s.internetPath(ep)
	lines := strings.Split(script, "\n")
	for _, raw := range lines {
		for _, line := range strings.Split(raw, "&&") {
			line = strings.TrimSpace(line)
			if m := reAptInstall.FindStringSubmatch(line); m != nil {
				for _, pkg := range strings.Fields(m[1]) {
					bname, ok := pkgBehaviors[pkg]
					if !ok {
						if !pre[pkg] && !netOK {
							console = append(console, fmt.Sprintf("startup-script: E: Unable to fetch %s: Could not connect to deb.debian.org:80 (%s)", pkg, netWhy))
						}
						continue
					}
					b := s.Images[bname]
					l := Listener{Name: pkg, Port: b.Port, Bind: "0.0.0.0", Behavior: b, Running: true}
					if !pre[pkg] && !netOK {
						l.Running = false
						l.Error = "package installation failed: no internet egress"
						console = append(console, fmt.Sprintf("startup-script: E: Failed to fetch http://deb.debian.org/debian/pool/main/%s: Connection timed out", pkg), "startup-script: E: Unable to locate package "+pkg)
					} else {
						console = append(console, fmt.Sprintf("startup-script: Setting up %s ...", pkg), fmt.Sprintf("systemd[1]: Started %s.", pkg))
					}
					out = append(out, l)
				}
			}
			if m := reHTTPServer.FindStringSubmatch(line); m != nil {
				port, _ := strconv.Atoi(m[1])
				out = append(out, Listener{Name: "python-http", Port: port, Bind: "0.0.0.0", Behavior: s.Images["static"], Running: true})
				console = append(console, fmt.Sprintf("startup-script: Serving HTTP on 0.0.0.0 port %d", port))
			}
			if m := reDocker.FindStringSubmatch(line); m != nil {
				l, msg := s.parseDockerRun(m[1], vm, ep)
				console = append(console, msg)
				if l != nil {
					out = append(out, *l)
				}
			}
		}
	}
	// Faults: stopped services or loopback-only bindings.
	for i := range out {
		key := fmt.Sprintf("svc:%s/%s/%s", project, vm.Name, out[i].Name)
		if vm.OS != nil {
			if why := vm.OS.serviceProblem(out[i].Name); why != "" {
				// the service crashes and stays failed until someone restarts it
				s.Extra[key] = "crashed"
				s.Extra[key+":why"] = why
				out[i].Running, out[i].Error = false, why
				console = append(console, fmt.Sprintf("%s[%d]: %s", out[i].Name, 900+i, why), fmt.Sprintf("systemd[1]: %s.service: Main process exited, code=exited, status=1/FAILURE", out[i].Name))
				continue
			}
			if s.Extra[key] == "crashed" && s.Extra[key+":why"] != "" {
				console = append(console, fmt.Sprintf("%s[%d]: %s", out[i].Name, 900+i, s.Extra[key+":why"]), fmt.Sprintf("systemd[1]: %s.service: Failed with result 'exit-code'.", out[i].Name))
			}
		}
		switch s.Extra[key] {
		case "stopped":
			out[i].Running = false
			out[i].Error = "service stopped"
		case "crashed":
			out[i].Running = false
			out[i].Error = "service crashed (exit code 1)"
		}
		if b := vm.Metadata["sim-bind-"+strconv.Itoa(out[i].Port)]; b != "" {
			out[i].Bind = b
		}
		if b := vm.Metadata["listen-address-"+out[i].Name]; b != "" {
			out[i].Bind = b
		}
	}
	console = append(console, "google_metadata_script_runner: Finished running startup scripts.")
	return out, console
}

func (s *State) parseDockerRun(args string, vm *Instance, ep Endpoint) (*Listener, string) {
	toks := strings.Fields(args)
	env := map[string]string{}
	hostPort, contPort := 0, 0
	image := ""
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch {
		case t == "-p" && i+1 < len(toks):
			i++
			hp := strings.Split(toks[i], ":")
			hostPort, _ = strconv.Atoi(hp[0])
			if len(hp) > 1 {
				contPort, _ = strconv.Atoi(hp[len(hp)-1])
			}
		case t == "-e" && i+1 < len(toks):
			i++
			kv := strings.SplitN(strings.Trim(toks[i], `"'`), "=", 2)
			if len(kv) == 2 {
				env[kv[0]] = kv[1]
			}
		case t == "-d" || t == "--rm" || strings.HasPrefix(t, "--restart") || strings.HasPrefix(t, "--name"):
			if t == "--name" {
				i++
			}
		case strings.HasPrefix(t, "-"):
		default:
			if image == "" {
				image = t
			}
		}
	}
	b, base := s.LookupImage(image)
	if b == nil {
		return nil, "docker: Error response from daemon: manifest for " + image + " not found"
	}
	if contPort == 0 {
		contPort = b.Port
	}
	if hostPort == 0 {
		hostPort = contPort
	}
	l := &Listener{Name: base, Port: hostPort, Bind: "0.0.0.0", Behavior: b, Env: env, Running: true}
	if strings.Contains(image, "-docker.pkg.dev/") && !strings.Contains(image, "gcplab-public") {
		if ok, why := s.GoogleAPIAccess(ep); !ok {
			l.Running = false
			l.Error = "image pull failed: " + why
			return l, "docker: Error response from daemon: Get \"https://" + strings.Split(image, "/")[0] + "/v2/\": dial tcp: i/o timeout"
		}
		if ok, why := s.ImagePullable(image, "serviceAccount:"+vm.ServiceAccount); !ok {
			l.Running = false
			l.Error = why
			return l, "docker: Error response from daemon: " + why
		}
	} else if ok, why := s.internetPath(ep); !ok && !strings.Contains(vm.Metadata["sim-preinstalled"], "docker-images") {
		l.Running = false
		l.Error = "image pull failed: " + why
		return l, "docker: Error response from daemon: Get \"https://registry-1.docker.io/v2/\": dial tcp: i/o timeout"
	}
	if b.Mode == "crash" {
		l.Running = false
		l.Error = "container exited with code 1"
		return l, "docker: container " + base + " exited with code 1"
	}
	return l, fmt.Sprintf("docker: started %s on port %d", image, hostPort)
}

// Workload is an execution context: a process on a VM, a Cloud Run revision or a pod.
type Workload struct {
	Kind      string
	Project   string
	Name      string
	Endpoint  Endpoint
	Principal string
	Scopes    []string
	Env       map[string]string
	Behavior  *Behavior
	CloudSQL  []string
	Instances int
	// Kubernetes context of a pod workload (RBAC checks against the API server).
	K8sNS     *Namespace
	K8sNSName string
	K8sSA     string
}

func scopeAllows(scopes []string, api string, write bool) bool {
	if len(scopes) == 0 {
		return true // non-VM workloads are not scope-limited
	}
	for _, sc := range scopes {
		switch sc {
		case "cloud-platform", "https://www.googleapis.com/auth/cloud-platform":
			return true
		case "storage-full", "storage-rw":
			if api == "storage" {
				return true
			}
		case "storage-ro", "default":
			if api == "storage" && !write {
				return true
			}
			if sc == "default" && (api == "logging" || api == "monitoring") {
				return true
			}
		case "pubsub":
			if api == "pubsub" {
				return true
			}
		case "sql-admin":
			if api == "sql" {
				return true
			}
		case "bigquery":
			if api == "bigquery" {
				return true
			}
		}
	}
	return false
}

// BucketResource builds the IAM resource for a bucket or object prefix.
func (s *State) BucketResource(b *Bucket, object string) Resource {
	name := "projects/_/buckets/" + b.Name
	typ := "storage.googleapis.com/Bucket"
	if object != "" {
		name += "/objects/" + object
		typ = "storage.googleapis.com/Object"
	}
	pap := b.PAP == "enforced" || s.OrgPolicyEnforced(b.Project, "storage.publicAccessPrevention")
	return Resource{Project: b.Project, Type: typ, Name: name, Service: "storage.googleapis.com", Policies: []*Policy{&b.IAM}, NoPublic: pap}
}

func pathMatch(paths []string, p string) bool {
	if len(paths) == 0 {
		return true
	}
	for _, x := range paths {
		if x == p || (strings.HasSuffix(x, "*") && strings.HasPrefix(p, strings.TrimSuffix(x, "*"))) {
			return true
		}
	}
	return false
}

// Call executes a request against a workload and returns status and message.
func (s *State) Call(w *Workload, path string) (int, string) {
	b := w.Behavior
	if b == nil {
		s.note("application", "fail", "no application is serving requests on %s", w.Name)
		return 502, "no application is serving requests"
	}
	if b.Mode == "crash" {
		s.note("application", "fail", "%s (%s) crashes at start-up and never listens", w.Name, b.Name)
		return 503, "container failed to start and listen on the port"
	}
	if path == "" {
		path = "/"
	}
	if len(b.Paths) > 0 && !pathMatch(b.Paths, path) {
		return 404, "Not Found"
	}
	for _, ep := range b.ErrorPaths {
		if ep == path {
			s.note("application", "fail", "%s: handler %s panics (code regression in this image)", b.Name, path)
			return 500, fmt.Sprintf("panic: runtime error: invalid memory address in handler %s (release regression)", path)
		}
	}
	s.note("application", "ok", "%s handles %s as %s", b.Name, path, w.Principal)
	for _, d := range b.Deps {
		if len(d.Paths) > 0 && !pathMatch(d.Paths, path) {
			continue
		}
		if st, msg := s.checkDep(w, d); st != 200 {
			s.note("dependency", "fail", "%s (%s=%q): %s", d.Kind, d.Env, w.Env[d.Env], msg)
			return st, msg
		}
		s.note("dependency", "ok", "%s (%s=%q) reachable and authorised", d.Kind, d.Env, w.Env[d.Env])
	}
	return 200, "OK"
}

func (s *State) checkDep(w *Workload, d Dep) (int, string) {
	val := w.Env[d.Env]
	switch d.Kind {
	case "postgres", "mysql", "tcp", "redis":
		if val == "" {
			return 500, fmt.Sprintf("configuration error: environment variable %s is not set", d.Env)
		}
		return s.checkDB(w, d, val)
	case "gcs-read", "gcs-write":
		if val == "" {
			return 500, fmt.Sprintf("configuration error: %s is not set", d.Env)
		}
		b, _ := s.FindBucket(val)
		if b == nil {
			return 500, fmt.Sprintf("googleapi: Error 404: The specified bucket does not exist., notFound (%s)", val)
		}
		if w.Kind == "vm" || w.Kind == "pod" {
			if ok, why := s.GoogleAPIAccess(w.Endpoint); !ok {
				return 503, "dial tcp storage.googleapis.com:443: i/o timeout (" + why + ")"
			}
		}
		write := d.Kind == "gcs-write"
		perm := "storage.objects.get"
		if write {
			perm = "storage.objects.create"
		}
		if !scopeAllows(w.Scopes, "storage", write) {
			return 403, "googleapi: Error 403: Request had insufficient authentication scopes., forbidden (instance access scopes)"
		}
		obj := w.Env["OBJECT_PREFIX"]
		if obj == "" {
			obj = "data/report.csv"
		}
		if !s.Allowed(w.Principal, perm, s.BucketResource(b, obj)) {
			return 403, fmt.Sprintf("googleapi: Error 403: %s does not have %s access to the Google Cloud Storage object. Permission '%s' denied on resource (or it may not exist)., forbidden", strings.TrimPrefix(w.Principal, "serviceAccount:"), perm, perm)
		}
		if !write && perm == "storage.objects.get" && !s.Allowed(w.Principal, "storage.objects.list", s.BucketResource(b, "")) && w.Env["NEEDS_LIST"] == "true" {
			return 403, "googleapi: Error 403: storage.objects.list denied"
		}
		return 200, ""
	case "pubsub-publish":
		if val == "" {
			return 500, "configuration error: TOPIC is not set"
		}
		tp, tn := splitResource(val, w.Project, "topics")
		p := s.Projects[tp]
		if p == nil || p.Topics[tn] == nil {
			return 500, fmt.Sprintf("rpc error: code = NotFound desc = Resource not found (resource=%s)", tn)
		}
		if w.Kind == "vm" || w.Kind == "pod" {
			if ok, why := s.GoogleAPIAccess(w.Endpoint); !ok {
				return 503, "pubsub.googleapis.com:443 unreachable (" + why + ")"
			}
		}
		if !scopeAllows(w.Scopes, "pubsub", true) {
			return 403, "rpc error: code = PermissionDenied desc = Request had insufficient authentication scopes."
		}
		if !s.Allowed(w.Principal, "pubsub.topics.publish", Resource{Project: tp, Type: "pubsub.googleapis.com/Topic", Name: "projects/" + tp + "/topics/" + tn, Service: "pubsub.googleapis.com"}) {
			return 403, fmt.Sprintf("rpc error: code = PermissionDenied desc = User not authorized to perform this action. (pubsub.topics.publish on projects/%s/topics/%s)", tp, tn)
		}
		return 200, ""
	case "k8s-api":
		if w.K8sNS == nil {
			return 500, "not running in Kubernetes: no in-cluster API credentials"
		}
		if !w.K8sNS.RBACAllows(w.K8sNSName, w.K8sSA, "list", "pods") {
			return 403, fmt.Sprintf("pods is forbidden: User \"system:serviceaccount:%s:%s\" cannot list resource \"pods\" in API group \"\" in the namespace \"%s\"", w.K8sNSName, w.K8sSA, w.K8sNSName)
		}
		return 200, ""
	case "secret":
		if val == "" {
			return 500, fmt.Sprintf("configuration error: secret %s is empty or not mounted", d.Env)
		}
		return 200, ""
	case "http":
		if val == "" {
			return 500, "configuration error: " + d.Env + " is not set"
		}
		r := s.HTTP(HTTPRequest{From: w.Endpoint, Principal: w.Principal, URL: val, Internal: true})
		if r.Status != 200 {
			if r.Status == 0 {
				return 502, "upstream request failed: " + r.Error
			}
			return 502, fmt.Sprintf("upstream %s returned %d", val, r.Status)
		}
		return 200, ""
	case "egress":
		if w.Kind == "run" {
			return 200, ""
		}
		if ok, why := s.internetPath(w.Endpoint); !ok {
			return 504, "dial tcp api.exchangerate.example:443: i/o timeout (" + why + ")"
		}
		return 200, ""
	case "bigquery":
		if val == "" {
			return 500, "configuration error: BQ_TABLE is not set"
		}
		parts := strings.Split(strings.ReplaceAll(val, ":", "."), ".")
		proj, ds := w.Project, parts[0]
		if len(parts) == 3 {
			proj, ds = parts[0], parts[1]
		}
		p := s.Projects[proj]
		if p == nil || p.Datasets[ds] == nil {
			return 500, "googleapi: Error 404: Not found: Dataset " + val
		}
		if !s.Allowed(w.Principal, "bigquery.tables.updateData", Resource{Project: proj, Type: "bigquery.googleapis.com/Dataset", Name: "projects/" + proj + "/datasets/" + ds, Service: "bigquery.googleapis.com", Policies: []*Policy{&p.Datasets[ds].IAM}}) {
			return 403, "googleapi: Error 403: Access Denied: Table " + val + ": Permission bigquery.tables.updateData denied"
		}
		return 200, ""
	case "vertex":
		return 200, ""
	}
	return 200, ""
}

func splitResource(val, defProject, kind string) (string, string) {
	if strings.HasPrefix(val, "projects/") {
		parts := strings.Split(val, "/")
		if len(parts) >= 4 {
			return parts[1], parts[3]
		}
	}
	return defProject, val
}

// DBMaxConnections returns max_connections for an instance.
func DBMaxConnections(in *SQLInstance) int {
	if v, ok := in.Flags["max_connections"]; ok {
		n, _ := strconv.Atoi(v)
		return n
	}
	switch in.Tier {
	case "db-f1-micro":
		return 25
	case "db-g1-small":
		return 50
	}
	if strings.HasPrefix(in.Tier, "db-custom-1") {
		return 100
	}
	if strings.HasPrefix(in.Tier, "db-custom-2") || strings.HasPrefix(in.Tier, "db-perf-optimized") {
		return 200
	}
	return 100
}

func (s *State) checkDB(w *Workload, d Dep, host string) (int, string) {
	port := d.Port
	var target Endpoint
	var sqlIn *SQLInstance
	if strings.HasPrefix(host, "/cloudsql/") {
		conn := strings.TrimPrefix(host, "/cloudsql/")
		parts := strings.Split(conn, ":")
		if len(parts) != 3 {
			return 500, "invalid Cloud SQL connection name " + conn
		}
		p := s.Projects[parts[0]]
		if p == nil || p.SQLInstances[parts[2]] == nil {
			return 500, fmt.Sprintf("failed to connect to instance: Cloud SQL instance %q does not exist", conn)
		}
		sqlIn = p.SQLInstances[parts[2]]
		if s.RegionDown(sqlIn.Region) {
			return 503, fmt.Sprintf("failed to connect to instance %s: instance unavailable (regional outage in %s)", conn, sqlIn.Region)
		}
		listed := false
		for _, c := range w.CloudSQL {
			if c == conn {
				listed = true
			}
		}
		if w.Kind == "run" && !listed {
			return 500, fmt.Sprintf("dial unix /cloudsql/%s/.s.PGSQL.5432: connect: no such file or directory (instance not attached with --add-cloudsql-instances)", conn)
		}
		if !s.Allowed(w.Principal, "cloudsql.instances.connect", Resource{Project: parts[0], Type: "sqladmin.googleapis.com/Instance", Name: "projects/" + parts[0] + "/instances/" + parts[2], Service: "sqladmin.googleapis.com"}) {
			return 500, fmt.Sprintf("failed to connect to instance: Cloud SQL Admin API returned 403: the principal %s lacks cloudsql.instances.connect (roles/cloudsql.client)", strings.TrimPrefix(w.Principal, "serviceAccount:"))
		}
		if sqlIn.PublicIP == "" && w.Kind == "run" && w.Endpoint.Network == "" {
			return 500, "failed to connect: instance has no public IP and the service has no VPC access"
		}
	} else {
		ip, ok := s.ResolveHost(host, w.Endpoint)
		if !ok {
			return 500, fmt.Sprintf("could not translate host name %q to address: Name or service not known", host)
		}
		t, ok := s.FindIP(ip)
		if !ok {
			return 504, fmt.Sprintf("connection to server at %q (%s), port %d failed: Connection timed out", host, ip, port)
		}
		target = t
		if t.Kind == "sql" {
			sqlIn = s.Projects[t.Project].SQLInstances[t.Name]
			if ip == sqlIn.PrivateIP {
				if w.Endpoint.Network == "" || !s.networksConnected(w.Endpoint.Project, w.Endpoint.Network, t.Project, sqlIn.Network) {
					return 504, fmt.Sprintf("connection to server at %q (%s), port %d failed: Connection timed out (source has no VPC path to the private IP)", host, ip, port)
				}
				if sp := s.Projects[w.Endpoint.Project]; sp != nil && (w.Endpoint.Kind == "vm" || w.Endpoint.Kind == "pod" || w.Endpoint.Kind == "run-vpc") {
					if ok, rule := sp.evalFirewall("EGRESS", w.Endpoint, t, true, "tcp", port); !ok {
						return 504, fmt.Sprintf("connection to server at %q (%s), port %d failed: Connection timed out (egress denied by %s)", host, ip, port, rule)
					}
				}
			} else {
				srcIP := s.egressIP(w)
				allowed := false
				for _, an := range sqlIn.AuthorizedNets {
					if srcIP != "" && IPInCIDR(srcIP, an) {
						allowed = true
					}
				}
				if !allowed {
					return 504, fmt.Sprintf("connection to server at %q (%s), port %d failed: Connection timed out (source %s not in authorized networks)", host, ip, port, srcIP)
				}
			}
		} else if t.Kind == "vm" {
			fr := s.CheckFlow(w.Endpoint, t, "tcp", port)
			if !fr.Allowed {
				return 504, fmt.Sprintf("connection to server at %q (%s), port %d failed: Connection timed out", host, ip, port)
			}
			vm := s.Projects[t.Project].Instances[t.Name]
			ls, _ := s.VMListeners(t.Project, vm)
			up := false
			for _, l := range ls {
				if l.Port == port && l.Running && l.Bind != "127.0.0.1" && l.Bind != "localhost" {
					up = true
				}
			}
			if !up {
				return 503, fmt.Sprintf("connection to server at %q (%s), port %d failed: Connection refused\n\tIs the server running on that host and accepting TCP/IP connections?", host, ip, port)
			}
			return 200, ""
		}
	}
	if sqlIn == nil {
		return 200, ""
	}
	_ = target
	if sqlIn.State != "RUNNABLE" {
		return 503, fmt.Sprintf("connection failed: instance %s is %s", sqlIn.Name, sqlIn.State)
	}
	user := w.Env["DB_USER"]
	if user == "" {
		user = "app"
	}
	pw, ok := sqlIn.Users[user]
	if !ok {
		return 500, fmt.Sprintf("FATAL:  password authentication failed for user \"%s\" (role does not exist)", user)
	}
	if w.Env["DB_PASSWORD"] != pw {
		return 500, fmt.Sprintf("FATAL:  password authentication failed for user \"%s\"", user)
	}
	if dbn := w.Env["DB_NAME"]; dbn != "" {
		found := false
		for _, x := range sqlIn.Databases {
			if x == dbn {
				found = true
			}
		}
		if !found {
			return 500, fmt.Sprintf("FATAL:  database \"%s\" does not exist", dbn)
		}
	}
	if sqlIn.Connections > DBMaxConnections(sqlIn) {
		return 503, "FATAL:  sorry, too many clients already (remaining connection slots are reserved for non-replication superuser connections)"
	}
	return 200, ""
}

// egressIP returns the public source IP used by a workload for internet traffic.
func (s *State) egressIP(w *Workload) string {
	switch w.Endpoint.Kind {
	case "vm":
		if p := s.Projects[w.Endpoint.Project]; p != nil {
			if vm := p.Instances[w.Endpoint.Name]; vm != nil && vm.ExternalIP != "" {
				return vm.ExternalIP
			}
		}
		if s.natCovers(w.Endpoint.Project, w.Endpoint.Network, w.Endpoint.Region, w.Endpoint.Subnet) {
			return "34.140.10.1"
		}
	case "internet":
		return w.Endpoint.IP
	case "run":
		return "34.117.99.99" // dynamic Google-owned egress
	}
	return ""
}

// ---------------------------------------------------------------------------
// HTTP engine

// HTTPRequest is a simulated HTTP call.
type HTTPRequest struct {
	From      Endpoint
	Principal string // identity token subject ("" = anonymous)
	URL       string
	Method    string
	ViaLB     bool
	Internal  bool
	SourceIP  string
}

// HTTPResponse is the simulated result.
type HTTPResponse struct {
	Status int      `json:"status"`
	Body   string   `json:"body"`
	Error  string   `json:"error,omitempty"`
	Trace  []string `json:"trace"`
	Target string   `json:"target"`
}

// HTTP executes a request end-to-end through the simulated infrastructure.
func (s *State) HTTP(req HTTPRequest) HTTPResponse {
	u, err := url.Parse(req.URL)
	if err != nil || u.Host == "" {
		if !strings.Contains(req.URL, "://") {
			u, err = url.Parse("http://" + req.URL)
		}
		if err != nil || u == nil || u.Host == "" {
			return HTTPResponse{Error: "URL rejected: Malformed input to a URL function"}
		}
	}
	host := u.Hostname()
	path := u.Path
	if path == "" {
		path = "/"
	}
	port := 80
	if u.Scheme == "https" {
		port = 443
	}
	if u.Port() != "" {
		port, _ = strconv.Atoi(u.Port())
	}
	res := HTTPResponse{}
	if (host == "localhost" || host == "127.0.0.1") && req.From.Kind == "vm" {
		if p := s.Projects[req.From.Project]; p != nil {
			if vm := p.Instances[req.From.Name]; vm != nil {
				return s.callVMPortLocal(req.From.Project, vm, port, path, res, true)
			}
		}
	}
	// Cloud Run URLs
	if strings.HasSuffix(host, ".run.app") {
		for _, pid := range SortedKeys(s.Projects) {
			for _, sn := range SortedKeys(s.Projects[pid].RunServices) {
				svc := s.Projects[pid].RunServices[sn]
				if strings.Contains(svc.URL, host) {
					s.note("dns", "ok", "%s is the Cloud Run URL of service %s (Google front end, TLS terminated by Google)", host, sn)
					return s.callRun(pid, svc, req, path)
				}
			}
		}
		return HTTPResponse{Status: 404, Body: "Error: Page not found", Error: "The requested URL was not found on this server."}
	}
	// Kubernetes in-cluster names
	if req.From.Kind == "pod" {
		if r, ok := s.k8sResolveCall(req.From, host, port, path); ok {
			return r
		}
	}
	ip, ok := s.ResolveHost(host, req.From)
	if !ok {
		s.note("dns", "fail", "%s does not resolve from %s (no public/private DNS record)", host, endpointName(req.From))
		return HTTPResponse{Error: fmt.Sprintf("Could not resolve host: %s", host)}
	}
	s.note("dns", "ok", "%s resolves to %s", host, ip)
	res.Trace = append(res.Trace, fmt.Sprintf("resolved %s -> %s", host, ip))
	// Load balancers
	for _, pid := range SortedKeys(s.Projects) {
		p := s.Projects[pid]
		for _, fn := range SortedKeys(p.ForwardingRules) {
			fr := p.ForwardingRules[fn]
			if fr.IP != ip {
				continue
			}
			if !portInList(fr.Ports, port) {
				s.note("load-balancer", "fail", "forwarding rule %s listens on %v, not on port %d", fn, fr.Ports, port)
				return HTTPResponse{Error: fmt.Sprintf("Failed to connect to %s port %d: Connection refused", host, port), Trace: res.Trace}
			}
			s.note("load-balancer", "ok", "%s:%d is forwarding rule %s (global external HTTP(S) load balancer, TCP/TLS terminated at the Google edge)", ip, port, fn)
			r := s.callLB(pid, fr, req, host, path)
			r.Trace = append(res.Trace, r.Trace...)
			return r
		}
		// GKE LoadBalancer services
		if r, ok := s.k8sExternalCall(pid, ip, port, path, req); ok {
			return r
		}
	}
	ep, ok := s.FindIP(ip)
	if !ok {
		// Somewhere on the internet.
		if req.From.Kind == "vm" || req.From.Kind == "pod" || req.From.Kind == "run-vpc" {
			if ok, why := s.internetPath(req.From); !ok {
				return HTTPResponse{Error: fmt.Sprintf("Failed to connect to %s port %d: Connection timed out", host, port), Trace: append(res.Trace, why)}
			}
		}
		return HTTPResponse{Status: 200, Body: "<html>external site</html>", Trace: res.Trace, Target: "internet"}
	}
	if ep.Kind != "vm" {
		return HTTPResponse{Error: fmt.Sprintf("Failed to connect to %s port %d: Connection refused", host, port), Trace: res.Trace}
	}
	src := req.From
	if src.Kind == "" {
		src = InternetEndpoint(req.SourceIP)
	}
	vm := s.Projects[ep.Project].Instances[ep.Name]
	if src.Kind == "internet" && ip == vm.InternalIP {
		return HTTPResponse{Error: fmt.Sprintf("Failed to connect to %s port %d: Connection timed out (private address not routable from the internet)", host, port), Trace: res.Trace}
	}
	dst := ep
	dst.IP = ip
	fl := s.CheckFlow(src, dst, "tcp", port)
	res.Trace = append(res.Trace, fl.Reason)
	if fl.Allowed {
		s.note("network", "ok", "TCP %s → %s:%d allowed: %s", endpointName(src), vm.Name, port, fl.Reason)
	} else {
		s.note("network", "fail", "TCP %s → %s:%d blocked: %s", endpointName(src), vm.Name, port, fl.Reason)
	}
	if !fl.Allowed {
		res.Error = fmt.Sprintf("Failed to connect to %s port %d after 130000 ms: Connection timed out", host, port)
		return res
	}
	return s.callVMPort(ep.Project, vm, port, path, res)
}

func portInList(ports []string, port int) bool {
	if len(ports) == 0 {
		return true
	}
	for _, p := range ports {
		if strings.Contains(p, "-") {
			ab := strings.SplitN(p, "-", 2)
			lo, _ := strconv.Atoi(ab[0])
			hi, _ := strconv.Atoi(ab[1])
			if port >= lo && port <= hi {
				return true
			}
		} else if n, _ := strconv.Atoi(p); n == port {
			return true
		}
	}
	return false
}

func (s *State) callVMPort(project string, vm *Instance, port int, path string, res HTTPResponse) HTTPResponse {
	return s.callVMPortLocal(project, vm, port, path, res, false)
}

func (s *State) callVMPortLocal(project string, vm *Instance, port int, path string, res HTTPResponse, local bool) HTTPResponse {
	if s.ZoneDown(vm.Zone) {
		s.note("zone", "fail", "zone %s of %s is unavailable (outage)", vm.Zone, vm.Name)
		res.Error = fmt.Sprintf("Failed to connect to %s port %d: Connection timed out (zone %s unavailable)", vm.InternalIP, port, vm.Zone)
		return res
	}
	ls, _ := s.VMListeners(project, vm)
	for _, l := range ls {
		if l.Port != port {
			continue
		}
		if !l.Running {
			s.note("process", "fail", "%s on %s is not running (%s) — nothing accepts connections on port %d", l.Name, vm.Name, l.Error, port)
			break
		}
		if !local && (l.Bind == "127.0.0.1" || l.Bind == "localhost") {
			s.note("process", "fail", "%s on %s listens only on %s:%d — remote clients are refused", l.Name, vm.Name, l.Bind, port)
			break
		}
		s.note("process", "ok", "%s listening on %s:%d on %s", l.Name, l.Bind, port, vm.Name)
		w := &Workload{Kind: "vm", Project: project, Name: vm.Name, Endpoint: s.VMEndpoint(project, vm), Principal: "serviceAccount:" + vm.ServiceAccount, Scopes: vm.Scopes, Env: l.Env, Behavior: l.Behavior}
		st, msg := s.Call(w, path)
		res.Status, res.Body, res.Target = st, msg, "vm:"+vm.Name
		if st == 200 {
			res.Body = fmt.Sprintf("Hello from %s (%s)", vm.Name, l.Name)
		}
		return res
	}
	if len(ls) == 0 || !anyPort(ls, port) {
		s.note("process", "fail", "nothing listens on port %d on %s (connection refused)", port, vm.Name)
	}
	res.Error = fmt.Sprintf("Failed to connect to %s port %d: Connection refused", vm.InternalIP, port)
	return res
}

func anyPort(ls []Listener, port int) bool {
	for _, l := range ls {
		if l.Port == port {
			return true
		}
	}
	return false
}

func endpointName(e Endpoint) string {
	switch e.Kind {
	case "", "internet":
		return "the internet"
	case "vm", "pod":
		return e.Kind + " " + e.Name
	}
	return e.Kind
}

// RunWorkload builds the workload view of a Cloud Run service.
func (s *State) RunWorkload(project string, svc *RunService) (*Workload, string) {
	p := s.Projects[project]
	sa := svc.SA
	if sa == "" {
		sa = p.Number + "-compute@developer.gserviceaccount.com"
	}
	principal := "serviceAccount:" + sa
	b, _ := s.LookupImage(svc.Image)
	env := map[string]string{}
	for k, v := range svc.Env {
		env[k] = v
	}
	for k, ref := range svc.Secrets {
		val, err := s.AccessSecret(project, principal, ref)
		if err != "" {
			return nil, fmt.Sprintf("Revision is not ready: spec.template.spec.containers[0].env.%s: %s", k, err)
		}
		env[k] = val
	}
	ep := Endpoint{Kind: "run", Project: project, Name: svc.Name, Region: svc.Region}
	if svc.Network != "" || svc.VPCConnector != "" {
		netw := svc.Network
		subnet := svc.Subnet
		if svc.VPCConnector != "" && netw == "" {
			netw = s.Extra["connector:"+project+"/"+svc.VPCConnector]
			if netw == "" {
				netw = "default"
			}
		}
		if subnet == "" {
			if sn := p.SubnetForRegion(netw, svc.Region); sn != nil {
				subnet = sn.Name
			}
		}
		ip := ""
		if sn := p.Subnets[subnet]; sn != nil {
			key := "runip:" + project + "/" + svc.Name
			if s.Extra[key] == "" {
				s.Extra[key] = s.AllocIP(sn.Range)
			}
			ip = s.Extra[key]
		}
		var tags []string
		if t := svc.Labels["network-tags"]; t != "" {
			tags = strings.Split(t, ".")
		}
		ep = Endpoint{Kind: "run-vpc", Project: project, Network: netw, Subnet: subnet, Region: svc.Region, Name: svc.Name, IP: ip, Tags: tags, SA: sa}
	}
	if b == nil {
		return nil, fmt.Sprintf("Revision is not ready: Image '%s' not found.", svc.Image)
	}
	if ok, why := s.ImagePullable(svc.Image, ""); !ok {
		return nil, "Revision is not ready: " + why
	}
	if b.Mode == "crash" {
		return nil, "Revision is not ready: The user-provided container failed to start and listen on the port defined provided by the PORT=8080 environment variable."
	}
	n := svc.Instances
	if n < 1 {
		n = 1
	}
	return &Workload{Kind: "run", Project: project, Name: svc.Name, Endpoint: ep, Principal: principal, Env: env, Behavior: b, CloudSQL: svc.CloudSQL, Instances: n}, ""
}

// AccessSecret resolves "name:version" (or projects/.../secrets/name/versions/v).
func (s *State) AccessSecret(project, principal, ref string) (string, string) {
	name, ver := ref, "latest"
	if strings.HasPrefix(ref, "projects/") {
		parts := strings.Split(ref, "/")
		if len(parts) >= 4 {
			project, name = parts[1], parts[3]
		}
		if len(parts) >= 6 {
			ver = parts[5]
		}
	} else if i := strings.LastIndex(ref, ":"); i >= 0 {
		name, ver = ref[:i], ref[i+1:]
	}
	p := s.Projects[project]
	if p == nil {
		return "", "project not found"
	}
	sec := p.Secrets[name]
	if sec == nil {
		return "", fmt.Sprintf("Secret projects/%s/secrets/%s was not found", p.Number, name)
	}
	if principal != "" && !s.Allowed(principal, "secretmanager.versions.access", Resource{Project: project, Type: "secretmanager.googleapis.com/Secret", Name: "projects/" + project + "/secrets/" + name, Service: "secretmanager.googleapis.com", Policies: []*Policy{&sec.IAM}}) {
		return "", fmt.Sprintf("Permission denied on secret: projects/%s/secrets/%s/versions/%s for Revision service account %s. The service account used must be granted the 'Secret Manager Secret Accessor' role (roles/secretmanager.secretAccessor) at the secret, project or higher level.", p.Number, name, ver, strings.TrimPrefix(principal, "serviceAccount:"))
	}
	var v *SecretVersion
	if ver == "latest" {
		for i := len(sec.Versions) - 1; i >= 0; i-- {
			if sec.Versions[i].State == "ENABLED" {
				v = &sec.Versions[i]
				break
			}
		}
	} else {
		n, _ := strconv.Atoi(ver)
		for i := range sec.Versions {
			if sec.Versions[i].ID == n {
				v = &sec.Versions[i]
			}
		}
	}
	if v == nil {
		return "", fmt.Sprintf("Secret version %s/%s not found", name, ver)
	}
	if v.State != "ENABLED" {
		return "", fmt.Sprintf("Secret version projects/%s/secrets/%s/versions/%d is in DISABLED state.", p.Number, name, v.ID)
	}
	return v.Data, ""
}

func (s *State) callRun(project string, svc *RunService, req HTTPRequest, path string) HTTPResponse {
	res := HTTPResponse{Target: "run:" + svc.Name}
	if s.RegionDown(svc.Region) {
		s.note("region", "fail", "region %s is unavailable (regional outage): Cloud Run service %s cannot serve", svc.Region, svc.Name)
		res.Status, res.Body = 503, "Service Unavailable (regional outage in "+svc.Region+")"
		return res
	}
	switch svc.Ingress {
	case "internal":
		if req.From.Kind != "vm" && req.From.Kind != "pod" && req.From.Kind != "run-vpc" {
			s.note("ingress", "fail", "Cloud Run service %s accepts only internal traffic", svc.Name)
			res.Status, res.Body = 404, "Error: Page not found (ingress is restricted to internal traffic)"
			return res
		}
	case "internal-and-cloud-load-balancing":
		if !req.ViaLB && req.From.Kind != "vm" && req.From.Kind != "pod" && req.From.Kind != "run-vpc" {
			res.Status, res.Body = 404, "Error: Page not found (ingress allows only internal and Cloud Load Balancing traffic)"
			return res
		}
	}
	principal := req.Principal
	if principal == "" {
		principal = "anonymous"
	}
	allowed := s.Allowed(principal, "run.routes.invoke", Resource{Project: project, Type: "run.googleapis.com/Service", Name: "projects/" + project + "/locations/" + svc.Region + "/services/" + svc.Name, Service: "run.googleapis.com", Policies: []*Policy{&svc.IAM}})
	if principal == "anonymous" && !svc.IAM.HasMember("roles/run.invoker", "allUsers") {
		allowed = false
	}
	if !allowed {
		s.note("iam", "fail", "%s lacks run.routes.invoke on Cloud Run service %s (no roles/run.invoker for it or allUsers)", principal, svc.Name)
		res.Status = 403
		res.Body = "Error: Forbidden\nYour client does not have permission to get URL " + path + " from this server."
		return res
	}
	s.note("iam", "ok", "%s may invoke %s (ingress %s)", principal, svc.Name, svc.Ingress)
	w, why := s.RunWorkload(project, svc)
	if w == nil {
		s.note("workload", "fail", "Cloud Run revision of %s cannot serve: %s", svc.Name, why)
		res.Status, res.Body = 503, "Service Unavailable: "+why
		return res
	}
	s.note("workload", "ok", "revision of %s runs %s as %s", svc.Name, svc.Image, w.Principal)
	st, msg := s.Call(w, path)
	res.Status, res.Body = st, msg
	if st == 200 {
		res.Body = fmt.Sprintf("Hello from Cloud Run service %s (%s)", svc.Name, svc.Image)
	}
	return res
}

// HealthState is the result of a load balancer health probe.
type HealthState struct {
	Instance string `json:"instance"`
	Healthy  bool   `json:"healthState"`
	Reason   string `json:"reason"`
}

// BackendHealth evaluates health of all backends of a backend service.
func (s *State) BackendHealth(project string, bs *BackendService) []HealthState {
	p := s.Projects[project]
	var out []HealthState
	var hc *HealthCheck
	if len(bs.HealthChecks) > 0 {
		hc = p.HealthChecks[bs.HealthChecks[0]]
	}
	for _, be := range bs.Backends {
		if be.NEG != "" {
			neg := p.NEGs[be.NEG]
			h := HealthState{Instance: "neg:" + be.NEG, Healthy: neg != nil}
			if neg != nil {
				if svc := p.RunServices[neg.RunService]; svc == nil {
					h.Healthy, h.Reason = false, "Cloud Run service not found"
				}
			}
			out = append(out, h)
			continue
		}
		ig := p.InstanceGroups[be.Group]
		if ig == nil {
			continue
		}
		port := 80
		if bs.PortName != "" {
			if np, ok := ig.NamedPorts[bs.PortName]; ok {
				port = np
			} else {
				for _, n := range ig.Instances {
					out = append(out, HealthState{Instance: n, Reason: fmt.Sprintf("instance group has no named port %q", bs.PortName)})
				}
				continue
			}
		}
		for _, n := range ig.Instances {
			vm := p.Instances[n]
			if vm == nil {
				continue
			}
			out = append(out, s.probeInstance(project, vm, hc, port))
		}
	}
	return out
}

func (s *State) probeInstance(project string, vm *Instance, hc *HealthCheck, servingPort int) HealthState {
	// health probes are summarised by the caller; do not record their inner hops
	saved := s.explain
	s.explain = nil
	defer func() { s.explain = saved }()
	h := HealthState{Instance: vm.Name}
	if hc == nil {
		h.Reason = "backend service has no health check"
		return h
	}
	if s.ZoneDown(vm.Zone) {
		h.Reason = "zone " + vm.Zone + " unavailable (outage)"
		return h
	}
	port := hc.Port
	if port == 0 {
		port = servingPort
	}
	gfe := Endpoint{Kind: "gfe", IP: "35.191.10.20"}
	dst := s.VMEndpoint(project, vm)
	fl := s.CheckFlow(gfe, dst, "tcp", port)
	if !fl.Allowed {
		h.Reason = "health check probes from 35.191.0.0/16 and 130.211.0.0/22 are blocked: " + fl.Reason
		return h
	}
	res := s.callVMPort(project, vm, port, hc.RequestPath, HTTPResponse{})
	if res.Status != 200 {
		if res.Error != "" {
			h.Reason = res.Error
		} else {
			h.Reason = fmt.Sprintf("health check %s returned HTTP %d", hc.RequestPath, res.Status)
		}
		return h
	}
	h.Healthy = true
	h.Reason = "HEALTHY"
	return h
}

func (s *State) callLB(project string, fr *ForwardingRule, req HTTPRequest, host, path string) HTTPResponse {
	p := s.Projects[project]
	res := HTTPResponse{Target: "lb:" + fr.Name}
	var bsName string
	if fr.BackendService != "" {
		bsName = fr.BackendService
	} else {
		tp := p.TargetProxies[fr.Target]
		if tp == nil {
			res.Status, res.Body = 404, "target proxy not found"
			return res
		}
		um := p.URLMaps[tp.URLMap]
		if um == nil {
			res.Status, res.Body = 404, "url map not found"
			return res
		}
		bsName = um.Resolve(host, path)
	}
	bs := p.BackendServices[bsName]
	res.Trace = append(res.Trace, "url-map -> backend service "+bsName)
	if bs == nil {
		s.note("load-balancer", "fail", "the URL map sends %s%s to %q which does not exist", host, path, bsName)
	} else {
		s.note("load-balancer", "ok", "URL map routes %s%s to backend service %s", host, path, bsName)
	}
	if bs == nil {
		res.Status, res.Body = 404, "no backend service for "+path
		return res
	}
	if bs.SecurityPolicy != "" {
		if sp := p.SecurityPolicies[bs.SecurityPolicy]; sp != nil {
			src := req.SourceIP
			if src == "" {
				src = StudentIP
			}
			if act, rule := sp.Evaluate(src, path); strings.HasPrefix(act, "deny") {
				code := 403
				if strings.Contains(act, "404") {
					code = 404
				} else if strings.Contains(act, "502") {
					code = 502
				}
				s.note("cloud-armor", "fail", "security policy %s rule %s matched source %s → %s", bs.SecurityPolicy, rule, src, act)
				res.Status, res.Body = code, "Forbidden by Cloud Armor rule "+rule
				return res
			}
		}
	}
	// serverless NEG
	for _, be := range bs.Backends {
		if be.NEG != "" {
			if neg := p.NEGs[be.NEG]; neg != nil {
				if svc := p.RunServices[neg.RunService]; svc != nil {
					if s.RegionDown(svc.Region) || s.RegionDown(neg.Region) {
						s.note("load-balancer", "info", "serverless NEG %s (%s) skipped: region unavailable", neg.Name, neg.Region)
						continue
					}
					r := req
					r.ViaLB = true
					r.Principal = ""
					out := s.callRun(project, svc, r, path)
					out.Trace = append(res.Trace, out.Trace...)
					return out
				}
			}
		}
	}
	health := s.BackendHealth(project, bs)
	var healthy []string
	for _, h := range health {
		if h.Healthy {
			healthy = append(healthy, h.Instance)
			s.note("backend-health", "ok", "%s is HEALTHY", h.Instance)
		} else {
			s.note("backend-health", "fail", "%s is UNHEALTHY: %s", h.Instance, h.Reason)
		}
	}
	if len(health) == 0 {
		s.note("backend-health", "fail", "backend service %s has no backends", bs.Name)
	}
	if len(healthy) == 0 {
		res.Status, res.Body = 502, "Error: Server Error\nThe server encountered a temporary error and could not complete your request. (failed_to_pick_backend: no healthy upstream)"
		return res
	}
	sort.Strings(healthy)
	vm := p.Instances[healthy[s.Tick%len(healthy)]]
	port := 80
	for _, be := range bs.Backends {
		if ig := p.InstanceGroups[be.Group]; ig != nil && bs.PortName != "" {
			if np, ok := ig.NamedPorts[bs.PortName]; ok {
				port = np
			}
		}
	}
	s.note("network", "ok", "load balancer proxies to %s:%d (named port %q)", vm.Name, port, bs.PortName)
	out := s.callVMPort(project, vm, port, path, res)
	if out.Status == 0 {
		out.Status, out.Body = 502, "upstream connect error"
	}
	return out
}

// Resolve picks the backend service for host and path.
func (um *URLMap) Resolve(host, path string) string {
	matcher := ""
	for _, hr := range um.HostRules {
		for _, h := range hr.Hosts {
			if h == "*" || h == host {
				matcher = hr.PathMatcher
			}
		}
	}
	for _, pm := range um.PathMatchers {
		if pm.Name != matcher {
			continue
		}
		for _, pr := range pm.PathRules {
			for _, pp := range pr.Paths {
				if pp == path || (strings.HasSuffix(pp, "/*") && strings.HasPrefix(path, strings.TrimSuffix(pp, "*"))) || (strings.HasSuffix(pp, "*") && strings.HasPrefix(path, strings.TrimSuffix(pp, "*"))) {
					return pr.Service
				}
			}
		}
		if pm.DefaultService != "" {
			return pm.DefaultService
		}
	}
	return um.DefaultService
}

// Evaluate applies Cloud Armor rules; returns action and rule priority.
func (sp *SecurityPolicy) Evaluate(srcIP, path string) (string, string) {
	rules := append([]SPRule{}, sp.Rules...)
	sort.Slice(rules, func(i, j int) bool { return rules[i].Priority < rules[j].Priority })
	for _, r := range rules {
		match := false
		for _, rg := range r.SrcIPRanges {
			if rg == "*" || IPInCIDR(srcIP, rg) {
				match = true
			}
		}
		if r.Expression != "" {
			e := r.Expression
			if m := regexp.MustCompile(`request\.path\.(?:matches|startsWith)\(['"]([^'"]+)['"]\)`).FindStringSubmatch(e); m != nil {
				pat := strings.Trim(m[1], "^$")
				pat = strings.TrimSuffix(pat, ".*")
				if strings.HasPrefix(path, pat) {
					match = true
				}
			}
			if m := regexp.MustCompile(`inIpRange\(origin\.ip,\s*['"]([^'"]+)['"]\)`).FindStringSubmatch(e); m != nil && IPInCIDR(srcIP, m[1]) {
				match = true
			}
			if strings.Contains(e, "evaluatePreconfiguredWaf") || strings.Contains(e, "evaluatePreconfiguredExpr") {
				if strings.Contains(path, "'") || strings.Contains(path, "<script") || strings.Contains(path, "..") {
					match = true
				}
			}
		}
		if match && !r.Preview {
			return strings.ToLower(r.Action), fmt.Sprint(r.Priority)
		}
	}
	return "allow", "default"
}
