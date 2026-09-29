package cli

import (
	"fmt"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

// Folder and organization IAM (governance, separation of duties).
func init() {
	nodeIAM := func(c *Cmd, kind string) (*sim.Policy, string, error) {
		id, err := c.Arg(0, strings.ToUpper(kind)+"_ID")
		if err != nil {
			return nil, "", err
		}
		st := c.S.State
		switch kind {
		case "folder":
			id = strings.TrimPrefix(id, "folders/")
			f := st.Folders["folders/"+id]
			if f == nil {
				return nil, "", fmt.Errorf("NOT_FOUND: folder %s (use `gcloud resource-manager folders list`)", id)
			}
			return &f.IAM, "folders/" + id, nil
		default:
			id = strings.TrimPrefix(id, "organizations/")
			if st.Org == nil || id != st.Org.ID {
				return nil, "", fmt.Errorf("NOT_FOUND: organization %s", id)
			}
			return &st.Org.IAM, "organizations/" + id, nil
		}
	}
	// policies above the node also grant setIamPolicy (org admins)
	nodeRes := func(c *Cmd, pol *sim.Policy, name string) sim.Resource {
		pols := []*sim.Policy{pol}
		if f := c.S.State.Folders[name]; f != nil {
			parent := f.Parent
			for i := 0; i < 10 && strings.HasPrefix(parent, "folders/"); i++ {
				pf := c.S.State.Folders[parent]
				if pf == nil {
					break
				}
				pols = append(pols, &pf.IAM)
				parent = pf.Parent
			}
			pols = append(pols, &c.S.State.Org.IAM)
		}
		return sim.Resource{Type: "cloudresourcemanager.googleapis.com/Folder", Name: name, Service: "cloudresourcemanager.googleapis.com", Policies: pols}
	}
	for _, kind := range []string{"folder", "organization"} {
		kind := kind
		prefix := "resource-manager folders"
		perm := "resourcemanager.folders"
		if kind == "organization" {
			prefix, perm = "organizations", "resourcemanager.organizations"
		}
		reg(prefix+" get-iam-policy", func(c *Cmd) (any, error) {
			pol, name, err := nodeIAM(c, kind)
			if err != nil {
				return nil, err
			}
			if err := c.Need(perm+".getIamPolicy", nodeRes(c, pol, name)); err != nil {
				return nil, err
			}
			return policyView(pol), nil
		})
		reg(prefix+" add-iam-policy-binding", func(c *Cmd) (any, error) {
			pol, name, err := nodeIAM(c, kind)
			if err != nil {
				return nil, err
			}
			m, role, cond, err := c.bindingArgs()
			if err != nil {
				return nil, err
			}
			if err := c.Need(perm+".setIamPolicy", nodeRes(c, pol, name)); err != nil {
				return nil, err
			}
			pol.AddBinding(role, m, cond)
			c.S.State.Audit("", c.Principal(), "cloudresourcemanager.googleapis.com", "SetIamPolicy", name)
			return "Updated IAM policy for " + kind + " [" + strings.SplitN(name, "/", 2)[1] + "].\n", nil
		})
		reg(prefix+" remove-iam-policy-binding", func(c *Cmd) (any, error) {
			pol, name, err := nodeIAM(c, kind)
			if err != nil {
				return nil, err
			}
			m, role, _, err := c.bindingArgs()
			if err != nil {
				return nil, err
			}
			if err := c.Need(perm+".setIamPolicy", nodeRes(c, pol, name)); err != nil {
				return nil, err
			}
			if !pol.RemoveBinding(role, m) {
				return nil, fmt.Errorf("Policy binding with the specified principal, role, and condition not found!")
			}
			c.S.State.Audit("", c.Principal(), "cloudresourcemanager.googleapis.com", "SetIamPolicy", name)
			return "Updated IAM policy for " + kind + " [" + strings.SplitN(name, "/", 2)[1] + "].\n", nil
		})
	}
	reg("resource-manager folders describe", func(c *Cmd) (any, error) {
		id, err := c.Arg(0, "FOLDER_ID")
		if err != nil {
			return nil, err
		}
		f := c.S.State.Folders["folders/"+strings.TrimPrefix(id, "folders/")]
		if f == nil {
			return nil, fmt.Errorf("NOT_FOUND: folder %s", id)
		}
		return Obj{V: map[string]any{"name": "folders/" + f.ID, "displayName": f.DisplayName, "parent": f.Parent, "lifecycleState": "ACTIVE"}}, nil
	})
}
