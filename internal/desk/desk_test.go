package desk

import (
	"strings"
	"testing"
	"time"
)

func newDesk() *Desk {
	return &Desk{
		Ticket: &Ticket{ID: "INC-1", Kind: "INC", Priority: "P2", Summary: "checkout is down", Status: "NEW"},
		Actors: []Actor{{
			Role: "developer", Name: "Lucía Gómez",
			Facts: []Fact{
				{Keywords: [][]string{{"deploy", "release", "cambio"}, {"today", "hoy", "yesterday"}}, Answer: "We shipped v42 at 08:40.", Evidence: "deploy.log", Content: "v42 08:40"},
			},
			Fallback: "No idea, sorry.",
		}},
	}
}

func TestAskMatchesFactsDeterministically(t *testing.T) {
	d := newDesk()
	// name prefix and accent-insensitive lookup
	ans, file, content, err := d.Ask("lucia", "Did you deploy anything today?", "09:00")
	if err != nil || !strings.Contains(ans, "v42") || file != "deploy.log" || content == "" {
		t.Fatalf("ask: %q %q %q %v", ans, file, content, err)
	}
	// every keyword group must match; otherwise the fallback is used
	ans, file, _, _ = d.Ask("developer", "any release?", "09:01")
	if ans != "No idea, sorry." || file != "" {
		t.Fatalf("partial match answered: %q", ans)
	}
	if len(d.Questions) != 2 || !d.Questions[0].Useful || d.Questions[1].Useful {
		t.Fatalf("question log %+v", d.Questions)
	}
	if _, _, _, err := d.Ask("finance", "cost?", "09:02"); err == nil || !strings.Contains(err.Error(), "developer") {
		t.Fatalf("unknown actor error: %v", err)
	}
}

func TestTicketWorkflow(t *testing.T) {
	d := newDesk()
	if err := d.Comment("student", "  ", "09:00", false); err == nil {
		t.Fatal("empty comment accepted")
	}
	if err := d.Comment("student", "Investigating the 502s", "09:00", true); err != nil {
		t.Fatal(err)
	}
	if d.Ticket.Status != "IN_PROGRESS" {
		t.Fatalf("status %s", d.Ticket.Status)
	}
	if err := d.Resolve("student", "fixed it", "09:30"); err == nil {
		t.Fatal("short resolution accepted")
	}
	if err := d.Resolve("student", "rolled back v42 which removed the invoker binding", "09:30"); err != nil {
		t.Fatal(err)
	}
	if d.Ticket.Status != "RESOLVED" || len(d.StudentComments("student")) != 2 {
		t.Fatalf("ticket %+v", d.Ticket)
	}
	c := d.Clone()
	c.Ticket.Comments = append(c.Ticket.Comments, Comment{From: "x"})
	if len(d.Ticket.Comments) == len(c.Ticket.Comments) {
		t.Fatal("clone shares comments with the original")
	}
}

func TestSLADuration(t *testing.T) {
	cases := map[string]time.Duration{"30m": 30 * time.Minute, "4h": 4 * time.Hour, "2d": 48 * time.Hour}
	for sla, want := range cases {
		if got := (&Ticket{SLA: sla}).SLADuration(); got != want {
			t.Errorf("%s: got %v", sla, got)
		}
	}
	if got := (&Ticket{Priority: "P1"}).SLADuration(); got != 30*time.Minute {
		t.Errorf("P1 default: %v", got)
	}
}
