package scenario

import (
	"strings"
	"testing"

	"github.com/neodevesp/gcp-trainer/internal/desk"
	"gopkg.in/yaml.v3"
)

const bilingualLab = `
id: t-i18n
title: "Asegura el bucket"
summary: Un bucket privado.
story: Legal necesita un bucket.
objectives: [Bucket privado, Versionado activo]
hints:
  - text: Mira la prevención de acceso público.
ticket: {id: REQ-1, kind: REQ, priority: P3, summary: "Necesitamos un bucket", reporter: "Ana (legal)"}
actors:
  - role: security
    name: Aisha (seguridad)
    fallback: Nada público.
    facts:
      - {keywords: [[public, publico]], answer: "Nunca público."}
rubric:
  - name: Seguridad
    validator: security
    points: 100
    checks:
      - {type: exists, path: "buckets.x", desc: "Bucket privado"}
en:
  title: Secure the bucket
  summary: A private bucket.
  story: Legal needs a bucket.
  objectives: [Private bucket, Versioning on]
  hints: [Look at public access prevention.]
  ticket: {summary: "We need a bucket"}
  text:
    "Ana (legal)": "Ana (legal)"
    "Aisha (seguridad)": "Aisha (security)"
    "Nada público.": "Nothing public."
    "Nunca público.": "Never public."
    "Seguridad": "Security"
    "Bucket privado": "Private bucket"
`

func loadBilingual(t *testing.T) *Lab {
	t.Helper()
	var l Lab
	if err := yaml.Unmarshal([]byte(bilingualLab), &l); err != nil {
		t.Fatal(err)
	}
	return &l
}

func TestLocalizedEnglishOverlay(t *testing.T) {
	l := loadBilingual(t)
	if got := l.Localized("es"); got != l {
		t.Fatal("Spanish must return the lab itself")
	}
	en := l.Localized("en-GB")
	if en.Title != "Secure the bucket" || en.Objectives[1] != "Versioning on" || en.Hints[0].Text != "Look at public access prevention." {
		t.Fatalf("overlay not applied: %+v", en)
	}
	if en.Ticket.Summary != "We need a bucket" || en.Actors[0].Name != "Aisha (security)" || en.Actors[0].Facts[0].Answer != "Never public." {
		t.Fatalf("ticket/actors not translated: %+v %+v", en.Ticket, en.Actors)
	}
	if en.Rubric[0].Name != "Security" || en.Rubric[0].Checks[0]["desc"] != "Private bucket" {
		t.Fatalf("rubric not translated: %+v", en.Rubric)
	}
	// the original is untouched and grading data is shared
	if l.Rubric[0].Checks[0]["desc"] != "Bucket privado" || l.Actors[0].Facts[0].Answer != "Nunca público." {
		t.Fatal("Localized mutated the Spanish lab")
	}
	if en.Rubric[0].Checks[0]["path"] != "buckets.x" || len(en.Actors[0].Facts[0].Keywords) != 1 {
		t.Fatal("checks and keywords must be preserved")
	}
	if p := l.TranslationProblems(); len(p) != 0 {
		t.Fatalf("complete translation reported problems: %v", p)
	}
}

func TestTranslationProblems(t *testing.T) {
	l := loadBilingual(t)
	delete(l.EN.Text, "Bucket privado")
	l.EN.Text["texto viejo"] = "old text"
	l.EN.Objectives = l.EN.Objectives[:1]
	p := strings.Join(l.TranslationProblems(), "\n")
	for _, want := range []string{"without English translation", "stale en.text", "en.objectives has 1 entries"} {
		if !strings.Contains(p, want) {
			t.Errorf("missing problem %q in:\n%s", want, p)
		}
	}
	l.EN = nil
	if p := l.TranslationProblems(); len(p) != 1 || !strings.Contains(p[0], "missing `en:`") {
		t.Fatalf("missing overlay not reported: %v", p)
	}
	_ = desk.Actor{}
}

// Generated incidents are Spanish with a complete English overlay built from
// the English view of the library.
func TestGeneratedIncidentIsBilingual(t *testing.T) {
	lib, err := LoadLibrary("../../content/failures")
	if err != nil {
		t.Fatal(err)
	}
	if p := lib.TranslationProblems(); len(p) > 0 {
		t.Fatalf("library translation problems: %v", p)
	}
	for _, g := range []GenSpec{
		{System: "three-tier", Difficulty: 2, Seed: 7},
		{System: "shop-platform", Difficulty: 4, Seed: 3, Mode: "production"},
		{System: "gke-shop", Difficulty: 3, Seed: 11, Mode: "unknown"},
	} {
		l, err := lib.Generate(g)
		if err != nil {
			t.Fatal(err)
		}
		if p := l.TranslationProblems(); len(p) > 0 {
			t.Errorf("%s: %v", g.System, p)
		}
		en := l.Localized("en")
		if en.Title == l.Title || en.Story == l.Story || en.Rubric[0].Name == l.Rubric[0].Name {
			t.Errorf("%s: English view not translated: %q / %q", g.System, en.Title, en.Rubric[0].Name)
		}
		if !strings.Contains(l.Story, "Impacto en el negocio") || !strings.Contains(en.Story, "Business impact") {
			t.Errorf("%s: story languages wrong:\n%s\n---\n%s", g.System, l.Story, en.Story)
		}
	}
}
