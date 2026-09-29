package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
	"gopkg.in/yaml.v3"
)

// Cloud Functions, Cloud Scheduler, Cloud Tasks and App Engine.

var functionRuntimes = map[string]bool{
	"python312": true, "python311": true, "python310": true, "nodejs22": true, "nodejs20": true, "nodejs18": true,
	"go122": true, "go121": true, "java21": true, "java17": true, "dotnet8": true, "ruby33": true, "php83": true,
}

func regionCodeOf(region string) string {
	if c, ok := regionCode[region]; ok {
		return c
	}
	return "uc"
}

func init() {
	fnRes := func(p *sim.Project, fn *sim.Function) sim.Resource {
		return sim.Resource{Project: p.ID, Type: "cloudfunctions.googleapis.com/CloudFunction", Name: "projects/" + p.ID + "/locations/" + fn.Region + "/functions/" + fn.Name, Service: "cloudfunctions.googleapis.com", Policies: []*sim.Policy{&fn.IAM}}
	}
	getFn := func(c *Cmd) (*sim.Project, *sim.Function, error) {
		p, err := c.P()
		if err != nil {
			return nil, nil, err
		}
		p.EnsureServices()
		n, err := c.Arg(0, "NAME")
		if err != nil {
			return nil, nil, err
		}
		n = n[strings.LastIndex(n, "/")+1:]
		fn := p.Functions[n]
		if fn == nil || (c.Has("region") && c.Str("region", "") != fn.Region) {
			return nil, nil, fmt.Errorf("NOT_FOUND: Resource 'projects/%s/locations/%s/functions/%s' was not found", p.ID, c.Str("region", c.S.Region), n)
		}
		return p, fn, nil
	}

	// ---- Cloud Functions ------------------------------------------------------
	reg("functions deploy", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		p.EnsureServices()
		if err := c.API("cloudfunctions.googleapis.com"); err != nil {
			return nil, err
		}
		n, err := c.Arg(0, "NAME")
		if err != nil {
			return nil, err
		}
		region := c.Str("region", c.S.Region)
		fn := p.Functions[n]
		isNew := fn == nil
		gen2 := c.Bool("gen2") || !c.Bool("no-gen2")
		if gen2 {
			if err := c.API("run.googleapis.com"); err != nil {
				return nil, err
			}
			if err := c.API("cloudbuild.googleapis.com"); err != nil {
				return nil, err
			}
		}
		if isNew {
			runtime := c.Str("runtime", "")
			if runtime == "" {
				return nil, fmt.Errorf("argument --runtime: Must be specified for new functions (e.g. python312, nodejs20, go122).")
			}
			fn = &sim.Function{Name: n, Region: region, Gen2: gen2, Memory: "256Mi", Timeout: 60, MaxInstances: 100, Ingress: "ALLOW_ALL", Env: map[string]string{}, State: "ACTIVE"}
		}
		if v := c.Str("runtime", ""); v != "" {
			if !functionRuntimes[v] {
				return nil, fmt.Errorf("INVALID_ARGUMENT: Runtime %q is not supported. Supported runtimes include python312, nodejs20, go122, java21.", v)
			}
			fn.Runtime = v
		}
		switch {
		case c.Bool("trigger-http"):
			fn.Trigger = "http"
		case c.Has("trigger-topic"):
			t := c.Str("trigger-topic", "")
			t = t[strings.LastIndex(t, "/")+1:]
			if p.Topics[t] == nil {
				// Like gcloud, a missing topic is created.
				p.Topics[t] = &sim.Topic{Name: t}
			}
			fn.Trigger = "topic:" + t
		case c.Has("trigger-bucket"):
			b := strings.TrimPrefix(c.Str("trigger-bucket", ""), "gs://")
			if p.Buckets[b] == nil {
				return nil, fmt.Errorf("INVALID_ARGUMENT: Bucket [%s] not found", b)
			}
			fn.Trigger = "bucket:" + b
		case isNew:
			return nil, fmt.Errorf("You must specify a trigger when deploying a new function: --trigger-http, --trigger-topic or --trigger-bucket.")
		}
		if v := c.Str("entry-point", ""); v != "" {
			fn.EntryPoint = v
		} else if fn.EntryPoint == "" {
			fn.EntryPoint = n
		}
		src := c.Str("source", ".")
		fn.Source = src
		code := ""
		for _, cand := range []string{"main.py", "index.js", "function.go", "main.go", "app.js"} {
			path := strings.TrimSuffix(strings.TrimPrefix(src, "./"), "/")
			if path == "." || path == "" {
				path = cand
			} else {
				path += "/" + cand
			}
			if v, ok := c.S.Files[c.S.path(path)]; ok {
				code = v
				break
			}
		}
		if code != "" {
			if !strings.Contains(code, fn.EntryPoint) {
				return nil, fmt.Errorf("OperationError: code=3, message=Build failed: function %s is not defined in the source (entry point not found). Check --entry-point.", fn.EntryPoint)
			}
			fn.Response = sim.FunctionResponse(code)
		} else if fn.Response == "" {
			fn.Response = "Hello World!"
		}
		for k, v := range c.KV("set-env-vars") {
			fn.Env[k] = v
		}
		for k, v := range c.KV("update-env-vars") {
			fn.Env[k] = v
		}
		for _, k := range c.List("remove-env-vars") {
			delete(fn.Env, k)
		}
		if v := c.Str("memory", ""); v != "" {
			fn.Memory = v
		}
		if c.Has("timeout") {
			fn.Timeout, _ = strconv.Atoi(strings.TrimSuffix(c.Str("timeout", "60"), "s"))
		}
		if c.Has("min-instances") {
			fn.MinInstances = c.Int("min-instances", 0)
		}
		if c.Has("max-instances") {
			fn.MaxInstances = c.Int("max-instances", 100)
		}
		if v := c.Str("ingress-settings", ""); v != "" {
			fn.Ingress = strings.ToUpper(strings.ReplaceAll(v, "-", "_"))
		}
		if v := c.Str("service-account", ""); v != "" {
			sa, err := c.resolveSA(p, v, false)
			if err != nil {
				return nil, err
			}
			fn.SA = sa
		} else if fn.SA == "" {
			if gen2 {
				fn.SA = p.Number + "-compute@developer.gserviceaccount.com"
			} else {
				fn.SA = p.ID + "@appspot.gserviceaccount.com"
			}
		}
		if isNew {
			if err := c.NeedProject("cloudfunctions.functions.create"); err != nil {
				return nil, err
			}
		} else if err := c.Need("cloudfunctions.functions.update", fnRes(p, fn)); err != nil {
			return nil, err
		}
		if c.Bool("allow-unauthenticated") {
			fn.IAM.AddBinding(invokerRole(fn), "allUsers", nil)
		}
		if c.Bool("no-allow-unauthenticated") {
			fn.IAM.RemoveBinding(invokerRole(fn), "allUsers")
		}
		if fn.Trigger == "http" && fn.URL == "" {
			if fn.Gen2 {
				fn.URL = fmt.Sprintf("https://%s-%s-%s.a.run.app", n, c.S.State.ID(10), regionCodeOf(region))
			} else {
				fn.URL = fmt.Sprintf("https://%s-%s.cloudfunctions.net/%s", region, p.ID, n)
			}
		}
		fn.Version++
		fn.Updated = c.S.State.Now()
		p.Functions[n] = fn
		c.Audit("cloudfunctions.googleapis.com", "google.cloud.functions.v2.FunctionService.CreateFunction", "projects/"+p.ID+"/locations/"+region+"/functions/"+n)
		var b strings.Builder
		b.WriteString("Preparing function...done.\n")
		if fn.Gen2 {
			b.WriteString("OK Deploying function...\n  [Build] Logs are available at [https://console.cloud.google.com/cloud-build/builds]\n  [Service]\nDone.\n")
		} else {
			b.WriteString("Deploying function (may take a while - up to 2 minutes)...done.\n")
		}
		fmt.Fprintf(&b, "availableMemoryMb: %s\nbuildConfig:\n  entryPoint: %s\n  runtime: %s\n", fn.Memory, fn.EntryPoint, fn.Runtime)
		if fn.URL != "" {
			fmt.Fprintf(&b, "url: %s\n", fn.URL)
		}
		fmt.Fprintf(&b, "name: projects/%s/locations/%s/functions/%s\nstate: ACTIVE\n", p.ID, region, n)
		if fn.Trigger == "http" && !sim.FunctionAllowsUnauth(fn) && isNew && !c.Bool("allow-unauthenticated") {
			b.WriteString("WARNING: The function requires authentication. To allow public access run the command again with --allow-unauthenticated.\n")
		}
		return b.String(), nil
	})
	reg("functions list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		p.EnsureServices()
		var rows []any
		for _, k := range sim.SortedKeys(p.Functions) {
			fn := p.Functions[k]
			env := "1st gen"
			if fn.Gen2 {
				env = "2nd gen"
			}
			rows = append(rows, map[string]any{"name": fn.Name, "state": fn.State, "trigger": triggerText(fn), "region": fn.Region, "environment": env})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"STATE", "state"}, {"TRIGGER", "trigger"}, {"REGION", "region"}, {"ENVIRONMENT", "environment"}}, Rows: rows}, nil
	})
	reg("functions describe", func(c *Cmd) (any, error) {
		_, fn, err := getFn(c)
		if err != nil {
			return nil, err
		}
		return Obj{V: fn}, nil
	})
	reg("functions delete", func(c *Cmd) (any, error) {
		p, fn, err := getFn(c)
		if err != nil {
			return nil, err
		}
		if err := c.Need("cloudfunctions.functions.delete", fnRes(p, fn)); err != nil {
			return nil, err
		}
		delete(p.Functions, fn.Name)
		c.Audit("cloudfunctions.googleapis.com", "google.cloud.functions.v2.FunctionService.DeleteFunction", "projects/"+p.ID+"/locations/"+fn.Region+"/functions/"+fn.Name)
		return "Deleted [projects/" + p.ID + "/locations/" + fn.Region + "/functions/" + fn.Name + "].\n", nil
	})
	reg("functions call", func(c *Cmd) (any, error) {
		p, fn, err := getFn(c)
		if err != nil {
			return nil, err
		}
		if err := c.Need("cloudfunctions.functions.invoke", fnRes(p, fn)); err != nil {
			return nil, err
		}
		st, body := c.S.State.InvokeFunction(p.ID, fn, "call", c.Str("data", "{}"))
		if st >= 400 {
			return nil, fmt.Errorf("ResponseError: status=[%d], code=[%s]", st, body)
		}
		return fmt.Sprintf("executionId: %s\nresult: %s\n", c.S.State.ID(12), body), nil
	})
	reg("functions logs read", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		name := ""
		if len(c.Args) > 0 {
			name = c.Args[0]
		}
		var b strings.Builder
		b.WriteString("LEVEL  NAME      TIME_UTC             LOG\n")
		entries := c.S.State.QueryLogs(p.ID, `logName:"cloud-functions"`, c.Int("limit", 20))
		for _, e := range entries {
			fnName := e.Resource.Labels["function_name"]
			if name != "" && fnName != name {
				continue
			}
			fmt.Fprintf(&b, "%-6s %-9s %s  %s\n", e.Severity[:1], fnName, strings.Replace(strings.TrimSuffix(e.Timestamp, "Z"), "T", " ", 1), e.Text)
		}
		return b.String(), nil
	})
	fnIAM := func(add bool) handler {
		return func(c *Cmd) (any, error) {
			p, fn, err := getFn(c)
			if err != nil {
				return nil, err
			}
			m, err := member(c.Str("member", ""))
			if err != nil {
				return nil, err
			}
			if err := c.Need("cloudfunctions.functions.setIamPolicy", fnRes(p, fn)); err != nil {
				return nil, err
			}
			role := c.Str("role", "")
			if add {
				fn.IAM.AddBinding(role, m, nil)
			} else {
				fn.IAM.RemoveBinding(role, m)
			}
			return renderPolicy(fn.IAM, "function "+fn.Name), nil
		}
	}
	reg("functions add-iam-policy-binding", fnIAM(true))
	reg("functions remove-iam-policy-binding", fnIAM(false))
	reg("functions add-invoker-policy-binding", func(c *Cmd) (any, error) {
		c.F["role"] = []string{"roles/run.invoker"}
		return registry["functions add-iam-policy-binding"](c)
	})

	// ---- Cloud Scheduler ---------------------------------------------------------
	schedLoc := func(c *Cmd) string { return c.Str("location", c.S.Region) }
	getJob := func(c *Cmd) (*sim.Project, *sim.SchedulerJob, error) {
		p, err := c.P()
		if err != nil {
			return nil, nil, err
		}
		p.EnsureServices()
		n, err := c.Arg(0, "JOB")
		if err != nil {
			return nil, nil, err
		}
		j := p.SchedulerJobs[n[strings.LastIndex(n, "/")+1:]]
		if j == nil {
			return nil, nil, fmt.Errorf("NOT_FOUND: Job not found: projects/%s/locations/%s/jobs/%s", p.ID, schedLoc(c), n)
		}
		return p, j, nil
	}
	createJob := func(target string) handler {
		return func(c *Cmd) (any, error) {
			p, err := c.P()
			if err != nil {
				return nil, err
			}
			p.EnsureServices()
			if err := c.API("cloudscheduler.googleapis.com"); err != nil {
				return nil, err
			}
			n, err := c.Arg(0, "JOB")
			if err != nil {
				return nil, err
			}
			if p.SchedulerJobs[n] != nil {
				return nil, fmt.Errorf("ALREADY_EXISTS: Job projects/%s/locations/%s/jobs/%s already exists.", p.ID, schedLoc(c), n)
			}
			sched := c.Str("schedule", "")
			if len(strings.Fields(sched)) != 5 {
				return nil, fmt.Errorf("INVALID_ARGUMENT: Schedule has more or less than 5 fields (unix-cron format, e.g. \"*/5 * * * *\").")
			}
			if err := c.NeedProject("cloudscheduler.jobs.create"); err != nil {
				return nil, err
			}
			j := &sim.SchedulerJob{Name: n, Location: schedLoc(c), Schedule: sched, TimeZone: c.Str("time-zone", "Etc/UTC"), Target: target, State: "ENABLED"}
			if target == "pubsub" {
				t := c.Str("topic", "")
				t = t[strings.LastIndex(t, "/")+1:]
				if p.Topics[t] == nil {
					return nil, fmt.Errorf("NOT_FOUND: Topic projects/%s/topics/%s not found.", p.ID, t)
				}
				j.Topic, j.Message = t, c.Str("message-body", "")
				if j.Message == "" {
					return nil, fmt.Errorf("argument --message-body: Must be specified.")
				}
			} else {
				j.URI, j.Method = c.Str("uri", ""), strings.ToUpper(c.Str("http-method", "POST"))
				if j.URI == "" {
					return nil, fmt.Errorf("argument --uri: Must be specified.")
				}
				j.OIDC = c.Str("oidc-service-account-email", "")
			}
			p.SchedulerJobs[n] = j
			c.Audit("cloudscheduler.googleapis.com", "google.cloud.scheduler.v1.CloudScheduler.CreateJob", "projects/"+p.ID+"/locations/"+j.Location+"/jobs/"+n)
			return fmt.Sprintf("name: projects/%s/locations/%s/jobs/%s\nschedule: '%s'\nstate: ENABLED\ntimeZone: %s\n", p.ID, j.Location, n, sched, j.TimeZone), nil
		}
	}
	reg("scheduler jobs create http", createJob("http"))
	reg("scheduler jobs create pubsub", createJob("pubsub"))
	reg("scheduler jobs list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		p.EnsureServices()
		var rows []any
		for _, k := range sim.SortedKeys(p.SchedulerJobs) {
			j := p.SchedulerJobs[k]
			tgt := "HTTP"
			if j.Target == "pubsub" {
				tgt = "Pub/Sub"
			}
			rows = append(rows, map[string]any{"id": j.Name, "location": j.Location, "schedule": j.Schedule + " (" + j.TimeZone + ")", "target": tgt, "state": j.State})
		}
		return Table{Cols: []Col{{"ID", "id"}, {"LOCATION", "location"}, {"SCHEDULE (TZ)", "schedule"}, {"TARGET_TYPE", "target"}, {"STATE", "state"}}, Rows: rows}, nil
	})
	reg("scheduler jobs describe", func(c *Cmd) (any, error) {
		_, j, err := getJob(c)
		if err != nil {
			return nil, err
		}
		return Obj{V: j}, nil
	})
	reg("scheduler jobs run", func(c *Cmd) (any, error) {
		p, j, err := getJob(c)
		if err != nil {
			return nil, err
		}
		c.S.State.RunSchedulerJob(p.ID, j)
		return "", nil
	})
	jobState := func(state string) handler {
		return func(c *Cmd) (any, error) {
			_, j, err := getJob(c)
			if err != nil {
				return nil, err
			}
			j.State = state
			if state == "PAUSED" {
				return "Job has been paused.\n", nil
			}
			return "Job has been resumed.\n", nil
		}
	}
	reg("scheduler jobs pause", jobState("PAUSED"))
	reg("scheduler jobs resume", jobState("ENABLED"))
	reg("scheduler jobs update http", func(c *Cmd) (any, error) {
		_, j, err := getJob(c)
		if err != nil {
			return nil, err
		}
		if v := c.Str("schedule", ""); v != "" {
			j.Schedule = v
		}
		if v := c.Str("uri", ""); v != "" {
			j.URI = v
		}
		return "Updated job [" + j.Name + "].\n", nil
	})
	reg("scheduler jobs update pubsub", func(c *Cmd) (any, error) {
		_, j, err := getJob(c)
		if err != nil {
			return nil, err
		}
		if v := c.Str("schedule", ""); v != "" {
			j.Schedule = v
		}
		if v := c.Str("message-body", ""); v != "" {
			j.Message = v
		}
		return "Updated job [" + j.Name + "].\n", nil
	})
	reg("scheduler jobs delete", func(c *Cmd) (any, error) {
		p, j, err := getJob(c)
		if err != nil {
			return nil, err
		}
		delete(p.SchedulerJobs, j.Name)
		return "Deleted job [" + j.Name + "].\n", nil
	})

	// ---- Cloud Tasks ----------------------------------------------------------------
	getQueue := func(c *Cmd, name string) (*sim.Project, *sim.TaskQueue, error) {
		p, err := c.P()
		if err != nil {
			return nil, nil, err
		}
		p.EnsureServices()
		q := p.TaskQueues[name[strings.LastIndex(name, "/")+1:]]
		if q == nil {
			return nil, nil, fmt.Errorf("NOT_FOUND: Requested entity was not found. (queue %s)", name)
		}
		return p, q, nil
	}
	reg("tasks queues create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		p.EnsureServices()
		if err := c.API("cloudtasks.googleapis.com"); err != nil {
			return nil, err
		}
		n, err := c.Arg(0, "QUEUE")
		if err != nil {
			return nil, err
		}
		if p.TaskQueues[n] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: The queue already exists.")
		}
		q := &sim.TaskQueue{Name: n, Location: c.Str("location", c.S.Region), State: "RUNNING", MaxDispatches: 500, MaxAttempts: c.Int("max-attempts", 100)}
		if c.Has("max-dispatches-per-second") {
			q.MaxDispatches, _ = strconv.ParseFloat(c.Str("max-dispatches-per-second", "500"), 64)
		}
		p.TaskQueues[n] = q
		return "Created queue [projects/" + p.ID + "/locations/" + q.Location + "/queues/" + n + "].\n", nil
	})
	reg("tasks queues list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		p.EnsureServices()
		var rows []any
		for _, k := range sim.SortedKeys(p.TaskQueues) {
			q := p.TaskQueues[k]
			rows = append(rows, map[string]any{"id": q.Name, "state": q.State, "tasks": len(q.Tasks), "rate": q.MaxDispatches, "attempts": q.MaxAttempts})
		}
		return Table{Cols: []Col{{"QUEUE_NAME", "id"}, {"STATE", "state"}, {"TASKS", "tasks"}, {"MAX_RATE (/sec)", "rate"}, {"MAX_ATTEMPTS", "attempts"}}, Rows: rows}, nil
	})
	reg("tasks queues describe", func(c *Cmd) (any, error) {
		n, _ := c.Arg(0, "QUEUE")
		_, q, err := getQueue(c, n)
		if err != nil {
			return nil, err
		}
		return Obj{V: q}, nil
	})
	queueState := func(state, msg string) handler {
		return func(c *Cmd) (any, error) {
			n, _ := c.Arg(0, "QUEUE")
			_, q, err := getQueue(c, n)
			if err != nil {
				return nil, err
			}
			q.State = state
			return msg + " queue [" + q.Name + "].\n", nil
		}
	}
	reg("tasks queues pause", queueState("PAUSED", "Paused"))
	reg("tasks queues resume", queueState("RUNNING", "Resumed"))
	reg("tasks queues purge", func(c *Cmd) (any, error) {
		n, _ := c.Arg(0, "QUEUE")
		_, q, err := getQueue(c, n)
		if err != nil {
			return nil, err
		}
		q.Tasks = nil
		return "Purged queue [" + q.Name + "].\n", nil
	})
	reg("tasks queues delete", func(c *Cmd) (any, error) {
		n, _ := c.Arg(0, "QUEUE")
		p, q, err := getQueue(c, n)
		if err != nil {
			return nil, err
		}
		delete(p.TaskQueues, q.Name)
		return "Deleted queue [" + q.Name + "].\n", nil
	})
	reg("tasks create-http-task", func(c *Cmd) (any, error) {
		_, q, err := getQueue(c, c.Str("queue", ""))
		if err != nil {
			return nil, err
		}
		url := c.Str("url", "")
		if url == "" {
			return nil, fmt.Errorf("argument --url: Must be specified.")
		}
		name := ""
		if len(c.Args) > 0 {
			name = c.Args[0]
		} else {
			name = c.S.State.ID(18)
		}
		q.Tasks = append(q.Tasks, &sim.CloudTask{Name: name, URL: url, Method: strings.ToUpper(c.Str("method", "POST")), Body: c.Str("body-content", ""), Created: c.S.State.Now()})
		return "Created task [" + name + "].\n", nil
	})
	reg("tasks list", func(c *Cmd) (any, error) {
		_, q, err := getQueue(c, c.Str("queue", ""))
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, t := range q.Tasks {
			rows = append(rows, map[string]any{"name": t.Name, "type": "HTTP", "created": t.Created, "attempts": t.Attempts, "last": t.Status})
		}
		return Table{Cols: []Col{{"TASK_NAME", "name"}, {"TYPE", "type"}, {"CREATE_TIME", "created"}, {"DISPATCH_ATTEMPTS", "attempts"}, {"LAST_RESPONSE", "last"}}, Rows: rows}, nil
	})

	// ---- App Engine -----------------------------------------------------------------
	getApp := func(c *Cmd) (*sim.Project, *sim.AppEngineApp, error) {
		p, err := c.P()
		if err != nil {
			return nil, nil, err
		}
		if p.AppEngine == nil {
			return nil, nil, fmt.Errorf("The current Google Cloud project [%s] does not contain an App Engine application. Use `gcloud app create` to initialize an App Engine application within the project.", p.ID)
		}
		return p, p.AppEngine, nil
	}
	reg("app create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		if err := c.API("appengine.googleapis.com"); err != nil {
			return nil, err
		}
		if p.AppEngine != nil {
			return nil, fmt.Errorf("The project [%s] already contains an App Engine application in region [%s]. You can deploy your application using `gcloud app deploy`. (The region of an App Engine application cannot be changed.)", p.ID, p.AppEngine.Region)
		}
		region := c.Str("region", "")
		if region == "" {
			return nil, fmt.Errorf("argument --region: Must be specified (e.g. europe-west, us-central). The region cannot be changed later.")
		}
		if err := c.NeedProject("appengine.applications.create"); err != nil {
			return nil, err
		}
		code := map[string]string{"europe-west": "ew", "europe-west1": "ew", "us-central": "uc", "us-east1": "ue", "europe-west3": "ey", "europe-southwest1": "no"}[region]
		if code == "" {
			code = "uc"
		}
		p.AppEngine = &sim.AppEngineApp{Region: region, Host: p.ID + "." + code + ".r.appspot.com", Status: "SERVING", Services: map[string]*sim.AEService{}}
		sa := p.ID + "@appspot.gserviceaccount.com"
		p.ServiceAccounts[sa] = &sim.ServiceAccount{Email: sa, DisplayName: "App Engine default service account"}
		c.Audit("appengine.googleapis.com", "google.appengine.v1.Applications.CreateApplication", "apps/"+p.ID)
		return fmt.Sprintf("Creating App Engine application in project [%s] and region [%s]....done.\nSuccess! The app is now created. Please use `gcloud app deploy` to deploy your first app.\n", p.ID, region), nil
	})
	reg("app describe", func(c *Cmd) (any, error) {
		_, app, err := getApp(c)
		if err != nil {
			return nil, err
		}
		return Obj{V: map[string]any{"defaultHostname": app.Host, "locationId": app.Region, "servingStatus": app.Status, "id": c.ProjectID()}}, nil
	})
	reg("app deploy", func(c *Cmd) (any, error) {
		p, app, err := getApp(c)
		if err != nil {
			return nil, err
		}
		file := "app.yaml"
		if len(c.Args) > 0 {
			file = c.Args[0]
		}
		raw, ok := c.S.Files[c.S.path(file)]
		if !ok {
			return nil, fmt.Errorf("An app.yaml (or appengine-web.xml) file is required to deploy this directory as an App Engine application. (%s not found; create it in Cloud Shell)", file)
		}
		var spec struct {
			Runtime       string         `yaml:"runtime"`
			Service       string         `yaml:"service"`
			Env           string         `yaml:"env"`
			InstanceClass string         `yaml:"instance_class"`
			AutoScaling   map[string]any `yaml:"automatic_scaling"`
			ManualScaling map[string]any `yaml:"manual_scaling"`
			BasicScaling  map[string]any `yaml:"basic_scaling"`
			EnvVariables  map[string]any `yaml:"env_variables"`
		}
		if err := yaml.Unmarshal([]byte(raw), &spec); err != nil {
			return nil, fmt.Errorf("An error occurred while parsing file: [%s]: %v", file, err)
		}
		if spec.Runtime == "" {
			return nil, fmt.Errorf("An error occurred while parsing file: [%s]: runtime is required", file)
		}
		svcName := spec.Service
		if svcName == "" {
			svcName = "default"
		}
		if svcName != "default" && app.Services["default"] == nil {
			return nil, fmt.Errorf("INVALID_ARGUMENT: The first service (module) you upload to a new application must be the 'default' service (module).")
		}
		if err := c.NeedProject("appengine.versions.create"); err != nil {
			return nil, err
		}
		svc := app.Services[svcName]
		if svc == nil {
			svc = &sim.AEService{Name: svcName, Versions: map[string]*sim.AEVersion{}, Split: map[string]float64{}}
			app.Services[svcName] = svc
		}
		vid := c.Str("version", "")
		if vid == "" {
			vid = strings.ReplaceAll(strings.ReplaceAll(strings.SplitN(c.S.State.Now(), "+", 2)[0], "-", ""), ":", "")
			vid = strings.ToLower(strings.ReplaceAll(strings.TrimSuffix(vid, "Z"), "T", "t"))
		}
		env := "standard"
		if spec.Env == "flex" || spec.Env == "flexible" {
			env = "flexible"
		}
		scaling := "automatic"
		if spec.ManualScaling != nil {
			scaling = "manual"
		} else if spec.BasicScaling != nil {
			scaling = "basic"
		}
		resp := fmt.Sprintf("Hello from App Engine (%s, version %s)", svcName, vid)
		dir := ""
		if i := strings.LastIndex(file, "/"); i >= 0 {
			dir = file[:i+1]
		}
		for _, f := range []string{"main.py", "app.js", "index.js", "main.go"} {
			if code, ok := c.S.Files[c.S.path(dir+f)]; ok {
				resp = sim.FunctionResponse(code)
				break
			}
		}
		svc.Versions[vid] = &sim.AEVersion{ID: vid, Runtime: spec.Runtime, Env: env, Status: "SERVING", Created: c.S.State.Now(), Scaling: scaling, Response: resp}
		promote := !c.Bool("no-promote")
		if promote {
			svc.Split = map[string]float64{vid: 1}
		}
		c.Audit("appengine.googleapis.com", "google.appengine.v1.Versions.CreateVersion", "apps/"+p.ID+"/services/"+svcName+"/versions/"+vid)
		url := "https://" + app.Host
		if svcName != "default" {
			url = "https://" + svcName + "-dot-" + app.Host
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Services to deploy:\n\ndescriptor:                  [/home/student/%s]\nsource:                      [/home/student]\ntarget project:              [%s]\ntarget service:              [%s]\ntarget version:              [%s]\ntarget url:                  [%s]\n\n", file, p.ID, svcName, vid, url)
		b.WriteString("Beginning deployment of service [" + svcName + "]...\nUpdating service [" + svcName + "]...done.\n")
		if promote {
			b.WriteString("Setting traffic split for service [" + svcName + "]...done.\n")
		} else {
			b.WriteString("The version was deployed without receiving traffic (--no-promote). Use `gcloud app services set-traffic` to migrate it.\n")
		}
		fmt.Fprintf(&b, "Deployed service [%s] to [%s]\n", svcName, url)
		return b.String(), nil
	})
	reg("app browse", func(c *Cmd) (any, error) {
		_, app, err := getApp(c)
		if err != nil {
			return nil, err
		}
		return "Did not detect your browser. Go to this link to view your app:\nhttps://" + app.Host + "\n", nil
	})
	reg("app services list", func(c *Cmd) (any, error) {
		_, app, err := getApp(c)
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(app.Services) {
			rows = append(rows, map[string]any{"service": k, "versions": len(app.Services[k].Versions)})
		}
		return Table{Cols: []Col{{"SERVICE", "service"}, {"NUM_VERSIONS", "versions"}}, Rows: rows}, nil
	})
	reg("app versions list", func(c *Cmd) (any, error) {
		_, app, err := getApp(c)
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, sk := range sim.SortedKeys(app.Services) {
			svc := app.Services[sk]
			if c.Has("service") && c.Str("service", "") != sk {
				continue
			}
			for _, vk := range sim.SortedKeys(svc.Versions) {
				v := svc.Versions[vk]
				rows = append(rows, map[string]any{"service": sk, "version": vk, "split": fmt.Sprintf("%.2f", svc.Split[vk]), "created": v.Created, "status": v.Status})
			}
		}
		return Table{Cols: []Col{{"SERVICE", "service"}, {"VERSION.ID", "version"}, {"TRAFFIC_SPLIT", "split"}, {"LAST_DEPLOYED", "created"}, {"SERVING_STATUS", "status"}}, Rows: rows}, nil
	})
	reg("app services set-traffic", func(c *Cmd) (any, error) {
		_, app, err := getApp(c)
		if err != nil {
			return nil, err
		}
		name := "default"
		if len(c.Args) > 0 {
			name = c.Args[0]
		}
		svc := app.Services[name]
		if svc == nil {
			return nil, fmt.Errorf("Service [%s] not found.", name)
		}
		splits := c.KV("splits")
		if len(splits) == 0 {
			return nil, fmt.Errorf("argument --splits: Must be specified (e.g. --splits=v1=0.5,v2=0.5).")
		}
		total := 0.0
		next := map[string]float64{}
		for v, w := range splits {
			if svc.Versions[v] == nil {
				return nil, fmt.Errorf("INVALID_ARGUMENT: Version [%s] does not exist in service [%s].", v, name)
			}
			f, err := strconv.ParseFloat(w, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid split %q", w)
			}
			next[v] = f
			total += f
		}
		if total <= 0 {
			return nil, fmt.Errorf("INVALID_ARGUMENT: splits must add up to a positive value")
		}
		for v := range next {
			next[v] /= total
		}
		svc.Split = next
		svc.SplitBy = strings.ToLower(c.Str("split-by", "random"))
		keys := make([]string, 0, len(next))
		for k := range next {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString("Setting the following traffic allocation:\n")
		for _, k := range keys {
			fmt.Fprintf(&b, " - %s/%s/%s: %.2f\n", c.ProjectID(), name, k, next[k])
		}
		b.WriteString("Setting traffic split for service [" + name + "]...done.\n")
		return b.String(), nil
	})
	versionState := func(state string) handler {
		return func(c *Cmd) (any, error) {
			_, app, err := getApp(c)
			if err != nil {
				return nil, err
			}
			svc := app.Services[c.Str("service", "default")]
			if svc == nil {
				return nil, fmt.Errorf("Service not found.")
			}
			for _, v := range c.Args {
				ver := svc.Versions[v]
				if ver == nil {
					return nil, fmt.Errorf("Version [%s] not found.", v)
				}
				if state == "DELETE" {
					if svc.Split[v] > 0 {
						return nil, fmt.Errorf("The version [%s] is currently serving %.0f%% of traffic. Migrate traffic before deleting it.", v, svc.Split[v]*100)
					}
					delete(svc.Versions, v)
					continue
				}
				ver.Status = state
			}
			return "Done.\n", nil
		}
	}
	reg("app versions stop", versionState("STOPPED"))
	reg("app versions start", versionState("SERVING"))
	reg("app versions delete", versionState("DELETE"))
	reg("app services delete", func(c *Cmd) (any, error) {
		_, app, err := getApp(c)
		if err != nil {
			return nil, err
		}
		for _, n := range c.Args {
			if n == "default" {
				return nil, fmt.Errorf("The default service cannot be deleted.")
			}
			delete(app.Services, n)
		}
		return "Deleted service.\n", nil
	})
	reg("app logs tail", func(c *Cmd) (any, error) {
		p, _, err := getApp(c)
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		for _, e := range c.S.State.QueryLogs(p.ID, `resource.type="gae_app"`, 20) {
			if e.HTTP != nil {
				fmt.Fprintf(&b, "%s %s[%s]  \"%s %s\" %d\n", e.Timestamp, e.Resource.Labels["module_id"], e.Resource.Labels["version_id"], e.HTTP.Method, e.HTTP.URL, e.HTTP.Status)
			}
		}
		return b.String(), nil
	})
}

func invokerRole(fn *sim.Function) string {
	if fn.Gen2 {
		return "roles/run.invoker"
	}
	return "roles/cloudfunctions.invoker"
}

func triggerText(fn *sim.Function) string {
	switch {
	case fn.Trigger == "http":
		return "HTTP Trigger"
	case strings.HasPrefix(fn.Trigger, "topic:"):
		return "Event Trigger (Pub/Sub " + strings.TrimPrefix(fn.Trigger, "topic:") + ")"
	case strings.HasPrefix(fn.Trigger, "bucket:"):
		return "Event Trigger (Storage " + strings.TrimPrefix(fn.Trigger, "bucket:") + ")"
	}
	return fn.Trigger
}

func renderPolicy(pol sim.Policy, what string) string {
	var b strings.Builder
	b.WriteString("Updated IAM policy for " + what + ".\nbindings:\n")
	for _, bd := range pol.Bindings {
		b.WriteString("- members:\n")
		for _, m := range bd.Members {
			b.WriteString("  - " + m + "\n")
		}
		b.WriteString("  role: " + bd.Role + "\n")
	}
	return b.String()
}
