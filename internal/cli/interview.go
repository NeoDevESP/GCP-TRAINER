package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Interview mode (Blueprint §12): progressive questions and technical
// follow-ups ("repreguntas") that depend on the previous answer, mixed with a
// practical exercise in the same environment.

// InterviewQuestion is the interviewer's view of a question.
type InterviewQuestion struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Options  []string `json:"options"`
	Answer   []int    `json:"-"`
	After    string   `json:"after,omitempty"`
	When     string   `json:"when,omitempty"`
	Probe    string   `json:"-"`
	Justify  bool     `json:"justify"`
}

// Interview holds the interview state of a session.
type Interview struct {
	Questions      []InterviewQuestion `json:"-"`
	Answers        map[string][]int    `json:"answers"`
	Justifications map[string]string   `json:"justifications"`
	Order          []string            `json:"order"`
}

func (iv *Interview) find(id string) *InterviewQuestion {
	for i := range iv.Questions {
		if iv.Questions[i].ID == id {
			return &iv.Questions[i]
		}
	}
	return nil
}

func sameSet(a, b []int) bool {
	x, y := append([]int{}, a...), append([]int{}, b...)
	sort.Ints(x)
	sort.Ints(y)
	return fmt.Sprint(x) == fmt.Sprint(y)
}

// correct reports whether a question was answered correctly.
func (iv *Interview) correct(id string) bool {
	q := iv.find(id)
	a, ok := iv.Answers[id]
	return q != nil && ok && sameSet(a, q.Answer)
}

// next returns the next question to ask, honouring follow-up conditions.
func (iv *Interview) next() *InterviewQuestion {
	for i := range iv.Questions {
		q := &iv.Questions[i]
		if _, done := iv.Answers[q.ID]; done {
			continue
		}
		if q.After != "" {
			if _, prev := iv.Answers[q.After]; !prev {
				continue
			}
			c := iv.correct(q.After)
			if (q.When == "correct" && !c) || (q.When == "incorrect" && c) {
				continue
			}
		}
		return q
	}
	return nil
}

func (s *Session) interviewCmd(args []string) (string, error) {
	iv := s.Interview
	if iv == nil || len(iv.Questions) == 0 {
		return "", fail(1, "%s", s.tr("este laboratorio no es una entrevista", "this lab is not an interview"))
	}
	if args[0] == "answer" {
		if len(args) < 3 {
			return "", fail(2, "%s", s.tr("uso: answer ID_PREGUNTA OPCIÓN[,OPCIÓN] [\"justificación\"]", "usage: answer QUESTION_ID OPTION[,OPTION] [\"justification\"]"))
		}
		q := iv.find(args[1])
		if q == nil {
			return "", fail(1, s.tr("pregunta desconocida %s", "unknown question %s"), args[1])
		}
		if n := iv.next(); n == nil || n.ID != q.ID {
			if _, done := iv.Answers[q.ID]; done {
				return "", fail(1, s.tr("la pregunta %s ya está respondida", "question %s was already answered"), q.ID)
			}
			return "", fail(1, s.tr("el entrevistador todavía no ha preguntado %s (ejecuta `interview`)", "the interviewer has not asked %s yet (run `interview`)"), q.ID)
		}
		var picks []int
		for _, p := range strings.Split(args[2], ",") {
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil || n < 1 || n > len(q.Options) {
				return "", fail(2, s.tr("las opciones van numeradas de 1 a %d", "options are numbered 1..%d"), len(q.Options))
			}
			picks = append(picks, n-1)
		}
		iv.Answers[q.ID] = picks
		iv.Order = append(iv.Order, q.ID)
		if len(args) > 3 {
			iv.Justifications[q.ID] = strings.Join(args[3:], " ")
		}
		who := s.tr("Entrevistador: ", "Interviewer: ")
		out := who + s.tr("Anotado.\n", "Noted.\n")
		if nq := iv.next(); nq != nil && nq.After == q.ID && nq.Probe != "" {
			out = who + nq.Probe + "\n"
		}
		return out + s.tr("(ejecuta `interview` para la siguiente pregunta)\n", "(run `interview` for the next question)\n"), nil
	}
	q := iv.next()
	if q == nil {
		return fmt.Sprintf(s.tr("Entrevistador: no tengo más preguntas (%d respondidas). Termina el ejercicio práctico y envía.\n", "Interviewer: That's all my questions (%d answered). Finish the practical exercise and submit.\n"), len(iv.Answers)), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, s.tr("Entrevistador [%s]: %s\n", "Interviewer [%s]: %s\n"), q.ID, q.Question)
	for i, o := range q.Options {
		fmt.Fprintf(&b, "  %d) %s\n", i+1, o)
	}
	if q.Justify {
		fmt.Fprintf(&b, s.tr("Responde con: answer %s N \"por qué\"  (se espera una justificación)\n", "Answer with: answer %s N \"why\"  (a justification is expected)\n"), q.ID)
	} else {
		fmt.Fprintf(&b, s.tr("Responde con: answer %s N\n", "Answer with: answer %s N\n"), q.ID)
	}
	return b.String(), nil
}
