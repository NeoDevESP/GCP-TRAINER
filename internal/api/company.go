package api

import (
	"github.com/neodevesp/gcp-trainer/internal/i18n"
	"net/http"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/company"
	"github.com/neodevesp/gcp-trainer/internal/grader"
	"github.com/neodevesp/gcp-trainer/internal/learning"
	"github.com/neodevesp/gcp-trainer/internal/orchestrator"
)

// Career Mode endpoints: the learner's persistent company.

func (s *Server) stageIndex() map[string]int {
	out := map[string]int{}
	for i, st := range s.Cat.Career.Stages {
		out[st.ID] = i
	}
	return out
}

func (s *Server) careerIndex(u *learning.User) int {
	att, _ := s.attemptsOf(u.ID)
	return s.Engine.ForLang(u.Lang).Profile(*u, att, "").Career.StageIndex
}

func (s *Server) loadCompany(userID string) (*company.Company, error) {
	var c company.Company
	if err := s.Store.Get("companies", userID, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Server) companyView(u *learning.User, c *company.Company, lang string) map[string]any {
	var visible []company.Pending
	for _, p := range c.Pending {
		if p.Day <= c.Day {
			visible = append(visible, p.Localized(lang))
		}
	}
	d := s.Company.Localized(lang)
	return map[string]any{
		"name": c.Name, "day": c.Day, "projects": c.Projects, "folders": c.Folders, "metrics": c.Metrics,
		"journal": journalIn(c.Journal, lang), "incidents": visible, "active": c.Active,
		"missions": c.Missions(s.Company, s.stageIndex(), s.careerIndex(u), lang),
		"people":   d.Actors,
	}
}

func journalIn(j []company.Event, lang string) []company.Event {
	out := make([]company.Event, len(j))
	for i, e := range j {
		out[i] = e.Localized(lang)
	}
	return out
}

func (s *Server) noCompany(r *http.Request) error {
	return httpErr{404, i18n.P(s.lang(r), "la simulación de empresa no está disponible", "company simulation not available")}
}

func (s *Server) companyStatus(w http.ResponseWriter, r *http.Request) (any, error) {
	if s.Company == nil {
		return nil, s.noCompany(r)
	}
	u := userOf(r)
	c, err := s.loadCompany(u.ID)
	if err != nil {
		return map[string]any{"exists": false, "name": s.Company.Name, "projects": s.Company.Localized(s.lang(r)).Projects, "folders": s.Company.Folders}, nil
	}
	v := s.companyView(u, c, s.lang(r))
	v["exists"] = true
	return v, nil
}

func (s *Server) companyCreate(w http.ResponseWriter, r *http.Request) (any, error) {
	if s.Company == nil {
		return nil, s.noCompany(r)
	}
	u := userOf(r)
	if _, err := s.loadCompany(u.ID); err == nil && r.URL.Query().Get("reset") != "1" {
		return nil, httpErr{409, i18n.P(s.lang(r), "la empresa ya existe (usa ?reset=1 para empezar de nuevo)", "company already exists (use ?reset=1 to start over)")}
	}
	c, err := company.New(s.Company, u.ID, time.Now().UnixNano()%1000000+1, "")
	if err != nil {
		return nil, err
	}
	if err := s.Store.Put("companies", u.ID, c); err != nil {
		return nil, err
	}
	return s.companyView(u, c, s.lang(r)), nil
}

func (s *Server) companyStart(w http.ResponseWriter, r *http.Request) (any, error) {
	if s.Company == nil {
		return nil, s.noCompany(r)
	}
	u := userOf(r)
	c, err := s.loadCompany(u.ID)
	if err != nil {
		return nil, httpErr{404, i18n.P(s.lang(r), "crea primero tu empresa", "create your company first")}
	}
	id := r.PathValue("id")
	l, _, err := c.Start(s.Company, id, s.stageIndex(), s.careerIndex(u))
	if err != nil {
		return nil, err
	}
	s.Cat.AddGenerated(l)
	params := map[string]string{}
	for k, v := range l.Params {
		if len(v) == 1 {
			params[k] = v[0]
		}
	}
	att := learning.Attempt{ID: learning.NewID("a-"), UserID: u.ID, LabID: l.ID, Track: l.Track, Seed: int64(c.Day), Started: time.Now(), Status: "running", HintsUsed: []int{}}
	info, err := s.Labs.Start(orchestrator.StartRequest{UserID: u.ID, LabID: l.ID, Fidelity: "F0", Seed: int64(c.Day), AttemptID: att.ID, Mission: &orchestrator.MissionStart{ID: id, Params: params, State: c.State}, Lang: s.lang(r)})
	if err != nil {
		return nil, err
	}
	att.Fidelity, att.Params, att.ProvisionMs, att.SessionID = info.Fidelity, info.Params, info.ProvisionMs, info.ID
	if err := s.Store.Put("attempts", att.ID, att); err != nil {
		return nil, err
	}
	if err := s.Store.Put("companies", u.ID, c); err != nil {
		return nil, err
	}
	return info, nil
}

// companyComplete persists the world after a mission and evaluates consequences.
func (s *Server) companyComplete(userID, missionID, sessionID string, res *grader.Result, lang string) map[string]any {
	c, err := s.loadCompany(userID)
	if err != nil {
		return nil
	}
	state, err := s.Labs.Export(sessionID)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	scheduled, err := c.Complete(s.Company, missionID, res, state)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	_ = s.Store.Put("companies", userID, c)
	// scheduled consequences stay secret until they happen; only the count leaks as "risk".
	return map[string]any{"day": c.Day, "metrics": c.Metrics, "journal": journalIn(c.Journal[max(0, len(c.Journal)-3):], lang), "latentRisks": len(scheduled)}
}
