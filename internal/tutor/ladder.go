package tutor

import (
	"regexp"
	"strings"
)

// The hint ladder: five levels, each giving away a bit more than the last.
//
//	1 guiding question  — what to look at, without naming the cause
//	2 concept           — the idea behind the fault
//	3 where to look     — the console page or read-only command that shows it
//	4 example command   — the shape of the change, with the values hidden
//	5 full walkthrough  — the solution
//
// Levels 1–3 never contain a command that changes anything, and level 4
// never contains the solution's real values: a learner who stops at any
// level still has to do the rest. Hints written by the lab author fill the
// level they belong to (an explanation → 2, a partial command → 4, the
// solution → 5); the rest is derived from the solution script.

// Authored is a hint written in the lab.
type Authored struct {
	Text string
	Kind string // hint, partial, solution
}

// Rung is one level of the ladder.
type Rung struct {
	Level int    `json:"level"` // 1..5
	Name  string `json:"name"`
	Text  string `json:"text"`
	Kind  string `json:"kind"` // hint, partial, solution
	Cost  int    `json:"cost"`
}

// LadderCosts are the XP each level costs.
var LadderCosts = [5]int{1, 3, 5, 8, 15}

// LevelNames names the levels (index = level-1).
func LevelNames(en bool) [5]string {
	if en {
		return [5]string{"Guiding question", "Concept", "Where to look", "Example command", "Full walkthrough"}
	}
	return [5]string{"Pregunta guía", "Concepto", "Dónde mirar", "Comando de ejemplo", "Solución completa"}
}

// a hint with a command in it (in backticks) is an example command, not a concept
var reCmdInText = regexp.MustCompile("`[^`]+`")

// Ladder builds the five levels for a lab. It returns nil when there is
// nothing to build them from.
func Ladder(authored []Authored, solution string, notes []Note, lang string) []Rung {
	en := strings.HasPrefix(strings.ToLower(lang), "en")
	L := func(es, e string) string {
		if en {
			return e
		}
		return es
	}
	plan := Build(solution, notes, lang)
	var look, fix *Step
	var fixes []Step
	for i := range plan.Steps {
		st := &plan.Steps[i]
		switch st.Phase {
		case Investigate:
			if look == nil && fix == nil && !platformCmd(st.Command) {
				look = st
			}
		case Fix:
			if fix == nil {
				fix = st
			}
			fixes = append(fixes, *st)
		}
	}
	text := [5]string{}
	// authored hints go to their level
	for _, a := range authored {
		t := strings.TrimSpace(a.Text)
		if t == "" {
			continue
		}
		lv := 2
		switch {
		case a.Kind == "solution":
			lv = 5
		case a.Kind == "partial":
			lv = 4
		case reCmdInText.MatchString(t):
			// a hint whose commands only look is a place to look; one that
			// changes something is an example command
			lv = 3
			for _, m := range reCmdInText.FindAllString(t, -1) {
				st, ok := explain(strings.Trim(m, "`"), en)
				if !ok || st.Phase != Investigate {
					lv = 4
				}
			}
		}
		if text[lv-1] == "" {
			text[lv-1] = t
		} else {
			text[lv-1] += "\n\n" + t
		}
	}

	// 1 · guiding question: where the symptom shows, never where the cause is
	if look != nil || fix != nil {
		area := L("el servicio afectado", "the affected service")
		if look != nil && look.Where.Path != "" && look.Where.Path != "Cloud Shell" {
			area = look.Where.Path
		}
		text[0] = L(
			"Antes de cambiar nada: ¿qué está fallando exactamente y para quién? Empieza por "+area+": ¿qué esperarías ver si todo estuviera bien, y qué ves? Escribe una hipótesis antes de tocar nada.",
			"Before changing anything: what exactly is failing, and for whom? Start with "+area+": what would you expect to see if everything were fine, and what do you see? Write down a hypothesis before touching anything.")
	} else if len(plan.Steps) > 0 {
		text[0] = L("¿Qué te pide exactamente el ticket y cómo comprobarías que ya está hecho?", "What exactly does the ticket ask for, and how would you check it is done?")
	}
	// 2 · concept
	if text[1] == "" && fix != nil {
		c := fix.Concept
		if c == "" {
			c = fix.Why
		}
		if c != "" && c != genericWhyES && c != genericWhyEN && !leaks(c, fixes) {
			text[1] = c
		}
	}
	// 3 · where to look: the read-only check that shows the fault, then the
	// place where the change is made
	var where []string
	if text[2] != "" {
		where = append(where, text[2]) // the author's own pointer
	} else if look != nil {
		t := L("Mira en "+look.Where.Path+".", "Look at "+look.Where.Path+".")
		if look.Command != "" {
			t += L(" Desde Cloud Shell te lo enseña: ", " From Cloud Shell this shows it: ") + "`" + firstLine(look.Command) + "`"
		}
		where = append(where, t)
	}
	if fix != nil && fix.Where.Path != "" && fix.Where.Path != "Cloud Shell" && (look == nil || fix.Where.Path != look.Where.Path) {
		where = append(where, L("Después revisa cómo está configurado "+fix.Where.Path+": ahí se hace el cambio.", "Then check how "+fix.Where.Path+" is configured: that is where the change is made."))
	}
	text[2] = strings.Join(where, " ")
	// 4 · example command (values hidden)
	if text[3] == "" && fix != nil {
		text[3] = L("El cambio tiene esta forma (pon tú los valores): ", "The change looks like this (fill in the values yourself): ") + "`" + Mask(firstLine(fix.Command)) + "`"
	}
	// 5 · full walkthrough
	if text[4] == "" && len(plan.Steps) > 0 {
		var b []string
		for _, st := range plan.Steps {
			if st.Phase == Fix || st.Phase == Verify {
				b = append(b, "`"+firstLine(st.Command)+"`")
			}
		}
		if len(b) > 0 {
			text[4] = L("Solución: ", "Solution: ") + strings.Join(b, " → ")
		}
	}

	names := LevelNames(en)
	kinds := [5]string{"hint", "hint", "hint", "partial", "solution"}
	var out []Rung
	for i, t := range text {
		if t == "" {
			continue
		}
		out = append(out, Rung{Level: i + 1, Name: names[i], Text: t, Kind: kinds[i], Cost: LadderCosts[i]})
	}
	if len(out) < 2 {
		return nil // not enough to grade the help: keep the lab's own hints
	}
	return out
}

// platformCmd: the lab's own helpers (ticket, team…), not a place to look.
func platformCmd(c string) bool {
	switch firstWord(strings.TrimSpace(c)) {
	case "ticket", "team", "ask", "why", "whatif", "answer", "interview", "arch", "echo", "printf", "cat":
		return true
	}
	return false
}

func firstLine(s string) string { return strings.TrimSpace(strings.SplitN(s, "\n", 2)[0]) }

// leaks reports whether a text contains one of the fix commands.
func leaks(t string, fixes []Step) bool {
	for _, f := range fixes {
		if c := firstLine(f.Command); len(c) > 12 && strings.Contains(t, c) {
			return true
		}
	}
	return false
}

// Mask keeps the shape of a command (tool, resource group, verb and flag
// names) and hides the values: names, members, roles, ranges, paths.
func Mask(cmd string) string {
	ws := words(cmd)
	if len(ws) == 0 {
		return ""
	}
	out := []string{ws[0]}
	keep := 1
	if ws[0] == "gcloud" {
		pos := 0
		for i := 1; i < len(ws) && !strings.HasPrefix(ws[i], "-"); i++ {
			pos++
		}
		// longest known resource group + verb
		p := ws[1 : 1+pos]
		start := 0
		for start < len(p) && (p[start] == "alpha" || p[start] == "beta") {
			start++
		}
		n := 0
		for k := min(4, len(p)-start); k >= 1; k-- {
			if _, ok := resources[strings.Join(p[start:start+k], " ")]; ok {
				n = k
				break
			}
		}
		if n == 0 {
			n = min(1, len(p)-start)
		}
		keep = 1 + start + n + 1 // tool + alpha/beta + group + verb
	} else if len(ws) > 1 && !strings.HasPrefix(ws[1], "-") {
		keep = 2
	}
	for i := 1; i < len(ws); i++ {
		w := ws[i]
		switch {
		case i < keep:
			out = append(out, w)
		case strings.HasPrefix(w, "--"):
			k, _, hasVal := strings.Cut(w, "=")
			if !hasVal && (boolFlag[strings.TrimPrefix(k, "--")] || i+1 >= len(ws) || strings.HasPrefix(ws[i+1], "-")) {
				out = append(out, k)
				continue
			}
			out = append(out, k+"=…")
			if !hasVal {
				i++ // the value was the next word
			}
		case strings.HasPrefix(w, "-"):
			out = append(out, w)
		default:
			out = append(out, "…")
		}
	}
	return strings.Join(out, " ")
}
