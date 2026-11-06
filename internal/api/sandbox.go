package api

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/i18n"
	"github.com/neodevesp/gcp-trainer/internal/orchestrator"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

// The free console (scenario/sandbox.go): a simulated project with no mission
// and no grading. It creates no attempt, so it never touches progress, scores
// or the plan. Each learner can keep one saved world and the session that is
// running now.

// Sandbox is what is stored per learner.
type Sandbox struct {
	World   string    `json:"world"`
	Session string    `json:"session,omitempty"` // running session, if any
	State   []byte    `json:"state,omitempty"`   // saved world (sim state)
	SavedAt time.Time `json:"savedAt,omitempty"`
}

func (s *Server) sandboxRoutes(h func(string, func(http.ResponseWriter, *http.Request) (any, error), ...string)) {
	h("GET /api/sandbox", func(w http.ResponseWriter, r *http.Request) (any, error) {
		u := userOf(r)
		lang := s.lang(r)
		var worlds []map[string]string
		for _, x := range scenario.SandboxWorlds {
			worlds = append(worlds, map[string]string{"id": x.ID, "name": i18n.P(lang, x.NameES, x.NameEN), "description": i18n.P(lang, x.DescES, x.DescEN)})
		}
		out := map[string]any{"worlds": worlds}
		var sb Sandbox
		if s.Store.Get("sandboxes", u.ID, &sb) == nil {
			if sb.Session != "" {
				if info, err := s.Labs.Info(sb.Session); err == nil && info.Status == "running" && info.UserID == u.ID {
					out["running"] = map[string]string{"session": info.ID, "world": sb.World, "labId": info.LabID}
				}
			}
			if len(sb.State) > 0 {
				out["saved"] = map[string]any{"world": sb.World, "savedAt": sb.SavedAt}
			}
		}
		return out, nil
	})
	h("POST /api/sandbox/start", func(w http.ResponseWriter, r *http.Request) (any, error) {
		u := userOf(r)
		var req struct {
			World  string `json:"world"`
			Resume bool   `json:"resume"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil {
			return nil, err
		}
		var sb Sandbox
		_ = s.Store.Get("sandboxes", u.ID, &sb)
		var state []byte
		if req.Resume {
			if len(sb.State) == 0 {
				return nil, httpErr{404, i18n.P(s.lang(r), "no tienes ningún mundo guardado", "you have no saved world")}
			}
			req.World, state = sb.World, sb.State
		}
		if scenario.SandboxWorldByID(req.World) == nil {
			return nil, httpErr{400, "unknown world"}
		}
		// one free console at a time: the previous one closes
		if sb.Session != "" {
			_ = s.Labs.Stop(sb.Session, "replaced")
		}
		info, err := s.Labs.Start(orchestrator.StartRequest{UserID: u.ID, LabID: scenario.SandboxPrefix + req.World, Fidelity: "F0", Lang: s.lang(r), SandboxState: state})
		if err != nil {
			return nil, err
		}
		sb.World, sb.Session = req.World, info.ID
		if err := s.Store.Put("sandboxes", u.ID, sb); err != nil {
			return nil, err
		}
		return info, nil
	})
	h("POST /api/sandbox/save", func(w http.ResponseWriter, r *http.Request) (any, error) {
		u := userOf(r)
		var sb Sandbox
		if err := s.Store.Get("sandboxes", u.ID, &sb); err != nil || sb.Session == "" {
			return nil, httpErr{404, i18n.P(s.lang(r), "no hay ninguna consola libre abierta", "no free console is open")}
		}
		info, err := s.Labs.Info(sb.Session)
		if err != nil || info.UserID != u.ID {
			return nil, httpErr{404, i18n.P(s.lang(r), "la consola libre ya se cerró", "the free console has closed")}
		}
		state, err := s.Labs.Export(sb.Session)
		if err != nil {
			return nil, err
		}
		sb.State, sb.SavedAt = state, time.Now()
		if err := s.Store.Put("sandboxes", u.ID, sb); err != nil {
			return nil, err
		}
		return map[string]any{"savedAt": sb.SavedAt}, nil
	})
	h("DELETE /api/sandbox/saved", func(w http.ResponseWriter, r *http.Request) (any, error) {
		u := userOf(r)
		var sb Sandbox
		if s.Store.Get("sandboxes", u.ID, &sb) == nil {
			sb.State, sb.SavedAt = nil, time.Time{}
			if err := s.Store.Put("sandboxes", u.ID, sb); err != nil {
				return nil, err
			}
		}
		return map[string]bool{"ok": true}, nil
	})
}
