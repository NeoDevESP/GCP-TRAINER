package sim

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sqlengine"
)

// More managed services: Cloud Functions, Cloud Scheduler, Cloud Tasks, App
// Engine, Firestore, Memorystore for Redis, Spanner, Dataflow, Dataproc,
// Cloud Composer and Filestore. They take part in the simulation: Pub/Sub
// messages and Cloud Storage uploads trigger functions, Scheduler jobs run
// on their cron, Cloud Tasks are dispatched and Dataflow streaming jobs move
// messages into BigQuery as simulated time passes.

// Function is a Cloud Function (1st or 2nd gen).
type Function struct {
	Name         string            `json:"name"`
	Region       string            `json:"region"`
	Runtime      string            `json:"runtime"`
	EntryPoint   string            `json:"entryPoint"`
	Gen2         bool              `json:"gen2"`
	Trigger      string            `json:"trigger"` // http, topic:NAME, bucket:NAME
	URL          string            `json:"url,omitempty"`
	SA           string            `json:"serviceAccount"`
	Env          map[string]string `json:"env"`
	Memory       string            `json:"memory"`
	Timeout      int               `json:"timeoutSeconds"`
	MinInstances int               `json:"minInstances"`
	MaxInstances int               `json:"maxInstances"`
	Ingress      string            `json:"ingress"`
	State        string            `json:"state"`
	Source       string            `json:"source"`
	Response     string            `json:"response,omitempty"` // what the code answers (from the source)
	Invocations  int               `json:"invocations"`
	Errors       int               `json:"errors"`
	Updated      string            `json:"updateTime"`
	Version      int               `json:"versionId"`
	IAM          Policy            `json:"iamPolicy"`
}

// SchedulerJob is a Cloud Scheduler job.
type SchedulerJob struct {
	Name       string `json:"name"`
	Location   string `json:"location"`
	Schedule   string `json:"schedule"`
	TimeZone   string `json:"timeZone"`
	Target     string `json:"targetType"` // http, pubsub
	URI        string `json:"uri,omitempty"`
	Method     string `json:"httpMethod,omitempty"`
	Topic      string `json:"topic,omitempty"`
	Message    string `json:"messageBody,omitempty"`
	OIDC       string `json:"oidcServiceAccount,omitempty"`
	State      string `json:"state"`
	LastRun    string `json:"lastAttemptTime,omitempty"`
	LastStatus string `json:"lastStatus,omitempty"`
	Runs       int    `json:"runs"`
}

// TaskQueue is a Cloud Tasks queue.
type TaskQueue struct {
	Name          string       `json:"name"`
	Location      string       `json:"location"`
	State         string       `json:"state"`
	MaxDispatches float64      `json:"maxDispatchesPerSecond"`
	MaxAttempts   int          `json:"maxAttempts"`
	Tasks         []*CloudTask `json:"tasks"`
	Dispatched    int          `json:"dispatched"`
}

// CloudTask is a task in a queue.
type CloudTask struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Method   string `json:"method"`
	Body     string `json:"body,omitempty"`
	Created  string `json:"createTime"`
	Attempts int    `json:"dispatchCount"`
	Status   string `json:"lastResponse,omitempty"`
}

// AppEngineApp is the App Engine application of a project.
type AppEngineApp struct {
	Region   string                `json:"locationId"`
	Host     string                `json:"defaultHostname"`
	Status   string                `json:"servingStatus"`
	Services map[string]*AEService `json:"services"`
}

// AEService is an App Engine service and its traffic split.
type AEService struct {
	Name     string                `json:"name"`
	Versions map[string]*AEVersion `json:"versions"`
	Split    map[string]float64    `json:"split"`
	SplitBy  string                `json:"shardBy"`
}

// AEVersion is a deployed App Engine version.
type AEVersion struct {
	ID       string `json:"id"`
	Runtime  string `json:"runtime"`
	Env      string `json:"env"`
	Status   string `json:"servingStatus"`
	Created  string `json:"createTime"`
	Scaling  string `json:"scaling"`
	Response string `json:"response"`
}

// FirestoreDB is a Firestore (or Datastore mode) database.
type FirestoreDB struct {
	Name             string                    `json:"name"`
	Location         string                    `json:"locationId"`
	Type             string                    `json:"type"`
	DeleteProtection bool                      `json:"deleteProtection"`
	Docs             map[string]map[string]any `json:"documents"` // "collection/doc" -> fields
	Indexes          []FirestoreIndex          `json:"indexes"`
	PITR             bool                      `json:"pointInTimeRecovery"`
}

// FirestoreIndex is a composite index.
type FirestoreIndex struct {
	ID         string   `json:"id"`
	Collection string   `json:"collectionGroup"`
	Fields     []string `json:"fields"`
	State      string   `json:"state"`
}

// RedisInstance is a Memorystore for Redis instance.
type RedisInstance struct {
	Name        string            `json:"name"`
	Region      string            `json:"region"`
	Tier        string            `json:"tier"`
	SizeGB      int               `json:"memorySizeGb"`
	Version     string            `json:"redisVersion"`
	Host        string            `json:"host"`
	Port        int               `json:"port"`
	Network     string            `json:"authorizedNetwork"`
	State       string            `json:"state"`
	AuthEnabled bool              `json:"authEnabled"`
	AuthString  string            `json:"authString,omitempty"`
	Data        map[string]string `json:"data"`
}

// SpannerInstance is a Cloud Spanner instance.
type SpannerInstance struct {
	Name            string                 `json:"name"`
	Config          string                 `json:"config"`
	DisplayName     string                 `json:"displayName"`
	ProcessingUnits int                    `json:"processingUnits"`
	State           string                 `json:"state"`
	Databases       map[string]*SpannerDB `json:"databases"`
}

// SpannerDB is a Spanner database: its DDL and data.
type SpannerDB struct {
	Name             string        `json:"name"`
	State            string        `json:"state"`
	DDL              []string      `json:"ddl"`
	Data             *sqlengine.DB `json:"data,omitempty"`
	DeleteProtection bool          `json:"enableDropProtection"`
}

// DataflowJob is a Dataflow job started from a template.
type DataflowJob struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Region    string            `json:"region"`
	Template  string            `json:"template"`
	Type      string            `json:"type"` // JOB_TYPE_BATCH, JOB_TYPE_STREAMING
	State     string            `json:"currentState"`
	Created   string            `json:"createTime"`
	Params    map[string]string `json:"parameters"`
	Processed int               `json:"elementsProcessed"`
	Workers   int               `json:"workers"`
	Error     string            `json:"error,omitempty"`
}

// DataprocCluster is a Dataproc cluster.
type DataprocCluster struct {
	Name         string `json:"clusterName"`
	Region       string `json:"region"`
	Zone         string `json:"zone"`
	MasterType   string `json:"masterMachineType"`
	WorkerType   string `json:"workerMachineType"`
	Workers      int    `json:"numWorkers"`
	Preemptible  int    `json:"numSecondaryWorkers"`
	ImageVersion string `json:"imageVersion"`
	SingleNode   bool   `json:"singleNode"`
	State        string `json:"state"`
	Created      string `json:"createTime"`
}

// DataprocJob is a job submitted to a cluster.
type DataprocJob struct {
	ID      string   `json:"jobId"`
	Cluster string   `json:"clusterName"`
	Region  string   `json:"region"`
	Type    string   `json:"type"`
	Main    string   `json:"main"`
	Args    []string `json:"args"`
	State   string   `json:"state"`
	Output  string   `json:"driverOutput"`
	Created string   `json:"submitTime"`
}

// ComposerEnv is a Cloud Composer environment.
type ComposerEnv struct {
	Name         string   `json:"name"`
	Location     string   `json:"location"`
	State        string   `json:"state"`
	ImageVersion string   `json:"imageVersion"`
	Size         string   `json:"environmentSize"`
	Bucket       string   `json:"bucket"`
	DagPrefix    string   `json:"dagGcsPrefix"`
	AirflowURI   string   `json:"airflowUri"`
	Runs         []DagRun `json:"dagRuns"`
}

// DagRun is a triggered DAG run.
type DagRun struct {
	DAG   string `json:"dagId"`
	RunID string `json:"runId"`
	State string `json:"state"`
	At    string `json:"executionDate"`
}

// FilestoreInstance is a Filestore NFS server.
type FilestoreInstance struct {
	Name       string `json:"name"`
	Location   string `json:"location"`
	Tier       string `json:"tier"`
	Share      string `json:"fileShare"`
	CapacityGB int    `json:"capacityGb"`
	IP         string `json:"ipAddress"`
	Network    string `json:"network"`
	State      string `json:"state"`
}

// EnsureServices initialises the maps of the newer services (states saved
// before they existed have them nil).
func (p *Project) EnsureServices() {
	if p.Functions == nil {
		p.Functions = map[string]*Function{}
	}
	if p.SchedulerJobs == nil {
		p.SchedulerJobs = map[string]*SchedulerJob{}
	}
	if p.TaskQueues == nil {
		p.TaskQueues = map[string]*TaskQueue{}
	}
	if p.Firestore == nil {
		p.Firestore = map[string]*FirestoreDB{}
	}
	if p.Redis == nil {
		p.Redis = map[string]*RedisInstance{}
	}
	if p.Spanner == nil {
		p.Spanner = map[string]*SpannerInstance{}
	}
	if p.DataprocClusters == nil {
		p.DataprocClusters = map[string]*DataprocCluster{}
	}
	if p.Composer == nil {
		p.Composer = map[string]*ComposerEnv{}
	}
	if p.Filestore == nil {
		p.Filestore = map[string]*FilestoreInstance{}
	}
}

// ---- Cloud Functions ----------------------------------------------------------

var reFuncResponse = regexp.MustCompile(`(?:return|res\.send|res\.status\(\d+\)\.send|fmt\.Fprint(?:f|ln)?\(w,)\s*\(?\s*[fb]?["'` + "`" + `]([^"'` + "`" + `]{1,120})["'` + "`" + `]`)

// FunctionResponse extracts what the function source answers.
func FunctionResponse(source string) string {
	if m := reFuncResponse.FindStringSubmatch(source); m != nil {
		return m[1]
	}
	return "Hello World!"
}

// InvokeFunction runs a function for an event and returns status and body.
func (s *State) InvokeFunction(project string, fn *Function, event, data string) (int, string) {
	fn.Invocations++
	body := fn.Response
	if body == "" {
		body = "Hello World!"
	}
	if strings.Contains(body, "{name}") || strings.Contains(body, "%s") {
		name := "World"
		var m map[string]any
		if json.Unmarshal([]byte(data), &m) == nil {
			if v, ok := m["name"]; ok {
				name = fmt.Sprint(v)
			}
		}
		body = strings.NewReplacer("{name}", name, "%s", name).Replace(body)
	}
	status := 200
	if fn.State != "ACTIVE" {
		status, body = 503, "Service Unavailable"
		fn.Errors++
	}
	kind := "cloud_function"
	if fn.Gen2 {
		kind = "cloud_run_revision"
	}
	s.Log(project, LogEntry{Severity: "INFO", LogName: "cloudfunctions.googleapis.com%2Fcloud-functions", Resource: LogResource{Type: kind, Labels: map[string]string{"function_name": fn.Name, "region": fn.Region, "service_name": fn.Name}},
		Text: fmt.Sprintf("Function execution started (%s)", event)})
	sev := "INFO"
	if status >= 500 {
		sev = "ERROR"
	}
	s.Log(project, LogEntry{Severity: sev, LogName: "cloudfunctions.googleapis.com%2Fcloud-functions", Resource: LogResource{Type: kind, Labels: map[string]string{"function_name": fn.Name, "region": fn.Region, "service_name": fn.Name}},
		Text: fmt.Sprintf("Function execution took %d ms, finished with status code: %d", 40+s.Rand().Intn(200), status)})
	return status, body
}

// OnPublish delivers a Pub/Sub message to functions triggered by the topic.
func (s *State) OnPublish(project, topic, data string) {
	p := s.Projects[project]
	if p == nil {
		return
	}
	for _, n := range SortedKeys(p.Functions) {
		if fn := p.Functions[n]; fn.Trigger == "topic:"+topic {
			s.InvokeFunction(project, fn, "google.cloud.pubsub.topic.v1.messagePublished", data)
		}
	}
}

// OnObjectFinalize delivers a Cloud Storage upload to functions triggered by the bucket.
func (s *State) OnObjectFinalize(project, bucket, object string) {
	for _, pid := range SortedKeys(s.Projects) {
		p := s.Projects[pid]
		for _, n := range SortedKeys(p.Functions) {
			if fn := p.Functions[n]; fn.Trigger == "bucket:"+bucket {
				s.InvokeFunction(pid, fn, "google.cloud.storage.object.v1.finalized", fmt.Sprintf(`{"name": %q, "bucket": %q}`, object, bucket))
			}
		}
	}
}

// FunctionAllowsUnauth reports whether allUsers may invoke the function.
func FunctionAllowsUnauth(fn *Function) bool {
	for _, b := range fn.IAM.Bindings {
		if b.Role == "roles/cloudfunctions.invoker" || b.Role == "roles/run.invoker" {
			for _, m := range b.Members {
				if m == "allUsers" {
					return true
				}
			}
		}
	}
	return false
}

// ---- time-driven behaviour -------------------------------------------------------

// CronMatches reports whether a 5-field cron expression fires at the clock minute.
func CronMatches(expr string, minute, hour, dom, month, dow int) bool {
	f := strings.Fields(expr)
	if len(f) != 5 {
		return false
	}
	vals := []int{minute, hour, dom, month, dow}
	for i, field := range f {
		if !cronField(field, vals[i]) {
			return false
		}
	}
	return true
}

func cronField(field string, v int) bool {
	for _, part := range strings.Split(field, ",") {
		step := 1
		if i := strings.Index(part, "/"); i >= 0 {
			step, _ = strconv.Atoi(part[i+1:])
			part = part[:i]
			if step <= 0 {
				step = 1
			}
		}
		lo, hi := 0, 59
		switch {
		case part == "*":
		case strings.Contains(part, "-"):
			ab := strings.SplitN(part, "-", 2)
			lo, _ = strconv.Atoi(ab[0])
			hi, _ = strconv.Atoi(ab[1])
		default:
			n, err := strconv.Atoi(part)
			if err != nil {
				return false
			}
			lo, hi = n, n
			if step > 1 {
				hi = 59
			}
		}
		if v >= lo && v <= hi && (v-lo)%step == 0 {
			return true
		}
	}
	return false
}

// RunSchedulerJob runs a job now (on its schedule or with "jobs run").
func (s *State) RunSchedulerJob(project string, j *SchedulerJob) {
	j.Runs++
	j.LastRun = s.Now()
	switch j.Target {
	case "pubsub":
		if msg, err := s.Publish(project, j.Topic, j.Message, nil); err != nil {
			j.LastStatus = "NOT_FOUND: " + err.Error()
		} else {
			_ = msg
			j.LastStatus = "OK"
		}
	default:
		principal := ""
		if j.OIDC != "" {
			principal = "serviceAccount:" + j.OIDC
		}
		res := s.HTTP(HTTPRequest{From: Endpoint{Kind: "internet", IP: "107.178.192.10"}, Principal: principal, URL: j.URI, SourceIP: "107.178.192.10"})
		switch {
		case res.Status == 0:
			j.LastStatus = "UNAVAILABLE: " + res.Error
		case res.Status >= 400:
			j.LastStatus = fmt.Sprintf("HTTP %d", res.Status)
		default:
			j.LastStatus = "OK"
		}
	}
	sev := "INFO"
	if j.LastStatus != "OK" {
		sev = "ERROR"
	}
	s.Log(project, LogEntry{Severity: sev, LogName: "cloudscheduler.googleapis.com%2Fexecutions", Resource: LogResource{Type: "cloud_scheduler_job", Labels: map[string]string{"job_id": j.Name, "location": j.Location}},
		Text: fmt.Sprintf("Job %s attempted: %s", j.Name, j.LastStatus)})
}

func (s *State) tickServices() {
	c := s.Clock.UTC()
	for _, pid := range SortedKeys(s.Projects) {
		p := s.Projects[pid]
		for _, n := range SortedKeys(p.SchedulerJobs) {
			j := p.SchedulerJobs[n]
			if j.State == "ENABLED" && CronMatches(j.Schedule, c.Minute(), c.Hour(), c.Day(), int(c.Month()), int(c.Weekday())) {
				s.RunSchedulerJob(pid, j)
			}
		}
		for _, n := range SortedKeys(p.TaskQueues) {
			q := p.TaskQueues[n]
			if q.State != "RUNNING" {
				continue
			}
			var keep []*CloudTask
			for _, t := range q.Tasks {
				t.Attempts++
				res := s.HTTP(HTTPRequest{From: Endpoint{Kind: "internet", IP: "107.178.192.20"}, URL: t.URL, SourceIP: "107.178.192.20"})
				t.Status = fmt.Sprintf("%d", res.Status)
				if res.Status >= 200 && res.Status < 300 {
					q.Dispatched++
					continue
				}
				if q.MaxAttempts > 0 && t.Attempts >= q.MaxAttempts {
					continue
				}
				keep = append(keep, t)
			}
			q.Tasks = keep
		}
		for _, j := range p.DataflowJobs {
			if j.State != "JOB_STATE_RUNNING" || j.Type != "JOB_TYPE_STREAMING" {
				continue
			}
			s.dataflowStep(pid, j)
		}
	}
}

// dataflowStep moves Pub/Sub messages into BigQuery or Cloud Storage.
func (s *State) dataflowStep(project string, j *DataflowJob) {
	p := s.Projects[project]
	sub := p.Subs[lastPart(j.Params["inputSubscription"])]
	if sub == nil {
		if t := lastPart(j.Params["inputTopic"]); t != "" {
			sub = p.Subs["dataflow-"+j.ID]
			if sub == nil && p.Topics[t] != nil {
				sub = &Subscription{Name: "dataflow-" + j.ID, Topic: t, AckDeadline: 60}
				p.Subs[sub.Name] = sub
			}
		}
	}
	if sub == nil || len(sub.Backlog) == 0 {
		return
	}
	msgs := sub.Backlog
	sub.Backlog = nil
	sub.Acked += len(msgs)
	j.Processed += len(msgs)
	if spec := j.Params["outputTableSpec"]; spec != "" {
		_, t, _, _ := s.findTable(project, spec)
		if t == nil {
			j.State, j.Error = "JOB_STATE_FAILED", "output table "+spec+" not found"
			return
		}
		var rows [][]any
		cols := []string{}
		for _, f := range t.Schema {
			cols = append(cols, f.Name)
		}
		for _, m := range msgs {
			var obj map[string]any
			if json.Unmarshal([]byte(m.Data), &obj) != nil {
				continue // malformed messages go to the dead-letter table in real life
			}
			row := make([]any, len(cols))
			for i, cn := range cols {
				row[i] = obj[cn]
			}
			rows = append(rows, row)
		}
		t.AppendRows(cols, rows)
		return
	}
	if dir := j.Params["outputDirectory"]; strings.HasPrefix(dir, "gs://") {
		b := s.findBucket(strings.SplitN(strings.TrimPrefix(dir, "gs://"), "/", 2)[0])
		if b == nil {
			return
		}
		var lines []string
		for _, m := range msgs {
			lines = append(lines, m.Data)
		}
		prefix := strings.TrimPrefix(strings.TrimPrefix(dir, "gs://"+b.Name), "/")
		name := fmt.Sprintf("%soutput-%s-%d.txt", prefix, j.ID[:8], s.Tick)
		content := strings.Join(lines, "\n") + "\n"
		b.Objects[name] = &Object{Name: name, Size: len(content), Content: content, StorageClass: b.StorageClass, Generation: 1, Updated: s.Now()}
	}
}

func lastPart(s string) string { return s[strings.LastIndex(s, "/")+1:] }

func (s *State) findBucket(name string) *Bucket {
	for _, pid := range SortedKeys(s.Projects) {
		if b := s.Projects[pid].Buckets[name]; b != nil {
			return b
		}
	}
	return nil
}

// WordCount counts words (Dataflow and Dataproc word count examples).
func WordCount(text string) string {
	counts := map[string]int{}
	for _, w := range regexp.MustCompile(`[A-Za-zÀ-ÿ']+`).FindAllString(strings.ToLower(text), -1) {
		counts[w]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool {
		if counts[keys[a]] != counts[keys[b]] {
			return counts[keys[a]] > counts[keys[b]]
		}
		return keys[a] < keys[b]
	})
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s: %d\n", k, counts[k])
	}
	return b.String()
}

// SpannerToSQLite rewrites Spanner DDL for the SQL engine.
func SpannerToSQLite(ddl string) string {
	s := regexp.MustCompile(`(?is)\)\s*PRIMARY\s+KEY\s*\(([^)]*)\)`).ReplaceAllString(ddl, ", PRIMARY KEY ($1))")
	s = regexp.MustCompile(`(?is),\s*INTERLEAVE\s+IN\s+PARENT\s+\w+(\s+ON\s+DELETE\s+(CASCADE|NO\s+ACTION))?`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`(?i)\bSTRING\s*\(\s*(\d+|MAX)\s*\)`).ReplaceAllString(s, "TEXT")
	s = regexp.MustCompile(`(?i)\bBYTES\s*\(\s*(\d+|MAX)\s*\)`).ReplaceAllString(s, "BLOB")
	s = regexp.MustCompile(`(?i)\bINT64\b`).ReplaceAllString(s, "INTEGER")
	s = regexp.MustCompile(`(?i)\bFLOAT64\b|\bNUMERIC\b`).ReplaceAllString(s, "REAL")
	s = regexp.MustCompile(`(?i)\bBOOL\b`).ReplaceAllString(s, "INTEGER")
	s = regexp.MustCompile(`(?i)\bOPTIONS\s*\([^)]*\)`).ReplaceAllString(s, "")
	return s
}

// ---- HTTP endpoints of the new services -------------------------------------------

// callServiceURL answers requests to Cloud Functions and App Engine URLs.
func (s *State) callServiceURL(host, path string, req HTTPRequest) (HTTPResponse, bool) {
	for _, pid := range SortedKeys(s.Projects) {
		p := s.Projects[pid]
		for _, n := range SortedKeys(p.Functions) {
			fn := p.Functions[n]
			if fn.URL == "" || !strings.Contains(fn.URL, "//"+host) {
				continue
			}
			if fn.Trigger != "http" {
				return HTTPResponse{Status: 404, Body: "Error: Page not found"}, true
			}
			principal := req.Principal
			if principal == "" {
				principal = "anonymous"
			}
			ok := FunctionAllowsUnauth(fn)
			if !ok && principal != "anonymous" {
				perm := "cloudfunctions.functions.invoke"
				if fn.Gen2 {
					perm = "run.routes.invoke"
				}
				ok = s.Allowed(principal, perm, Resource{Project: pid, Type: "cloudfunctions.googleapis.com/CloudFunction", Name: "projects/" + pid + "/locations/" + fn.Region + "/functions/" + fn.Name, Service: "cloudfunctions.googleapis.com", Policies: []*Policy{&fn.IAM}})
			}
			if !ok {
				s.note("iam", "fail", "function %s requires authentication and %s cannot invoke it", fn.Name, principal)
				return HTTPResponse{Status: 403, Body: "<html><head><title>403 Forbidden</title></head><body><h1>Error: Forbidden</h1><h2>Your client does not have permission to get URL /" + fn.Name + " from this server.</h2></body></html>"}, true
			}
			st, body := s.InvokeFunction(pid, fn, "http", req.Body)
			return HTTPResponse{Status: st, Body: body, Target: "function:" + fn.Name}, true
		}
		if app := p.AppEngine; app != nil && strings.HasSuffix(host, ".appspot.com") && (host == app.Host || strings.HasSuffix(host, "-dot-"+app.Host)) {
			svcName, version := "default", ""
			if pre := strings.TrimSuffix(host, app.Host); pre != "" {
				parts := strings.Split(strings.TrimSuffix(pre, "-dot-"), "-dot-")
				if len(parts) == 2 {
					version, svcName = parts[0], parts[1]
				} else {
					svcName = parts[0]
					if app.Services[svcName] == nil {
						svcName, version = "default", parts[0]
					}
				}
			}
			svc := app.Services[svcName]
			if svc == nil || len(svc.Versions) == 0 {
				return HTTPResponse{Status: 404, Body: "Error: Not Found\nThe requested URL was not found on this server."}, true
			}
			if version == "" {
				version = s.pickVersion(svc, req.SourceIP)
			}
			v := svc.Versions[version]
			if v == nil || v.Status != "SERVING" {
				return HTTPResponse{Status: 404, Body: "Error: Not Found\nThe requested version was not found or is stopped."}, true
			}
			s.Log(pid, LogEntry{Severity: "INFO", LogName: "appengine.googleapis.com%2Frequest_log", Resource: LogResource{Type: "gae_app", Labels: map[string]string{"module_id": svcName, "version_id": version}}, HTTP: &HTTPRequestLog{Method: "GET", URL: path, Status: 200}})
			return HTTPResponse{Status: 200, Body: v.Response, Target: "appengine:" + svcName + "/" + version}, true
		}
	}
	return HTTPResponse{}, false
}

// pickVersion chooses the version that serves a request from the traffic split.
func (s *State) pickVersion(svc *AEService, sourceIP string) string {
	keys := SortedKeys(svc.Split)
	if len(keys) == 0 {
		return ""
	}
	r := s.Rand().Float64()
	if svc.SplitBy == "ip" && sourceIP != "" {
		h := 0
		for _, c := range sourceIP {
			h = h*31 + int(c)
		}
		r = float64(h%1000) / 1000
	}
	acc := 0.0
	for _, k := range keys {
		acc += svc.Split[k]
		if r < acc {
			return k
		}
	}
	return keys[len(keys)-1]
}

// ---- Firestore REST API -------------------------------------------------------------

var reFirestorePath = regexp.MustCompile(`^/v1/projects/([^/]+)/databases/([^/]+)/documents(?:/(.*))?$`)

// firestoreREST serves firestore.googleapis.com/v1 document requests.
func (s *State) firestoreREST(path string, req HTTPRequest) HTTPResponse {
	query := ""
	if i := strings.Index(path, "?"); i >= 0 {
		path, query = path[:i], path[i+1:]
	}
	m := reFirestorePath.FindStringSubmatch(path)
	if m == nil {
		return firestoreErr(404, "NOT_FOUND", "The requested URL "+path+" was not found on this server.")
	}
	pid, dbName, rest := m[1], strings.Trim(m[2], "()"), strings.Trim(m[3], "/")
	p := s.Projects[pid]
	if p == nil {
		return firestoreErr(403, "PERMISSION_DENIED", "Permission denied on resource project "+pid+".")
	}
	if req.Principal == "" || req.Principal == "anonymous" {
		return firestoreErr(401, "UNAUTHENTICATED", "Request is missing required authentication credential. Expected OAuth 2 access token. Add -H \"Authorization: Bearer $(gcloud auth print-access-token)\".")
	}
	p.EnsureServices()
	if dbName == "default" {
		dbName = "(default)"
	}
	db := p.Firestore[dbName]
	if db == nil {
		return firestoreErr(404, "NOT_FOUND", fmt.Sprintf("The database %s does not exist for project %s. Create it with gcloud firestore databases create.", dbName, pid))
	}
	method := req.Method
	if method == "" {
		method = "GET"
	}
	perm := map[string]string{"GET": "datastore.entities.get", "POST": "datastore.entities.create", "PATCH": "datastore.entities.update", "DELETE": "datastore.entities.delete"}[method]
	if perm == "" {
		return firestoreErr(405, "INVALID_ARGUMENT", "method not allowed")
	}
	if !s.Allowed(req.Principal, perm, Resource{Project: pid, Type: "firestore.googleapis.com/Database", Name: "projects/" + pid + "/databases/" + dbName, Service: "firestore.googleapis.com"}) {
		return firestoreErr(403, "PERMISSION_DENIED", "Missing or insufficient permissions ("+perm+").")
	}
	if db.Docs == nil {
		db.Docs = map[string]map[string]any{}
	}
	base := "projects/" + pid + "/databases/" + dbName + "/documents/"
	segs := strings.Split(rest, "/")
	if rest == "" {
		return firestoreErr(400, "INVALID_ARGUMENT", "a collection id is required")
	}
	isColl := len(segs)%2 == 1
	switch {
	case method == "GET" && isColl:
		var docs []any
		for _, k := range SortedKeys(db.Docs) {
			if strings.HasPrefix(k, rest+"/") && !strings.Contains(strings.TrimPrefix(k, rest+"/"), "/") {
				docs = append(docs, firestoreDoc(base+k, db.Docs[k]))
			}
		}
		if len(docs) == 0 {
			return HTTPResponse{Status: 200, Body: "{}"}
		}
		return jsonResp(200, map[string]any{"documents": docs})
	case method == "GET":
		d, ok := db.Docs[rest]
		if !ok {
			return firestoreErr(404, "NOT_FOUND", "Document \""+base+rest+"\" not found.")
		}
		return jsonResp(200, firestoreDoc(base+rest, d))
	case method == "POST" && isColl:
		id := ""
		for _, kv := range strings.Split(query, "&") {
			if strings.HasPrefix(kv, "documentId=") {
				id = strings.TrimPrefix(kv, "documentId=")
			}
		}
		if id == "" {
			id = s.ID(20)
		}
		key := rest + "/" + id
		if _, ok := db.Docs[key]; ok {
			return firestoreErr(409, "ALREADY_EXISTS", "Document already exists: "+base+key)
		}
		fields, err := firestoreFields(req.Body)
		if err != nil {
			return firestoreErr(400, "INVALID_ARGUMENT", err.Error())
		}
		db.Docs[key] = fields
		return jsonResp(200, firestoreDoc(base+key, fields))
	case method == "PATCH" && !isColl:
		fields, err := firestoreFields(req.Body)
		if err != nil {
			return firestoreErr(400, "INVALID_ARGUMENT", err.Error())
		}
		if old, ok := db.Docs[rest]; ok && strings.Contains(query, "updateMask") {
			for k, v := range fields {
				old[k] = v
			}
			fields = old
		}
		db.Docs[rest] = fields
		return jsonResp(200, firestoreDoc(base+rest, fields))
	case method == "DELETE" && !isColl:
		delete(db.Docs, rest)
		return HTTPResponse{Status: 200, Body: "{}"}
	}
	return firestoreErr(400, "INVALID_ARGUMENT", "unsupported request "+method+" "+path)
}

func jsonResp(status int, v any) HTTPResponse {
	b, _ := json.MarshalIndent(v, "", "  ")
	return HTTPResponse{Status: status, Body: string(b)}
}

func firestoreErr(code int, status, msg string) HTTPResponse {
	return jsonResp(code, map[string]any{"error": map[string]any{"code": code, "message": msg, "status": status}})
}

// firestoreFields decodes {"fields": {"k": {"stringValue": "v"}}} into plain values.
func firestoreFields(body string) (map[string]any, error) {
	var in struct {
		Fields map[string]map[string]any `json:"fields"`
	}
	if strings.TrimSpace(body) == "" {
		return map[string]any{}, nil
	}
	if err := json.Unmarshal([]byte(body), &in); err != nil {
		return nil, fmt.Errorf("Invalid JSON payload received. %v", err)
	}
	out := map[string]any{}
	for k, typed := range in.Fields {
		for t, v := range typed {
			switch t {
			case "integerValue":
				n, _ := strconv.ParseInt(fmt.Sprint(v), 10, 64)
				out[k] = n
			case "stringValue", "doubleValue", "booleanValue", "timestampValue":
				out[k] = v
			case "nullValue":
				out[k] = nil
			default:
				return nil, fmt.Errorf("Invalid value at 'document.fields[%s]': unsupported type %s", k, t)
			}
		}
	}
	return out, nil
}

func firestoreDoc(name string, fields map[string]any) map[string]any {
	enc := map[string]any{}
	for k, v := range fields {
		switch x := v.(type) {
		case string:
			enc[k] = map[string]any{"stringValue": x}
		case bool:
			enc[k] = map[string]any{"booleanValue": x}
		case int64:
			enc[k] = map[string]any{"integerValue": strconv.FormatInt(x, 10)}
		case float64:
			if x == float64(int64(x)) {
				enc[k] = map[string]any{"integerValue": strconv.FormatInt(int64(x), 10)}
			} else {
				enc[k] = map[string]any{"doubleValue": x}
			}
		case nil:
			enc[k] = map[string]any{"nullValue": nil}
		default:
			enc[k] = map[string]any{"stringValue": fmt.Sprint(x)}
		}
	}
	return map[string]any{"name": name, "fields": enc, "createTime": "2026-09-01T09:00:00.000000Z", "updateTime": "2026-09-01T09:00:00.000000Z"}
}

// DataflowDrain processes what is left in the input before a streaming job stops.
func (s *State) DataflowDrain(project string, j *DataflowJob) {
	s.dataflowStep(project, j)
}
