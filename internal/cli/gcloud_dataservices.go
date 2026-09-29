package cli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
	"github.com/neodevesp/gcp-trainer/internal/sqlengine"
)

// Firestore, Memorystore for Redis, Spanner, Dataflow, Dataproc, Cloud
// Composer and Filestore.

// project returns the current project with the newer service maps ready.
func (c *Cmd) project() (*sim.Project, error) {
	p, err := c.P()
	if err != nil {
		return nil, err
	}
	p.EnsureServices()
	return p, nil
}

func (c *Cmd) name(what string) (string, error) {
	n, err := c.Arg(0, what)
	if err != nil {
		return "", err
	}
	return n[strings.LastIndex(n, "/")+1:], nil
}

// columns renders a gcloud-style table (no borders, two spaces between columns).
func columns(cols []string, rows [][]any) string {
	w := make([]int, len(cols))
	cells := make([][]string, len(rows))
	for i, c := range cols {
		w[i] = len(c)
	}
	for r, row := range rows {
		cells[r] = make([]string, len(cols))
		for i := range cols {
			v := ""
			if i < len(row) && row[i] != nil {
				v = sqlengine.Display(row[i])
			} else if i < len(row) {
				v = "NULL"
			}
			cells[r][i] = v
			if len(v) > w[i] {
				w[i] = len(v)
			}
		}
	}
	var b strings.Builder
	line := func(vals []string) {
		var l strings.Builder
		for i, v := range vals {
			l.WriteString(v)
			if i < len(vals)-1 {
				l.WriteString(strings.Repeat(" ", w[i]-len(v)+2))
			}
		}
		b.WriteString(strings.TrimRight(l.String(), " ") + "\n")
	}
	line(cols)
	for _, r := range cells {
		line(r)
	}
	return b.String()
}

var reDagID = regexp.MustCompile(`(?:dag_id\s*=\s*|DAG\(\s*)["']([A-Za-z0-9_.-]+)["']`)

func init() {
	// ---- Firestore ------------------------------------------------------------------
	reg("firestore databases create", func(c *Cmd) (any, error) {
		p, err := c.project()
		if err != nil {
			return nil, err
		}
		if err := c.API("firestore.googleapis.com"); err != nil {
			return nil, err
		}
		name := c.Str("database", "(default)")
		loc := c.Str("location", "")
		if loc == "" {
			return nil, fmt.Errorf("argument --location: Must be specified (e.g. eur3, nam5, europe-west1).")
		}
		if p.Firestore[name] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: Database '%s' already exists for project %s.", name, p.ID)
		}
		if err := c.NeedProject("datastore.databases.create"); err != nil {
			return nil, err
		}
		typ := strings.ToUpper(strings.ReplaceAll(c.Str("type", "firestore-native"), "-", "_"))
		p.Firestore[name] = &sim.FirestoreDB{Name: name, Location: loc, Type: typ, DeleteProtection: c.Bool("delete-protection"), Docs: map[string]map[string]any{}}
		c.Audit("firestore.googleapis.com", "google.firestore.admin.v1.FirestoreAdmin.CreateDatabase", "projects/"+p.ID+"/databases/"+name)
		return fmt.Sprintf("Success! Selected Google Cloud Firestore %s database for %s\nname: projects/%s/databases/%s\nlocationId: %s\ntype: %s\n", strings.ToLower(strings.TrimPrefix(typ, "FIRESTORE_")), p.ID, p.ID, name, loc, typ), nil
	})
	reg("firestore databases list", func(c *Cmd) (any, error) {
		p, err := c.project()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Firestore) {
			d := p.Firestore[k]
			rows = append(rows, map[string]any{"name": "projects/" + p.ID + "/databases/" + k, "location": d.Location, "type": d.Type, "docs": len(d.Docs)})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"LOCATION", "location"}, {"TYPE", "type"}, {"DOCUMENTS", "docs"}}, Rows: rows}, nil
	})
	getFS := func(c *Cmd) (*sim.Project, *sim.FirestoreDB, error) {
		p, err := c.project()
		if err != nil {
			return nil, nil, err
		}
		name := c.Str("database", "(default)")
		d := p.Firestore[name]
		if d == nil {
			return nil, nil, fmt.Errorf("NOT_FOUND: The database %s does not exist for project %s. Create it with `gcloud firestore databases create --location=...`.", name, p.ID)
		}
		return p, d, nil
	}
	reg("firestore databases describe", func(c *Cmd) (any, error) {
		_, d, err := getFS(c)
		if err != nil {
			return nil, err
		}
		return Obj{V: map[string]any{"name": d.Name, "locationId": d.Location, "type": d.Type, "deleteProtectionState": d.DeleteProtection, "pointInTimeRecoveryEnablement": d.PITR}}, nil
	})
	reg("firestore databases update", func(c *Cmd) (any, error) {
		_, d, err := getFS(c)
		if err != nil {
			return nil, err
		}
		if c.Has("delete-protection") {
			d.DeleteProtection = c.Bool("delete-protection")
		}
		if c.Bool("no-delete-protection") {
			d.DeleteProtection = false
		}
		if c.Has("enable-pitr") {
			d.PITR = c.Bool("enable-pitr")
		}
		return "Updated database [" + d.Name + "].\n", nil
	})
	reg("firestore databases delete", func(c *Cmd) (any, error) {
		p, d, err := getFS(c)
		if err != nil {
			return nil, err
		}
		if d.DeleteProtection {
			return nil, fmt.Errorf("FAILED_PRECONDITION: Delete protection is enabled for database %s. Disable it with `gcloud firestore databases update --no-delete-protection`.", d.Name)
		}
		delete(p.Firestore, d.Name)
		return "Deleted database [" + d.Name + "].\n", nil
	})
	reg("firestore indexes composite create", func(c *Cmd) (any, error) {
		_, d, err := getFS(c)
		if err != nil {
			return nil, err
		}
		coll := c.Str("collection-group", "")
		if coll == "" {
			return nil, fmt.Errorf("argument --collection-group: Must be specified.")
		}
		var fields []string
		for _, fc := range c.F["field-config"] {
			kv := map[string]string{}
			for _, part := range strings.Split(fc, ",") {
				if i := strings.Index(part, "="); i > 0 {
					kv[part[:i]] = part[i+1:]
				}
			}
			order := kv["order"]
			if order == "" {
				order = kv["array-config"]
			}
			fields = append(fields, kv["field-path"]+" "+strings.ToUpper(order))
		}
		if len(fields) < 2 {
			return nil, fmt.Errorf("INVALID_ARGUMENT: A composite index needs at least two --field-config entries.")
		}
		id := c.S.State.ID(12)
		d.Indexes = append(d.Indexes, sim.FirestoreIndex{ID: id, Collection: coll, Fields: fields, State: "READY"})
		return "Create request issued\nCreated index [" + id + "].\n", nil
	})
	reg("firestore indexes composite list", func(c *Cmd) (any, error) {
		_, d, err := getFS(c)
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, ix := range d.Indexes {
			rows = append(rows, map[string]any{"id": ix.ID, "coll": ix.Collection, "fields": strings.Join(ix.Fields, ", "), "state": ix.State})
		}
		return Table{Cols: []Col{{"NAME", "id"}, {"COLLECTION_GROUP", "coll"}, {"FIELDS", "fields"}, {"STATE", "state"}}, Rows: rows}, nil
	})
	reg("firestore indexes composite delete", func(c *Cmd) (any, error) {
		_, d, err := getFS(c)
		if err != nil {
			return nil, err
		}
		id, err := c.name("INDEX")
		if err != nil {
			return nil, err
		}
		var keep []sim.FirestoreIndex
		for _, ix := range d.Indexes {
			if ix.ID != id {
				keep = append(keep, ix)
			}
		}
		d.Indexes = keep
		return "Deleted index [" + id + "].\n", nil
	})
	reg("firestore export", func(c *Cmd) (any, error) {
		p, d, err := getFS(c)
		if err != nil {
			return nil, err
		}
		dst, err := c.Arg(0, "OUTPUT_URI_PREFIX")
		if err != nil {
			return nil, err
		}
		b, _, err := c.bucket(dst)
		if err != nil {
			return nil, err
		}
		prefix := strings.Trim(strings.TrimPrefix(strings.TrimPrefix(dst, "gs://"), b.Name), "/")
		if prefix == "" {
			prefix = strings.NewReplacer(":", "_", "-", "_").Replace(c.S.State.Now())
		}
		colls := c.List("collection-ids")
		var lines []string
		for _, k := range sim.SortedKeys(d.Docs) {
			coll := k[:strings.Index(k, "/")]
			if len(colls) > 0 && !contains(colls, coll) {
				continue
			}
			lines = append(lines, k+"\t"+string(mustJSON(d.Docs[k])))
		}
		content := strings.Join(lines, "\n")
		obj := prefix + "/all_namespaces/all_kinds/output-0"
		b.Objects[obj] = &sim.Object{Name: obj, Content: content, Size: len(content), StorageClass: b.StorageClass, Generation: 1, Updated: c.S.State.Now()}
		meta := prefix + "/" + prefix[strings.LastIndex(prefix, "/")+1:] + ".overall_export_metadata"
		b.Objects[meta] = &sim.Object{Name: meta, Content: "export", Size: 6, StorageClass: b.StorageClass, Generation: 1, Updated: c.S.State.Now()}
		c.Audit("firestore.googleapis.com", "google.firestore.admin.v1.FirestoreAdmin.ExportDocuments", "projects/"+p.ID+"/databases/"+d.Name)
		return fmt.Sprintf("Waiting for [projects/%s/databases/%s/operations/%s] to finish...done.\nmetadata:\n  outputUriPrefix: gs://%s/%s\n  operationState: SUCCESSFUL\n  progressDocuments:\n    completedWork: '%d'\n", p.ID, d.Name, c.S.State.ID(16), b.Name, prefix, len(lines)), nil
	})
	reg("firestore import", func(c *Cmd) (any, error) {
		_, d, err := getFS(c)
		if err != nil {
			return nil, err
		}
		src, err := c.Arg(0, "INPUT_URI_PREFIX")
		if err != nil {
			return nil, err
		}
		b, _, err := c.bucket(src)
		if err != nil {
			return nil, err
		}
		prefix := strings.Trim(strings.TrimPrefix(strings.TrimPrefix(src, "gs://"), b.Name), "/")
		o := b.Objects[prefix+"/all_namespaces/all_kinds/output-0"]
		if o == nil {
			return nil, fmt.Errorf("INVALID_ARGUMENT: The specified input path %s does not contain a Firestore export (overall_export_metadata not found).", src)
		}
		n := 0
		for _, line := range strings.Split(o.Content, "\n") {
			k, js, ok := strings.Cut(line, "\t")
			if !ok {
				continue
			}
			var fields map[string]any
			if err := json.Unmarshal([]byte(js), &fields); err == nil {
				d.Docs[k] = fields
				n++
			}
		}
		return fmt.Sprintf("Waiting for import to finish...done.\noperationState: SUCCESSFUL\nprogressDocuments:\n  completedWork: '%d'\n", n), nil
	})

	// ---- Memorystore for Redis --------------------------------------------------------
	getRedis := func(c *Cmd) (*sim.Project, *sim.RedisInstance, error) {
		p, err := c.project()
		if err != nil {
			return nil, nil, err
		}
		n, err := c.name("INSTANCE")
		if err != nil {
			return nil, nil, err
		}
		r := p.Redis[n]
		if r == nil {
			return nil, nil, fmt.Errorf("NOT_FOUND: Resource 'projects/%s/locations/%s/instances/%s' was not found", p.ID, c.Str("region", c.S.Region), n)
		}
		return p, r, nil
	}
	reg("redis instances create", func(c *Cmd) (any, error) {
		p, err := c.project()
		if err != nil {
			return nil, err
		}
		if err := c.API("redis.googleapis.com"); err != nil {
			return nil, err
		}
		n, err := c.name("INSTANCE")
		if err != nil {
			return nil, err
		}
		if p.Redis[n] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: Instance %s already exists.", n)
		}
		nw := c.Str("network", "default")
		nw = nw[strings.LastIndex(nw, "/")+1:]
		if p.Networks[nw] == nil {
			return nil, fmt.Errorf("INVALID_ARGUMENT: Network %s not found in project %s.", nw, p.ID)
		}
		size := c.Int("size", 1)
		tier := strings.ToUpper(c.Str("tier", "basic"))
		if tier != "BASIC" && tier != "STANDARD" {
			return nil, fmt.Errorf("argument --tier: Invalid choice: '%s'. Valid choices are [basic, standard].", strings.ToLower(tier))
		}
		if err := c.NeedProject("redis.instances.create"); err != nil {
			return nil, err
		}
		ver := strings.ToUpper(c.Str("redis-version", "redis_7_2"))
		r := &sim.RedisInstance{Name: n, Region: c.Str("region", c.S.Region), Tier: tier, SizeGB: size, Version: ver, Host: c.S.State.AllocIP("10.137.125.0/29"), Port: 6379, Network: nw, State: "READY", AuthEnabled: c.Bool("enable-auth"), Data: map[string]string{}}
		if r.AuthEnabled {
			r.AuthString = c.S.State.ID(8) + "-" + c.S.State.ID(4) + "-" + c.S.State.ID(12)
		}
		p.Redis[n] = r
		c.Audit("redis.googleapis.com", "google.cloud.redis.v1.CloudRedis.CreateInstance", "projects/"+p.ID+"/locations/"+r.Region+"/instances/"+n)
		return fmt.Sprintf("Create request issued for: [%s]\nWaiting for operation [projects/%s/locations/%s/operations/operation-%s] to complete...done.\nCreated instance [%s].\n", n, p.ID, r.Region, c.S.State.ID(10), n), nil
	})
	reg("redis instances list", func(c *Cmd) (any, error) {
		p, err := c.project()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Redis) {
			r := p.Redis[k]
			rows = append(rows, map[string]any{"name": k, "version": r.Version, "region": r.Region, "tier": r.Tier, "size": r.SizeGB, "host": r.Host, "port": r.Port, "network": r.Network, "status": r.State})
		}
		return Table{Cols: []Col{{"INSTANCE_NAME", "name"}, {"VERSION", "version"}, {"REGION", "region"}, {"TIER", "tier"}, {"SIZE_GB", "size"}, {"HOST", "host"}, {"PORT", "port"}, {"NETWORK", "network"}, {"STATUS", "status"}}, Rows: rows}, nil
	})
	reg("redis instances describe", func(c *Cmd) (any, error) {
		_, r, err := getRedis(c)
		if err != nil {
			return nil, err
		}
		v := *r
		v.Data, v.AuthString = nil, ""
		return Obj{V: v}, nil
	})
	reg("redis instances get-auth-string", func(c *Cmd) (any, error) {
		_, r, err := getRedis(c)
		if err != nil {
			return nil, err
		}
		if !r.AuthEnabled {
			return nil, fmt.Errorf("FAILED_PRECONDITION: AUTH is not enabled for instance %s.", r.Name)
		}
		return "authString: " + r.AuthString + "\n", nil
	})
	reg("redis instances update", func(c *Cmd) (any, error) {
		_, r, err := getRedis(c)
		if err != nil {
			return nil, err
		}
		if c.Has("size") {
			r.SizeGB = c.Int("size", r.SizeGB)
		}
		if c.Has("enable-auth") {
			r.AuthEnabled = c.Bool("enable-auth")
			if r.AuthEnabled && r.AuthString == "" {
				r.AuthString = c.S.State.ID(8) + "-" + c.S.State.ID(4) + "-" + c.S.State.ID(12)
			}
		}
		return "Request issued for: [" + r.Name + "]\nUpdated instance [" + r.Name + "].\n", nil
	})
	reg("redis instances delete", func(c *Cmd) (any, error) {
		p, r, err := getRedis(c)
		if err != nil {
			return nil, err
		}
		delete(p.Redis, r.Name)
		return "Deleted instance [" + r.Name + "].\n", nil
	})

	// ---- Spanner ------------------------------------------------------------------------
	getSpanner := func(c *Cmd, name string) (*sim.Project, *sim.SpannerInstance, error) {
		p, err := c.project()
		if err != nil {
			return nil, nil, err
		}
		in := p.Spanner[name]
		if in == nil {
			return nil, nil, fmt.Errorf("NOT_FOUND: Instance not found: projects/%s/instances/%s", p.ID, name)
		}
		return p, in, nil
	}
	reg("spanner instances create", func(c *Cmd) (any, error) {
		p, err := c.project()
		if err != nil {
			return nil, err
		}
		if err := c.API("spanner.googleapis.com"); err != nil {
			return nil, err
		}
		n, err := c.name("INSTANCE")
		if err != nil {
			return nil, err
		}
		if p.Spanner[n] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: Instance already exists: projects/%s/instances/%s", p.ID, n)
		}
		cfg := c.Str("config", "")
		if cfg == "" {
			return nil, fmt.Errorf("argument --config: Must be specified (e.g. regional-europe-west1).")
		}
		pu := c.Int("processing-units", 0)
		if nodes := c.Int("nodes", 0); nodes > 0 {
			pu = nodes * 1000
		}
		if pu == 0 {
			return nil, fmt.Errorf("One of --nodes or --processing-units must be specified.")
		}
		if pu < 100 || (pu < 1000 && pu%100 != 0) || (pu >= 1000 && pu%1000 != 0) {
			return nil, fmt.Errorf("INVALID_ARGUMENT: Processing units must be a multiple of 100 below 1000 and a multiple of 1000 from 1000.")
		}
		if err := c.NeedProject("spanner.instances.create"); err != nil {
			return nil, err
		}
		p.Spanner[n] = &sim.SpannerInstance{Name: n, Config: cfg, DisplayName: c.Str("description", n), ProcessingUnits: pu, State: "READY", Databases: map[string]*sim.SpannerDB{}}
		c.Audit("spanner.googleapis.com", "google.spanner.admin.instance.v1.InstanceAdmin.CreateInstance", "projects/"+p.ID+"/instances/"+n)
		return "Creating instance...done.\n", nil
	})
	reg("spanner instances list", func(c *Cmd) (any, error) {
		p, err := c.project()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Spanner) {
			in := p.Spanner[k]
			rows = append(rows, map[string]any{"name": k, "display": in.DisplayName, "config": in.Config, "nodes": in.ProcessingUnits / 1000, "pu": in.ProcessingUnits, "state": in.State})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"DISPLAY_NAME", "display"}, {"CONFIG", "config"}, {"NODE_COUNT", "nodes"}, {"PROCESSING_UNITS", "pu"}, {"STATE", "state"}}, Rows: rows}, nil
	})
	reg("spanner instances describe", func(c *Cmd) (any, error) {
		n, _ := c.name("INSTANCE")
		_, in, err := getSpanner(c, n)
		if err != nil {
			return nil, err
		}
		return Obj{V: map[string]any{"name": in.Name, "config": in.Config, "displayName": in.DisplayName, "processingUnits": in.ProcessingUnits, "state": in.State}}, nil
	})
	reg("spanner instances update", func(c *Cmd) (any, error) {
		n, _ := c.name("INSTANCE")
		_, in, err := getSpanner(c, n)
		if err != nil {
			return nil, err
		}
		if v := c.Int("nodes", 0); v > 0 {
			in.ProcessingUnits = v * 1000
		}
		if v := c.Int("processing-units", 0); v > 0 {
			in.ProcessingUnits = v
		}
		return "Updating instance...done.\n", nil
	})
	reg("spanner instances delete", func(c *Cmd) (any, error) {
		n, _ := c.name("INSTANCE")
		p, in, err := getSpanner(c, n)
		if err != nil {
			return nil, err
		}
		for _, d := range in.Databases {
			if d.DeleteProtection {
				return nil, fmt.Errorf("FAILED_PRECONDITION: Instance %s has database %s with drop protection enabled.", n, d.Name)
			}
		}
		delete(p.Spanner, n)
		return "Deleted instance [" + n + "].\n", nil
	})
	getSDB := func(c *Cmd) (*sim.SpannerInstance, *sim.SpannerDB, error) {
		_, in, err := getSpanner(c, c.Str("instance", ""))
		if err != nil {
			return nil, nil, err
		}
		n, err := c.name("DATABASE")
		if err != nil {
			return nil, nil, err
		}
		d := in.Databases[n]
		if d == nil {
			return nil, nil, fmt.Errorf("NOT_FOUND: Database not found: projects/%s/instances/%s/databases/%s", c.ProjectID(), in.Name, n)
		}
		return in, d, nil
	}
	reg("spanner databases create", func(c *Cmd) (any, error) {
		_, in, err := getSpanner(c, c.Str("instance", ""))
		if err != nil {
			return nil, err
		}
		n, err := c.name("DATABASE")
		if err != nil {
			return nil, err
		}
		if in.Databases[n] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: Database already exists: %s", n)
		}
		d := &sim.SpannerDB{Name: n, State: "READY"}
		if ddl := c.Str("ddl", ""); ddl != "" {
			if err := spannerDDL(d, ddl); err != nil {
				return nil, err
			}
		}
		in.Databases[n] = d
		return "Creating database...done.\n", nil
	})
	reg("spanner databases list", func(c *Cmd) (any, error) {
		_, in, err := getSpanner(c, c.Str("instance", ""))
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(in.Databases) {
			rows = append(rows, map[string]any{"name": k, "state": in.Databases[k].State})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"STATE", "state"}}, Rows: rows}, nil
	})
	reg("spanner databases ddl update", func(c *Cmd) (any, error) {
		_, d, err := getSDB(c)
		if err != nil {
			return nil, err
		}
		if err := spannerDDL(d, c.Str("ddl", "")); err != nil {
			return nil, err
		}
		return "Schema updating...done.\n", nil
	})
	reg("spanner databases ddl describe", func(c *Cmd) (any, error) {
		_, d, err := getSDB(c)
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		for _, st := range d.DDL {
			b.WriteString(st + ";\n\n")
		}
		return b.String(), nil
	})
	reg("spanner databases update", func(c *Cmd) (any, error) {
		_, d, err := getSDB(c)
		if err != nil {
			return nil, err
		}
		if c.Has("enable-drop-protection") {
			d.DeleteProtection = c.Bool("enable-drop-protection")
		}
		if c.Bool("no-enable-drop-protection") {
			d.DeleteProtection = false
		}
		return "Updated database [" + d.Name + "].\n", nil
	})
	reg("spanner databases delete", func(c *Cmd) (any, error) {
		in, d, err := getSDB(c)
		if err != nil {
			return nil, err
		}
		if d.DeleteProtection {
			return nil, fmt.Errorf("FAILED_PRECONDITION: Database %s has drop protection enabled. Run gcloud spanner databases update %s --instance=%s --no-enable-drop-protection first.", d.Name, d.Name, in.Name)
		}
		delete(in.Databases, d.Name)
		return "Deleted database [" + d.Name + "].\n", nil
	})
	reg("spanner databases execute-sql", func(c *Cmd) (any, error) {
		_, d, err := getSDB(c)
		if err != nil {
			return nil, err
		}
		sql := c.Str("sql", "")
		if sql == "" {
			return nil, fmt.Errorf("argument --sql: Must be specified.")
		}
		return spannerExec(d, sql)
	})
	reg("spanner rows insert", func(c *Cmd) (any, error) {
		_, sp, err := getSpanner(c, c.Str("instance", ""))
		if err != nil {
			return nil, err
		}
		d := sp.Databases[c.Str("database", "")]
		if d == nil {
			return nil, fmt.Errorf("NOT_FOUND: Database not found: %s", c.Str("database", ""))
		}
		kv := c.KV("data")
		keys := make([]string, 0, len(kv))
		for k := range kv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		vals := make([]string, len(keys))
		for i, k := range keys {
			v := kv[k]
			if _, err := strconv.ParseFloat(v, 64); err == nil {
				vals[i] = v
			} else {
				vals[i] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
			}
		}
		_, err = spannerExec(d, fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", c.Str("table", ""), strings.Join(keys, ", "), strings.Join(vals, ", ")))
		if err != nil {
			return nil, err
		}
		return "commitTimestamp: '" + c.S.State.Now() + "'\n", nil
	})

	// ---- Dataflow --------------------------------------------------------------------------
	getDF := func(c *Cmd) (*sim.Project, *sim.DataflowJob, error) {
		p, err := c.project()
		if err != nil {
			return nil, nil, err
		}
		id, err := c.name("JOB_ID")
		if err != nil {
			return nil, nil, err
		}
		for _, j := range p.DataflowJobs {
			if j.ID == id || j.Name == id {
				return p, j, nil
			}
		}
		return nil, nil, fmt.Errorf("NOT_FOUND: (%s) Dataflow job not found", id)
	}
	reg("dataflow jobs run", func(c *Cmd) (any, error) {
		p, err := c.project()
		if err != nil {
			return nil, err
		}
		if err := c.API("dataflow.googleapis.com"); err != nil {
			return nil, err
		}
		n, err := c.name("JOB_NAME")
		if err != nil {
			return nil, err
		}
		loc := c.Str("gcs-location", "")
		tpl := loc[strings.LastIndex(loc, "/")+1:]
		if tpl == "" {
			return nil, fmt.Errorf("argument --gcs-location: Must be specified (e.g. gs://dataflow-templates-europe-west1/latest/Word_Count).")
		}
		region := c.Str("region", c.S.Region)
		params := c.KV("parameters")
		now := c.S.State.Now()
		id := strings.NewReplacer("-", "_", ":", "_", "T", "_").Replace(strings.TrimSuffix(now, "Z")) + "-" + strconv.Itoa(1000000000+c.S.State.Rand().Intn(899999999))
		j := &sim.DataflowJob{ID: id, Name: n, Region: region, Template: tpl, Type: "JOB_TYPE_BATCH", State: "JOB_STATE_RUNNING", Created: now, Params: params, Workers: c.Int("num-workers", 1)}
		if err := c.NeedProject("dataflow.jobs.create"); err != nil {
			return nil, err
		}
		switch tpl {
		case "Word_Count":
			in := params["inputFile"]
			out := params["output"]
			if in == "" || out == "" {
				return nil, fmt.Errorf("INVALID_ARGUMENT: The template parameters are invalid: inputFile and output are required.")
			}
			text := kingLear
			if !strings.HasPrefix(in, "gs://dataflow-samples/") {
				b, _, err := c.bucket(in)
				if err != nil {
					return nil, err
				}
				o := b.Objects[objectName(in)]
				if o == nil {
					return nil, fmt.Errorf("INVALID_ARGUMENT: No files matched spec: %s", in)
				}
				text = o.Content
			}
			ob, _, err := c.bucket(out)
			if err != nil {
				return nil, fmt.Errorf("INVALID_ARGUMENT: output location %s is not writable: bucket does not exist", out)
			}
			res := sim.WordCount(text)
			name := objectName(out) + "-00000-of-00001"
			ob.Objects[name] = &sim.Object{Name: name, Content: res, Size: len(res), StorageClass: ob.StorageClass, Generation: 1, Updated: now}
			j.State, j.Processed = "JOB_STATE_DONE", len(strings.Fields(text))
		case "PubSub_Subscription_to_BigQuery", "PubSub_to_BigQuery", "Cloud_PubSub_to_GCS_Text":
			j.Type = "JOB_TYPE_STREAMING"
			if tpl == "Cloud_PubSub_to_GCS_Text" {
				if params["inputTopic"] == "" || params["outputDirectory"] == "" {
					return nil, fmt.Errorf("INVALID_ARGUMENT: inputTopic and outputDirectory are required.")
				}
			} else if params["outputTableSpec"] == "" || (params["inputSubscription"] == "" && params["inputTopic"] == "") {
				return nil, fmt.Errorf("INVALID_ARGUMENT: outputTableSpec and inputSubscription (or inputTopic) are required.")
			}
			if s := params["inputSubscription"]; s != "" && p.Subs[s[strings.LastIndex(s, "/")+1:]] == nil {
				return nil, fmt.Errorf("INVALID_ARGUMENT: subscription %s does not exist", s)
			}
			if t := params["inputTopic"]; t != "" && p.Topics[t[strings.LastIndex(t, "/")+1:]] == nil {
				return nil, fmt.Errorf("INVALID_ARGUMENT: topic %s does not exist", t)
			}
		default:
			return nil, fmt.Errorf("INVALID_ARGUMENT: Template %s not found. Available: Word_Count, PubSub_Subscription_to_BigQuery, PubSub_to_BigQuery, Cloud_PubSub_to_GCS_Text.", loc)
		}
		p.DataflowJobs = append(p.DataflowJobs, j)
		c.Audit("dataflow.googleapis.com", "dataflow.jobs.create", "projects/"+p.ID+"/locations/"+region+"/jobs/"+id)
		return fmt.Sprintf("createTime: '%s'\ncurrentStateTime: '1970-01-01T00:00:00Z'\nid: %s\nlocation: %s\nname: %s\nprojectId: %s\nstartTime: '%s'\ntype: %s\n", now, id, region, n, p.ID, now, j.Type), nil
	})
	reg("dataflow jobs list", func(c *Cmd) (any, error) {
		p, err := c.project()
		if err != nil {
			return nil, err
		}
		var rows []any
		for i := len(p.DataflowJobs) - 1; i >= 0; i-- {
			j := p.DataflowJobs[i]
			if c.Str("status", "all") == "active" && j.State != "JOB_STATE_RUNNING" {
				continue
			}
			typ := "Batch"
			if j.Type == "JOB_TYPE_STREAMING" {
				typ = "Streaming"
			}
			rows = append(rows, map[string]any{"id": j.ID, "name": j.Name, "type": typ, "created": j.Created, "state": strings.Title(strings.ToLower(strings.TrimPrefix(j.State, "JOB_STATE_"))), "region": j.Region})
		}
		return Table{Cols: []Col{{"JOB_ID", "id"}, {"NAME", "name"}, {"TYPE", "type"}, {"CREATION_TIME", "created"}, {"STATE", "state"}, {"REGION", "region"}}, Rows: rows}, nil
	})
	reg("dataflow jobs describe", func(c *Cmd) (any, error) {
		_, j, err := getDF(c)
		if err != nil {
			return nil, err
		}
		return Obj{V: j}, nil
	})
	dfStop := func(state string) handler {
		return func(c *Cmd) (any, error) {
			p, j, err := getDF(c)
			if err != nil {
				return nil, err
			}
			if j.State != "JOB_STATE_RUNNING" {
				return nil, fmt.Errorf("FAILED_PRECONDITION: Job %s is not running (%s).", j.ID, j.State)
			}
			if state == "JOB_STATE_DRAINED" {
				if j.Type != "JOB_TYPE_STREAMING" {
					return nil, fmt.Errorf("FAILED_PRECONDITION: Only streaming jobs can be drained.")
				}
				c.S.State.DataflowDrain(p.ID, j)
			}
			j.State = state
			verb := "Cancelled"
			if state == "JOB_STATE_DRAINED" {
				verb = "Started draining"
			}
			return verb + " job [" + j.ID + "]\n", nil
		}
	}
	reg("dataflow jobs cancel", dfStop("JOB_STATE_CANCELLED"))
	reg("dataflow jobs drain", dfStop("JOB_STATE_DRAINED"))

	// ---- Dataproc ----------------------------------------------------------------------------
	getDP := func(c *Cmd) (*sim.Project, *sim.DataprocCluster, error) {
		p, err := c.project()
		if err != nil {
			return nil, nil, err
		}
		n, err := c.name("CLUSTER")
		if err != nil {
			return nil, nil, err
		}
		cl := p.DataprocClusters[n]
		if cl == nil {
			return nil, nil, fmt.Errorf("NOT_FOUND: Not found: Cluster projects/%s/regions/%s/clusters/%s", p.ID, c.Str("region", c.S.Region), n)
		}
		return p, cl, nil
	}
	reg("dataproc clusters create", func(c *Cmd) (any, error) {
		p, err := c.project()
		if err != nil {
			return nil, err
		}
		if err := c.API("dataproc.googleapis.com"); err != nil {
			return nil, err
		}
		n, err := c.name("CLUSTER")
		if err != nil {
			return nil, err
		}
		if p.DataprocClusters[n] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: Already exists: Failed to create cluster: Cluster projects/%s/regions/%s/clusters/%s", p.ID, c.Str("region", c.S.Region), n)
		}
		region := c.Str("region", "")
		if region == "" {
			return nil, fmt.Errorf("The required property [region] is not currently set. Use --region or set dataproc/region.")
		}
		single := c.Bool("single-node")
		workers := c.Int("num-workers", 2)
		if single {
			workers = 0
		} else if workers < 2 {
			return nil, fmt.Errorf("INVALID_ARGUMENT: Multi-node clusters must have at least 2 primary worker nodes. Use --single-node for a single VM.")
		}
		if err := c.NeedProject("dataproc.clusters.create"); err != nil {
			return nil, err
		}
		zone := c.Str("zone", region+"-b")
		cl := &sim.DataprocCluster{Name: n, Region: region, Zone: zone, MasterType: c.Str("master-machine-type", "n2-standard-4"), WorkerType: c.Str("worker-machine-type", "n2-standard-4"), Workers: workers, Preemptible: c.Int("num-secondary-workers", 0), ImageVersion: c.Str("image-version", "2.2-debian12"), SingleNode: single, State: "RUNNING", Created: c.S.State.Now()}
		p.DataprocClusters[n] = cl
		c.Audit("dataproc.googleapis.com", "google.cloud.dataproc.v1.ClusterController.CreateCluster", "projects/"+p.ID+"/regions/"+region+"/clusters/"+n)
		return fmt.Sprintf("Waiting on operation [projects/%s/regions/%s/operations/%s].\nWaiting for cluster creation operation...done.\nCreated [https://dataproc.googleapis.com/v1/projects/%s/regions/%s/clusters/%s] Cluster placed in zone [%s].\n", p.ID, region, c.S.State.ID(12), p.ID, region, n, zone), nil
	})
	reg("dataproc clusters list", func(c *Cmd) (any, error) {
		p, err := c.project()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.DataprocClusters) {
			cl := p.DataprocClusters[k]
			rows = append(rows, map[string]any{"name": k, "platform": "GCE", "workers": cl.Workers, "preempt": cl.Preemptible, "status": cl.State, "zone": cl.Zone})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"PLATFORM", "platform"}, {"PRIMARY_WORKER_COUNT", "workers"}, {"SECONDARY_WORKER_COUNT", "preempt"}, {"STATUS", "status"}, {"ZONE", "zone"}}, Rows: rows}, nil
	})
	reg("dataproc clusters describe", func(c *Cmd) (any, error) {
		_, cl, err := getDP(c)
		if err != nil {
			return nil, err
		}
		return Obj{V: cl}, nil
	})
	reg("dataproc clusters update", func(c *Cmd) (any, error) {
		_, cl, err := getDP(c)
		if err != nil {
			return nil, err
		}
		if cl.SingleNode && c.Has("num-workers") {
			return nil, fmt.Errorf("INVALID_ARGUMENT: Single node clusters cannot be scaled.")
		}
		if c.Has("num-workers") {
			cl.Workers = c.Int("num-workers", cl.Workers)
		}
		if c.Has("num-secondary-workers") {
			cl.Preemptible = c.Int("num-secondary-workers", cl.Preemptible)
		}
		return "Waiting for cluster update operation...done.\nUpdated [" + cl.Name + "].\n", nil
	})
	dpState := func(state string) handler {
		return func(c *Cmd) (any, error) {
			_, cl, err := getDP(c)
			if err != nil {
				return nil, err
			}
			cl.State = state
			return "Waiting for cluster operation...done.\n", nil
		}
	}
	reg("dataproc clusters stop", dpState("STOPPED"))
	reg("dataproc clusters start", dpState("RUNNING"))
	reg("dataproc clusters delete", func(c *Cmd) (any, error) {
		p, cl, err := getDP(c)
		if err != nil {
			return nil, err
		}
		delete(p.DataprocClusters, cl.Name)
		return "Waiting for cluster deletion operation...done.\nDeleted [" + cl.Name + "].\n", nil
	})
	submit := func(kind string) handler {
		return func(c *Cmd) (any, error) {
			p, err := c.project()
			if err != nil {
				return nil, err
			}
			name := c.Str("cluster", "")
			cl := p.DataprocClusters[name]
			if cl == nil {
				return nil, fmt.Errorf("NOT_FOUND: Cluster projects/%s/regions/%s/clusters/%s not found", p.ID, c.Str("region", c.S.Region), name)
			}
			if cl.State != "RUNNING" {
				return nil, fmt.Errorf("FAILED_PRECONDITION: Cluster %s is %s; jobs can only run on RUNNING clusters.", name, cl.State)
			}
			j := &sim.DataprocJob{ID: c.S.State.ID(32), Cluster: name, Region: cl.Region, Type: kind, Created: c.S.State.Now(), State: "DONE", Args: c.Rest}
			var out strings.Builder
			switch kind {
			case "spark":
				j.Main = c.Str("class", "")
				if j.Main == "" {
					return nil, fmt.Errorf("Exactly one of (--class | --jar) must be specified.")
				}
				if strings.HasSuffix(j.Main, "SparkPi") {
					n := 10
					if len(c.Rest) > 0 {
						n, _ = strconv.Atoi(c.Rest[0])
					}
					fmt.Fprintf(&out, "Pi is roughly %.6f\n", 3.14159265+float64(c.S.State.Rand().Intn(2000)-1000)/float64(100000*max(n, 1)))
				} else {
					out.WriteString("Job finished.\n")
				}
			case "pyspark":
				script, err := c.Arg(0, "PY_FILE")
				if err != nil {
					return nil, err
				}
				j.Main = script
				code := ""
				if strings.HasPrefix(script, "gs://") {
					b, _, err := c.bucket(script)
					if err != nil {
						return nil, err
					}
					o := b.Objects[objectName(script)]
					if o == nil {
						return nil, fmt.Errorf("INVALID_ARGUMENT: Google Cloud Storage object does not exist '%s'", script)
					}
					code = o.Content
				} else if v, ok := c.S.Files[c.S.path(script)]; ok {
					code = v
				} else {
					return nil, fmt.Errorf("ERROR: (gcloud.dataproc.jobs.submit.pyspark) File not found: %s", script)
				}
				if strings.Contains(code, "flatMap") || strings.Contains(strings.ToLower(code), "wordcount") || strings.Contains(code, "split") {
					for _, a := range c.Rest {
						if strings.HasPrefix(a, "gs://") {
							if b, _, err := c.bucket(a); err == nil {
								if o := b.Objects[objectName(a)]; o != nil {
									out.WriteString(sim.WordCount(o.Content))
									break
								}
							}
						}
					}
				}
				if m := regexp.MustCompile(`print\(\s*["']([^"']*)["']\s*\)`).FindAllStringSubmatch(code, -1); m != nil {
					for _, x := range m {
						out.WriteString(x[1] + "\n")
					}
				}
			case "spark-sql", "hive":
				j.Main = c.Str("execute", "")
				out.WriteString("OK\nTime taken: 1.2 seconds\n")
			}
			j.Output = out.String()
			p.DataprocJobs = append(p.DataprocJobs, j)
			c.Audit("dataproc.googleapis.com", "google.cloud.dataproc.v1.JobController.SubmitJob", "projects/"+p.ID+"/regions/"+cl.Region+"/jobs/"+j.ID)
			return fmt.Sprintf("Job [%s] submitted.\nWaiting for job output...\n%sJob [%s] finished successfully.\ndone: true\nstatus:\n  state: DONE\n", j.ID, j.Output, j.ID), nil
		}
	}
	reg("dataproc jobs submit spark", submit("spark"))
	reg("dataproc jobs submit pyspark", submit("pyspark"))
	reg("dataproc jobs submit spark-sql", submit("spark-sql"))
	reg("dataproc jobs submit hive", submit("hive"))
	reg("dataproc jobs list", func(c *Cmd) (any, error) {
		p, err := c.project()
		if err != nil {
			return nil, err
		}
		var rows []any
		for i := len(p.DataprocJobs) - 1; i >= 0; i-- {
			j := p.DataprocJobs[i]
			if c.Has("cluster") && c.Str("cluster", "") != j.Cluster {
				continue
			}
			rows = append(rows, map[string]any{"id": j.ID, "type": j.Type, "status": j.State})
		}
		return Table{Cols: []Col{{"JOB_ID", "id"}, {"TYPE", "type"}, {"STATUS", "status"}}, Rows: rows}, nil
	})
	reg("dataproc jobs describe", func(c *Cmd) (any, error) {
		p, err := c.project()
		if err != nil {
			return nil, err
		}
		id, _ := c.name("JOB")
		for _, j := range p.DataprocJobs {
			if j.ID == id {
				return Obj{V: j}, nil
			}
		}
		return nil, fmt.Errorf("NOT_FOUND: Job %s not found", id)
	})

	// ---- Cloud Composer --------------------------------------------------------------------
	getEnv := func(c *Cmd) (*sim.Project, *sim.ComposerEnv, error) {
		p, err := c.project()
		if err != nil {
			return nil, nil, err
		}
		n, err := c.name("ENVIRONMENT")
		if err != nil {
			n = c.Str("environment", "")
			if n == "" {
				return nil, nil, err
			}
		}
		e := p.Composer[n]
		if e == nil {
			return nil, nil, fmt.Errorf("NOT_FOUND: Environment projects/%s/locations/%s/environments/%s not found.", p.ID, c.Str("location", c.S.Region), n)
		}
		return p, e, nil
	}
	reg("composer environments create", func(c *Cmd) (any, error) {
		p, err := c.project()
		if err != nil {
			return nil, err
		}
		if err := c.API("composer.googleapis.com"); err != nil {
			return nil, err
		}
		n, err := c.name("ENVIRONMENT")
		if err != nil {
			return nil, err
		}
		loc := c.Str("location", "")
		if loc == "" {
			return nil, fmt.Errorf("argument --location: Must be specified.")
		}
		if p.Composer[n] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: Environment %s already exists.", n)
		}
		if err := c.NeedProject("composer.environments.create"); err != nil {
			return nil, err
		}
		bucket := loc + "-" + n + "-" + c.S.State.ID(8) + "-bucket"
		p.Buckets[bucket] = &sim.Bucket{Name: bucket, Project: p.ID, Location: strings.ToUpper(loc), StorageClass: "STANDARD", Objects: map[string]*sim.Object{}, UBLA: true}
		e := &sim.ComposerEnv{Name: n, Location: loc, State: "RUNNING", ImageVersion: c.Str("image-version", "composer-3-airflow-2.10.2"), Size: strings.ToUpper(c.Str("environment-size", "small")), Bucket: bucket, DagPrefix: "gs://" + bucket + "/dags",
			AirflowURI: "https://" + c.S.State.ID(10) + "-dot-" + loc + ".composer.googleusercontent.com"}
		p.Composer[n] = e
		c.Audit("composer.googleapis.com", "google.cloud.orchestration.airflow.service.v1.Environments.CreateEnvironment", "projects/"+p.ID+"/locations/"+loc+"/environments/"+n)
		return fmt.Sprintf("Waiting for [projects/%s/locations/%s/environments/%s] to be created with [projects/%s/locations/%s/operations/%s]...done.\n", p.ID, loc, n, p.ID, loc, c.S.State.ID(12)), nil
	})
	reg("composer environments list", func(c *Cmd) (any, error) {
		p, err := c.project()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Composer) {
			e := p.Composer[k]
			rows = append(rows, map[string]any{"name": k, "location": e.Location, "state": e.State, "create": e.ImageVersion})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"LOCATION", "location"}, {"STATE", "state"}, {"IMAGE_VERSION", "create"}}, Rows: rows}, nil
	})
	reg("composer environments describe", func(c *Cmd) (any, error) {
		_, e, err := getEnv(c)
		if err != nil {
			return nil, err
		}
		return Obj{V: map[string]any{"name": e.Name, "state": e.State, "config": map[string]any{"airflowUri": e.AirflowURI, "dagGcsPrefix": e.DagPrefix, "environmentSize": "ENVIRONMENT_SIZE_" + e.Size, "softwareConfig": map[string]any{"imageVersion": e.ImageVersion}}}}, nil
	})
	reg("composer environments delete", func(c *Cmd) (any, error) {
		p, e, err := getEnv(c)
		if err != nil {
			return nil, err
		}
		delete(p.Composer, e.Name)
		return "Deleted [" + e.Name + "]. The environment bucket " + e.Bucket + " is kept.\n", nil
	})
	reg("composer environments storage dags import", func(c *Cmd) (any, error) {
		p, e, err := getEnv(c)
		if err != nil {
			return nil, err
		}
		src := c.Str("source", "")
		code, ok := c.S.Files[c.S.path(src)]
		if !ok {
			return nil, fmt.Errorf("source %s not found in Cloud Shell", src)
		}
		b := p.Buckets[e.Bucket]
		obj := "dags/" + src[strings.LastIndex(src, "/")+1:]
		b.Objects[obj] = &sim.Object{Name: obj, Content: code, Size: len(code), StorageClass: "STANDARD", Generation: 1, Updated: c.S.State.Now()}
		return "Uploading " + src + " to gs://" + e.Bucket + "/dags/\n", nil
	})
	reg("composer environments storage dags list", func(c *Cmd) (any, error) {
		p, e, err := getEnv(c)
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		b.WriteString("NAME\n")
		for _, k := range sim.SortedKeys(p.Buckets[e.Bucket].Objects) {
			if strings.HasPrefix(k, "dags/") {
				b.WriteString(k + "\n")
			}
		}
		return b.String(), nil
	})
	reg("composer environments run", func(c *Cmd) (any, error) {
		p, e, err := getEnv(c)
		if err != nil {
			return nil, err
		}
		dags := map[string]bool{}
		if b := p.Buckets[e.Bucket]; b != nil {
			for k, o := range b.Objects {
				if strings.HasPrefix(k, "dags/") {
					for _, m := range reDagID.FindAllStringSubmatch(o.Content, -1) {
						dags[m[1]] = true
					}
				}
			}
		}
		// gcloud composer environments run ENV --location L dags list|trigger -- ARGS
		sub := ""
		if n := len(c.Args) - len(c.Rest); n > 1 {
			sub = strings.Join(c.Args[1:n], " ")
		}
		switch sub {
		case "dags list", "list_dags":
			ids := make([]string, 0, len(dags))
			for k := range dags {
				ids = append(ids, k)
			}
			sort.Strings(ids)
			rows := make([][]any, len(ids))
			for i, d := range ids {
				rows[i] = []any{d, "/home/airflow/gcs/dags/" + d + ".py", "airflow", "False"}
			}
			return columns([]string{"dag_id", "fileloc", "owners", "is_paused"}, rows), nil
		case "dags trigger", "trigger_dag":
			if len(c.Rest) == 0 {
				return nil, fmt.Errorf("the DAG id must be given after --, e.g. -- my_dag")
			}
			id := c.Rest[len(c.Rest)-1]
			if !dags[id] {
				return nil, fmt.Errorf("Dag id %s not found in DagModel. Upload it with gcloud composer environments storage dags import.", id)
			}
			run := sim.DagRun{DAG: id, RunID: "manual__" + c.S.State.Now(), State: "success", At: c.S.State.Now()}
			e.Runs = append(e.Runs, run)
			return fmt.Sprintf("Executing within the following Kubernetes cluster namespace: composer-user-workloads\nCreated <DagRun %s @ %s: %s, state:queued>\n", id, run.At, run.RunID), nil
		case "dags list-runs":
			var rows [][]any
			for _, r := range e.Runs {
				if len(c.Rest) == 0 || contains(c.Rest, r.DAG) {
					rows = append(rows, []any{r.DAG, r.RunID, r.State, r.At})
				}
			}
			return columns([]string{"dag_id", "run_id", "state", "execution_date"}, rows), nil
		}
		return nil, fmt.Errorf("unsupported Airflow command %q (try: dags list, dags trigger -- DAG_ID, dags list-runs -- -d DAG_ID)", sub)
	})

	// ---- Filestore -----------------------------------------------------------------------------
	reg("filestore instances create", func(c *Cmd) (any, error) {
		p, err := c.project()
		if err != nil {
			return nil, err
		}
		if err := c.API("file.googleapis.com"); err != nil {
			return nil, err
		}
		n, err := c.name("INSTANCE")
		if err != nil {
			return nil, err
		}
		if p.Filestore[n] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: Instance %s already exists.", n)
		}
		loc := c.Str("location", c.Str("zone", ""))
		if loc == "" {
			return nil, fmt.Errorf("argument --location (or --zone): Must be specified.")
		}
		share := c.KV("file-share")
		nw := c.KV("network")["name"]
		if nw == "" {
			nw = "default"
		}
		if p.Networks[nw] == nil {
			return nil, fmt.Errorf("INVALID_ARGUMENT: network %s not found", nw)
		}
		capGB := parseCapacity(share["capacity"])
		tier := strings.ToUpper(c.Str("tier", "BASIC_HDD"))
		if min := map[string]int{"BASIC_HDD": 1024, "BASIC_SSD": 2560, "ZONAL": 1024, "REGIONAL": 1024, "ENTERPRISE": 1024}[tier]; capGB < min {
			return nil, fmt.Errorf("INVALID_ARGUMENT: capacity must be at least %d GB for tier %s", min, tier)
		}
		if err := c.NeedProject("file.instances.create"); err != nil {
			return nil, err
		}
		fi := &sim.FilestoreInstance{Name: n, Location: loc, Tier: tier, Share: share["name"], CapacityGB: capGB, IP: c.S.State.AllocIP("10.51.20.0/29"), Network: nw, State: "READY"}
		if fi.Share == "" {
			return nil, fmt.Errorf("argument --file-share: name is required (e.g. --file-share=name=vol1,capacity=1TB).")
		}
		p.Filestore[n] = fi
		c.Audit("file.googleapis.com", "google.cloud.filestore.v1.CloudFilestoreManager.CreateInstance", "projects/"+p.ID+"/locations/"+loc+"/instances/"+n)
		return "Waiting for [operation-" + c.S.State.ID(12) + "] to finish...done.\n", nil
	})
	reg("filestore instances list", func(c *Cmd) (any, error) {
		p, err := c.project()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Filestore) {
			f := p.Filestore[k]
			rows = append(rows, map[string]any{"name": k, "loc": f.Location, "tier": f.Tier, "cap": f.CapacityGB, "share": f.Share, "ip": f.IP, "state": f.State})
		}
		return Table{Cols: []Col{{"INSTANCE_NAME", "name"}, {"LOCATION", "loc"}, {"TIER", "tier"}, {"CAPACITY_GB", "cap"}, {"FILE_SHARE_NAME", "share"}, {"IP_ADDRESS", "ip"}, {"STATE", "state"}}, Rows: rows}, nil
	})
	reg("filestore instances describe", func(c *Cmd) (any, error) {
		p, err := c.project()
		if err != nil {
			return nil, err
		}
		n, _ := c.name("INSTANCE")
		if f := p.Filestore[n]; f != nil {
			return Obj{V: f}, nil
		}
		return nil, fmt.Errorf("NOT_FOUND: instance %s not found", n)
	})
	reg("filestore instances delete", func(c *Cmd) (any, error) {
		p, err := c.project()
		if err != nil {
			return nil, err
		}
		n, _ := c.name("INSTANCE")
		if p.Filestore[n] == nil {
			return nil, fmt.Errorf("NOT_FOUND: instance %s not found", n)
		}
		delete(p.Filestore, n)
		return "Deleted instance [" + n + "].\n", nil
	})
}

func parseCapacity(s string) int {
	s = strings.ToUpper(strings.TrimSpace(s))
	mult := 1
	switch {
	case strings.HasSuffix(s, "TB"), strings.HasSuffix(s, "TIB"):
		mult = 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "TIB"), "TB")
	case strings.HasSuffix(s, "GB"), strings.HasSuffix(s, "GIB"):
		s = strings.TrimSuffix(strings.TrimSuffix(s, "GIB"), "GB")
	}
	n, _ := strconv.Atoi(s)
	return n * mult
}

// spannerDDL applies DDL statements to a Spanner database.
func spannerDDL(d *sim.SpannerDB, ddl string) error {
	c, err := sqlengine.New()
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Load(d.Data); err != nil {
		return err
	}
	var applied []string
	for _, st := range sqlengine.Split(ddl) {
		st = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(st), ";"))
		if st == "" {
			continue
		}
		up := strings.ToUpper(st)
		if strings.HasPrefix(up, "CREATE TABLE") && !strings.Contains(up, "PRIMARY KEY") {
			return fmt.Errorf("INVALID_ARGUMENT: Error parsing Spanner DDL statement: %s : Expecting 'PRIMARY' but found 'EOF' (Spanner tables need PRIMARY KEY (...) after the column list).", st)
		}
		if _, err := c.Exec(sim.SpannerToSQLite(st)); err != nil {
			return fmt.Errorf("FAILED_PRECONDITION: %s", strings.TrimPrefix(err.Error(), "SQL logic error: "))
		}
		applied = append(applied, st)
	}
	dump, err := c.Dump()
	if err != nil {
		return err
	}
	d.Data = dump
	d.DDL = append(d.DDL, applied...)
	return nil
}

// spannerExec runs GoogleSQL on a Spanner database and prints like gcloud.
func spannerExec(d *sim.SpannerDB, sql string) (string, error) {
	c, err := sqlengine.New()
	if err != nil {
		return "", err
	}
	defer c.Close()
	if err := c.Load(d.Data); err != nil {
		return "", err
	}
	q := regexp.MustCompile("`([^`]+)`").ReplaceAllString(sql, `"$1"`)
	up := strings.ToUpper(strings.TrimSpace(q))
	if strings.HasPrefix(up, "CREATE") || strings.HasPrefix(up, "ALTER") || strings.HasPrefix(up, "DROP") {
		return "", fmt.Errorf("INVALID_ARGUMENT: DDL statements are not supported by execute-sql. Use gcloud spanner databases ddl update --ddl=...")
	}
	res, err := c.Exec(q)
	if err != nil {
		msg := strings.TrimPrefix(err.Error(), "SQL logic error: ")
		msg = regexp.MustCompile(`\s*\(\d+\)$`).ReplaceAllString(msg, "")
		if m := regexp.MustCompile(`no such table: (?:main\.)?(\S+)`).FindStringSubmatch(msg); m != nil {
			return "", fmt.Errorf("INVALID_ARGUMENT: Table not found: %s [at 1:1]", m[1])
		}
		if m := regexp.MustCompile(`no such column: (\S+)`).FindStringSubmatch(msg); m != nil {
			return "", fmt.Errorf("INVALID_ARGUMENT: Unrecognized name: %s [at 1:1]", m[1])
		}
		if strings.Contains(msg, "UNIQUE constraint") {
			return "", fmt.Errorf("ALREADY_EXISTS: Row already exists (%s)", msg)
		}
		return "", fmt.Errorf("INVALID_ARGUMENT: %s", msg)
	}
	if dump, err := c.Dump(); err == nil {
		d.Data = dump
	}
	if len(res) == 0 {
		return "", nil
	}
	r := res[len(res)-1]
	if !r.Query {
		return fmt.Sprintf("Statement modified %d row%s\n", r.Affected, map[bool]string{true: "", false: "s"}[r.Affected == 1]), nil
	}
	return columns(r.Columns, r.Rows), nil
}

const kingLear = `Meantime we shall express our darker purpose.
Give me the map there. Know that we have divided
In three our kingdom: and 'tis our fast intent
To shake all cares and business from our age;
Conferring them on younger strengths, while we
Unburthen'd crawl toward death. Our son of Cornwall,
And you, our no less loving son of Albany,
We have this hour a constant will to publish
Our daughters' several dowers, that future strife
May be prevented now. Nothing will come of nothing: speak again.`

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
