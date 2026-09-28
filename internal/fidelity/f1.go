package fidelity

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/cli"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

// F1Config points to emulator endpoints (see deploy/docker-compose.yaml).
type F1Config struct {
	PubSubHost    string // e.g. pubsub-emulator:8085 (PUBSUB_EMULATOR_HOST)
	GCSHost       string // e.g. http://fake-gcs:4443 (STORAGE_EMULATOR_HOST)
	Kubeconfig    string // kubeconfig of a kind cluster
	KubectlPath   string
	HTTP          *http.Client
}

// F1ConfigFromEnv reads the standard emulator environment variables.
func F1ConfigFromEnv() F1Config {
	return F1Config{PubSubHost: os.Getenv("PUBSUB_EMULATOR_HOST"), GCSHost: os.Getenv("STORAGE_EMULATOR_HOST"), Kubeconfig: os.Getenv("KIND_KUBECONFIG"), KubectlPath: envOr("KUBECTL", "kubectl")}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// F1Runtime runs labs whose supported services are backed by emulators. The
// simulator remains the control plane (IAM, VPC, grading views); emulator
// results are authoritative for the data plane of the emulated services.
type F1Runtime struct{ Cfg F1Config }

func (r F1Runtime) Level() Level { return F1 }

func (r F1Runtime) client() *http.Client {
	if r.Cfg.HTTP != nil {
		return r.Cfg.HTTP
	}
	return &http.Client{Timeout: 5 * time.Second}
}

// Available checks that at least one emulator answers.
func (r F1Runtime) Available() (bool, string) {
	if r.Cfg.PubSubHost == "" && r.Cfg.GCSHost == "" && r.Cfg.Kubeconfig == "" {
		return false, "no emulator endpoints configured (PUBSUB_EMULATOR_HOST, STORAGE_EMULATOR_HOST, KIND_KUBECONFIG)"
	}
	if r.Cfg.PubSubHost != "" {
		if _, err := r.client().Get(r.psURL("/v1/projects/health/topics")); err != nil {
			return false, "pubsub emulator unreachable: " + err.Error()
		}
	}
	return true, ""
}

func (r F1Runtime) Provision(l *scenario.Lab, seed int64, projectID, userID string) (*Env, error) {
	w, err := scenario.Provision(l, seed, projectID)
	if err != nil {
		return nil, err
	}
	// Replay baseline Pub/Sub topics and buckets into the emulators.
	p := w.State.Projects[projectID]
	if r.Cfg.PubSubHost != "" {
		for t := range p.Topics {
			_, _ = r.ps("PUT", "/v1/projects/"+projectID+"/topics/"+t, nil)
		}
		for n, s := range p.Subs {
			_, _ = r.ps("PUT", "/v1/projects/"+projectID+"/subscriptions/"+n, map[string]any{"topic": "projects/" + projectID + "/topics/" + s.Topic, "ackDeadlineSeconds": s.AckDeadline})
		}
	}
	if r.Cfg.GCSHost != "" {
		for b, bk := range p.Buckets {
			_, _ = r.gcs("POST", "/storage/v1/b?project="+projectID, map[string]any{"name": b}, "")
			for o, obj := range bk.Objects {
				_, _ = r.gcs("POST", "/upload/storage/v1/b/"+b+"/o?uploadType=media&name="+url.QueryEscape(o), nil, obj.Content)
			}
		}
	}
	w.Session.Interceptor = chainInterceptors(r.pubsubInterceptor, r.storageInterceptor, r.kubectlInterceptor)
	return &Env{Level: F1, World: w}, nil
}

// mirror replays the command on the simulator without the interceptor so the
// F0 control plane stays consistent for grading and console views.
func mirror(s *cli.Session, args []string, stdin string) (string, error) {
	ic := s.Interceptor
	s.Interceptor = nil
	defer func() { s.Interceptor = ic }()
	return s.RunArgs(args, stdin)
}

func (r F1Runtime) psURL(path string) string {
	h := r.Cfg.PubSubHost
	if !strings.HasPrefix(h, "http") {
		h = "http://" + h
	}
	return strings.TrimSuffix(h, "/") + path
}

func (r F1Runtime) ps(method, path string, body any) (map[string]any, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, r.psURL(path), rd)
	req.Header.Set("Content-Type", "application/json")
	return doJSON(r.client(), req)
}

func doJSON(c *http.Client, req *http.Request) (map[string]any, error) {
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(b))
		if e, ok := m["error"].(map[string]any); ok {
			msg = fmt.Sprint(e["message"])
		}
		return m, fmt.Errorf("%d %s", resp.StatusCode, msg)
	}
	return m, nil
}

func flagVal(args []string, name string) string {
	for i, a := range args {
		if strings.HasPrefix(a, "--"+name+"=") {
			return strings.TrimPrefix(a, "--"+name+"=")
		}
		if a == "--"+name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func positional(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			if !strings.Contains(a, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") && a != "--auto-ack" && a != "--quiet" {
				i++
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

func short(n string) string { return n[strings.LastIndex(n, "/")+1:] }

func (r F1Runtime) pubsubInterceptor(s *cli.Session, args []string, stdin string) (bool, string, error) {
	if r.Cfg.PubSubHost == "" || len(args) < 4 || args[0] != "gcloud" || args[1] != "pubsub" {
		return false, "", nil
	}
	pos := positional(args[1:])
	if len(pos) < 3 {
		return false, "", nil
	}
	project := flagVal(args, "project")
	if project == "" {
		project = s.Project
	}
	kind, verb, name := pos[1], pos[2], ""
	if len(pos) > 3 {
		name = short(pos[3])
	}
	fail := func(err error) (bool, string, error) {
		return true, "", cli.Fail(1, "ERROR: (gcloud.pubsub.%s.%s) [F1 emulator] %v", kind, verb, err)
	}
	// IAM and permissions are enforced by the F0 control plane first.
	if verb != "list" && verb != "pull" {
		if _, err := mirror(s, args, stdin); err != nil {
			return true, "", err
		}
	}
	switch kind + " " + verb {
	case "topics create":
		if _, err := r.ps("PUT", "/v1/projects/"+project+"/topics/"+name, nil); err != nil {
			return fail(err)
		}
		return true, "Created topic [projects/" + project + "/topics/" + name + "] (pubsub emulator).\n", nil
	case "topics delete":
		if _, err := r.ps("DELETE", "/v1/projects/"+project+"/topics/"+name, nil); err != nil {
			return fail(err)
		}
		return true, "Deleted topic [projects/" + project + "/topics/" + name + "].\n", nil
	case "topics publish":
		msg := flagVal(args, "message")
		res, err := r.ps("POST", "/v1/projects/"+project+"/topics/"+name+":publish", map[string]any{"messages": []any{map[string]any{"data": base64.StdEncoding.EncodeToString([]byte(msg))}}})
		if err != nil {
			return fail(err)
		}
		return true, fmt.Sprintf("messageIds:\n- '%v'\n", firstOf(res["messageIds"])), nil
	case "topics list":
		res, err := r.ps("GET", "/v1/projects/"+project+"/topics", nil)
		if err != nil {
			return fail(err)
		}
		var b strings.Builder
		b.WriteString("NAME\n")
		if l, ok := res["topics"].([]any); ok {
			for _, t := range l {
				b.WriteString(fmt.Sprint(t.(map[string]any)["name"]) + "\n")
			}
		}
		return true, b.String(), nil
	case "subscriptions create":
		body := map[string]any{"topic": "projects/" + project + "/topics/" + short(flagVal(args, "topic")), "ackDeadlineSeconds": 10}
		if pe := flagVal(args, "push-endpoint"); pe != "" {
			body["pushConfig"] = map[string]any{"pushEndpoint": pe}
		}
		if _, err := r.ps("PUT", "/v1/projects/"+project+"/subscriptions/"+name, body); err != nil {
			return fail(err)
		}
		return true, "Created subscription [projects/" + project + "/subscriptions/" + name + "] (pubsub emulator).\n", nil
	case "subscriptions delete":
		if _, err := r.ps("DELETE", "/v1/projects/"+project+"/subscriptions/"+name, nil); err != nil {
			return fail(err)
		}
		return true, "Deleted subscription [" + name + "].\n", nil
	case "subscriptions pull":
		limit := 1
		fmt.Sscanf(flagVal(args, "limit"), "%d", &limit)
		res, err := r.ps("POST", "/v1/projects/"+project+"/subscriptions/"+name+":pull", map[string]any{"maxMessages": limit, "returnImmediately": true})
		if err != nil {
			return fail(err)
		}
		msgs, _ := res["receivedMessages"].([]any)
		if len(msgs) == 0 {
			return true, "Listed 0 items.\n", nil
		}
		var b strings.Builder
		b.WriteString("DATA  MESSAGE_ID  ACK_ID\n")
		var acks []string
		for _, m := range msgs {
			mm := m.(map[string]any)
			msg := mm["message"].(map[string]any)
			data, _ := base64.StdEncoding.DecodeString(fmt.Sprint(msg["data"]))
			b.WriteString(fmt.Sprintf("%s  %v  %v\n", data, msg["messageId"], mm["ackId"]))
			acks = append(acks, fmt.Sprint(mm["ackId"]))
		}
		for _, a := range args {
			if a == "--auto-ack" {
				_, _ = r.ps("POST", "/v1/projects/"+project+"/subscriptions/"+name+":acknowledge", map[string]any{"ackIds": acks})
				_, _ = mirror(s, args, stdin)
			}
		}
		return true, b.String(), nil
	}
	return false, "", nil
}

func firstOf(v any) any {
	if l, ok := v.([]any); ok && len(l) > 0 {
		return l[0]
	}
	return v
}

func (r F1Runtime) gcs(method, path string, body any, raw string) (map[string]any, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else if raw != "" {
		rd = strings.NewReader(raw)
	}
	req, _ := http.NewRequest(method, strings.TrimSuffix(r.Cfg.GCSHost, "/")+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return doJSON(r.client(), req)
}

func (r F1Runtime) gcsRaw(path string) (string, error) {
	resp, err := r.client().Get(strings.TrimSuffix(r.Cfg.GCSHost, "/") + path)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("%d %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return string(b), nil
}

// storageInterceptor routes object data-plane operations to fake-gcs-server.
// Bucket IAM, PAP and lifecycle stay in F0 (fake-gcs does not implement them),
// exactly as the fidelity matrix documents.
func (r F1Runtime) storageInterceptor(s *cli.Session, args []string, stdin string) (bool, string, error) {
	if r.Cfg.GCSHost == "" || len(args) < 2 {
		return false, "", nil
	}
	var verb string
	var pos []string
	switch {
	case args[0] == "gsutil" && len(args) >= 2:
		pos = positional(args[1:])
		if len(pos) == 0 {
			return false, "", nil
		}
		verb = map[string]string{"mb": "create", "cp": "cp", "cat": "cat", "rm": "rm", "ls": "ls"}[pos[0]]
		pos = pos[1:]
	case args[0] == "gcloud" && len(args) >= 3 && args[1] == "storage":
		pos = positional(args[2:])
		if len(pos) == 0 {
			return false, "", nil
		}
		if pos[0] == "buckets" && len(pos) > 1 && pos[1] == "create" {
			verb, pos = "create", pos[2:]
		} else {
			verb, pos = map[string]string{"cp": "cp", "cat": "cat", "rm": "rm", "ls": "ls"}[pos[0]], pos[1:]
		}
	default:
		return false, "", nil
	}
	if verb == "" {
		return false, "", nil
	}
	// Control plane (IAM, PAP, org policy) first.
	out, err := mirror(s, args, stdin)
	if err != nil {
		return true, "", err
	}
	bucketOf := func(u string) (string, string) {
		u = strings.TrimPrefix(u, "gs://")
		b, o, _ := strings.Cut(u, "/")
		return b, o
	}
	switch verb {
	case "create":
		b, _ := bucketOf(pos[0])
		if _, err := r.gcs("POST", "/storage/v1/b?project="+s.Project, map[string]any{"name": b}, ""); err != nil && !strings.Contains(err.Error(), "409") {
			return true, "", cli.Fail(1, "ERROR: [F1 fake-gcs-server] %v", err)
		}
		return true, out, nil
	case "cp":
		if len(pos) < 2 {
			return true, out, nil
		}
		src, dst := pos[0], pos[1]
		if strings.HasPrefix(dst, "gs://") && !strings.HasPrefix(src, "gs://") {
			b, o := bucketOf(dst)
			if o == "" || strings.HasSuffix(o, "/") {
				o += src[strings.LastIndex(src, "/")+1:]
			}
			if _, err := r.gcs("POST", "/upload/storage/v1/b/"+b+"/o?uploadType=media&name="+url.QueryEscape(o), nil, s.Files[strings.TrimPrefix(src, "./")]); err != nil {
				return true, "", cli.Fail(1, "ERROR: [F1 fake-gcs-server] %v", err)
			}
		}
		return true, out, nil
	case "cat":
		b, o := bucketOf(pos[0])
		body, err := r.gcsRaw("/storage/v1/b/" + b + "/o/" + url.PathEscape(o) + "?alt=media")
		if err != nil {
			return true, "", cli.Fail(1, "ERROR: [F1 fake-gcs-server] %v", err)
		}
		return true, body, nil
	case "rm":
		for _, u := range pos {
			b, o := bucketOf(u)
			if o != "" {
				_, _ = r.gcs("DELETE", "/storage/v1/b/"+b+"/o/"+url.PathEscape(o), nil, "")
			}
		}
		return true, out, nil
	case "ls":
		if len(pos) == 0 {
			return true, out, nil
		}
		b, _ := bucketOf(pos[0])
		res, err := r.gcs("GET", "/storage/v1/b/"+b+"/o", nil, "")
		if err != nil {
			return true, "", cli.Fail(1, "ERROR: [F1 fake-gcs-server] %v", err)
		}
		var sb strings.Builder
		if items, ok := res["items"].([]any); ok {
			for _, it := range items {
				sb.WriteString("gs://" + b + "/" + fmt.Sprint(it.(map[string]any)["name"]) + "\n")
			}
		}
		return true, sb.String(), nil
	}
	return false, "", nil
}

// kubectlInterceptor forwards kubectl to a real kind cluster (Kubernetes
// semantics are authoritative) and mirrors mutations into the simulator.
func (r F1Runtime) kubectlInterceptor(s *cli.Session, args []string, stdin string) (bool, string, error) {
	if r.Cfg.Kubeconfig == "" || len(args) == 0 || args[0] != "kubectl" {
		return false, "", nil
	}
	kargs := append([]string{}, args[1:]...)
	for i := 0; i < len(kargs); i++ {
		if (kargs[i] == "-f" || kargs[i] == "--filename") && i+1 < len(kargs) && kargs[i+1] != "-" {
			if content, ok := s.Files[strings.TrimPrefix(kargs[i+1], "./")]; ok {
				stdin = content
				kargs[i+1] = "-"
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.Cfg.KubectlPath, kargs...)
	cmd.Env = append(os.Environ(), "KUBECONFIG="+r.Cfg.Kubeconfig)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	if len(args) > 1 {
		switch args[1] {
		case "apply", "create", "delete", "scale", "set", "autoscale", "rollout", "expose", "annotate", "run":
			_, _ = mirror(s, args, stdin)
		}
	}
	if err != nil {
		return true, "", cli.Fail(1, "%s", strings.TrimSpace(string(out)))
	}
	return true, string(out), nil
}
