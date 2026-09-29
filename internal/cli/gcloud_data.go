package cli

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

// ---- Cloud SQL ----------------------------------------------------------------

func sqlRes(p *sim.Project, n string) sim.Resource {
	return sim.Resource{Project: p.ID, Type: "sqladmin.googleapis.com/Instance", Name: "projects/" + p.ID + "/instances/" + n, Service: "sqladmin.googleapis.com"}
}

func (c *Cmd) sqlInstance(name string) (*sim.Project, *sim.SQLInstance, error) {
	p, err := c.P()
	if err != nil {
		return nil, nil, err
	}
	in := p.SQLInstances[name]
	if in == nil {
		return nil, nil, fmt.Errorf("HTTPError 404: The Cloud SQL instance does not exist.")
	}
	return p, in, nil
}

func tierFromCPU(cpu, mem string) string {
	if cpu == "" {
		return ""
	}
	m := strings.TrimSuffix(strings.TrimSuffix(mem, "GB"), "GiB")
	gb, _ := strconv.ParseFloat(m, 64)
	return fmt.Sprintf("db-custom-%s-%d", cpu, int(gb*1024))
}

func (c *Cmd) sqlPatch(p *sim.Project, in *sim.SQLInstance) error {
	if v := c.Str("tier", tierFromCPU(c.Str("cpu", ""), c.Str("memory", ""))); v != "" {
		in.Tier = v
	}
	if v := c.Str("availability-type", ""); v != "" {
		in.Availability = strings.ToUpper(v)
	}
	if c.Has("authorized-networks") {
		in.AuthorizedNets = c.List("authorized-networks")
		if in.PublicIP == "" {
			return fmt.Errorf("Authorized networks require a public IP (use --assign-ip).")
		}
	}
	if c.Bool("clear-authorized-networks") {
		in.AuthorizedNets = nil
	}
	if v := c.Str("backup-start-time", ""); v != "" {
		in.BackupsEnabled, in.BackupStart = true, v
	}
	if c.Bool("no-backup") {
		in.BackupsEnabled = false
	}
	if c.Bool("enable-point-in-time-recovery") {
		if !in.BackupsEnabled {
			return fmt.Errorf("Point-in-time recovery requires automated backups (--backup-start-time).")
		}
		in.PITR = true
	}
	if c.Bool("no-enable-point-in-time-recovery") {
		in.PITR = false
	}
	if c.Has("database-flags") {
		in.Flags = c.KV("database-flags")
	}
	if c.Bool("clear-database-flags") {
		in.Flags = map[string]string{}
	}
	if c.Bool("require-ssl") {
		in.RequireSSL = true
	}
	if c.Bool("deletion-protection") {
		in.DeletionProtection = true
	}
	if c.Bool("no-deletion-protection") {
		in.DeletionProtection = false
	}
	if c.Bool("no-assign-ip") {
		if in.PrivateIP == "" {
			return fmt.Errorf("Cannot remove the public IP of an instance without a private IP.")
		}
		in.PublicIP, in.AuthorizedNets = "", nil
	}
	if c.Bool("assign-ip") && in.PublicIP == "" {
		if c.S.State.OrgPolicyEnforced(p.ID, "sql.restrictPublicIp") {
			return fmt.Errorf("Constraint constraints/sql.restrictPublicIp violated.")
		}
		in.PublicIP = c.S.State.ExternalIP()
	}
	if v := c.Str("network", ""); v != "" {
		v = v[strings.LastIndex(v, "/")+1:]
		nw := p.Networks[v]
		if nw == nil {
			return fmt.Errorf("network %s not found", v)
		}
		if !nw.PSAConnected {
			return fmt.Errorf("HTTPError 400: Invalid request: Incorrect Service Networking config for instance: %s:%s:SERVICE_NETWORKING_NOT_ENABLED. Create a private services access connection first (gcloud services vpc-peerings connect).", p.ID, in.Name)
		}
		in.Network = v
		if in.PrivateIP == "" {
			in.PrivateIP = c.S.State.AllocIP(psaRange(p, nw))
		}
	}
	if v := c.Str("activation-policy", ""); v != "" {
		if strings.ToUpper(v) == "NEVER" {
			in.State = "STOPPED"
		} else {
			in.State = "RUNNABLE"
		}
	}
	return nil
}

func psaRange(p *sim.Project, nw *sim.Network) string {
	for _, r := range nw.PSARanges {
		if a := p.Addresses[r]; a != nil {
			return fmt.Sprintf("%s/%d", a.Address, a.Prefix)
		}
	}
	return "10.200.0.0/16"
}

func sqlView(p *sim.Project, in *sim.SQLInstance) map[string]any {
	var ips []any
	if in.PublicIP != "" {
		ips = append(ips, map[string]any{"type": "PRIMARY", "ipAddress": in.PublicIP})
	}
	if in.PrivateIP != "" {
		ips = append(ips, map[string]any{"type": "PRIVATE", "ipAddress": in.PrivateIP})
	}
	var an []any
	for _, a := range in.AuthorizedNets {
		an = append(an, map[string]any{"value": a})
	}
	var flags []any
	for _, k := range sim.SortedKeys(in.Flags) {
		flags = append(flags, map[string]any{"name": k, "value": in.Flags[k]})
	}
	return map[string]any{"name": in.Name, "databaseVersion": in.Version, "region": in.Region, "state": in.State, "connectionName": p.ID + ":" + in.Region + ":" + in.Name,
		"ipAddresses": ips, "gceZone": in.Region + "-b", "masterInstanceName": in.Master, "replicaNames": in.Replicas,
		"settings": map[string]any{"tier": in.Tier, "availabilityType": in.Availability, "databaseFlags": flags, "deletionProtectionEnabled": in.DeletionProtection,
			"backupConfiguration": map[string]any{"enabled": in.BackupsEnabled, "startTime": in.BackupStart, "pointInTimeRecoveryEnabled": in.PITR},
			"ipConfiguration":     map[string]any{"ipv4Enabled": in.PublicIP != "", "privateNetwork": in.Network, "authorizedNetworks": an, "requireSsl": in.RequireSSL}},
		"primaryAddress": in.PublicIP, "privateAddress": in.PrivateIP, "serviceAccountEmailAddress": sqlAgent(p)}
}

// sqlAgent is the Cloud SQL service agent that reads/writes import/export files.
func sqlAgent(p *sim.Project) string {
	return "p" + p.Number + "-sql@gcp-sa-cloud-sql.iam.gserviceaccount.com"
}

func init() {
	reg("sql instances create", func(c *Cmd) (any, error) {
		if err := c.API("sqladmin.googleapis.com"); err != nil {
			return nil, err
		}
		p, _ := c.P()
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		if p.SQLInstances[n] != nil {
			return nil, fmt.Errorf("HTTPError 409: The Cloud SQL instance already exists.")
		}
		if err := c.NeedProject("cloudsql.instances.create"); err != nil {
			return nil, err
		}
		in := &sim.SQLInstance{Name: n, Version: c.Str("database-version", "POSTGRES_15"), Tier: "db-custom-1-3840", Availability: "ZONAL", State: "RUNNABLE",
			Flags: map[string]string{}, Databases: []string{"postgres"}, Users: map[string]string{"postgres": c.Str("root-password", "")}}
		if m := c.Str("master-instance-name", ""); m != "" {
			master := p.SQLInstances[m]
			if master == nil {
				return nil, fmt.Errorf("master instance %s not found", m)
			}
			if !master.BackupsEnabled && strings.HasPrefix(master.Version, "MYSQL") {
				return nil, fmt.Errorf("Replicas require binary logging / backups enabled on the primary.")
			}
			*in = *master
			in.Name, in.Master, in.Replicas, in.Backups = n, m, nil, nil
			in.Users = copyStrMap(master.Users)
			in.Flags = copyStrMap(master.Flags)
			in.Region = c.Str("region", master.Region)
			master.Replicas = append(master.Replicas, n)
		} else {
			region, err := c.RegionFlag()
			if err != nil {
				return nil, err
			}
			in.Region = region
		}
		if err := c.sqlPatch(p, in); err != nil {
			return nil, err
		}
		if !c.Bool("no-assign-ip") && in.PublicIP == "" && in.Master == "" {
			if c.S.State.OrgPolicyEnforced(p.ID, "sql.restrictPublicIp") {
				return nil, fmt.Errorf("HTTPError 400: Invalid request: Organization Policy check failure: the external IP of this instance violates the constraints/sql.restrictPublicIp enforced at the %s project.", p.ID)
			}
			in.PublicIP = c.S.State.ExternalIP()
		}
		if in.Master != "" && c.Bool("no-assign-ip") {
			in.PublicIP = ""
		}
		if in.Master != "" && in.Network != "" {
			in.PrivateIP = c.S.State.AllocIP(psaRange(p, p.Networks[in.Network]))
		}
		if in.PublicIP == "" && in.PrivateIP == "" {
			return nil, fmt.Errorf("HTTPError 400: At least one of public IP or private IP must be enabled.")
		}
		p.SQLInstances[n] = in
		c.Audit("cloudsql.googleapis.com", "cloudsql.instances.create", "projects/"+p.ID+"/instances/"+n)
		t, _ := render(Table{Cols: sqlCols, Rows: []any{sqlView(p, in)}}, Flags{})
		return fmt.Sprintf("Creating Cloud SQL instance for %s...done.\nCreated [https://sqladmin.googleapis.com/sql/v1beta4/projects/%s/instances/%s].\n%s", in.Version, p.ID, n, t), nil
	})
	reg("sql instances list", func(c *Cmd) (any, error) {
		if err := c.API("sqladmin.googleapis.com"); err != nil {
			return nil, err
		}
		p, _ := c.P()
		var rows []any
		for _, k := range sim.SortedKeys(p.SQLInstances) {
			rows = append(rows, sqlView(p, p.SQLInstances[k]))
		}
		return Table{Cols: sqlCols, Rows: rows}, nil
	})
	reg("sql instances describe", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "INSTANCE")
		if err != nil {
			return nil, err
		}
		p, in, err := c.sqlInstance(n)
		if err != nil {
			return nil, err
		}
		if err := c.Need("cloudsql.instances.get", sqlRes(p, n)); err != nil {
			return nil, err
		}
		return Obj{V: sqlView(p, in)}, nil
	})
	reg("sql instances patch", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "INSTANCE")
		if err != nil {
			return nil, err
		}
		p, in, err := c.sqlInstance(n)
		if err != nil {
			return nil, err
		}
		if err := c.Need("cloudsql.instances.update", sqlRes(p, n)); err != nil {
			return nil, err
		}
		if err := c.sqlPatch(p, in); err != nil {
			return nil, err
		}
		c.Audit("cloudsql.googleapis.com", "cloudsql.instances.update", "projects/"+p.ID+"/instances/"+n)
		return "Patching Cloud SQL instance...done.\nUpdated [https://sqladmin.googleapis.com/sql/v1beta4/projects/" + p.ID + "/instances/" + n + "].\n", nil
	})
	reg("sql instances delete", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "INSTANCE")
		if err != nil {
			return nil, err
		}
		p, in, err := c.sqlInstance(n)
		if err != nil {
			return nil, err
		}
		if in.DeletionProtection {
			return nil, fmt.Errorf("HTTPError 400: The instance is protected from deletion (deletion protection is enabled).")
		}
		if err := c.Need("cloudsql.instances.delete", sqlRes(p, n)); err != nil {
			return nil, err
		}
		delete(p.SQLInstances, n)
		c.Audit("cloudsql.googleapis.com", "cloudsql.instances.delete", "projects/"+p.ID+"/instances/"+n)
		return "Deleting Cloud SQL instance...done.\n", nil
	})
	reg("sql instances restart", func(c *Cmd) (any, error) {
		n, _ := c.Arg(0, "INSTANCE")
		p, in, err := c.sqlInstance(n)
		if err != nil {
			return nil, err
		}
		in.State = "RUNNABLE"
		c.Audit("cloudsql.googleapis.com", "cloudsql.instances.restart", "projects/"+p.ID+"/instances/"+n)
		return "Restarting Cloud SQL instance...done.\n", nil
	})
	reg("sql instances failover", func(c *Cmd) (any, error) {
		n, _ := c.Arg(0, "INSTANCE")
		_, in, err := c.sqlInstance(n)
		if err != nil {
			return nil, err
		}
		if in.Availability != "REGIONAL" {
			return nil, fmt.Errorf("HTTPError 400: The instance is not configured for high availability (availabilityType=ZONAL).")
		}
		return "Failing over Cloud SQL instance...done.\n", nil
	})
	reg("sql instances promote-replica", func(c *Cmd) (any, error) {
		n, _ := c.Arg(0, "REPLICA")
		p, in, err := c.sqlInstance(n)
		if err != nil {
			return nil, err
		}
		if in.Master == "" {
			return nil, fmt.Errorf("instance %s is not a replica", n)
		}
		if m := p.SQLInstances[in.Master]; m != nil {
			var keep []string
			for _, r := range m.Replicas {
				if r != n {
					keep = append(keep, r)
				}
			}
			m.Replicas = keep
		}
		in.Master = ""
		return "Promoting read replica...done.\n", nil
	})
	reg("sql databases create", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "DATABASE")
		if err != nil {
			return nil, err
		}
		p, in, err := c.sqlInstance(c.Str("instance", ""))
		if err != nil {
			return nil, err
		}
		if err := c.Need("cloudsql.databases.create", sqlRes(p, in.Name)); err != nil {
			return nil, err
		}
		if contains(in.Databases, n) {
			return nil, fmt.Errorf("HTTPError 400: database %s already exists", n)
		}
		in.Databases = append(in.Databases, n)
		c.Audit("cloudsql.googleapis.com", "cloudsql.databases.create", "projects/"+p.ID+"/instances/"+in.Name+"/databases/"+n)
		return "Creating Cloud SQL database...done.\nCreated database [" + n + "].\n", nil
	})
	reg("sql databases delete", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "DATABASE")
		if err != nil {
			return nil, err
		}
		p, in, err := c.sqlInstance(c.Str("instance", ""))
		if err != nil {
			return nil, err
		}
		if err := c.Need("cloudsql.databases.delete", sqlRes(p, in.Name)); err != nil {
			return nil, err
		}
		var keep []string
		found := false
		for _, d := range in.Databases {
			if d == n {
				found = true
				continue
			}
			keep = append(keep, d)
		}
		if !found {
			return nil, fmt.Errorf("HTTPError 404: database %s not found", n)
		}
		in.Databases = keep
		c.Audit("cloudsql.googleapis.com", "cloudsql.databases.delete", "projects/"+p.ID+"/instances/"+in.Name+"/databases/"+n)
		return "Deleted database [" + n + "].\n", nil
	})
	// sql import / export: the Cloud SQL service agent (not the caller) reads or
	// writes the file, so it needs access to the bucket.
	for _, verb := range []string{"import", "export"} {
		verb := verb
		reg("sql "+verb+" sql", func(c *Cmd) (any, error) {
			n, err := c.Arg(0, "INSTANCE")
			if err != nil {
				return nil, err
			}
			uri, err := c.Arg(1, "URI")
			if err != nil {
				return nil, err
			}
			p, in, err := c.sqlInstance(n)
			if err != nil {
				return nil, err
			}
			db := c.Str("database", "")
			if db == "" {
				return nil, fmt.Errorf("argument --database: Must be specified.")
			}
			if !contains(in.Databases, db) {
				return nil, fmt.Errorf("HTTPError 400: database %q does not exist on %s", db, n)
			}
			perm := "cloudsql.instances." + verb
			if err := c.Need(perm, sqlRes(p, n)); err != nil {
				return nil, err
			}
			bn, obj, _ := strings.Cut(strings.TrimPrefix(uri, "gs://"), "/")
			b, _ := c.S.State.FindBucket(bn)
			if b == nil {
				return nil, fmt.Errorf("HTTPError 400: bucket %s not found", bn)
			}
			agent := "serviceAccount:" + sqlAgent(p)
			if verb == "import" {
				if b.Objects[obj] == nil {
					return nil, fmt.Errorf("HTTPError 400: file %s not found", uri)
				}
				if !c.S.State.Allowed(agent, "storage.objects.get", c.S.State.BucketResource(b, obj)) {
					return nil, fmt.Errorf("HTTPError 403: The service account %s does not have the required permissions for the bucket. Grant it roles/storage.objectViewer on gs://%s.", sqlAgent(p), bn)
				}
				in.Flags["sim-imported-"+db] = uri
				c.Audit("cloudsql.googleapis.com", "cloudsql.instances.import", "projects/"+p.ID+"/instances/"+n)
				return "Importing data into Cloud SQL instance...done.\nImported data from [" + uri + "] into [" + n + "].\n", nil
			}
			if !c.S.State.Allowed(agent, "storage.objects.create", c.S.State.BucketResource(b, obj)) {
				return nil, fmt.Errorf("HTTPError 403: The service account %s does not have the required permissions for the bucket. Grant it roles/storage.objectCreator on gs://%s.", sqlAgent(p), bn)
			}
			b.Objects[obj] = &sim.Object{Name: obj, Content: "-- pg_dump of " + db, Size: 1 << 20, StorageClass: b.StorageClass, Updated: c.S.State.Now()}
			c.Audit("cloudsql.googleapis.com", "cloudsql.instances.export", "projects/"+p.ID+"/instances/"+n)
			return "Exporting Cloud SQL instance...done.\nExported [" + n + "] to [" + uri + "].\n", nil
		})
	}
	reg("sql databases list", func(c *Cmd) (any, error) {
		_, in, err := c.sqlInstance(c.Str("instance", ""))
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, d := range in.Databases {
			rows = append(rows, map[string]any{"name": d, "charset": "UTF8", "collation": "en_US.UTF8"})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"CHARSET", "charset"}, {"COLLATION", "collation"}}, Rows: rows}, nil
	})
	reg("sql users create", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "USERNAME")
		if err != nil {
			return nil, err
		}
		p, in, err := c.sqlInstance(c.Str("instance", ""))
		if err != nil {
			return nil, err
		}
		if err := c.Need("cloudsql.users.create", sqlRes(p, in.Name)); err != nil {
			return nil, err
		}
		in.Users[n] = c.Str("password", "")
		c.Audit("cloudsql.googleapis.com", "cloudsql.users.create", "projects/"+p.ID+"/instances/"+in.Name+"/users/"+n)
		return "Creating Cloud SQL user...done.\nCreated user [" + n + "].\n", nil
	})
	reg("sql users set-password", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "USERNAME")
		if err != nil {
			return nil, err
		}
		p, in, err := c.sqlInstance(c.Str("instance", ""))
		if err != nil {
			return nil, err
		}
		if _, ok := in.Users[n]; !ok {
			return nil, fmt.Errorf("HTTPError 404: user %s does not exist", n)
		}
		if err := c.Need("cloudsql.users.update", sqlRes(p, in.Name)); err != nil {
			return nil, err
		}
		in.Users[n] = c.Str("password", "")
		c.Audit("cloudsql.googleapis.com", "cloudsql.users.update", "projects/"+p.ID+"/instances/"+in.Name+"/users/"+n)
		return "Updating Cloud SQL user...done.\n", nil
	})
	reg("sql users list", func(c *Cmd) (any, error) {
		_, in, err := c.sqlInstance(c.Str("instance", ""))
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, u := range sim.SortedKeys(in.Users) {
			rows = append(rows, map[string]any{"name": u, "type": "BUILT_IN"})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"TYPE", "type"}}, Rows: rows}, nil
	})
	reg("sql users delete", func(c *Cmd) (any, error) {
		n, _ := c.Arg(0, "USERNAME")
		_, in, err := c.sqlInstance(c.Str("instance", ""))
		if err != nil {
			return nil, err
		}
		delete(in.Users, n)
		return "Deleting Cloud SQL user...done.\n", nil
	})
	reg("sql backups create", func(c *Cmd) (any, error) {
		p, in, err := c.sqlInstance(c.Str("instance", ""))
		if err != nil {
			return nil, err
		}
		if err := c.Need("cloudsql.backupRuns.create", sqlRes(p, in.Name)); err != nil {
			return nil, err
		}
		id := fmt.Sprintf("17%011d", len(in.Backups)+1)
		in.Backups = append(in.Backups, sim.SQLBackup{ID: id, Status: "SUCCESSFUL", Time: c.S.State.Now(), Databases: append([]string{}, in.Databases...)})
		c.Audit("cloudsql.googleapis.com", "cloudsql.backupRuns.create", "projects/"+p.ID+"/instances/"+in.Name)
		return "Backing up Cloud SQL instance...done.\n[" + id + "] Backup created.\n", nil
	})
	reg("sql backups list", func(c *Cmd) (any, error) {
		_, in, err := c.sqlInstance(c.Str("instance", ""))
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, b := range in.Backups {
			rows = append(rows, b)
		}
		return Table{Cols: []Col{{"ID", "id"}, {"WINDOW_START_TIME", "windowStartTime"}, {"STATUS", "status"}}, Rows: rows}, nil
	})
	reg("sql backups restore", func(c *Cmd) (any, error) {
		id, _ := c.Arg(0, "BACKUP_ID")
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		target := p.SQLInstances[c.Str("restore-instance", "")]
		src := p.SQLInstances[c.Str("backup-instance", c.Str("restore-instance", ""))]
		if target == nil || src == nil {
			return nil, fmt.Errorf("instance not found")
		}
		for _, b := range src.Backups {
			if b.ID == id {
				if target.Master != "" {
					return nil, fmt.Errorf("HTTPError 400: cannot restore a backup to a read replica")
				}
				target.State = "RUNNABLE"
				target.SlowQueries = nil
				if b.Databases != nil {
					// a restore overwrites all data on the target instance
					target.Databases = append([]string{}, b.Databases...)
				}
				c.Audit("cloudsql.googleapis.com", "cloudsql.instances.restoreBackup", "projects/"+p.ID+"/instances/"+target.Name)
				return "Restoring Cloud SQL instance...done.\n", nil
			}
		}
		return nil, fmt.Errorf("backup %s not found", id)
	})
	reg("sql connect", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "INSTANCE")
		if err != nil {
			return nil, err
		}
		p, in, err := c.sqlInstance(n)
		if err != nil {
			return nil, err
		}
		if err := c.Need("cloudsql.instances.connect", sqlRes(p, n)); err != nil {
			return nil, err
		}
		if in.PublicIP == "" {
			return nil, fmt.Errorf("It seems your client does not have ipv6 connectivity and the database instance does not have an ipv4 address. (Private-IP-only instance: connect from a VM in the VPC or through the Cloud SQL Auth Proxy.)")
		}
		user := c.Str("user", "postgres")
		if _, ok := in.Users[user]; !ok {
			return nil, fmt.Errorf("FATAL: password authentication failed for user %q", user)
		}
		db := c.Str("database", "postgres")
		return fmt.Sprintf("Allowlisting your IP for incoming connection for 5 minutes...done.\nConnecting to database with SQL user [%s].\npsql (15.8)\n%s=> (interactive shell not available — run `psql \"host=%s user=%s dbname=%s\" -c \"SQL\"` with PGPASSWORD set)\n", user, db, in.PublicIP, user, db), nil
	})

	// ---- Pub/Sub --------------------------------------------------------------
	topicRes := func(p *sim.Project, n string) sim.Resource {
		return sim.Resource{Project: p.ID, Type: "pubsub.googleapis.com/Topic", Name: "projects/" + p.ID + "/topics/" + n, Service: "pubsub.googleapis.com"}
	}
	short := func(n string) string { return n[strings.LastIndex(n, "/")+1:] }
	reg("pubsub topics create", func(c *Cmd) (any, error) {
		if err := c.API("pubsub.googleapis.com"); err != nil {
			return nil, err
		}
		p, _ := c.P()
		out := ""
		for _, n := range c.Args {
			n = short(n)
			if p.Topics[n] != nil {
				return nil, fmt.Errorf("Failed to create topic [projects/%s/topics/%s]: Resource already exists in the project (resource=%s).", p.ID, n, n)
			}
			if err := c.Need("pubsub.topics.create", topicRes(p, n)); err != nil {
				return nil, err
			}
			p.Topics[n] = &sim.Topic{Name: n, Retention: c.Str("message-retention-duration", ""), KMSKey: c.Str("topic-encryption-key", "")}
			c.Audit("pubsub.googleapis.com", "google.pubsub.v1.Publisher.CreateTopic", "projects/"+p.ID+"/topics/"+n)
			out += "Created topic [projects/" + p.ID + "/topics/" + n + "].\n"
		}
		return out, nil
	})
	reg("pubsub topics list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Topics) {
			rows = append(rows, map[string]any{"name": "projects/" + p.ID + "/topics/" + k})
		}
		return Table{Cols: []Col{{"NAME", "name"}}, Rows: rows}, nil
	})
	reg("pubsub topics describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "TOPIC")
		t := p.Topics[short(n)]
		if t == nil {
			return nil, fmt.Errorf("NOT_FOUND: Resource not found (resource=%s).", n)
		}
		return Obj{V: map[string]any{"name": "projects/" + p.ID + "/topics/" + t.Name, "messageRetentionDuration": t.Retention, "kmsKeyName": t.KMSKey}}, nil
	})
	reg("pubsub topics delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		for _, n := range c.Args {
			n = short(n)
			if p.Topics[n] == nil {
				return nil, fmt.Errorf("NOT_FOUND: Resource not found (resource=%s).", n)
			}
			if err := c.Need("pubsub.topics.delete", topicRes(p, n)); err != nil {
				return nil, err
			}
			delete(p.Topics, n)
			for _, s := range p.Subs {
				if s.Topic == n {
					s.Topic = "_deleted-topic_"
				}
			}
		}
		return "Deleted.\n", nil
	})
	reg("pubsub topics publish", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.Arg(0, "TOPIC")
		if err != nil {
			return nil, err
		}
		n = short(n)
		if p.Topics[n] == nil {
			return nil, fmt.Errorf("NOT_FOUND: Resource not found (resource=%s).", n)
		}
		if err := c.Need("pubsub.topics.publish", topicRes(p, n)); err != nil {
			return nil, err
		}
		id, err := c.S.State.Publish(p.ID, n, c.Str("message", ""), c.KV("attribute"))
		if err != nil {
			return nil, err
		}
		return "messageIds:\n- '" + id + "'\n", nil
	})
	subRes := func(p *sim.Project, n string) sim.Resource {
		return sim.Resource{Project: p.ID, Type: "pubsub.googleapis.com/Subscription", Name: "projects/" + p.ID + "/subscriptions/" + n, Service: "pubsub.googleapis.com"}
	}
	applySub := func(c *Cmd, p *sim.Project, s *sim.Subscription) error {
		if c.Has("ack-deadline") {
			s.AckDeadline = c.Int("ack-deadline", 10)
		}
		if v := c.Str("push-endpoint", ""); v != "" {
			s.PushEndpoint = v
		}
		if v := c.Str("push-auth-service-account", ""); v != "" {
			s.PushSA = v
		}
		if v := c.Str("dead-letter-topic", ""); v != "" {
			if p.Topics[short(v)] == nil {
				return fmt.Errorf("NOT_FOUND: dead letter topic %s not found", v)
			}
			s.DeadLetterTopic = short(v)
			s.MaxDeliveryAttempts = c.Int("max-delivery-attempts", 5)
			if s.MaxDeliveryAttempts < 5 || s.MaxDeliveryAttempts > 100 {
				return fmt.Errorf("INVALID_ARGUMENT: max-delivery-attempts must be between 5 and 100")
			}
		}
		if c.Bool("clear-dead-letter-policy") {
			s.DeadLetterTopic, s.MaxDeliveryAttempts = "", 0
		}
		if v := c.Str("bigquery-table", ""); v != "" {
			s.BQTable = v
		}
		if v := c.Str("message-filter", ""); v != "" {
			s.Filter = v
		}
		if v := c.Str("message-retention-duration", ""); v != "" {
			s.Retention = v
		}
		if c.Bool("enable-exactly-once-delivery") {
			s.ExactlyOnce = true
		}
		if c.Bool("clear-push-config") {
			s.PushEndpoint = ""
		}
		return nil
	}
	reg("pubsub subscriptions create", func(c *Cmd) (any, error) {
		if err := c.API("pubsub.googleapis.com"); err != nil {
			return nil, err
		}
		p, _ := c.P()
		n, err := c.Arg(0, "SUBSCRIPTION")
		if err != nil {
			return nil, err
		}
		n = short(n)
		t := short(c.Str("topic", ""))
		if p.Topics[t] == nil {
			return nil, fmt.Errorf("NOT_FOUND: Resource not found (resource=%s).", t)
		}
		if p.Subs[n] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: Resource already exists in the project (resource=%s).", n)
		}
		if err := c.Need("pubsub.subscriptions.create", subRes(p, n)); err != nil {
			return nil, err
		}
		s := &sim.Subscription{Name: n, Topic: t, AckDeadline: 10}
		if err := applySub(c, p, s); err != nil {
			return nil, err
		}
		p.Subs[n] = s
		c.Audit("pubsub.googleapis.com", "google.pubsub.v1.Subscriber.CreateSubscription", "projects/"+p.ID+"/subscriptions/"+n)
		out := "Created subscription [projects/" + p.ID + "/subscriptions/" + n + "].\n"
		if s.DeadLetterTopic != "" {
			out += "WARNING: the Pub/Sub service agent needs roles/pubsub.publisher on the dead letter topic and roles/pubsub.subscriber on this subscription.\n"
		}
		return out, nil
	})
	reg("pubsub subscriptions update", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "SUBSCRIPTION")
		s := p.Subs[short(n)]
		if s == nil {
			return nil, fmt.Errorf("NOT_FOUND: Resource not found (resource=%s).", n)
		}
		if err := c.Need("pubsub.subscriptions.update", subRes(p, s.Name)); err != nil {
			return nil, err
		}
		if err := applySub(c, p, s); err != nil {
			return nil, err
		}
		c.Audit("pubsub.googleapis.com", "google.pubsub.v1.Subscriber.UpdateSubscription", "projects/"+p.ID+"/subscriptions/"+s.Name)
		return "Updated subscription [projects/" + p.ID + "/subscriptions/" + s.Name + "].\n", nil
	})
	reg("pubsub subscriptions modify-push-config", registry["pubsub subscriptions update"])
	reg("pubsub subscriptions list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Subs) {
			s := p.Subs[k]
			rows = append(rows, map[string]any{"name": "projects/" + p.ID + "/subscriptions/" + k, "topic": "projects/" + p.ID + "/topics/" + s.Topic, "pushConfig": map[string]any{"pushEndpoint": s.PushEndpoint}, "ackDeadlineSeconds": s.AckDeadline, "deadLetterPolicy": map[string]any{"deadLetterTopic": s.DeadLetterTopic, "maxDeliveryAttempts": s.MaxDeliveryAttempts}})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"TOPIC", "topic"}}, Rows: rows}, nil
	})
	reg("pubsub subscriptions describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "SUBSCRIPTION")
		s := p.Subs[short(n)]
		if s == nil {
			return nil, fmt.Errorf("NOT_FOUND: Resource not found (resource=%s).", n)
		}
		return Obj{V: map[string]any{"name": "projects/" + p.ID + "/subscriptions/" + s.Name, "topic": "projects/" + p.ID + "/topics/" + s.Topic, "ackDeadlineSeconds": s.AckDeadline,
			"pushConfig":       map[string]any{"pushEndpoint": s.PushEndpoint, "oidcToken": map[string]any{"serviceAccountEmail": s.PushSA}},
			"deadLetterPolicy": map[string]any{"deadLetterTopic": s.DeadLetterTopic, "maxDeliveryAttempts": s.MaxDeliveryAttempts},
			"bigqueryConfig":   map[string]any{"table": s.BQTable}, "filter": s.Filter, "numUndeliveredMessages": len(s.Backlog), "ackedMessages": s.Acked, "deadLettered": s.DeadLettered}}, nil
	})
	reg("pubsub subscriptions delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		for _, n := range c.Args {
			delete(p.Subs, short(n))
		}
		return "Deleted.\n", nil
	})
	reg("pubsub subscriptions pull", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.Arg(0, "SUBSCRIPTION")
		if err != nil {
			return nil, err
		}
		s := p.Subs[short(n)]
		if s == nil {
			return nil, fmt.Errorf("NOT_FOUND: Resource not found (resource=%s).", n)
		}
		if err := c.Need("pubsub.subscriptions.consume", subRes(p, s.Name)); err != nil {
			return nil, err
		}
		if s.PushEndpoint != "" {
			return nil, fmt.Errorf("FAILED_PRECONDITION: pull is not allowed on a push subscription")
		}
		limit := c.Int("limit", 1)
		var rows []any
		var keep []sim.Message
		for i, m := range s.Backlog {
			if i < limit {
				rows = append(rows, map[string]any{"data": m.Data, "messageId": m.ID, "attributes": m.Attributes, "deliveryAttempt": m.Attempts + 1})
				if !c.Bool("auto-ack") {
					keep = append(keep, m)
				} else {
					s.Acked++
				}
			} else {
				keep = append(keep, m)
			}
		}
		s.Backlog = keep
		if len(rows) == 0 {
			return "Listed 0 items.\n", nil
		}
		return Table{Cols: []Col{{"DATA", "data"}, {"MESSAGE_ID", "messageId"}, {"ATTRIBUTES", "attributes"}, {"DELIVERY_ATTEMPT", "deliveryAttempt"}}, Rows: rows}, nil
	})
	reg("pubsub subscriptions seek", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "SUBSCRIPTION")
		s := p.Subs[short(n)]
		if s == nil {
			return nil, fmt.Errorf("NOT_FOUND")
		}
		return "Seeked subscription [" + s.Name + "].\n", nil
	})
	topicIAM := func(add bool, kind string) handler {
		return func(c *Cmd) (any, error) {
			p, err := c.P()
			if err != nil {
				return nil, err
			}
			n, _ := c.Arg(0, "NAME")
			n = short(n)
			if (kind == "topics" && p.Topics[n] == nil) || (kind == "subscriptions" && p.Subs[n] == nil) {
				return nil, fmt.Errorf("NOT_FOUND: Resource not found (resource=%s).", n)
			}
			m, role, cond, err := c.bindingArgs()
			if err != nil {
				return nil, err
			}
			// Topic/subscription policies are modelled on the project with a resource condition.
			if cond == nil {
				cond = &sim.Condition{Title: kind + "/" + n, Expression: fmt.Sprintf(`resource.name == "projects/%s/%s/%s"`, p.ID, kind, n)}
			}
			if add {
				p.IAM.AddBinding(role, m, cond)
			} else {
				p.IAM.RemoveBinding(role, m)
			}
			c.Audit("pubsub.googleapis.com", "google.iam.v1.IAMPolicy.SetIamPolicy", "projects/"+p.ID+"/"+kind+"/"+n)
			return "Updated IAM policy for " + strings.TrimSuffix(kind, "s") + " [" + n + "].\n", nil
		}
	}
	reg("pubsub topics add-iam-policy-binding", topicIAM(true, "topics"))
	reg("pubsub topics remove-iam-policy-binding", topicIAM(false, "topics"))
	reg("pubsub subscriptions add-iam-policy-binding", topicIAM(true, "subscriptions"))
	reg("pubsub subscriptions remove-iam-policy-binding", topicIAM(false, "subscriptions"))

	// ---- Secret Manager --------------------------------------------------------
	secRes := func(p *sim.Project, s *sim.Secret) sim.Resource {
		return sim.Resource{Project: p.ID, Type: "secretmanager.googleapis.com/Secret", Name: "projects/" + p.ID + "/secrets/" + s.Name, Service: "secretmanager.googleapis.com", Policies: []*sim.Policy{&s.IAM}}
	}
	getSec := func(c *Cmd, n string) (*sim.Project, *sim.Secret, error) {
		p, err := c.P()
		if err != nil {
			return nil, nil, err
		}
		s := p.Secrets[short(n)]
		if s == nil {
			return nil, nil, fmt.Errorf("NOT_FOUND: Secret [projects/%s/secrets/%s] not found or has no versions.", p.Number, n)
		}
		return p, s, nil
	}
	readData := func(c *Cmd) (string, error) {
		f := c.Str("data-file", "")
		if f == "-" {
			return strings.TrimSuffix(c.Stdin, "\n"), nil
		}
		if f == "" {
			return "", fmt.Errorf("argument --data-file: Must be specified (use - for stdin).")
		}
		v, ok := c.S.Files[c.S.path(f)]
		if !ok {
			return "", fmt.Errorf("Unable to read file [%s]", f)
		}
		return strings.TrimSuffix(v, "\n"), nil
	}
	reg("secrets create", func(c *Cmd) (any, error) {
		if err := c.API("secretmanager.googleapis.com"); err != nil {
			return nil, err
		}
		p, _ := c.P()
		n, err := c.name0()
		if err != nil {
			n, err = c.Arg(0, "SECRET")
			if err != nil {
				return nil, err
			}
		}
		if p.Secrets[n] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: Secret [projects/%s/secrets/%s] already exists.", p.Number, n)
		}
		if err := c.NeedProject("secretmanager.secrets.create"); err != nil {
			return nil, err
		}
		s := &sim.Secret{Name: n, Replication: c.Str("replication-policy", "automatic"), Labels: c.KV("labels"), KMSKey: c.Str("kms-key-name", ""), Rotation: c.Str("rotation-period", "")}
		if c.Has("data-file") {
			d, err := readData(c)
			if err != nil {
				return nil, err
			}
			s.Versions = append(s.Versions, sim.SecretVersion{ID: 1, Data: d, State: "ENABLED"})
		}
		p.Secrets[n] = s
		c.Audit("secretmanager.googleapis.com", "google.cloud.secretmanager.v1.SecretManagerService.CreateSecret", "projects/"+p.ID+"/secrets/"+n)
		out := "Created secret [" + n + "].\n"
		if len(s.Versions) > 0 {
			out = "Created version [1] of the secret [" + n + "].\n"
		}
		return out, nil
	})
	reg("secrets list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("secretmanager.secrets.list"); err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Secrets) {
			rows = append(rows, map[string]any{"name": k, "createTime": "2026-09-01T09:00:00Z", "replication": p.Secrets[k].Replication})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"CREATED", "createTime"}, {"REPLICATION_POLICY", "replication"}}, Rows: rows}, nil
	})
	reg("secrets describe", func(c *Cmd) (any, error) {
		n, _ := c.Arg(0, "SECRET")
		_, s, err := getSec(c, n)
		if err != nil {
			return nil, err
		}
		return Obj{V: map[string]any{"name": s.Name, "replication": s.Replication, "labels": s.Labels, "rotation": s.Rotation, "kmsKeyName": s.KMSKey}}, nil
	})
	reg("secrets delete", func(c *Cmd) (any, error) {
		n, _ := c.Arg(0, "SECRET")
		p, s, err := getSec(c, n)
		if err != nil {
			return nil, err
		}
		if err := c.Need("secretmanager.secrets.delete", secRes(p, s)); err != nil {
			return nil, err
		}
		delete(p.Secrets, s.Name)
		return "Deleted secret [" + s.Name + "].\n", nil
	})
	reg("secrets versions add", func(c *Cmd) (any, error) {
		n, err := c.Arg(0, "SECRET")
		if err != nil {
			return nil, err
		}
		p, s, err := getSec(c, n)
		if err != nil {
			return nil, err
		}
		if err := c.Need("secretmanager.versions.add", secRes(p, s)); err != nil {
			return nil, err
		}
		d, err := readData(c)
		if err != nil {
			return nil, err
		}
		id := len(s.Versions) + 1
		s.Versions = append(s.Versions, sim.SecretVersion{ID: id, Data: d, State: "ENABLED"})
		c.Audit("secretmanager.googleapis.com", "google.cloud.secretmanager.v1.SecretManagerService.AddSecretVersion", fmt.Sprintf("projects/%s/secrets/%s/versions/%d", p.ID, s.Name, id))
		return fmt.Sprintf("Created version [%d] of the secret [%s].\n", id, s.Name), nil
	})
	reg("secrets versions access", func(c *Cmd) (any, error) {
		v, err := c.Arg(0, "VERSION")
		if err != nil {
			return nil, err
		}
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		data, e := c.S.State.AccessSecret(p.ID, c.Principal(), c.Str("secret", "")+":"+v)
		if e != "" {
			return nil, fmt.Errorf("%s", strings.Replace(e, "for Revision service account", "for", 1))
		}
		c.S.State.Log(p.ID, sim.LogEntry{Severity: "INFO", LogName: "cloudaudit.googleapis.com%2Fdata_access", Resource: sim.LogResource{Type: "audited_resource", Labels: map[string]string{"service": "secretmanager.googleapis.com"}},
			Proto: map[string]string{"methodName": "google.cloud.secretmanager.v1.SecretManagerService.AccessSecretVersion", "resourceName": "projects/" + p.ID + "/secrets/" + c.Str("secret", "") + "/versions/" + v, "authenticationInfo.principalEmail": strings.SplitN(c.Principal(), ":", 2)[1]}})
		return data + "\n", nil
	})
	reg("secrets versions list", func(c *Cmd) (any, error) {
		n, _ := c.Arg(0, "SECRET")
		_, s, err := getSec(c, n)
		if err != nil {
			return nil, err
		}
		var rows []any
		for i := len(s.Versions) - 1; i >= 0; i-- {
			v := s.Versions[i]
			rows = append(rows, map[string]any{"name": v.ID, "state": v.State, "createTime": "2026-09-01T09:00:00Z"})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"STATE", "state"}, {"CREATED", "createTime"}}, Rows: rows}, nil
	})
	versionState := func(state string) handler {
		return func(c *Cmd) (any, error) {
			v, err := c.Arg(0, "VERSION")
			if err != nil {
				return nil, err
			}
			p, s, err := getSec(c, c.Str("secret", ""))
			if err != nil {
				return nil, err
			}
			perm := map[string]string{"DISABLED": "secretmanager.versions.disable", "ENABLED": "secretmanager.versions.enable", "DESTROYED": "secretmanager.versions.destroy"}[state]
			if err := c.Need(perm, secRes(p, s)); err != nil {
				return nil, err
			}
			id, _ := strconv.Atoi(v)
			for i := range s.Versions {
				if s.Versions[i].ID == id {
					if s.Versions[i].State == "DESTROYED" {
						return nil, fmt.Errorf("FAILED_PRECONDITION: version is destroyed")
					}
					s.Versions[i].State = state
					if state == "DESTROYED" {
						s.Versions[i].Data = ""
					}
					c.Audit("secretmanager.googleapis.com", "google.cloud.secretmanager.v1.SecretManagerService."+strings.Title(strings.ToLower(state[:len(state)-1])), fmt.Sprintf("projects/%s/secrets/%s/versions/%d", p.ID, s.Name, id))
					return fmt.Sprintf("%s version [%d] of the secret [%s].\n", strings.Title(strings.ToLower(state)), id, s.Name), nil
				}
			}
			return nil, fmt.Errorf("NOT_FOUND: version %s", v)
		}
	}
	reg("secrets versions disable", versionState("DISABLED"))
	reg("secrets versions enable", versionState("ENABLED"))
	reg("secrets versions destroy", versionState("DESTROYED"))
	secIAM := func(add bool) handler {
		return func(c *Cmd) (any, error) {
			n, _ := c.Arg(0, "SECRET")
			p, s, err := getSec(c, n)
			if err != nil {
				return nil, err
			}
			if err := c.Need("secretmanager.secrets.setIamPolicy", secRes(p, s)); err != nil {
				return nil, err
			}
			m, role, cond, err := c.bindingArgs()
			if err != nil {
				return nil, err
			}
			if add {
				s.IAM.AddBinding(role, m, cond)
			} else if !s.IAM.RemoveBinding(role, m) {
				return nil, fmt.Errorf("Policy binding with the specified principal, role, and condition not found!")
			}
			c.Audit("secretmanager.googleapis.com", "google.iam.v1.IAMPolicy.SetIamPolicy", "projects/"+p.ID+"/secrets/"+s.Name)
			r, _ := render(policyView(&s.IAM), Flags{})
			return "Updated IAM policy for secret [" + s.Name + "].\n" + r, nil
		}
	}
	reg("secrets add-iam-policy-binding", secIAM(true))
	reg("secrets remove-iam-policy-binding", secIAM(false))
	reg("secrets get-iam-policy", func(c *Cmd) (any, error) {
		n, _ := c.Arg(0, "SECRET")
		_, s, err := getSec(c, n)
		if err != nil {
			return nil, err
		}
		return policyView(&s.IAM), nil
	})
	reg("secrets update", func(c *Cmd) (any, error) {
		n, _ := c.Arg(0, "SECRET")
		_, s, err := getSec(c, n)
		if err != nil {
			return nil, err
		}
		if v := c.Str("rotation-period", ""); v != "" {
			s.Rotation = v
		}
		for k, v := range c.KV("update-labels") {
			s.Labels[k] = v
		}
		return "Updated secret [" + s.Name + "].\n", nil
	})

	// ---- Cloud KMS -----------------------------------------------------------------
	keyOf := func(c *Cmd, name string) (*sim.Project, *sim.KeyRing, *sim.CryptoKey, error) {
		p, err := c.P()
		if err != nil {
			return nil, nil, nil, err
		}
		kr := p.KeyRings[c.Str("keyring", "")]
		if kr == nil {
			return nil, nil, nil, fmt.Errorf("NOT_FOUND: KeyRing %s not found.", c.Str("keyring", ""))
		}
		k := kr.Keys[name]
		if k == nil {
			return nil, nil, nil, fmt.Errorf("NOT_FOUND: CryptoKey %s not found.", name)
		}
		return p, kr, k, nil
	}
	keyRes := func(p *sim.Project, kr *sim.KeyRing, k *sim.CryptoKey) sim.Resource {
		return sim.Resource{Project: p.ID, Type: "cloudkms.googleapis.com/CryptoKey", Name: "projects/" + p.ID + "/locations/" + kr.Location + "/keyRings/" + kr.Name + "/cryptoKeys/" + k.Name, Service: "cloudkms.googleapis.com", Policies: []*sim.Policy{&k.IAM}}
	}
	reg("kms keyrings create", func(c *Cmd) (any, error) {
		if err := c.API("cloudkms.googleapis.com"); err != nil {
			return nil, err
		}
		p, _ := c.P()
		n, err := c.Arg(0, "KEYRING")
		if err != nil {
			return nil, err
		}
		if p.KeyRings[n] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: KeyRing projects/%s/locations/%s/keyRings/%s already exists.", p.ID, c.Str("location", ""), n)
		}
		if err := c.NeedProject("cloudkms.keyRings.create"); err != nil {
			return nil, err
		}
		loc := c.Str("location", "")
		if loc == "" {
			return nil, fmt.Errorf("argument --location: Must be specified.")
		}
		p.KeyRings[n] = &sim.KeyRing{Name: n, Location: loc, Keys: map[string]*sim.CryptoKey{}}
		c.Audit("cloudkms.googleapis.com", "CreateKeyRing", "projects/"+p.ID+"/locations/"+loc+"/keyRings/"+n)
		return "", nil
	})
	reg("kms keyrings list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.KeyRings) {
			kr := p.KeyRings[k]
			rows = append(rows, map[string]any{"name": "projects/" + p.ID + "/locations/" + kr.Location + "/keyRings/" + k})
		}
		return Table{Cols: []Col{{"NAME", "name"}}, Rows: rows}, nil
	})
	reg("kms keys create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.Arg(0, "KEY")
		if err != nil {
			return nil, err
		}
		kr := p.KeyRings[c.Str("keyring", "")]
		if kr == nil {
			return nil, fmt.Errorf("NOT_FOUND: KeyRing %s not found.", c.Str("keyring", ""))
		}
		if kr.Keys[n] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: CryptoKey %s already exists.", n)
		}
		if err := c.NeedProject("cloudkms.cryptoKeys.create"); err != nil {
			return nil, err
		}
		purpose := map[string]string{"encryption": "ENCRYPT_DECRYPT", "asymmetric-signing": "ASYMMETRIC_SIGN", "asymmetric-encryption": "ASYMMETRIC_DECRYPT", "mac": "MAC"}[c.Str("purpose", "encryption")]
		if purpose == "" {
			return nil, fmt.Errorf("argument --purpose: Invalid choice")
		}
		k := &sim.CryptoKey{Name: n, Purpose: purpose, RotationPeriod: c.Str("rotation-period", ""), NextRotation: c.Str("next-rotation-time", ""), Protection: strings.ToUpper(c.Str("protection-level", "software")), Versions: []sim.KeyVersion{{ID: 1, State: "ENABLED"}}}
		if k.RotationPeriod != "" && k.NextRotation == "" {
			return nil, fmt.Errorf("--next-rotation-time must be specified together with --rotation-period")
		}
		kr.Keys[n] = k
		c.Audit("cloudkms.googleapis.com", "CreateCryptoKey", "projects/"+p.ID+"/locations/"+kr.Location+"/keyRings/"+kr.Name+"/cryptoKeys/"+n)
		return "", nil
	})
	reg("kms keys list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		kr := p.KeyRings[c.Str("keyring", "")]
		if kr == nil {
			return nil, fmt.Errorf("NOT_FOUND: KeyRing not found")
		}
		var rows []any
		for _, n := range sim.SortedKeys(kr.Keys) {
			k := kr.Keys[n]
			rows = append(rows, map[string]any{"name": "projects/" + p.ID + "/locations/" + kr.Location + "/keyRings/" + kr.Name + "/cryptoKeys/" + n, "purpose": k.Purpose, "rotationPeriod": k.RotationPeriod, "protectionLevel": k.Protection})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"PURPOSE", "purpose"}, {"ROTATION_PERIOD", "rotationPeriod"}, {"PROTECTION_LEVEL", "protectionLevel"}}, Rows: rows}, nil
	})
	reg("kms keys describe", func(c *Cmd) (any, error) {
		n, _ := c.Arg(0, "KEY")
		_, _, k, err := keyOf(c, n)
		if err != nil {
			return nil, err
		}
		return Obj{V: k}, nil
	})
	reg("kms keys update", func(c *Cmd) (any, error) {
		n, _ := c.Arg(0, "KEY")
		p, kr, k, err := keyOf(c, n)
		if err != nil {
			return nil, err
		}
		if err := c.Need("cloudkms.cryptoKeys.update", keyRes(p, kr, k)); err != nil {
			return nil, err
		}
		if v := c.Str("rotation-period", ""); v != "" {
			k.RotationPeriod = v
			k.NextRotation = c.Str("next-rotation-time", c.S.State.Now())
		}
		if c.Bool("remove-rotation-schedule") {
			k.RotationPeriod, k.NextRotation = "", ""
		}
		if v := c.Str("primary-version", ""); v != "" {
			_ = v
		}
		c.Audit("cloudkms.googleapis.com", "UpdateCryptoKey", keyRes(p, kr, k).Name)
		return "", nil
	})
	kmsIAM := func(add bool) handler {
		return func(c *Cmd) (any, error) {
			n, _ := c.Arg(0, "KEY")
			p, kr, k, err := keyOf(c, n)
			if err != nil {
				return nil, err
			}
			if err := c.Need("cloudkms.cryptoKeys.setIamPolicy", keyRes(p, kr, k)); err != nil {
				return nil, err
			}
			m, err := member(c.Str("member", ""))
			if err != nil {
				return nil, err
			}
			role := c.Str("role", "")
			if add {
				k.IAM.AddBinding(role, m, nil)
			} else if !k.IAM.RemoveBinding(role, m) {
				return nil, fmt.Errorf("Policy binding with the specified principal, role, and condition not found!")
			}
			c.Audit("cloudkms.googleapis.com", "SetIamPolicy", keyRes(p, kr, k).Name)
			r, _ := render(policyView(&k.IAM), Flags{})
			return "Updated IAM policy for key [" + n + "].\n" + r, nil
		}
	}
	reg("kms keys add-iam-policy-binding", kmsIAM(true))
	reg("kms keys remove-iam-policy-binding", kmsIAM(false))
	reg("kms keys get-iam-policy", func(c *Cmd) (any, error) {
		n, _ := c.Arg(0, "KEY")
		_, _, k, err := keyOf(c, n)
		if err != nil {
			return nil, err
		}
		return policyView(&k.IAM), nil
	})
	reg("kms keys versions list", func(c *Cmd) (any, error) {
		_, _, k, err := keyOf(c, c.Str("key", ""))
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, v := range k.Versions {
			rows = append(rows, v)
		}
		return Table{Cols: []Col{{"NAME", "id"}, {"STATE", "state"}}, Rows: rows}, nil
	})
	reg("kms keys versions create", func(c *Cmd) (any, error) {
		_, _, k, err := keyOf(c, c.Str("key", ""))
		if err != nil {
			return nil, err
		}
		k.Versions = append(k.Versions, sim.KeyVersion{ID: len(k.Versions) + 1, State: "ENABLED"})
		return "", nil
	})
	reg("kms keys versions destroy", func(c *Cmd) (any, error) {
		v, _ := c.Arg(0, "VERSION")
		_, _, k, err := keyOf(c, c.Str("key", ""))
		if err != nil {
			return nil, err
		}
		id, _ := strconv.Atoi(v)
		for i := range k.Versions {
			if k.Versions[i].ID == id {
				k.Versions[i].State = "DESTROY_SCHEDULED"
			}
		}
		return "", nil
	})
	kmsCrypt := func(encrypt bool) handler {
		return func(c *Cmd) (any, error) {
			p, kr, k, err := keyOf(c, c.Str("key", ""))
			if err != nil {
				return nil, err
			}
			perm := "cloudkms.cryptoKeyVersions.useToEncrypt"
			if !encrypt {
				perm = "cloudkms.cryptoKeyVersions.useToDecrypt"
			}
			if err := c.Need(perm, keyRes(p, kr, k)); err != nil {
				return nil, err
			}
			inF, outF := c.Str("plaintext-file", ""), c.Str("ciphertext-file", "")
			if !encrypt {
				inF, outF = c.Str("ciphertext-file", ""), c.Str("plaintext-file", "")
			}
			var in string
			if inF == "-" {
				in = c.Stdin
			} else {
				v, ok := c.S.Files[c.S.path(inF)]
				if !ok {
					return nil, fmt.Errorf("Failed to read file [%s]", inF)
				}
				in = v
			}
			var out string
			if encrypt {
				out = "KMSCT:" + k.Name + ":" + base64.StdEncoding.EncodeToString([]byte(in))
			} else {
				pre := "KMSCT:" + k.Name + ":"
				if !strings.HasPrefix(in, pre) {
					return nil, fmt.Errorf("INVALID_ARGUMENT: Decryption failed: the ciphertext is invalid or was encrypted with a different key.")
				}
				b, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(in, pre))
				out = string(b)
			}
			if outF == "-" {
				return out, nil
			}
			c.S.Files[c.S.path(outF)] = out
			return "", nil
		}
	}
	reg("kms encrypt", kmsCrypt(true))
	reg("kms decrypt", kmsCrypt(false))
}

var sqlCols = []Col{{"NAME", "name"}, {"DATABASE_VERSION", "databaseVersion"}, {"LOCATION", "gceZone"}, {"TIER", "settings.tier"}, {"PRIMARY_ADDRESS", "primaryAddress"}, {"PRIVATE_ADDRESS", "privateAddress"}, {"STATUS", "state"}}
