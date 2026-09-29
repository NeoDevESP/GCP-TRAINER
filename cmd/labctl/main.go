// Command labctl is the content toolchain: schema validation, the lab
// regression pipeline (provision → broken fails → official solution → passes),
// variant preview, curriculum sync, interactive play and load testing.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/neodevesp/gcp-trainer/internal/company"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/grader"
	"github.com/neodevesp/gcp-trainer/internal/labtest"
	"github.com/neodevesp/gcp-trainer/internal/learning"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
	"gopkg.in/yaml.v3"
)

func usage() {
	fmt.Fprint(os.Stderr, `labctl — GCP Lab Simulator content toolchain

  labctl validate              schema + referential integrity of content/
  labctl test [-lab ID] [-seeds 1,7,42] [-json out.json]
                               run the regression pipeline for every lab
  labctl variants -lab ID [-n 5]   preview scenario-engine variants
  labctl play -lab ID [-seed N]    play a lab in the terminal (F0)
  labctl curriculum            compare tracks with the official learning paths
  labctl loadtest [-n 50] [-c 10]  concurrent provisioning/exec benchmark
  labctl stats                 catalogue statistics (labs, incidents, capstones)
  labctl failures              failure library: symptom graph and systems
  labctl generate -system ID [-difficulty 1-5] [-symptom S] [-failures a,b]
                  [-context C] [-mode unknown] [-seed N] [-play]
                               generate an incident (YAML) or play it

Global: -content DIR (default ./content)
`)
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	content := fs.String("content", "content", "content directory")
	labID := fs.String("lab", "", "lab id")
	seeds := fs.String("seeds", "1,7", "comma separated seeds")
	jsonOut := fs.String("json", "", "write JSON report")
	n := fs.Int("n", 5, "count")
	c := fs.Int("c", 8, "concurrency")
	seed := fs.Int64("seed", 1, "seed")
	system := fs.String("system", "three-tier", "failure-library system")
	difficulty := fs.Int("difficulty", 3, "difficulty 1-5")
	symptom := fs.String("symptom", "", "symptom id")
	failures := fs.String("failures", "", "comma separated failure ids")
	ctxID := fs.String("context", "", "business context id")
	mode := fs.String("mode", "", "production or unknown")
	play := fs.Bool("play", false, "play the generated incident")
	_ = fs.Parse(os.Args[2:])
	scenario.BaselineDir = filepath.Join(*content, "baselines")
	grader.PolicyDir = filepath.Join(*content, "policies")
	switch cmd {
	case "validate":
		cat, err := learning.LoadCatalog(*content)
		if err != nil {
			fail(err)
		}
		problems := cat.Validate()
		for _, l := range cat.Labs {
			if l.MaxPoints() != 100 {
				problems = append(problems, fmt.Sprintf("lab %s: rubric totals %d", l.ID, l.MaxPoints()))
			}
			if strings.TrimSpace(l.Solution) == "" && l.Type != "quiz" {
				problems = append(problems, fmt.Sprintf("lab %s: missing official solution", l.ID))
			}
			if len(l.Objectives) == 0 {
				problems = append(problems, fmt.Sprintf("lab %s: no objectives", l.ID))
			}
		}
		lib, err := scenario.LoadLibrary(filepath.Join(*content, "failures"))
		if err != nil {
			problems = append(problems, "failure library: "+err.Error())
		} else {
			problems = append(problems, lib.Validate()...)
			known := cat.SkillMap()
			for _, f := range lib.Failures {
				for _, sk := range f.Skills {
					if _, ok := known[sk]; !ok {
						problems = append(problems, fmt.Sprintf("failure %s: unknown skill %s", f.ID, sk))
					}
				}
			}
		}
		if co, err := company.Load(filepath.Join(*content, "company"), "nebula"); err != nil {
			problems = append(problems, "company: "+err.Error())
		} else {
			problems = append(problems, co.Validate()...)
			known := cat.SkillMap()
			for id, m := range co.Missions {
				for _, sk := range m.Skills {
					if _, ok := known[sk]; !ok {
						problems = append(problems, fmt.Sprintf("mission %s: unknown skill %s", id, sk))
					}
				}
				if m.MaxPoints() != 100 {
					problems = append(problems, fmt.Sprintf("mission %s: rubric totals %d", id, m.MaxPoints()))
				}
			}
		}
		sort.Strings(problems)
		for _, p := range problems {
			fmt.Println("✗", p)
		}
		if len(problems) > 0 {
			os.Exit(1)
		}
		fmt.Printf("✓ %d labs, %d tracks, %d badges, %d certification blueprints\n", len(cat.Labs), len(cat.Tracks), len(cat.Badges), len(cat.Certs))
	case "i18n":
		// Spanish is the primary language: every text needs its English overlay.
		cat, err := learning.LoadCatalog(*content)
		if err != nil {
			fail(err)
		}
		problems := translationProblems(cat, *content)
		for _, p := range problems {
			fmt.Println("✗", p)
		}
		if len(problems) > 0 {
			os.Exit(1)
		}
		fmt.Println("✓ all content has its English translation")
	case "test":
		labs, err := scenario.LoadAll(filepath.Join(*content, "labs"))
		if err != nil {
			fail(err)
		}
		var ss []int64
		for _, x := range strings.Split(*seeds, ",") {
			var v int64
			fmt.Sscan(x, &v)
			ss = append(ss, v)
		}
		var reports []labtest.Report
		failed := 0
		for _, l := range labs {
			if *labID != "" && l.ID != *labID {
				continue
			}
			for _, s := range ss {
				t0 := time.Now()
				r := labtest.Verify(l, s)
				reports = append(reports, r)
				mark := "✓"
				if !r.OK {
					mark = "✗"
					failed++
				}
				fmt.Printf("%s %-42s seed=%-4d before=%3d after=%3d %5dms\n", mark, l.ID, s, r.BeforeScore, r.AfterScore, time.Since(t0).Milliseconds())
				for _, p := range r.Problems {
					fmt.Println("    -", p)
				}
				for _, f := range r.Failed {
					fmt.Println("      ·", f)
				}
			}
		}
		if *jsonOut != "" {
			b, _ := json.MarshalIndent(reports, "", "  ")
			_ = os.WriteFile(*jsonOut, b, 0o644)
		}
		fmt.Printf("\n%d runs, %d failed\n", len(reports), failed)
		if failed > 0 {
			os.Exit(1)
		}
	case "variants":
		l := loadLab(*content, *labID)
		for i := 1; i <= *n; i++ {
			_, params, err := l.Variant(int64(i), "lab-project")
			if err != nil {
				fail(err)
			}
			b, _ := json.Marshal(params)
			fmt.Printf("seed %d: %s\n", i, b)
		}
	case "play":
		playLab(loadLab(*content, *labID), *seed)
	case "curriculum":
		curriculum(*content)
	case "failures":
		lib, err := scenario.LoadLibrary(filepath.Join(*content, "failures"))
		if err != nil {
			fail(err)
		}
		for _, p := range lib.Validate() {
			fmt.Println("✗", p)
		}
		last := ""
		for _, e := range lib.SymptomGraph() {
			if e.Symptom != last {
				name := e.Symptom
				if s := lib.Symptoms[e.Symptom]; s != nil {
					name = s.Name
				}
				fmt.Printf("\n%s\n", name)
				last = e.Symptom
			}
			fmt.Printf("  ├─ %-15s %-24s %s [%s]\n", e.Layer, e.Failure, e.Title, e.System)
		}
	case "generate":
		lib, err := scenario.LoadLibrary(filepath.Join(*content, "failures"))
		if err != nil {
			fail(err)
		}
		spec := scenario.GenSpec{System: *system, Symptom: *symptom, Context: *ctxID, Difficulty: *difficulty, Mode: *mode, Seed: *seed}
		if *failures != "" {
			spec.Failures = strings.Split(*failures, ",")
		}
		l, err := lib.Generate(spec)
		if err != nil {
			fail(err)
		}
		if !*play {
			b, _ := yaml.Marshal(l)
			fmt.Print(string(b))
			return
		}
		playLab(l, *seed)
	case "stats":
		cat, err := learning.LoadCatalog(*content)
		if err != nil {
			fail(err)
		}
		byType, byBranch, byLevel := map[string]int{}, map[string]int{}, map[string]int{}
		for _, l := range cat.Labs {
			byType[l.Type]++
			byBranch[l.Branch]++
			byLevel[l.Level]++
		}
		fmt.Printf("labs: %d\nby type: %v\nby branch: %v\nby level: %v\n", len(cat.Labs), byType, byBranch, byLevel)
	case "loadtest":
		labs, err := scenario.LoadAll(filepath.Join(*content, "labs"))
		if err != nil {
			fail(err)
		}
		var mu sync.Mutex
		var prov, exec []time.Duration
		errs := 0
		sem := make(chan struct{}, *c)
		var wg sync.WaitGroup
		for i := 0; i < *n; i++ {
			wg.Add(1)
			sem <- struct{}{}
			go func(i int) {
				defer wg.Done()
				defer func() { <-sem }()
				l := labs[i%len(labs)]
				t0 := time.Now()
				w, err := scenario.Provision(l, int64(i+1), fmt.Sprintf("load-%d", i))
				d := time.Since(t0)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					errs++
					return
				}
				prov = append(prov, d)
				mu.Unlock()
				t1 := time.Now()
				w.Session.Exec("gcloud compute instances list")
				d2 := time.Since(t1)
				mu.Lock()
				exec = append(exec, d2)
			}(i)
		}
		wg.Wait()
		fmt.Printf("sessions=%d errors=%d\nprovision p50=%v p95=%v\nexec p50=%v p95=%v\n", *n, errs, pct(prov, 50), pct(prov, 95), pct(exec, 50), pct(exec, 95))
	default:
		usage()
	}
}

func pct(d []time.Duration, p int) time.Duration {
	if len(d) == 0 {
		return 0
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	return d[(len(d)-1)*p/100]
}

func loadLab(content, id string) *scenario.Lab {
	labs, err := scenario.LoadAll(filepath.Join(content, "labs"))
	if err != nil {
		fail(err)
	}
	for _, l := range labs {
		if l.ID == id {
			return l
		}
	}
	fail(fmt.Errorf("lab %q not found", id))
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

// curriculum compares the official learning-path activities (maintained in
// content/curriculum.yaml, refreshed quarterly) with internal coverage and
// prints ticket stubs for gaps.
func curriculum(content string) {
	var doc struct {
		Paths []struct {
			ID         string `yaml:"id"`
			Title      string `yaml:"title"`
			URL        string `yaml:"url"`
			Reviewed   string `yaml:"reviewed"`
			Activities []struct {
				Title string   `yaml:"title"`
				Labs  []string `yaml:"labs"`
			} `yaml:"activities"`
		} `yaml:"paths"`
	}
	b, err := os.ReadFile(filepath.Join(content, "curriculum.yaml"))
	if err != nil {
		fail(err)
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		fail(err)
	}
	cat, err := learning.LoadCatalog(content)
	if err != nil {
		fail(err)
	}
	gaps := 0
	for _, p := range doc.Paths {
		covered := 0
		var missing []string
		for _, a := range p.Activities {
			ok := len(a.Labs) > 0
			for _, l := range a.Labs {
				if cat.Labs[l] == nil {
					ok = false
				}
			}
			if ok {
				covered++
			} else {
				missing = append(missing, a.Title)
			}
		}
		age := ""
		if t, err := time.Parse("2006-01-02", p.Reviewed); err == nil && time.Since(t) > 92*24*time.Hour {
			age = " — REVIEW OVERDUE (quarterly sync)"
		}
		fmt.Printf("%s (%s): %d/%d activities covered%s\n", p.Title, p.URL, covered, len(p.Activities), age)
		for _, m := range missing {
			gaps++
			fmt.Printf("  TICKET: [curriculum-sync] %s — add or map a lab for %q\n", p.ID, m)
		}
	}
	if gaps > 0 {
		os.Exit(3)
	}
}

func playLab(l *scenario.Lab, seed int64) {
	w, err := scenario.Provision(l, seed, "lab-play-"+fmt.Sprint(seed))
	if err != nil {
		fail(err)
	}
	fmt.Printf("%s\n\n%s\n", w.Lab.Title, w.Lab.Story)
	for _, o := range w.Lab.Objectives {
		fmt.Println(" □", o)
	}
	fmt.Println("\nType commands; `:grade` to score, `:solution` to replay the official solution, `:quit` to exit.")
	in := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("student@cloudshell:~ (%s)$ ", w.Project)
		if !in.Scan() {
			return
		}
		line := in.Text()
		switch strings.TrimSpace(line) {
		case ":quit":
			return
		case ":solution":
			out, err := scenario.RunSolution(w)
			fmt.Print(out)
			if err != nil {
				fmt.Println(err)
			}
			continue
		case ":grade":
			res := grader.Grade(w.Lab, w.State, w.Session, w.Project, labtest.SampleSubmission(w.Lab))
			for _, it := range res.Items {
				fmt.Printf("  %-32s %5.1f/%d\n", it.Name, it.Earned, it.Points)
			}
			fmt.Printf("  score %d passed=%v\n", res.Score, res.Passed)
			if res.Process != nil {
				for _, f := range res.Process.Factors {
					fmt.Printf("  · %-14s %5.1f %s\n", f.Name, f.Score, strings.Join(f.Signal, "; "))
				}
			}
			continue
		}
		fmt.Print(w.Session.Exec(line).Output)
	}
}

// translationProblems lists content without its English overlay.
func translationProblems(cat *learning.Catalog, content string) []string {
	problems := cat.TranslationProblems()
	if lib, err := scenario.LoadLibrary(filepath.Join(content, "failures")); err == nil {
		problems = append(problems, lib.TranslationProblems()...)
	}
	if co, err := company.Load(filepath.Join(content, "company"), "nebula"); err == nil {
		problems = append(problems, co.TranslationProblems()...)
	}
	sort.Strings(problems)
	return slices.Compact(problems)
}
