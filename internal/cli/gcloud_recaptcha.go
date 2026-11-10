package cli

import (
	"crypto/sha1"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

// reCAPTCHA (Enterprise) site keys: gcloud recaptcha keys create, list,
// describe, update and delete. The usual support request is "add this
// domain to our key": the domain goes without https:// or a path, and
// --domains on update replaces the whole list (the console adds one).

const keyChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

// newKeyID makes a 40-character id like the real ones ("6L…").
func newKeyID(project, name string, n int) string {
	h := sha1.Sum([]byte(fmt.Sprintf("%s/%s/%d", project, name, n)))
	var b strings.Builder
	b.WriteString("6L")
	for i := 0; b.Len() < 40; i++ {
		b.WriteByte(keyChars[int(h[i%len(h)]+byte(i*7))%len(keyChars)])
	}
	return b.String()
}

var reDomain = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z0-9]([a-z0-9-]*[a-z0-9])?$|^localhost$`)

// domains validates a domain list as the API does.
func recaptchaDomains(list []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, d := range list {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		if strings.Contains(d, "://") || strings.ContainsAny(d, "/:?#") || !reDomain.MatchString(d) {
			return nil, fmt.Errorf("INVALID_ARGUMENT: Invalid domain %q: use only the domain name, without https:// or a path (e.g. %s).", d, hostOnly(d))
		}
		if len(out) >= 250 {
			return nil, fmt.Errorf("INVALID_ARGUMENT: A key allows at most 250 domains.")
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out, nil
}

func hostOnly(d string) string {
	if i := strings.Index(d, "://"); i >= 0 {
		d = d[i+3:]
	}
	if i := strings.IndexAny(d, "/:?#"); i >= 0 {
		d = d[:i]
	}
	return d
}

func recaptchaRes(p *sim.Project, id string) sim.Resource {
	return sim.Resource{Project: p.ID, Type: "recaptchaenterprise.googleapis.com/Key", Name: "projects/" + p.ID + "/keys/" + id, Service: "recaptchaenterprise.googleapis.com"}
}

func (c *Cmd) recaptchaKey() (*sim.Project, *sim.RecaptchaKey, error) {
	if err := c.API("recaptchaenterprise.googleapis.com"); err != nil {
		return nil, nil, err
	}
	p, err := c.P()
	if err != nil {
		return nil, nil, err
	}
	id, err := c.Arg(0, "KEY")
	if err != nil {
		return nil, nil, err
	}
	id = id[strings.LastIndex(id, "/")+1:]
	k := p.RecaptchaKeys[id]
	if k == nil {
		return nil, nil, fmt.Errorf("NOT_FOUND: Key projects/%s/keys/%s not found. Check the project: a key belongs to one project (gcloud config get-value project).", p.Number, id)
	}
	return p, k, nil
}

func recaptchaView(p *sim.Project, k *sim.RecaptchaKey) map[string]any {
	web := map[string]any{"integrationType": k.IntegrationType}
	if k.AllowAllDomains {
		web["allowAllDomains"] = true
	} else {
		web["allowedDomains"] = k.Domains
	}
	return map[string]any{"name": "projects/" + p.Number + "/keys/" + k.ID, "displayName": k.DisplayName, "webSettings": web, "labels": k.Labels, "createTime": k.Created}
}

func init() {
	reg("recaptcha keys create", func(c *Cmd) (any, error) {
		if err := c.API("recaptchaenterprise.googleapis.com"); err != nil {
			return nil, err
		}
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("recaptchaenterprise.keys.create"); err != nil {
			return nil, err
		}
		name := c.Str("display-name", "")
		if name == "" {
			return nil, fmt.Errorf("argument --display-name: Must be specified.")
		}
		if !c.Bool("web") {
			return nil, fmt.Errorf("Exactly one of (--android | --ios | --web) must be specified. The simulator supports --web keys.")
		}
		doms, err := recaptchaDomains(c.List("domains"))
		if err != nil {
			return nil, err
		}
		all := c.Bool("allow-all-domains")
		if len(doms) == 0 && !all {
			return nil, fmt.Errorf("INVALID_ARGUMENT: A web key needs --domains or --allow-all-domains.")
		}
		it := strings.ToUpper(c.Str("integration-type", "SCORE"))
		if it != "SCORE" && it != "CHECKBOX" && it != "INVISIBLE" {
			return nil, fmt.Errorf("argument --integration-type: Invalid choice: %q (score, checkbox, invisible).", strings.ToLower(it))
		}
		if p.RecaptchaKeys == nil {
			p.RecaptchaKeys = map[string]*sim.RecaptchaKey{}
		}
		// the platform's setup may fix the id so a ticket can quote it
		id := c.Str("key-id", "")
		if id == "" {
			id = newKeyID(p.ID, name, len(p.RecaptchaKeys))
		}
		if p.RecaptchaKeys[id] != nil {
			return nil, fmt.Errorf("ALREADY_EXISTS: Key %s already exists.", id)
		}
		p.RecaptchaKeys[id] = &sim.RecaptchaKey{ID: id, DisplayName: name, Platform: "WEB", IntegrationType: it, Domains: doms, AllowAllDomains: all, Labels: c.KV("labels"), Created: c.S.State.Now()}
		c.Audit("recaptchaenterprise.googleapis.com", "google.cloud.recaptchaenterprise.v1.RecaptchaEnterpriseService.CreateKey", "projects/"+p.ID+"/keys/"+id)
		return fmt.Sprintf("Created key [%s].\n", id), nil
	})
	reg("recaptcha keys list", func(c *Cmd) (any, error) {
		if err := c.API("recaptchaenterprise.googleapis.com"); err != nil {
			return nil, err
		}
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("recaptchaenterprise.keys.list"); err != nil {
			return nil, err
		}
		var rows []any
		ids := sim.SortedKeys(p.RecaptchaKeys)
		sort.SliceStable(ids, func(i, j int) bool { return p.RecaptchaKeys[ids[i]].DisplayName < p.RecaptchaKeys[ids[j]].DisplayName })
		for _, id := range ids {
			k := p.RecaptchaKeys[id]
			d := strings.Join(k.Domains, ",")
			if k.AllowAllDomains {
				d = "(all)"
			}
			rows = append(rows, map[string]any{"id": id, "name": k.DisplayName, "platform": k.Platform, "type": k.IntegrationType, "domains": d})
		}
		return Table{Cols: []Col{{"KEY_ID", "id"}, {"DISPLAY_NAME", "name"}, {"PLATFORM", "platform"}, {"INTEGRATION_TYPE", "type"}, {"DOMAINS", "domains"}}, Rows: rows}, nil
	})
	reg("recaptcha keys describe", func(c *Cmd) (any, error) {
		p, k, err := c.recaptchaKey()
		if err != nil {
			return nil, err
		}
		if err := c.Need("recaptchaenterprise.keys.get", recaptchaRes(p, k.ID)); err != nil {
			return nil, err
		}
		return Obj{V: recaptchaView(p, k)}, nil
	})
	reg("recaptcha keys update", func(c *Cmd) (any, error) {
		p, k, err := c.recaptchaKey()
		if err != nil {
			return nil, err
		}
		if err := c.Need("recaptchaenterprise.keys.update", recaptchaRes(p, k.ID)); err != nil {
			return nil, err
		}
		if c.Has("display-name") {
			k.DisplayName = c.Str("display-name", k.DisplayName)
		}
		if c.Has("domains") {
			doms, err := recaptchaDomains(c.List("domains"))
			if err != nil {
				return nil, err
			}
			// like the real command: --domains replaces the whole list
			k.Domains = doms
		}
		if c.Has("allow-all-domains") {
			k.AllowAllDomains = c.Bool("allow-all-domains")
		}
		if len(k.Domains) == 0 && !k.AllowAllDomains {
			return nil, fmt.Errorf("INVALID_ARGUMENT: A web key needs at least one domain.")
		}
		if c.Has("labels") {
			k.Labels = c.KV("labels")
		}
		c.Audit("recaptchaenterprise.googleapis.com", "google.cloud.recaptchaenterprise.v1.RecaptchaEnterpriseService.UpdateKey", "projects/"+p.ID+"/keys/"+k.ID)
		return fmt.Sprintf("Updated key [%s].\n", k.ID), nil
	})
	reg("recaptcha keys delete", func(c *Cmd) (any, error) {
		p, k, err := c.recaptchaKey()
		if err != nil {
			return nil, err
		}
		if err := c.Need("recaptchaenterprise.keys.delete", recaptchaRes(p, k.ID)); err != nil {
			return nil, err
		}
		delete(p.RecaptchaKeys, k.ID)
		c.Audit("recaptchaenterprise.googleapis.com", "google.cloud.recaptchaenterprise.v1.RecaptchaEnterpriseService.DeleteKey", "projects/"+p.ID+"/keys/"+k.ID)
		return fmt.Sprintf("Deleted key [%s].\n", k.ID), nil
	})
}
