// Package api is the Learning Plane HTTP API and backend-for-frontend.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/fidelity"
	"github.com/neodevesp/gcp-trainer/internal/grader"
	"github.com/neodevesp/gcp-trainer/internal/learning"
	"github.com/neodevesp/gcp-trainer/internal/mentor"
	"github.com/neodevesp/gcp-trainer/internal/orchestrator"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
	"github.com/neodevesp/gcp-trainer/internal/store"
)

// Server wires the learning plane.
type Server struct {
	Cat      *learning.Catalog
	Engine   *learning.Engine
	Store    store.Store
	Labs     orchestrator.LabPlane
	Tokens   *learning.Tokens
	OIDC     *learning.OIDC
	Pool     *fidelity.Pool
	WebDir   string
	Log      *slog.Logger
	F2Monthly int // max real-cloud sessions per user per month
	mu       sync.Mutex
	rl       map[string][]time.Time
}

// Class is an enterprise classroom (instructor + members + assigned tracks).
type Class struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Instructor string   `json:"instructor"`
	Members    []string `json:"members"`
	Tracks     []string `json:"tracks"`
}

type ctxKey string

func (s *Server) logger() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	h := func(pattern string, fn func(w http.ResponseWriter, r *http.Request) (any, error), roles ...string) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			var u *learning.User
			if !strings.Contains(pattern, "/api/auth/") && !strings.HasSuffix(pattern, "/api/health") && !strings.HasPrefix(pattern, "GET /api/catalog") {
				var err error
				u, err = s.authenticate(r)
				if err != nil {
					writeErr(w, http.StatusUnauthorized, err)
					return
				}
				if len(roles) > 0 && !contains(roles, u.Role) {
					writeErr(w, http.StatusForbidden, fmt.Errorf("requires role %v", roles))
					return
				}
				r = r.WithContext(context.WithValue(r.Context(), ctxKey("user"), u))
			}
			if !s.allow(r) {
				writeErr(w, http.StatusTooManyRequests, fmt.Errorf("rate limit exceeded"))
				return
			}
			v, err := fn(w, r)
			if err != nil {
				code := http.StatusBadRequest
				var he httpErr
				if errors.As(err, &he) {
					code = he.code
				}
				writeErr(w, code, err)
			} else if v != nil {
				writeJSON(w, v)
			}
			s.logger().Info("request", "method", r.Method, "path", r.URL.Path, "ms", time.Since(start).Milliseconds(), "err", err != nil)
		})
	}
	h("GET /api/health", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]any{"ok": true, "labs": len(s.Cat.Labs), "time": time.Now().UTC()}, nil
	})
	// ---- auth -----------------------------------------------------------------
	h("POST /api/auth/register", s.register)
	h("POST /api/auth/login", s.login)
	h("GET /api/auth/oidc/login", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if s.OIDC == nil {
			return nil, httpErr{404, "OIDC not configured"}
		}
		u, err := s.OIDC.AuthURL(learning.NewID("st-"))
		if err != nil {
			return nil, err
		}
		http.Redirect(w, r, u, http.StatusFound)
		return nil, nil
	})
	h("GET /api/auth/oidc/callback", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if s.OIDC == nil {
			return nil, httpErr{404, "OIDC not configured"}
		}
		email, name, err := s.OIDC.Exchange(r.URL.Query().Get("code"))
		if err != nil {
			return nil, err
		}
		u, err := s.userByEmail(email)
		if err != nil {
			u = &learning.User{ID: learning.NewID("u-"), Email: email, Name: name, Role: "student", Created: time.Now(), Provider: "oidc"}
			if err := s.Store.Put("users", u.ID, u); err != nil {
				return nil, err
			}
		}
		http.Redirect(w, r, "/#token="+s.Tokens.Issue(*u), http.StatusFound)
		return nil, nil
	})
	// ---- profile ---------------------------------------------------------------
	h("GET /api/me", func(w http.ResponseWriter, r *http.Request) (any, error) {
		u := userOf(r)
		att, _ := s.attemptsOf(u.ID)
		return s.Engine.Profile(*u, att, s.activeTrack(r)), nil
	})
	h("POST /api/me/leaderboard", func(w http.ResponseWriter, r *http.Request) (any, error) {
		u := userOf(r)
		var req struct{ OptIn bool }
		_ = json.NewDecoder(r.Body).Decode(&req)
		u.Leaderboard = req.OptIn
		return map[string]bool{"optIn": u.Leaderboard}, s.Store.Put("users", u.ID, u)
	})
	h("GET /api/me/attempts", func(w http.ResponseWriter, r *http.Request) (any, error) {
		att, err := s.attemptsOf(userOf(r).ID)
		for i := range att {
			att[i].Result = nil
		}
		sort.Slice(att, func(i, j int) bool { return att[i].Started.After(att[j].Started) })
		return att, err
	})
	h("GET /api/report/{track}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		att, _ := s.attemptsOf(userOf(r).ID)
		return s.Engine.Report(r.PathValue("track"), att), nil
	})
	h("GET /api/leaderboard", func(w http.ResponseWriter, r *http.Request) (any, error) {
		u := userOf(r)
		users, _ := store.ListAs[learning.User](s.Store, "users")
		all, _ := store.ListAs[learning.Attempt](s.Store, "attempts")
		mine, _ := s.attemptsOf(u.ID)
		prof := s.Engine.Profile(*u, mine, "")
		league := r.URL.Query().Get("league")
		if league == "" {
			league = prof.League
		}
		return map[string]any{"league": league, "optIn": u.Leaderboard, "entries": s.Engine.Leaderboard(users, all, league)}, nil
	})
	// ---- catalog --------------------------------------------------------------
	h("GET /api/catalog", func(w http.ResponseWriter, r *http.Request) (any, error) {
		labs := []map[string]any{}
		for _, id := range s.Cat.LabOrder {
			labs = append(labs, labSummary(s.Cat.Labs[id]))
		}
		return map[string]any{"branches": s.Cat.Branches, "tracks": s.Cat.Tracks, "certs": s.Cat.Certs, "badges": s.Cat.Badges, "labs": labs, "levels": learning.Levels}, nil
	})
	h("GET /api/labs/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		l := s.Cat.Labs[r.PathValue("id")]
		if l == nil {
			return nil, httpErr{404, "lab not found"}
		}
		m := labSummary(l)
		m["story"], m["objectives"], m["instructions"], m["constraints"] = l.Story, l.Objectives, l.Instructions, l.Constraints
		var rubric []map[string]any
		for _, it := range l.Rubric {
			rubric = append(rubric, map[string]any{"name": it.Name, "points": it.Points, "validator": it.Validator, "critical": it.Critical})
		}
		m["rubric"] = rubric
		return m, nil
	})
	// ---- lab sessions ---------------------------------------------------------
	h("POST /api/labs/{id}/start", s.start)
	h("GET /api/sessions/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		_, info, err := s.ownSession(r)
		return info, err
	})
	h("POST /api/sessions/{id}/exec", func(w http.ResponseWriter, r *http.Request) (any, error) {
		_, info, err := s.ownSession(r)
		if err != nil {
			return nil, err
		}
		var req struct{ Line string }
		if err := json.NewDecoder(io.LimitReader(r.Body, 128<<10)).Decode(&req); err != nil {
			return nil, err
		}
		res, err := s.Labs.Exec(info.ID, req.Line)
		if errors.Is(err, orchestrator.ErrExpired) {
			return nil, httpErr{410, "session expired"}
		}
		return res, err
	})
	h("GET /api/sessions/{id}/views/{kind}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		_, info, err := s.ownSession(r)
		if err != nil {
			return nil, err
		}
		kind := r.PathValue("kind")
		if kind == "findings" {
			return nil, httpErr{403, "findings are visible through `gcloud scc findings list`"}
		}
		params := map[string]string{"filter": r.URL.Query().Get("filter"), "limit": r.URL.Query().Get("limit")}
		return s.Labs.View(info.ID, kind, params)
	})
	h("PUT /api/sessions/{id}/files", func(w http.ResponseWriter, r *http.Request) (any, error) {
		_, info, err := s.ownSession(r)
		if err != nil {
			return nil, err
		}
		var req struct{ Path, Content string }
		if err := json.NewDecoder(io.LimitReader(r.Body, 512<<10)).Decode(&req); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, s.Labs.PutFile(info.ID, req.Path, req.Content)
	})
	h("POST /api/sessions/{id}/hint", s.hint)
	h("POST /api/sessions/{id}/check", func(w http.ResponseWriter, r *http.Request) (any, error) {
		_, info, err := s.ownSession(r)
		if err != nil {
			return nil, err
		}
		res, err := s.Labs.Grade(info.ID, grader.Submission{})
		if err != nil {
			return nil, err
		}
		// Check work shows progress per criterion without revealing expected values.
		var items []map[string]any
		for _, it := range res.Items {
			if it.Validator == "evidence" || it.Validator == "quiz" {
				continue
			}
			failing := []string{}
			for _, c := range it.Checks {
				if !c.Pass && c.Desc != "" {
					failing = append(failing, c.Desc)
				}
			}
			items = append(items, map[string]any{"name": it.Name, "earned": it.Earned, "points": it.Points, "failing": failing})
		}
		return map[string]any{"items": items, "mentor": mentor.Socratic(s.Cat.Labs[info.LabID], res)}, nil
	})
	h("POST /api/sessions/{id}/submit", s.submit)
	h("POST /api/sessions/{id}/stop", func(w http.ResponseWriter, r *http.Request) (any, error) {
		att, info, err := s.ownSession(r)
		if err != nil {
			return nil, err
		}
		if att != nil && att.Status == "running" {
			att.Status = "abandoned"
			now := time.Now()
			att.Finished = &now
			_ = s.Store.Put("attempts", att.ID, att)
		}
		return map[string]bool{"ok": true}, s.Labs.Stop(info.ID, "abandoned")
	})
	// ---- instructor / admin ----------------------------------------------------
	h("GET /api/admin/kpis", func(w http.ResponseWriter, r *http.Request) (any, error) {
		all, _ := store.ListAs[learning.Attempt](s.Store, "attempts")
		if cid := r.URL.Query().Get("class"); cid != "" {
			var c Class
			if err := s.Store.Get("classes", cid, &c); err != nil {
				return nil, httpErr{404, "class not found"}
			}
			var f []learning.Attempt
			for _, a := range all {
				if contains(c.Members, a.UserID) {
					f = append(f, a)
				}
			}
			all = f
		}
		return s.Engine.ComputeKPIs(all), nil
	}, "instructor", "admin")
	h("POST /api/classes", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var c Class
		_ = json.NewDecoder(r.Body).Decode(&c)
		if c.Name == "" {
			return nil, fmt.Errorf("name required")
		}
		c.ID, c.Instructor = learning.NewID("c-"), userOf(r).ID
		return c, s.Store.Put("classes", c.ID, c)
	}, "instructor", "admin")
	h("GET /api/classes", func(w http.ResponseWriter, r *http.Request) (any, error) {
		cs, err := store.ListAs[Class](s.Store, "classes")
		u := userOf(r)
		var out []Class
		for _, c := range cs {
			if u.Role == "admin" || c.Instructor == u.ID {
				out = append(out, c)
			}
		}
		return out, err
	}, "instructor", "admin")
	h("POST /api/classes/{id}/members", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var c Class
		if err := s.Store.Get("classes", r.PathValue("id"), &c); err != nil {
			return nil, httpErr{404, "class not found"}
		}
		var req struct{ Email string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		u, err := s.userByEmail(req.Email)
		if err != nil {
			return nil, httpErr{404, "user not found"}
		}
		if !contains(c.Members, u.ID) {
			c.Members = append(c.Members, u.ID)
		}
		u.Classes = append(u.Classes, c.ID)
		_ = s.Store.Put("users", u.ID, u)
		return c, s.Store.Put("classes", c.ID, c)
	}, "instructor", "admin")
	h("GET /api/classes/{id}/analytics", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var c Class
		if err := s.Store.Get("classes", r.PathValue("id"), &c); err != nil {
			return nil, httpErr{404, "class not found"}
		}
		type row struct {
			User      string             `json:"user"`
			Email     string             `json:"email"`
			XP        int                `json:"xp"`
			Level     string             `json:"level"`
			Branches  map[string]float64 `json:"branches"`
			Readiness []learning.Readiness `json:"readiness"`
		}
		var rows []row
		team := map[string]float64{}
		for _, id := range c.Members {
			var u learning.User
			if s.Store.Get("users", id, &u) != nil {
				continue
			}
			att, _ := s.attemptsOf(id)
			p := s.Engine.Profile(u, att, "")
			rows = append(rows, row{User: u.Name, Email: u.Email, XP: p.XP, Level: p.Level, Branches: p.Branches, Readiness: p.Readiness})
			for b, v := range p.Branches {
				team[b] += v / float64(len(c.Members))
			}
		}
		return map[string]any{"class": c, "members": rows, "teamBranches": team}, nil
	}, "instructor", "admin")
	h("GET /api/admin/pool", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if s.Pool == nil {
			return map[string]any{"enabled": false}, nil
		}
		return map[string]any{"enabled": true, "stats": s.Pool.Stats(), "projects": s.Pool.List(), "driver": s.Pool.Driver.Name()}, nil
	}, "admin")
	h("POST /api/admin/pool/janitor", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if s.Pool == nil {
			return nil, httpErr{404, "pool disabled"}
		}
		n, errs := s.Pool.Janitor(r.Context())
		msgs := []string{}
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		return map[string]any{"reclaimed": n, "errors": msgs}, nil
	}, "admin")
	h("POST /api/admin/users/{id}/role", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var u learning.User
		if err := s.Store.Get("users", r.PathValue("id"), &u); err != nil {
			return nil, httpErr{404, "user not found"}
		}
		var req struct{ Role string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Role != "student" && req.Role != "instructor" && req.Role != "admin" {
			return nil, fmt.Errorf("invalid role")
		}
		u.Role = req.Role
		return map[string]string{"role": u.Role}, s.Store.Put("users", u.ID, u)
	}, "admin")
	h("GET /api/admin/content", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]any{"labs": len(s.Cat.Labs), "problems": s.Cat.Validate(), "faultTypes": scenario.FaultTypes}, nil
	}, "instructor", "admin")

	// static web console
	if s.WebDir != "" {
		fs := http.FileServer(http.Dir(s.WebDir))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			p := filepath.Join(s.WebDir, filepath.Clean(r.URL.Path))
			if st, err := os.Stat(p); err != nil || st.IsDir() {
				if _, err := os.Stat(p + ".html"); err == nil {
					r.URL.Path += ".html"
				} else if _, err := os.Stat(filepath.Join(p, "index.html")); err != nil {
					r.URL.Path = "/"
				}
			}
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			fs.ServeHTTP(w, r)
		})
	}
	return cors(mux)
}

func cors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := os.Getenv("CORS_ORIGIN"); o != "" {
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
			if r.Method == http.MethodOptions {
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}

type httpErr struct {
	code int
	msg  string
}

func (e httpErr) Error() string { return e.msg }

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func contains(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

func userOf(r *http.Request) *learning.User {
	u, _ := r.Context().Value(ctxKey("user")).(*learning.User)
	return u
}

// allow is a simple per-IP rate limiter (anti-abuse).
func (s *Server) allow(r *http.Request) bool {
	key := strings.Split(r.RemoteAddr, ":")[0]
	if u := userOf(r); u != nil {
		key = u.ID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rl == nil {
		s.rl = map[string][]time.Time{}
	}
	now := time.Now()
	var keep []time.Time
	for _, t := range s.rl[key] {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= 600 {
		s.rl[key] = keep
		return false
	}
	s.rl[key] = append(keep, now)
	return true
}

func (s *Server) authenticate(r *http.Request) (*learning.User, error) {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tok == "" {
		tok = r.URL.Query().Get("token")
	}
	c, err := s.Tokens.Verify(tok)
	if err != nil {
		return nil, fmt.Errorf("unauthenticated: %v", err)
	}
	var u learning.User
	if err := s.Store.Get("users", c.Sub, &u); err != nil {
		return nil, fmt.Errorf("unknown user")
	}
	return &u, nil
}

func (s *Server) userByEmail(email string) (*learning.User, error) {
	users, err := store.ListAs[learning.User](s.Store, "users")
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		if strings.EqualFold(u.Email, email) {
			uu := u
			return &uu, nil
		}
	}
	return nil, store.ErrNotFound
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) (any, error) {
	var req struct{ Email, Name, Password string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !strings.Contains(req.Email, "@") || req.Name == "" {
		return nil, fmt.Errorf("email and name are required")
	}
	if _, err := s.userByEmail(req.Email); err == nil {
		return nil, httpErr{409, "email already registered"}
	}
	hash, err := learning.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}
	users, _ := s.Store.List("users")
	role := "student"
	if len(users) == 0 {
		role = "admin" // first user bootstraps the platform
	}
	u := learning.User{ID: learning.NewID("u-"), Email: req.Email, Name: req.Name, Role: role, PasswordHash: hash, Created: time.Now(), Provider: "local"}
	if err := s.Store.Put("users", u.ID, u); err != nil {
		return nil, err
	}
	u.PasswordHash = ""
	return map[string]any{"token": s.Tokens.Issue(u), "user": u}, nil
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) (any, error) {
	var req struct{ Email, Password string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	u, err := s.userByEmail(strings.TrimSpace(req.Email))
	if err != nil || !learning.CheckPassword(u.PasswordHash, req.Password) {
		return nil, httpErr{401, "invalid credentials"}
	}
	u.PasswordHash = ""
	return map[string]any{"token": s.Tokens.Issue(*u), "user": u}, nil
}

func (s *Server) attemptsOf(userID string) ([]learning.Attempt, error) {
	all, err := store.ListAs[learning.Attempt](s.Store, "attempts")
	if err != nil {
		return nil, err
	}
	var out []learning.Attempt
	for _, a := range all {
		if a.UserID == userID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (s *Server) activeTrack(r *http.Request) string {
	if t := r.URL.Query().Get("track"); t != "" {
		return t
	}
	return "ace-30"
}

func labSummary(l *scenario.Lab) map[string]any {
	return map[string]any{"id": l.ID, "title": l.Title, "summary": l.Summary, "track": l.Track, "day": l.Day, "level": l.Level, "branch": l.Branch, "type": l.Type,
		"skills": l.Skills, "wellArchitected": l.WellArchitected, "certs": l.Certs, "difficulty": l.Difficulty, "minutes": l.Minutes, "fidelity": l.Fidelity.Supported,
		"defaultFidelity": l.Fidelity.Default, "hints": len(l.Hints), "retestOf": l.RetestOf}
}

// ownSession loads the session and verifies ownership.
func (s *Server) ownSession(r *http.Request) (*learning.Attempt, *orchestrator.SessionInfo, error) {
	info, err := s.Labs.Info(r.PathValue("id"))
	if err != nil {
		return nil, nil, httpErr{404, "session not found"}
	}
	u := userOf(r)
	if info.UserID != u.ID && u.Role != "admin" {
		return nil, nil, httpErr{403, "not your session"}
	}
	var a learning.Attempt
	if err := s.Store.Get("attempts", info.AttemptID, &a); err != nil {
		return nil, info, nil
	}
	return &a, info, nil
}

func (s *Server) start(w http.ResponseWriter, r *http.Request) (any, error) {
	u := userOf(r)
	l := s.Cat.Labs[r.PathValue("id")]
	if l == nil {
		return nil, httpErr{404, "lab not found"}
	}
	var req struct {
		Fidelity string `json:"fidelity"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	prior, _ := s.attemptsOf(u.ID)
	running := 0
	for _, a := range prior {
		if a.Status == "running" && time.Since(a.Started) < 3*time.Hour {
			running++
		}
	}
	if running >= 3 {
		return nil, httpErr{429, "you already have 3 running labs; stop one first"}
	}
	if strings.EqualFold(req.Fidelity, "F2") && s.F2Monthly > 0 {
		n := 0
		for _, a := range prior {
			if a.Fidelity == "F2" && a.Started.Month() == time.Now().Month() {
				n++
			}
		}
		if n >= s.F2Monthly {
			return nil, httpErr{429, fmt.Sprintf("monthly real-GCP quota reached (%d sessions)", s.F2Monthly)}
		}
	}
	// each attempt gets a different variant so answers cannot be memorised
	seed := int64(len(prior)*7919+int(time.Now().UnixNano()%9973)) + 1
	att := learning.Attempt{ID: learning.NewID("a-"), UserID: u.ID, LabID: l.ID, Track: l.Track, Seed: seed, Started: time.Now(), Status: "running", HintsUsed: []int{}}
	info, err := s.Labs.Start(orchestrator.StartRequest{UserID: u.ID, LabID: l.ID, Fidelity: req.Fidelity, Seed: seed, AttemptID: att.ID})
	if err != nil {
		att.Status = "error"
		now := time.Now()
		att.Finished = &now
		_ = s.Store.Put("attempts", att.ID, att)
		return nil, err
	}
	att.Fidelity, att.Params, att.ProvisionMs = info.Fidelity, info.Params, info.ProvisionMs
	if err := s.Store.Put("attempts", att.ID, att); err != nil {
		return nil, err
	}
	return info, nil
}

func (s *Server) hint(w http.ResponseWriter, r *http.Request) (any, error) {
	att, info, err := s.ownSession(r)
	if err != nil {
		return nil, err
	}
	if att == nil || att.Status != "running" {
		return nil, fmt.Errorf("attempt not running")
	}
	l := s.Cat.Labs[info.LabID]
	if l.Type == "boss" {
		return nil, httpErr{403, "boss battles have no hints"}
	}
	n := len(att.HintsUsed)
	h, err := s.Labs.Hint(info.ID, n)
	if err != nil {
		return nil, err
	}
	att.HintsUsed = append(att.HintsUsed, n)
	att.HintCost += h.Cost
	if h.Kind == "solution" {
		att.SolutionShown = true
	}
	if err := s.Store.Put("attempts", att.ID, att); err != nil {
		return nil, err
	}
	return map[string]any{"index": n, "text": h.Text, "cost": h.Cost, "kind": h.Kind, "remaining": len(l.Hints) - n - 1, "totalCost": att.HintCost}, nil
}

func (s *Server) submit(w http.ResponseWriter, r *http.Request) (any, error) {
	att, info, err := s.ownSession(r)
	if err != nil {
		return nil, err
	}
	if att == nil || att.Status != "running" {
		return nil, fmt.Errorf("attempt already submitted")
	}
	var sub grader.Submission
	if err := json.NewDecoder(io.LimitReader(r.Body, 256<<10)).Decode(&sub); err != nil {
		return nil, err
	}
	res, err := s.Labs.Grade(info.ID, sub)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	att.Finished, att.Status, att.Result = &now, "submitted", res
	att.Score, att.Passed, att.Critical = res.Score, res.Passed, res.CriticalFailed
	att.Commands, att.Errors = info.Commands, info.Errors
	prior, _ := s.attemptsOf(att.UserID)
	s.Engine.Award(att, prior)
	if err := s.Store.Put("attempts", att.ID, att); err != nil {
		return nil, err
	}
	_ = s.Labs.Stop(info.ID, "submitted")
	u := userOf(r)
	all, _ := s.attemptsOf(u.ID)
	prof := s.Engine.Profile(*u, all, att.Track)
	out := map[string]any{"result": res, "xp": att.XP, "bonus": att.Bonus, "bonusReasons": att.BonusReasons, "profile": prof, "mentor": mentor.Socratic(s.Cat.Labs[info.LabID], res)}
	if mentor.Enabled() && len(sub.Evidence) > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		if review, err := mentor.ReviewPostmortem(ctx, s.Cat.Labs[info.LabID], sub.Evidence); err == nil {
			out["postmortemReview"] = review
		}
	}
	return out, nil
}

// OnExpire marks attempts of expired sessions.
func (s *Server) OnExpire(info orchestrator.SessionInfo) {
	var a learning.Attempt
	if err := s.Store.Get("attempts", info.AttemptID, &a); err == nil && a.Status == "running" {
		now := time.Now()
		a.Status, a.Finished = "expired", &now
		_ = s.Store.Put("attempts", a.ID, a)
	}
}
