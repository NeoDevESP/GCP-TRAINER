// Package api is the Learning Plane HTTP API and backend-for-frontend.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/neodevesp/gcp-trainer/internal/company"
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
	"github.com/neodevesp/gcp-trainer/internal/i18n"
	"github.com/neodevesp/gcp-trainer/internal/learning"
	"github.com/neodevesp/gcp-trainer/internal/mentor"
	"github.com/neodevesp/gcp-trainer/internal/orchestrator"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
	"github.com/neodevesp/gcp-trainer/internal/store"
)

// Server wires the learning plane.
type Server struct {
	Cat       *learning.Catalog
	Engine    *learning.Engine
	Store     store.Store
	Labs      orchestrator.LabPlane
	Tokens    *learning.Tokens
	OIDC      *learning.OIDC
	Pool      *fidelity.Pool
	WebDir    string
	Log       *slog.Logger
	F2Monthly int                 // max real-cloud sessions per user per month
	Lib       *scenario.Library   // failure library (incident generator)
	Company   *company.Definition // persistent company simulation (Career Mode)
	mu        sync.Mutex
	studyMu   sync.Mutex
	rl        map[string][]time.Time
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
					code := http.StatusUnauthorized
					var he httpErr
					if errors.As(err, &he) {
						code = he.code // e.g. 403 for a missing CSRF header: the session is fine
					}
					writeErr(w, s.lang(r), code, err)
					return
				}
				if len(roles) > 0 && !contains(roles, u.Role) {
					writeErr(w, s.lang(r), http.StatusForbidden, fmt.Errorf("requires role %v", roles))
					return
				}
				r = r.WithContext(context.WithValue(r.Context(), ctxKey("user"), u))
			}
			if !s.allow(r) {
				writeErr(w, s.lang(r), http.StatusTooManyRequests, fmt.Errorf("rate limit exceeded"))
				return
			}
			v, err := fn(w, r)
			if err != nil {
				code := http.StatusBadRequest
				var he httpErr
				if errors.As(err, &he) {
					code = he.code
				}
				writeErr(w, s.lang(r), code, err)
			} else if v != nil {
				writeJSON(w, v)
			}
			s.logger().Info("request", "method", r.Method, "path", r.URL.Path, "ms", time.Since(start).Milliseconds(), "err", err != nil)
		})
	}
	h("GET /api/health", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]any{"ok": true, "labs": len(s.Cat.Labs), "time": time.Now().UTC(), "invite": os.Getenv("INVITE_CODE") != ""}, nil
	})
	// ---- auth -----------------------------------------------------------------
	h("POST /api/auth/register", s.register)
	h("POST /api/auth/login", s.login)
	h("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) (any, error) {
		s.setSession(w, r, "")
		return map[string]bool{"ok": true}, nil
	})
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
		// the session goes in an HttpOnly cookie, never in the URL
		s.setSession(w, r, s.Tokens.Issue(*u))
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return nil, nil
	})
	// ---- profile ---------------------------------------------------------------
	h("GET /api/me", func(w http.ResponseWriter, r *http.Request) (any, error) {
		u := userOf(r)
		att, _ := s.attemptsOf(u.ID)
		return s.eng(r).Profile(*u, att, s.activeTrack(r)), nil
	})
	h("POST /api/me/leaderboard", func(w http.ResponseWriter, r *http.Request) (any, error) {
		u := userOf(r)
		var req struct{ OptIn bool }
		_ = json.NewDecoder(r.Body).Decode(&req)
		u.Leaderboard = req.OptIn
		return map[string]bool{"optIn": u.Leaderboard}, s.Store.Put("users", u.ID, u)
	})
	h("POST /api/me/lang", func(w http.ResponseWriter, r *http.Request) (any, error) {
		u := userOf(r)
		var req struct{ Lang string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		u.Lang = i18n.Norm(req.Lang)
		return map[string]string{"lang": u.Lang}, s.Store.Put("users", u.ID, u)
	})
	h("GET /api/me/attempts", func(w http.ResponseWriter, r *http.Request) (any, error) {
		att, err := s.attemptsOf(userOf(r).ID)
		for i := range att {
			att[i].Result = nil
		}
		sort.Slice(att, func(i, j int) bool { return att[i].Started.After(att[j].Started) })
		return att, err
	})
	h("GET /api/me/transcript", func(w http.ResponseWriter, r *http.Request) (any, error) {
		u := userOf(r)
		att, _ := s.attemptsOf(u.ID)
		p := s.eng(r).Profile(*u, att, s.activeTrack(r))
		return learning.NewTranscript(p, att, s.Tokens.Secret, time.Now()), nil
	})
	h("POST /api/auth/verify-transcript", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var t learning.Transcript
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&t); err != nil {
			return nil, err
		}
		return map[string]any{"valid": t.Verify(s.Tokens.Secret), "user": t.Name, "careerStage": t.Stage, "issued": t.Issued}, nil
	})
	h("GET /api/catalog/skills/graph", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.cat(r).Graph(), nil
	})
	h("GET /api/catalog/career", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.cat(r).Career, nil
	})
	h("GET /api/report/{track}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		att, _ := s.attemptsOf(userOf(r).ID)
		return s.eng(r).Report(r.PathValue("track"), att), nil
	})
	h("GET /api/leaderboard", func(w http.ResponseWriter, r *http.Request) (any, error) {
		u := userOf(r)
		users, _ := store.ListAs[learning.User](s.Store, "users")
		all, _ := store.ListAs[learning.Attempt](s.Store, "attempts")
		mine, _ := s.attemptsOf(u.ID)
		prof := s.eng(r).Profile(*u, mine, "")
		league := r.URL.Query().Get("league")
		if league == "" {
			league = prof.League
		}
		return map[string]any{"league": league, "optIn": u.Leaderboard, "entries": s.eng(r).Leaderboard(users, all, league)}, nil
	})
	// ---- catalog --------------------------------------------------------------
	h("GET /api/catalog", func(w http.ResponseWriter, r *http.Request) (any, error) {
		cat := s.cat(r)
		labs := []map[string]any{}
		for _, id := range cat.LabOrder {
			labs = append(labs, labSummary(cat.Lab(id)))
		}
		return map[string]any{"branches": cat.Branches, "tracks": cat.Tracks, "certs": cat.Certs, "badges": cat.Badges, "labs": labs, "levels": learning.LevelNames(s.lang(r)), "lang": s.lang(r)}, nil
	})
	s.studyRoutes(h)
	h("GET /api/learn", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]any{"topics": s.Cat.StudyView(s.lang(r)), "exams": s.Cat.ExamView(s.lang(r))}, nil
	})
	h("GET /api/runbooks", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]any{"runbooks": s.cat(r).RunbookView(s.lang(r))}, nil
	})
	h("GET /api/worlds", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.Cat.WorldList(s.lang(r)), nil
	})
	h("GET /api/worlds/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if v := s.Cat.WorldView(r.PathValue("id"), s.lang(r)); v != nil {
			return v, nil
		}
		return nil, httpErr{404, i18n.P(s.lang(r), "empresa no encontrada", "company not found")}
	})
	h("GET /api/labs/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		l := s.cat(r).Lab(r.PathValue("id"))
		if l == nil {
			return nil, httpErr{404, i18n.P(s.lang(r), "laboratorio no encontrado", "lab not found")}
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
		a, info, err := s.ownSession(r)
		if err != nil || a == nil {
			return info, err
		}
		cp := *info
		cp.LearnMode = a.Mode
		return &cp, nil
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
	h("POST /api/sessions/{id}/tutor", s.tutorOn)
	h("GET /api/sessions/{id}/views/{kind}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		att, info, err := s.ownSession(r)
		if err != nil {
			return nil, err
		}
		kind := r.PathValue("kind")
		if kind == "findings" {
			return nil, httpErr{403, "findings are visible through `gcloud scc findings list`"}
		}
		if kind == "tutor" && (att == nil || !att.Tutor) {
			return nil, httpErr{403, "turn on tutor mode first"}
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
	h("GET /api/failures", s.failureLibrary)
	h("GET /api/company", s.companyStatus)
	h("POST /api/company", s.companyCreate)
	h("POST /api/company/missions/{id}/start", s.companyStart)
	h("POST /api/incidents", s.generateIncident)
	h("POST /api/sessions/{id}/check", func(w http.ResponseWriter, r *http.Request) (any, error) {
		_, info, err := s.ownSession(r)
		if err != nil {
			return nil, err
		}
		res, err := s.Labs.Grade(info.ID, grader.Submission{Lang: info.Lang})
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
		return map[string]any{"items": items, "mentor": mentor.Socratic(s.Cat.Lab(info.LabID).Localized(info.Lang), res, info.Lang)}, nil
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
		return s.eng(r).ComputeKPIs(all), nil
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
			User      string               `json:"user"`
			Email     string               `json:"email"`
			XP        int                  `json:"xp"`
			Level     string               `json:"level"`
			Branches  map[string]float64   `json:"branches"`
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
			p := s.eng(r).Profile(u, att, "")
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
			securityHeaders(w, r)
			fs.ServeHTTP(w, r)
		})
	}
	return cors(mux)
}

// CSP for the web app. 'unsafe-inline' scripts are needed by the static
// export (Next.js inlines its page data) and the theme boot script; external
// scripts are limited to the Monaco editor's CDN. Styles inline for Mermaid.
// Videos come from youtube-nocookie. Nothing may frame the app.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; " +
	"style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; " +
	"font-src 'self' data: https://cdn.jsdelivr.net; " +
	"img-src 'self' data: blob: https://i.ytimg.com; " +
	"connect-src 'self'; " +
	"worker-src 'self' blob:; " +
	"frame-src https://www.youtube-nocookie.com https://www.youtube.com; " +
	"frame-ancestors 'none'; base-uri 'self'; form-action 'self'; object-src 'none'"

func securityHeaders(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Security-Policy", contentSecurityPolicy)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin") // YouTube embeds need the origin
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	}
}

func cors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := os.Getenv("CORS_ORIGIN"); o != "" {
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Lang, X-Run-Base")
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
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store") // personal data: never cached
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, lang string, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": localizeError(lang, err.Error())})
}

func contains(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

// lang is the request's language: X-Lang / ?lang=, then the user's saved
// preference, then Accept-Language, then Spanish (the primary language).
func (s *Server) lang(r *http.Request) string {
	pref := ""
	if u := userOf(r); u != nil {
		pref = u.Lang
	}
	return i18n.FromRequest(r, pref)
}

// cat and eng are the catalog and analytics engine in the request's language.
func (s *Server) cat(r *http.Request) *learning.Catalog { return s.Cat.ForLang(s.lang(r)) }
func (s *Server) eng(r *http.Request) *learning.Engine  { return s.Engine.ForLang(s.lang(r)) }

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

// SessionCookie holds the web session. It is HttpOnly (scripts cannot read
// it), SameSite=Strict and Secure over HTTPS. API clients keep using the
// Authorization: Bearer header; the token is never accepted in the URL.
const SessionCookie = "cm_session"

// CSRFHeader must come with every state-changing request authenticated by
// the cookie: a cross-site form or image cannot add a custom header, and a
// cross-site script would need a CORS preflight that the server does not grant.
const CSRFHeader = "X-CM-Client"

func (s *Server) setSession(w http.ResponseWriter, r *http.Request, tok string) {
	c := &http.Cookie{Name: SessionCookie, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")}
	if tok == "" {
		c.MaxAge = -1
	} else {
		c.MaxAge = int(s.Tokens.TTL.Seconds())
	}
	http.SetCookie(w, c)
}

func (s *Server) authenticate(r *http.Request) (*learning.User, error) {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tok == "" {
		if ck, err := r.Cookie(SessionCookie); err == nil {
			tok = ck.Value
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
			default:
				if r.Header.Get(CSRFHeader) == "" {
					return nil, httpErr{403, "missing " + CSRFHeader + " header"}
				}
			}
		}
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
	var req struct{ Email, Name, Password, Invite string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	// INVITE_CODE closes sign-up on a public deployment: only people who
	// know the code can create an account.
	if code := os.Getenv("INVITE_CODE"); code != "" && subtle.ConstantTimeCompare([]byte(strings.TrimSpace(req.Invite)), []byte(code)) != 1 {
		return nil, httpErr{403, i18n.P(s.lang(r), "código de invitación incorrecto", "wrong invitation code")}
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
	tok := s.Tokens.Issue(u)
	s.setSession(w, r, tok)
	return map[string]any{"token": tok, "user": u}, nil
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
	tok := s.Tokens.Issue(*u)
	s.setSession(w, r, tok)
	return map[string]any{"token": tok, "user": u}, nil
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
		"defaultFidelity": l.Fidelity.Default, "hints": len(orchestrator.HintLadder(l, i18n.ES)), "retestOf": l.RetestOf, "ticket": ticketSummary(l)}
}

// ticketSummary is the service-desk framing of a lab, used to build support shifts.
func ticketSummary(l *scenario.Lab) map[string]string {
	if l.Ticket == nil {
		return nil
	}
	return map[string]string{"id": l.Ticket.ID, "kind": l.Ticket.Kind, "priority": l.Ticket.Priority, "sla": l.Ticket.SLA}
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
	l := s.Cat.Lab(r.PathValue("id"))
	if l == nil {
		return nil, httpErr{404, "lab not found"}
	}
	var req struct {
		Fidelity string `json:"fidelity"`
		Mode     string `json:"mode"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	mode := req.Mode
	if mode == "" {
		mode = "assisted"
		if l.Type == "guided" {
			mode = "explorer"
		}
	}
	if !contains(learning.AttemptModes, mode) {
		return nil, httpErr{400, i18n.P(s.lang(r), "modo de aprendizaje desconocido", "unknown learning mode")}
	}
	if l.Type == "boss" && (mode == "explorer" || mode == "assisted") {
		mode = "independent" // boss battles never had help
	}
	return s.startLab(userOf(r), l, req.Fidelity, s.lang(r), mode)
}

// startLab creates an attempt and provisions a session (catalog or generated lab).
func (s *Server) startLab(u *learning.User, l *scenario.Lab, fidelity, lang, mode string) (*orchestrator.SessionInfo, error) {
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
	if strings.EqualFold(fidelity, "F2") && s.F2Monthly > 0 {
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
	if l.Generated != nil {
		seed = l.Generated.Seed
	}
	att := learning.Attempt{ID: learning.NewID("a-"), UserID: u.ID, LabID: l.ID, Track: l.Track, Seed: seed, Started: time.Now(), Status: "running", HintsUsed: []int{}, Mode: mode}
	info, err := s.Labs.Start(orchestrator.StartRequest{UserID: u.ID, LabID: l.ID, Fidelity: fidelity, Seed: seed, AttemptID: att.ID, Gen: l.Generated, Lang: lang})
	if err != nil {
		att.Status = "error"
		now := time.Now()
		att.Finished = &now
		_ = s.Store.Put("attempts", att.ID, att)
		return nil, err
	}
	att.Fidelity, att.Params, att.ProvisionMs, att.SessionID = info.Fidelity, info.Params, info.ProvisionMs, info.ID
	info.LearnMode = mode
	if err := s.Store.Put("attempts", att.ID, att); err != nil {
		return nil, err
	}
	return info, nil
}

// generateIncident builds an incident from the failure library and starts it.
func (s *Server) generateIncident(w http.ResponseWriter, r *http.Request) (any, error) {
	if s.Lib == nil {
		return nil, httpErr{404, "failure library not loaded"}
	}
	var spec scenario.GenSpec
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&spec); err != nil {
		return nil, err
	}
	if spec.Seed == 0 {
		spec.Seed = time.Now().UnixNano()%1000000 + 1
	}
	l, err := s.Lib.Generate(spec)
	if err != nil {
		return nil, err
	}
	s.Cat.AddGenerated(l)
	_ = s.Store.Put("genlabs", l.ID, spec)
	return s.startLab(userOf(r), l, "F0", s.lang(r), "assisted")
}

// LoadGenerated re-registers generated labs persisted in the store (after a restart).
func (s *Server) LoadGenerated() {
	if s.Company != nil {
		for _, m := range s.Company.Missions {
			s.Cat.AddGenerated(m)
		}
	}
	if s.Lib == nil {
		return
	}
	specs, _ := store.ListAs[scenario.GenSpec](s.Store, "genlabs")
	for _, spec := range specs {
		if l, err := s.Lib.Generate(spec); err == nil {
			s.Cat.AddGenerated(l)
		}
	}
}

// failureLibrary exposes the symptom graph and systems (never solutions).
func (s *Server) failureLibrary(w http.ResponseWriter, r *http.Request) (any, error) {
	if s.Lib == nil {
		return nil, httpErr{404, "failure library not loaded"}
	}
	type fv struct {
		ID, Title, Symptom, Layer, Kind string
		Skills                          []string
	}
	type sv struct {
		ID, Title, Topology string
		Failures            []fv
	}
	lib := s.Lib.Localized(s.lang(r))
	var systems []sv
	for _, id := range sortedKeys(lib.Systems) {
		sys := lib.Systems[id]
		v := sv{ID: sys.ID, Title: sys.Title, Topology: sys.Topology}
		for _, f := range sys.Failures {
			v.Failures = append(v.Failures, fv{f.ID, f.Title, f.Symptom, f.Layer, f.Kind, f.Skills})
		}
		systems = append(systems, v)
	}
	return map[string]any{"symptoms": lib.Symptoms, "contexts": lib.Contexts, "systems": systems, "graph": lib.SymptomGraph()}, nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// tutorOn turns on tutor mode: the walkthrough reveals the solution step by
// step, so the attempt counts as guided (like showing the solution).
func (s *Server) tutorOn(w http.ResponseWriter, r *http.Request) (any, error) {
	att, info, err := s.ownSession(r)
	if err != nil {
		return nil, err
	}
	if att == nil || att.Status != "running" {
		return nil, fmt.Errorf("attempt not running")
	}
	if l := s.Cat.Lab(info.LabID); l != nil && l.Type == "boss" {
		return nil, httpErr{403, "boss battles have no tutor"}
	}
	if !learning.ModeAllows(att.Mode, "tutor") {
		return nil, httpErr{403, i18n.P(s.lang(r), "en este modo no hay tutor: se evalúa tu autonomía", "no tutor in this mode: your autonomy is being assessed")}
	}
	if !att.Tutor {
		att.Tutor, att.SolutionShown = true, true
		if err := s.Store.Put("attempts", att.ID, att); err != nil {
			return nil, err
		}
	}
	return map[string]bool{"tutor": true}, nil
}

func (s *Server) hint(w http.ResponseWriter, r *http.Request) (any, error) {
	att, info, err := s.ownSession(r)
	if err != nil {
		return nil, err
	}
	if att == nil || att.Status != "running" {
		return nil, fmt.Errorf("attempt not running")
	}
	l := s.Cat.Lab(info.LabID).Localized(info.Lang)
	if l.Type == "boss" {
		return nil, httpErr{403, "boss battles have no hints"}
	}
	if !learning.ModeAllows(att.Mode, "hint") {
		return nil, httpErr{403, i18n.P(s.lang(r), "en modo independiente no hay pistas: se evalúa tu autonomía", "no hints in independent mode: your autonomy is being assessed")}
	}
	n := len(att.HintsUsed)
	h, err := s.Labs.Hint(info.ID, n)
	if err != nil {
		return nil, err
	}
	if h.Kind == "solution" && !learning.ModeAllows(att.Mode, "solution") {
		return nil, httpErr{403, i18n.P(s.lang(r), "en modo producción no se muestra la solución: escala o sigue investigando", "no solution in production mode: escalate or keep investigating")}
	}
	att.HintsUsed = append(att.HintsUsed, n)
	att.HintCost += h.Cost
	if h.Kind == "solution" {
		att.SolutionShown = true
	}
	if err := s.Store.Put("attempts", att.ID, att); err != nil {
		return nil, err
	}
	total := h.Total
	if total == 0 {
		total = len(l.Hints)
	}
	return map[string]any{"index": n, "text": h.Text, "cost": h.Cost, "kind": h.Kind, "level": h.Level, "name": h.Name, "remaining": total - n - 1, "totalCost": att.HintCost}, nil
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
	sub.HintsUsed = len(att.HintsUsed)
	sub.Lang = info.Lang
	res, err := s.Labs.Grade(info.ID, sub)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	att.Finished, att.Status, att.Result = &now, "submitted", res
	att.Score, att.Passed, att.Critical = res.Score, res.Passed, res.CriticalFailed
	att.Commands, att.Errors = info.Commands, info.Errors
	prior, _ := s.attemptsOf(att.UserID)
	s.Engine.ForLang(info.Lang).Award(att, prior)
	var companyOut map[string]any
	if l := s.Cat.Lab(info.LabID); l != nil && l.Company != nil && s.Company != nil {
		companyOut = s.companyComplete(att.UserID, l.ID, info.ID, res, info.Lang)
	}
	if err := s.Store.Put("attempts", att.ID, att); err != nil {
		return nil, err
	}
	_ = s.Labs.Stop(info.ID, "submitted")
	u := userOf(r)
	all, _ := s.attemptsOf(u.ID)
	prof := s.eng(r).Profile(*u, all, att.Track)
	out := map[string]any{"result": res, "xp": att.XP, "bonus": att.Bonus, "bonusReasons": att.BonusReasons, "profile": prof, "mentor": mentor.Socratic(s.Cat.Lab(info.LabID).Localized(info.Lang), res, info.Lang)}
	out["tutor"] = att.Tutor
	if l := s.Cat.Lab(info.LabID); l != nil {
		out["skills"] = l.Skills
	}
	if companyOut != nil {
		out["company"] = companyOut
	}
	if mentor.Enabled() && len(sub.Evidence) > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		if review, err := mentor.ReviewPostmortem(ctx, s.Cat.Lab(info.LabID).Localized(info.Lang), sub.Evidence, info.Lang); err == nil {
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
