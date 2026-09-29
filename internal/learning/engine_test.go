package learning

import (
	"testing"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/grader"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

func loadCat(t *testing.T) *Catalog {
	t.Helper()
	c, err := LoadCatalog("../../content")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func att(id, lab string, score int, passed bool, hints int, fin time.Time) Attempt {
	f := fin
	res := &grader.Result{Score: score, Passed: passed, Validators: map[string][2]float64{"functional": {float64(score) / 2, 50}, "security": {float64(score) / 2, 50}}}
	res.Process = &grader.ProcessReport{Factors: []grader.Factor{{Name: "diagnosis", Score: 80}, {Name: "security", Score: 90}}}
	h := make([]int, hints)
	return Attempt{ID: id, UserID: "u", LabID: lab, Track: "ace-30", Started: fin.Add(-time.Hour), Finished: &f, Score: score, Passed: passed, Status: "submitted", Result: res, HintsUsed: h}
}

func TestCatalogValid(t *testing.T) {
	c := loadCat(t)
	if p := c.Validate(); len(p) > 0 {
		t.Fatalf("catalog problems: %v", p)
	}
	g := c.Graph()
	if len(g) < 60 {
		t.Fatalf("expected a rich skill graph, got %d nodes", len(g))
	}
	pre := c.Prerequisites("networking.hybrid")
	want := map[string]bool{"netfund.tcpip": false, "netfund.cidr": false, "networking.vpc": false, "networking.firewall": false}
	for _, p := range pre {
		if _, ok := want[p]; ok {
			want[p] = true
		}
	}
	for k, v := range want {
		if !v {
			t.Errorf("networking.hybrid should transitively require %s", k)
		}
	}
	ok, missing := c.Unlocked("networking.firewall", map[string]float64{}, 60)
	if ok || len(missing) == 0 {
		t.Error("firewall should be locked without routing mastery")
	}
}

func TestMasteryFormulaAndStudentModel(t *testing.T) {
	c := loadCat(t)
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	e := &Engine{Cat: c, Now: func() time.Time { return now }}
	atts := []Attempt{
		att("1", "ace-d11-app-403", 95, true, 0, now.Add(-40*24*time.Hour)),
		att("2", "ace-d02-iam-storage-read", 90, true, 0, now.Add(-45*24*time.Hour)),
		att("3", "ace-d19-secret-env", 60, false, 2, now.Add(-2*24*time.Hour)),
	}
	skills := e.Mastery(atts)
	if len(skills) == 0 {
		t.Fatal("no skills")
	}
	for _, s := range skills {
		want := round1(0.40*s.Correctness + 0.20*s.Independence + 0.15*s.Retention + 0.15*s.Difficulty + 0.10*s.Incident)
		if s.Mastery != want {
			t.Fatalf("mastery formula mismatch for %s: %v vs %v", s.Skill, s.Mastery, want)
		}
	}
	sm := e.Student(atts, skills)
	if len(sm.Dimensions) != 9 {
		t.Fatalf("expected 9 dimensions, got %d", len(sm.Dimensions))
	}
	// forgetting: skills practised 40+ days ago must have decayed below demonstrated mastery
	for _, r := range sm.Retention {
		if r.Retention > r.Demonstrated {
			t.Fatalf("retention cannot exceed demonstrated mastery: %+v", r)
		}
		if r.Skill == "iam.leastprivilege" && !r.Due {
			t.Errorf("iam.leastprivilege should be due for review after 40 days: %+v", r)
		}
	}
}

func TestAutonomyLadder(t *testing.T) {
	c := loadCat(t)
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	e := &Engine{Cat: c, Now: func() time.Time { return now }}
	var atts []Attempt
	if a := e.Autonomy(atts); a.Index != 0 || a.Stage != "Guiado" {
		t.Fatalf("empty history should be Guided, got %s", a.Stage)
	}
	for i, l := range []string{"ace-d11-app-403", "ace-d12-private-egress", "ace-d18-api-db-firewall", "ace-d19-secret-env", "ace-d20-sql-saturated", "ace-d25-public-exposure"} {
		atts = append(atts, att(string(rune('a'+i)), l, 95, true, 0, now.Add(-time.Duration(i+1)*time.Hour)))
	}
	a := e.Autonomy(atts)
	if a.Index < 3 {
		t.Fatalf("six unassisted incident passes across branches should reach at least Professional, got %+v", a)
	}
	// hints-heavy passes do not count
	var helped []Attempt
	for _, x := range atts {
		x.HintsUsed = []int{0, 1, 2}
		helped = append(helped, x)
	}
	if e.Autonomy(helped).Index != 0 {
		t.Fatal("assisted passes must not raise autonomy")
	}
}

func TestCareerAndTranscript(t *testing.T) {
	c := loadCat(t)
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	e := &Engine{Cat: c, Now: func() time.Time { return now }}
	p := e.Profile(User{ID: "u", Name: "Ada"}, nil, "ace-30")
	if p.Career.Stage != "Becario/a cloud" {
		t.Fatalf("new learner should be Becario/a cloud, got %q", p.Career.Stage)
	}
	if en := e.ForLang("en").Profile(User{ID: "u", Name: "Ada"}, nil, "ace-30"); en.Career.Stage != "Cloud Intern" {
		t.Fatalf("in English the first stage is Cloud Intern, got %q", en.Career.Stage)
	}
	if len(p.Career.NextNeeds) == 0 {
		t.Fatal("next stage should list needs")
	}
	tr := NewTranscript(p, nil, []byte("k"), now)
	if !tr.Verify([]byte("k")) {
		t.Fatal("transcript should verify")
	}
	tr.Stage = "Cloud Architect"
	if tr.Verify([]byte("k")) {
		t.Fatal("tampered transcript must not verify")
	}
}

func TestAwardHintCostsAndBonusCap(t *testing.T) {
	c := loadCat(t)
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	e := &Engine{Cat: c, Now: func() time.Time { return now }}
	a := att("x", "ace-d30-boss-incident", 95, true, 0, now)
	a.HintCost = 0
	e.Award(&a, nil)
	if a.XP != 95 || a.Bonus == 0 {
		t.Fatalf("unexpected award %d/%d", a.XP, a.Bonus)
	}
	var prior []Attempt
	for i := 0; i < 20; i++ {
		prior = append(prior, Attempt{ID: string(rune('A' + i)), Track: "ace-30", Bonus: 20, Status: "submitted"})
	}
	b := att("y", "ace-d29-capstone-orders", 95, true, 0, now)
	e.Award(&b, prior)
	if b.Bonus != 0 {
		t.Fatalf("bonus should be capped at %d per track, got %d", BonusCap, b.Bonus)
	}
}

func TestAdaptiveStealthRetention(t *testing.T) {
	c := loadCat(t)
	lib, err := scenario.LoadLibrary("../../content/failures")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	e := &Engine{Cat: c, Now: func() time.Time { return now }, Lib: lib}
	atts := []Attempt{att("1", "ace-d11-app-403", 95, true, 0, now.Add(-60*24*time.Hour))}
	p := e.Profile(User{ID: "u"}, atts, "ace-30")
	var stealth *Recommendation
	for i, r := range p.Adaptive {
		if r.Kind == "stealth" {
			stealth = &p.Adaptive[i]
			break
		}
	}
	if stealth == nil || stealth.Gen == nil {
		t.Fatalf("expected a stealth retention incident, got %+v", p.Adaptive)
	}
	l, err := lib.Generate(*stealth.Gen)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range l.Skills {
		if s == stealth.Skill {
			found = true
		}
	}
	if !found {
		t.Fatalf("generated incident %s should exercise %s (skills %v)", l.ID, stealth.Skill, l.Skills)
	}
}
