package cli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
	"gopkg.in/yaml.v3"
)

func logView(e sim.LogEntry) map[string]any {
	m := map[string]any{"timestamp": e.Timestamp, "severity": e.Severity, "logName": e.LogName, "resource": map[string]any{"type": e.Resource.Type, "labels": e.Resource.Labels}}
	if e.Text != "" {
		m["textPayload"] = e.Text
	}
	if e.HTTP != nil {
		m["httpRequest"] = map[string]any{"requestMethod": e.HTTP.Method, "requestUrl": e.HTTP.URL, "status": e.HTTP.Status, "latency": e.HTTP.Latency}
	}
	if len(e.Proto) > 0 {
		pp := map[string]any{}
		for k, v := range e.Proto {
			if strings.HasPrefix(k, "authenticationInfo.") {
				pp["authenticationInfo"] = map[string]any{"principalEmail": v}
			} else {
				pp[k] = v
			}
		}
		m["protoPayload"] = pp
	}
	return m
}

func init() {
	// ---- Cloud Logging --------------------------------------------------------
	reg("logging read", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		filter := strings.Join(c.Args, " ")
		if err := c.NeedProject("logging.logEntries.list"); err != nil {
			return nil, err
		}
		if strings.Contains(filter, "data_access") && !c.S.State.Allowed(c.Principal(), "logging.privateLogEntries.list", sim.ProjectResource(p.ID)) {
			return nil, fmt.Errorf("PERMISSION_DENIED: Data Access audit logs require roles/logging.privateLogViewer")
		}
		entries := c.S.State.QueryLogs(p.ID, filter, c.Int("limit", 50))
		var rows []any
		for _, e := range entries {
			rows = append(rows, logView(e))
		}
		if c.Str("format", "") == "" {
			c.F["format"] = []string{"yaml"}
		}
		return Table{Cols: []Col{{"TIMESTAMP", "timestamp"}, {"SEVERITY", "severity"}, {"TEXT", "textPayload"}}, Rows: rows}, nil
	})
	reg("logging logs list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		var rows []any
		for _, e := range c.S.State.Logs {
			if e.Project == p.ID && !seen[e.LogName] {
				seen[e.LogName] = true
				rows = append(rows, map[string]any{"name": e.LogName})
			}
		}
		return Table{Cols: []Col{{"NAME", "name"}}, Rows: rows}, nil
	})
	reg("logging metrics create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.Arg(0, "METRIC_NAME")
		if err != nil {
			return nil, err
		}
		if p.LogMetrics[n] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: metric %s already exists", n)
		}
		if err := c.NeedProject("logging.logMetrics.create"); err != nil {
			return nil, err
		}
		f := c.Str("log-filter", "")
		if f == "" {
			if cfg := c.Str("config-from-file", ""); cfg != "" {
				var m map[string]any
				_ = yaml.Unmarshal([]byte(c.S.Files[c.S.path(cfg)]), &m)
				f, _ = m["filter"].(string)
			}
		}
		if f == "" {
			return nil, fmt.Errorf("argument --log-filter: Must be specified.")
		}
		p.LogMetrics[n] = &sim.LogMetric{Name: n, Filter: f, Description: c.Str("description", "")}
		c.Audit("logging.googleapis.com", "google.logging.v2.MetricsServiceV2.CreateLogMetric", "projects/"+p.ID+"/metrics/"+n)
		return "Created [" + n + "].\n", nil
	})
	reg("logging metrics update", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "METRIC_NAME")
		m := p.LogMetrics[n]
		if m == nil {
			return nil, fmt.Errorf("NOT_FOUND: metric %s", n)
		}
		if v := c.Str("log-filter", ""); v != "" {
			m.Filter = v
		}
		if v := c.Str("description", ""); v != "" {
			m.Description = v
		}
		return "Updated [" + n + "].\n", nil
	})
	reg("logging metrics list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.LogMetrics) {
			rows = append(rows, p.LogMetrics[k])
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"DESCRIPTION", "description"}, {"FILTER", "filter"}}, Rows: rows}, nil
	})
	reg("logging metrics describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "METRIC_NAME")
		if m := p.LogMetrics[n]; m != nil {
			return Obj{V: m}, nil
		}
		return nil, fmt.Errorf("NOT_FOUND: metric %s", n)
	})
	reg("logging metrics delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "METRIC_NAME")
		delete(p.LogMetrics, n)
		return "Deleted [" + n + "].\n", nil
	})
	reg("logging sinks create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		if len(c.Args) < 2 {
			return nil, fmt.Errorf("argument SINK_NAME DESTINATION: Must be specified.")
		}
		n, dest := c.Args[0], c.Args[1]
		if err := c.NeedProject("logging.sinks.create"); err != nil {
			return nil, err
		}
		writer := "serviceAccount:service-" + p.Number + "@gcp-sa-logging.iam.gserviceaccount.com"
		p.LogSinks[n] = &sim.LogSink{Name: n, Destination: dest, Filter: c.Str("log-filter", ""), Writer: writer}
		return fmt.Sprintf("Created [https://logging.googleapis.com/v2/projects/%s/sinks/%s].\nPlease remember to grant `%s` the appropriate role on the destination.\n", p.ID, n, writer), nil
	})
	reg("logging sinks list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.LogSinks) {
			rows = append(rows, p.LogSinks[k])
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"DESTINATION", "destination"}, {"FILTER", "filter"}}, Rows: rows}, nil
	})

	// ---- Cloud Monitoring -----------------------------------------------------
	reg("monitoring channels create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		id := "projects/" + p.ID + "/notificationChannels/" + fmt.Sprint(1000+len(p.Channels))
		p.Channels[id] = &sim.NotificationChannel{Name: id, DisplayName: c.Str("display-name", ""), Type: c.Str("type", "email"), Labels: c.KV("channel-labels")}
		c.Audit("monitoring.googleapis.com", "google.monitoring.v3.NotificationChannelService.CreateNotificationChannel", id)
		return "Created notification channel [" + id + "].\n", nil
	})
	reg("monitoring channels list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Channels) {
			rows = append(rows, p.Channels[k])
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"DISPLAY_NAME", "displayName"}, {"TYPE", "type"}}, Rows: rows}, nil
	})
	reg("monitoring policies create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("monitoring.alertPolicies.create"); err != nil {
			return nil, err
		}
		ap := &sim.AlertPolicy{Enabled: true}
		if f := c.Str("policy-from-file", ""); f != "" {
			content, ok := c.S.Files[c.S.path(f)]
			if !ok {
				return nil, fmt.Errorf("Unable to read file [%s]", f)
			}
			var doc struct {
				DisplayName   string   `json:"displayName" yaml:"displayName"`
				Channels      []string `json:"notificationChannels" yaml:"notificationChannels"`
				Documentation struct {
					Content string `json:"content" yaml:"content"`
				} `json:"documentation" yaml:"documentation"`
				Conditions []struct {
					DisplayName string `json:"displayName" yaml:"displayName"`
					Threshold   struct {
						Filter     string  `json:"filter" yaml:"filter"`
						Comparison string  `json:"comparison" yaml:"comparison"`
						Value      float64 `json:"thresholdValue" yaml:"thresholdValue"`
						Duration   string  `json:"duration" yaml:"duration"`
					} `json:"conditionThreshold" yaml:"conditionThreshold"`
				} `json:"conditions" yaml:"conditions"`
			}
			if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
				return nil, fmt.Errorf("Invalid policy file: %v", err)
			}
			ap.DisplayName, ap.Channels, ap.Documentation = doc.DisplayName, doc.Channels, doc.Documentation.Content
			for _, cd := range doc.Conditions {
				ap.Conditions = append(ap.Conditions, sim.AlertCondition{DisplayName: cd.DisplayName, Filter: cd.Threshold.Filter, Comparison: cd.Threshold.Comparison, Threshold: cd.Threshold.Value, Duration: cd.Threshold.Duration})
			}
		} else {
			ap.DisplayName = c.Str("display-name", "")
			ap.Channels = c.List("notification-channels")
			ap.Documentation = c.Str("documentation", "")
			cond := sim.AlertCondition{DisplayName: c.Str("condition-display-name", ap.DisplayName), Filter: c.Str("condition-filter", ""), Duration: c.Str("duration", "60s"), Comparison: "COMPARISON_GT"}
			ifv := strings.TrimSpace(c.Str("if", ""))
			if strings.HasPrefix(ifv, "<") {
				cond.Comparison = "COMPARISON_LT"
			}
			cond.Threshold, _ = strconv.ParseFloat(strings.TrimSpace(strings.TrimLeft(ifv, "<>= ")), 64)
			ap.Conditions = []sim.AlertCondition{cond}
		}
		if ap.DisplayName == "" || len(ap.Conditions) == 0 || ap.Conditions[0].Filter == "" {
			return nil, fmt.Errorf("INVALID_ARGUMENT: alert policy requires displayName and at least one condition with a filter")
		}
		for _, cd := range ap.Conditions {
			if m := regexp.MustCompile(`logging\.googleapis\.com/user/([A-Za-z0-9_-]+)`).FindStringSubmatch(cd.Filter); m != nil && p.LogMetrics[m[1]] == nil {
				return nil, fmt.Errorf("INVALID_ARGUMENT: Field alert_policy.conditions[0].condition_threshold.filter had an invalid value: metric logging.googleapis.com/user/%s does not exist", m[1])
			}
		}
		for _, ch := range ap.Channels {
			if p.Channels[ch] == nil {
				return nil, fmt.Errorf("NOT_FOUND: notification channel %s not found", ch)
			}
		}
		ap.Name = "projects/" + p.ID + "/alertPolicies/" + fmt.Sprint(5000+len(p.AlertPolicies))
		p.AlertPolicies[ap.Name] = ap
		c.Audit("monitoring.googleapis.com", "google.monitoring.v3.AlertPolicyService.CreateAlertPolicy", ap.Name)
		return "Created alert policy [" + ap.Name + "].\n", nil
	})
	reg("monitoring policies list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.AlertPolicies) {
			rows = append(rows, p.AlertPolicies[k])
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"DISPLAY_NAME", "displayName"}, {"ENABLED", "enabled"}, {"FIRING", "firing"}}, Rows: rows}, nil
	})
	reg("monitoring policies describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "POLICY")
		if ap := p.AlertPolicies[n]; ap != nil {
			return Obj{V: ap}, nil
		}
		return nil, fmt.Errorf("NOT_FOUND: policy %s", n)
	})
	reg("monitoring policies update", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "POLICY")
		ap := p.AlertPolicies[n]
		if ap == nil {
			return nil, fmt.Errorf("NOT_FOUND: policy %s", n)
		}
		if c.Bool("enabled") {
			ap.Enabled = true
		}
		if c.Bool("no-enabled") {
			ap.Enabled = false
		}
		if c.Has("add-notification-channels") {
			ap.Channels = append(ap.Channels, c.List("add-notification-channels")...)
		}
		return "Updated alert policy [" + n + "].\n", nil
	})
	reg("monitoring policies delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "POLICY")
		delete(p.AlertPolicies, n)
		return "Deleted alert policy [" + n + "].\n", nil
	})
	reg("monitoring uptime create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.Arg(0, "DISPLAY_NAME")
		if err != nil {
			return nil, err
		}
		labels := c.KV("resource-labels")
		p.UptimeChecks[n] = &sim.UptimeCheck{Name: n, Host: labels["host"], Path: c.Str("path", "/"), Port: c.Int("port", 80), Period: c.Str("period", "1")}
		return "New uptime check [" + n + "] created.\n", nil
	})
	reg("monitoring uptime list-configs", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.UptimeChecks) {
			rows = append(rows, p.UptimeChecks[k])
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"HOST", "host"}, {"PATH", "path"}, {"PASSING", "passing"}}, Rows: rows}, nil
	})
	reg("monitoring dashboards create", func(c *Cmd) (any, error) {
		f := c.Str("config-from-file", "")
		if _, ok := c.S.Files[c.S.path(f)]; !ok {
			return nil, fmt.Errorf("Unable to read file [%s]", f)
		}
		c.S.State.Extra["dashboard:"+c.ProjectID()+"/"+f] = c.S.Files[c.S.path(f)]
		return "Created [projects/" + c.ProjectID() + "/dashboards/" + c.S.State.ID(8) + "].\n", nil
	})

	// ---- GKE -------------------------------------------------------------------
	reg("container clusters create", func(c *Cmd) (any, error) { return createCluster(c, false) })
	reg("container clusters create-auto", func(c *Cmd) (any, error) { return createCluster(c, true) })
	reg("container clusters list", func(c *Cmd) (any, error) {
		if err := c.API("container.googleapis.com"); err != nil {
			return nil, err
		}
		p, _ := c.P()
		var rows []any
		for _, k := range sim.SortedKeys(p.Clusters) {
			cl := p.Clusters[k]
			nodes := 0
			for _, np := range cl.NodePools {
				nodes += np.Count
			}
			mt := ""
			if len(cl.NodePools) > 0 {
				mt = cl.NodePools[0].MachineType
			}
			rows = append(rows, map[string]any{"name": cl.Name, "location": cl.Location, "masterVersion": "1.31.1-gke.1678000", "endpoint": cl.Endpoint, "machineType": mt, "nodeCount": nodes, "status": cl.Status, "autopilot": cl.Autopilot})
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"LOCATION", "location"}, {"MASTER_VERSION", "masterVersion"}, {"MASTER_IP", "endpoint"}, {"MACHINE_TYPE", "machineType"}, {"NUM_NODES", "nodeCount"}, {"STATUS", "status"}}, Rows: rows}, nil
	})
	reg("container clusters describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		cl := p.Clusters[n]
		if cl == nil {
			return nil, fmt.Errorf("ResponseError: code=404, message=Not found: projects/%s/locations/%s/clusters/%s.", p.ID, c.Str("location", c.Str("zone", c.Str("region", ""))), n)
		}
		v := map[string]any{"name": cl.Name, "location": cl.Location, "autopilot": map[string]any{"enabled": cl.Autopilot}, "nodePools": cl.NodePools,
			"workloadIdentityConfig": map[string]any{"workloadPool": cl.WorkloadPool}, "network": cl.Network, "subnetwork": cl.Subnet,
			"privateClusterConfig": map[string]any{"enablePrivateNodes": cl.PrivateNodes}, "masterAuthorizedNetworksConfig": map[string]any{"cidrBlocks": cl.MasterAuthNets},
			"releaseChannel": map[string]any{"channel": cl.ReleaseChannel}, "status": cl.Status, "endpoint": cl.Endpoint, "shieldedNodes": map[string]any{"enabled": cl.ShieldedNodes},
			"binaryAuthorization": map[string]any{"enabled": cl.BinaryAuthz}}
		return Obj{V: v}, nil
	})
	reg("container clusters get-credentials", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.Arg(0, "NAME")
		if err != nil {
			return nil, err
		}
		cl := p.Clusters[n]
		if cl == nil {
			return nil, fmt.Errorf("ResponseError: code=404, message=Not found: projects/%s/locations/%s/clusters/%s.", p.ID, c.Str("zone", c.Str("region", c.Str("location", ""))), n)
		}
		if err := c.Need("container.clusters.get", sim.ProjectResource(p.ID)); err != nil {
			return nil, err
		}
		c.S.Kube = KubeContext{Project: p.ID, Cluster: n, Namespace: "default"}
		return "Fetching cluster endpoint and auth data.\nkubeconfig entry generated for " + n + ".\n", nil
	})
	reg("container clusters delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		if p.Clusters[n] == nil {
			return nil, fmt.Errorf("cluster %s not found", n)
		}
		if err := c.NeedProject("container.clusters.delete"); err != nil {
			return nil, err
		}
		delete(p.Clusters, n)
		c.Audit("container.googleapis.com", "google.container.v1.ClusterManager.DeleteCluster", "projects/"+p.ID+"/clusters/"+n)
		return "Deleted [" + n + "].\n", nil
	})
	reg("container clusters update", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		cl := p.Clusters[n]
		if cl == nil {
			return nil, fmt.Errorf("cluster %s not found", n)
		}
		if err := c.NeedProject("container.clusters.update"); err != nil {
			return nil, err
		}
		if v := c.Str("workload-pool", ""); v != "" {
			if v != p.ID+".svc.id.goog" {
				return nil, fmt.Errorf("INVALID_ARGUMENT: workload pool must be %s.svc.id.goog", p.ID)
			}
			if cl.WorkloadPool == "" {
				// existing node pools keep node metadata until they are updated
				for i := range cl.NodePools {
					cl.NodePools[i].WorkloadMetadata = "GCE_METADATA"
				}
			}
			cl.WorkloadPool = v
		}
		if c.Bool("enable-master-authorized-networks") || c.Has("master-authorized-networks") {
			cl.MasterAuthNets = c.List("master-authorized-networks")
		}
		if c.Bool("enable-shielded-nodes") {
			cl.ShieldedNodes = true
		}
		if v := c.Str("binauthz-evaluation-mode", ""); v != "" {
			cl.BinaryAuthz = v != "DISABLED"
		}
		if c.Bool("enable-autoscaling") {
			for i := range cl.NodePools {
				if np := c.Str("node-pool", cl.NodePools[i].Name); np == cl.NodePools[i].Name {
					cl.NodePools[i].Autoscaling = true
					cl.NodePools[i].Min = c.Int("min-nodes", 1)
					cl.NodePools[i].Max = c.Int("max-nodes", 3)
				}
			}
		}
		c.Audit("container.googleapis.com", "google.container.v1.ClusterManager.UpdateCluster", "projects/"+p.ID+"/clusters/"+n)
		return "Updating " + n + "...done.\n", nil
	})
	reg("container clusters resize", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		cl := p.Clusters[n]
		if cl == nil {
			return nil, fmt.Errorf("cluster %s not found", n)
		}
		pool := c.Str("node-pool", "default-pool")
		for i := range cl.NodePools {
			if cl.NodePools[i].Name == pool {
				cl.NodePools[i].Count = c.Int("num-nodes", cl.NodePools[i].Count)
				return "Resizing " + n + "...done.\n", nil
			}
		}
		return nil, fmt.Errorf("node pool %s not found", pool)
	})
	reg("container node-pools create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		cl := p.Clusters[c.Str("cluster", "")]
		if cl == nil {
			return nil, fmt.Errorf("cluster %s not found", c.Str("cluster", ""))
		}
		if cl.Autopilot {
			return nil, fmt.Errorf("FAILED_PRECONDITION: Autopilot clusters manage node pools automatically.")
		}
		mt := c.Str("machine-type", "e2-medium")
		if !reMachine.MatchString(mt) {
			return nil, fmt.Errorf("invalid machine type %s", mt)
		}
		if strings.HasPrefix(mt, "a2-") && !c.S.Policy.AllowGPUs {
			return nil, fmt.Errorf("Quota 'NVIDIA_A100_GPUS' exceeded (GPUs are disabled in this lab).")
		}
		sa := c.Str("service-account", "")
		if sa != "" {
			if _, err := c.resolveSA(p, sa, false); err != nil {
				return nil, err
			}
		}
		wm := c.Str("workload-metadata", "")
		if wm == "" && cl.WorkloadPool == "" {
			wm = "GCE_METADATA"
		}
		cl.NodePools = append(cl.NodePools, sim.NodePool{Name: n, MachineType: mt, Count: c.Int("num-nodes", 3), Autoscaling: c.Bool("enable-autoscaling"), Min: c.Int("min-nodes", 0), Max: c.Int("max-nodes", 0), SA: sa, Spot: c.Bool("spot"), WorkloadMetadata: wm})
		return "Creating node pool " + n + "...done.\n", nil
	})
	reg("container node-pools list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		cl := p.Clusters[c.Str("cluster", "")]
		if cl == nil {
			return nil, fmt.Errorf("cluster not found")
		}
		var rows []any
		for _, np := range cl.NodePools {
			rows = append(rows, np)
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"MACHINE_TYPE", "machineType"}, {"NODE_COUNT", "nodeCount"}, {"AUTOSCALING", "autoscaling"}}, Rows: rows}, nil
	})
	reg("container node-pools update", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		cl := p.Clusters[c.Str("cluster", "")]
		if cl == nil {
			return nil, fmt.Errorf("cluster not found")
		}
		for i := range cl.NodePools {
			if cl.NodePools[i].Name == n {
				if wm := c.Str("workload-metadata", ""); wm != "" {
					if wm != "GKE_METADATA" && wm != "GCE_METADATA" {
						return nil, fmt.Errorf("argument --workload-metadata: Invalid choice: '%s' (GKE_METADATA, GCE_METADATA)", wm)
					}
					if wm == "GKE_METADATA" && cl.WorkloadPool == "" {
						return nil, fmt.Errorf("FAILED_PRECONDITION: Workload Identity must be enabled on the cluster (--workload-pool) before node pools can use GKE_METADATA")
					}
					cl.NodePools[i].WorkloadMetadata = wm
				}
				if c.Bool("enable-autoscaling") {
					cl.NodePools[i].Autoscaling = true
					cl.NodePools[i].Min, cl.NodePools[i].Max = c.Int("min-nodes", 1), c.Int("max-nodes", 3)
				}
				return "Updating node pool " + n + "...done.\n", nil
			}
		}
		return nil, fmt.Errorf("node pool %s not found", n)
	})
	reg("container node-pools delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "NAME")
		cl := p.Clusters[c.Str("cluster", "")]
		if cl == nil {
			return nil, fmt.Errorf("cluster not found")
		}
		var keep []sim.NodePool
		for _, np := range cl.NodePools {
			if np.Name != n {
				keep = append(keep, np)
			}
		}
		cl.NodePools = keep
		return "Deleted node pool " + n + ".\n", nil
	})

	// ---- Artifact Registry ------------------------------------------------------
	reg("artifacts repositories create", func(c *Cmd) (any, error) {
		if err := c.API("artifactregistry.googleapis.com"); err != nil {
			return nil, err
		}
		p, _ := c.P()
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		if p.ArtifactRepos[n] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: the repository already exists")
		}
		if err := c.NeedProject("artifactregistry.repositories.create"); err != nil {
			return nil, err
		}
		loc := c.Str("location", "")
		if loc == "" {
			return nil, fmt.Errorf("argument --location: Must be specified.")
		}
		p.ArtifactRepos[n] = &sim.ArtifactRepo{Name: n, Location: loc, Format: strings.ToUpper(c.Str("repository-format", "docker")), Images: map[string][]string{}, Scanning: true}
		c.Audit("artifactregistry.googleapis.com", "google.devtools.artifactregistry.v1.ArtifactRegistry.CreateRepository", "projects/"+p.ID+"/locations/"+loc+"/repositories/"+n)
		return "Created repository [" + n + "].\n", nil
	})
	reg("artifacts repositories delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.name0()
		if err != nil {
			return nil, err
		}
		r := p.ArtifactRepos[n]
		loc := c.Str("location", "")
		if r == nil || (loc != "" && loc != r.Location) {
			return nil, fmt.Errorf("NOT_FOUND: Requested entity was not found.")
		}
		if err := c.NeedProject("artifactregistry.repositories.delete"); err != nil {
			return nil, err
		}
		delete(p.ArtifactRepos, n)
		c.Audit("artifactregistry.googleapis.com", "google.devtools.artifactregistry.v1.ArtifactRegistry.DeleteRepository", "projects/"+p.ID+"/locations/"+r.Location+"/repositories/"+n)
		return "Deleted repository [" + n + "].\n", nil
	})
	reg("artifacts repositories list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.ArtifactRepos) {
			r := p.ArtifactRepos[k]
			rows = append(rows, map[string]any{"name": r.Name, "format": r.Format, "location": r.Location, "mode": "STANDARD_REPOSITORY"})
		}
		return Table{Cols: []Col{{"REPOSITORY", "name"}, {"FORMAT", "format"}, {"MODE", "mode"}, {"LOCATION", "location"}}, Rows: rows}, nil
	})
	reg("artifacts repositories describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "REPOSITORY")
		if r := p.ArtifactRepos[n]; r != nil {
			return Obj{V: r}, nil
		}
		return nil, fmt.Errorf("NOT_FOUND: repository %s", n)
	})
	reg("artifacts repositories add-iam-policy-binding", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "REPOSITORY")
		r := p.ArtifactRepos[n]
		if r == nil {
			return nil, fmt.Errorf("NOT_FOUND: repository %s", n)
		}
		m, role, _, err := c.bindingArgs()
		if err != nil {
			return nil, err
		}
		p.IAM.AddBinding(role, m, &sim.Condition{Title: "repo-" + n, Expression: `resource.name.startsWith("projects/` + p.ID + `/locations/` + r.Location + `/repositories/` + n + `")`})
		c.Audit("artifactregistry.googleapis.com", "SetIamPolicy", "projects/"+p.ID+"/locations/"+r.Location+"/repositories/"+n)
		return "Updated IAM policy for repository [" + n + "].\n", nil
	})
	reg("artifacts docker images list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		prefix, _ := c.Arg(0, "IMAGE_PATH")
		var rows []any
		for _, rn := range sim.SortedKeys(p.ArtifactRepos) {
			r := p.ArtifactRepos[rn]
			for _, img := range sim.SortedKeys(r.Images) {
				full := fmt.Sprintf("%s-docker.pkg.dev/%s/%s/%s", r.Location, p.ID, rn, img)
				if prefix != "" && !strings.HasPrefix(full, prefix) {
					continue
				}
				rows = append(rows, map[string]any{"package": full, "tags": strings.Join(r.Images[img], ","), "createTime": c.S.State.Now()})
			}
		}
		return Table{Cols: []Col{{"IMAGE", "package"}, {"TAGS", "tags"}, {"CREATE_TIME", "createTime"}}, Rows: rows}, nil
	})
	reg("artifacts docker tags list", registry["artifacts docker images list"])

	// ---- Source repos / Cloud Build ---------------------------------------------
	reg("source repos create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "REPOSITORY")
		if p.SourceRepos[n] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: repo %s", n)
		}
		p.SourceRepos[n] = &sim.SourceRepo{Name: n}
		return "Created [" + n + "].\n", nil
	})
	reg("source repos list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.SourceRepos) {
			rows = append(rows, map[string]any{"name": k, "url": "https://source.developers.google.com/p/" + p.ID + "/r/" + k})
		}
		return Table{Cols: []Col{{"REPO_NAME", "name"}, {"URL", "url"}}, Rows: rows}, nil
	})
	reg("source repos clone", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "REPOSITORY")
		r := p.SourceRepos[n]
		if r == nil {
			return nil, fmt.Errorf("NOT_FOUND: repo %s", n)
		}
		c.S.Git = GitState{Initialized: true, Branch: "main", Remote: n, Staged: map[string]string{}}
		if len(r.Commits) > 0 {
			for k, v := range r.Commits[len(r.Commits)-1].Files {
				c.S.Files[k] = v
			}
			c.S.Git.Commits = append([]sim.GitCommit{}, r.Commits...)
		}
		return "Cloning into '" + n + "'...\nProject [" + p.ID + "] repository [" + n + "] was cloned to [.].\n", nil
	})
	reg("builds submit", func(c *Cmd) (any, error) {
		if err := c.API("cloudbuild.googleapis.com"); err != nil {
			return nil, err
		}
		p, _ := c.P()
		if err := c.NeedProject("cloudbuild.builds.create"); err != nil {
			return nil, err
		}
		b, err := c.S.runBuild(p, c.S.Files, c.Str("tag", ""), c.Str("config", ""), "", "")
		if err != nil {
			return nil, err
		}
		out := strings.Join(b.Log, "\n") + "\n"
		if b.Status != "SUCCESS" {
			return nil, fmt.Errorf("%sBUILD FAILURE: Build step failure: build step exited with non-zero status (build %s)", out, b.ID)
		}
		return out + fmt.Sprintf("ID  CREATE_TIME  DURATION  SOURCE  IMAGES  STATUS\n%s  %s  32S  gs://%s_cloudbuild/source  %s  SUCCESS\n", b.ID, b.Created, p.ID, strings.Join(b.Images, ",")), nil
	})
	reg("builds list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for i := len(p.Builds) - 1; i >= 0; i-- {
			rows = append(rows, p.Builds[i])
		}
		return Table{Cols: []Col{{"ID", "id"}, {"CREATE_TIME", "createTime"}, {"SOURCE", "source"}, {"IMAGES", "images"}, {"STATUS", "status"}, {"TRIGGER", "buildTriggerId"}}, Rows: rows}, nil
	})
	reg("builds describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		id, _ := c.Arg(0, "BUILD")
		for _, b := range p.Builds {
			if b.ID == id {
				return Obj{V: b}, nil
			}
		}
		return nil, fmt.Errorf("NOT_FOUND: build %s", id)
	})
	reg("builds log", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		id, _ := c.Arg(0, "BUILD")
		for _, b := range p.Builds {
			if b.ID == id {
				return strings.Join(b.Log, "\n") + "\n", nil
			}
		}
		return nil, fmt.Errorf("NOT_FOUND: build %s", id)
	})
	trigCreate := func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n := c.Str("name", "")
		if n == "" {
			return nil, fmt.Errorf("argument --name: Must be specified.")
		}
		repo := c.Str("repo", c.Str("repo-name", ""))
		if repo == "" {
			return nil, fmt.Errorf("argument --repo: Must be specified.")
		}
		if strings.Contains(c.Path, "cloud-source-repositories") && p.SourceRepos[repo] == nil {
			return nil, fmt.Errorf("NOT_FOUND: repository %s", repo)
		}
		if err := c.NeedProject("cloudbuild.builds.create"); err != nil {
			return nil, err
		}
		sa := c.Str("service-account", "")
		if sa != "" {
			sa = sa[strings.LastIndex(sa, "/")+1:]
			if _, err := c.resolveSA(p, sa, false); err != nil {
				return nil, err
			}
		}
		p.Triggers[n] = &sim.BuildTrigger{Name: n, Repo: repo, Branch: c.Str("branch-pattern", "^main$"), BuildConfig: c.Str("build-config", "cloudbuild.yaml"), SA: sa, Region: c.Str("region", "global")}
		c.Audit("cloudbuild.googleapis.com", "google.devtools.cloudbuild.v1.CloudBuild.CreateBuildTrigger", "projects/"+p.ID+"/triggers/"+n)
		return "Created [https://cloudbuild.googleapis.com/v1/projects/" + p.ID + "/triggers/" + n + "].\n", nil
	}
	reg("builds triggers create cloud-source-repositories", trigCreate)
	reg("builds triggers create github", trigCreate)
	reg("builds triggers create manual", trigCreate)
	reg("builds triggers list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Triggers) {
			rows = append(rows, p.Triggers[k])
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"REPO", "repo"}, {"BRANCH", "branchPattern"}, {"CONFIG", "filename"}}, Rows: rows}, nil
	})
	reg("builds triggers run", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "TRIGGER")
		t := p.Triggers[n]
		if t == nil {
			return nil, fmt.Errorf("NOT_FOUND: trigger %s", n)
		}
		files := c.S.Files
		sha := ""
		if r := p.SourceRepos[t.Repo]; r != nil && len(r.Commits) > 0 {
			files = r.Commits[len(r.Commits)-1].Files
			sha = r.Commits[len(r.Commits)-1].SHA
		}
		b, err := c.S.runBuild(p, files, "", t.BuildConfig, n, sha)
		if err != nil {
			return nil, err
		}
		return "Build " + b.ID + " finished with status " + b.Status + "\n", nil
	})
	reg("builds triggers delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "TRIGGER")
		delete(p.Triggers, n)
		return "Deleted.\n", nil
	})

	// ---- Cloud Deploy --------------------------------------------------------------
	reg("deploy apply", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		f := c.Str("file", "")
		content, ok := c.S.Files[c.S.path(f)]
		if !ok {
			return nil, fmt.Errorf("Unable to read file [%s]", f)
		}
		region, err := c.RegionFlag()
		if err != nil {
			return nil, err
		}
		dec := yaml.NewDecoder(strings.NewReader(content))
		out := ""
		for {
			var m map[string]any
			if err := dec.Decode(&m); err != nil {
				break
			}
			kind := fmt.Sprint(m["kind"])
			meta, _ := m["metadata"].(map[string]any)
			name := fmt.Sprint(meta["name"])
			switch kind {
			case "DeliveryPipeline":
				dp := &sim.DeliveryPipeline{Name: name, Region: region}
				sp, _ := m["serialPipeline"].(map[string]any)
				stages, _ := sp["stages"].([]any)
				for _, st := range stages {
					sm, _ := st.(map[string]any)
					stage := sim.DeployStage{Target: fmt.Sprint(sm["targetId"])}
					if strat, ok := sm["strategy"].(map[string]any); ok {
						if std, ok := strat["standard"].(map[string]any); ok {
							stage.Verify = std["verify"] == true
						}
						if can, ok := strat["canary"].(map[string]any); ok {
							if cd, ok := can["canaryDeployment"].(map[string]any); ok {
								stage.Verify = cd["verify"] == true
								if pcts, ok := cd["percentages"].([]any); ok {
									for _, x := range pcts {
										n, _ := strconv.Atoi(fmt.Sprint(x))
										stage.Canary = append(stage.Canary, n)
									}
								}
							}
						}
					}
					dp.Stages = append(dp.Stages, stage)
				}
				if p.Pipelines[name] != nil {
					dp.RollbackOnFailure = p.Pipelines[name].RollbackOnFailure
				}
				p.Pipelines[name] = dp
				out += "Created Cloud Deploy resource: projects/" + p.ID + "/locations/" + region + "/deliveryPipelines/" + name + ".\n"
			case "Target":
				t := &sim.DeployTarget{Name: name, Region: region, RequireApproval: m["requireApproval"] == true}
				if run, ok := m["run"].(map[string]any); ok {
					t.RunService = fmt.Sprint(run["location"])
				}
				if gke, ok := m["gke"].(map[string]any); ok {
					t.Cluster = fmt.Sprint(gke["cluster"])
				}
				if svc, ok := meta["annotations"].(map[string]any); ok {
					if s, ok := svc["sim/run-service"]; ok {
						t.RunService = fmt.Sprint(s)
					}
				}
				if t.RunService == "" || strings.HasPrefix(t.RunService, "projects/") {
					t.RunService = name
				}
				p.DeployTargets[name] = t
				out += "Created Cloud Deploy resource: projects/" + p.ID + "/locations/" + region + "/targets/" + name + ".\n"
			case "Automation":
				if sel, ok := m["selector"].(map[string]any); ok {
					_ = sel
				}
				for _, dp := range p.Pipelines {
					dp.RollbackOnFailure = true
				}
				out += "Created Cloud Deploy automation " + name + " (repairRollout: rollback).\n"
			default:
				return nil, fmt.Errorf("unsupported Cloud Deploy kind %q", kind)
			}
		}
		c.Audit("clouddeploy.googleapis.com", "google.cloud.deploy.v1.CloudDeploy.CreateDeliveryPipeline", "projects/"+p.ID)
		return out, nil
	})
	reg("deploy delivery-pipelines list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Pipelines) {
			rows = append(rows, p.Pipelines[k])
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"STAGES", "stages"}}, Rows: rows}, nil
	})
	reg("deploy delivery-pipelines describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "PIPELINE")
		if dp := p.Pipelines[n]; dp != nil {
			return Obj{V: dp}, nil
		}
		return nil, fmt.Errorf("NOT_FOUND: pipeline %s", n)
	})
	reg("deploy releases create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.Arg(0, "RELEASE")
		if err != nil {
			return nil, err
		}
		dp := p.Pipelines[c.Str("delivery-pipeline", "")]
		if dp == nil {
			return nil, fmt.Errorf("NOT_FOUND: delivery pipeline %s", c.Str("delivery-pipeline", ""))
		}
		if err := c.NeedProject("clouddeploy.releases.create"); err != nil {
			return nil, err
		}
		img := ""
		for _, v := range c.KV("images") {
			img = v
		}
		if img == "" {
			return nil, fmt.Errorf("argument --images: Must be specified.")
		}
		rel := &sim.Release{Name: n, Pipeline: dp.Name, Image: img}
		out := fmt.Sprintf("Creating Cloud Deploy release %s.\nCreated Cloud Deploy release %s.\n", n, n)
		if len(dp.Stages) > 0 {
			o, _ := c.S.rollout(c, p, dp, rel, 0)
			out += o
		}
		p.Releases = append(p.Releases, rel)
		c.Audit("clouddeploy.googleapis.com", "google.cloud.deploy.v1.CloudDeploy.CreateRelease", "projects/"+p.ID+"/deliveryPipelines/"+dp.Name+"/releases/"+n)
		return out, nil
	})
	reg("deploy releases promote", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n := c.Str("release", "")
		var rel *sim.Release
		for _, r := range p.Releases {
			if r.Name == n {
				rel = r
			}
		}
		if rel == nil {
			return nil, fmt.Errorf("NOT_FOUND: release %s", n)
		}
		dp := p.Pipelines[rel.Pipeline]
		next := len(rel.Rollouts)
		if next >= len(dp.Stages) {
			return nil, fmt.Errorf("Release %s is already deployed to the last target.", n)
		}
		out, _ := c.S.rollout(c, p, dp, rel, next)
		return out, nil
	})
	reg("deploy releases list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, r := range p.Releases {
			rows = append(rows, r)
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"PIPELINE", "deliveryPipeline"}, {"IMAGE", "image"}}, Rows: rows}, nil
	})
	reg("deploy rollouts list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, r := range p.Releases {
			if rf := c.Str("release", ""); rf != "" && rf != r.Name {
				continue
			}
			for _, ro := range r.Rollouts {
				rows = append(rows, map[string]any{"release": r.Name, "target": ro.Target, "state": ro.State, "detail": ro.Detail})
			}
		}
		return Table{Cols: []Col{{"RELEASE", "release"}, {"TARGET", "target"}, {"STATE", "state"}, {"DETAIL", "detail"}}, Rows: rows}, nil
	})
	reg("deploy targets rollback", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		tn, _ := c.Arg(0, "TARGET")
		t := p.DeployTargets[tn]
		if t == nil {
			return nil, fmt.Errorf("NOT_FOUND: target %s", tn)
		}
		var prev *sim.Release
		count := 0
		for i := len(p.Releases) - 1; i >= 0; i-- {
			for _, ro := range p.Releases[i].Rollouts {
				if ro.Target == tn && ro.State == "SUCCEEDED" {
					count++
					if count == 2 || (count == 1 && p.Releases[i].Name != lastRelease(p, tn)) {
						prev = p.Releases[i]
					}
				}
			}
			if prev != nil {
				break
			}
		}
		if prev == nil {
			return nil, fmt.Errorf("FAILED_PRECONDITION: no previous successful release to roll back to")
		}
		if svc := p.RunServices[t.RunService]; svc != nil {
			svc.Image = prev.Image
		}
		return "Rolling back target " + tn + " to release " + prev.Name + "...done.\n", nil
	})

	// ---- Vertex AI ------------------------------------------------------------------
	reg("ai models upload", func(c *Cmd) (any, error) {
		if err := c.API("aiplatform.googleapis.com"); err != nil {
			return nil, err
		}
		p, _ := c.P()
		if err := c.NeedProject("aiplatform.models.upload"); err != nil {
			return nil, err
		}
		region, err := c.RegionFlag()
		if err != nil {
			return nil, err
		}
		dn := c.Str("display-name", "")
		img := c.Str("container-image-uri", "")
		if dn == "" || img == "" {
			return nil, fmt.Errorf("--display-name and --container-image-uri are required")
		}
		if art := c.Str("artifact-uri", ""); art != "" {
			if b, _ := c.S.State.FindBucket(bucketName(art)); b == nil {
				return nil, fmt.Errorf("INVALID_ARGUMENT: artifact URI %s does not exist", art)
			}
		}
		id := fmt.Sprint(8000000000 + len(p.AIModels))
		p.AIModels[id] = &sim.AIModel{ID: id, DisplayName: dn, Region: region, Image: img, ArtifactURI: c.Str("artifact-uri", "")}
		c.Audit("aiplatform.googleapis.com", "google.cloud.aiplatform.v1.ModelService.UploadModel", "projects/"+p.Number+"/locations/"+region+"/models/"+id)
		return "Using endpoint [https://" + region + "-aiplatform.googleapis.com/]\nModel [" + id + "] uploaded.\n", nil
	})
	reg("ai models list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.AIModels) {
			rows = append(rows, p.AIModels[k])
		}
		return Table{Cols: []Col{{"MODEL_ID", "id"}, {"DISPLAY_NAME", "displayName"}}, Rows: rows}, nil
	})
	reg("ai endpoints create", func(c *Cmd) (any, error) {
		if err := c.API("aiplatform.googleapis.com"); err != nil {
			return nil, err
		}
		p, _ := c.P()
		region, err := c.RegionFlag()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("aiplatform.endpoints.create"); err != nil {
			return nil, err
		}
		id := fmt.Sprint(4000000000 + len(p.AIEndpoints))
		p.AIEndpoints[id] = &sim.AIEndpoint{ID: id, DisplayName: c.Str("display-name", ""), Region: region}
		c.Audit("aiplatform.googleapis.com", "google.cloud.aiplatform.v1.EndpointService.CreateEndpoint", "projects/"+p.Number+"/locations/"+region+"/endpoints/"+id)
		return "Using endpoint [https://" + region + "-aiplatform.googleapis.com/]\nCreated Vertex AI endpoint: projects/" + p.Number + "/locations/" + region + "/endpoints/" + id + ".\n", nil
	})
	reg("ai endpoints list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.AIEndpoints) {
			rows = append(rows, p.AIEndpoints[k])
		}
		return Table{Cols: []Col{{"ENDPOINT_ID", "id"}, {"DISPLAY_NAME", "displayName"}}, Rows: rows}, nil
	})
	reg("ai endpoints describe", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "ENDPOINT")
		if e := findEndpoint(p, n); e != nil {
			return Obj{V: e}, nil
		}
		return nil, fmt.Errorf("NOT_FOUND: endpoint %s", n)
	})
	reg("ai endpoints deploy-model", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "ENDPOINT")
		e := findEndpoint(p, n)
		if e == nil {
			return nil, fmt.Errorf("NOT_FOUND: endpoint %s", n)
		}
		mid := c.Str("model", "")
		if p.AIModels[mid] == nil {
			return nil, fmt.Errorf("NOT_FOUND: model %s", mid)
		}
		if err := c.NeedProject("aiplatform.endpoints.deploy"); err != nil {
			return nil, err
		}
		gpus := 0
		if acc := c.KV("accelerator"); acc["count"] != "" {
			gpus, _ = strconv.Atoi(acc["count"])
		}
		if gpus > 0 && !c.S.Policy.AllowGPUs {
			return nil, fmt.Errorf("Quota 'custom_model_serving_nvidia_t4_gpus' exceeded. Limit: 0 (GPUs are disabled in this lab).")
		}
		mt := c.Str("machine-type", "n1-standard-2")
		if machineCPUs(mt) > 8 && c.S.Policy.MaxMachineCPUs > 0 && machineCPUs(mt) > c.S.Policy.MaxMachineCPUs {
			return nil, fmt.Errorf("Quota 'custom_model_serving_cpus' exceeded for machine type %s (lab quota).", mt)
		}
		sa := c.Str("service-account", "")
		if sa != "" {
			if _, err := c.resolveSA(p, sa, false); err != nil {
				return nil, err
			}
		}
		dm := sim.DeployedModel{ID: fmt.Sprint(1000000000 + len(e.Deployed)*7919 + len(p.AIModels)), ModelID: mid, ModelName: p.AIModels[mid].DisplayName, MachineType: mt, MinReplicas: c.Int("min-replica-count", 1), MaxReplicas: c.Int("max-replica-count", 1), GPUs: gpus, SA: sa}
		// --traffic-split=0=10,EXISTING_ID=90: "0" is the model being deployed.
		// Without it the new model takes all traffic.
		split := map[string]int{"0": 100}
		if c.Has("traffic-split") {
			var err error
			if split, err = parseTrafficSplit(c.KV("traffic-split")); err != nil {
				return nil, err
			}
		}
		next := append(append([]sim.DeployedModel{}, e.Deployed...), dm)
		if err := applyTrafficSplit(next, split, dm.ID); err != nil {
			return nil, err
		}
		e.Deployed = next
		c.Audit("aiplatform.googleapis.com", "google.cloud.aiplatform.v1.EndpointService.DeployModel", "projects/"+p.Number+"/locations/"+e.Region+"/endpoints/"+e.ID)
		return "Deployed a model to the endpoint " + e.ID + ". Id of the deployed model: " + dm.ID + ".\n", nil
	})
	reg("ai endpoints undeploy-model", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "ENDPOINT")
		e := findEndpoint(p, n)
		if e == nil {
			return nil, fmt.Errorf("NOT_FOUND: endpoint %s", n)
		}
		id := c.Str("deployed-model-id", "")
		if id == "" {
			return nil, fmt.Errorf("argument --deployed-model-id: Must be specified.")
		}
		var keep []sim.DeployedModel
		var removed *sim.DeployedModel
		for i := range e.Deployed {
			if e.Deployed[i].ID == id {
				removed = &e.Deployed[i]
				continue
			}
			keep = append(keep, e.Deployed[i])
		}
		if removed == nil {
			return nil, fmt.Errorf("NOT_FOUND: deployed model %s not found on endpoint %s", id, e.ID)
		}
		if c.Has("traffic-split") {
			split, err := parseTrafficSplit(c.KV("traffic-split"))
			if err != nil {
				return nil, err
			}
			if err := applyTrafficSplit(keep, split, ""); err != nil {
				return nil, err
			}
		} else if removed.Traffic > 0 && len(keep) > 0 {
			if len(keep) > 1 {
				return nil, fmt.Errorf("FAILED_PRECONDITION: deployed model %s receives %d%% of traffic; pass --traffic-split for the remaining models", id, removed.Traffic)
			}
			keep[0].Traffic = 100
		}
		e.Deployed = keep
		c.Audit("aiplatform.googleapis.com", "google.cloud.aiplatform.v1.EndpointService.UndeployModel", "projects/"+p.Number+"/locations/"+e.Region+"/endpoints/"+e.ID)
		return "Undeployed model " + id + " from endpoint " + e.ID + ".\n", nil
	})
	reg("ai endpoints predict", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "ENDPOINT")
		e := findEndpoint(p, n)
		if e == nil {
			return nil, fmt.Errorf("NOT_FOUND: endpoint %s", n)
		}
		if err := c.Need("aiplatform.endpoints.predict", sim.ProjectResource(p.ID)); err != nil {
			return nil, err
		}
		if len(e.Deployed) == 0 {
			return nil, fmt.Errorf("FAILED_PRECONDITION: Endpoint has no deployed models.")
		}
		f := c.Str("json-request", "")
		content, ok := c.S.Files[c.S.path(f)]
		if !ok {
			return nil, fmt.Errorf("Unable to read file [%s]", f)
		}
		var req map[string]any
		if err := json.Unmarshal([]byte(content), &req); err != nil || req["instances"] == nil {
			return nil, fmt.Errorf("INVALID_ARGUMENT: request must be JSON with an \"instances\" array")
		}
		inst, _ := req["instances"].([]any)
		e.Predictions += len(inst)
		var preds []string
		for range inst {
			preds = append(preds, fmt.Sprintf("%.3f", 0.5+c.S.State.Rand().Float64()/2))
		}
		c.S.State.Log(p.ID, sim.LogEntry{Severity: "INFO", LogName: "aiplatform.googleapis.com%2Fprediction", Resource: sim.LogResource{Type: "aiplatform.googleapis.com/Endpoint", Labels: map[string]string{"endpoint_id": e.ID}}, Text: fmt.Sprintf("prediction request with %d instances", len(inst))})
		return "Using endpoint [https://" + e.Region + "-prediction-aiplatform.googleapis.com/]\n[" + strings.Join(preds, ", ") + "]\n", nil
	})
	reg("ai endpoints update", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, _ := c.Arg(0, "ENDPOINT")
		e := findEndpoint(p, n)
		if e == nil {
			return nil, fmt.Errorf("NOT_FOUND: endpoint %s", n)
		}
		if c.Bool("enable-monitoring") || c.Has("monitoring") {
			e.Monitoring = true
		}
		if c.Has("traffic-split") {
			split, err := parseTrafficSplit(c.KV("traffic-split"))
			if err != nil {
				return nil, err
			}
			if err := applyTrafficSplit(e.Deployed, split, ""); err != nil {
				return nil, err
			}
			c.Audit("aiplatform.googleapis.com", "google.cloud.aiplatform.v1.EndpointService.UpdateEndpoint", "projects/"+p.Number+"/locations/"+e.Region+"/endpoints/"+e.ID)
		}
		return "Updated endpoint.\n", nil
	})
	reg("ai model-monitoring-jobs create", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		e := findEndpoint(p, c.Str("endpoint", ""))
		if e == nil {
			return nil, fmt.Errorf("NOT_FOUND: endpoint %s", c.Str("endpoint", ""))
		}
		e.Monitoring = true
		return "Created model monitoring job for endpoint " + e.ID + ".\n", nil
	})
}

func lastRelease(p *sim.Project, target string) string {
	for i := len(p.Releases) - 1; i >= 0; i-- {
		for _, ro := range p.Releases[i].Rollouts {
			if ro.Target == target && ro.State == "SUCCEEDED" {
				return p.Releases[i].Name
			}
		}
	}
	return ""
}

func parseTrafficSplit(kv map[string]string) (map[string]int, error) {
	out := map[string]int{}
	for k, v := range kv {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 100 {
			return nil, fmt.Errorf("argument --traffic-split: invalid percentage %q for %s", v, k)
		}
		out[k] = n
	}
	return out, nil
}

// applyTrafficSplit sets traffic on deployed models; newID is the id the key
// "0" refers to (the model being deployed). Percentages must total 100 and
// every key must name a deployed model; unlisted models get 0.
func applyTrafficSplit(models []sim.DeployedModel, split map[string]int, newID string) error {
	sum := 0
	for k, v := range split {
		id := k
		if k == "0" && newID != "" {
			id = newID
		}
		found := false
		for i := range models {
			if models[i].ID == id {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("INVALID_ARGUMENT: traffic split references deployed model %s, which is not on the endpoint", k)
		}
		sum += v
	}
	if sum != 100 {
		return fmt.Errorf("INVALID_ARGUMENT: traffic split percentages must sum to 100 (got %d)", sum)
	}
	for i := range models {
		key := models[i].ID
		if key == newID {
			key = "0"
		}
		models[i].Traffic = split[key]
	}
	return nil
}

func findEndpoint(p *sim.Project, n string) *sim.AIEndpoint {
	n = n[strings.LastIndex(n, "/")+1:]
	if e := p.AIEndpoints[n]; e != nil {
		return e
	}
	for _, e := range p.AIEndpoints {
		if e.DisplayName == n {
			return e
		}
	}
	return nil
}

// rollout deploys a release to stage i, verifying and rolling back if configured.
func (s *Session) rollout(c *Cmd, p *sim.Project, dp *sim.DeliveryPipeline, rel *sim.Release, i int) (string, error) {
	st := dp.Stages[i]
	t := p.DeployTargets[st.Target]
	if t == nil {
		rel.Rollouts = append(rel.Rollouts, sim.Rollout{Target: st.Target, State: "FAILED", Detail: "target not found"})
		return "Rollout to " + st.Target + " FAILED: target not found\n", nil
	}
	if t.RequireApproval {
		rel.Rollouts = append(rel.Rollouts, sim.Rollout{Target: st.Target, State: "PENDING_APPROVAL"})
		return "Rollout to " + st.Target + " is PENDING_APPROVAL.\n", nil
	}
	svc := p.RunServices[t.RunService]
	if svc == nil {
		svc = &sim.RunService{Name: t.RunService, Region: t.Region, Env: map[string]string{}, Secrets: map[string]string{}, Labels: map[string]string{}, Ingress: "all", MaxInstances: 100, Concurrency: 80, Port: 8080}
		svc.URL = runURL(p, svc.Name, t.Region)
		svc.IAM.AddBinding("roles/run.invoker", "allUsers", nil)
		p.RunServices[svc.Name] = svc
	}
	prevImage := svc.Image
	svc.Image = rel.Image
	svc.Revisions = append(svc.Revisions, sim.Revision{Name: fmt.Sprintf("%s-%05d-%s", svc.Name, len(svc.Revisions)+1, s.State.ID(3)), Image: rel.Image, Env: copyStrMap(svc.Env), Traffic: 100})
	out := fmt.Sprintf("Rolling out release %s to target %s (Cloud Run service %s)...\n", rel.Name, st.Target, svc.Name)
	ok := true
	detail := ""
	if st.Verify {
		w, why := s.State.RunWorkload(p.ID, svc)
		if w == nil {
			ok, detail = false, why
		} else {
			code, msg := s.State.Call(w, "/healthz")
			if code != 200 {
				ok, detail = false, fmt.Sprintf("verify: /healthz returned %d (%s)", code, msg)
			} else {
				for _, path := range w.Behavior.Paths {
					if code, msg := s.State.Call(w, path); code >= 500 {
						ok, detail = false, fmt.Sprintf("verify: smoke test %s returned %d (%s)", path, code, msg)
						break
					}
				}
			}
		}
	}
	if !ok {
		rel.Rollouts = append(rel.Rollouts, sim.Rollout{Target: st.Target, State: "FAILED", Detail: detail})
		out += "Verification FAILED: " + detail + "\n"
		if dp.RollbackOnFailure && prevImage != "" {
			svc.Image = prevImage
			svc.Revisions = append(svc.Revisions, sim.Revision{Name: fmt.Sprintf("%s-%05d-rb", svc.Name, len(svc.Revisions)+1), Image: prevImage, Traffic: 100})
			for j := range svc.Revisions[:len(svc.Revisions)-1] {
				svc.Revisions[j].Traffic = 0
			}
			rel.Rollouts[len(rel.Rollouts)-1].Detail += " — automatically rolled back to " + prevImage
			out += "Automation repairRollout: rolled back " + st.Target + " to previous release (" + prevImage + ").\n"
			s.State.Audit(p.ID, "serviceAccount:service-"+p.Number+"@gcp-sa-clouddeploy.iam.gserviceaccount.com", "clouddeploy.googleapis.com", "RollbackTarget", "projects/"+p.ID+"/targets/"+st.Target)
		}
		return out, nil
	}
	rel.Rollouts = append(rel.Rollouts, sim.Rollout{Target: st.Target, State: "SUCCEEDED"})
	return out + "Rollout SUCCEEDED.\n", nil
}

func createCluster(c *Cmd, auto bool) (any, error) {
	if err := c.API("container.googleapis.com"); err != nil {
		return nil, err
	}
	p, _ := c.P()
	n, err := c.name0()
	if err != nil {
		return nil, err
	}
	if p.Clusters[n] != nil {
		return nil, fmt.Errorf("ResponseError: code=409, message=Already exists: projects/%s/locations/-/clusters/%s.", p.ID, n)
	}
	if err := c.NeedProject("container.clusters.create"); err != nil {
		return nil, err
	}
	loc := c.Str("zone", c.Str("region", c.Str("location", c.S.Zone)))
	if loc == "" {
		return nil, fmt.Errorf("One of [--zone, --region] must be supplied.")
	}
	if err := c.S.checkRegion(sim.RegionOf(loc), p.ID); err != nil {
		return nil, err
	}
	netName := c.Str("network", "default")
	if p.Networks[netName] == nil {
		return nil, fmt.Errorf("network %s not found", netName)
	}
	subnet := c.Str("subnetwork", "")
	if subnet == "" {
		if sn := p.SubnetForRegion(netName, sim.RegionOf(loc)); sn != nil {
			subnet = sn.Name
		} else {
			return nil, fmt.Errorf("network %s has no subnetwork in %s", netName, sim.RegionOf(loc))
		}
	}
	mt := c.Str("machine-type", "e2-medium")
	nodes := c.Int("num-nodes", 3)
	if c.S.Policy.MaxInstances > 0 && nodes > c.S.Policy.MaxInstances {
		return nil, fmt.Errorf("Quota 'INSTANCES' exceeded (lab quota %d nodes).", c.S.Policy.MaxInstances)
	}
	if strings.HasPrefix(mt, "a2-") && !c.S.Policy.AllowGPUs {
		return nil, fmt.Errorf("Quota 'NVIDIA_A100_GPUS' exceeded (GPUs are disabled in this lab).")
	}
	sa := c.Str("service-account", "")
	if sa != "" {
		if _, err := c.resolveSA(p, sa, false); err != nil {
			return nil, err
		}
	}
	cl := &sim.Cluster{Name: n, Location: loc, Autopilot: auto, Network: netName, Subnet: subnet, PrivateNodes: c.Bool("enable-private-nodes"),
		ReleaseChannel: c.Str("release-channel", "regular"), Status: "RUNNING", Endpoint: c.S.State.ExternalIP(), K8s: sim.NewK8s(), WorkloadPool: c.Str("workload-pool", ""),
		MasterAuthNets: c.List("master-authorized-networks"), ShieldedNodes: true}
	if auto {
		cl.WorkloadPool = p.ID + ".svc.id.goog"
	} else {
		cl.NodePools = []sim.NodePool{{Name: "default-pool", MachineType: mt, Count: nodes, Autoscaling: c.Bool("enable-autoscaling"), Min: c.Int("min-nodes", 1), Max: c.Int("max-nodes", nodes), SA: sa, Spot: c.Bool("spot")}}
	}
	if cl.WorkloadPool != "" && cl.WorkloadPool != p.ID+".svc.id.goog" {
		return nil, fmt.Errorf("INVALID_ARGUMENT: workload pool must be %s.svc.id.goog", p.ID)
	}
	p.Clusters[n] = cl
	c.S.Kube = KubeContext{Project: p.ID, Cluster: n, Namespace: "default"}
	c.Audit("container.googleapis.com", "google.container.v1.ClusterManager.CreateCluster", "projects/"+p.ID+"/locations/"+loc+"/clusters/"+n)
	return fmt.Sprintf("Creating cluster %s in %s... Cluster is being health-checked...done.\nCreated [https://container.googleapis.com/v1/projects/%s/zones/%s/clusters/%s].\nkubeconfig entry generated for %s.\nNAME  LOCATION  MASTER_VERSION  MASTER_IP  MACHINE_TYPE  NUM_NODES  STATUS\n%s  %s  1.31.1-gke.1678000  %s  %s  %d  RUNNING\n", n, loc, p.ID, loc, n, n, n, loc, cl.Endpoint, mt, nodes), nil
}
