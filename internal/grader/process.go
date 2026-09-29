package grader

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/cli"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

// Process assessment (Blueprint §9): the platform scores the *outcome* (the
// 100-point rubric) and the *process*. Solving an incident by restarting
// resources at random must not be worth the same as solving it through
// hypotheses and evidence. The process report never changes the rubric score;
// it feeds the student model dimensions, the autonomy ladder and feedback.

// Factor is one professional-assessment factor (0..100).
type Factor struct {
	Name   string   `json:"name"`
	Score  float64  `json:"score"`
	Signal []string `json:"signals,omitempty"`
}

// ProcessReport scores how the student worked.
type ProcessReport struct {
	Factors     []Factor `json:"factors"`
	Overall     float64  `json:"overall"`
	Commands    int      `json:"commands"`
	ReadOnly    int      `json:"readOnlyCommands"`
	Mutating    int      `json:"mutatingCommands"`
	Destructive []string `json:"destructiveCommands,omitempty"`
	RiskyGrants []string `json:"riskyGrants,omitempty"`
	BlindFixes  []string `json:"blindFixes,omitempty"` // changes made before any evidence was gathered
	Errors      int      `json:"errors"`
	Violations  []string `json:"constraintViolations,omitempty"`
}

var (
	reReadOnly    = regexp.MustCompile(`(^|\s)(list|describe|get-iam-policy|get-health|get-serial-port-output|read|logs|troubleshoot-policy|search-all-resources|findings|estimate|get|top|explain|events|status|plan|show|validate|dry_run|dry-run|print-access-token|get-value|ls|cat|versions list)(\s|$)|^(curl|dig|nslookup|host|nc|ping|psql|whoami|history|help|jq|kubectl (get|describe|logs|top|explain)|gcloud (config list|info|auth list)|bq (show|ls|query --dry_run))`)
	reDestructive = regexp.MustCompile(`(^|\s)(delete|destroy|reset|stop|disable|remove-iam-policy-binding|drain|rollback|failover|restart|delete-access-config|rm)(\s|$)|--force\b|terraform (destroy|apply -replace)|kubectl (delete|drain|rollout restart)|gsutil (rm|-m rm)`)
	reRestart     = regexp.MustCompile(`instances (reset|stop|start)|rollout restart|sql instances restart|services update .*--update-labels=restart|systemctl restart|delete pod`)
	reRiskyGrant  = regexp.MustCompile(`(roles/(owner|editor)\b|--member=allUsers|--member=allAuthenticatedUsers|--source-ranges=0\.0\.0\.0/0.*(tcp:22|tcp:3389|tcp:5432|tcp:3306|all)|--allow=all|--scopes=cloud-platform.*--service-account=\S*-compute@)`)
)

// isReadOnly reports whether a command line only observes state.
func isReadOnly(line string) bool {
	l := strings.TrimSpace(line)
	if l == "" {
		return true
	}
	first := strings.Fields(l)[0]
	switch first {
	case "why", "whatif", "ticket", "team", "ask", "arch", "interview", "answer":
		return true
	case "echo", "printf", "export", "unset", "sleep", "cd", "pwd", "date", "env", "printenv", "cat", "ls", "grep", "head", "tail", "wc", "sort", "uniq", "awk", "cut", "base64", "openssl", "uuidgen", "watch", "help", "history", "whoami", "hostname", "jq", "dig", "nslookup", "host", "ping", "nc", "ncat", "telnet":
		return !strings.Contains(l, ">")
	}
	if strings.Contains(l, "config set") || strings.Contains(l, "auth login") || strings.Contains(l, "get-credentials") {
		return true // context configuration is not a change to the environment
	}
	return reReadOnly.MatchString(l) && !reDestructive.MatchString(l) && !strings.Contains(l, " create") && !strings.Contains(l, " update") && !strings.Contains(l, " add-")
}

// AssessProcess analyses the command history, the rubric result and the
// submission. hints is the number of hints consumed.
func AssessProcess(lab *scenario.Lab, sess *cli.Session, res *Result, sub Submission, hints int) *ProcessReport {
	pr := &ProcessReport{}
	evidenceBefore := 0
	for _, r := range sess.Records {
		line := r.Line
		if line == "" {
			continue
		}
		pr.Commands++
		if r.Exit != 0 {
			pr.Errors++
		}
		if isReadOnly(line) {
			pr.ReadOnly++
			evidenceBefore++
			continue
		}
		pr.Mutating++
		if reDestructive.MatchString(line) {
			pr.Destructive = append(pr.Destructive, line)
		}
		if reRiskyGrant.MatchString(line) {
			pr.RiskyGrants = append(pr.RiskyGrants, line)
		}
		if reRestart.MatchString(line) && evidenceBefore < 2 {
			pr.BlindFixes = append(pr.BlindFixes, line)
		} else if evidenceBefore == 0 && isIncident(lab) {
			pr.BlindFixes = append(pr.BlindFixes, line)
		}
		for _, c := range lab.Constraints {
			if v := constraintViolated(c, line); v != "" {
				pr.Violations = append(pr.Violations, v)
			}
		}
	}
	share := func(validators ...string) (float64, bool) {
		var e, m float64
		for _, v := range validators {
			x := res.Validators[v]
			e += x[0]
			m += x[1]
		}
		if m == 0 {
			return 0, false
		}
		return 100 * e / m, true
	}
	add := func(name string, score float64, sig ...string) {
		pr.Factors = append(pr.Factors, Factor{Name: name, Score: clamp(score), Signal: sig})
	}
	// Resolution: does the service meet its objective again?
	if v, ok := share("functional", "state"); ok {
		add("resolution", v)
	} else {
		add("resolution", float64(res.Score))
	}
	// Diagnosis: evidence gathered before acting + diagnosis/evidence rubric.
	diag := 0.0
	if pr.Commands > 0 {
		diag = 100 * math.Min(1, float64(pr.ReadOnly)/math.Max(3, float64(pr.Mutating)))
	}
	if v, ok := share("diagnosis", "evidence"); ok {
		diag = 0.5*diag + 0.5*v
	}
	sig := []string{fmt.Sprintf("%d read-only / %d mutating commands", pr.ReadOnly, pr.Mutating)}
	if len(pr.BlindFixes) > 0 {
		diag -= 15 * float64(len(pr.BlindFixes))
		sig = append(sig, fmt.Sprintf("%d change(s) before gathering evidence", len(pr.BlindFixes)))
	}
	add("diagnosis", diag, sig...)
	// Security
	sec := 100.0
	if v, ok := share("security"); ok {
		sec = v
	}
	if res.CriticalFailed {
		sec = math.Min(sec, 30)
	}
	sec -= 20 * float64(len(pr.RiskyGrants))
	add("security", sec, pluralSig(len(pr.RiskyGrants), "over-broad grant or exposure command"))
	// Cost
	cost := 100.0
	if v, ok := share("cost"); ok {
		cost = v
	}
	add("cost", cost)
	// Efficiency: commands vs reference solution, errors and hints.
	ref := 0
	for _, l := range strings.Split(lab.Solution, "\n") {
		if strings.TrimSpace(l) != "" {
			ref++
		}
	}
	eff := 100.0
	if ref > 0 && pr.Commands > 3*ref {
		eff -= math.Min(40, float64(pr.Commands-3*ref)*2)
	}
	if pr.Commands > 0 {
		eff -= 40 * float64(pr.Errors) / float64(pr.Commands)
	}
	eff -= 5 * float64(hints)
	add("efficiency", eff, fmt.Sprintf("%d commands (reference %d), %d errors, %d hints", pr.Commands, ref, pr.Errors, hints))
	// Risk: destructive changes, blind restarts and constraint violations.
	risk := 100 - 10*float64(len(pr.Destructive)) - 20*float64(len(pr.Violations))
	add("risk", risk, pluralSig(len(pr.Destructive), "destructive command"), pluralSig(len(pr.Violations), "constraint violation"))
	// Communication: incident updates / ticket comments.
	comm := textQuality(sub.Evidence, []string{"update", "communication", "ticket", "status", "impact"})
	if d := sess.Desk; d != nil {
		updates := 0
		for _, cm := range d.StudentComments("student") {
			if len(strings.Fields(cm.Text)) >= 6 {
				updates++
			}
		}
		asked := 0
		for _, q := range d.Questions {
			if q.Useful {
				asked++
			}
		}
		deskScore := math.Min(100, 35*float64(updates)+15*float64(asked))
		if d.Ticket != nil && d.Ticket.Status == "RESOLVED" {
			deskScore = math.Min(100, deskScore+30)
		}
		comm = math.Max(comm, deskScore)
	}
	add("communication", comm)
	// Documentation: postmortem / runbook quality.
	add("documentation", textQuality(sub.Evidence, []string{"rootCause", "prevention", "postmortem", "runbook", "explanation", "architecture", "fix"}))
	// Autonomy: minimal assistance.
	add("autonomy", 100-20*float64(hints))
	// drop empty signals
	for i := range pr.Factors {
		var s []string
		for _, x := range pr.Factors[i].Signal {
			if x != "" {
				s = append(s, x)
			}
		}
		pr.Factors[i].Signal = s
	}
	weights := map[string]float64{"resolution": 0.25, "diagnosis": 0.15, "security": 0.15, "cost": 0.05, "efficiency": 0.1, "risk": 0.1, "communication": 0.05, "documentation": 0.05, "autonomy": 0.1}
	for _, f := range pr.Factors {
		pr.Overall += weights[f.Name] * f.Score
	}
	pr.Overall = math.Round(pr.Overall*10) / 10
	return pr
}

func isIncident(l *scenario.Lab) bool {
	return l.Type == "incident" || l.Type == "boss"
}

// constraintViolated checks declarative lab constraints against a command.
// Supported: "no-downtime" (no stop/delete/reset of serving resources),
// "no-delete", "no-public" (no allUsers grants), "no-basic-roles",
// "protect:<name>" (resource must not be modified).
func constraintViolated(c, line string) string {
	c = strings.TrimSpace(c)
	switch {
	case c == "no-downtime":
		if regexp.MustCompile(`instances (stop|delete|reset)|services delete|sql instances (restart|delete)|clusters delete|forwarding-rules delete|kubectl delete (deploy|deployment|svc|service)`).MatchString(line) {
			return "no-downtime: " + line
		}
	case c == "no-delete":
		if regexp.MustCompile(`(^|\s)(delete|destroy)(\s|$)|\brm\b`).MatchString(line) {
			return "no-delete: " + line
		}
	case c == "no-public":
		if strings.Contains(line, "allUsers") || strings.Contains(line, "allAuthenticatedUsers") {
			if !strings.Contains(line, "remove-iam-policy-binding") {
				return "no-public: " + line
			}
		}
	case c == "no-basic-roles":
		if regexp.MustCompile(`roles/(owner|editor|viewer)\b`).MatchString(line) && strings.Contains(line, "add-iam-policy-binding") {
			return "no-basic-roles: " + line
		}
	case strings.HasPrefix(c, "protect:"):
		name := strings.TrimPrefix(c, "protect:")
		if !isReadOnly(line) && regexp.MustCompile(`(^|[\s/=])`+regexp.QuoteMeta(name)+`(\s|$|[,:])`).MatchString(line) {
			return "protected resource " + name + " modified: " + line
		}
	}
	return ""
}

func textQuality(ev map[string]string, fields []string) float64 {
	words := 0
	present := 0
	for _, f := range fields {
		for k, v := range ev {
			if strings.EqualFold(k, f) && strings.TrimSpace(v) != "" {
				present++
				words += len(strings.Fields(v))
			}
		}
	}
	if present == 0 {
		return 0
	}
	return math.Min(100, float64(words)*4+float64(present)*10)
}

func pluralSig(n int, what string) string {
	if n == 0 {
		return ""
	}
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

func clamp(v float64) float64 {
	return math.Round(math.Max(0, math.Min(100, v))*10) / 10
}
