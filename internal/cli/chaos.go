package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

// ChaosRun records one chaos experiment (Blueprint §12, Chaos Engineering).
type ChaosRun struct {
	Experiment   string   `json:"experiment"`
	Target       string   `json:"target"`
	Hypothesis   string   `json:"hypothesis"`
	SLO          float64  `json:"slo"`
	Minutes      int      `json:"minutes"`
	Availability float64  `json:"availability"`
	Held         bool     `json:"held"`
	Aborted      bool     `json:"aborted"`
	Timeline     []string `json:"timeline"`
	At           string   `json:"at"`
}

const chaosUsage = `usage: chaos run EXPERIMENT --target=lb:NAME|run:NAME [--path=/] [--slo=99] [--duration=10] [--abort-below=50]
experiments:
  zone-outage --zone=ZONE          all resources in the zone become unavailable
  region-outage --region=REGION    the whole region becomes unavailable
  kill-instance --instance=NAME    the VM is stopped (does something bring it back?)
  stop-service --instance=NAME --service=SVC   a process on a VM stops
Steady state is measured before the fault; the hypothesis holds if the target
keeps at least the SLO availability during the experiment. The fault is
reverted at the end (instances stay as the platform left them).
  chaos history                    list past experiments`

func (s *Session) chaosCmd(args []string) (string, error) {
	pos, f := parseArgs(args[1:])
	if len(pos) == 0 {
		return chaosUsage + "\n", nil
	}
	if pos[0] == "history" {
		if len(s.Chaos) == 0 {
			return "No experiments yet.\n", nil
		}
		var b strings.Builder
		for _, r := range s.Chaos {
			v := "HELD"
			if !r.Held {
				v = "FAILED"
			}
			fmt.Fprintf(&b, "%s  %-14s %-22s availability %6.2f%% (SLO %.1f%%) %s\n", r.At, r.Experiment, r.Target, r.Availability, r.SLO, v)
		}
		return b.String(), nil
	}
	if pos[0] != "run" || len(pos) < 2 {
		return "", fail(2, "%s", chaosUsage)
	}
	exp := pos[1]
	target := firstOr(f["target"], "")
	if target == "" {
		return "", fail(2, "--target is required (e.g. --target=lb:checkout-lb)")
	}
	path := firstOr(f["path"], "/")
	slo, _ := strconv.ParseFloat(firstOr(f["slo"], "99"), 64)
	minutes, _ := strconv.Atoi(firstOr(f["duration"], "10"))
	if minutes <= 0 || minutes > 60 {
		minutes = 10
	}
	abort, _ := strconv.ParseFloat(firstOr(f["abort-below"], "0"), 64)
	st := s.State
	p := st.Projects[s.Project]
	if p == nil {
		return "", fail(1, "no project configured")
	}
	probeOnce := func() bool {
		url := st.ResolveTrafficURL(s.Project, target, path)
		if url == "" {
			return false
		}
		r := st.HTTP(sim.HTTPRequest{URL: url, SourceIP: sim.StudentIP})
		return r.Status > 0 && r.Status < 500
	}
	run := ChaosRun{Experiment: exp, Target: target + path, SLO: slo, Minutes: minutes, At: st.Now()}
	// steady state
	if !probeOnce() {
		return "", fail(1, "steady state not met: %s%s is already failing — fix it before experimenting", target, path)
	}
	var inject, revert func()
	switch exp {
	case "zone-outage", "region-outage":
		key := firstOr(f["zone"], "")
		if exp == "region-outage" {
			key = firstOr(f["region"], "")
		}
		if key == "" {
			return "", fail(2, "--zone / --region is required")
		}
		run.Hypothesis = fmt.Sprintf("%s%s stays ≥%.1f%% available while %s is down", target, path, slo, key)
		inject = func() { st.Extra["outage:"+key] = "down" }
		revert = func() { delete(st.Extra, "outage:"+key) }
	case "kill-instance":
		vm := p.Instances[firstOr(f["instance"], "")]
		if vm == nil {
			return "", fail(1, "instance %q not found", firstOr(f["instance"], ""))
		}
		run.Hypothesis = fmt.Sprintf("%s%s stays ≥%.1f%% available when %s dies", target, path, slo, vm.Name)
		inject = func() { vm.Status = "TERMINATED" }
		revert = func() {}
	case "stop-service":
		vmName, svc := firstOr(f["instance"], ""), firstOr(f["service"], "")
		if p.Instances[vmName] == nil || svc == "" {
			return "", fail(2, "--instance and --service are required")
		}
		key := fmt.Sprintf("svc:%s/%s/%s", s.Project, vmName, svc)
		run.Hypothesis = fmt.Sprintf("%s%s stays ≥%.1f%% available when %s stops on %s", target, path, slo, svc, vmName)
		inject = func() { st.Extra[key] = "stopped" }
		revert = func() { delete(st.Extra, key) }
	default:
		return "", fail(2, "unknown experiment %q\n%s", exp, chaosUsage)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Hypothesis: %s\nSteady state: OK\nInjecting %s at %s...\n", run.Hypothesis, exp, st.Now())
	inject()
	st.Audit(s.Project, s.Principal(), "chaos.gcplab.dev", "InjectFault", exp)
	ok := 0
	for m := 1; m <= minutes; m++ {
		st.Step(1)
		// requests during the minute (several probes smooth out round-robin effects)
		good := 0
		for i := 0; i < 4; i++ {
			if probeOnce() {
				good++
			}
			st.Tick++
		}
		ok += good
		line := fmt.Sprintf("t+%02dm  %d/4 requests OK", m, good)
		run.Timeline = append(run.Timeline, line)
		fmt.Fprintf(&b, "  %s\n", line)
		avail := 100 * float64(ok) / float64(4*m)
		if abort > 0 && avail < abort {
			run.Aborted = true
			fmt.Fprintf(&b, "  ABORT: availability %.1f%% below the abort threshold %.1f%%\n", avail, abort)
			run.Minutes = m
			break
		}
	}
	revert()
	st.Step(1)
	run.Availability = 100 * float64(ok) / float64(4*run.Minutes)
	run.Held = run.Availability >= slo && !run.Aborted
	s.Chaos = append(s.Chaos, run)
	verdict := "HELD ✓"
	if !run.Held {
		verdict = "FAILED ✗ — the system is not resilient to this failure yet"
	}
	fmt.Fprintf(&b, "Fault reverted.\nAvailability during the experiment: %.2f%% (SLO %.1f%%) → hypothesis %s\n", run.Availability, slo, verdict)
	return b.String(), nil
}
