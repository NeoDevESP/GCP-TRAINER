package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
	"github.com/neodevesp/gcp-trainer/internal/sqlengine"
	"gopkg.in/yaml.v3"
)

var reARImage = regexp.MustCompile(`^([a-z0-9-]+)-docker\.pkg\.dev/([a-z0-9-]+)/([a-z0-9-]+)/([a-z0-9._/-]+?)(?::([A-Za-z0-9._-]+))?$`)

// pushImage stores an image tag in Artifact Registry.
func (s *Session) pushImage(principal, ref, behavior string) error {
	m := reARImage.FindStringSubmatch(ref)
	if m == nil {
		if strings.HasPrefix(ref, "gcr.io/") || strings.Contains(ref, ".gcr.io/") {
			return fmt.Errorf("denied: Container Registry is shut down for this project. Push to Artifact Registry (REGION-docker.pkg.dev/PROJECT/REPO/IMAGE) instead.")
		}
		return fmt.Errorf("denied: unsupported registry for %s (use Artifact Registry)", ref)
	}
	p := s.State.Projects[m[2]]
	if p == nil {
		return fmt.Errorf("denied: project %s not found", m[2])
	}
	r := p.ArtifactRepos[m[3]]
	if r == nil {
		return fmt.Errorf("name unknown: Repository \"%s\" not found", m[3])
	}
	if r.Location != m[1] {
		return fmt.Errorf("name unknown: Repository \"%s\" not found in location %s (it is in %s)", m[3], m[1], r.Location)
	}
	if !s.State.Allowed(principal, "artifactregistry.repositories.uploadArtifacts", sim.Resource{Project: p.ID, Type: "artifactregistry.googleapis.com/Repository", Name: "projects/" + p.ID + "/locations/" + r.Location + "/repositories/" + r.Name, Service: "artifactregistry.googleapis.com"}) {
		return fmt.Errorf("denied: Permission \"artifactregistry.repositories.uploadArtifacts\" denied on resource \"projects/%s/locations/%s/repositories/%s\" (or it may not exist)", p.ID, r.Location, r.Name)
	}
	tag := m[5]
	if tag == "" {
		tag = "latest"
	}
	img := m[4]
	if !contains(r.Images[img], tag) {
		r.Images[img] = append(r.Images[img], tag)
	}
	full := fmt.Sprintf("%s-docker.pkg.dev/%s/%s/%s:%s", m[1], m[2], m[3], img, tag)
	if behavior != "" {
		s.State.Extra["image:"+full] = behavior
		if m[5] == "" {
			s.State.Extra["image:"+strings.TrimSuffix(full, ":latest")] = behavior
		}
	}
	s.State.Audit(p.ID, principal, "artifactregistry.googleapis.com", "Docker-PutManifest", "projects/"+p.ID+"/locations/"+r.Location+"/repositories/"+r.Name+"/dockerImages/"+img)
	return nil
}

type buildStep struct {
	Name       string   `yaml:"name"`
	Args       []string `yaml:"args"`
	ID         string   `yaml:"id"`
	Entrypoint string   `yaml:"entrypoint"`
	Script     string   `yaml:"script"`
}

type buildConfig struct {
	Steps         []buildStep       `yaml:"steps"`
	Images        []string          `yaml:"images"`
	Substitutions map[string]string `yaml:"substitutions"`
	ServiceAcct   string            `yaml:"serviceAccount"`
	Options       map[string]any    `yaml:"options"`
}

// runBuild executes a Cloud Build over a set of source files.
func (s *Session) runBuild(p *sim.Project, files map[string]string, tag, config, trigger, sha string) (*sim.Build, error) {
	if sha == "" {
		sha = s.State.ID(40)
	}
	b := &sim.Build{ID: s.State.ID(8) + "-" + s.State.ID(4) + "-" + s.State.ID(4), Status: "SUCCESS", Source: "local", Trigger: trigger, Commit: sha, Created: s.State.Now()}
	sa := p.Number + "@cloudbuild.gserviceaccount.com"
	if t := p.Triggers[trigger]; t != nil && t.SA != "" {
		sa = t.SA
	}
	b.SA = sa
	principal := "serviceAccount:" + sa
	if sa == p.Number+"@cloudbuild.gserviceaccount.com" && !p.IAM.HasMember("roles/cloudbuild.builds.builder", principal) {
		p.IAM.AddBinding("roles/cloudbuild.builds.builder", principal, nil)
	}
	logf := func(f string, a ...any) { b.Log = append(b.Log, fmt.Sprintf(f, a...)) }
	logf("BUILD %s started (service account %s)", b.ID, sa)
	failBuild := func(f string, a ...any) (*sim.Build, error) {
		b.Status = "FAILURE"
		logf("ERROR: "+f, a...)
		p.Builds = append(p.Builds, b)
		s.State.Log(p.ID, sim.LogEntry{Severity: "ERROR", LogName: "cloudbuild", Resource: sim.LogResource{Type: "build", Labels: map[string]string{"build_id": b.ID, "build_trigger_id": trigger}}, Text: b.Log[len(b.Log)-1]})
		return b, nil
	}
	df, hasDocker := files["Dockerfile"]
	if tag != "" {
		if !hasDocker {
			return nil, fmt.Errorf("Dockerfile required when specifying --tag")
		}
		logf("Step #0: docker build -t %s .", tag)
		logf("Step #0: Successfully built and tagged %s", tag)
		if err := s.pushImage(principal, tag, behaviorFromDockerfile(df)); err != nil {
			return failBuild("pushing %s: %v", tag, err)
		}
		logf("PUSH %s", tag)
		b.Images = []string{tag}
		b.Steps = []string{"docker build", "docker push"}
		p.Builds = append(p.Builds, b)
		return b, nil
	}
	if config == "" {
		config = "cloudbuild.yaml"
	}
	content, ok := files[config]
	if !ok {
		return nil, fmt.Errorf("Unable to read file [%s]: build config not found (pass --tag or --config)", config)
	}
	var cfg buildConfig
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return nil, fmt.Errorf("invalid build config: %v", err)
	}
	subst := map[string]string{"PROJECT_ID": p.ID, "SHORT_SHA": sha[:7], "COMMIT_SHA": sha, "BUILD_ID": b.ID, "BRANCH_NAME": "main", "LOCATION": "global"}
	for k, v := range cfg.Substitutions {
		subst[k] = v
	}
	expand := func(v string) string {
		return regexp.MustCompile(`\$\{?([A-Z_][A-Z0-9_]*)\}?`).ReplaceAllStringFunc(v, func(m string) string {
			k := strings.Trim(m, "${}")
			if x, ok := subst[k]; ok {
				return x
			}
			return m
		})
	}
	built := map[string]string{}
	for i, st := range cfg.Steps {
		args := make([]string, len(st.Args))
		for j, a := range st.Args {
			args[j] = expand(a)
		}
		name := st.Name
		b.Steps = append(b.Steps, name+" "+strings.Join(args, " "))
		logf("Step #%d%s: %s %s", i, map[bool]string{true: " - \"" + st.ID + "\"", false: ""}[st.ID != ""], name, strings.Join(args, " "))
		switch {
		case strings.Contains(name, "cloud-builders/docker") || name == "docker":
			if len(args) == 0 {
				continue
			}
			switch args[0] {
			case "build":
				if !hasDocker {
					return failBuild("Step #%d: unable to prepare context: unable to evaluate symlinks in Dockerfile path: lstat /workspace/Dockerfile: no such file or directory", i)
				}
				for j := 0; j < len(args)-1; j++ {
					if args[j] == "-t" || args[j] == "--tag" {
						built[args[j+1]] = behaviorFromDockerfile(df)
					}
				}
				logf("Step #%d: Successfully built", i)
			case "push":
				ref := args[len(args)-1]
				if _, ok := built[ref]; !ok {
					return failBuild("Step #%d: An image does not exist locally with the tag: %s", i, ref)
				}
				if err := s.pushImage(principal, ref, built[ref]); err != nil {
					return failBuild("Step #%d: %v", i, err)
				}
				b.Images = append(b.Images, ref)
			}
		case strings.Contains(name, "cloud-sdk") || strings.Contains(name, "gcloud") || st.Entrypoint == "gcloud":
			gargs := args
			if len(gargs) > 0 && gargs[0] == "gcloud" {
				gargs = gargs[1:]
			}
			sub := NewSession(s.State, p.ID, sa)
			sub.NoTick = true
			sub.Region = s.Region
			for k, v := range files {
				sub.Files[k] = v
			}
			out, err := sub.gcloud(gargs, "")
			for _, l := range splitLines(out) {
				logf("Step #%d: %s", i, l)
			}
			if err != nil {
				return failBuild("Step #%d: %v", i, err)
			}
		default:
			// test / lint steps: fail when the source declares failing tests
			if strings.Contains(files["TESTS_STATUS"], "fail") && (strings.Contains(strings.Join(args, " "), "test") || strings.Contains(st.Script, "test")) {
				return failBuild("Step #%d: tests failed (see TESTS_STATUS)", i)
			}
			logf("Step #%d: PASS", i)
		}
	}
	for _, img := range cfg.Images {
		img = expand(img)
		if _, ok := built[img]; !ok {
			return failBuild("image %s listed in images was not built", img)
		}
		if !contains(b.Images, img) {
			if err := s.pushImage(principal, img, built[img]); err != nil {
				return failBuild("%v", err)
			}
			b.Images = append(b.Images, img)
		}
	}
	logf("DONE")
	p.Builds = append(p.Builds, b)
	s.State.Log(p.ID, sim.LogEntry{Severity: "INFO", LogName: "cloudbuild", Resource: sim.LogResource{Type: "build", Labels: map[string]string{"build_id": b.ID, "build_trigger_id": trigger}}, Text: "Build " + b.ID + " SUCCESS images=" + strings.Join(b.Images, ",")})
	return b, nil
}

func (s *Session) git(args []string) (string, error) {
	if len(args) == 0 {
		return "usage: git <command>\n", nil
	}
	g := &s.Git
	if g.Staged == nil {
		g.Staged = map[string]string{}
	}
	switch args[0] {
	case "init":
		g.Initialized, g.Branch = true, "main"
		return "Initialized empty Git repository in /home/student/.git/\n", nil
	case "config":
		return "", nil
	case "checkout", "switch":
		for _, a := range args[1:] {
			if !strings.HasPrefix(a, "-") {
				g.Branch = a
			}
		}
		return "Switched to branch '" + g.Branch + "'\n", nil
	case "branch":
		return "* " + g.Branch + "\n", nil
	}
	if !g.Initialized {
		return "", fail(128, "fatal: not a git repository (or any of the parent directories): .git")
	}
	switch args[0] {
	case "add":
		for _, a := range args[1:] {
			if a == "." || a == "-A" || a == "--all" {
				for k, v := range s.Files {
					g.Staged[k] = v
				}
			} else if v, ok := s.Files[s.path(a)]; ok {
				g.Staged[s.path(a)] = v
			} else {
				return "", fail(128, "fatal: pathspec '%s' did not match any files", a)
			}
		}
		return "", nil
	case "status":
		var b strings.Builder
		b.WriteString("On branch " + g.Branch + "\n")
		for _, k := range sim.SortedKeys(g.Staged) {
			b.WriteString("\tnew file:   " + k + "\n")
		}
		return b.String(), nil
	case "commit":
		msg := "update"
		for i := 1; i < len(args)-1; i++ {
			if args[i] == "-m" || args[i] == "-am" {
				msg = args[i+1]
			}
		}
		if contains(args, "-am") || contains(args, "-a") {
			for k, v := range s.Files {
				g.Staged[k] = v
			}
		}
		if len(g.Staged) == 0 {
			return "", fail(1, "nothing to commit, working tree clean")
		}
		files := map[string]string{}
		if len(g.Commits) > 0 {
			for k, v := range g.Commits[len(g.Commits)-1].Files {
				files[k] = v
			}
		}
		for k, v := range g.Staged {
			files[k] = v
		}
		c := sim.GitCommit{SHA: s.State.ID(40), Message: msg, Branch: g.Branch, Files: files}
		g.Commits = append(g.Commits, c)
		g.Staged = map[string]string{}
		return fmt.Sprintf("[%s %s] %s\n %d files changed\n", g.Branch, c.SHA[:7], msg, len(files)), nil
	case "log":
		var b strings.Builder
		for i := len(g.Commits) - 1; i >= 0; i-- {
			b.WriteString(g.Commits[i].SHA[:7] + " " + g.Commits[i].Message + "\n")
		}
		return b.String(), nil
	case "remote":
		if len(args) >= 4 && args[1] == "add" {
			url := args[3]
			m := regexp.MustCompile(`/r/([a-z0-9_-]+)`).FindStringSubmatch(url)
			if m == nil {
				return "", fail(1, "only Cloud Source Repositories remotes (https://source.developers.google.com/p/PROJECT/r/REPO) are supported")
			}
			g.Remote = m[1]
			return "", nil
		}
		return g.Remote + "\n", nil
	case "push":
		if g.Remote == "" {
			return "", fail(128, "fatal: No configured push destination.")
		}
		if len(g.Commits) == 0 {
			return "", fail(1, "error: src refspec main does not match any")
		}
		var p *sim.Project
		var repo *sim.SourceRepo
		for _, pp := range s.State.Projects {
			if r := pp.SourceRepos[g.Remote]; r != nil {
				p, repo = pp, r
			}
		}
		if repo == nil {
			return "", fail(128, "fatal: remote repository %s not found", g.Remote)
		}
		last := g.Commits[len(g.Commits)-1]
		repo.Commits = append(repo.Commits, last)
		out := fmt.Sprintf("To https://source.developers.google.com/p/%s/r/%s\n   %s  %s -> %s\n", p.ID, repo.Name, last.SHA[:7], g.Branch, g.Branch)
		for _, tn := range sim.SortedKeys(p.Triggers) {
			t := p.Triggers[tn]
			if t.Repo != repo.Name {
				continue
			}
			if re, err := regexp.Compile(t.Branch); err == nil && re.MatchString(g.Branch) {
				b, err := s.runBuild(p, last.Files, "", t.BuildConfig, tn, last.SHA)
				if err != nil {
					out += "remote: trigger " + tn + " failed to start: " + err.Error() + "\n"
				} else {
					out += fmt.Sprintf("remote: Cloud Build trigger %s started build %s -> %s\n", tn, b.ID, b.Status)
				}
			}
		}
		return out, nil
	case "clone":
		return "", fail(1, "use `gcloud source repos clone REPO` in the simulator")
	}
	return "", fail(1, "git: '%s' is not supported in the simulator", args[0])
}

func (s *Session) docker(args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: docker COMMAND\n", nil
	}
	switch args[0] {
	case "build":
		tag := ""
		for i := 1; i < len(args)-1; i++ {
			if args[i] == "-t" || args[i] == "--tag" {
				tag = args[i+1]
			}
		}
		df, ok := s.Files["Dockerfile"]
		if !ok {
			return "", fail(1, "ERROR: failed to solve: failed to read dockerfile: open Dockerfile: no such file or directory")
		}
		if tag == "" {
			return "", fail(1, "use -t to tag the image in the simulator")
		}
		s.LocalImages[tag] = behaviorFromDockerfile(df)
		return "[+] Building 12.3s (8/8) FINISHED\n => naming to " + tag + "\n", nil
	case "tag":
		if len(args) < 3 {
			return "", fail(1, "Usage: docker tag SOURCE TARGET")
		}
		b, ok := s.LocalImages[args[1]]
		if !ok {
			return "", fail(1, "Error response from daemon: No such image: %s", args[1])
		}
		s.LocalImages[args[2]] = b
		return "", nil
	case "push":
		if len(args) < 2 {
			return "", fail(1, "Usage: docker push IMAGE")
		}
		ref := args[1]
		b, ok := s.LocalImages[ref]
		if !ok {
			return "", fail(1, "An image does not exist locally with the tag: %s", ref)
		}
		host := strings.Split(ref, "/")[0]
		if !s.DockerAuth[host] {
			return "", fail(1, "denied: Unauthenticated request. Unauthenticated requests do not have permission \"artifactregistry.repositories.uploadArtifacts\". Run `gcloud auth configure-docker %s`.", host)
		}
		if err := s.pushImage(s.Principal(), ref, b); err != nil {
			return "", fail(1, "%v", err)
		}
		return "The push refers to repository [" + ref + "]\nlatest: digest: sha256:" + s.State.ID(64) + " size: 1570\n", nil
	case "images":
		var b strings.Builder
		b.WriteString("REPOSITORY   TAG   IMAGE ID\n")
		for _, k := range sim.SortedKeys(s.LocalImages) {
			b.WriteString(k + "   latest   " + s.State.ID(12) + "\n")
		}
		return b.String(), nil
	case "run":
		return "", fail(1, "docker run is not available in Cloud Shell simulation; deploy to Cloud Run, GKE or a VM instead")
	}
	return "", fail(1, "docker: '%s' is not supported in the simulator", args[0])
}

// ---- bq -----------------------------------------------------------------------

func (s *Session) bq(args []string, stdin string) (string, error) {
	pos, f := parseArgs(args)
	if len(pos) == 0 {
		return "Usage: bq COMMAND\n", nil
	}
	c := &Cmd{S: s, F: f, Args: pos[1:], Path: "bq " + pos[0]}
	p, err := c.P()
	if err != nil {
		return "", fail(1, "BigQuery error: %v", err)
	}
	if !p.Services["bigquery.googleapis.com"] {
		return "", fail(1, "BigQuery error in %s operation: Access Denied: BigQuery API has not been used in project %s before or it is disabled.", pos[0], p.Number)
	}
	ref := func(r string) (string, string) {
		r = strings.ReplaceAll(r, ":", ".")
		parts := strings.Split(r, ".")
		switch len(parts) {
		case 1:
			return parts[0], ""
		case 2:
			if s.State.Projects[parts[0]] != nil && p.Datasets[parts[0]] == nil {
				return parts[1], ""
			}
			return parts[0], parts[1]
		default:
			return parts[1], parts[2]
		}
	}
	dsRes := func(ds *sim.Dataset) sim.Resource {
		return sim.Resource{Project: p.ID, Type: "bigquery.googleapis.com/Dataset", Name: "projects/" + p.ID + "/datasets/" + ds.ID, Service: "bigquery.googleapis.com", Policies: []*sim.Policy{&ds.IAM}}
	}
	switch pos[0] {
	case "mk":
		if len(c.Args) == 0 {
			return "", fail(1, "BigQuery error in mk operation: missing identifier")
		}
		dsn, tn := ref(c.Args[0])
		if c.Bool("d") || c.Bool("dataset") || tn == "" && !c.Bool("t") && !c.Has("table") {
			if p.Datasets[dsn] != nil {
				return "", fail(1, "BigQuery error in mk operation: Dataset '%s:%s' already exists.", p.ID, dsn)
			}
			if err := c.NeedProject("bigquery.datasets.create"); err != nil {
				return "", fail(1, "BigQuery error in mk operation: %v", err)
			}
			loc := c.Str("location", "US")
			if !s.State.OrgPolicyAllows(p.ID, "gcp.resourceLocations", loc) {
				return "", fail(1, "BigQuery error in mk operation: location %s violates constraints/gcp.resourceLocations", loc)
			}
			p.Datasets[dsn] = &sim.Dataset{ID: dsn, Location: loc, Tables: map[string]*sim.Table{}, DefaultExpirationDays: c.Int("default_table_expiration", 0) / 86400}
			s.State.Audit(p.ID, s.Principal(), "bigquery.googleapis.com", "google.cloud.bigquery.v2.DatasetService.InsertDataset", "projects/"+p.ID+"/datasets/"+dsn)
			return fmt.Sprintf("Dataset '%s:%s' successfully created.\n", p.ID, dsn), nil
		}
		ds := p.Datasets[dsn]
		if ds == nil {
			return "", fail(1, "BigQuery error in mk operation: Not found: Dataset %s:%s", p.ID, dsn)
		}
		if err := c.Need("bigquery.tables.create", dsRes(ds)); err != nil {
			return "", fail(1, "BigQuery error in mk operation: %v", err)
		}
		t := &sim.Table{ID: tn, Kind: "TABLE", PartitionField: c.Str("time_partitioning_field", ""), Clustering: c.List("clustering_fields"), RequirePartitionFilter: c.Bool("require_partition_filter")}
		if t.PartitionField != "" || c.Has("time_partitioning_type") {
			t.PartitionType, t.PartitionDays = c.Str("time_partitioning_type", "DAY"), 365
		}
		if v := c.Str("view", ""); v != "" {
			t.Kind, t.Query = "VIEW", v
		}
		if len(c.Args) > 1 {
			for _, fd := range strings.Split(c.Args[1], ",") {
				parts := strings.Split(fd, ":")
				f := sim.Field{Name: parts[0], Type: "STRING", Bytes: 8}
				if len(parts) > 1 {
					f.Type = strings.ToUpper(parts[1])
				}
				if len(parts) > 2 {
					fmt.Sscan(parts[2], &f.Bytes)
				}
				t.Schema = append(t.Schema, f)
			}
		}
		// Simulator extension used by lab setups to model large historical tables.
		if v := c.Str("sim_rows", ""); v != "" {
			fmt.Sscan(v, &t.Rows)
		}
		if v := c.Str("sim_days", ""); v != "" {
			fmt.Sscan(v, &t.PartitionDays)
		}
		ds.Tables[tn] = t
		s.State.Audit(p.ID, s.Principal(), "bigquery.googleapis.com", "google.cloud.bigquery.v2.TableService.InsertTable", "projects/"+p.ID+"/datasets/"+dsn+"/tables/"+tn)
		return fmt.Sprintf("Table '%s:%s.%s' successfully created.\n", p.ID, dsn, tn), nil
	case "ls":
		var b strings.Builder
		if len(c.Args) == 0 {
			b.WriteString("  datasetId\n ------------\n")
			for _, k := range sim.SortedKeys(p.Datasets) {
				b.WriteString("  " + k + "\n")
			}
			return b.String(), nil
		}
		dsn, _ := ref(c.Args[0])
		ds := p.Datasets[dsn]
		if ds == nil {
			return "", fail(1, "BigQuery error in ls operation: Not found: Dataset %s", dsn)
		}
		b.WriteString("  tableId     Type    Time Partitioning   Clustered Fields\n ----------- ------- ------------------- -----------------\n")
		for _, k := range sim.SortedKeys(ds.Tables) {
			t := ds.Tables[k]
			part := ""
			if t.PartitionField != "" {
				part = "DAY (field: " + t.PartitionField + ")"
			}
			b.WriteString(fmt.Sprintf("  %-10s  %-6s  %-18s  %s\n", k, t.Kind, part, strings.Join(t.Clustering, ", ")))
		}
		return b.String(), nil
	case "show":
		if len(c.Args) == 0 {
			return "", fail(1, "BigQuery error in show operation: missing identifier")
		}
		dsn, tn := ref(c.Args[len(c.Args)-1])
		ds := p.Datasets[dsn]
		if ds == nil {
			return "", fail(1, "BigQuery error in show operation: Not found: Dataset %s", dsn)
		}
		if tn == "" {
			out, _ := render(Obj{V: map[string]any{"datasetReference": map[string]any{"datasetId": ds.ID, "projectId": p.ID}, "location": ds.Location, "access": ds.IAM.Bindings}}, Flags{"format": f["format"]})
			return out, nil
		}
		t := ds.Tables[tn]
		if t == nil {
			return "", fail(1, "BigQuery error in show operation: Not found: Table %s:%s.%s", p.ID, dsn, tn)
		}
		if c.Bool("schema") {
			out, _ := render(Obj{V: t.Schema}, Flags{"format": {"json"}})
			return out, nil
		}
		out, _ := render(Obj{V: map[string]any{"tableReference": map[string]any{"tableId": t.ID, "datasetId": dsn}, "type": t.Kind, "numRows": t.Rows, "timePartitioning": map[string]any{"field": t.PartitionField, "type": t.PartitionType, "requirePartitionFilter": t.RequirePartitionFilter}, "clustering": map[string]any{"fields": t.Clustering}, "schema": t.Schema, "view": t.Query, "modelType": t.ModelType}}, Flags{"format": f["format"]})
		return out, nil
	case "rm":
		if len(c.Args) == 0 {
			return "", fail(1, "BigQuery error in rm operation: missing identifier")
		}
		dsn, tn := ref(c.Args[0])
		ds := p.Datasets[dsn]
		if ds == nil {
			return "", fail(1, "BigQuery error in rm operation: Not found: Dataset %s", dsn)
		}
		if tn != "" {
			if err := c.Need("bigquery.tables.delete", dsRes(ds)); err != nil {
				return "", fail(1, "BigQuery error in rm operation: %v", err)
			}
			delete(ds.Tables, tn)
			return "", nil
		}
		if len(ds.Tables) > 0 && !c.Bool("r") {
			return "", fail(1, "BigQuery error in rm operation: Dataset %s:%s is still in use", p.ID, dsn)
		}
		delete(p.Datasets, dsn)
		return "", nil
	case "query":
		sql := strings.Join(c.Args, " ")
		if sql == "" {
			sql = stdin
		}
		legacy := !(c.Str("use_legacy_sql", "true") == "false" || c.Bool("nouse_legacy_sql"))
		if legacy && strings.Contains(sql, "`") {
			return "", fail(1, "Error in query string: Error processing job '%s:bqjob_r%s': Invalid table name: `%s` [Try using standard SQL (https://cloud.google.com/bigquery/docs/reference/standard-sql/enabling-standard-sql)] (hint: pass --use_legacy_sql=false)", p.ID, s.State.ID(8), strings.Split(strings.SplitN(sql, "`", 3)[1], "`")[0])
		}
		dry := c.Bool("dry_run") || c.Bool("dry-run")
		dest := c.Str("destination_table", "")
		if sim.IsDML(sql) {
			if dry {
				return "Query successfully validated. Assuming the tables are not modified, running this query will process 10485760 bytes of data.\n", nil
			}
			n, err := s.State.RunDML(p.ID, s.Principal(), strings.TrimSuffix(strings.TrimSpace(sql), ";"))
			if err != nil {
				return "", fail(1, "Error in query string: %v", err)
			}
			return fmt.Sprintf("Waiting on bqjob_r%s ... (1s) Current status: DONE\nNumber of affected rows: %d\n", s.State.ID(10), n), nil
		}
		res, err := s.State.RunQuery(p.ID, s.Principal(), sql, dry, strings.ReplaceAll(dest, ":", "."), c.Str("time_partitioning_field", ""), c.List("clustering_fields"))
		if err != nil {
			return "", fail(1, "Error in query string: %v", err)
		}
		for _, fm := range regexp.MustCompile("(?i)FROM\\s+`?([a-zA-Z0-9_.:-]+)`?").FindAllStringSubmatch(sql, -1) {
			dsn, tn := ref(fm[1])
			if ds := p.Datasets[dsn]; ds != nil && ds.Tables[tn] != nil {
				if !s.State.Allowed(s.Principal(), "bigquery.tables.getData", dsRes(ds)) {
					return "", fail(1, "BigQuery error in query operation: Access Denied: Table %s:%s.%s: User does not have permission to query table %s:%s.%s, or perhaps it does not exist.", p.ID, dsn, tn, p.ID, dsn, tn)
				}
			}
		}
		if dry {
			return fmt.Sprintf("Query successfully validated. Assuming the tables are not modified, running this query will process %d bytes of data.\n", res.Bytes), nil
		}
		if res.Created != "" {
			return fmt.Sprintf("Waiting on bqjob_r%s ... (1s) Current status: DONE\nCreated %s (processed %s)\n", s.State.ID(10), res.Created, sim.FormatBytes(res.Bytes)), nil
		}
		head := fmt.Sprintf("Waiting on bqjob_r%s ... (2s) Current status: DONE\n", s.State.ID(10))
		foot := fmt.Sprintf("Bytes processed: %s (%d)\n", sim.FormatBytes(res.Bytes), res.Bytes)
		if regexp.MustCompile(`(?i)\bML\.|INFORMATION_SCHEMA`).MatchString(sql) {
			return head + syntheticRows(s, res.Columns) + foot, nil
		}
		data, err := s.State.QueryData(p.ID, sql)
		if err != nil {
			msg := err.Error()
			if strings.HasPrefix(msg, "Unrecognized name") || strings.HasPrefix(msg, "Not found") || strings.HasPrefix(msg, "Function not found") || strings.Contains(msg, "not supported by the simulator") {
				return "", fail(1, "Error in query string: %s", msg)
			}
			return "", fail(1, "Error in query string: %s", msg)
		}
		return head + formatBQ(c, data) + foot, nil
	case "head":
		if len(c.Args) == 0 {
			return "", fail(1, "BigQuery error in head operation: missing identifier")
		}
		dsn, tn := ref(c.Args[0])
		ds := p.Datasets[dsn]
		if ds == nil || ds.Tables[tn] == nil {
			return "", fail(1, "BigQuery error in head operation: Not found: Table %s:%s.%s", p.ID, dsn, tn)
		}
		if !s.State.Allowed(s.Principal(), "bigquery.tables.getData", dsRes(ds)) {
			return "", fail(1, "BigQuery error in head operation: Access Denied: Table %s:%s.%s", p.ID, dsn, tn)
		}
		n := c.Int("n", c.Int("max_rows", 100))
		data, err := s.State.QueryData(p.ID, fmt.Sprintf("SELECT * FROM `%s.%s` LIMIT %d", dsn, tn, n))
		if err != nil {
			return "", fail(1, "BigQuery error in head operation: %v", err)
		}
		return formatBQ(c, data), nil
	case "load", "insert":
		if len(c.Args) < 2 {
			return "", fail(1, "BigQuery error in %s operation: usage: bq %s DATASET.TABLE SOURCE [SCHEMA]", pos[0], pos[0])
		}
		dsn, tn := ref(c.Args[0])
		ds := p.Datasets[dsn]
		if ds == nil {
			return "", fail(1, "BigQuery error in %s operation: Not found: Dataset %s:%s", pos[0], p.ID, dsn)
		}
		if err := c.Need("bigquery.tables.updateData", dsRes(ds)); err != nil {
			return "", fail(1, "BigQuery error in %s operation: %v", pos[0], err)
		}
		content, err := s.readSource(c.Args[1], stdin)
		if err != nil {
			return "", fail(1, "BigQuery error in %s operation: %v", pos[0], err)
		}
		format := strings.ToUpper(c.Str("source_format", "CSV"))
		if pos[0] == "insert" || strings.HasSuffix(strings.ToLower(c.Args[1]), ".json") && !c.Has("source_format") {
			format = "NEWLINE_DELIMITED_JSON"
		}
		cols, rows, err := parseRecords(content, format, c.Int("skip_leading_rows", 0))
		if err != nil {
			return "", fail(1, "BigQuery error in %s operation: Error while reading data: %v", pos[0], err)
		}
		t := ds.Tables[tn]
		if t == nil || c.Bool("replace") {
			nt := &sim.Table{ID: tn, Kind: "TABLE"}
			if len(c.Args) > 2 {
				nt.Schema = schemaFromSpec(c.Args[2])
			} else if t != nil {
				nt.Schema = t.Schema
			} else if c.Bool("autodetect") || format == "NEWLINE_DELIMITED_JSON" {
				nt.Schema = autodetect(cols, rows)
			} else {
				return "", fail(1, "BigQuery error in load operation: No schema specified on job or table. (use --autodetect or pass SCHEMA)")
			}
			t = nt
			ds.Tables[tn] = t
		}
		if format == "CSV" && len(cols) == 0 {
			for _, f := range t.Schema {
				cols = append(cols, f.Name)
			}
		}
		if format == "CSV" && c.Int("skip_leading_rows", 0) == 0 && c.Bool("autodetect") && len(rows) > 0 {
			rows = rows[1:]
		}
		t.AppendRows(cols, rows)
		s.State.Audit(p.ID, s.Principal(), "bigquery.googleapis.com", "google.cloud.bigquery.v2.JobService.InsertJob", "projects/"+p.ID+"/datasets/"+dsn+"/tables/"+tn)
		if pos[0] == "insert" {
			return "", nil
		}
		return fmt.Sprintf("Waiting on bqjob_r%s ... (1s) Current status: DONE\nLoaded %d row(s) into %s:%s.%s.\n", s.State.ID(10), len(rows), p.ID, dsn, tn), nil
	case "extract":
		if len(c.Args) < 2 {
			return "", fail(1, "BigQuery error in extract operation: usage: bq extract DATASET.TABLE gs://BUCKET/OBJECT")
		}
		dsn, tn := ref(c.Args[0])
		ds := p.Datasets[dsn]
		if ds == nil || ds.Tables[tn] == nil {
			return "", fail(1, "BigQuery error in extract operation: Not found: Table %s:%s.%s", p.ID, dsn, tn)
		}
		data, err := s.State.QueryData(p.ID, fmt.Sprintf("SELECT * FROM `%s.%s`", dsn, tn))
		if err != nil {
			return "", fail(1, "BigQuery error in extract operation: %v", err)
		}
		var b strings.Builder
		if strings.EqualFold(c.Str("destination_format", "CSV"), "NEWLINE_DELIMITED_JSON") {
			for _, r := range data.Rows {
				m := map[string]any{}
				for i, cn := range data.Columns {
					m[cn] = r[i]
				}
				j, _ := json.Marshal(m)
				b.Write(j)
				b.WriteString("\n")
			}
		} else {
			w := csv.NewWriter(&b)
			w.Write(data.Columns)
			for _, r := range data.Rows {
				rec := make([]string, len(r))
				for i, v := range r {
					if v != nil {
						rec[i] = sqlengine.Display(v)
					}
				}
				w.Write(rec)
			}
			w.Flush()
		}
		if _, err := s.writeGCS(c.Args[1], b.String()); err != nil {
			return "", fail(1, "BigQuery error in extract operation: %v", err)
		}
		return fmt.Sprintf("Waiting on bqjob_r%s ... (1s) Current status: DONE\n", s.State.ID(10)), nil
	case "add-iam-policy-binding", "remove-iam-policy-binding":
		if len(c.Args) == 0 {
			return "", fail(1, "missing dataset")
		}
		dsn, _ := ref(c.Args[0])
		ds := p.Datasets[dsn]
		if ds == nil {
			return "", fail(1, "BigQuery error: Not found: Dataset %s", dsn)
		}
		m, err := member(c.Str("member", ""))
		if err != nil {
			return "", fail(1, "%v", err)
		}
		role := c.Str("role", "")
		if pos[0] == "add-iam-policy-binding" {
			ds.IAM.AddBinding(role, m, nil)
		} else {
			ds.IAM.RemoveBinding(role, m)
		}
		s.State.Audit(p.ID, s.Principal(), "bigquery.googleapis.com", "google.iam.v1.IAMPolicy.SetIamPolicy", "projects/"+p.ID+"/datasets/"+dsn)
		return "Updated IAM policy for dataset " + dsn + ".\n", nil
	}
	return "", fail(1, "FATAL Flags parsing error: Unknown command %q (supported: mk, ls, show, rm, query, head, load, insert, extract, add-iam-policy-binding)", pos[0])
}
