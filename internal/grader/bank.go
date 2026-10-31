package grader

import (
	"fmt"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/scenario"
	"github.com/neodevesp/gcp-trainer/internal/sim"
)

// bankCheck covers the banking platform: VPC Service Controls, hybrid BGP and
// DNS forwarding to the data centre.
func (c *Context) bankCheck(typ string, ch scenario.Check) (bool, string) {
	p := c.State.Projects[c.Project]
	want := boolean(ch, "expect", true)
	switch typ {
	case "api_call":
		// A call to a Google API of this project from outside (VPC-SC).
		caller := sim.ParseFrom(str(ch, "principal"), str(ch, "from"))
		d := c.State.VPCSC(c.Project, str(ch, "service"), caller)
		if boolean(ch, "dryRun", false) {
			ok := !d.DryRun
			return ok == want, fmt.Sprintf(c.p("modo prueba: infracción=%v %s", "dry run: violation=%v %s"), d.DryRun, d.Reason)
		}
		ok := !d.Blocked
		detail := c.p("permitida", "allowed")
		if d.Blocked {
			detail = fmt.Sprintf(c.p("bloqueada por el perímetro %s (%s)", "blocked by perimeter %s (%s)"), d.Perimeter, d.Reason)
		}
		return ok == want, detail
	case "perimeter":
		per := p.Perimeters[str(ch, "name")]
		if per == nil {
			return !want, c.p("no existe el perímetro ", "no perimeter ") + str(ch, "name")
		}
		cfg := per.Status
		if boolean(ch, "dryRun", false) {
			cfg = per.Spec
		}
		if cfg == nil {
			return !want, c.p("el perímetro no tiene esa configuración", "the perimeter has no such configuration")
		}
		for _, svc := range list(ch, "restricts") {
			if !inList(cfg.RestrictedServices, svc) {
				return false, fmt.Sprintf(c.p("%s no está restringido", "%s is not restricted"), svc)
			}
		}
		if res := str(ch, "resource"); res != "" {
			found := false
			for _, r := range cfg.Resources {
				if r == "projects/"+c.Project || r == "projects/"+p.Number {
					found = true
				}
			}
			if found != (res == "self") {
				return false, c.p("el proyecto no está en el perímetro", "the project is not in the perimeter")
			}
		}
		if n := int(num(ch, "maxIngress", -1)); n >= 0 && len(cfg.IngressPolicies) > n {
			return false, fmt.Sprintf(c.p("%d reglas de entrada (máximo %d)", "%d ingress rules (max %d)"), len(cfg.IngressPolicies), n)
		}
		for _, in := range cfg.IngressPolicies {
			if boolean(ch, "noWildcardIngress", false) && (strings.EqualFold(in.IngressFrom.IdentityType, "ANY_IDENTITY") || inList(in.IngressFrom.Identities, "*")) {
				return false, c.p("hay una regla de entrada para cualquier identidad", "an ingress rule allows any identity")
			}
		}
		return want, c.p("perímetro correcto", "perimeter as expected")
	case "bgp":
		r := p.Routers[str(ch, "router")]
		if r == nil {
			return false, c.p("no existe el router ", "no router ") + str(ch, "router")
		}
		for _, ps := range c.State.BGPStatus(c.Project, r) {
			if str(ch, "peer") == "" || ps.Name == str(ch, "peer") {
				up := ps.Status == "UP"
				return up == want, fmt.Sprintf("%s %s %s", ps.Name, ps.State, ps.Reason)
			}
		}
		return false, c.p("el router no tiene sesiones BGP", "the router has no BGP sessions")
	case "dns_resolve":
		from, _, err := c.endpoint(str(ch, "from"))
		if err != nil {
			return false, err.Error()
		}
		ip, ok := c.State.ResolveHost(str(ch, "host"), from)
		if exp := str(ch, "ip"); exp != "" && ok {
			ok = ip == exp
		}
		return ok == want, fmt.Sprintf("%s -> %s", str(ch, "host"), firstNonEmpty(ip, "NXDOMAIN/SERVFAIL"))
	}
	return false, "?"
}

func inList(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
