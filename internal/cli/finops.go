package cli

import (
	"fmt"
	"strings"
)

// FinOps commands:
//
//	gcloud billing report [--group-by=service|resource|label:KEY]
//	gcloud recommender recommendations list --recommender=ID [--location=global]
func init() {
	reg("billing report", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("billing.resourceCosts.get"); err != nil {
			return nil, err
		}
		group := c.Str("group-by", "service")
		lines, total, prev := c.S.State.BillReport(p.ID, group)
		if c.Str("format", "") != "" {
			rows := []any{}
			for _, l := range lines {
				rows = append(rows, l)
			}
			return Table{Cols: []Col{{"KEY", "key"}, {"CURRENT_EUR", "currentEur"}, {"PREVIOUS_EUR", "previousEur"}, {"DELTA_EUR", "deltaEur"}, {"ANOMALY", "anomaly"}}, Rows: rows}, nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Billing report for %s (current month forecast, grouped by %s)\n\n", p.ID, group)
		if group == "service" {
			fmt.Fprintf(&b, "%-22s %12s %12s %12s\n", "SERVICE", "PREVIOUS", "CURRENT", "DELTA")
			for _, l := range lines {
				flag := ""
				if l.Anomaly {
					flag = "  ⚠ anomaly"
				}
				fmt.Fprintf(&b, "%-22s %12.2f %12.2f %+12.2f%s\n", l.Key, l.Previous, l.Current, l.Delta, flag)
			}
			fmt.Fprintf(&b, "%-22s %12.2f %12.2f %+12.2f\n", "TOTAL", prev, total, total-prev)
		} else {
			fmt.Fprintf(&b, "%-40s %12s\n", strings.ToUpper(strings.TrimPrefix(group, "label:")), "CURRENT")
			for _, l := range lines {
				fmt.Fprintf(&b, "%-40s %12.2f\n", l.Key, l.Current)
			}
			fmt.Fprintf(&b, "%-40s %12.2f\n", "TOTAL", total)
		}
		b.WriteString("\nAmounts in EUR (training price list). Budgets alert but never cap spending.\n")
		return b.String(), nil
	})
	reg("recommender recommendations list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("recommender.computeInstanceIdleResourceRecommendations.list"); err != nil {
			// viewers of the project may list recommendations too
			if err2 := c.NeedProject("compute.instances.list"); err2 != nil {
				return nil, err
			}
		}
		rec := c.Str("recommender", "")
		if rec == "" {
			return nil, fmt.Errorf("argument --recommender: Must be specified (e.g. google.compute.instance.IdleResourceRecommender)")
		}
		rs := c.S.State.Recommendations(p.ID, rec)
		rows := []any{}
		for _, r := range rs {
			rows = append(rows, r)
		}
		return Table{Cols: []Col{{"RESOURCE", "resource"}, {"SAVINGS_EUR_MONTH", "savingsEurMonthly"}, {"DESCRIPTION", "description"}, {"ACTION", "suggestedAction"}}, Rows: rows}, nil
	})
}
