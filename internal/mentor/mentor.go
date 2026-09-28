// Package mentor provides feedback that never replaces deterministic grading:
// a Socratic mentor derived from failing validators, and an optional LLM
// reviewer for postmortems (enabled only when ANTHROPIC_API_KEY or another
// Anthropic credential is configured and MENTOR_LLM=on).
package mentor

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/neodevesp/gcp-trainer/internal/grader"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

// Model is the Claude model used for optional postmortem reviews.
const Model = "claude-opus-5-5"

var socratic = map[string][]string{
	"tcp":             {"From which exact source to which destination and port does the traffic need to flow?", "Which firewall rules apply to the destination's tags or service account, and which one has the lowest priority number?", "Can you prove the path with `nc -zv` from the source VM?"},
	"http":            {"What does the request return right now, and from which layer (load balancer, service, application)?", "Which component would produce that status code?", "Did the logs show an application error or a network timeout?"},
	"egress":          {"How does a VM without an external IP reach the internet?", "Which region and subnet does the VM use, and what covers that subnet?"},
	"google_api":      {"Which paths exist to reach *.googleapis.com from a private subnet?"},
	"iam":             {"Which identity is the workload really using?", "Which is the smallest role containing the permission in the error message?", "At which level (resource, project) should it be granted?"},
	"no_basic_roles":  {"Would a predefined role cover the task without Owner/Editor?"},
	"forbid_firewall": {"Is any rule broader than needed? Who really needs to reach that port?"},
	"policy":          {"Run a security review: what is exposed to 0.0.0.0/0 or allUsers?"},
	"evidence":        {"Explain the causal chain: symptom → evidence → cause → fix → verification → prevention."},
	"cost_max":        {"Which line of the cost estimate dominates? Is it needed for the requirement?"},
	"log_metric":      {"Which exact filter isolates the errors you saw in Logs Explorer?"},
	"alert_policy":    {"Does the alert fire on a user-visible symptom, and who gets notified?"},
	"k8s_ready":       {"What does `kubectl describe pod` say in Events?", "Does the probe port match the container port?"},
	"hpa_scaled":      {"Can the HPA compute utilisation without CPU requests?", "Did you generate load and then stop it?"},
	"terraform_clean": {"Does a second `terraform plan` show no changes?"},
	"bq_bytes_max":    {"Which columns and partitions does your query really need? Try `--dry_run`."},
	"build":           {"Which identity runs the build, and can it push to the repository?"},
	"sql_healthy":     {"How many connections does each instance open, and what is max_connections?"},
	"pubsub":          {"What happens to a message that fails delivery repeatedly?"},
	"exists":          {"Compare the resource you created with each stated requirement (location, flags, names)."},
}

// Socratic returns guiding questions for the validators that are failing,
// without revealing expected values.
func Socratic(lab *scenario.Lab, res *grader.Result) []string {
	seen := map[string]bool{}
	var out []string
	for _, it := range res.Items {
		for _, c := range it.Checks {
			if c.Pass || seen[c.Type] {
				continue
			}
			seen[c.Type] = true
			if qs, ok := socratic[c.Type]; ok {
				out = append(out, fmt.Sprintf("%s — %s", it.Name, qs[len(out)%len(qs)]))
			}
		}
	}
	if len(out) == 0 {
		out = append(out, "Everything checks out. Can you explain why your fix is the minimal one?")
	}
	sort.Strings(out)
	return out
}

// Enabled reports whether the optional LLM reviewer is configured.
func Enabled() bool {
	return os.Getenv("MENTOR_LLM") == "on"
}

// ReviewPostmortem asks Claude for qualitative feedback on a postmortem. The
// technical score always comes from the deterministic grader.
func ReviewPostmortem(ctx context.Context, lab *scenario.Lab, evidence map[string]string) (string, error) {
	if !Enabled() {
		return "", fmt.Errorf("LLM mentor disabled")
	}
	client := anthropic.NewClient()
	var b strings.Builder
	for _, k := range []string{"rootCause", "fix", "verification", "prevention", "postmortem", "explanation"} {
		if v := evidence[k]; v != "" {
			fmt.Fprintf(&b, "## %s\n%s\n\n", k, v)
		}
	}
	system := "You are a senior SRE mentoring a learner on Google Cloud. Review the postmortem for causal clarity, evidence, verification and prevention. Ask at most three Socratic questions and give at most three concrete improvements. Never give a score and never reveal a full solution."
	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     Model,
		MaxTokens: 16000,
		System:    []anthropic.TextBlockParam{{Text: system}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(fmt.Sprintf("Lab: %s\nStory:\n%s\n\nPostmortem:\n%s", lab.Title, lab.Story, b.String()))),
		},
	})
	if err != nil {
		return "", err
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return "", fmt.Errorf("the reviewer declined this request")
	}
	var out strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			out.WriteString(t.Text)
		}
	}
	return out.String(), nil
}
