package scenario

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/desk"
	"github.com/neodevesp/gcp-trainer/internal/i18n"
)

// Incident generator (Blueprint §7-8). A scenario is declarative and
// composable:
//
//	service + configuration + fault + business context + difficulty + noise +
//	constraints + available evidence + validators + scoring
//
// Generate turns a GenSpec into an ordinary Lab, so generated incidents use
// the same provisioning, grading and content CI as hand-written labs.

// GenSpec describes an incident to generate.
type GenSpec struct {
	System     string   `yaml:"system" json:"system"`
	Failures   []string `yaml:"failures" json:"failures,omitempty"` // explicit failure ids
	Symptom    string   `yaml:"symptom" json:"symptom,omitempty"`   // or pick by symptom
	Context    string   `yaml:"context" json:"context,omitempty"`
	Difficulty int      `yaml:"difficulty" json:"difficulty"` // 1..5
	Mode       string   `yaml:"mode" json:"mode,omitempty"`   // "", production (P1 war room), unknown (only impact is given)
	Seed       int64    `yaml:"seed" json:"seed"`
	Focus      []string `yaml:"focus" json:"focus,omitempty"` // skills to embed (stealth retention checks)
}

// ID returns a stable identifier for the spec.
func (g GenSpec) ID() string {
	h := fnv.New32a()
	fmt.Fprintf(h, "%s|%v|%s|%s|%d|%s|%d|%v", g.System, g.Failures, g.Symptom, g.Context, g.Difficulty, g.Mode, g.Seed, g.Focus)
	return fmt.Sprintf("gen-%s-%08x", g.System, h.Sum32())
}

func pick[T any](seed int64, salt string, xs []T) T {
	return xs[int(mix(uint64(seed), salt)%uint64(len(xs)))]
}

func compatible(chosen []*FailureMode, f *FailureMode) bool {
	for _, c := range chosen {
		if c.ID == f.ID || c.Layer == f.Layer {
			return false
		}
		for _, x := range c.Conflicts {
			if x == f.ID {
				return false
			}
		}
		for _, x := range f.Conflicts {
			if x == c.ID {
				return false
			}
		}
	}
	return true
}

// SelectFailures resolves which failures a spec injects.
func (lib *Library) SelectFailures(g GenSpec) ([]*FailureMode, error) {
	sys := lib.Systems[g.System]
	if sys == nil {
		return nil, fmt.Errorf("unknown system %q", g.System)
	}
	var chosen []*FailureMode
	for _, id := range g.Failures {
		f := lib.Failures[id]
		if f == nil || f.System != sys.ID {
			return nil, fmt.Errorf("failure %q not found in system %s", id, sys.ID)
		}
		chosen = append(chosen, f)
	}
	if len(chosen) > 0 {
		return chosen, nil
	}
	n := 1
	switch {
	case g.Difficulty >= 5:
		n = 3
	case g.Difficulty == 4:
		n = 2
	}
	cands := append([]*FailureMode{}, sys.Failures...)
	sort.Slice(cands, func(i, j int) bool { return cands[i].ID < cands[j].ID })
	// focus skills (stealth assessment) are preferred for the primary failure
	primary := cands
	if len(g.Focus) > 0 {
		var f []*FailureMode
		for _, c := range cands {
			for _, s := range c.Skills {
				for _, want := range g.Focus {
					if s == want {
						f = append(f, c)
					}
				}
			}
		}
		if len(f) > 0 {
			primary = dedupe(f)
		}
	}
	if g.Symptom != "" {
		var f []*FailureMode
		for _, c := range primary {
			if c.Symptom == g.Symptom {
				f = append(f, c)
			}
		}
		if len(f) == 0 {
			return nil, fmt.Errorf("no failure with symptom %s in system %s", g.Symptom, sys.ID)
		}
		primary = f
	}
	chosen = append(chosen, pick(g.Seed, "primary", primary))
	for i := 1; i < n; i++ {
		var rest []*FailureMode
		for _, c := range cands {
			if compatible(chosen, c) && c.Kind != "COST" {
				rest = append(rest, c)
			}
		}
		if len(rest) == 0 {
			break
		}
		chosen = append(chosen, pick(g.Seed, fmt.Sprintf("extra-%d", i), rest))
	}
	return chosen, nil
}

func dedupe(fs []*FailureMode) []*FailureMode {
	seen := map[string]bool{}
	var out []*FailureMode
	for _, f := range fs {
		if !seen[f.ID] {
			seen[f.ID] = true
			out = append(out, f)
		}
	}
	return out
}

// Generate builds a lab from a spec. The lab is written in Spanish (the
// primary language) and carries its English overlay: the same spec and seed
// are generated from the English view of the library and paired field by
// field.
func (lib *Library) Generate(g GenSpec) (*Lab, error) {
	es, err := lib.generate(g, i18n.ES)
	if err != nil {
		return nil, err
	}
	en, err := lib.Localized(i18n.EN).generate(g, i18n.EN)
	if err != nil {
		return nil, err
	}
	es.EN = pairOverlay(es, en)
	return es, nil
}

func (lib *Library) generate(g GenSpec, lang string) (*Lab, error) {
	p := func(es, en string) string { return i18n.P(lang, es, en) }
	if g.Difficulty < 1 {
		g.Difficulty = 2
	}
	if g.Difficulty > 5 {
		g.Difficulty = 5
	}
	sys := lib.Systems[g.System]
	if sys == nil {
		return nil, fmt.Errorf("unknown system %q", g.System)
	}
	fails, err := lib.SelectFailures(g)
	if err != nil {
		return nil, err
	}
	ctx := lib.Contexts[g.Context]
	if ctx == nil {
		keys := make([]string, 0, len(lib.Contexts))
		for k := range lib.Contexts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) > 0 {
			ctx = lib.Contexts[pick(g.Seed, "context", keys)]
		} else {
			ctx = &Context{ID: "generic", Company: "Nebula Corporation", Service: sys.Title, Impact: p("Los usuarios están afectados.", "Users are affected."), Priority: "P2"}
		}
	}
	primary := fails[0]
	sym := lib.Symptoms[primary.Symptom]
	l := &Lab{
		ID:          g.ID(),
		Track:       "generated",
		Level:       []string{"basic", "basic", "intermediate", "advanced", "professional", "professional"}[g.Difficulty],
		Branch:      sys.Branch,
		Type:        "incident",
		Mode:        g.Mode,
		Difficulty:  g.Difficulty,
		Minutes:     30 + 15*g.Difficulty,
		Baseline:    append([]string{}, sys.Baseline...),
		Setup:       sys.Setup,
		Traffic:     sys.Traffic,
		Student:     sys.Student,
		Params:      map[string][]string{},
		Generated:   &g,
		Constraints: append([]string{}, ctx.Constraints...),
		PassScore:   70,
	}
	for _, d := range sys.Decoys {
		l.Constraints = append(l.Constraints, "protect:"+d)
	}
	skills := map[string]bool{}
	for _, s := range sys.Skills {
		skills[s] = true
	}
	var titles []string
	for _, f := range fails {
		titles = append(titles, f.Title)
		for _, s := range f.Skills {
			skills[s] = true
		}
		for k, v := range f.Params {
			l.Params[k] = v
		}
		if f.Kind == "SEC" && primary == f {
			l.Branch = "security"
		}
	}
	for s := range skills {
		l.Skills = append(l.Skills, s)
	}
	sort.Strings(l.Skills)
	symName := primary.Symptom
	if sym != nil {
		symName = sym.Name
	}
	l.Title = fmt.Sprintf("%s · %s: %s", ctx.Company, symName, ctx.Service)
	if g.Mode == "unknown" {
		l.Title = fmt.Sprintf(p("%s · Problema desconocido: %s", "%s · Unknown problem: %s"), ctx.Company, ctx.Service)
	}
	l.Summary = fmt.Sprintf(p("Incidente generado en %s (dificultad %d, %d fallo(s)).", "Generated incident on %s (difficulty %d, %d fault(s))."), sys.Title, g.Difficulty, len(fails))

	// Faults (+ noise at difficulty ≥ 2)
	for _, f := range fails {
		l.Faults = append(l.Faults, f.Faults...)
	}
	if g.Difficulty >= 2 {
		l.Faults = append(l.Faults, sys.Noise...)
	}

	// Ticket: accurate → vague → misleading → impact only.
	kind := firstNonEmpty(primary.Kind, "INC")
	prio := firstNonEmpty(ctx.Priority, "P2")
	if (g.Difficulty >= 4 || g.Mode == "production") && kind == "INC" {
		prio = "P1"
	}
	summary := primary.Summary
	misleading := false
	switch {
	case g.Mode == "unknown":
		summary = ctx.Impact
	case g.Difficulty >= 4 && sym != nil && sym.Misleading[primary.Layer] != "":
		summary, misleading = sym.Misleading[primary.Layer], true
	case g.Difficulty >= 3 && sym != nil && len(sym.Vague) > 0:
		summary = pick(g.Seed, "vague", sym.Vague)
	}
	ticketID := fmt.Sprintf("%s-%d", kind, 1000+int(mix(uint64(g.Seed), "ticket")%9000))
	reporter := firstNonEmpty(ctx.Reporter, p("Atención al cliente", "Customer Support"))
	l.Ticket = &desk.Ticket{ID: ticketID, Kind: kind, Priority: prio, Summary: summary, Impact: ctx.Impact, Reporter: reporter, Service: ctx.Service, Status: "NEW", Misleading: misleading,
		Comments: []desk.Comment{{At: "08:55", From: reporter, Text: summary}}}
	l.Ticket.SLA = map[string]string{"P1": "30m", "P2": "4h", "P3": "1d", "P4": "3d"}[prio]

	// Story
	var sb strings.Builder
	fmt.Fprintf(&sb, p("**%s · %s %s** — abierto por %s.\n\n> %s\n\n", "**%s · %s %s** — reported by %s.\n\n> %s\n\n"), ticketID, kind, prio, l.Ticket.Reporter, summary)
	if g.Mode != "unknown" {
		fmt.Fprintf(&sb, p("Servicio: **%s** (%s).\n\n", "Service: **%s** (%s).\n\n"), ctx.Service, sys.Title)
	}
	fmt.Fprintf(&sb, p("Impacto en el negocio: %s\n\n", "Business impact: %s\n\n"), ctx.Impact)
	if len(l.Constraints) > 0 {
		fmt.Fprintf(&sb, p("Restricciones: %s.\n\n", "Constraints: %s.\n\n"), humanConstraints(l.Constraints, lang))
	}
	sb.WriteString(p("Usa `ticket`, `team` y `ask QUIÉN \"pregunta\"` para trabajar el ticket. Publica una actualización (`ticket update`) y resuélvelo con una nota cuando termines.", "Use `ticket`, `team` and `ask WHO \"question\"` to work the ticket. Post an update (`ticket update`) and resolve it with a note when done."))
	if prio == "P1" {
		sb.WriteString(p(" **P1: el post-mortem es obligatorio.**", " **P1: a postmortem is mandatory.**"))
	}
	l.Story = sb.String()
	if prio == "P1" {
		// Mode P1: alert cascade, business pressure and a mandatory postmortem.
		l.Timeline = []TimelineEvent{
			{At: "08:50", From: "Cloud Monitoring", Text: p("La comprobación de disponibilidad falla desde 3 regiones", "Uptime check failing from 3 regions")},
			{At: "08:51", From: "PagerDuty", Text: p("Aviso P1: ratio de 5xx > 50 % en ", "P1 page: 5xx ratio > 50% on ") + ctx.Service},
			{At: "08:52", From: "Cloud Monitoring", Text: p("Consumo del presupuesto de error 14x (consumo rápido)", "Error budget burn rate 14x (fast burn)")},
			{At: "08:54", From: p("Atención al cliente", "Customer Support"), Text: p("Más de 40 quejas de clientes en 5 minutos", "40+ customer complaints in 5 minutes")},
			{At: "08:56", From: p("Responsable del incidente", "Incident Commander"), Text: p("Actualización de estado cada 30 minutos; post-mortem en un plazo de 48 h", "Status update required every 30 minutes; postmortem due within 48h")},
		}
	} else {
		l.Timeline = []TimelineEvent{{At: "08:52", From: "Cloud Monitoring", Text: p("Alerta: ", "Alert: ") + symName}}
	}
	l.Objectives = []string{
		p("Reúne evidencias y formula hipótesis antes de cambiar nada", "Gather evidence and form hypotheses before changing anything"),
		p("Restablece el servicio con el cambio seguro mínimo", "Restore the service with the minimum safe change"),
		p("Respeta las barreras de seguridad y las restricciones indicadas", "Keep security guardrails and stated constraints"),
		p("Comunica en el ticket y documenta la causa raíz", "Communicate on the ticket and document the root cause"),
	}

	// Actors: system actors + failure facts merged by role.
	actors := map[string]*desk.Actor{}
	var order []string
	for _, a := range sys.Actors {
		cp := a
		cp.Facts = append([]desk.Fact{}, a.Facts...)
		actors[a.Role] = &cp
		order = append(order, a.Role)
	}
	for _, f := range fails {
		roles := make([]string, 0, len(f.Facts))
		for r := range f.Facts {
			roles = append(roles, r)
		}
		sort.Strings(roles)
		for _, r := range roles {
			a := actors[r]
			if a == nil {
				a = &desk.Actor{Role: r, Name: strings.Title(r)}
				actors[r] = a
				order = append(order, r)
			}
			a.Facts = append(append([]desk.Fact{}, f.Facts[r]...), a.Facts...)
		}
	}
	for _, r := range order {
		l.Actors = append(l.Actors, *actors[r])
	}

	// Hints shrink with difficulty; none at 5.
	maxHints := map[int]int{1: 3, 2: 3, 3: 2, 4: 1, 5: 0}[g.Difficulty]
	for _, f := range fails {
		for _, h := range f.Hints {
			if len(l.Hints) < maxHints {
				l.Hints = append(l.Hints, h)
			}
		}
	}

	// Solution: diagnosis, fixes, settle, communication.
	var sol strings.Builder
	settle := 0
	for _, f := range fails {
		sol.WriteString(strings.TrimRight(f.Solution, "\n") + "\n")
		if f.Settle > settle {
			settle = f.Settle
		}
	}
	if settle > 0 {
		fmt.Fprintf(&sol, "sleep %d\n", settle*60)
	}
	sol.WriteString(p("ticket update \"Servicio restablecido tras corregir la causa raíz; vigilamos la recuperación, próxima actualización en 30 minutos. Impacto limitado a la ventana del incidente.\"\n",
		"ticket update \"Service restored after fixing the root cause; monitoring recovery, next update in 30 minutes. Impact limited to the incident window.\"\n"))
	sol.WriteString(p("ticket resolve \"Causa raíz identificada y corregida; verificado que el servicio vuelve a responder correctamente.\"\n",
		"ticket resolve \"Root cause identified and fixed; verified the service responds correctly again.\"\n"))
	l.Solution = sol.String()

	// Tutor mode: what each failure is and why the fix works.
	l.Tutor = append(l.Tutor, TutorNote{Match: "@intro", Why: p(
		"Vamos a resolverlo como en una guardia real: primero investigamos sin cambiar nada, después arreglamos la causa (no el síntoma), comprobamos que funciona y cerramos informando al negocio.",
		"We will solve it like a real on-call shift: first investigate without changing anything, then fix the cause (not the symptom), check it works and close by updating the business.")})
	for _, f := range fails {
		if f.Evidence.Sample != "" {
			l.Tutor = append(l.Tutor, TutorNote{Match: "@fix", Why: p("La causa: ", "The cause: ") + f.Evidence.Sample})
		}
	}

	// Evidence
	l.Evidence = &Evidence{Fields: []string{"rootCause", "prevention"}, Sample: map[string]string{}}
	var rc []string
	for _, f := range fails {
		rc = append(rc, f.Evidence.Sample)
	}
	l.Evidence.Sample["rootCause"] = strings.Join(rc, " ")
	l.Evidence.Sample["prevention"] = p("Añadir una comprobación de políticas en CI para este cambio, alertar sobre el síntoma y documentar el runbook para que el cambio se revise antes de llegar a producción.", "Add a policy check in CI for this change, alert on the symptom and document the runbook so the change is reviewed before it reaches production.")
	if prio == "P1" {
		l.Evidence.Fields = append(l.Evidence.Fields, "postmortem")
		l.Evidence.Sample["postmortem"] = p("Cronología: detección por las alertas, diagnóstico con registros y configuración, corrección aplicada y verificada. Impacto: usuarios afectados durante la ventana. La causa raíz y las acciones con responsables están indicadas arriba.", "Timeline: detection from alerts, diagnosis with logs and configuration, fix applied and verified. Impact: users affected during the window. Root cause and action items with owners are listed above.")
	}

	// Rubric (100 points)
	var healthy []Check
	healthy = append(healthy, sys.Healthy...)
	var safety []Check
	safety = append(safety, sys.Safety...)
	for _, f := range fails {
		safety = append(safety, f.Checks...)
	}
	var diag []Check
	for _, f := range fails {
		if f.Evidence.Commands != "" {
			diag = append(diag, Check{"type": "command", "regex": f.Evidence.Commands, "desc": p("Evidencias reunidas para: ", "Evidence gathered for: ") + f.Title})
		}
	}
	var rcChecks []Check
	for _, f := range fails {
		groups := make([]any, 0, len(f.Evidence.RootCause))
		for _, g := range f.Evidence.RootCause {
			gg := make([]any, len(g))
			for i, x := range g {
				gg[i] = x
			}
			groups = append(groups, gg)
		}
		rcChecks = append(rcChecks, Check{"type": "evidence", "fields": []any{"rootCause"}, "keywords": groups, "desc": p("Causa raíz explicada: ", "Root cause explained: ") + f.Title})
	}
	comm := []Check{
		{"type": "ticket_update", "min": 1, "desc": p("Actualización para las partes interesadas publicada en el ticket", "Stakeholder update posted on the ticket")},
		{"type": "ticket_resolved", "desc": p("Ticket resuelto con una nota de resolución", "Ticket resolved with a resolution note")},
	}
	if prio == "P1" {
		comm = append(comm, Check{"type": "evidence", "fields": []any{"postmortem"}, "keywords": []any{[]any{"timeline", "cronologia", "detect"}, []any{"impact", "impacto"}}, "desc": p("Post-mortem (obligatorio en P1)", "Postmortem (mandatory for P1)")})
	}
	l.Rubric = []RubricItem{{Name: p("Servicio restablecido", "Service restored"), Validator: "functional", Points: 35, All: true, Checks: healthy}}
	if len(safety) > 0 {
		l.Rubric = append(l.Rubric, RubricItem{Name: p("Corrección segura", "Safe fix"), Validator: "security", Points: 15, Critical: true, All: true, Checks: safety})
	} else {
		l.Rubric[0].Points += 15
	}
	l.Rubric = append(l.Rubric,
		RubricItem{Name: p("Diagnóstico", "Diagnosis"), Validator: "diagnosis", Points: 15, Checks: diag},
		RubricItem{Name: p("Causa raíz", "Root cause"), Validator: "evidence", Points: 20, Checks: rcChecks},
		RubricItem{Name: p("Comunicación", "Communication"), Validator: "evidence", Points: 15, Checks: comm},
	)
	if len(diag) == 0 {
		l.Rubric[len(l.Rubric)-3].Checks = []Check{{"type": "command", "regex": "logging read|describe|list|get-health", "desc": p("Evidencias reunidas", "Evidence gathered")}}
	}
	l.WellArchitected = []string{"reliability", "operations"}
	l.Certs = []string{"ACE"}
	l.defaults()
	_ = titles
	return l, nil
}

func humanConstraints(cs []string, lang string) string {
	p := func(es, en string) string { return i18n.P(lang, es, en) }
	var out []string
	for _, c := range cs {
		switch {
		case c == "no-downtime":
			out = append(out, p("no se permiten cortes (no detengas ni borres recursos que estén sirviendo)", "no downtime allowed (do not stop or delete serving resources)"))
		case c == "no-public":
			out = append(out, p("no se puede exponer nada públicamente", "nothing may be exposed publicly"))
		case c == "no-basic-roles":
			out = append(out, p("sin roles básicos (owner/editor/viewer)", "no basic roles (owner/editor/viewer)"))
		case c == "no-delete":
			out = append(out, p("sin borrados", "no deletions"))
		case strings.HasPrefix(c, "protect:"):
			out = append(out, i18n.Pf(lang, "no modifiques %s (pertenece a otro equipo)", "do not modify %s (owned by another team)", strings.TrimPrefix(c, "protect:")))
		default:
			out = append(out, c)
		}
	}
	return strings.Join(out, "; ")
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
