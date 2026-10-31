package cli

import (
	"strings"
	"testing"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

func TestWorkspaceDirectory(t *testing.T) {
	s := newTestSession()
	run(t, s, "gam create org /Ingenieria description Ingenieria")
	run(t, s, "gam create user ana@zentra.dev firstname Ana lastname Ruiz org /Ingenieria title SRE")
	run(t, s, "gam create user luis@zentra.dev firstname Luis lastname Gil org /Ingenieria")
	run(t, s, "gcloud identity groups create sre@zentra.dev --organization=zentra.dev --display-name=SRE")
	run(t, s, "gcloud identity groups create plataforma@zentra.dev --organization=zentra.dev --display-name=Plataforma")
	run(t, s, "gcloud identity groups memberships add --group-email=sre@zentra.dev --member-email=ana@zentra.dev")
	// nested: sre is a member of plataforma
	run(t, s, "gcloud identity groups memberships add --group-email=plataforma@zentra.dev --member-email=sre@zentra.dev")
	run(t, s, "gcloud projects add-iam-policy-binding lab-proj --member=group:plataforma@zentra.dev --role=roles/viewer")

	res := sim.ProjectResource("lab-proj")
	if !s.State.Allowed("user:ana@zentra.dev", "compute.instances.list", res) {
		t.Fatal("ana should inherit viewer through nested groups")
	}
	if s.State.Allowed("user:luis@zentra.dev", "compute.instances.list", res) {
		t.Fatal("luis is not in any group")
	}
	out := run(t, s, "gcloud identity groups memberships check-transitive-membership --group-email=plataforma@zentra.dev --member-email=ana@zentra.dev")
	if !strings.Contains(out, "true") {
		t.Fatal("transitive membership expected")
	}

	// external members are rejected unless the group allows them
	out = run(t, s, "gcloud identity groups memberships add --group-email=sre@zentra.dev --member-email=hacker@gmail.com")
	if !strings.Contains(out, "outside the organization") {
		t.Fatal("external member should be rejected")
	}
	run(t, s, "gam update group sre@zentra.dev allowexternalmembers true")
	run(t, s, "gam update group sre@zentra.dev add member user partner@consultora.com")
	if !strings.Contains(run(t, s, "gam info group sre@zentra.dev"), "(external)") {
		t.Fatal("external member should be flagged")
	}

	// suspending a user removes all their access
	run(t, s, "gam update user ana@zentra.dev suspended on")
	if s.State.Allowed("user:ana@zentra.dev", "compute.instances.list", res) {
		t.Fatal("suspended user must lose access")
	}
	out = run(t, s, "gam report admin")
	if !strings.Contains(out, "suspendUser") || !strings.Contains(out, "addGroupMember") {
		t.Fatal("admin audit report should list the changes")
	}
	if !strings.Contains(run(t, s, "gam info user ana@zentra.dev"), "Account Suspended: true") {
		t.Fatal("info user should show suspension")
	}
	if !strings.Contains(run(t, s, "gam print groups member ana@zentra.dev"), "plataforma@zentra.dev") {
		t.Fatal("nested group membership should be listed")
	}

	// the grader view exposes the directory with transitive members
	v := s.State.ProjectView("lab-proj")
	d, _ := v["directory"].(map[string]any)
	g := d["groups"].(map[string]any)["plataforma@zentra.dev"].(map[string]any)
	if !strings.Contains(strings.Join(toStrings(g["allMembers"]), ","), "ana@zentra.dev") {
		t.Fatalf("allMembers should include ana: %v", g["allMembers"])
	}
}

func toStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}
