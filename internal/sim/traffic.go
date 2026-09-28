package sim

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Step advances simulated time by n ticks (one minute each), running
// synthetic traffic, autoscalers, message delivery, metrics and alerting.
func (s *State) Step(n int) {
	for i := 0; i < n; i++ {
		s.tick()
	}
}

func (s *State) tick() {
	s.init()
	s.Tick++
	s.Advance(time.Minute)
	rpsBySvc := map[string]map[string]int{} // project/cluster -> svc -> rps
	runRPS := map[string]int{}
	for _, t := range s.Traffic {
		pid := t.Project
		if pid == "" {
			pid = s.defaultProject()
		}
		if strings.HasPrefix(t.Target, "k8s:") {
			parts := strings.SplitN(strings.TrimPrefix(t.Target, "k8s:"), "/", 2)
			if len(parts) == 2 {
				key := pid + "/" + parts[0]
				if rpsBySvc[key] == nil {
					rpsBySvc[key] = map[string]int{}
				}
				rpsBySvc[key][parts[1]] += t.RPS
			}
		}
		if strings.HasPrefix(t.Target, "run:") {
			runRPS[pid+"/"+strings.TrimPrefix(t.Target, "run:")] += t.RPS
		}
	}
	for _, pid := range SortedKeys(s.Projects) {
		p := s.Projects[pid]
		// Cloud Run autoscaling
		for _, sn := range SortedKeys(p.RunServices) {
			svc := p.RunServices[sn]
			conc := svc.Concurrency
			if conc <= 0 {
				conc = 80
			}
			maxI := svc.MaxInstances
			if maxI <= 0 {
				maxI = 100
			}
			rps := runRPS[pid+"/"+sn]
			if v := svc.Labels["sim-rps"]; v != "" {
				n, _ := strconv.Atoi(v)
				rps += n
			}
			inst := int(math.Ceil(float64(rps) / float64(conc) * 4))
			if inst < svc.MinInstances {
				inst = svc.MinInstances
			}
			if inst > maxI {
				inst = maxI
			}
			if rps > 0 && inst < 1 {
				inst = 1
			}
			svc.Instances = inst
			s.Metric("run/"+sn+"/instance_count", float64(inst))
		}
		s.reconcileMIGs(pid)
		s.updateSQLConnections(pid)
		for _, cn := range SortedKeys(p.Clusters) {
			s.tickK8s(pid, p.Clusters[cn], rpsBySvc[pid+"/"+cn])
		}
	}
	for _, t := range s.Traffic {
		s.runTraffic(t)
	}
	for _, pid := range SortedKeys(s.Projects) {
		s.deliverPubSub(pid)
		s.evalLogMetrics(pid)
		s.evalAlerts(pid)
		s.evalUptime(pid)
		_, total := s.CostEstimate(pid)
		s.Metric("billing/"+pid+"/monthly_estimate", total)
		p := s.Projects[pid]
		for _, n := range SortedKeys(p.Instances) {
			vm := p.Instances[n]
			cpu := 3.0
			if vm.Labels["sim-cpu"] == "high" || vm.GPUs > 0 {
				cpu = 97
			}
			if vm.Status != "RUNNING" {
				cpu = 0
			}
			vm.CPU = cpu
			s.Metric("compute/"+n+"/cpu_utilization", cpu)
		}
	}
}

func (s *State) defaultProject() string {
	for _, k := range SortedKeys(s.Projects) {
		return k
	}
	return ""
}

// ResolveTrafficURL turns a traffic target into a URL.
func (s *State) ResolveTrafficURL(project, target, path string) string {
	p := s.Projects[project]
	if p == nil {
		return target
	}
	switch {
	case strings.HasPrefix(target, "run:"):
		if svc := p.RunServices[strings.TrimPrefix(target, "run:")]; svc != nil {
			return svc.URL + path
		}
		return ""
	case strings.HasPrefix(target, "lb:"):
		if fr := p.ForwardingRules[strings.TrimPrefix(target, "lb:")]; fr != nil {
			return "http://" + fr.IP + path
		}
		return ""
	case strings.HasPrefix(target, "vm:"):
		parts := strings.Split(strings.TrimPrefix(target, "vm:"), ":")
		if vm := p.Instances[parts[0]]; vm != nil {
			port := "80"
			if len(parts) > 1 {
				port = parts[1]
			}
			return "http://" + vm.ExternalIP + ":" + port + path
		}
		return ""
	case strings.HasPrefix(target, "k8s:"):
		parts := strings.SplitN(strings.TrimPrefix(target, "k8s:"), "/", 2)
		if c := p.Clusters[parts[0]]; c != nil && c.K8s != nil && len(parts) == 2 {
			for _, ns := range c.K8s.Namespaces {
				if svc := ns.Services[parts[1]]; svc != nil && svc.ExternalIP != "" {
					return "http://" + svc.ExternalIP + path
				}
				if ing := ns.Ingresses[parts[1]]; ing != nil {
					return "http://" + ing.IP + path
				}
			}
		}
		return ""
	}
	return target + path
}

func (s *State) runTraffic(t TrafficSpec) {
	pid := t.Project
	if pid == "" {
		pid = s.defaultProject()
	}
	p := s.Projects[pid]
	if p == nil {
		return
	}
	if strings.HasPrefix(t.Target, "pubsub:") {
		topic := strings.TrimPrefix(t.Target, "pubsub:")
		for i := 0; i < max(1, t.RPS/10); i++ {
			s.Publish(pid, topic, fmt.Sprintf(`{"orderId":"%s","amount":%d}`, s.ID(8), 10+s.Rand().Intn(90)), nil)
		}
		return
	}
	path := t.Path
	u := s.ResolveTrafficURL(pid, t.Target, path)
	name := t.Name
	if name == "" {
		name = t.Target
	}
	var res HTTPResponse
	if u == "" {
		res = HTTPResponse{Error: "target not found"}
	} else {
		from := InternetEndpoint("198.51.100.23")
		if strings.HasPrefix(t.Target, "internal:") {
			parts := strings.SplitN(strings.TrimPrefix(t.Target, "internal:"), " ", 2)
			if vm := p.Instances[parts[0]]; vm != nil && len(parts) == 2 {
				from = s.VMEndpoint(pid, vm)
				u = parts[1] + path
			}
		}
		res = s.HTTP(HTTPRequest{From: from, URL: u, SourceIP: "198.51.100.23"})
	}
	status := res.Status
	if status == 0 {
		status = 504
	}
	errRate := 0.0
	if status >= 500 {
		errRate = 1.0
	} else if status >= 400 {
		errRate = 0.5
	}
	latency := 45 + s.Rand().Intn(30)
	if status >= 500 {
		latency = 10000 + s.Rand().Intn(20000)
	}
	s.Metric("traffic/"+name+"/requests", float64(t.RPS*60))
	s.Metric("traffic/"+name+"/error_rate", errRate)
	s.Metric("traffic/"+name+"/latency_ms", float64(latency))
	resType, labels := "http_load_balancer", map[string]string{"forwarding_rule_name": strings.TrimPrefix(t.Target, "lb:")}
	switch {
	case strings.HasPrefix(res.Target, "run:"):
		resType, labels = "cloud_run_revision", map[string]string{"service_name": strings.TrimPrefix(res.Target, "run:"), "location": s.runRegion(pid, strings.TrimPrefix(res.Target, "run:"))}
	case strings.HasPrefix(res.Target, "vm:"):
		resType, labels = "gce_instance", map[string]string{"instance_id": strings.TrimPrefix(res.Target, "vm:")}
	case strings.HasPrefix(res.Target, "k8s:"):
		resType, labels = "k8s_container", map[string]string{"container_name": strings.TrimPrefix(res.Target, "k8s:")}
	}
	sev := "INFO"
	if status >= 500 {
		sev = "ERROR"
	} else if status >= 400 {
		sev = "WARNING"
	}
	s.Log(pid, LogEntry{Severity: sev, LogName: "requests", Resource: LogResource{Type: resType, Labels: labels},
		HTTP: &HTTPRequestLog{Method: "GET", URL: u, Status: status, Latency: fmt.Sprintf("%.3fs", float64(latency)/1000)}})
	if status >= 500 {
		msg := res.Body
		if res.Error != "" {
			msg = res.Error
		}
		if msg == "" {
			msg = "upstream request timeout"
		}
		s.Log(pid, LogEntry{Severity: "ERROR", LogName: "stderr", Resource: LogResource{Type: resType, Labels: labels}, Text: msg})
	}
	// Successful order requests publish events.
	if status == 200 && strings.HasPrefix(res.Target, "run:") {
		if svc := p.RunServices[strings.TrimPrefix(res.Target, "run:")]; svc != nil && svc.Env["TOPIC"] != "" && strings.Contains(path, "order") {
			tp, tn := splitResource(svc.Env["TOPIC"], pid, "topics")
			s.Publish(tp, tn, fmt.Sprintf(`{"orderId":"%s"}`, s.ID(8)), nil)
		}
	}
}

func (s *State) runRegion(pid, svc string) string {
	if p := s.Projects[pid]; p != nil {
		if r := p.RunServices[svc]; r != nil {
			return r.Region
		}
	}
	return ""
}

// Publish sends a message to a topic and fans it out to subscriptions.
func (s *State) Publish(project, topic, data string, attrs map[string]string) (string, error) {
	p := s.Projects[project]
	if p == nil || p.Topics[topic] == nil {
		return "", fmt.Errorf("NOT_FOUND: Resource not found (resource=%s)", topic)
	}
	p.Topics[topic].Published++
	id := fmt.Sprintf("%d", 9000000000000+s.Rand().Int63n(999999999))
	for _, sn := range SortedKeys(p.Subs) {
		sub := p.Subs[sn]
		if sub.Topic == topic {
			if sub.Filter != "" && !pubsubFilter(sub.Filter, attrs) {
				continue
			}
			sub.Backlog = append(sub.Backlog, Message{ID: id, Data: data, Attributes: attrs, Published: s.Now()})
			if len(sub.Backlog) > 1000 {
				sub.Backlog = sub.Backlog[len(sub.Backlog)-1000:]
			}
		}
	}
	return id, nil
}

func pubsubFilter(f string, attrs map[string]string) bool {
	// attributes.key = "value"
	f = strings.TrimSpace(f)
	if strings.HasPrefix(f, "attributes.") {
		kv := strings.SplitN(strings.TrimPrefix(f, "attributes."), "=", 2)
		if len(kv) == 2 {
			return attrs[strings.TrimSpace(kv[0])] == strings.Trim(strings.TrimSpace(kv[1]), `"`)
		}
	}
	return true
}

func (s *State) deliverPubSub(pid string) {
	p := s.Projects[pid]
	for _, sn := range SortedKeys(p.Subs) {
		sub := p.Subs[sn]
		if sub.PushEndpoint == "" && sub.BQTable == "" {
			s.Metric("pubsub/"+sn+"/num_undelivered_messages", float64(len(sub.Backlog)))
			continue
		}
		var keep []Message
		for _, m := range sub.Backlog {
			ok := false
			errMsg := ""
			if sub.BQTable != "" {
				ok = s.writeBQSubscription(pid, sub)
				if !ok {
					errMsg = "BigQuery table not found or permission denied"
				}
			} else {
				principal := ""
				if sub.PushSA != "" {
					principal = "serviceAccount:" + sub.PushSA
				}
				r := s.HTTP(HTTPRequest{URL: sub.PushEndpoint, Principal: principal, From: Endpoint{Kind: "google"}})
				ok = r.Status >= 200 && r.Status < 300
				errMsg = fmt.Sprintf("push endpoint returned %d", r.Status)
				if r.Status == 0 {
					errMsg = r.Error
				}
			}
			if ok {
				sub.Acked++
				continue
			}
			m.Attempts++
			if sub.DeadLetterTopic != "" && sub.MaxDeliveryAttempts > 0 && m.Attempts >= sub.MaxDeliveryAttempts {
				dlp, dlt := splitResource(sub.DeadLetterTopic, pid, "topics")
				if _, err := s.Publish(dlp, dlt, m.Data, m.Attributes); err == nil {
					sub.DeadLettered++
					continue
				}
			}
			keep = append(keep, m)
			if len(keep) == 1 {
				s.Log(pid, LogEntry{Severity: "WARNING", LogName: "pubsub", Resource: LogResource{Type: "pubsub_subscription", Labels: map[string]string{"subscription_id": sn}}, Text: "push delivery failed: " + errMsg})
			}
		}
		sub.Backlog = keep
		s.Metric("pubsub/"+sn+"/num_undelivered_messages", float64(len(sub.Backlog)))
		s.Metric("pubsub/"+sn+"/dead_letter_count", float64(sub.DeadLettered))
	}
}

func (s *State) writeBQSubscription(pid string, sub *Subscription) bool {
	parts := strings.Split(strings.ReplaceAll(sub.BQTable, ":", "."), ".")
	if len(parts) < 2 {
		return false
	}
	proj, ds, tb := pid, parts[len(parts)-2], parts[len(parts)-1]
	if len(parts) == 3 {
		proj = parts[0]
	}
	p := s.Projects[proj]
	if p == nil || p.Datasets[ds] == nil || p.Datasets[ds].Tables[tb] == nil {
		return false
	}
	p.Datasets[ds].Tables[tb].Rows++
	return true
}

func (s *State) evalLogMetrics(pid string) {
	p := s.Projects[pid]
	now := s.Now()
	for _, mn := range SortedKeys(p.LogMetrics) {
		lm := p.LogMetrics[mn]
		count := 0
		for i := len(s.Logs) - 1; i >= 0; i-- {
			e := s.Logs[i]
			if e.Timestamp != now {
				break
			}
			if e.Project == pid && MatchFilter(lm.Filter, e) {
				count++
			}
		}
		s.Metric("logging/user/"+mn, float64(count))
	}
}

// MetricForFilter maps a Monitoring filter to a simulator series.
func (s *State) MetricForFilter(pid, filter string) string {
	if i := strings.Index(filter, "logging.googleapis.com/user/"); i >= 0 {
		rest := filter[i+len("logging.googleapis.com/user/"):]
		name := strings.FieldsFunc(rest, func(r rune) bool { return r == '"' || r == ' ' || r == '\'' })
		if len(name) > 0 {
			return "logging/user/" + name[0]
		}
	}
	if strings.Contains(filter, "run.googleapis.com/request_count") || strings.Contains(filter, "loadbalancing.googleapis.com") {
		for _, t := range s.Traffic {
			n := t.Name
			if n == "" {
				n = t.Target
			}
			return "traffic/" + n + "/error_rate"
		}
	}
	if strings.Contains(filter, "cloudsql.googleapis.com/database/cpu") {
		for _, n := range SortedKeys(s.Projects[pid].SQLInstances) {
			return "sql/" + n + "/cpu"
		}
	}
	if strings.Contains(filter, "cloudsql.googleapis.com/database/postgresql/num_backends") {
		for _, n := range SortedKeys(s.Projects[pid].SQLInstances) {
			return "sql/" + n + "/connections"
		}
	}
	if strings.Contains(filter, "compute.googleapis.com/instance/cpu/utilization") {
		return "compute/*/cpu_utilization"
	}
	if strings.Contains(filter, "pubsub.googleapis.com/subscription/num_undelivered_messages") {
		for _, n := range SortedKeys(s.Projects[pid].Subs) {
			return "pubsub/" + n + "/num_undelivered_messages"
		}
	}
	if strings.Contains(filter, "billing") {
		return "billing/" + pid + "/monthly_estimate"
	}
	return ""
}

func (s *State) evalAlerts(pid string) {
	p := s.Projects[pid]
	for _, an := range SortedKeys(p.AlertPolicies) {
		ap := p.AlertPolicies[an]
		if !ap.Enabled {
			ap.Firing = false
			continue
		}
		firing := false
		for _, c := range ap.Conditions {
			series := s.MetricForFilter(pid, c.Filter)
			if series == "" {
				continue
			}
			var v float64
			if strings.Contains(series, "*") {
				for k := range s.Metrics {
					if strings.HasPrefix(k, "compute/") && strings.HasSuffix(k, "/cpu_utilization") {
						if x, ok := s.LastMetric(k); ok && x > v {
							v = x
						}
					}
				}
			} else {
				v, _ = s.LastMetric(series)
			}
			switch c.Comparison {
			case "COMPARISON_LT":
				firing = firing || v < c.Threshold
			default:
				firing = firing || v > c.Threshold
			}
		}
		if firing && !ap.Firing {
			s.Log(pid, LogEntry{Severity: "WARNING", LogName: "monitoring.googleapis.com%2Fincidents", Resource: LogResource{Type: "alerting_policy", Labels: map[string]string{"policy": ap.DisplayName}}, Text: "Incident opened for policy " + ap.DisplayName})
		}
		ap.Firing = firing
	}
}

func (s *State) evalUptime(pid string) {
	p := s.Projects[pid]
	for _, n := range SortedKeys(p.UptimeChecks) {
		u := p.UptimeChecks[n]
		scheme := "http"
		if u.Port == 443 {
			scheme = "https"
		}
		r := s.HTTP(HTTPRequest{URL: fmt.Sprintf("%s://%s%s", scheme, u.Host, u.Path), From: InternetEndpoint("35.186.1.1"), SourceIP: "35.186.1.1"})
		u.Passing = r.Status == 200
		v := 0.0
		if u.Passing {
			v = 1
		}
		s.Metric("uptime/"+n+"/check_passed", v)
	}
}

// updateSQLConnections estimates open DB connections from connection pools.
func (s *State) updateSQLConnections(pid string) {
	p := s.Projects[pid]
	for _, n := range SortedKeys(p.SQLInstances) {
		in := p.SQLInstances[n]
		conns := 0
		conn := pid + ":" + in.Region + ":" + in.Name
		matches := func(env map[string]string) bool {
			h := env["DB_HOST"]
			return h != "" && (h == in.PrivateIP || h == in.PublicIP || h == "/cloudsql/"+conn)
		}
		for _, sn := range SortedKeys(p.RunServices) {
			svc := p.RunServices[sn]
			b, _ := s.LookupImage(svc.Image)
			if b == nil || !matches(svc.Env) {
				continue
			}
			pool := 5
			if b.PoolEnv != "" && svc.Env[b.PoolEnv] != "" {
				pool, _ = strconv.Atoi(svc.Env[b.PoolEnv])
			}
			inst := svc.Instances
			if inst < 1 {
				inst = 1
			}
			conns += pool * inst
		}
		for _, vn := range SortedKeys(p.Instances) {
			ls, _ := s.VMListeners(pid, p.Instances[vn])
			for _, l := range ls {
				if l.Env != nil && matches(l.Env) {
					pool := 5
					if l.Behavior != nil && l.Behavior.PoolEnv != "" && l.Env[l.Behavior.PoolEnv] != "" {
						pool, _ = strconv.Atoi(l.Env[l.Behavior.PoolEnv])
					}
					conns += pool
				}
			}
		}
		if v := in.Flags["sim-extra-connections"]; v != "" {
			x, _ := strconv.Atoi(v)
			conns += x
		}
		in.Connections = conns
		maxc := DBMaxConnections(in)
		cpu := math.Min(100, float64(conns)/float64(max(1, maxc))*60+float64(len(in.SlowQueries))*25)
		in.CPU = cpu
		s.Metric("sql/"+n+"/connections", float64(conns))
		s.Metric("sql/"+n+"/cpu", cpu)
		if conns > maxc {
			s.Log(pid, LogEntry{Severity: "ERROR", LogName: "cloudsql.googleapis.com%2Fpostgres.log", Resource: LogResource{Type: "cloudsql_database", Labels: map[string]string{"database_id": pid + ":" + n}},
				Text: fmt.Sprintf("FATAL:  remaining connection slots are reserved for non-replication superuser connections (active=%d max_connections=%d)", conns, maxc)})
		}
		for _, q := range in.SlowQueries {
			s.Log(pid, LogEntry{Severity: "WARNING", LogName: "cloudsql.googleapis.com%2Fpostgres.log", Resource: LogResource{Type: "cloudsql_database", Labels: map[string]string{"database_id": pid + ":" + n}},
				Text: "LOG:  duration: 8421.337 ms  statement: " + q})
		}
	}
}

// reconcileMIGs creates/deletes instances to match target size and autoscaler.
func (s *State) reconcileMIGs(pid string) {
	p := s.Projects[pid]
	for _, gn := range SortedKeys(p.InstanceGroups) {
		ig := p.InstanceGroups[gn]
		if !ig.Managed {
			continue
		}
		if ig.Autoscaler != nil {
			target := ig.Autoscaler.Min
			if ig.Autoscaler.TargetCPU > 0 && ig.Autoscaler.TargetCPU < 0.05 {
				target = ig.Autoscaler.Max // pathological target utilisation
			}
			if v := ig.NamedPorts["sim-load"]; v > 0 {
				need := int(math.Ceil(float64(v) / (ig.Autoscaler.TargetCPU * 100)))
				if need > target {
					target = need
				}
			}
			if target > ig.Autoscaler.Max {
				target = ig.Autoscaler.Max
			}
			ig.TargetSize = target
		}
		s.ResizeMIG(pid, ig)
	}
}

// ResizeMIG materialises instances for a managed group.
func (s *State) ResizeMIG(pid string, ig *InstanceGroup) {
	p := s.Projects[pid]
	tpl := p.InstanceTemplates[ig.Template]
	for len(ig.Instances) < ig.TargetSize && tpl != nil {
		name := fmt.Sprintf("%s-%s", ig.BaseInstanceName, s.ID(4))
		vm := &Instance{Name: name, Zone: ig.Zone, MachineType: tpl.MachineType, Status: "RUNNING", Tags: append([]string{}, tpl.Tags...),
			Labels: copyMap(tpl.Labels), Metadata: copyMap(tpl.Metadata), Network: tpl.Network, Subnet: tpl.Subnet, ServiceAccount: tpl.ServiceAccount,
			Scopes: []string{"default"}, Image: tpl.Image, Group: ig.Name, CreatedBy: "serviceAccount:" + p.Number + "@cloudservices.gserviceaccount.com", ShieldedVM: true}
		if sn := p.Subnets[tpl.Subnet]; sn != nil {
			vm.InternalIP = s.AllocIP(sn.Range)
		}
		if !tpl.NoAddress {
			vm.ExternalIP = s.ExternalIP()
		}
		if vm.ServiceAccount == "" {
			vm.ServiceAccount = p.Number + "-compute@developer.gserviceaccount.com"
		}
		p.Instances[name] = vm
		ig.Instances = append(ig.Instances, name)
	}
	for len(ig.Instances) > ig.TargetSize {
		last := ig.Instances[len(ig.Instances)-1]
		delete(p.Instances, last)
		ig.Instances = ig.Instances[:len(ig.Instances)-1]
	}
}

func copyMap(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = v
	}
	return out
}
