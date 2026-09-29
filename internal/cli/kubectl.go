package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
	"gopkg.in/yaml.v3"
)

var kindAlias = map[string]string{
	"po": "pods", "pod": "pods", "pods": "pods", "deploy": "deployments", "deployment": "deployments", "deployments": "deployments",
	"svc": "services", "service": "services", "services": "services", "hpa": "hpa", "horizontalpodautoscaler": "hpa", "horizontalpodautoscalers": "hpa",
	"no": "nodes", "node": "nodes", "nodes": "nodes", "ns": "namespaces", "namespace": "namespaces", "namespaces": "namespaces",
	"cm": "configmaps", "configmap": "configmaps", "configmaps": "configmaps", "secret": "secrets", "secrets": "secrets",
	"sa": "serviceaccounts", "serviceaccount": "serviceaccounts", "serviceaccounts": "serviceaccounts", "ing": "ingresses", "ingress": "ingresses", "ingresses": "ingresses",
	"netpol": "networkpolicies", "networkpolicy": "networkpolicies", "networkpolicies": "networkpolicies", "all": "all", "events": "events", "ev": "events", "rs": "replicasets", "replicasets": "replicasets",
	"ep": "endpoints", "endpoints": "endpoints",
	"pvc": "persistentvolumeclaims", "persistentvolumeclaim": "persistentvolumeclaims", "persistentvolumeclaims": "persistentvolumeclaims",
	"role": "roles", "roles": "roles", "rolebinding": "rolebindings", "rolebindings": "rolebindings", "sc": "storageclasses", "storageclass": "storageclasses", "storageclasses": "storageclasses",
}

func (s *Session) kube() (*sim.Project, *sim.Cluster, error) {
	if s.Kube.Cluster == "" {
		return nil, nil, fail(1, "The connection to the server localhost:8080 was refused - did you specify the right host or port?\n(hint: run `gcloud container clusters get-credentials CLUSTER --zone ZONE`)")
	}
	p := s.State.Projects[s.Kube.Project]
	if p == nil || p.Clusters[s.Kube.Cluster] == nil {
		return nil, nil, fail(1, "Unable to connect to the server: cluster %s no longer exists", s.Kube.Cluster)
	}
	c := p.Clusters[s.Kube.Cluster]
	if len(c.MasterAuthNets) > 0 {
		ok := false
		for _, n := range c.MasterAuthNets {
			if sim.IPInCIDR(sim.StudentIP, n) {
				ok = true
			}
		}
		if !ok {
			return nil, nil, fail(1, "Unable to connect to the server: dial tcp %s:443: i/o timeout (Cloud Shell IP %s is not in master authorized networks)", c.Endpoint, sim.StudentIP)
		}
	}
	if !s.State.Allowed(s.Principal(), "container.clusters.get", sim.ProjectResource(p.ID)) {
		return nil, nil, fail(1, "error: You must be logged in to the server (Unauthorized)")
	}
	return p, c, nil
}

func (s *Session) kneed(p *sim.Project, perm string) error {
	if s.State.Allowed(s.Principal(), perm, sim.ProjectResource(p.ID)) {
		return nil
	}
	who := strings.SplitN(s.Principal(), ":", 2)[1]
	parts := strings.Split(perm, ".")
	return fail(1, "Error from server (Forbidden): %s is forbidden: User \"%s\" cannot %s resource \"%s\" in API group \"apps\": requires one of [\"%s\"] permission(s).", parts[1], who, parts[len(parts)-1], parts[1], perm)
}

func (s *Session) kubectl(args []string, stdin string) (string, error) {
	// split args after "--"
	var tail []string
	for i, a := range args {
		if a == "--" {
			tail = args[i+1:]
			args = args[:i]
			break
		}
	}
	pos, f := parseArgs(args)
	if len(pos) == 0 {
		return "kubectl controls the Kubernetes cluster manager.\n", nil
	}
	if pos[0] == "config" {
		switch {
		case len(pos) > 1 && pos[1] == "current-context":
			if s.Kube.Cluster == "" {
				return "", fail(1, "error: current-context is not set")
			}
			return fmt.Sprintf("gke_%s_%s\n", s.Kube.Project, s.Kube.Cluster), nil
		case len(pos) > 1 && pos[1] == "set-context":
			if ns := last(f["namespace"]); ns != "" {
				s.Kube.Namespace = ns
			}
			return "Context modified.\n", nil
		}
		return "", nil
	}
	if pos[0] == "version" {
		return "Client Version: v1.31.1\nServer Version: v1.31.1-gke.1678000\n", nil
	}
	p, c, err := s.kube()
	if err != nil {
		return "", err
	}
	ns := s.Kube.Namespace
	if v := last(f["n"]); v != "" {
		ns = v
	}
	if v := last(f["namespace"]); v != "" {
		ns = v
	}
	allNS := f["A"] != nil || f["all-namespaces"] != nil
	k := c.K8s
	opts := kubeOpts{sel: parseLabelSelector(firstNonEmptyStr(last(f["l"]), last(f["selector"]))), showLabels: f["show-labels"] != nil}
	out := last(f["o"])
	if out == "" {
		out = last(f["output"])
	}
	switch pos[0] {
	case "apply", "create":
		if file := last(f["f"]); file != "" || last(f["filename"]) != "" {
			if file == "" {
				file = last(f["filename"])
			}
			if err := s.kneed(p, "container.deployments.create"); err != nil {
				return "", err
			}
			doc := stdin
			if file != "-" {
				var ok bool
				doc, ok = s.Files[s.path(file)]
				if !ok {
					// directory apply
					var parts []string
					for _, fn := range sim.SortedKeys(s.Files) {
						if strings.HasPrefix(fn, strings.TrimSuffix(s.path(file), "/")+"/") && (strings.HasSuffix(fn, ".yaml") || strings.HasSuffix(fn, ".yml")) {
							parts = append(parts, s.Files[fn])
						}
					}
					if len(parts) == 0 {
						return "", fail(1, "error: the path \"%s\" does not exist", file)
					}
					doc = strings.Join(parts, "\n---\n")
				}
			}
			lines, err := s.State.ApplyManifest(c, ns, doc)
			res := strings.Join(lines, "\n")
			if res != "" {
				res += "\n"
			}
			if err != nil {
				return res, fail(1, "%v", err)
			}
			s.State.Audit(p.ID, s.Principal(), "k8s.io", "io.k8s.apply", "projects/"+p.ID+"/clusters/"+c.Name)
			return res, nil
		}
		if pos[0] == "create" && len(pos) >= 3 {
			return s.kubectlCreate(p, c, ns, pos, f)
		}
		return "", fail(1, "error: must specify one of -f and -k")
	case "expose":
		if len(pos) < 3 && !strings.Contains(strings.Join(pos, " "), "/") {
			return "", fail(1, "error: You must provide one or more resources by argument or filename.")
		}
		name := pos[len(pos)-1]
		name = name[strings.LastIndex(name, "/")+1:]
		d := k.NS(ns).Deployments[name]
		if d == nil {
			return "", fail(1, "Error from server (NotFound): deployments.apps \"%s\" not found", name)
		}
		port, _ := strconv.Atoi(last(f["port"]))
		tp, _ := strconv.Atoi(last(f["target-port"]))
		if tp == 0 {
			tp = port
		}
		typ := last(f["type"])
		if typ == "" {
			typ = "ClusterIP"
		}
		svcName := last(f["name"])
		if svcName == "" {
			svcName = name
		}
		manifest := fmt.Sprintf("apiVersion: v1\nkind: Service\nmetadata:\n  name: %s\nspec:\n  type: %s\n  selector:\n", svcName, typ)
		for kk, v := range d.Selector {
			manifest += fmt.Sprintf("    %s: %s\n", kk, v)
		}
		manifest += fmt.Sprintf("  ports:\n  - port: %d\n    targetPort: %d\n", port, tp)
		if _, err := s.State.ApplyManifest(c, ns, manifest); err != nil {
			return "", fail(1, "%v", err)
		}
		return "service/" + svcName + " exposed\n", nil
	case "get":
		if len(pos) < 2 {
			return "", fail(1, "You must specify the type of resource to get.")
		}
		kinds := strings.Split(pos[1], ",")
		var b strings.Builder
		for _, kd := range kinds {
			name := ""
			if strings.Contains(kd, "/") {
				kd, name, _ = strings.Cut(kd, "/")
			} else if len(pos) > 2 {
				name = pos[2]
			}
			kind := kindAlias[strings.ToLower(kd)]
			if kind == "" {
				return b.String(), fail(1, "error: the server doesn't have a resource type \"%s\"", kd)
			}
			o, err := s.kubectlGet(p, c, ns, allNS, kind, name, out, opts)
			b.WriteString(o)
			if err != nil {
				return b.String(), err
			}
		}
		return b.String(), nil
	case "describe":
		if len(pos) < 2 {
			return "", fail(1, "You must specify the type of resource to describe.")
		}
		kd, name := pos[1], ""
		if strings.Contains(kd, "/") {
			kd, name, _ = strings.Cut(kd, "/")
		} else if len(pos) > 2 {
			name = pos[2]
		}
		return s.kubectlDescribe(p, c, ns, kindAlias[kd], name, opts)
	case "logs":
		if len(pos) < 2 {
			return "", fail(1, "error: expected POD")
		}
		target := pos[1]
		if strings.HasPrefix(target, "deployment/") || strings.HasPrefix(target, "deploy/") {
			target = strings.SplitN(target, "/", 2)[1]
		}
		for _, pd := range s.State.ComputePods(p.ID, c)[ns] {
			if pd.Name == target || pd.Owner == target {
				return s.podLogs(p, c, ns, pd), nil
			}
		}
		return "", fail(1, "Error from server (NotFound): pods \"%s\" not found", pos[1])
	case "scale":
		if err := s.kneed(p, "container.deployments.update"); err != nil {
			return "", err
		}
		target := strings.Join(pos[1:], "/")
		name := target[strings.LastIndex(target, "/")+1:]
		d := k.NS(ns).Deployments[name]
		if d == nil {
			return "", fail(1, "Error from server (NotFound): deployments.apps \"%s\" not found", name)
		}
		n, err := strconv.Atoi(last(f["replicas"]))
		if err != nil {
			return "", fail(1, "error: --replicas is required")
		}
		d.Replicas = n
		return "deployment.apps/" + name + " scaled\n", nil
	case "set":
		if len(pos) < 3 {
			return "", fail(1, "error: usage: kubectl set image|resources|env deployment/NAME ...")
		}
		if err := s.kneed(p, "container.deployments.update"); err != nil {
			return "", err
		}
		target := pos[2]
		rest := pos[3:]
		if !strings.Contains(target, "/") && len(pos) > 3 {
			target = pos[3]
			rest = pos[4:]
		}
		name := target[strings.LastIndex(target, "/")+1:]
		d := k.NS(ns).Deployments[name]
		if d == nil || len(d.Template.Containers) == 0 {
			return "", fail(1, "Error from server (NotFound): deployments.apps \"%s\" not found", name)
		}
		tpl := deepCopyTemplate(d.Template)
		switch pos[1] {
		case "image":
			for _, kv := range rest {
				cn, img, ok := strings.Cut(kv, "=")
				if !ok {
					continue
				}
				found := false
				for i := range tpl.Containers {
					if tpl.Containers[i].Name == cn || cn == "*" {
						tpl.Containers[i].Image = img
						found = true
					}
				}
				if !found {
					return "", fail(1, "error: unable to find container named \"%s\"", cn)
				}
			}
		case "resources":
			for _, kv := range strings.Split(last(f["requests"]), ",") {
				kk, v, _ := strings.Cut(kv, "=")
				if kk == "cpu" {
					tpl.Containers[0].Requests.CPU = v
				} else if kk == "memory" {
					tpl.Containers[0].Requests.Memory = v
				}
			}
			for _, kv := range strings.Split(last(f["limits"]), ",") {
				kk, v, _ := strings.Cut(kv, "=")
				if kk == "cpu" {
					tpl.Containers[0].Limits.CPU = v
				} else if kk == "memory" {
					tpl.Containers[0].Limits.Memory = v
				}
			}
		case "env":
			for _, kv := range rest {
				kk, v, ok := strings.Cut(kv, "=")
				if ok {
					if strings.HasSuffix(kk, "-") {
						delete(tpl.Containers[0].Env, strings.TrimSuffix(kk, "-"))
					} else {
						tpl.Containers[0].Env[kk] = v
					}
				} else if strings.HasSuffix(kv, "-") {
					delete(tpl.Containers[0].Env, strings.TrimSuffix(kv, "-"))
				}
			}
		case "serviceaccount", "sa":
			if len(rest) > 0 {
				tpl.ServiceAccount = rest[len(rest)-1]
			}
		}
		d.History = append(d.History, d.Template)
		d.Template = tpl
		d.Revision++
		s.State.Audit(p.ID, s.Principal(), "k8s.io", "io.k8s.apps.v1.deployments.patch", "projects/"+p.ID+"/clusters/"+c.Name+"/deployments/"+name)
		return "deployment.apps/" + name + " " + map[string]string{"image": "image updated", "resources": "resource requirements updated", "env": "env updated", "serviceaccount": "serviceaccount updated", "sa": "serviceaccount updated"}[pos[1]] + "\n", nil
	case "autoscale":
		target := strings.Join(pos[1:], "/")
		name := target[strings.LastIndex(target, "/")+1:]
		if k.NS(ns).Deployments[name] == nil {
			return "", fail(1, "Error from server (NotFound): deployments.apps \"%s\" not found", name)
		}
		mn, _ := strconv.Atoi(last(f["min"]))
		mx, _ := strconv.Atoi(last(f["max"]))
		cpu, _ := strconv.Atoi(last(f["cpu-percent"]))
		if mx == 0 {
			return "", fail(1, "error: --max=MAXPODS is required and must be at least 1")
		}
		if mn == 0 {
			mn = 1
		}
		if cpu == 0 {
			cpu = 80
		}
		k.NS(ns).HPAs[name] = &sim.HPA{Name: name, Target: name, Min: mn, Max: mx, TargetCPU: cpu}
		return "horizontalpodautoscaler.autoscaling/" + name + " autoscaled\n", nil
	case "rollout":
		if len(pos) < 3 {
			return "", fail(1, "error: usage: kubectl rollout status|history|undo|restart deployment/NAME")
		}
		target := strings.Join(pos[2:], "/")
		name := target[strings.LastIndex(target, "/")+1:]
		d := k.NS(ns).Deployments[name]
		if d == nil {
			return "", fail(1, "Error from server (NotFound): deployments.apps \"%s\" not found", name)
		}
		switch pos[1] {
		case "status":
			s.State.ComputePods(p.ID, c)
			if d.Available >= d.Replicas {
				return fmt.Sprintf("deployment \"%s\" successfully rolled out\n", name), nil
			}
			return fmt.Sprintf("Waiting for deployment \"%s\" rollout to finish: %d of %d updated replicas are available...\n", name, d.Available, d.Replicas), fail(1, "error: deployment \"%s\" exceeded its progress deadline", name)
		case "history":
			var b strings.Builder
			b.WriteString("deployment.apps/" + name + "\nREVISION  CHANGE-CAUSE\n")
			start := d.Revision - len(d.History)
			for i := range d.History {
				b.WriteString(fmt.Sprintf("%d         image=%s\n", start+i, d.History[i].Containers[0].Image))
			}
			b.WriteString(fmt.Sprintf("%d         image=%s\n", d.Revision, d.Template.Containers[0].Image))
			return b.String(), nil
		case "undo":
			if len(d.History) == 0 {
				return "", fail(1, "error: no rollout history found for deployment \"%s\"", name)
			}
			idx := len(d.History) - 1
			if tr := last(f["to-revision"]); tr != "" {
				n, _ := strconv.Atoi(tr)
				idx = n - (d.Revision - len(d.History))
				if idx < 0 || idx >= len(d.History) {
					return "", fail(1, "error: unable to find specified revision %s in history", tr)
				}
			}
			prev := d.History[idx]
			d.History = append(d.History, d.Template)
			d.Template = prev
			d.Revision++
			s.State.Audit(p.ID, s.Principal(), "k8s.io", "io.k8s.apps.v1.deployments.rollback", "projects/"+p.ID+"/clusters/"+c.Name+"/deployments/"+name)
			return "deployment.apps/" + name + " rolled back\n", nil
		case "restart":
			return "deployment.apps/" + name + " restarted\n", nil
		}
	case "delete":
		if err := s.kneed(p, "container.deployments.delete"); err != nil {
			return "", err
		}
		if file := last(f["f"]); file != "" {
			doc := s.Files[s.path(file)]
			var outb strings.Builder
			dec := yaml.NewDecoder(strings.NewReader(doc))
			for {
				var m map[string]any
				if dec.Decode(&m) != nil {
					break
				}
				kind := strings.ToLower(fmt.Sprint(m["kind"]))
				meta, _ := m["metadata"].(map[string]any)
				o, _ := s.kubectlDelete(k.NS(ns), kindAlias[kind], fmt.Sprint(meta["name"]))
				outb.WriteString(o)
			}
			return outb.String(), nil
		}
		if len(pos) < 2 {
			return "", fail(1, "error: resource(s) were provided, but no name was specified")
		}
		kd, name := pos[1], ""
		if strings.Contains(kd, "/") {
			kd, name, _ = strings.Cut(kd, "/")
		} else if len(pos) > 2 {
			name = pos[2]
		}
		return s.kubectlDelete(k.NS(ns), kindAlias[kd], name)
	case "run":
		if len(pos) < 2 {
			return "", fail(1, "error: NAME is required for run")
		}
		img := last(f["image"])
		if img == "" {
			return "", fail(1, "error: --image is required")
		}
		cmd := strings.Join(tail, " ")
		pd := &sim.Pod{Name: pos[1], Labels: map[string]string{"run": pos[1]}, Spec: sim.Container{Name: pos[1], Image: img, Env: map[string]string{}}, Command: cmd, IP: fmt.Sprintf("10.108.99.%d", 10+len(k.NS(ns).Pods))}
		k.NS(ns).Pods[pos[1]] = pd
		if f["rm"] != nil && f["i"] != nil || f["it"] != nil || f["rm"] != nil {
			// one-shot command: execute and remove
			out, err := s.podExec(p, c, ns, pd, tail)
			delete(k.NS(ns).Pods, pos[1])
			return out + "pod \"" + pos[1] + "\" deleted\n", err
		}
		return "pod/" + pos[1] + " created\n", nil
	case "exec":
		if len(pos) < 2 {
			return "", fail(1, "error: expected POD")
		}
		for _, pd := range s.State.ComputePods(p.ID, c)[ns] {
			if pd.Name == pos[1] || strings.TrimPrefix(pos[1], "deploy/") == pd.Owner {
				return s.podExec(p, c, ns, pd, tail)
			}
		}
		return "", fail(1, "Error from server (NotFound): pods \"%s\" not found", pos[1])
	case "top":
		var b strings.Builder
		if len(pos) > 1 && strings.HasPrefix(pos[1], "no") {
			b.WriteString("NAME                                 CPU(cores)   CPU%   MEMORY(bytes)   MEMORY%\n")
			for _, np := range c.NodePools {
				for i := 0; i < np.Count; i++ {
					b.WriteString(fmt.Sprintf("gke-%s-%s-%d   %dm   %d%%   1100Mi   38%%\n", c.Name, np.Name, i, 180+i*20, 9+i))
				}
			}
			return b.String(), nil
		}
		b.WriteString("NAME                          CPU(cores)   MEMORY(bytes)\n")
		for _, pd := range s.State.ComputePods(p.ID, c)[ns] {
			if pd.Phase != "Running" {
				continue
			}
			v, _ := s.State.LastMetric(fmt.Sprintf("k8s/%s/%s/cpu_millicores", c.Name, pd.Owner))
			b.WriteString(fmt.Sprintf("%-28s  %dm          64Mi\n", pd.Name, int(v)))
		}
		return b.String(), nil
	case "annotate", "label":
		if len(pos) < 3 {
			return "", fail(1, "error: usage: kubectl annotate TYPE NAME KEY=VALUE")
		}
		kd, name := pos[1], pos[2]
		rest := pos[3:]
		if strings.Contains(kd, "/") {
			kd, name, _ = strings.Cut(kd, "/")
			rest = pos[2:]
		}
		kind := kindAlias[kd]
		nsObj := k.NS(ns)
		for _, kv := range rest {
			kk, v, _ := strings.Cut(kv, "=")
			switch kind {
			case "serviceaccounts":
				sa := nsObj.ServiceAccounts[name]
				if sa == nil {
					return "", fail(1, "Error from server (NotFound): serviceaccounts \"%s\" not found", name)
				}
				if sa.Annotations == nil {
					sa.Annotations = map[string]string{}
				}
				if strings.HasSuffix(kk, "-") {
					delete(sa.Annotations, strings.TrimSuffix(kk, "-"))
				} else {
					sa.Annotations[kk] = v
				}
			case "deployments":
				d := nsObj.Deployments[name]
				if d == nil {
					return "", fail(1, "Error from server (NotFound): deployments.apps \"%s\" not found", name)
				}
				if d.Labels == nil {
					d.Labels = map[string]string{}
				}
				d.Labels[kk] = v
			}
		}
		return kind + "/" + name + " annotated\n", nil
	case "auth":
		if as := firstOr(f["as"], ""); len(pos) >= 4 && pos[1] == "can-i" && strings.HasPrefix(as, "system:serviceaccount:") {
			parts := strings.Split(strings.TrimPrefix(as, "system:serviceaccount:"), ":")
			if len(parts) == 2 {
				ns := c.K8s.NS(parts[0])
				if ns.RBACAllows(parts[0], parts[1], pos[2], kindAlias[pos[3]]) {
					return "yes\n", nil
				}
				return "no\n", fail(1, "")
			}
		}
		if len(pos) >= 4 && pos[1] == "can-i" {
			perm := "container." + kindAlias[pos[3]] + "." + map[string]string{"get": "get", "list": "list", "create": "create", "delete": "delete", "update": "update", "patch": "update"}[pos[2]]
			if s.State.Allowed(s.Principal(), perm, sim.ProjectResource(p.ID)) {
				return "yes\n", nil
			}
			return "no\n", fail(1, "")
		}
	case "cluster-info":
		return "Kubernetes control plane is running at https://" + c.Endpoint + "\n", nil
	case "port-forward", "proxy", "attach", "cp", "edit":
		return "", fail(1, "error: `kubectl %s` is not available in the simulator; use kubectl apply / set / patch", pos[0])
	case "patch":
		return "", fail(1, "error: use `kubectl apply -f` with an updated manifest (patch is not supported by the simulator)")
	}
	return "", fail(1, "error: unknown command \"%s\" for \"kubectl\"", pos[0])
}

func deepCopyTemplate(t sim.PodTemplate) sim.PodTemplate {
	b, _ := yaml.Marshal(t)
	var out sim.PodTemplate
	_ = yaml.Unmarshal(b, &out)
	if out.Labels == nil {
		out.Labels = map[string]string{}
	}
	for i := range out.Containers {
		if out.Containers[i].Env == nil {
			out.Containers[i].Env = map[string]string{}
		}
		if out.Containers[i].EnvRefs == nil {
			out.Containers[i].EnvRefs = map[string]string{}
		}
	}
	return out
}

func (s *Session) kubectlCreate(p *sim.Project, c *sim.Cluster, ns string, pos []string, f Flags) (string, error) {
	k := c.K8s
	switch pos[1] {
	case "deployment", "deploy":
		img := last(f["image"])
		if img == "" {
			return "", fail(1, "error: required flag(s) \"image\" not set")
		}
		reps := 1
		if r := last(f["replicas"]); r != "" {
			reps, _ = strconv.Atoi(r)
		}
		port := ""
		if pt := last(f["port"]); pt != "" {
			port = fmt.Sprintf("\n        ports:\n        - containerPort: %s", pt)
		}
		m := fmt.Sprintf("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: %s\n  labels:\n    app: %s\nspec:\n  replicas: %d\n  selector:\n    matchLabels:\n      app: %s\n  template:\n    metadata:\n      labels:\n        app: %s\n    spec:\n      containers:\n      - name: %s\n        image: %s%s\n", pos[2], pos[2], reps, pos[2], pos[2], strings.Split(img[strings.LastIndex(img, "/")+1:], ":")[0], img, port)
		if _, err := s.State.ApplyManifest(c, ns, m); err != nil {
			return "", fail(1, "%v", err)
		}
		return "deployment.apps/" + pos[2] + " created\n", nil
	case "namespace", "ns":
		k.NS(pos[2])
		return "namespace/" + pos[2] + " created\n", nil
	case "serviceaccount", "sa":
		k.NS(ns).ServiceAccounts[pos[2]] = &sim.KSA{Name: pos[2], Annotations: map[string]string{}}
		return "serviceaccount/" + pos[2] + " created\n", nil
	case "secret", "configmap", "cm":
		name := pos[2]
		if pos[1] == "secret" && len(pos) > 3 {
			name = pos[3]
		}
		data := map[string]string{}
		for _, kv := range f["from-literal"] {
			kk, v, _ := strings.Cut(kv, "=")
			data[kk] = v
		}
		for _, ff := range f["from-file"] {
			kk, path, ok := strings.Cut(ff, "=")
			if !ok {
				path, kk = ff, ff[strings.LastIndex(ff, "/")+1:]
			}
			data[kk] = s.Files[s.path(path)]
		}
		if pos[1] == "secret" {
			k.NS(ns).Secrets[name] = data
			return "secret/" + name + " created\n", nil
		}
		k.NS(ns).ConfigMaps[name] = data
		return "configmap/" + name + " created\n", nil
	}
	return "", fail(1, "error: unsupported kind for kubectl create: %s (use -f)", pos[1])
}

func (s *Session) kubectlDelete(ns *sim.Namespace, kind, name string) (string, error) {
	switch kind {
	case "deployments":
		if ns.Deployments[name] == nil {
			return "", fail(1, "Error from server (NotFound): deployments.apps \"%s\" not found", name)
		}
		delete(ns.Deployments, name)
		return "deployment.apps \"" + name + "\" deleted\n", nil
	case "services":
		delete(ns.Services, name)
		return "service \"" + name + "\" deleted\n", nil
	case "hpa":
		delete(ns.HPAs, name)
		return "horizontalpodautoscaler.autoscaling \"" + name + "\" deleted\n", nil
	case "pods":
		if ns.Pods[name] != nil {
			delete(ns.Pods, name)
		}
		return "pod \"" + name + "\" deleted\n", nil
	case "configmaps":
		delete(ns.ConfigMaps, name)
		return "configmap \"" + name + "\" deleted\n", nil
	case "secrets":
		delete(ns.Secrets, name)
		return "secret \"" + name + "\" deleted\n", nil
	case "ingresses":
		delete(ns.Ingresses, name)
		return "ingress.networking.k8s.io \"" + name + "\" deleted\n", nil
	case "networkpolicies":
		delete(ns.NetworkPolicies, name)
		return "networkpolicy.networking.k8s.io \"" + name + "\" deleted\n", nil
	case "serviceaccounts":
		delete(ns.ServiceAccounts, name)
		return "serviceaccount \"" + name + "\" deleted\n", nil
	case "persistentvolumeclaims":
		delete(ns.PVCs, name)
		return "persistentvolumeclaim \"" + name + "\" deleted\n", nil
	case "roles":
		delete(ns.Roles, name)
		return "role.rbac.authorization.k8s.io \"" + name + "\" deleted\n", nil
	case "rolebindings":
		delete(ns.RoleBindings, name)
		return "rolebinding.rbac.authorization.k8s.io \"" + name + "\" deleted\n", nil
	}
	return "", fail(1, "error: the server doesn't have a resource type \"%s\"", kind)
}

func age() string { return "5m" }

// kubeOpts carries list options shared by get and describe.
type kubeOpts struct {
	sel        map[string]string // -l / --selector (equality-based, comma-separated)
	showLabels bool
}

// parseLabelSelector parses "app=web,tier=backend" (also "app==web").
func parseLabelSelector(v string) map[string]string {
	if v == "" {
		return nil
	}
	out := map[string]string{}
	for _, part := range strings.Split(v, ",") {
		k, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(strings.TrimPrefix(val, "="))
		}
	}
	return out
}

// matches reports whether labels satisfy the selector (an empty selector matches everything).
func (o kubeOpts) matches(labels map[string]string) bool {
	for k, v := range o.sel {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func labelString(labels map[string]string) string {
	if len(labels) == 0 {
		return "<none>"
	}
	var parts []string
	for _, k := range sim.SortedKeys(labels) {
		parts = append(parts, k+"="+labels[k])
	}
	return strings.Join(parts, ",")
}

func (s *Session) kubectlGet(p *sim.Project, c *sim.Cluster, nsName string, allNS bool, kind, name, out string, o kubeOpts) (string, error) {
	k := c.K8s
	var nss []string
	if allNS {
		nss = sim.SortedKeys(k.Namespaces)
	} else {
		nss = []string{nsName}
	}
	var rows []any
	var b strings.Builder
	w := func(format string, a ...any) { b.WriteString(fmt.Sprintf(format, a...)) }
	pods := s.State.ComputePods(p.ID, c)
	switch kind {
	case "all":
		for _, kk := range []string{"pods", "services", "deployments", "hpa"} {
			txt, _ := s.kubectlGet(p, c, nsName, allNS, kk, "", out, o)
			b.WriteString(txt + "\n")
		}
		return b.String(), nil
	case "pods":
		hdr := fmt.Sprintf("%-40s %-6s %-26s %-9s %s", "NAME", "READY", "STATUS", "RESTARTS", "AGE")
		if o.showLabels {
			hdr += "   LABELS"
		}
		w("%s\n", hdr)
		for _, ns := range nss {
			for _, pd := range pods[ns] {
				if name != "" && pd.Name != name || !o.matches(pd.Labels) {
					continue
				}
				ready := "0/1"
				if pd.Ready {
					ready = "1/1"
				}
				st := pd.Phase
				if pd.Reason != "" && (strings.HasPrefix(pd.Reason, "CrashLoop") || strings.HasPrefix(pd.Reason, "ImagePull") || strings.HasPrefix(pd.Reason, "CreateContainer")) {
					st = strings.SplitN(pd.Reason, ":", 2)[0]
					st = strings.SplitN(st, " ", 2)[0]
				}
				rows = append(rows, pd)
				line := fmt.Sprintf("%-40s %-6s %-26s %-9d %s", pd.Name, ready, st, pd.Restarts, age())
				if o.showLabels {
					line += "   " + labelString(pd.Labels)
				}
				w("%s\n", line)
			}
		}
	case "deployments":
		w("%-24s %-7s %-11s %-10s %s\n", "NAME", "READY", "UP-TO-DATE", "AVAILABLE", "AGE")
		for _, ns := range nss {
			for _, dn := range sim.SortedKeys(k.Namespaces[ns].Deployments) {
				if name != "" && dn != name {
					continue
				}
				d := k.Namespaces[ns].Deployments[dn]
				if !o.matches(d.Template.Labels) {
					continue
				}
				rows = append(rows, d)
				w("%-24s %-7s %-11d %-10d %s\n", dn, fmt.Sprintf("%d/%d", d.Available, d.Replicas), d.Replicas, d.Available, age())
			}
		}
	case "services":
		w("%-20s %-13s %-15s %-15s %-12s %s\n", "NAME", "TYPE", "CLUSTER-IP", "EXTERNAL-IP", "PORT(S)", "AGE")
		for _, ns := range nss {
			for _, sn := range sim.SortedKeys(k.Namespaces[ns].Services) {
				if name != "" && sn != name {
					continue
				}
				sv := k.Namespaces[ns].Services[sn]
				ext := "<none>"
				if sv.ExternalIP != "" {
					ext = sv.ExternalIP
				}
				var ports []string
				for _, pt := range sv.Ports {
					ports = append(ports, fmt.Sprintf("%d/TCP", pt.Port))
				}
				rows = append(rows, sv)
				w("%-20s %-13s %-15s %-15s %-12s %s\n", sn, sv.Type, sv.ClusterIP, ext, strings.Join(ports, ","), age())
			}
		}
		if !allNS && nsName == "default" && name == "" {
			w("%-20s %-13s %-15s %-15s %-12s %s\n", "kubernetes", "ClusterIP", "10.112.0.1", "<none>", "443/TCP", "1d")
		}
	case "endpoints":
		w("%-20s %-40s %s\n", "NAME", "ENDPOINTS", "AGE")
		for _, ns := range nss {
			for _, sn := range sim.SortedKeys(k.Namespaces[ns].Services) {
				if name != "" && sn != name {
					continue
				}
				sv := k.Namespaces[ns].Services[sn]
				var eps []string
				for _, pd := range pods[ns] {
					if pd.Ready && selectorMatchesCLI(sv.Selector, pd.Labels) {
						for _, pt := range sv.Ports {
							eps = append(eps, fmt.Sprintf("%s:%d", pd.IP, pt.TargetPort))
						}
					}
				}
				ep := strings.Join(eps, ",")
				if ep == "" {
					ep = "<none>"
				}
				rows = append(rows, map[string]any{"name": sn, "endpoints": eps})
				w("%-20s %-40s %s\n", sn, ep, age())
			}
		}
	case "hpa":
		w("%-18s %-28s %-22s %-8s %-8s %-9s %s\n", "NAME", "REFERENCE", "TARGETS", "MINPODS", "MAXPODS", "REPLICAS", "AGE")
		for _, ns := range nss {
			for _, hn := range sim.SortedKeys(k.Namespaces[ns].HPAs) {
				if name != "" && hn != name {
					continue
				}
				h := k.Namespaces[ns].HPAs[hn]
				tgt := fmt.Sprintf("cpu: %d%%/%d%%", h.CurrentCPU, h.TargetCPU)
				if h.Unknown {
					tgt = fmt.Sprintf("cpu: <unknown>/%d%%", h.TargetCPU)
				}
				reps := 0
				if d := k.Namespaces[ns].Deployments[h.Target]; d != nil {
					reps = d.Replicas
				}
				rows = append(rows, h)
				w("%-18s %-28s %-22s %-8d %-8d %-9d %s\n", hn, "Deployment/"+h.Target, tgt, h.Min, h.Max, reps, age())
			}
		}
	case "nodes":
		w("%-44s %-7s %-7s %-4s %s\n", "NAME", "STATUS", "ROLES", "AGE", "VERSION")
		for _, np := range c.NodePools {
			for i := 0; i < np.Count; i++ {
				w("%-44s %-7s %-7s %-4s %s\n", fmt.Sprintf("gke-%s-%s-%s", c.Name, np.Name, s.State.ID(4)), "Ready", "<none>", "1d", "v1.31.1-gke.1678000")
			}
		}
		if c.Autopilot {
			w("%-44s %-7s %-7s %-4s %s\n", "gk3-"+c.Name+"-pool-1-"+s.State.ID(4), "Ready", "<none>", "1d", "v1.31.1-gke.1678000")
		}
	case "namespaces":
		w("%-18s %-7s %s\n", "NAME", "STATUS", "AGE")
		for _, n := range sim.SortedKeys(k.Namespaces) {
			w("%-18s %-7s %s\n", n, "Active", "1d")
		}
	case "configmaps", "secrets", "serviceaccounts", "ingresses", "networkpolicies":
		w("%-24s %s\n", "NAME", "DATA/INFO")
		for _, ns := range nss {
			n := k.Namespaces[ns]
			switch kind {
			case "configmaps":
				for _, x := range sim.SortedKeys(n.ConfigMaps) {
					if name == "" || x == name {
						rows = append(rows, map[string]any{"name": x, "data": n.ConfigMaps[x]})
						w("%-24s %d\n", x, len(n.ConfigMaps[x]))
					}
				}
			case "secrets":
				for _, x := range sim.SortedKeys(n.Secrets) {
					if name == "" || x == name {
						rows = append(rows, map[string]any{"name": x, "type": "Opaque", "keys": len(n.Secrets[x])})
						w("%-24s Opaque %d\n", x, len(n.Secrets[x]))
					}
				}
			case "serviceaccounts":
				for _, x := range sim.SortedKeys(n.ServiceAccounts) {
					if name == "" || x == name {
						rows = append(rows, n.ServiceAccounts[x])
						w("%-24s %v\n", x, n.ServiceAccounts[x].Annotations)
					}
				}
			case "ingresses":
				for _, x := range sim.SortedKeys(n.Ingresses) {
					if name == "" || x == name {
						rows = append(rows, n.Ingresses[x])
						w("%-24s %s\n", x, n.Ingresses[x].IP)
					}
				}
			case "networkpolicies":
				for _, x := range sim.SortedKeys(n.NetworkPolicies) {
					if name == "" || x == name {
						rows = append(rows, n.NetworkPolicies[x])
						w("%-24s %v\n", x, n.NetworkPolicies[x].Selector)
					}
				}
			}
		}
	case "persistentvolumeclaims":
		w("%-20s %-8s %-10s %-14s %s\n", "NAME", "STATUS", "CAPACITY", "STORAGECLASS", "AGE")
		for _, ns := range nss {
			n := k.NS(ns)
			for _, x := range sim.SortedKeys(n.PVCs) {
				pv := n.PVCs[x]
				if name == "" || x == name {
					rows = append(rows, pv)
					w("%-20s %-8s %-10s %-14s %s\n", x, pv.Phase, pv.Size, pv.StorageClass, age())
				}
			}
		}
	case "storageclasses":
		w("%-14s %-24s %s\n", "NAME", "PROVISIONER", "RECLAIMPOLICY")
		for _, sc := range sim.SortedKeys(sim.StorageClasses) {
			w("%-14s %-24s %s\n", sc, "pd.csi.storage.gke.io", "Delete")
		}
	case "roles", "rolebindings":
		w("%-24s %s\n", "NAME", "DETAIL")
		for _, ns := range nss {
			n := k.NS(ns)
			if kind == "roles" {
				for _, x := range sim.SortedKeys(n.Roles) {
					if name == "" || x == name {
						rows = append(rows, n.Roles[x])
						w("%-24s %v\n", x, n.Roles[x].Rules)
					}
				}
			} else {
				for _, x := range sim.SortedKeys(n.RoleBindings) {
					if name == "" || x == name {
						rows = append(rows, n.RoleBindings[x])
						w("%-24s Role/%s → %v\n", x, n.RoleBindings[x].Role, n.RoleBindings[x].Subjects)
					}
				}
			}
		}
	case "events":
		w("LAST SEEN   TYPE      REASON    OBJECT   MESSAGE\n")
		for _, ns := range nss {
			for _, pd := range pods[ns] {
				if pd.Reason != "" {
					w("1m          Warning   %s   pod/%s   %s\n", strings.SplitN(pd.Reason, ":", 2)[0], pd.Name, pd.Reason)
				}
			}
		}
	default:
		return "", fail(1, "error: the server doesn't have a resource type \"%s\"", kind)
	}
	if name != "" && len(rows) == 0 && kind != "nodes" && kind != "namespaces" && kind != "events" {
		return "", fail(1, "Error from server (NotFound): %s \"%s\" not found", kind, name)
	}
	switch {
	case out == "json":
		o, _ := render(Obj{V: map[string]any{"items": rows}}, Flags{"format": {"json"}})
		return o, nil
	case out == "yaml":
		if len(rows) == 1 {
			o, _ := render(Obj{V: rows[0]}, Flags{"format": {"yaml"}})
			return o, nil
		}
		o, _ := render(Obj{V: map[string]any{"items": rows}}, Flags{"format": {"yaml"}})
		return o, nil
	case strings.HasPrefix(out, "jsonpath="):
		path := strings.Trim(strings.TrimPrefix(out, "jsonpath="), "{}'\"")
		path = strings.TrimPrefix(path, ".")
		path = strings.TrimPrefix(path, "items[*].")
		var vals []string
		for _, r := range rows {
			vals = append(vals, scalar(getPath(toAny(r), path), " "))
		}
		return strings.Join(vals, " "), nil
	}
	if b.Len() > 0 && len(rows) == 0 && kind != "nodes" && kind != "namespaces" && kind != "events" && kind != "services" {
		return "No resources found in " + nsName + " namespace.\n", nil
	}
	return b.String(), nil
}

func (s *Session) kubectlDescribe(p *sim.Project, c *sim.Cluster, nsName, kind, name string, o kubeOpts) (string, error) {
	ns := c.K8s.NS(nsName)
	var b strings.Builder
	switch kind {
	case "pods":
		for _, pd := range s.State.ComputePods(p.ID, c)[nsName] {
			if name != "" && pd.Name != name && pd.Owner != name || !o.matches(pd.Labels) {
				continue
			}
			fmt.Fprintf(&b, "Name:         %s\nNamespace:    %s\nLabels:       %s\nNode:         %s\nStatus:       %s\nIP:           %s\nService Account: %s\nContainers:\n  %s:\n    Image:      %s\n    Ports:      %v\n    Requests:   cpu=%s memory=%s\n    Limits:     cpu=%s memory=%s\n",
				pd.Name, nsName, labelString(pd.Labels), pd.Node, pd.Phase, pd.IP, pd.SA, pd.Spec.Name, pd.Spec.Image, pd.Spec.Ports, pd.Spec.Requests.CPU, pd.Spec.Requests.Memory, pd.Spec.Limits.CPU, pd.Spec.Limits.Memory)
			if pd.Spec.Readiness != nil {
				fmt.Fprintf(&b, "    Readiness:  http-get :%d%s delay=%ds\n", pd.Spec.Readiness.Port, pd.Spec.Readiness.Path, pd.Spec.Readiness.InitialDelay)
			}
			if pd.Spec.Liveness != nil {
				fmt.Fprintf(&b, "    Liveness:   http-get :%d%s delay=%ds\n", pd.Spec.Liveness.Port, pd.Spec.Liveness.Path, pd.Spec.Liveness.InitialDelay)
			}
			fmt.Fprintf(&b, "Conditions:\n  Ready: %v\nEvents:\n", pd.Ready)
			if pd.Reason != "" {
				fmt.Fprintf(&b, "  Warning  %s  %s\n", strings.SplitN(pd.Reason, ":", 2)[0], pd.Reason)
			} else {
				b.WriteString("  Normal   Started  Started container\n")
			}
			b.WriteString("\n")
		}
	case "deployments":
		d := ns.Deployments[name]
		if d == nil {
			return "", fail(1, "Error from server (NotFound): deployments.apps \"%s\" not found", name)
		}
		s.State.ComputePods(p.ID, c)
		ct := d.Template.Containers[0]
		fmt.Fprintf(&b, "Name:               %s\nNamespace:          %s\nSelector:           %v\nReplicas:           %d desired | %d available\nPod Template:\n  Labels:  %v\n  Service Account: %s\n  Containers:\n   %s:\n    Image:  %s\n    Ports:  %v\n    Requests: cpu=%s memory=%s\n    Environment: %v\n", d.Name, nsName, d.Selector, d.Replicas, d.Available, d.Template.Labels, d.Template.ServiceAccount, ct.Name, ct.Image, ct.Ports, ct.Requests.CPU, ct.Requests.Memory, ct.Env)
	case "services":
		sv := ns.Services[name]
		if sv == nil {
			return "", fail(1, "Error from server (NotFound): services \"%s\" not found", name)
		}
		var eps []string
		for _, pd := range s.State.ComputePods(p.ID, c)[nsName] {
			if pd.Ready && selectorMatchesCLI(sv.Selector, pd.Labels) {
				tp := 0
				if len(sv.Ports) > 0 {
					tp = sv.Ports[0].TargetPort
				}
				eps = append(eps, fmt.Sprintf("%s:%d", pd.IP, tp))
			}
		}
		ep := strings.Join(eps, ",")
		if ep == "" {
			ep = "<none>"
		}
		fmt.Fprintf(&b, "Name:              %s\nNamespace:         %s\nSelector:          %v\nType:              %s\nIP:                %s\nLoadBalancer Ingress: %s\nPort:              %v\nEndpoints:         %s\n", sv.Name, nsName, sv.Selector, sv.Type, sv.ClusterIP, sv.ExternalIP, sv.Ports, ep)
	case "hpa":
		h := ns.HPAs[name]
		if h == nil {
			return "", fail(1, "Error from server (NotFound): horizontalpodautoscalers.autoscaling \"%s\" not found", name)
		}
		fmt.Fprintf(&b, "Name:          %s\nReference:     Deployment/%s\nMetrics:       resource cpu on pods (as a percentage of request): %d%% / %d%%\nMin replicas:  %d\nMax replicas:  %d\n", h.Name, h.Target, h.CurrentCPU, h.TargetCPU, h.Min, h.Max)
		if h.Unknown {
			b.WriteString("Conditions:\n  ScalingActive  False  FailedGetResourceMetric  the HPA was unable to compute the replica count: failed to get cpu utilization: missing request for cpu in container\n")
		} else {
			b.WriteString("Conditions:\n  ScalingActive  True   ValidMetricFound\n")
		}
	case "nodes":
		for _, np := range c.NodePools {
			fmt.Fprintf(&b, "Name: gke-%s-%s\nAllocatable: cpu per node for %s\nNodes: %d\n", c.Name, np.Name, np.MachineType, np.Count)
		}
	case "networkpolicies":
		for _, npn := range sim.SortedKeys(ns.NetworkPolicies) {
			np := ns.NetworkPolicies[npn]
			if name != "" && npn != name {
				continue
			}
			from := "<none> (all ingress is denied)"
			if len(np.From) > 0 {
				from = "PodSelector: " + labelString(np.From)
			}
			fmt.Fprintf(&b, "Name:         %s\nNamespace:    %s\nSpec:\n  PodSelector:     %s\n  Allowing ingress traffic:\n    From:\n      %s\n  Policy Types: Ingress\n\n", np.Name, nsName, labelString(np.Selector), from)
		}
	case "configmaps":
		for _, cn := range sim.SortedKeys(ns.ConfigMaps) {
			if name != "" && cn != name {
				continue
			}
			fmt.Fprintf(&b, "Name:         %s\nNamespace:    %s\n\nData\n====\n", cn, nsName)
			for _, k := range sim.SortedKeys(ns.ConfigMaps[cn]) {
				fmt.Fprintf(&b, "%s:\n----\n%s\n\n", k, ns.ConfigMaps[cn][k])
			}
		}
	case "serviceaccounts":
		sa := ns.ServiceAccounts[name]
		if sa == nil {
			return "", fail(1, "Error from server (NotFound): serviceaccounts \"%s\" not found", name)
		}
		fmt.Fprintf(&b, "Name:         %s\nNamespace:    %s\nAnnotations:  %v\n", sa.Name, nsName, sa.Annotations)
	default:
		return "", fail(1, "error: describe is not supported for %s in the simulator", kind)
	}
	if b.Len() == 0 {
		return "", fail(1, "Error from server (NotFound): %s \"%s\" not found", kind, name)
	}
	return b.String(), nil
}

func selectorMatchesCLI(sel, labels map[string]string) bool {
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

func (s *Session) podLogs(p *sim.Project, c *sim.Cluster, ns string, pd *sim.Pod) string {
	if pd.Phase == "Pending" {
		return "Error from server (BadRequest): container \"" + pd.Spec.Name + "\" in pod \"" + pd.Name + "\" is waiting to start: " + pd.Reason + "\n"
	}
	w := s.State.PodWorkload(p.ID, c, ns, pd)
	var lines []string
	lines = append(lines, fmt.Sprintf("%s INFO starting %s", s.State.Now(), pd.Spec.Image))
	if w.Behavior != nil {
		lines = append(lines, fmt.Sprintf("%s INFO listening on :%d", s.State.Now(), w.Behavior.Port))
		paths := append([]string{}, w.Behavior.Paths...)
		sort.Strings(paths)
		for _, path := range paths {
			if st, msg := s.State.Call(w, path); st >= 400 {
				lines = append(lines, fmt.Sprintf("%s ERROR GET %s -> %d: %s", s.State.Now(), path, st, msg))
			}
		}
		if w.Behavior.Mode == "crash" {
			lines = append(lines, "panic: failed to load configuration: exit status 1")
		}
	}
	if pd.Command != "" {
		lines = append(lines, "OK")
	}
	return strings.Join(lines, "\n") + "\n"
}

func (s *Session) podExec(p *sim.Project, c *sim.Cluster, ns string, pd *sim.Pod, cmd []string) (string, error) {
	if len(cmd) == 0 {
		return "", fail(1, "error: you must specify at least one command for the container")
	}
	w := s.State.PodWorkload(p.ID, c, ns, pd)
	line := strings.Join(cmd, " ")
	if (cmd[0] == "sh" || cmd[0] == "/bin/sh" || cmd[0] == "bash") && len(cmd) >= 3 && cmd[1] == "-c" {
		line = strings.Join(cmd[2:], " ")
	}
	toks, _ := s.tokenize(line)
	if len(toks) == 0 {
		return "", nil
	}
	switch toks[0] {
	case "curl", "wget":
		return s.curl(toks, w.Endpoint, w.Principal)
	case "nc":
		return s.nc(toks, w.Endpoint)
	case "nslookup", "dig":
		return s.dig(toks, w.Endpoint)
	case "env", "printenv":
		var b strings.Builder
		for _, k := range sim.SortedKeys(w.Env) {
			b.WriteString(k + "=" + w.Env[k] + "\n")
		}
		return b.String(), nil
	case "gcloud", "gsutil":
		if strings.HasPrefix(w.Principal, "principal://") {
			return "", fail(1, "ERROR: (gcloud) There was a problem refreshing your current auth tokens: the Kubernetes service account is not bound to a Google service account (Workload Identity) — principal %s has no IAM roles", w.Principal)
		}
		sub := NewSession(s.State, p.ID, strings.TrimPrefix(w.Principal, "serviceAccount:"))
		sub.NoTick = true
		return sub.run(toks, "")
	case "cat":
		if len(toks) > 1 && strings.Contains(toks[1], "serviceaccount/token") {
			return "eyJhbGciOiJSUzI1NiIsImtpZCI6InNpbSJ9.k8s\n", nil
		}
	}
	return "", fail(126, "OCI runtime exec failed: exec: \"%s\": executable file not found in $PATH", toks[0])
}
