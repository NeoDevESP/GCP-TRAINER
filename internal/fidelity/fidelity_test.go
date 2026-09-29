package fidelity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

func testLab(supported ...string) *scenario.Lab {
	return &scenario.Lab{
		ID:       "t-lab",
		Title:    "test",
		Fidelity: scenario.Fidelity{Default: supported[0], Supported: supported, F2: &scenario.F2Spec{TTLMinutes: 30, StudentRoles: []string{"roles/storage.admin"}}},
	}
}

func TestRouterChoosesAndFallsBack(t *testing.T) {
	r := NewRouter()
	d, err := r.Choose(testLab("F0", "F2"), "", "u1")
	if err != nil || d.Level != F0 {
		t.Fatalf("default: %v %v", d, err)
	}
	if _, err := r.Choose(testLab("F0"), "F2", "u1"); err == nil {
		t.Fatal("expected error for unsupported fidelity")
	}
	// F2 requested but no runtime configured: fall back to F0 with a reason.
	d, err = r.Choose(testLab("F0", "F2"), "f2", "u1")
	if err != nil || d.Level != F0 || !strings.Contains(d.Reason, "fell back") {
		t.Fatalf("fallback: %+v %v", d, err)
	}
	// F2-only lab with no F2 runtime cannot fall back.
	if _, err := r.Choose(testLab("F2"), "F2", "u1"); err == nil {
		t.Fatal("expected error when no fallback is possible")
	}
	// Quota exhausted.
	pool := NewPool(NewSimDriver(), []string{"p1"})
	r = NewRouter(&F2Runtime{Pool: pool})
	r.F2Quota = func(string) (bool, string) { return false, "monthly quota reached" }
	d, err = r.Choose(testLab("F0", "F2"), "F2", "u1")
	if err != nil || d.Level != F0 || !strings.Contains(d.Reason, "quota") {
		t.Fatalf("quota fallback: %+v %v", d, err)
	}
	r.F2Quota = nil
	d, err = r.Choose(testLab("F0", "F2"), "F2", "u1")
	if err != nil || d.Level != F2 {
		t.Fatalf("F2: %+v %v", d, err)
	}
}

func TestPoolLifecycleWithSimDriver(t *testing.T) {
	drv := NewSimDriver()
	pool := NewPool(drv, []string{"sbx-a", "sbx-b"})
	rt := &F2Runtime{Pool: pool, MemberFor: func(u string) string { return "user:lab-" + u + "@gcplab.dev" }}
	l := testLab("F2")

	env, err := rt.Provision(l, 1, "", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if env.Lease.ProjectID != "sbx-a" || pool.Projects["sbx-a"].State != StateActive {
		t.Fatalf("lease %+v state %s", env.Lease, pool.Projects["sbx-a"].State)
	}
	// the lease grants a time-bound conditional binding, never a standing one
	b := drv.Worlds["sbx-a"].Projects["sbx-a"].IAM.Bindings
	found := false
	for _, x := range b {
		for _, m := range x.Members {
			if m == "user:lab-u1@gcplab.dev" && x.Role == "roles/storage.admin" {
				found = x.Condition != nil && strings.Contains(x.Condition.Expression, "request.time")
			}
		}
	}
	if !found {
		t.Fatalf("expected conditional lab binding, got %+v", b)
	}
	// learner commands run in the leased project through the driver
	if _, err := env.World.Session.RunArgs([]string{"gcloud", "storage", "buckets", "create", "gs://sbx-a-data", "--location=europe-west1"}, ""); err != nil {
		t.Fatal(err)
	}
	if left, _ := drv.ListResources(context.Background(), "sbx-a"); len(left) != 1 {
		t.Fatalf("expected the bucket in the real project, got %v", left)
	}
	// grading mirror refreshes from the real project
	if err := rt.RefreshMirror(env); err != nil {
		t.Fatal(err)
	}
	if _, ok := env.World.State.Projects["sbx-a"].Buckets["sbx-a-data"]; !ok {
		t.Fatal("mirror does not contain the bucket")
	}
	// second learner gets the other project; third finds the pool exhausted
	if _, err := rt.Provision(l, 1, "", "u2"); err != nil {
		t.Fatal(err)
	}
	if ok, why := rt.Available(); ok || !strings.Contains(why, "exhausted") {
		t.Fatalf("available=%v %s", ok, why)
	}
	if _, err := pool.Acquire(context.Background(), l, "u3", "user:x"); !errors.Is(err, ErrPoolExhausted) {
		t.Fatalf("expected ErrPoolExhausted, got %v", err)
	}
	// release cleans, validates and returns the project to READY
	if err := env.Cleanup(); err != nil {
		t.Fatal(err)
	}
	pa := pool.Projects["sbx-a"]
	if pa.State != StateReady || pa.Recycled != 1 || len(pa.Orphans) != 0 {
		t.Fatalf("after release: %+v", pa)
	}
	for _, x := range drv.Worlds["sbx-a"].Projects["sbx-a"].IAM.Bindings {
		for _, m := range x.Members {
			if m == "user:lab-u1@gcplab.dev" {
				t.Fatal("access not revoked")
			}
		}
	}
	st := pool.Stats()
	if st[StateReady] != 1 || st[StateActive] != 1 || st["total"] != 2 {
		t.Fatalf("stats %v", st)
	}
}

// flakyDriver fails DestroyAll once, leaving orphans behind.
type flakyDriver struct {
	*SimDriver
	mu    sync.Mutex
	fails int
}

func (f *flakyDriver) DestroyAll(ctx context.Context, project string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fails > 0 {
		f.fails--
		return errors.New("quota error deleting instance")
	}
	return f.SimDriver.DestroyAll(ctx, project)
}

func TestJanitorReclaimsExpiredLeasesAndRecoversQuarantine(t *testing.T) {
	drv := &flakyDriver{SimDriver: NewSimDriver(), fails: 1}
	pool := NewPool(drv, []string{"sbx-a"})
	now := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	pool.Now = func() time.Time { return now }
	l := testLab("F2")
	lease, err := pool.Acquire(context.Background(), l, "u1", "user:lab-u1@gcplab.dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := drv.Exec(context.Background(), "sbx-a", "user:"+scenario.AdminAccount, []string{"gcloud", "storage", "buckets", "create", "gs://leftover"}, ""); err != nil {
		t.Fatal(err)
	}
	// before the TTL nothing happens
	if n, _ := pool.Janitor(context.Background()); n != 0 {
		t.Fatalf("reclaimed %d before expiry", n)
	}
	now = lease.Expires.Add(time.Minute)
	n, errs := pool.Janitor(context.Background())
	if n != 1 || len(errs) != 1 {
		t.Fatalf("janitor n=%d errs=%v", n, errs)
	}
	pp := pool.Projects["sbx-a"]
	if pp.State != StateQuarantine || len(pp.Orphans) == 0 {
		t.Fatalf("expected quarantine with orphans, got %+v", pp)
	}
	if ok, _ := (&F2Runtime{Pool: pool}).Available(); ok {
		t.Fatal("quarantined project must not be leased")
	}
	// next janitor pass retries the cleanup and recovers the project
	if _, errs := pool.Janitor(context.Background()); len(errs) != 0 {
		t.Fatal(errs)
	}
	if pp.State != StateReady || pp.Orphans != nil {
		t.Fatalf("expected recovery, got %+v", pp)
	}
	var trail []string
	for _, e := range pool.Events {
		trail = append(trail, e.To)
	}
	want := "LEASED BASELINE ACTIVE REVOKING CLEANING VALIDATING QUARANTINE READY"
	if strings.Join(trail, " ") != want {
		t.Fatalf("audit trail %v", trail)
	}
}

// fakePubSub is a minimal Pub/Sub emulator REST surface.
type fakePubSub struct {
	mu     sync.Mutex
	topics map[string]bool
	subs   map[string]string   // sub -> topic
	queue  map[string][]string // sub -> base64 data
	calls  []string
}

func newFakePubSub() *fakePubSub {
	return &fakePubSub{topics: map[string]bool{}, subs: map[string]string{}, queue: map[string][]string{}}
}

func (f *fakePubSub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	p := r.URL.Path
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	reply := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	switch {
	case r.Method == "GET" && strings.HasSuffix(p, "/topics"):
		var ts []any
		for t := range f.topics {
			ts = append(ts, map[string]any{"name": t})
		}
		reply(map[string]any{"topics": ts})
	case r.Method == "PUT" && strings.Contains(p, "/topics/"):
		if f.topics[p] {
			w.WriteHeader(409)
			reply(map[string]any{"error": map[string]any{"message": "Topic already exists"}})
			return
		}
		f.topics[p] = true
		reply(map[string]any{"name": p})
	case r.Method == "POST" && strings.HasSuffix(p, ":publish"):
		topic := strings.TrimSuffix(p, ":publish")
		msgs, _ := body["messages"].([]any)
		for s, t := range f.subs {
			if "/v1/"+t == topic {
				for _, m := range msgs {
					f.queue[s] = append(f.queue[s], m.(map[string]any)["data"].(string))
				}
			}
		}
		reply(map[string]any{"messageIds": []any{"1"}})
	case r.Method == "PUT" && strings.Contains(p, "/subscriptions/"):
		f.subs[p] = body["topic"].(string)
		reply(map[string]any{"name": p})
	case r.Method == "POST" && strings.HasSuffix(p, ":pull"):
		s := strings.TrimSuffix(p, ":pull")
		var rm []any
		for i, d := range f.queue[s] {
			rm = append(rm, map[string]any{"ackId": "a" + string(rune('0'+i)), "message": map[string]any{"data": d, "messageId": "1"}})
		}
		reply(map[string]any{"receivedMessages": rm})
	case r.Method == "POST" && strings.HasSuffix(p, ":acknowledge"):
		delete(f.queue, strings.TrimSuffix(p, ":acknowledge"))
		reply(map[string]any{})
	default:
		w.WriteHeader(404)
	}
}

func TestF1PubSubEmulator(t *testing.T) {
	if ok, _ := (F1Runtime{}).Available(); ok {
		t.Fatal("F1 without endpoints must be unavailable")
	}
	fake := newFakePubSub()
	srv := httptest.NewServer(fake)
	defer srv.Close()
	rt := F1Runtime{Cfg: F1Config{PubSubHost: strings.TrimPrefix(srv.URL, "http://")}}
	if ok, why := rt.Available(); !ok {
		t.Fatal(why)
	}
	env, err := rt.Provision(testLab("F1"), 1, "f1-proj", "u1")
	if err != nil {
		t.Fatal(err)
	}
	s := env.World.Session
	run := func(line ...string) string {
		t.Helper()
		out, err := s.RunArgs(line, "")
		if err != nil {
			t.Fatalf("%v: %v %s", line, err, out)
		}
		return out
	}
	if out := run("gcloud", "pubsub", "topics", "create", "orders"); !strings.Contains(out, "pubsub emulator") {
		t.Fatalf("not routed to the emulator: %q", out)
	}
	run("gcloud", "pubsub", "subscriptions", "create", "orders-worker", "--topic=orders")
	run("gcloud", "pubsub", "topics", "publish", "orders", "--message=hello-f1")
	out := run("gcloud", "pubsub", "subscriptions", "pull", "orders-worker", "--auto-ack", "--limit=5")
	if !strings.Contains(out, "hello-f1") {
		t.Fatalf("pull output %q", out)
	}
	if out := run("gcloud", "pubsub", "subscriptions", "pull", "orders-worker", "--auto-ack"); !strings.Contains(out, "Listed 0 items") {
		t.Fatalf("message not acknowledged: %q", out)
	}
	// the F0 control plane mirrors the change for grading and console views
	if _, ok := env.World.State.Projects["f1-proj"].Topics["orders"]; !ok {
		t.Fatal("topic not mirrored into the control plane")
	}
	// the emulator is authoritative: a duplicate create surfaces its error
	if _, err := s.RunArgs([]string{"gcloud", "pubsub", "topics", "create", "orders"}, ""); err == nil {
		t.Fatal("expected duplicate topic error")
	}
}
