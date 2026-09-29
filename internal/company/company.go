// Package company is the persistent company simulation (Blueprint §5, roadmap
// phase 8, "Career Mode"). Isolated labs limit transfer, so every learner owns
// a virtual company — Nebula Corporation — whose organisation, folders and
// projects persist between missions. Decisions have consequences: a risky
// change accepted today (SSH open to the internet, a public bucket, no
// backups, no budget) schedules tomorrow's incident.
package company

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/desk"
	"github.com/neodevesp/gcp-trainer/internal/grader"
	"github.com/neodevesp/gcp-trainer/internal/i18n"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
	"github.com/neodevesp/gcp-trainer/internal/sim"
	"gopkg.in/yaml.v3"
)

// ProjectDef is a company project.
type ProjectDef struct {
	Key      string            `yaml:"key" json:"key"`       // prod-web, prod-data...
	Folder   string            `yaml:"folder" json:"folder"` // Production, Development, Security
	Baseline []string          `yaml:"baseline" json:"-"`    // baseline scripts / builtins
	Labels   map[string]string `yaml:"labels" json:"labels"`
	Purpose  string            `yaml:"purpose" json:"purpose"`
}

// Consequence turns a latent risk into a future incident.
type Consequence struct {
	ID      string         `yaml:"id" json:"id"`
	Policy  string         `yaml:"policy" json:"-"`  // OPA package (gcplab.security, gcplab.reliability, gcplab.cost)
	Deny    string         `yaml:"deny" json:"-"`    // deny id prefix, e.g. OPEN_ADMIN_PORT
	Finding string         `yaml:"finding" json:"-"` // SCC category
	Missing string         `yaml:"missing" json:"-"` // "budget": no budget configured
	Params  map[string]any `yaml:"params" json:"-"`
	// NewOnly ignores risks that already existed when the company was created
	// (the learner is accountable for decisions, not for inherited defaults).
	NewOnly bool   `yaml:"newOnly" json:"-"`
	After   int    `yaml:"after" json:"after"` // days until it materialises
	Mission string `yaml:"mission" json:"mission"`
	Note    string `yaml:"note" json:"note"`
	// EN is the English overlay (content is written in Spanish).
	EN *struct {
		Note string `yaml:"note"`
	} `yaml:"en,omitempty" json:"-"`
}

// NoteIn returns the consequence note in the given language.
func (c Consequence) NoteIn(lang string) string {
	if lang == i18n.EN && c.EN != nil && c.EN.Note != "" {
		return c.EN.Note
	}
	return c.Note
}

// Definition is the company content (content/company/<id>.yaml).
type Definition struct {
	ID           string                   `yaml:"id" json:"id"`
	Name         string                   `yaml:"name" json:"name"`
	Domain       string                   `yaml:"domain" json:"domain"`
	Folders      []string                 `yaml:"folders" json:"folders"`
	Projects     []ProjectDef             `yaml:"projects" json:"projects"`
	Actors       []desk.Actor             `yaml:"actors" json:"actors"`
	Consequences []Consequence            `yaml:"consequences" json:"-"`
	StudentRoles []string                 `yaml:"studentRoles" json:"studentRoles"`
	EN           *DefinitionEN            `yaml:"en,omitempty" json:"-"`
	Missions     map[string]*scenario.Lab `yaml:"-" json:"-"`
	MissionOrder []string                 `yaml:"-" json:"-"`
}

// Load reads content/company/<id>.yaml and content/company/missions.
func Load(dir, id string) (*Definition, error) {
	b, err := os.ReadFile(filepath.Join(dir, id+".yaml"))
	if err != nil {
		return nil, err
	}
	var d Definition
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("%s: %w", id, err)
	}
	labs, err := scenario.LoadAll(filepath.Join(dir, "missions"))
	if err != nil {
		return nil, err
	}
	d.Missions = map[string]*scenario.Lab{}
	for _, l := range labs {
		if l.Company == nil {
			return nil, fmt.Errorf("mission %s: missing company block", l.ID)
		}
		d.Missions[l.ID] = l
		d.MissionOrder = append(d.MissionOrder, l.ID)
	}
	return &d, nil
}

// Validate checks the definition.
func (d *Definition) Validate() []string {
	var p []string
	keys := map[string]bool{}
	for _, pr := range d.Projects {
		keys[pr.Key] = true
	}
	cons := map[string]bool{}
	for _, c := range d.Consequences {
		cons[c.ID] = true
		if d.Missions[c.Mission] == nil {
			p = append(p, fmt.Sprintf("consequence %s: unknown mission %s", c.ID, c.Mission))
		}
	}
	for id, m := range d.Missions {
		if !keys[m.Company.Project] {
			p = append(p, fmt.Sprintf("mission %s: unknown project %s", id, m.Company.Project))
		}
		for _, a := range m.Company.After {
			if d.Missions[a] == nil {
				p = append(p, fmt.Sprintf("mission %s: unknown prerequisite %s", id, a))
			}
		}
		if m.Company.Trigger != "" && !cons[m.Company.Trigger] {
			p = append(p, fmt.Sprintf("mission %s: unknown trigger %s", id, m.Company.Trigger))
		}
	}
	sort.Strings(p)
	return p
}

func (d *Definition) consequence(id string) *Consequence {
	for i := range d.Consequences {
		if d.Consequences[i].ID == id {
			return &d.Consequences[i]
		}
	}
	return nil
}

// Event is a journal entry.
type Event struct {
	Day  int    `json:"day"`
	At   string `json:"at"`
	Kind string `json:"kind"`         // mission, consequence, incident, metric
	Text string `json:"text"`         // Spanish (primary language)
	EN   string `json:"en,omitempty"` // English
}

// Localized returns the event in the given language.
func (e Event) Localized(lang string) Event {
	if lang == i18n.EN && e.EN != "" {
		e.Text = e.EN
	}
	e.EN = ""
	return e
}

// Pending is an incident scheduled by a consequence.
type Pending struct {
	Consequence string `json:"consequence"`
	Resource    string `json:"resource,omitempty"`   // resource that carries the risk
	ProjectKey  string `json:"projectKey,omitempty"` // company project where it was found
	Mission     string `json:"mission"`
	Day         int    `json:"day"` // day it becomes available
	Note        string `json:"note"`
	NoteEN      string `json:"noteEn,omitempty"`
}

// Localized returns the pending incident in the given language.
func (p Pending) Localized(lang string) Pending {
	if lang == i18n.EN && p.NoteEN != "" {
		p.Note = p.NoteEN
	}
	p.NoteEN = ""
	return p
}

// missionTitle returns a mission title in the given language.
func missionTitle(m *scenario.Lab, lang string) string {
	return m.Localized(lang).Title
}

// Metrics are the company health indicators the learner is responsible for.
type Metrics struct {
	Availability float64 `json:"availability"` // % of missions ending with services healthy
	Security     float64 `json:"security"`     // posture score (100 - weighted findings)
	MonthlyCost  float64 `json:"monthlyCostEur"`
	Satisfaction float64 `json:"satisfaction"` // stakeholder satisfaction (communication, SLAs)
}

// Company is a learner's persistent world.
type Company struct {
	ID        string            `json:"id"`
	UserID    string            `json:"userId"`
	Def       string            `json:"definition"`
	Name      string            `json:"name"`
	Day       int               `json:"day"`
	Projects  map[string]string `json:"projects"` // key -> real project id
	Folders   map[string]string `json:"folders"`  // display name -> folder id
	Baseline  map[string]bool   `json:"baseline"` // consequence/resource risks inherited at creation
	State     []byte            `json:"state"`
	Completed map[string]int    `json:"completed"` // mission -> best score
	Pending   []Pending         `json:"pending"`
	Journal   []Event           `json:"journal"`
	Metrics   Metrics           `json:"metrics"`
	Active    string            `json:"activeMission,omitempty"`
	Created   time.Time         `json:"created"`
}

// Params returns template parameters: {{.prod_web}} etc. plus {{.project}}.
func (c *Company) Params() map[string]string {
	out := map[string]string{"company": c.Name}
	for k, v := range c.Projects {
		out[strings.ReplaceAll(k, "-", "_")] = v
	}
	for k, v := range c.Folders {
		out["folder_"+strings.ToLower(k)] = v
	}
	return out
}

// New builds the initial world of a company for a user.
func New(d *Definition, userID string, seed int64, baselineDir string) (*Company, error) {
	suffix := fmt.Sprintf("%04x", uint32(seed)&0xffff)
	c := &Company{ID: "co-" + userID, UserID: userID, Def: d.ID, Name: d.Name, Day: 1, Projects: map[string]string{}, Folders: map[string]string{}, Baseline: map[string]bool{}, Completed: map[string]int{}, Created: time.Now()}
	for _, p := range d.Projects {
		c.Projects[p.Key] = p.Key + "-" + suffix
	}
	first := c.Projects[d.Projects[0].Key]
	st := sim.New(seed, first, "user:"+scenario.AdminAccount)
	st.Clock = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	delete(st.Projects, first) // recreated under its folder below
	st.Folders = map[string]*sim.Folder{}
	st.Org.DisplayName = d.Domain
	st.Org.IAM.AddBinding("roles/owner", "user:"+scenario.AdminAccount, nil)
	folderID := map[string]string{}
	for i, f := range d.Folders {
		id := fmt.Sprintf("%d", 880100+i)
		st.Folders["folders/"+id] = &sim.Folder{ID: id, DisplayName: f, Parent: "organizations/" + st.Org.ID}
		folderID[f] = "folders/" + id
		c.Folders[f] = id
	}
	for _, pd := range d.Projects {
		pid := c.Projects[pd.Key]
		p := st.NewProject(pid, folderID[pd.Folder])
		for k, v := range pd.Labels {
			p.Labels[k] = v
		}
		for _, r := range d.StudentRoles {
			p.IAM.AddBinding(r, "user:student@gcplab.dev", nil)
		}
		for _, b := range pd.Baseline {
			if err := scenario.RunBaseline(st, pid, b, c.Params()); err != nil {
				return nil, fmt.Errorf("project %s: %w", pd.Key, err)
			}
		}
	}
	st.Traffic = nil
	st.Step(5)
	b, err := st.Marshal()
	if err != nil {
		return nil, err
	}
	c.State = b
	for _, cons := range d.Consequences {
		for _, h := range c.risks(st, cons) {
			c.Baseline[cons.ID+"|"+h.key+"|"+h.res] = true
		}
	}
	c.log("company",
		fmt.Sprintf("Te incorporas a %s como becario/a de cloud. %d proyectos en %d carpetas.", d.Name, len(d.Projects), len(d.Folders)),
		fmt.Sprintf("You joined %s as a Cloud Intern. %d projects in %d folders.", d.Name, len(d.Projects), len(d.Folders)))
	c.Metrics = c.measure(st, d)
	return c, nil
}

func (c *Company) log(kind, es, en string) {
	c.Journal = append(c.Journal, Event{Day: c.Day, At: fmt.Sprintf("Day %d", c.Day), Kind: kind, Text: es, EN: en})
	if len(c.Journal) > 200 {
		c.Journal = c.Journal[len(c.Journal)-200:]
	}
}

// MissionStatus describes a mission for the learner.
type MissionStatus struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	Kind      string `json:"kind"`
	Stage     string `json:"stage"`
	Available bool   `json:"available"`
	Done      bool   `json:"done"`
	Score     int    `json:"score,omitempty"`
	Incident  bool   `json:"incident"`
	Why       string `json:"why,omitempty"`
}

// Missions lists missions with availability. careerIndex is the learner's
// career stage index (0 = intern).
func (c *Company) Missions(d *Definition, stageIndex map[string]int, careerIndex int, lang string) []MissionStatus {
	p := func(es, en string) string { return i18n.P(lang, es, en) }
	var out []MissionStatus
	pending := map[string]bool{}
	for _, p := range c.Pending {
		if p.Day <= c.Day {
			pending[p.Mission] = true
		}
	}
	for _, id := range d.MissionOrder {
		m := d.Missions[id].Localized(lang)
		ms := MissionStatus{ID: id, Title: m.Title, Summary: m.Summary, Kind: m.Company.Kind, Stage: m.Company.Stage, Incident: m.Company.Trigger != ""}
		score, done := c.Completed[id]
		ms.Done, ms.Score = done, score
		switch {
		case m.Company.Trigger != "":
			ms.Available = pending[id]
			if !ms.Available {
				if done {
					ms.Why = p("resuelto", "resolved")
				} else {
					ms.Why = p("no está ocurriendo (todavía)", "not happening (yet)")
				}
			}
		case done && !m.Company.Repeatable:
			ms.Why = p("completada", "completed")
		default:
			ms.Available = true
			for _, a := range m.Company.After {
				if _, ok := c.Completed[a]; !ok {
					ms.Available, ms.Why = false, p("requiere ", "requires ")+missionTitle(d.Missions[a], lang)
				}
			}
			if si, ok := stageIndex[m.Company.Stage]; ok && si > careerIndex {
				ms.Available, ms.Why = false, p("requiere la etapa profesional ", "requires career stage ")+m.Company.Stage
			}
		}
		if m.Company.Trigger != "" && !ms.Available {
			continue // incidents stay invisible until they happen
		}
		out = append(out, ms)
	}
	return out
}

// MissionLab builds the lab for a mission on the current world.
func MissionLab(d *Definition, base *scenario.Lab, params map[string]string, state []byte) *scenario.Lab {
	l := *base
	l.Params = map[string][]string{}
	for k, v := range base.Params {
		l.Params[k] = v
	}
	for k, v := range params {
		l.Params[k] = []string{v}
	}
	key := strings.ReplaceAll(base.Company.Project, "-", "_")
	l.FixedProject = params[key]
	l.InitialState = state
	l.Baseline = nil
	if len(l.Actors) == 0 {
		l.Actors = d.Actors
	} else {
		// mission-specific people first, then the company roster
		have := map[string]bool{}
		for _, a := range l.Actors {
			have[a.Role] = true
		}
		for _, a := range d.Actors {
			if !have[a.Role] {
				l.Actors = append(l.Actors, a)
			}
		}
	}
	// the company roster brings its English texts into the mission overlay
	if base.EN != nil && d.EN != nil {
		en := *base.EN
		en.Text = map[string]string{}
		for k, v := range d.EN.Text {
			en.Text[k] = v
		}
		for k, v := range base.EN.Text {
			en.Text[k] = v
		}
		l.EN = &en
	}
	if l.Track == "" {
		l.Track = "company-" + d.ID
	}
	return &l
}

// Start prepares a mission for launch and returns the lab and primary project.
func (c *Company) Start(d *Definition, missionID string, stageIndex map[string]int, careerIndex int) (*scenario.Lab, string, error) {
	var ok bool
	for _, m := range c.Missions(d, stageIndex, careerIndex, i18n.ES) {
		if m.ID == missionID && m.Available {
			ok = true
		}
	}
	if !ok {
		return nil, "", fmt.Errorf("mission %s is not available", missionID)
	}
	base := d.Missions[missionID]
	params := c.Params()
	for _, p := range c.Pending {
		if p.Mission == missionID {
			params["risk"], params["risk_project"] = p.Resource, c.Projects[p.ProjectKey]
		}
	}
	if _, ok := params["risk"]; !ok {
		params["risk"], params["risk_project"] = "", ""
	}
	l := MissionLab(d, base, params, c.State)
	c.Active = missionID
	return l, l.FixedProject, nil
}

// Complete persists the world after a mission, updates metrics and evaluates
// consequences. finalState is the marshalled simulator state.
func (c *Company) Complete(d *Definition, missionID string, res *grader.Result, finalState []byte) ([]Pending, error) {
	st, err := sim.Unmarshal(finalState)
	if err != nil {
		return nil, err
	}
	m := d.Missions[missionID]
	if m == nil {
		return nil, fmt.Errorf("unknown mission %s", missionID)
	}
	if res.Score > c.Completed[missionID] || c.Completed[missionID] == 0 {
		c.Completed[missionID] = res.Score
	}
	es, en := "completada", "completed"
	if !res.Passed {
		es, en = "cerrada sin cumplir los objetivos", "closed without meeting the objectives"
	}
	c.log("mission",
		fmt.Sprintf("%s — %s (puntuación %d).", missionTitle(m, i18n.ES), es, res.Score),
		fmt.Sprintf("%s — %s (score %d).", missionTitle(m, i18n.EN), en, res.Score))
	// resolved incidents leave the pending list
	var keep []Pending
	for _, p := range c.Pending {
		if p.Mission == missionID && res.Passed {
			continue
		}
		keep = append(keep, p)
	}
	c.Pending = keep
	// the day ends: time moves on and background traffic runs
	c.Day++
	st.Clock = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC).AddDate(0, 0, c.Day-1)
	st.Traffic = nil
	// risks fixed before they materialise are cancelled
	var still []Pending
	for _, p := range c.Pending {
		cons := d.consequence(p.Consequence)
		if p.Day > c.Day-1 && cons != nil {
			if hit, _, _, _ := c.risky(st, *cons); !hit {
				c.log("consequence", "Riesgo mitigado a tiempo: "+cons.NoteIn(i18n.ES), "Risk mitigated in time: "+cons.NoteIn(i18n.EN))
				continue
			}
		}
		still = append(still, p)
	}
	c.Pending = still
	// consequences
	var scheduled []Pending
	for _, cons := range d.Consequences {
		if c.hasPending(cons.ID) {
			continue
		}
		hit, where, key, res := c.risky(st, cons)
		if !hit {
			continue
		}
		p := Pending{Consequence: cons.ID, Mission: cons.Mission, Day: c.Day - 1 + max(1, cons.After), Note: cons.NoteIn(i18n.ES) + " (" + where + ")", NoteEN: cons.NoteIn(i18n.EN) + " (" + where + ")", Resource: res, ProjectKey: key}
		c.Pending = append(c.Pending, p)
		scheduled = append(scheduled, p)
	}
	for _, p := range scheduled {
		if p.Day <= c.Day {
			c.log("incident", "Nuevo incidente: "+missionTitle(d.Missions[p.Mission], i18n.ES), "New incident: "+missionTitle(d.Missions[p.Mission], i18n.EN))
		}
	}
	c.Metrics = c.measureWith(st, d, res)
	b, err := st.Marshal()
	if err != nil {
		return nil, err
	}
	c.State, c.Active = b, ""
	return scheduled, nil
}

func (c *Company) hasPending(id string) bool {
	for _, p := range c.Pending {
		if p.Consequence == id {
			return true
		}
	}
	return false
}

type riskHit struct{ where, key, res string }

// risky returns the first risk of a consequence that the learner is
// accountable for.
func (c *Company) risky(st *sim.State, cons Consequence) (bool, string, string, string) {
	for _, h := range c.risks(st, cons) {
		if cons.NewOnly && c.Baseline[cons.ID+"|"+h.key+"|"+h.res] {
			continue
		}
		return true, h.where, h.key, h.res
	}
	return false, "", "", ""
}

// risks evaluates one consequence rule across company projects.
func (c *Company) risks(st *sim.State, cons Consequence) []riskHit {
	var out []riskHit
	keys := make([]string, 0, len(c.Projects))
	for k := range c.Projects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		pid := c.Projects[k]
		p := st.Projects[pid]
		if p == nil {
			continue
		}
		switch {
		case cons.Policy != "":
			msgs, err := grader.EvalPolicy(cons.Policy, map[string]any{"project": st.ProjectView(pid), "projectId": pid, "params": cons.Params})
			if err != nil {
				continue
			}
			for _, m := range msgs {
				if strings.HasPrefix(m, cons.Deny) {
					out = append(out, riskHit{k + ": " + m, k, resourceIn(m)})
				}
			}
		case cons.Finding != "":
			for _, f := range st.Findings(pid) {
				if f.Category == cons.Finding {
					out = append(out, riskHit{k + ": " + f.Resource, k, f.Resource[strings.LastIndex(f.Resource, "/")+1:]})
				}
			}
		case cons.Missing == "budget":
			if len(p.Budgets) == 0 && k == "prod-web" {
				out = append(out, riskHit{k + ": sin presupuesto / no budget", k, ""})
			}
		}
	}
	return out
}

// resourceIn extracts the resource name from a policy message such as
// "OPEN_ADMIN_PORT: firewall allow-ssh exposes ..." or "PUBLIC_BUCKET: bucket x grants".
func resourceIn(msg string) string {
	f := strings.Fields(msg)
	for i := 1; i+1 < len(f); i++ {
		switch f[i] {
		case "firewall", "bucket", "instance", "service", "database", "key", "address", "disk":
			return strings.Trim(f[i+1], ",:")
		}
	}
	for _, x := range f {
		if strings.HasSuffix(x, ":") {
			continue
		}
		return strings.Trim(x, ",")
	}
	return ""
}

func (c *Company) measure(st *sim.State, d *Definition) Metrics {
	return c.measureWith(st, d, nil)
}

func (c *Company) measureWith(st *sim.State, d *Definition, res *grader.Result) Metrics {
	m := c.Metrics
	if m.Availability == 0 {
		m = Metrics{Availability: 100, Security: 100, Satisfaction: 80}
	}
	findings := 0.0
	cost := 0.0
	for _, pid := range c.Projects {
		for _, f := range st.Findings(pid) {
			switch f.Severity {
			case "CRITICAL":
				findings += 25
			case "HIGH":
				findings += 10
			default:
				findings += 3
			}
		}
		_, t := st.CostEstimate(pid)
		cost += t
	}
	m.Security = clamp(100 - findings)
	m.MonthlyCost = float64(int(cost*100)) / 100
	if res != nil {
		ok := 0.0
		if res.Passed {
			ok = 100
		}
		m.Availability = clamp(0.8*m.Availability + 0.2*ok)
		comm := 50.0
		if res.Process != nil {
			for _, f := range res.Process.Factors {
				if f.Name == "communication" {
					comm = f.Score
				}
			}
		}
		m.Satisfaction = clamp(0.7*m.Satisfaction + 0.3*(0.5*float64(res.Score)+0.5*comm))
	}
	return m
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return float64(int(v*10)) / 10
}

// DefinitionEN is the English overlay of a company definition: project
// purposes by key and every people text (name, persona, fallback) keyed by
// its Spanish text.
type DefinitionEN struct {
	Projects map[string]string `yaml:"projects"`
	Text     map[string]string `yaml:"text"`
}

func (d *Definition) tr(s string) string {
	if d.EN != nil {
		if v := d.EN.Text[strings.TrimSpace(s)]; v != "" {
			return v
		}
	}
	return s
}

// Localized returns the definition with its texts in the given language.
func (d *Definition) Localized(lang string) *Definition {
	if lang != i18n.EN || d.EN == nil {
		return d
	}
	c := *d
	c.Projects = append([]ProjectDef{}, d.Projects...)
	for i, p := range c.Projects {
		if v := d.EN.Projects[p.Key]; v != "" {
			c.Projects[i].Purpose = v
		}
	}
	c.Actors = append([]desk.Actor{}, d.Actors...)
	for i, a := range c.Actors {
		c.Actors[i].Name, c.Actors[i].Persona, c.Actors[i].Fallback = d.tr(a.Name), d.tr(a.Persona), d.tr(a.Fallback)
	}
	return &c
}

// TranslationProblems lists company texts without their English overlay.
func (d *Definition) TranslationProblems() []string {
	var p []string
	if d.EN == nil {
		p = append(p, fmt.Sprintf("company %s: missing `en:` translation", d.ID))
	} else {
		for _, pr := range d.Projects {
			if d.EN.Projects[pr.Key] == "" {
				p = append(p, fmt.Sprintf("company %s: project %s purpose not translated", d.ID, pr.Key))
			}
		}
		for _, a := range d.Actors {
			for _, t := range []string{a.Name, a.Persona, a.Fallback} {
				if t != "" && d.EN.Text[strings.TrimSpace(t)] == "" {
					p = append(p, fmt.Sprintf("company %s: %q (%s) not translated", d.ID, t, a.Role))
				}
			}
		}
	}
	for _, c := range d.Consequences {
		if c.Note != "" && (c.EN == nil || c.EN.Note == "") {
			p = append(p, fmt.Sprintf("company %s: consequence %s note not translated", d.ID, c.ID))
		}
	}
	for _, id := range d.MissionOrder {
		p = append(p, d.Missions[id].TranslationProblems()...)
	}
	return p
}
