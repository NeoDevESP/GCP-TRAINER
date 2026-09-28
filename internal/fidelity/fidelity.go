// Package fidelity implements the three interchangeable execution layers and
// the router that picks one per exercise:
//
//	F0 Simulator  – deterministic state engine (internal/sim)
//	F1 Emulator   – real API semantics from official emulators / OSS (Pub/Sub,
//	                fake-gcs-server, kind) mirrored into the simulator for grading
//	F2 Real GCP   – an ephemeral project leased from a pre-provisioned pool
package fidelity

import (
	"fmt"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/cli"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

// Level identifies a fidelity layer.
type Level string

const (
	F0 Level = "F0"
	F1 Level = "F1"
	F2 Level = "F2"
)

// Env is a provisioned lab environment at some fidelity.
type Env struct {
	Level   Level
	World   *scenario.World // always present: F0 state or F1/F2 mirror used for grading & views
	Lease   *Lease          // F2 only
	Cleanup func() error
}

// Runtime provisions and tears down environments at one fidelity.
type Runtime interface {
	Level() Level
	Available() (bool, string)
	Provision(l *scenario.Lab, seed int64, projectID, userID string) (*Env, error)
}

// Router selects the runtime for a lab request.
type Router struct {
	Runtimes map[Level]Runtime
	// F2Quota limits real-cloud sessions per user per month (cost control).
	F2Quota func(userID string) (bool, string)
}

// NewRouter builds a router with the F0 runtime always available.
func NewRouter(rts ...Runtime) *Router {
	r := &Router{Runtimes: map[Level]Runtime{}}
	r.Runtimes[F0] = F0Runtime{}
	for _, rt := range rts {
		r.Runtimes[rt.Level()] = rt
	}
	return r
}

// Decision explains the router's choice.
type Decision struct {
	Level  Level  `json:"level"`
	Reason string `json:"reason"`
}

// Choose returns the fidelity to use: the requested one if the lab supports
// it and it is available, otherwise the lab default, otherwise F0.
func (r *Router) Choose(l *scenario.Lab, requested string, userID string) (Decision, error) {
	supported := map[Level]bool{}
	for _, s := range l.Fidelity.Supported {
		supported[Level(strings.ToUpper(s))] = true
	}
	want := Level(strings.ToUpper(requested))
	if want == "" {
		want = Level(strings.ToUpper(l.Fidelity.Default))
	}
	if !supported[want] {
		return Decision{}, fmt.Errorf("lab %s does not support fidelity %s (supported: %v)", l.ID, want, l.Fidelity.Supported)
	}
	rt := r.Runtimes[want]
	if rt == nil {
		return r.fallback(l, want, "runtime not configured on this platform")
	}
	if ok, why := rt.Available(); !ok {
		return r.fallback(l, want, why)
	}
	if want == F2 && r.F2Quota != nil {
		if ok, why := r.F2Quota(userID); !ok {
			return r.fallback(l, want, why)
		}
	}
	return Decision{Level: want, Reason: "requested"}, nil
}

func (r *Router) fallback(l *scenario.Lab, want Level, why string) (Decision, error) {
	for _, lv := range []Level{F1, F0} {
		if lv == want {
			continue
		}
		for _, s := range l.Fidelity.Supported {
			if Level(strings.ToUpper(s)) == lv {
				if rt := r.Runtimes[lv]; rt != nil {
					if ok, _ := rt.Available(); ok {
						return Decision{Level: lv, Reason: fmt.Sprintf("%s unavailable (%s); fell back to %s", want, why, lv)}, nil
					}
				}
			}
		}
	}
	return Decision{}, fmt.Errorf("fidelity %s unavailable: %s", want, why)
}

// Provision creates the environment with the chosen runtime.
func (r *Router) Provision(d Decision, l *scenario.Lab, seed int64, projectID, userID string) (*Env, error) {
	return r.Runtimes[d.Level].Provision(l, seed, projectID, userID)
}

// F0Runtime is the simulator.
type F0Runtime struct{}

func (F0Runtime) Level() Level              { return F0 }
func (F0Runtime) Available() (bool, string) { return true, "" }
func (F0Runtime) Provision(l *scenario.Lab, seed int64, projectID, userID string) (*Env, error) {
	w, err := scenario.Provision(l, seed, projectID)
	if err != nil {
		return nil, err
	}
	return &Env{Level: F0, World: w}, nil
}

// chainInterceptors composes interceptors (first handler wins).
func chainInterceptors(fs ...func(*cli.Session, []string, string) (bool, string, error)) func(*cli.Session, []string, string) (bool, string, error) {
	return func(s *cli.Session, args []string, stdin string) (bool, string, error) {
		for _, f := range fs {
			if f == nil {
				continue
			}
			if ok, out, err := f(s, args, stdin); ok {
				return ok, out, err
			}
		}
		return false, "", nil
	}
}
