package cli

import (
	"strings"
	"testing"

	"github.com/neodevesp/gcp-trainer/internal/desk"
)

// The web ticket panel sends replies as single-quoted shell words; text with
// quotes, newlines and $ must reach the ticket unchanged.
func TestDeskQuotedText(t *testing.T) {
	s := newTestSession()
	s.Desk = &desk.Desk{Ticket: &desk.Ticket{ID: "REQ-1", Status: "NEW"}}
	text := "Hola Laura: it's done, cost $5 \"ok\" segunda línea"
	q := "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
	if r := s.Exec("ticket update " + q); r.Exit != 0 {
		t.Fatal(r.Output)
	}
	got := s.Desk.Ticket.Comments[len(s.Desk.Ticket.Comments)-1].Text
	if got != text {
		t.Fatalf("got %q want %q", got, text)
	}
}
