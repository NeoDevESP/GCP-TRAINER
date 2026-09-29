package sim

import "fmt"

// "Teach me why" (Blueprint §13): the request path is instrumented so that a
// single request can be explained as a causal chain
//
//	DNS → IP → TCP → TLS → load balancer → firewall → service → workload →
//	application → dependencies (database, APIs, IAM)
//
// Recording is opt-in and never changes behaviour.

// Hop is one link of the causal chain.
type Hop struct {
	Layer  string `json:"layer"`
	Status string `json:"status"` // ok, fail, info
	Detail string `json:"detail"`
}

func (s *State) note(layer, status, format string, a ...any) {
	if s.explain == nil {
		return
	}
	*s.explain = append(*s.explain, Hop{Layer: layer, Status: status, Detail: fmt.Sprintf(format, a...)})
}

// ExplainHTTP executes a request and returns the causal chain it followed.
func (s *State) ExplainHTTP(req HTTPRequest) (HTTPResponse, []Hop) {
	var hops []Hop
	prev := s.explain
	s.explain = &hops
	defer func() { s.explain = prev }()
	r := s.HTTP(req)
	switch {
	case r.Error != "":
		s.note("result", "fail", "%s", r.Error)
	case r.Status >= 400:
		s.note("result", "fail", "HTTP %d: %s", r.Status, firstLine(r.Body))
	default:
		s.note("result", "ok", "HTTP %d", r.Status)
	}
	return r, hops
}

func firstLine(s string) string {
	for i := range s {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}

// RegionDown reports a simulated regional outage (DR drills, chaos).
func (s *State) RegionDown(region string) bool {
	return region != "" && s.Extra["outage:"+region] == "down"
}

// ZoneDown reports a zonal outage (explicit zone or its region).
func (s *State) ZoneDown(zone string) bool {
	return zone != "" && (s.Extra["outage:"+zone] == "down" || s.RegionDown(RegionOf(zone)))
}
