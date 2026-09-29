// Package orchestrator is the Lab Plane: it creates, runs, snapshots, grades
// and destroys lab sessions through the fidelity router. It knows nothing
// about users' XP or progress (Learning Plane) — the separation limits the
// blast radius of any vulnerability that manipulates labs.
package orchestrator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/neodevesp/gcp-trainer/internal/company"
	"hash/fnv"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/cli"
	"github.com/neodevesp/gcp-trainer/internal/fidelity"
	"github.com/neodevesp/gcp-trainer/internal/grader"
	"github.com/neodevesp/gcp-trainer/internal/i18n"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
	"github.com/neodevesp/gcp-trainer/internal/sim"
	"github.com/neodevesp/gcp-trainer/internal/store"
)

// StartRequest asks for a new lab session.
type StartRequest struct {
	UserID    string `json:"userId"`
	LabID     string `json:"labId"`
	Fidelity  string `json:"fidelity"`
	Seed      int64  `json:"seed"`
	AttemptID string `json:"attemptId"`
	// Gen asks the lab plane to generate the lab from its failure library.
	Gen *scenario.GenSpec `json:"gen,omitempty"`
	// Mission runs a company-simulation mission on a persisted world.
	Mission *MissionStart `json:"mission,omitempty"`
	// Lang is the learner's language (es primary, en); content and the
	// platform's own terminal commands follow it.
	Lang string `json:"lang,omitempty"`
}

// SessionInfo is the public view of a session.
type SessionInfo struct {
	ID           string                   `json:"id"`
	UserID       string                   `json:"userId"`
	LabID        string                   `json:"labId"`
	AttemptID    string                   `json:"attemptId"`
	Project      string                   `json:"project"`
	Region       string                   `json:"region"`
	Zone         string                   `json:"zone"`
	Fidelity     string                   `json:"fidelity"`
	Reason       string                   `json:"fidelityReason"`
	Seed         int64                    `json:"seed"`
	Params       map[string]string        `json:"params"`
	Started      time.Time                `json:"started"`
	Expires      time.Time                `json:"expires"`
	Status       string                   `json:"status"`
	ProvisionMs  int64                    `json:"provisionMs"`
	Commands     int                      `json:"commands"`
	Errors       int                      `json:"errors"`
	SimTime      string                   `json:"simTime"`
	Title        string                   `json:"title"`
	Story        string                   `json:"story"`
	Instructions string                   `json:"instructions"`
	Objectives   []string                 `json:"objectives"`
	Timeline     []scenario.TimelineEvent `json:"timeline"`
	Constraints  []string                 `json:"constraints"`
	HintCount    int                      `json:"hintCount"`
	Evidence     *scenario.Evidence       `json:"evidence,omitempty"`
	Quiz         []scenario.Question      `json:"quiz,omitempty"`
	Live         map[string]any           `json:"live"`
	Gen          *scenario.GenSpec        `json:"gen,omitempty"`
	Mission      *MissionStart            `json:"mission,omitempty"` // without state
	Mode         string                   `json:"mode,omitempty"`
	Type         string                   `json:"type,omitempty"`
	Lang         string                   `json:"lang"`
}

// LabPlane is implemented by the in-process Service and by the HTTP client
// used when the lab plane runs as a separate, less trusted workload.
type LabPlane interface {
	Start(req StartRequest) (*SessionInfo, error)
	Info(id string) (*SessionInfo, error)
	Exec(id, line string) (cli.Result, error)
	View(id, kind string, params map[string]string) (any, error)
	PutFile(id, path, content string) error
	Grade(id string, sub grader.Submission) (*grader.Result, error)
	Stop(id, reason string) error
	Hint(id string, n int) (*scenario.Hint, error)
	// Export returns the marshalled world (persistent company simulation).
	Export(id string) ([]byte, error)
}

// MissionStart asks the lab plane to run a company mission on a saved world.
type MissionStart struct {
	ID     string            `json:"id"`
	Params map[string]string `json:"params"`
	State  []byte            `json:"state,omitempty"`
}

// Session is a running lab.
type Session struct {
	mu       sync.Mutex
	Info     SessionInfo
	Lab      *scenario.Lab
	Env      *fidelity.Env
	dirty    int
	lastSave time.Time
}

// Service is the in-process lab plane.
type Service struct {
	Labs      map[string]*scenario.Lab
	Router    *fidelity.Router
	Store     store.Store
	TTL       time.Duration
	GraderURL string              // optional remote grader worker
	Lib       *scenario.Library   // failure library for generated incidents
	Company   *company.Definition // company simulation content
	OnExpire  func(info SessionInfo)
	Now       func() time.Time

	mu       sync.Mutex
	sessions map[string]*Session
	extra    map[string]*scenario.Lab // generated labs
	stop     chan struct{}
	stopOnce sync.Once
}

// New creates a lab plane.
func New(labs map[string]*scenario.Lab, router *fidelity.Router, st store.Store) *Service {
	return &Service{Labs: labs, Router: router, Store: st, TTL: 90 * time.Minute, sessions: map[string]*Session{}, stop: make(chan struct{})}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

var ErrNotFound = errors.New("session not found")
var ErrExpired = errors.New("session expired")

func projectID(user, lab string, seed int64) string {
	h := fnv.New32a()
	h.Write([]byte(user + lab))
	return fmt.Sprintf("gcplab-%06x-%d", h.Sum32()&0xffffff, seed%10000)
}

// Register adds a lab created at runtime (incident generator, company sim).
func (s *Service) Register(l *scenario.Lab) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.extra == nil {
		s.extra = map[string]*scenario.Lab{}
	}
	s.extra[l.ID] = l
}

func (s *Service) lab(id string) *scenario.Lab {
	if l := s.Labs[id]; l != nil {
		return l
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.extra[id]
}

// Start provisions a lab.
func (s *Service) Start(req StartRequest) (*SessionInfo, error) {
	l := s.lab(req.LabID)
	if l == nil && req.Gen != nil && s.Lib != nil {
		g, err := s.Lib.Generate(*req.Gen)
		if err != nil {
			return nil, err
		}
		if req.LabID != "" && g.ID != req.LabID {
			return nil, fmt.Errorf("generated lab id mismatch")
		}
		s.Register(g)
		l, req.LabID = g, g.ID
	}
	var mission *MissionStart
	if l == nil && req.Mission != nil && s.Company != nil {
		base := s.Company.Missions[req.Mission.ID]
		if base == nil {
			return nil, fmt.Errorf("mission %s not found", req.Mission.ID)
		}
		l = company.MissionLab(s.Company, base, req.Mission.Params, req.Mission.State)
		mission = &MissionStart{ID: req.Mission.ID, Params: req.Mission.Params}
	}
	if l == nil {
		return nil, fmt.Errorf("lab %s not found", req.LabID)
	}
	lang := i18n.Norm(req.Lang)
	l = l.Localized(lang)
	d, err := s.Router.Choose(l, req.Fidelity, req.UserID)
	if err != nil {
		return nil, err
	}
	seed := req.Seed
	if seed == 0 {
		seed = s.now().UnixNano()%100000 + 1
	}
	proj := projectID(req.UserID, req.LabID, seed)
	if l.FixedProject != "" {
		proj = l.FixedProject
	}
	t0 := time.Now()
	env, err := s.Router.Provision(d, l, seed, proj, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("provision %s at %s: %w", l.ID, d.Level, err)
	}
	w := env.World
	w.Session.Lang = lang
	id := fmt.Sprintf("s-%s-%d", strings.TrimPrefix(proj, "gcplab-"), s.now().UnixNano()%1e6)
	info := SessionInfo{ID: id, UserID: req.UserID, LabID: l.ID, AttemptID: req.AttemptID, Project: w.Project, Region: w.Lab.Region, Zone: w.Lab.Zone,
		Fidelity: string(d.Level), Reason: d.Reason, Seed: seed, Params: w.Params, Started: s.now(), Expires: s.now().Add(s.ttlFor(l)), Status: "running",
		ProvisionMs: time.Since(t0).Milliseconds()}
	info.Mission = mission
	info.Lang = lang
	se := &Session{Info: info, Lab: w.Lab, Env: env}
	s.fillStatic(se)
	s.mu.Lock()
	s.sessions[id] = se
	s.mu.Unlock()
	s.save(se, true)
	out := s.info(se)
	return &out, nil
}

func (s *Service) ttlFor(l *scenario.Lab) time.Duration {
	ttl := s.TTL
	if d := time.Duration(l.Minutes*2) * time.Minute; d > ttl {
		ttl = d
	}
	return ttl
}

func (s *Service) fillStatic(se *Session) {
	l := se.Lab
	se.Info.Title, se.Info.Story, se.Info.Instructions, se.Info.Objectives = l.Title, l.Story, l.Instructions, l.Objectives
	se.Info.Timeline, se.Info.Constraints, se.Info.HintCount = l.Timeline, l.Constraints, len(l.Hints)
	if l.Evidence != nil {
		ev := *l.Evidence
		ev.Keywords, ev.Sample = nil, nil
		se.Info.Evidence = &ev
	}
	se.Info.Quiz = l.Quiz
	se.Info.Gen, se.Info.Mode, se.Info.Type = l.Generated, l.Mode, l.Type
}

func (s *Service) get(id string) (*Session, error) {
	s.mu.Lock()
	se := s.sessions[id]
	s.mu.Unlock()
	if se == nil {
		if restored, err := s.restore(id); err == nil {
			return restored, nil
		}
		return nil, ErrNotFound
	}
	return se, nil
}

// live computes the status panel (health, cost, alerts, time left).
func (s *Service) live(se *Session) map[string]any {
	st := se.Env.World.State
	p := se.Info.Project
	status := map[string]any{}
	for _, t := range st.Traffic {
		n := t.Name
		if n == "" {
			n = t.Target
		}
		v, ok := st.LastMetric("traffic/" + n + "/error_rate")
		status[n] = map[string]any{"ok": ok && v < 0.5, "errorRate": v}
	}
	_, cost := st.CostEstimate(p)
	firing := 0
	if pr := st.Projects[p]; pr != nil {
		for _, a := range pr.AlertPolicies {
			if a.Firing {
				firing++
			}
		}
	}
	left := se.Info.Expires.Sub(s.now())
	if left < 0 {
		left = 0
	}
	return map[string]any{"services": status, "monthlyCostEur": cost, "alertsFiring": firing, "findings": len(st.Findings(p)), "secondsLeft": int(left.Seconds()), "simTime": st.Clock.Format("15:04")}
}

func (s *Service) info(se *Session) SessionInfo {
	i := se.Info
	i.SimTime = se.Env.World.State.Clock.Format(time.RFC3339)
	i.Live = s.live(se)
	return i
}

// Info returns session metadata and live status.
func (s *Service) Info(id string) (*SessionInfo, error) {
	se, err := s.get(id)
	if err != nil {
		return nil, err
	}
	se.mu.Lock()
	defer se.mu.Unlock()
	out := s.info(se)
	return &out, nil
}

// Exec runs a terminal line.
func (s *Service) Exec(id, line string) (cli.Result, error) {
	se, err := s.get(id)
	if err != nil {
		return cli.Result{}, err
	}
	se.mu.Lock()
	defer se.mu.Unlock()
	if se.Info.Status != "running" {
		return cli.Result{}, ErrExpired
	}
	if s.now().After(se.Info.Expires) {
		return cli.Result{}, ErrExpired
	}
	if len(line) > 64*1024 {
		return cli.Result{Output: "input too long\n", Exit: 2}, nil
	}
	r := se.Env.World.Session.Exec(line)
	se.Info.Commands++
	if r.Exit != 0 {
		se.Info.Errors++
	}
	se.dirty++
	s.save(se, false)
	return r, nil
}

// PutFile writes into the session workspace (Files/Editor tab).
func (s *Service) PutFile(id, path, content string) error {
	se, err := s.get(id)
	if err != nil {
		return err
	}
	se.mu.Lock()
	defer se.mu.Unlock()
	if len(content) > 256*1024 || strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
		return fmt.Errorf("invalid file")
	}
	if content == "" {
		delete(se.Env.World.Session.Files, path)
	} else {
		se.Env.World.Session.Files[path] = content
	}
	se.dirty++
	s.save(se, false)
	return nil
}

// Hint returns hint n (0-based); the learning plane charges XP for it.
func (s *Service) Hint(id string, n int) (*scenario.Hint, error) {
	se, err := s.get(id)
	if err != nil {
		return nil, err
	}
	if n < 0 || n >= len(se.Lab.Hints) {
		return nil, fmt.Errorf("no more hints")
	}
	h := se.Lab.Hints[n]
	return &h, nil
}

// View returns the console/telemetry views of the session.
func (s *Service) View(id, kind string, params map[string]string) (any, error) {
	se, err := s.get(id)
	if err != nil {
		return nil, err
	}
	se.mu.Lock()
	defer se.mu.Unlock()
	st := se.Env.World.State
	p := se.Info.Project
	switch kind {
	case "console":
		v := st.ProjectView(p)
		delete(v, "findings")
		if rs, ok := v["redis"].(map[string]any); ok {
			for _, r := range rs {
				if m, ok := r.(map[string]any); ok {
					delete(m, "authString")
				}
			}
		}
		// The console lists database users but, like Cloud SQL, never shows
		// their passwords.
		if sql, ok := v["sqlInstances"].(map[string]any); ok {
			for _, inst := range sql {
				if m, ok := inst.(map[string]any); ok {
					if users, ok := m["users"].(map[string]any); ok {
						for u := range users {
							users[u] = ""
						}
					}
				}
			}
		}
		return v, nil
	case "desk":
		sess := se.Env.World.Session
		if sess.Desk == nil {
			return map[string]any{}, nil
		}
		return sess.Desk, nil
	case "topology":
		nodes, edges := st.Topology(p)
		return map[string]any{"mermaid": st.Mermaid(p), "nodes": nodes, "edges": edges}, nil
	case "logs":
		limit, _ := strconv.Atoi(params["limit"])
		if limit <= 0 || limit > 500 {
			limit = 100
		}
		return st.QueryLogs(p, params["filter"], limit), nil
	case "metrics":
		out := map[string]any{}
		for _, k := range sim.SortedKeys(st.Metrics) {
			if strings.HasPrefix(k, "billing/") && !strings.Contains(k, p) {
				continue
			}
			out[k] = st.Metrics[k]
		}
		return out, nil
	case "files":
		return se.Env.World.Session.Files, nil
	case "iam":
		pr := st.Projects[p]
		sas := []string{}
		for k := range pr.ServiceAccounts {
			sas = append(sas, k)
		}
		sort.Strings(sas)
		return map[string]any{"policy": pr.IAM, "serviceAccounts": sas, "customRoles": pr.CustomRoles}, nil
	case "cost":
		lines, total := st.CostEstimate(p)
		return map[string]any{"monthlyEur": total, "lines": lines}, nil
	case "history":
		return se.Env.World.Session.Records, nil
	case "findings":
		return st.Findings(p), nil
	}
	return nil, fmt.Errorf("unknown view %q", kind)
}

// Grade scores the current state (used for "check work" and final submit).
func (s *Service) Grade(id string, sub grader.Submission) (*grader.Result, error) {
	se, err := s.get(id)
	if err != nil {
		return nil, err
	}
	se.mu.Lock()
	defer se.mu.Unlock()
	if rt, ok := s.Router.Runtimes[fidelity.F2].(*fidelity.F2Runtime); ok && se.Env.Level == fidelity.F2 {
		_ = rt.RefreshMirror(se.Env)
	}
	w := se.Env.World
	if s.GraderURL != "" {
		return RemoteGrade(s.GraderURL, se.Lab.ID, se.Info.Seed, w, sub, se.Info.Mission)
	}
	return grader.Grade(w.Lab, w.State, w.Session, w.Project, sub), nil
}

// Export returns the marshalled simulator state of a session.
func (s *Service) Export(id string) ([]byte, error) {
	se, err := s.get(id)
	if err != nil {
		return nil, err
	}
	se.mu.Lock()
	defer se.mu.Unlock()
	return se.Env.World.State.Marshal()
}

// Stop ends a session and releases real-cloud resources.
func (s *Service) Stop(id, reason string) error {
	se, err := s.get(id)
	if err != nil {
		return err
	}
	se.mu.Lock()
	if se.Info.Status == "running" {
		se.Info.Status = reason
	}
	cleanup := se.Env.Cleanup
	se.Env.Cleanup = nil
	s.save(se, true)
	se.mu.Unlock()
	if cleanup != nil {
		return cleanup()
	}
	return nil
}

// Janitor expires sessions whose TTL passed, independent of the browser.
func (s *Service) Janitor() int {
	s.mu.Lock()
	var expired []*Session
	for _, se := range s.sessions {
		if se.Info.Status == "running" && s.now().After(se.Info.Expires) {
			expired = append(expired, se)
		}
	}
	s.mu.Unlock()
	for _, se := range expired {
		_ = s.Stop(se.Info.ID, "expired")
		if s.OnExpire != nil {
			s.OnExpire(se.Info)
		}
	}
	// drop finished sessions from memory after a grace period
	s.mu.Lock()
	for id, se := range s.sessions {
		if se.Info.Status != "running" && s.now().Sub(se.Info.Expires) > time.Hour {
			delete(s.sessions, id)
		}
	}
	s.mu.Unlock()
	return len(expired)
}

// RunJanitor starts the background TTL loop.
func (s *Service) RunJanitor(every time.Duration) {
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				s.Janitor()
			case <-s.stop:
				return
			}
		}
	}()
}

// Close stops background work and flushes sessions. It is safe to call twice.
func (s *Service) Close() {
	s.stopOnce.Do(func() { close(s.stop) })
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, se := range s.sessions {
		se.mu.Lock()
		s.save(se, true)
		se.mu.Unlock()
	}
}

// ---- persistence ------------------------------------------------------------

type snapshot struct {
	Info    SessionInfo     `json:"info"`
	State   json.RawMessage `json:"state"`
	Session json.RawMessage `json:"session"`
}

func (s *Service) save(se *Session, force bool) {
	if s.Store == nil || se.Env.Level == fidelity.F2 && !force {
		return
	}
	if !force && se.dirty < 5 && time.Since(se.lastSave) < 20*time.Second {
		return
	}
	st, err := se.Env.World.State.Marshal()
	if err != nil {
		return
	}
	sb, _ := json.Marshal(se.Env.World.Session)
	_ = s.Store.Put("labsessions", se.Info.ID, snapshot{Info: se.Info, State: st, Session: sb})
	se.dirty = 0
	se.lastSave = time.Now()
}

func (s *Service) restore(id string) (*Session, error) {
	if s.Store == nil {
		return nil, ErrNotFound
	}
	var snap snapshot
	if err := s.Store.Get("labsessions", id, &snap); err != nil {
		return nil, err
	}
	base := s.lab(snap.Info.LabID)
	if base == nil && snap.Info.Gen != nil && s.Lib != nil {
		if g, err := s.Lib.Generate(*snap.Info.Gen); err == nil {
			s.Register(g)
			base = g
		}
	}
	if base == nil && snap.Info.Mission != nil && s.Company != nil {
		if m := s.Company.Missions[snap.Info.Mission.ID]; m != nil {
			base = company.MissionLab(s.Company, m, snap.Info.Mission.Params, nil)
		}
	}
	if base == nil {
		return nil, ErrNotFound
	}
	l, params, err := base.Localized(snap.Info.Lang).Variant(snap.Info.Seed, snap.Info.Project)
	if err != nil {
		return nil, err
	}
	st, err := sim.Unmarshal(snap.State)
	if err != nil {
		return nil, err
	}
	sess := cli.NewSession(st, snap.Info.Project, l.Student.Account)
	_ = json.Unmarshal(snap.Session, sess)
	sess.State = st
	sess.Policy = l.Policy
	scenario.AttachInterview(sess, l)
	if sess.Desk != nil {
		sess.Desk.Actors = l.Actors // facts are not persisted with the session
	}
	w := &scenario.World{Lab: l, Params: params, Project: snap.Info.Project, State: st, Session: sess, Seed: snap.Info.Seed}
	se := &Session{Info: snap.Info, Lab: l, Env: &fidelity.Env{Level: fidelity.Level(snap.Info.Fidelity), World: w}}
	s.fillStatic(se)
	s.mu.Lock()
	s.sessions[id] = se
	s.mu.Unlock()
	return se, nil
}

// ---- remote grader -------------------------------------------------------------

// GradeRequest is sent to the grader worker.
type GradeRequest struct {
	LabID      string            `json:"labId"`
	Seed       int64             `json:"seed"`
	Project    string            `json:"project"`
	State      json.RawMessage   `json:"state"`
	Session    json.RawMessage   `json:"session"`
	Submission grader.Submission `json:"submission"`
	// Gen identifies a generated incident; the worker regenerates it from its
	// own failure library instead of trusting a lab definition from the lab plane.
	Gen *scenario.GenSpec `json:"gen,omitempty"`
	// Mission identifies a company mission (rebuilt from the worker's content).
	Mission *MissionStart `json:"mission,omitempty"`
}

// RemoteGrade calls an isolated grader worker (it executes probes against
// environments students could have tampered with, so it runs separately
// from the web application).
func RemoteGrade(url, labID string, seed int64, w *scenario.World, sub grader.Submission, mission ...*MissionStart) (*grader.Result, error) {
	st, _ := w.State.Marshal()
	sb, _ := json.Marshal(w.Session)
	req := GradeRequest{LabID: labID, Seed: seed, Project: w.Project, State: st, Session: sb, Submission: sub, Gen: w.Lab.Generated}
	if len(mission) > 0 {
		req.Mission = mission[0]
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(strings.TrimSuffix(url, "/")+"/grade", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("grader worker: %d %s", resp.StatusCode, b)
	}
	var r grader.Result
	return &r, json.Unmarshal(b, &r)
}

// HandleGrade is the grader worker's HTTP handler.
// lib may be nil when the worker has no failure library.
func HandleGrade(labs map[string]*scenario.Lab, lib *scenario.Library, co *company.Definition) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req GradeRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		base := labs[req.LabID]
		if base == nil && req.Gen != nil && lib != nil {
			if g, err := lib.Generate(*req.Gen); err == nil && g.ID == req.LabID {
				base = g
			}
		}
		if base == nil && req.Mission != nil && co != nil {
			if m := co.Missions[req.Mission.ID]; m != nil && m.ID == req.LabID {
				base = company.MissionLab(co, m, req.Mission.Params, nil)
			}
		}
		if base == nil {
			http.Error(w, "unknown lab", 404)
			return
		}
		// feedback and check descriptions in the learner's language
		l, _, err := base.Localized(req.Submission.Lang).Variant(req.Seed, req.Project)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		st, err := sim.Unmarshal(req.State)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		sess := cli.NewSession(st, req.Project, l.Student.Account)
		_ = json.Unmarshal(req.Session, sess)
		sess.State = st
		res := grader.Grade(l, st, sess, req.Project, req.Submission)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	}
}
