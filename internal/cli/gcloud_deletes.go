package cli

import (
	"fmt"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

// Delete commands used by the console for resources created elsewhere.

func deleter[T any](what, perm string, pick func(p *sim.Project) map[string]T) handler {
	return func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.Arg(0, "NAME")
		if err != nil {
			return nil, err
		}
		n = n[strings.LastIndex(n, "/")+1:]
		m := pick(p)
		if _, ok := m[n]; !ok {
			return nil, fmt.Errorf("NOT_FOUND: %s [%s] not found", what, n)
		}
		if err := c.NeedProject(perm); err != nil {
			return nil, err
		}
		delete(m, n)
		return "Deleted " + what + " [" + n + "].\n", nil
	}
}

func init() {
	reg("monitoring uptime delete", deleter("uptime check", "monitoring.uptimeCheckConfigs.delete", func(p *sim.Project) map[string]*sim.UptimeCheck { return p.UptimeChecks }))
	reg("logging sinks delete", deleter("sink", "logging.sinks.delete", func(p *sim.Project) map[string]*sim.LogSink { return p.LogSinks }))
	reg("billing budgets delete", deleter("budget", "billing.budgets.delete", func(p *sim.Project) map[string]*sim.Budget { return p.Budgets }))
	reg("source repos delete", deleter("repository", "source.repos.delete", func(p *sim.Project) map[string]*sim.SourceRepo { return p.SourceRepos }))
	reg("compute security-policies delete", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		n, err := c.Arg(0, "NAME")
		if err != nil {
			return nil, err
		}
		for _, bs := range p.BackendServices {
			if bs.SecurityPolicy == n {
				return nil, fmt.Errorf("The security policy resource '%s' is already being used by backend service '%s'.", n, bs.Name)
			}
		}
		return deleter("security policy", "compute.securityPolicies.delete", func(p *sim.Project) map[string]*sim.SecurityPolicy { return p.SecurityPolicies })(c)
	})
}
