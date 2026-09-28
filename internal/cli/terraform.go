package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/neodevesp/gcp-trainer/internal/sim"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

// Terraform-lite: parses real HCL (*.tf) with the HashiCorp parser, evaluates
// variables/locals/references and reconciles a subset of google provider
// resources against the simulator through the same code paths as gcloud.

type tfResource struct {
	Type, Name string
	Body       *hclsyntax.Body
	Deps       []string
	Desired    map[string]any
}

type tfConfig struct {
	Project, Region, Zone string
	Vars                  map[string]cty.Value
	Locals                map[string]hcl.Expression
	Resources             []*tfResource
	Outputs               map[string]hcl.Expression
	Backend               map[string]string
}

type tfStateEntry struct {
	Type string `json:"type"`
	Name string `json:"name"`
	ID   string `json:"id"`
}

type tfState struct {
	Version   int            `json:"version"`
	Serial    int            `json:"serial"`
	Resources []tfStateEntry `json:"resources"`
	Outputs   map[string]any `json:"outputs"`
}

func (s *Session) terraform(args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: terraform [global options] <subcommand> [args]\n", nil
	}
	pos, f := parseArgs(args)
	sub := pos[0]
	extraVars := map[string]string{}
	for _, v := range f["var"] {
		k, val, _ := strings.Cut(v, "=")
		extraVars[k] = val
	}
	for i := 0; i < len(args); i++ {
		if args[i] == "-var" && i+1 < len(args) {
			k, val, _ := strings.Cut(args[i+1], "=")
			extraVars[k] = val
		}
	}
	autoApprove := f["auto-approve"] != nil
	switch sub {
	case "version":
		return "Terraform v1.9.5 (simulated)\non linux_amd64\n+ provider registry.terraform.io/hashicorp/google v6.8.0\n", nil
	case "fmt":
		var names []string
		for _, k := range sim.SortedKeys(s.Files) {
			if strings.HasSuffix(k, ".tf") && !strings.Contains(k, "/") {
				names = append(names, k)
			}
		}
		return strings.Join(names, "\n") + "\n", nil
	case "init":
		cfg, err := s.tfLoad(extraVars)
		if err != nil {
			return "", err
		}
		if b := cfg.Backend["bucket"]; b != "" {
			if bk, _ := s.State.FindBucket(b); bk == nil {
				return "", fail(1, "Error: Failed to get existing workspaces: querying Cloud Storage failed: storage: bucket doesn't exist (%s)", b)
			}
		}
		s.Env["TF_INITIALIZED"] = "1"
		backend := "local"
		if cfg.Backend["bucket"] != "" {
			backend = "gcs"
		}
		return fmt.Sprintf("\nInitializing the backend...\nSuccessfully configured the backend \"%s\"!\n\nInitializing provider plugins...\n- Installing hashicorp/google v6.8.0...\n\nTerraform has been successfully initialized!\n", backend), nil
	}
	if s.Env["TF_INITIALIZED"] != "1" && sub != "validate" {
		return "", fail(1, "Error: Inconsistent dependency lock file\n\nThe working directory has not been initialized. Run \"terraform init\".")
	}
	cfg, err := s.tfLoad(extraVars)
	if err != nil {
		return "", err
	}
	st := s.tfReadState(cfg)
	switch sub {
	case "validate":
		if _, err := s.tfEvaluate(cfg); err != nil {
			return "", err
		}
		return "Success! The configuration is valid.\n", nil
	case "plan":
		ev, err := s.tfEvaluate(cfg)
		if err != nil {
			return "", err
		}
		out, _, _ := s.tfPlan(cfg, ev, st, false)
		return out, nil
	case "apply":
		ev, err := s.tfEvaluate(cfg)
		if err != nil {
			return "", err
		}
		out, changes, _ := s.tfPlan(cfg, ev, st, false)
		if changes == 0 {
			return out + s.tfOutputs(cfg, ev, st), nil
		}
		if !autoApprove {
			return out, fail(1, "\nError: No confirmation in non-interactive terminal. Re-run with -auto-approve.")
		}
		res, err := s.tfApply(cfg, ev, st)
		out += res
		if err != nil {
			s.tfWriteState(cfg, st)
			return out, err
		}
		ev, _ = s.tfEvaluate(cfg)
		out += s.tfOutputs(cfg, ev, st)
		s.tfWriteState(cfg, st)
		return out, nil
	case "destroy":
		if !autoApprove {
			return "", fail(1, "Error: No confirmation in non-interactive terminal. Re-run with -auto-approve.")
		}
		var b strings.Builder
		for i := len(st.Resources) - 1; i >= 0; i-- {
			e := st.Resources[i]
			if err := s.tfDestroy(cfg, e); err != nil {
				s.tfWriteState(cfg, st)
				return b.String(), fail(1, "Error: deleting %s.%s: %v", e.Type, e.Name, err)
			}
			b.WriteString(fmt.Sprintf("%s.%s: Destruction complete\n", e.Type, e.Name))
		}
		n := len(st.Resources)
		st.Resources = nil
		st.Outputs = nil
		s.tfWriteState(cfg, st)
		return b.String() + fmt.Sprintf("\nDestroy complete! Resources: %d destroyed.\n", n), nil
	case "output":
		if len(st.Outputs) == 0 {
			return "", fail(1, "Warning: No outputs found")
		}
		if len(pos) > 1 {
			v, ok := st.Outputs[pos[1]]
			if !ok {
				return "", fail(1, "Error: Output %q not found", pos[1])
			}
			if f["raw"] != nil {
				return fmt.Sprint(v) + "\n", nil
			}
			j, _ := json.Marshal(v)
			return string(j) + "\n", nil
		}
		var b strings.Builder
		for _, k := range sortedAnyKeys(st.Outputs) {
			j, _ := json.Marshal(st.Outputs[k])
			b.WriteString(k + " = " + string(j) + "\n")
		}
		return b.String(), nil
	case "show", "state":
		if sub == "state" && (len(pos) < 2 || pos[1] != "list") {
			return "", fail(1, "only `terraform state list` is supported")
		}
		var b strings.Builder
		for _, e := range st.Resources {
			b.WriteString(e.Type + "." + e.Name)
			if sub == "show" {
				b.WriteString("  (id=" + e.ID + ")")
			}
			b.WriteString("\n")
		}
		return b.String(), nil
	}
	return "", fail(1, "Terraform has no command named %q (supported: init, validate, plan, apply, destroy, output, show, state list, fmt)", sub)
}

func (s *Session) tfLoad(extraVars map[string]string) (*tfConfig, error) {
	cfg := &tfConfig{Project: s.Project, Region: s.Region, Zone: s.Zone, Vars: map[string]cty.Value{}, Locals: map[string]hcl.Expression{}, Outputs: map[string]hcl.Expression{}, Backend: map[string]string{}}
	var files []string
	for _, k := range sim.SortedKeys(s.Files) {
		if strings.HasSuffix(k, ".tf") && !strings.Contains(k, "/") {
			files = append(files, k)
		}
	}
	if len(files) == 0 {
		return nil, fail(1, "Error: No configuration files\n\nApply requires configuration to be present in the working directory (*.tf).")
	}
	tfvars := map[string]cty.Value{}
	if content, ok := s.Files["terraform.tfvars"]; ok {
		f, diags := hclsyntax.ParseConfig([]byte(content), "terraform.tfvars", hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return nil, fail(1, "Error: %s", diags.Error())
		}
		attrs, _ := f.Body.JustAttributes()
		for n, a := range attrs {
			v, d := a.Expr.Value(nil)
			if !d.HasErrors() {
				tfvars[n] = v
			}
		}
	}
	var providerBody *hclsyntax.Body
	for _, fn := range files {
		f, diags := hclsyntax.ParseConfig([]byte(s.Files[fn]), fn, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return nil, fail(1, "Error: %s", diags.Error())
		}
		body := f.Body.(*hclsyntax.Body)
		for _, blk := range body.Blocks {
			switch blk.Type {
			case "variable":
				name := blk.Labels[0]
				var v cty.Value = cty.NilVal
				if a, ok := blk.Body.Attributes["default"]; ok {
					dv, d := a.Expr.Value(nil)
					if d.HasErrors() {
						return nil, fail(1, "Error: %s", d.Error())
					}
					v = dv
				}
				if tv, ok := tfvars[name]; ok {
					v = tv
				}
				if ev, ok := extraVars[name]; ok {
					v = cty.StringVal(ev)
				}
				if ev, ok := s.Env["TF_VAR_"+name]; ok {
					v = cty.StringVal(ev)
				}
				if v == cty.NilVal {
					return nil, fail(1, "Error: No value for required variable\n\n  on %s: variable %q: The root module input variable %q is not set, and has no default value. Use a -var or -var-file command line argument to provide a value.", fn, name, name)
				}
				cfg.Vars[name] = v
			case "locals":
				for n, a := range blk.Body.Attributes {
					cfg.Locals[n] = a.Expr
				}
			case "provider":
				if blk.Labels[0] == "google" || blk.Labels[0] == "google-beta" {
					providerBody = blk.Body
				}
			case "resource":
				if len(blk.Labels) != 2 {
					return nil, fail(1, "Error: resource block requires two labels")
				}
				if tfTypes[blk.Labels[0]] == nil {
					return nil, fail(1, "Error: Invalid resource type\n\n  on %s line %d: The provider hashicorp/google does not support resource type %q in the simulator.\n  Supported: %s", fn, blk.DefRange().Start.Line, blk.Labels[0], strings.Join(sim.SortedKeys(tfTypes), ", "))
				}
				r := &tfResource{Type: blk.Labels[0], Name: blk.Labels[1], Body: blk.Body}
				for _, t := range bodyVars(blk.Body) {
					root := t.RootName()
					if strings.HasPrefix(root, "google_") && len(t) > 1 {
						if a, ok := t[1].(hcl.TraverseAttr); ok {
							r.Deps = append(r.Deps, root+"."+a.Name)
						}
					}
				}
				cfg.Resources = append(cfg.Resources, r)
			case "output":
				if a, ok := blk.Body.Attributes["value"]; ok {
					cfg.Outputs[blk.Labels[0]] = a.Expr
				}
			case "terraform":
				for _, b2 := range blk.Body.Blocks {
					if b2.Type == "backend" && len(b2.Labels) > 0 && b2.Labels[0] == "gcs" {
						for n, a := range b2.Body.Attributes {
							v, _ := a.Expr.Value(nil)
							if v.Type() == cty.String {
								cfg.Backend[n] = v.AsString()
							}
						}
					}
				}
			case "data", "module":
				return nil, fail(1, "Error: %s blocks are not supported by the simulator's Terraform engine", blk.Type)
			}
		}
	}
	if providerBody != nil {
		ctx := cfg.evalCtx(s, nil)
		for n, a := range providerBody.Attributes {
			v, d := a.Expr.Value(ctx)
			if d.HasErrors() || v.Type() != cty.String {
				continue
			}
			switch n {
			case "project":
				cfg.Project = v.AsString()
			case "region":
				cfg.Region = v.AsString()
			case "zone":
				cfg.Zone = v.AsString()
			}
		}
	}
	// topological order
	ordered, err := tfTopo(cfg.Resources)
	if err != nil {
		return nil, err
	}
	cfg.Resources = ordered
	return cfg, nil
}

func tfTopo(rs []*tfResource) ([]*tfResource, error) {
	byKey := map[string]*tfResource{}
	for _, r := range rs {
		byKey[r.Type+"."+r.Name] = r
	}
	var out []*tfResource
	state := map[string]int{}
	var visit func(r *tfResource) error
	visit = func(r *tfResource) error {
		k := r.Type + "." + r.Name
		switch state[k] {
		case 1:
			return fail(1, "Error: Cycle: %s", k)
		case 2:
			return nil
		}
		state[k] = 1
		for _, d := range r.Deps {
			dep := byKey[d]
			if dep == nil {
				return fail(1, "Error: Reference to undeclared resource\n\nA managed resource %q has not been declared in the root module.", d)
			}
			if err := visit(dep); err != nil {
				return err
			}
		}
		state[k] = 2
		out = append(out, r)
		return nil
	}
	for _, r := range rs {
		if err := visit(r); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (cfg *tfConfig) evalCtx(s *Session, resources map[string]map[string]cty.Value) *hcl.EvalContext {
	vars := map[string]cty.Value{"var": cty.ObjectVal(cfg.Vars)}
	if len(cfg.Vars) == 0 {
		vars["var"] = cty.EmptyObjectVal
	}
	for typ, m := range resources {
		vars[typ] = cty.ObjectVal(m)
	}
	ctx := &hcl.EvalContext{Variables: vars, Functions: map[string]function.Function{
		"format": stdlib.FormatFunc, "join": stdlib.JoinFunc, "lower": stdlib.LowerFunc, "upper": stdlib.UpperFunc,
		"concat": stdlib.ConcatFunc, "length": stdlib.LengthFunc, "tostring": stdlib.MakeToFunc(cty.String), "tonumber": stdlib.MakeToFunc(cty.Number),
		"replace": stdlib.ReplaceFunc, "split": stdlib.SplitFunc, "trimspace": stdlib.TrimSpaceFunc, "merge": stdlib.MergeFunc, "coalesce": stdlib.CoalesceFunc,
		"file": function.New(&function.Spec{Params: []function.Parameter{{Name: "path", Type: cty.String}}, Type: function.StaticReturnType(cty.String),
			Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
				p := s.path(strings.TrimPrefix(args[0].AsString(), "${path.module}/"))
				p = strings.TrimPrefix(p, "path.module/")
				c, ok := s.Files[p]
				if !ok {
					return cty.NilVal, fmt.Errorf("no file exists at %q", args[0].AsString())
				}
				return cty.StringVal(c), nil
			}}),
	}}
	locals := map[string]cty.Value{}
	for i := 0; i < 3; i++ { // resolve locals referencing locals
		ctx.Variables["local"] = cty.ObjectVal(locals)
		if len(locals) == 0 {
			ctx.Variables["local"] = cty.EmptyObjectVal
		}
		for n, e := range cfg.Locals {
			if v, d := e.Value(ctx); !d.HasErrors() {
				locals[n] = v
			}
		}
	}
	if len(locals) > 0 {
		ctx.Variables["local"] = cty.ObjectVal(locals)
	}
	ctx.Variables["path"] = cty.ObjectVal(map[string]cty.Value{"module": cty.StringVal("."), "root": cty.StringVal(".")})
	return ctx
}

func ctyToGo(v cty.Value) any {
	if v.IsNull() {
		return nil
	}
	if !v.IsKnown() {
		return "(known after apply)"
	}
	t := v.Type()
	switch {
	case t == cty.String:
		return v.AsString()
	case t == cty.Number:
		f, _ := v.AsBigFloat().Float64()
		if f == float64(int64(f)) {
			return int64(f)
		}
		return f
	case t == cty.Bool:
		return v.True()
	case t.IsListType() || t.IsTupleType() || t.IsSetType():
		var out []any
		for it := v.ElementIterator(); it.Next(); {
			_, e := it.Element()
			out = append(out, ctyToGo(e))
		}
		return out
	case t.IsMapType() || t.IsObjectType():
		out := map[string]any{}
		for it := v.ElementIterator(); it.Next(); {
			k, e := it.Element()
			out[k.AsString()] = ctyToGo(e)
		}
		return out
	}
	return nil
}

func goToCty(v any) cty.Value {
	switch x := v.(type) {
	case string:
		return cty.StringVal(x)
	case int:
		return cty.NumberIntVal(int64(x))
	case int64:
		return cty.NumberIntVal(x)
	case bool:
		return cty.BoolVal(x)
	case []string:
		if len(x) == 0 {
			return cty.ListValEmpty(cty.String)
		}
		var l []cty.Value
		for _, e := range x {
			l = append(l, cty.StringVal(e))
		}
		return cty.ListVal(l)
	case map[string]any:
		m := map[string]cty.Value{}
		for k, e := range x {
			m[k] = goToCty(e)
		}
		return cty.ObjectVal(m)
	}
	return cty.StringVal(fmt.Sprint(v))
}

// bodyToMap evaluates a resource body (attributes + nested blocks).
func bodyToMap(b *hclsyntax.Body, ctx *hcl.EvalContext) (map[string]any, error) {
	out := map[string]any{}
	for n, a := range b.Attributes {
		if n == "depends_on" || n == "lifecycle" {
			continue
		}
		v, d := a.Expr.Value(ctx)
		if d.HasErrors() {
			return nil, fmt.Errorf("%s", d.Error())
		}
		out[n] = ctyToGo(v)
	}
	for _, blk := range b.Blocks {
		if blk.Type == "lifecycle" {
			continue
		}
		m, err := bodyToMap(blk.Body, ctx)
		if err != nil {
			return nil, err
		}
		l, _ := out[blk.Type].([]any)
		out[blk.Type] = append(l, m)
	}
	return out, nil
}

type tfEvaluated struct {
	desired map[string]map[string]any
	attrs   map[string]map[string]cty.Value // type -> name -> object
	ctx     *hcl.EvalContext
}

func (s *Session) tfEvaluate(cfg *tfConfig) (*tfEvaluated, error) {
	ev := &tfEvaluated{desired: map[string]map[string]any{}, attrs: map[string]map[string]cty.Value{}}
	for _, r := range cfg.Resources {
		ctx := cfg.evalCtx(s, ev.attrs)
		d, err := bodyToMap(r.Body, ctx)
		if err != nil {
			return nil, fail(1, "Error: %s.%s: %v", r.Type, r.Name, err)
		}
		t := tfTypes[r.Type]
		for _, req := range t.required {
			if d[req] == nil {
				return nil, fail(1, "Error: Missing required argument\n\n  on %s.%s: The argument %q is required, but no definition was found.", r.Type, r.Name, req)
			}
		}
		r.Desired = d
		ev.desired[r.Type+"."+r.Name] = d
		computed := t.computed(cfg, d)
		for k, v := range d {
			if _, ok := computed[k]; !ok {
				if vs, ok := v.(string); ok {
					computed[k] = vs
				}
			}
		}
		if ev.attrs[r.Type] == nil {
			ev.attrs[r.Type] = map[string]cty.Value{}
		}
		ev.attrs[r.Type][r.Name] = goToCty(computed)
	}
	ev.ctx = cfg.evalCtx(s, ev.attrs)
	return ev, nil
}

func (s *Session) tfReadState(cfg *tfConfig) *tfState {
	st := &tfState{Version: 4}
	raw := s.Files["terraform.tfstate"]
	if b := cfg.Backend["bucket"]; b != "" {
		if bk, _ := s.State.FindBucket(b); bk != nil {
			key := strings.Trim(cfg.Backend["prefix"], "/") + "/default.tfstate"
			if o := bk.Objects[strings.TrimPrefix(key, "/")]; o != nil {
				raw = o.Content
			}
		}
	}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), st)
	}
	return st
}

func (s *Session) tfWriteState(cfg *tfConfig, st *tfState) {
	st.Serial++
	b, _ := json.MarshalIndent(st, "", "  ")
	if bn := cfg.Backend["bucket"]; bn != "" {
		if bk, _ := s.State.FindBucket(bn); bk != nil {
			key := strings.TrimPrefix(strings.Trim(cfg.Backend["prefix"], "/")+"/default.tfstate", "/")
			bk.Objects[key] = &sim.Object{Name: key, Content: string(b), Size: len(b), StorageClass: bk.StorageClass, Generation: st.Serial, Updated: s.State.Now()}
			return
		}
	}
	s.Files["terraform.tfstate"] = string(b)
}

func stateFind(st *tfState, typ, name string) *tfStateEntry {
	for i := range st.Resources {
		if st.Resources[i].Type == typ && st.Resources[i].Name == name {
			return &st.Resources[i]
		}
	}
	return nil
}

type tfAction struct {
	r       *tfResource
	kind    string // create, replace, destroy, noop
	changed []string
	entry   tfStateEntry
}

func (s *Session) tfActions(cfg *tfConfig, ev *tfEvaluated, st *tfState) []tfAction {
	var acts []tfAction
	inConfig := map[string]bool{}
	for _, r := range cfg.Resources {
		inConfig[r.Type+"."+r.Name] = true
		t := tfTypes[r.Type]
		id := t.id(cfg, r.Desired)
		actual, exists := t.read(s, cfg, id)
		e := stateFind(st, r.Type, r.Name)
		if !exists {
			acts = append(acts, tfAction{r: r, kind: "create"})
			continue
		}
		if e == nil {
			// exists in the cloud but not in state: terraform would fail with "already exists"
			acts = append(acts, tfAction{r: r, kind: "create"})
			continue
		}
		want := t.norm(cfg, r.Desired)
		var changed []string
		for _, k := range sim.SortedKeys(want) {
			if fmt.Sprint(want[k]) != fmt.Sprint(actual[k]) {
				changed = append(changed, fmt.Sprintf("%s: %v -> %v", k, actual[k], want[k]))
			}
		}
		if e.ID != id {
			changed = append(changed, fmt.Sprintf("id: %s -> %s", e.ID, id))
		}
		if len(changed) > 0 {
			acts = append(acts, tfAction{r: r, kind: "replace", changed: changed, entry: *e})
		}
	}
	for i := len(st.Resources) - 1; i >= 0; i-- {
		e := st.Resources[i]
		if !inConfig[e.Type+"."+e.Name] {
			acts = append(acts, tfAction{kind: "destroy", entry: e})
		}
	}
	return acts
}

func (s *Session) tfPlan(cfg *tfConfig, ev *tfEvaluated, st *tfState, _ bool) (string, int, []tfAction) {
	acts := s.tfActions(cfg, ev, st)
	if len(acts) == 0 {
		return "\nNo changes. Your infrastructure matches the configuration.\n\nTerraform has compared your real infrastructure against your configuration and found no differences, so no changes are needed.\n", 0, nil
	}
	var b strings.Builder
	b.WriteString("\nTerraform used the selected providers to generate the following execution plan. Resource actions are indicated with the following symbols:\n  + create\n  - destroy\n-/+ destroy and then create replacement\n\nTerraform will perform the following actions:\n\n")
	add, change, destroy := 0, 0, 0
	for _, a := range acts {
		switch a.kind {
		case "create":
			add++
			fmt.Fprintf(&b, "  # %s.%s will be created\n  + resource %q %q {\n", a.r.Type, a.r.Name, a.r.Type, a.r.Name)
			for _, k := range sim.SortedKeys(a.r.Desired) {
				j, _ := json.Marshal(a.r.Desired[k])
				fmt.Fprintf(&b, "      + %-22s = %s\n", k, string(j))
			}
			b.WriteString("    }\n\n")
		case "replace":
			add++
			destroy++
			fmt.Fprintf(&b, "  # %s.%s must be replaced\n-/+ resource %q %q {\n", a.r.Type, a.r.Name, a.r.Type, a.r.Name)
			for _, c := range a.changed {
				fmt.Fprintf(&b, "      ~ %s # forces replacement\n", c)
			}
			b.WriteString("    }\n\n")
		case "destroy":
			destroy++
			fmt.Fprintf(&b, "  # %s.%s will be destroyed\n  - resource %q %q { id = %q }\n\n", a.entry.Type, a.entry.Name, a.entry.Type, a.entry.Name, a.entry.ID)
		}
	}
	fmt.Fprintf(&b, "Plan: %d to add, %d to change, %d to destroy.\n", add, change, destroy)
	return b.String(), add + change + destroy, acts
}

func (s *Session) tfApply(cfg *tfConfig, ev *tfEvaluated, st *tfState) (string, error) {
	acts := s.tfActions(cfg, ev, st)
	var b strings.Builder
	added, destroyed := 0, 0
	for _, a := range acts {
		if a.kind == "destroy" {
			if err := s.tfDestroy(cfg, a.entry); err != nil {
				return b.String(), fail(1, "Error: deleting %s.%s: %v", a.entry.Type, a.entry.Name, err)
			}
			removeState(st, a.entry.Type, a.entry.Name)
			destroyed++
			fmt.Fprintf(&b, "%s.%s: Destruction complete\n", a.entry.Type, a.entry.Name)
		}
	}
	for _, a := range acts {
		if a.kind == "destroy" {
			continue
		}
		t := tfTypes[a.r.Type]
		if a.kind == "replace" {
			if err := s.tfDestroy(cfg, a.entry); err != nil {
				return b.String(), fail(1, "Error: replacing %s.%s: %v", a.r.Type, a.r.Name, err)
			}
			removeState(st, a.r.Type, a.r.Name)
			destroyed++
		}
		fmt.Fprintf(&b, "%s.%s: Creating...\n", a.r.Type, a.r.Name)
		if err := t.create(s, cfg, a.r.Desired); err != nil {
			return b.String(), fail(1, "\nError: Error creating %s.%s: %v", a.r.Type, a.r.Name, strings.TrimPrefix(err.Error(), "ERROR: "))
		}
		id := t.id(cfg, a.r.Desired)
		st.Resources = append(st.Resources, tfStateEntry{Type: a.r.Type, Name: a.r.Name, ID: id})
		added++
		fmt.Fprintf(&b, "%s.%s: Creation complete after 2s [id=%s]\n", a.r.Type, a.r.Name, id)
	}
	fmt.Fprintf(&b, "\nApply complete! Resources: %d added, 0 changed, %d destroyed.\n", added, destroyed)
	return b.String(), nil
}

func removeState(st *tfState, typ, name string) {
	var keep []tfStateEntry
	for _, e := range st.Resources {
		if !(e.Type == typ && e.Name == name) {
			keep = append(keep, e)
		}
	}
	st.Resources = keep
}

func (s *Session) tfDestroy(cfg *tfConfig, e tfStateEntry) error {
	t := tfTypes[e.Type]
	if t == nil {
		return nil
	}
	if _, exists := t.read(s, cfg, e.ID); !exists {
		return nil
	}
	return t.destroy(s, cfg, e.ID)
}

func (s *Session) tfOutputs(cfg *tfConfig, ev *tfEvaluated, st *tfState) string {
	if len(cfg.Outputs) == 0 {
		return ""
	}
	st.Outputs = map[string]any{}
	var b strings.Builder
	b.WriteString("\nOutputs:\n\n")
	for _, k := range sortedExprKeys(cfg.Outputs) {
		v, d := cfg.Outputs[k].Value(ev.ctx)
		if d.HasErrors() {
			continue
		}
		g := ctyToGo(v)
		st.Outputs[k] = g
		j, _ := json.Marshal(g)
		b.WriteString(k + " = " + string(j) + "\n")
	}
	s.tfWriteState(cfg, st)
	return b.String()
}

func sortedExprKeys(m map[string]hcl.Expression) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	sort.Strings(k)
	return k
}

// ---- resource type adapters ---------------------------------------------------

type tfType struct {
	required []string
	id       func(cfg *tfConfig, d map[string]any) string
	computed func(cfg *tfConfig, d map[string]any) map[string]any
	norm     func(cfg *tfConfig, d map[string]any) map[string]any
	read     func(s *Session, cfg *tfConfig, id string) (map[string]any, bool)
	create   func(s *Session, cfg *tfConfig, d map[string]any) error
	destroy  func(s *Session, cfg *tfConfig, id string) error
}

func sv(d map[string]any, k, def string) string {
	if v, ok := d[k]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return def
}

func bv(d map[string]any, k string, def bool) bool {
	if v, ok := d[k].(bool); ok {
		return v
	}
	return def
}

func lv(d map[string]any, k string) []string {
	var out []string
	if l, ok := d[k].([]any); ok {
		for _, e := range l {
			out = append(out, fmt.Sprint(e))
		}
	}
	sort.Strings(out)
	return out
}

func blk(d map[string]any, k string) map[string]any {
	if l, ok := d[k].([]any); ok && len(l) > 0 {
		m, _ := l[0].(map[string]any)
		return m
	}
	return nil
}

func blks(d map[string]any, k string) []map[string]any {
	var out []map[string]any
	if l, ok := d[k].([]any); ok {
		for _, e := range l {
			if m, ok := e.(map[string]any); ok {
				out = append(out, m)
			}
		}
	}
	return out
}

func base(v string) string { return v[strings.LastIndex(v, "/")+1:] }

func (cfg *tfConfig) proj(d map[string]any) string { return sv(d, "project", cfg.Project) }

// tfExec runs a gcloud command as the current principal without ticking time.
func (s *Session) tfExec(project string, args ...string) error {
	sub := NewSession(s.State, project, s.Account)
	sub.NoTick = true
	sub.Impersonate = s.Impersonate
	sub.Policy = s.Policy
	sub.Region, sub.Zone = s.Region, s.Zone
	for k, v := range s.Files {
		sub.Files[k] = v
	}
	_, err := sub.gcloud(args, "")
	return err
}

func tfSelf(cfg *tfConfig, d map[string]any, path string) map[string]any {
	id := "projects/" + cfg.proj(d) + "/" + path
	return map[string]any{"id": id, "self_link": "https://www.googleapis.com/compute/v1/" + id, "name": sv(d, "name", "")}
}

var tfTypes = map[string]*tfType{}

func init() {
	proj := func(s *Session, cfg *tfConfig, d string) *sim.Project { return s.State.Projects[d] }
	_ = proj
	tfTypes["google_project_service"] = &tfType{required: []string{"service"},
		id:       func(cfg *tfConfig, d map[string]any) string { return cfg.proj(d) + "/" + sv(d, "service", "") },
		computed: func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{"id": cfg.proj(d) + "/" + sv(d, "service", "")} },
		norm:     func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{} },
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			p, svc, _ := strings.Cut(id, "/")
			pr := s.State.Projects[p]
			return map[string]any{}, pr != nil && pr.Services[svc]
		},
		create:  func(s *Session, cfg *tfConfig, d map[string]any) error { return s.tfExec(cfg.proj(d), "services", "enable", sv(d, "service", "")) },
		destroy: func(s *Session, cfg *tfConfig, id string) error { return nil },
	}
	tfTypes["google_compute_network"] = &tfType{required: []string{"name"},
		id:       func(cfg *tfConfig, d map[string]any) string { return "projects/" + cfg.proj(d) + "/global/networks/" + sv(d, "name", "") },
		computed: func(cfg *tfConfig, d map[string]any) map[string]any { return tfSelf(cfg, d, "global/networks/"+sv(d, "name", "")) },
		norm: func(cfg *tfConfig, d map[string]any) map[string]any {
			return map[string]any{"auto_create_subnetworks": bv(d, "auto_create_subnetworks", true)}
		},
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			parts := strings.Split(id, "/")
			p := s.State.Projects[parts[1]]
			if p == nil || p.Networks[parts[4]] == nil {
				return nil, false
			}
			return map[string]any{"auto_create_subnetworks": p.Networks[parts[4]].Mode == "auto"}, true
		},
		create: func(s *Session, cfg *tfConfig, d map[string]any) error {
			mode := "auto"
			if !bv(d, "auto_create_subnetworks", true) {
				mode = "custom"
			}
			return s.tfExec(cfg.proj(d), "compute", "networks", "create", sv(d, "name", ""), "--subnet-mode="+mode)
		},
		destroy: func(s *Session, cfg *tfConfig, id string) error {
			parts := strings.Split(id, "/")
			return s.tfExec(parts[1], "compute", "networks", "delete", parts[4], "--quiet")
		},
	}
	tfTypes["google_compute_subnetwork"] = &tfType{required: []string{"name", "ip_cidr_range", "network"},
		id: func(cfg *tfConfig, d map[string]any) string {
			return "projects/" + cfg.proj(d) + "/regions/" + sv(d, "region", cfg.Region) + "/subnetworks/" + sv(d, "name", "")
		},
		computed: func(cfg *tfConfig, d map[string]any) map[string]any {
			return tfSelf(cfg, d, "regions/"+sv(d, "region", cfg.Region)+"/subnetworks/"+sv(d, "name", ""))
		},
		norm: func(cfg *tfConfig, d map[string]any) map[string]any {
			return map[string]any{"ip_cidr_range": sv(d, "ip_cidr_range", ""), "region": sv(d, "region", cfg.Region), "network": base(sv(d, "network", "")), "private_ip_google_access": bv(d, "private_ip_google_access", false)}
		},
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			parts := strings.Split(id, "/")
			p := s.State.Projects[parts[1]]
			if p == nil || p.Subnets[parts[5]] == nil {
				return nil, false
			}
			sn := p.Subnets[parts[5]]
			return map[string]any{"ip_cidr_range": sn.Range, "region": sn.Region, "network": sn.Network, "private_ip_google_access": sn.PrivateGoogleAccess}, true
		},
		create: func(s *Session, cfg *tfConfig, d map[string]any) error {
			args := []string{"compute", "networks", "subnets", "create", sv(d, "name", ""), "--network=" + base(sv(d, "network", "")), "--range=" + sv(d, "ip_cidr_range", ""), "--region=" + sv(d, "region", cfg.Region)}
			if bv(d, "private_ip_google_access", false) {
				args = append(args, "--enable-private-ip-google-access")
			}
			return s.tfExec(cfg.proj(d), args...)
		},
		destroy: func(s *Session, cfg *tfConfig, id string) error {
			parts := strings.Split(id, "/")
			return s.tfExec(parts[1], "compute", "networks", "subnets", "delete", parts[5], "--region="+parts[3])
		},
	}
	fwRules := func(d map[string]any) (string, string) {
		action := "ALLOW"
		bl := blks(d, "allow")
		if len(bl) == 0 {
			bl = blks(d, "deny")
			action = "DENY"
		}
		var rs []string
		for _, r := range bl {
			ports := lv(r, "ports")
			if len(ports) == 0 {
				rs = append(rs, sv(r, "protocol", ""))
			}
			for _, pt := range ports {
				rs = append(rs, sv(r, "protocol", "")+":"+pt)
			}
		}
		sort.Strings(rs)
		return action, strings.Join(rs, ",")
	}
	tfTypes["google_compute_firewall"] = &tfType{required: []string{"name", "network"},
		id:       func(cfg *tfConfig, d map[string]any) string { return "projects/" + cfg.proj(d) + "/global/firewalls/" + sv(d, "name", "") },
		computed: func(cfg *tfConfig, d map[string]any) map[string]any { return tfSelf(cfg, d, "global/firewalls/"+sv(d, "name", "")) },
		norm: func(cfg *tfConfig, d map[string]any) map[string]any {
			a, r := fwRules(d)
			src := lv(d, "source_ranges")
			dir := strings.ToUpper(sv(d, "direction", "INGRESS"))
			if len(src) == 0 && len(lv(d, "source_tags")) == 0 && len(lv(d, "source_service_accounts")) == 0 && dir == "INGRESS" {
				src = []string{"0.0.0.0/0"}
			}
			return map[string]any{"action": a, "rules": r, "network": base(sv(d, "network", "")), "direction": dir, "priority": sv(d, "priority", "1000"),
				"source_ranges": src, "target_tags": lv(d, "target_tags"), "source_tags": lv(d, "source_tags")}
		},
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			parts := strings.Split(id, "/")
			p := s.State.Projects[parts[1]]
			if p == nil || p.Firewalls[parts[4]] == nil {
				return nil, false
			}
			fw := p.Firewalls[parts[4]]
			var rs []string
			for _, r := range fw.Rules {
				if len(r.Ports) == 0 {
					rs = append(rs, r.Protocol)
				}
				for _, pt := range r.Ports {
					rs = append(rs, r.Protocol+":"+pt)
				}
			}
			sort.Strings(rs)
			srt := func(l []string) []string { c := append([]string{}, l...); sort.Strings(c); return c }
			return map[string]any{"action": fw.Action, "rules": strings.Join(rs, ","), "network": fw.Network, "direction": fw.Direction, "priority": fmt.Sprint(fw.Priority),
				"source_ranges": srt(fw.SourceRanges), "target_tags": srt(fw.TargetTags), "source_tags": srt(fw.SourceTags)}, true
		},
		create: func(s *Session, cfg *tfConfig, d map[string]any) error {
			a, r := fwRules(d)
			args := []string{"compute", "firewall-rules", "create", sv(d, "name", ""), "--network=" + base(sv(d, "network", "")), "--direction=" + sv(d, "direction", "INGRESS"), "--priority=" + sv(d, "priority", "1000"), "--action=" + a, "--rules=" + r}
			if l := lv(d, "source_ranges"); len(l) > 0 {
				args = append(args, "--source-ranges="+strings.Join(l, ","))
			}
			if l := lv(d, "target_tags"); len(l) > 0 {
				args = append(args, "--target-tags="+strings.Join(l, ","))
			}
			if l := lv(d, "source_tags"); len(l) > 0 {
				args = append(args, "--source-tags="+strings.Join(l, ","))
			}
			if l := lv(d, "destination_ranges"); len(l) > 0 {
				args = append(args, "--destination-ranges="+strings.Join(l, ","))
			}
			return s.tfExec(cfg.proj(d), args...)
		},
		destroy: func(s *Session, cfg *tfConfig, id string) error {
			parts := strings.Split(id, "/")
			return s.tfExec(parts[1], "compute", "firewall-rules", "delete", parts[4], "--quiet")
		},
	}
	tfTypes["google_compute_instance"] = &tfType{required: []string{"name", "machine_type", "boot_disk", "network_interface"},
		id: func(cfg *tfConfig, d map[string]any) string {
			return "projects/" + cfg.proj(d) + "/zones/" + sv(d, "zone", cfg.Zone) + "/instances/" + sv(d, "name", "")
		},
		computed: func(cfg *tfConfig, d map[string]any) map[string]any {
			m := tfSelf(cfg, d, "zones/"+sv(d, "zone", cfg.Zone)+"/instances/"+sv(d, "name", ""))
			m["instance_id"] = "(known after apply)"
			return m
		},
		norm: func(cfg *tfConfig, d map[string]any) map[string]any {
			ni := blk(d, "network_interface")
			script := sv(d, "metadata_startup_script", "")
			if md, ok := d["metadata"].(map[string]any); ok && script == "" {
				script = sv(md, "startup-script", "")
			}
			sa := ""
			if b := blk(d, "service_account"); b != nil {
				sa = sv(b, "email", "")
			}
			return map[string]any{"machine_type": sv(d, "machine_type", ""), "tags": lv(d, "tags"), "subnetwork": base(sv(ni, "subnetwork", "")), "external_ip": len(blks(ni, "access_config")) > 0, "startup": strings.TrimSpace(script), "sa": sa}
		},
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			parts := strings.Split(id, "/")
			p := s.State.Projects[parts[1]]
			if p == nil || p.Instances[parts[5]] == nil {
				return nil, false
			}
			vm := p.Instances[parts[5]]
			tags := append([]string{}, vm.Tags...)
			sort.Strings(tags)
			sa := vm.ServiceAccount
			if strings.HasSuffix(sa, "-compute@developer.gserviceaccount.com") {
				sa = ""
			}
			return map[string]any{"machine_type": vm.MachineType, "tags": tags, "subnetwork": vm.Subnet, "external_ip": vm.ExternalIP != "", "startup": strings.TrimSpace(vm.Metadata["startup-script"]), "sa": sa}, true
		},
		create: func(s *Session, cfg *tfConfig, d map[string]any) error {
			ni := blk(d, "network_interface")
			args := []string{"compute", "instances", "create", sv(d, "name", ""), "--zone=" + sv(d, "zone", cfg.Zone), "--machine-type=" + sv(d, "machine_type", "")}
			if sn := sv(ni, "subnetwork", ""); sn != "" {
				args = append(args, "--subnet="+base(sn))
			} else {
				args = append(args, "--network="+base(sv(ni, "network", "default")))
			}
			if len(blks(ni, "access_config")) == 0 {
				args = append(args, "--no-address")
			}
			if t := lv(d, "tags"); len(t) > 0 {
				args = append(args, "--tags="+strings.Join(t, ","))
			}
			if bd := blk(d, "boot_disk"); bd != nil {
				if ip := blk(bd, "initialize_params"); ip != nil {
					img := base(sv(ip, "image", "debian-12"))
					args = append(args, "--image-family="+strings.TrimSuffix(img, "-v20260901"))
				}
			}
			script := sv(d, "metadata_startup_script", "")
			if md, ok := d["metadata"].(map[string]any); ok {
				for k, v := range md {
					if k == "startup-script" {
						script = fmt.Sprint(v)
					} else {
						args = append(args, "--metadata="+k+"="+fmt.Sprint(v))
					}
				}
			}
			if script != "" {
				s.Files[".tf-startup-"+sv(d, "name", "")] = script
				args = append(args, "--metadata-from-file=startup-script=.tf-startup-"+sv(d, "name", ""))
			}
			if b := blk(d, "service_account"); b != nil {
				if e := sv(b, "email", ""); e != "" {
					args = append(args, "--service-account="+e)
				}
				if sc := lv(b, "scopes"); len(sc) > 0 {
					args = append(args, "--scopes="+strings.Join(sc, ","))
				}
			}
			if l, ok := d["labels"].(map[string]any); ok {
				var kv []string
				for k, v := range l {
					kv = append(kv, k+"="+fmt.Sprint(v))
				}
				args = append(args, "--labels="+strings.Join(kv, ","))
			}
			err := s.tfExec(cfg.proj(d), args...)
			delete(s.Files, ".tf-startup-"+sv(d, "name", ""))
			return err
		},
		destroy: func(s *Session, cfg *tfConfig, id string) error {
			parts := strings.Split(id, "/")
			return s.tfExec(parts[1], "compute", "instances", "delete", parts[5], "--zone="+parts[3], "--quiet")
		},
	}
	tfTypes["google_compute_router"] = &tfType{required: []string{"name", "network"},
		id: func(cfg *tfConfig, d map[string]any) string {
			return "projects/" + cfg.proj(d) + "/regions/" + sv(d, "region", cfg.Region) + "/routers/" + sv(d, "name", "")
		},
		computed: func(cfg *tfConfig, d map[string]any) map[string]any {
			return tfSelf(cfg, d, "regions/"+sv(d, "region", cfg.Region)+"/routers/"+sv(d, "name", ""))
		},
		norm: func(cfg *tfConfig, d map[string]any) map[string]any {
			return map[string]any{"network": base(sv(d, "network", "")), "region": sv(d, "region", cfg.Region)}
		},
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			parts := strings.Split(id, "/")
			p := s.State.Projects[parts[1]]
			if p == nil || p.Routers[parts[5]] == nil {
				return nil, false
			}
			r := p.Routers[parts[5]]
			return map[string]any{"network": r.Network, "region": r.Region}, true
		},
		create: func(s *Session, cfg *tfConfig, d map[string]any) error {
			return s.tfExec(cfg.proj(d), "compute", "routers", "create", sv(d, "name", ""), "--network="+base(sv(d, "network", "")), "--region="+sv(d, "region", cfg.Region))
		},
		destroy: func(s *Session, cfg *tfConfig, id string) error {
			parts := strings.Split(id, "/")
			return s.tfExec(parts[1], "compute", "routers", "delete", parts[5], "--region="+parts[3])
		},
	}
	tfTypes["google_compute_router_nat"] = &tfType{required: []string{"name", "router", "nat_ip_allocate_option", "source_subnetwork_ip_ranges_to_nat"},
		id: func(cfg *tfConfig, d map[string]any) string {
			return cfg.proj(d) + "/" + sv(d, "region", cfg.Region) + "/" + base(sv(d, "router", "")) + "/" + sv(d, "name", "")
		},
		computed: func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{"name": sv(d, "name", "")} },
		norm: func(cfg *tfConfig, d map[string]any) map[string]any {
			return map[string]any{"all": sv(d, "source_subnetwork_ip_ranges_to_nat", "") == "ALL_SUBNETWORKS_ALL_IP_RANGES"}
		},
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			parts := strings.Split(id, "/")
			p := s.State.Projects[parts[0]]
			if p == nil || p.Routers[parts[2]] == nil {
				return nil, false
			}
			for _, n := range p.Routers[parts[2]].NATs {
				if n.Name == parts[3] {
					return map[string]any{"all": n.AllSubnets}, true
				}
			}
			return nil, false
		},
		create: func(s *Session, cfg *tfConfig, d map[string]any) error {
			args := []string{"compute", "routers", "nats", "create", sv(d, "name", ""), "--router=" + base(sv(d, "router", "")), "--region=" + sv(d, "region", cfg.Region), "--auto-allocate-nat-external-ips"}
			if sv(d, "source_subnetwork_ip_ranges_to_nat", "") == "ALL_SUBNETWORKS_ALL_IP_RANGES" {
				args = append(args, "--nat-all-subnet-ip-ranges")
			} else {
				var sns []string
				for _, sn := range blks(d, "subnetwork") {
					sns = append(sns, base(sv(sn, "name", "")))
				}
				args = append(args, "--nat-custom-subnet-ip-ranges="+strings.Join(sns, ","))
			}
			return s.tfExec(cfg.proj(d), args...)
		},
		destroy: func(s *Session, cfg *tfConfig, id string) error {
			parts := strings.Split(id, "/")
			return s.tfExec(parts[0], "compute", "routers", "nats", "delete", parts[3], "--router="+parts[2], "--region="+parts[1])
		},
	}
	tfTypes["google_compute_address"] = &tfType{required: []string{"name"},
		id: func(cfg *tfConfig, d map[string]any) string {
			return "projects/" + cfg.proj(d) + "/regions/" + sv(d, "region", cfg.Region) + "/addresses/" + sv(d, "name", "")
		},
		computed: func(cfg *tfConfig, d map[string]any) map[string]any {
			m := tfSelf(cfg, d, "regions/"+sv(d, "region", cfg.Region)+"/addresses/"+sv(d, "name", ""))
			m["address"] = "(known after apply)"
			return m
		},
		norm: func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{} },
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			parts := strings.Split(id, "/")
			p := s.State.Projects[parts[1]]
			return map[string]any{}, p != nil && p.Addresses[parts[5]] != nil
		},
		create: func(s *Session, cfg *tfConfig, d map[string]any) error {
			return s.tfExec(cfg.proj(d), "compute", "addresses", "create", sv(d, "name", ""), "--region="+sv(d, "region", cfg.Region))
		},
		destroy: func(s *Session, cfg *tfConfig, id string) error {
			parts := strings.Split(id, "/")
			return s.tfExec(parts[1], "compute", "addresses", "delete", parts[5])
		},
	}
	tfTypes["google_storage_bucket"] = &tfType{required: []string{"name", "location"},
		id:       func(cfg *tfConfig, d map[string]any) string { return sv(d, "name", "") },
		computed: func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{"id": sv(d, "name", ""), "url": "gs://" + sv(d, "name", ""), "name": sv(d, "name", "")} },
		norm: func(cfg *tfConfig, d map[string]any) map[string]any {
			ver := false
			if v := blk(d, "versioning"); v != nil {
				ver = bv(v, "enabled", false)
			}
			return map[string]any{"location": strings.ToUpper(sv(d, "location", "")), "storage_class": strings.ToUpper(sv(d, "storage_class", "STANDARD")), "ubla": bv(d, "uniform_bucket_level_access", false),
				"pap": sv(d, "public_access_prevention", "inherited"), "versioning": ver, "lifecycle_rules": len(blks(d, "lifecycle_rule"))}
		},
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			b, _ := s.State.FindBucket(id)
			if b == nil {
				return nil, false
			}
			return map[string]any{"location": b.Location, "storage_class": b.StorageClass, "ubla": b.UBLA, "pap": b.PAP, "versioning": b.Versioning, "lifecycle_rules": len(b.Lifecycle)}, true
		},
		create: func(s *Session, cfg *tfConfig, d map[string]any) error {
			n := sv(d, "name", "")
			args := []string{"storage", "buckets", "create", "gs://" + n, "--location=" + sv(d, "location", ""), "--default-storage-class=" + sv(d, "storage_class", "STANDARD")}
			if bv(d, "uniform_bucket_level_access", false) {
				args = append(args, "--uniform-bucket-level-access")
			}
			if sv(d, "public_access_prevention", "") == "enforced" {
				args = append(args, "--public-access-prevention")
			}
			if v := blk(d, "versioning"); v != nil && bv(v, "enabled", false) {
				args = append(args, "--versioning")
			}
			if rules := blks(d, "lifecycle_rule"); len(rules) > 0 {
				var lr []map[string]any
				for _, r := range rules {
					cond := map[string]any{}
					if c := blk(r, "condition"); c != nil {
						for k, v := range c {
							switch k {
							case "age":
								cond["age"] = v
							case "num_newer_versions":
								cond["numNewerVersions"] = v
							case "with_state":
								cond["isLive"] = v == "LIVE"
							case "matches_storage_class":
								cond["matchesStorageClass"] = v
							}
						}
					}
					act := map[string]any{}
					if a := blk(r, "action"); a != nil {
						act["type"] = sv(a, "type", "")
						if c := sv(a, "storage_class", ""); c != "" {
							act["storageClass"] = c
						}
					}
					lr = append(lr, map[string]any{"action": act, "condition": cond})
				}
				j, _ := json.Marshal(map[string]any{"rule": lr})
				s.Files[".tf-lifecycle-"+n+".json"] = string(j)
				args = append(args, "--lifecycle-file=.tf-lifecycle-"+n+".json")
			}
			err := s.tfExec(cfg.proj(d), args...)
			delete(s.Files, ".tf-lifecycle-"+n+".json")
			return err
		},
		destroy: func(s *Session, cfg *tfConfig, id string) error {
			b, p := s.State.FindBucket(id)
			if b != nil && len(b.Objects) > 0 {
				return fmt.Errorf("Error trying to delete bucket %s containing objects without `force_destroy` set to true", id)
			}
			return s.tfExec(p.ID, "storage", "buckets", "delete", "gs://"+id)
		},
	}
	tfTypes["google_service_account"] = &tfType{required: []string{"account_id"},
		id: func(cfg *tfConfig, d map[string]any) string {
			return sv(d, "account_id", "") + "@" + cfg.proj(d) + ".iam.gserviceaccount.com"
		},
		computed: func(cfg *tfConfig, d map[string]any) map[string]any {
			e := sv(d, "account_id", "") + "@" + cfg.proj(d) + ".iam.gserviceaccount.com"
			return map[string]any{"email": e, "member": "serviceAccount:" + e, "id": "projects/" + cfg.proj(d) + "/serviceAccounts/" + e, "name": "projects/" + cfg.proj(d) + "/serviceAccounts/" + e}
		},
		norm: func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{"display_name": sv(d, "display_name", "")} },
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			for _, p := range s.State.Projects {
				if sa := p.ServiceAccounts[id]; sa != nil {
					return map[string]any{"display_name": sa.DisplayName}, true
				}
			}
			return nil, false
		},
		create: func(s *Session, cfg *tfConfig, d map[string]any) error {
			return s.tfExec(cfg.proj(d), "iam", "service-accounts", "create", sv(d, "account_id", ""), "--display-name="+sv(d, "display_name", ""))
		},
		destroy: func(s *Session, cfg *tfConfig, id string) error {
			return s.tfExec(strings.TrimSuffix(strings.SplitN(id, "@", 2)[1], ".iam.gserviceaccount.com"), "iam", "service-accounts", "delete", id, "--quiet")
		},
	}
	tfTypes["google_project_iam_member"] = &tfType{required: []string{"role", "member"},
		id:       func(cfg *tfConfig, d map[string]any) string { return cfg.proj(d) + "/" + sv(d, "role", "") + "/" + sv(d, "member", "") },
		computed: func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{"id": cfg.proj(d) + "/" + sv(d, "role", "") + "/" + sv(d, "member", "")} },
		norm:     func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{} },
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			parts := strings.SplitN(id, "/", 2)
			p := s.State.Projects[parts[0]]
			i := strings.LastIndex(parts[1], "/")
			return map[string]any{}, p != nil && p.IAM.HasMember(parts[1][:i], parts[1][i+1:])
		},
		create: func(s *Session, cfg *tfConfig, d map[string]any) error {
			return s.tfExec(cfg.proj(d), "projects", "add-iam-policy-binding", cfg.proj(d), "--member="+sv(d, "member", ""), "--role="+sv(d, "role", ""))
		},
		destroy: func(s *Session, cfg *tfConfig, id string) error {
			parts := strings.SplitN(id, "/", 2)
			i := strings.LastIndex(parts[1], "/")
			return s.tfExec(parts[0], "projects", "remove-iam-policy-binding", parts[0], "--member="+parts[1][i+1:], "--role="+parts[1][:i])
		},
	}
	tfTypes["google_storage_bucket_iam_member"] = &tfType{required: []string{"bucket", "role", "member"},
		id: func(cfg *tfConfig, d map[string]any) string {
			return strings.TrimPrefix(sv(d, "bucket", ""), "gs://") + "|" + sv(d, "role", "") + "|" + sv(d, "member", "")
		},
		computed: func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{} },
		norm:     func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{} },
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			parts := strings.Split(id, "|")
			b, _ := s.State.FindBucket(parts[0])
			return map[string]any{}, b != nil && b.IAM.HasMember(parts[1], parts[2])
		},
		create: func(s *Session, cfg *tfConfig, d map[string]any) error {
			return s.tfExec(cfg.proj(d), "storage", "buckets", "add-iam-policy-binding", "gs://"+strings.TrimPrefix(sv(d, "bucket", ""), "gs://"), "--member="+sv(d, "member", ""), "--role="+sv(d, "role", ""))
		},
		destroy: func(s *Session, cfg *tfConfig, id string) error {
			parts := strings.Split(id, "|")
			return s.tfExec(cfg.Project, "storage", "buckets", "remove-iam-policy-binding", "gs://"+parts[0], "--member="+parts[2], "--role="+parts[1])
		},
	}
	runIngress := map[string]string{"INGRESS_TRAFFIC_ALL": "all", "INGRESS_TRAFFIC_INTERNAL_ONLY": "internal", "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER": "internal-and-cloud-load-balancing"}
	tfTypes["google_cloud_run_v2_service"] = &tfType{required: []string{"name", "location", "template"},
		id: func(cfg *tfConfig, d map[string]any) string {
			return "projects/" + cfg.proj(d) + "/locations/" + sv(d, "location", "") + "/services/" + sv(d, "name", "")
		},
		computed: func(cfg *tfConfig, d map[string]any) map[string]any {
			return map[string]any{"id": "projects/" + cfg.proj(d) + "/locations/" + sv(d, "location", "") + "/services/" + sv(d, "name", ""), "name": sv(d, "name", ""), "uri": "(known after apply)", "location": sv(d, "location", "")}
		},
		norm: func(cfg *tfConfig, d map[string]any) map[string]any {
			t := blk(d, "template")
			ct := blk(t, "containers")
			env := map[string]string{}
			for _, e := range blks(ct, "env") {
				env[sv(e, "name", "")] = sv(e, "value", "")
			}
			ing := runIngress[sv(d, "ingress", "INGRESS_TRAFFIC_ALL")]
			return map[string]any{"image": sv(ct, "image", ""), "env": fmt.Sprint(env), "sa": sv(t, "service_account", ""), "ingress": ing}
		},
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			parts := strings.Split(id, "/")
			p := s.State.Projects[parts[1]]
			if p == nil || p.RunServices[parts[5]] == nil {
				return nil, false
			}
			svc := p.RunServices[parts[5]]
			sa := svc.SA
			return map[string]any{"image": svc.Image, "env": fmt.Sprint(svc.Env), "sa": sa, "ingress": svc.Ingress}, true
		},
		create: func(s *Session, cfg *tfConfig, d map[string]any) error {
			t := blk(d, "template")
			ct := blk(t, "containers")
			args := []string{"run", "deploy", sv(d, "name", ""), "--image=" + sv(ct, "image", ""), "--region=" + sv(d, "location", ""), "--ingress=" + runIngress[sv(d, "ingress", "INGRESS_TRAFFIC_ALL")]}
			var env []string
			for _, e := range blks(ct, "env") {
				env = append(env, sv(e, "name", "")+"="+sv(e, "value", ""))
			}
			if len(env) > 0 {
				args = append(args, "--set-env-vars=^|^"+strings.Join(env, "|"))
			}
			if sa := sv(t, "service_account", ""); sa != "" {
				args = append(args, "--service-account="+sa)
			}
			return s.tfExec(cfg.proj(d), args...)
		},
		destroy: func(s *Session, cfg *tfConfig, id string) error {
			parts := strings.Split(id, "/")
			return s.tfExec(parts[1], "run", "services", "delete", parts[5], "--region="+parts[3], "--quiet")
		},
	}
	runIAMType := &tfType{required: []string{"location", "role", "member"},
		id: func(cfg *tfConfig, d map[string]any) string {
			return cfg.proj(d) + "|" + base(sv(d, "name", sv(d, "service", ""))) + "|" + sv(d, "role", "") + "|" + sv(d, "member", "")
		},
		computed: func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{} },
		norm:     func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{} },
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			parts := strings.Split(id, "|")
			p := s.State.Projects[parts[0]]
			if p == nil || p.RunServices[parts[1]] == nil {
				return nil, false
			}
			return map[string]any{}, p.RunServices[parts[1]].IAM.HasMember(parts[2], parts[3])
		},
		create: func(s *Session, cfg *tfConfig, d map[string]any) error {
			return s.tfExec(cfg.proj(d), "run", "services", "add-iam-policy-binding", base(sv(d, "name", sv(d, "service", ""))), "--region="+sv(d, "location", ""), "--member="+sv(d, "member", ""), "--role="+sv(d, "role", ""))
		},
		destroy: func(s *Session, cfg *tfConfig, id string) error {
			parts := strings.Split(id, "|")
			return s.tfExec(parts[0], "run", "services", "remove-iam-policy-binding", parts[1], "--member="+parts[3], "--role="+parts[2])
		},
	}
	tfTypes["google_cloud_run_v2_service_iam_member"] = runIAMType
	tfTypes["google_cloud_run_service_iam_member"] = runIAMType
	tfTypes["google_pubsub_topic"] = &tfType{required: []string{"name"},
		id:       func(cfg *tfConfig, d map[string]any) string { return "projects/" + cfg.proj(d) + "/topics/" + sv(d, "name", "") },
		computed: func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{"id": "projects/" + cfg.proj(d) + "/topics/" + sv(d, "name", ""), "name": sv(d, "name", "")} },
		norm:     func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{} },
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			parts := strings.Split(id, "/")
			p := s.State.Projects[parts[1]]
			return map[string]any{}, p != nil && p.Topics[parts[3]] != nil
		},
		create: func(s *Session, cfg *tfConfig, d map[string]any) error {
			return s.tfExec(cfg.proj(d), "pubsub", "topics", "create", sv(d, "name", ""))
		},
		destroy: func(s *Session, cfg *tfConfig, id string) error {
			parts := strings.Split(id, "/")
			return s.tfExec(parts[1], "pubsub", "topics", "delete", parts[3])
		},
	}
	tfTypes["google_pubsub_subscription"] = &tfType{required: []string{"name", "topic"},
		id:       func(cfg *tfConfig, d map[string]any) string { return "projects/" + cfg.proj(d) + "/subscriptions/" + sv(d, "name", "") },
		computed: func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{"id": "projects/" + cfg.proj(d) + "/subscriptions/" + sv(d, "name", ""), "name": sv(d, "name", "")} },
		norm: func(cfg *tfConfig, d map[string]any) map[string]any {
			push := ""
			if pc := blk(d, "push_config"); pc != nil {
				push = sv(pc, "push_endpoint", "")
			}
			dlq := ""
			if dl := blk(d, "dead_letter_policy"); dl != nil {
				dlq = base(sv(dl, "dead_letter_topic", ""))
			}
			return map[string]any{"topic": base(sv(d, "topic", "")), "push": push, "dlq": dlq}
		},
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			parts := strings.Split(id, "/")
			p := s.State.Projects[parts[1]]
			if p == nil || p.Subs[parts[3]] == nil {
				return nil, false
			}
			sb := p.Subs[parts[3]]
			return map[string]any{"topic": sb.Topic, "push": sb.PushEndpoint, "dlq": sb.DeadLetterTopic}, true
		},
		create: func(s *Session, cfg *tfConfig, d map[string]any) error {
			args := []string{"pubsub", "subscriptions", "create", sv(d, "name", ""), "--topic=" + base(sv(d, "topic", "")), "--ack-deadline=" + sv(d, "ack_deadline_seconds", "10")}
			if pc := blk(d, "push_config"); pc != nil {
				args = append(args, "--push-endpoint="+sv(pc, "push_endpoint", ""))
				if o := blk(pc, "oidc_token"); o != nil {
					args = append(args, "--push-auth-service-account="+sv(o, "service_account_email", ""))
				}
			}
			if dl := blk(d, "dead_letter_policy"); dl != nil {
				args = append(args, "--dead-letter-topic="+base(sv(dl, "dead_letter_topic", "")), "--max-delivery-attempts="+sv(dl, "max_delivery_attempts", "5"))
			}
			return s.tfExec(cfg.proj(d), args...)
		},
		destroy: func(s *Session, cfg *tfConfig, id string) error {
			parts := strings.Split(id, "/")
			return s.tfExec(parts[1], "pubsub", "subscriptions", "delete", parts[3])
		},
	}
	tfTypes["google_artifact_registry_repository"] = &tfType{required: []string{"repository_id", "format"},
		id: func(cfg *tfConfig, d map[string]any) string {
			return "projects/" + cfg.proj(d) + "/locations/" + sv(d, "location", cfg.Region) + "/repositories/" + sv(d, "repository_id", "")
		},
		computed: func(cfg *tfConfig, d map[string]any) map[string]any {
			return map[string]any{"id": "projects/" + cfg.proj(d) + "/locations/" + sv(d, "location", cfg.Region) + "/repositories/" + sv(d, "repository_id", ""), "name": sv(d, "repository_id", "")}
		},
		norm: func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{"format": strings.ToUpper(sv(d, "format", ""))} },
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			parts := strings.Split(id, "/")
			p := s.State.Projects[parts[1]]
			if p == nil || p.ArtifactRepos[parts[5]] == nil {
				return nil, false
			}
			return map[string]any{"format": p.ArtifactRepos[parts[5]].Format}, true
		},
		create: func(s *Session, cfg *tfConfig, d map[string]any) error {
			return s.tfExec(cfg.proj(d), "artifacts", "repositories", "create", sv(d, "repository_id", ""), "--repository-format="+strings.ToLower(sv(d, "format", "")), "--location="+sv(d, "location", cfg.Region))
		},
		destroy: func(s *Session, cfg *tfConfig, id string) error {
			parts := strings.Split(id, "/")
			delete(s.State.Projects[parts[1]].ArtifactRepos, parts[5])
			return nil
		},
	}
	tfTypes["google_secret_manager_secret"] = &tfType{required: []string{"secret_id"},
		id:       func(cfg *tfConfig, d map[string]any) string { return "projects/" + cfg.proj(d) + "/secrets/" + sv(d, "secret_id", "") },
		computed: func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{"id": "projects/" + cfg.proj(d) + "/secrets/" + sv(d, "secret_id", ""), "secret_id": sv(d, "secret_id", "")} },
		norm:     func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{} },
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			parts := strings.Split(id, "/")
			p := s.State.Projects[parts[1]]
			return map[string]any{}, p != nil && p.Secrets[parts[3]] != nil
		},
		create: func(s *Session, cfg *tfConfig, d map[string]any) error {
			return s.tfExec(cfg.proj(d), "secrets", "create", sv(d, "secret_id", ""), "--replication-policy=automatic")
		},
		destroy: func(s *Session, cfg *tfConfig, id string) error {
			parts := strings.Split(id, "/")
			return s.tfExec(parts[1], "secrets", "delete", parts[3], "--quiet")
		},
	}
	tfTypes["google_sql_database_instance"] = &tfType{required: []string{"name", "database_version", "settings"},
		id:       func(cfg *tfConfig, d map[string]any) string { return cfg.proj(d) + "/" + sv(d, "name", "") },
		computed: func(cfg *tfConfig, d map[string]any) map[string]any { return map[string]any{"name": sv(d, "name", ""), "connection_name": cfg.proj(d) + ":" + sv(d, "region", cfg.Region) + ":" + sv(d, "name", ""), "private_ip_address": "(known after apply)"} },
		norm: func(cfg *tfConfig, d map[string]any) map[string]any {
			st := blk(d, "settings")
			return map[string]any{"tier": sv(st, "tier", ""), "availability": sv(st, "availability_type", "ZONAL")}
		},
		read: func(s *Session, cfg *tfConfig, id string) (map[string]any, bool) {
			parts := strings.Split(id, "/")
			p := s.State.Projects[parts[0]]
			if p == nil || p.SQLInstances[parts[1]] == nil {
				return nil, false
			}
			in := p.SQLInstances[parts[1]]
			return map[string]any{"tier": in.Tier, "availability": in.Availability}, true
		},
		create: func(s *Session, cfg *tfConfig, d map[string]any) error {
			st := blk(d, "settings")
			args := []string{"sql", "instances", "create", sv(d, "name", ""), "--database-version=" + sv(d, "database_version", ""), "--region=" + sv(d, "region", cfg.Region), "--tier=" + sv(st, "tier", "db-f1-micro"), "--availability-type=" + sv(st, "availability_type", "ZONAL")}
			if ip := blk(st, "ip_configuration"); ip != nil {
				if !bv(ip, "ipv4_enabled", true) {
					args = append(args, "--no-assign-ip")
				}
				if pn := sv(ip, "private_network", ""); pn != "" {
					args = append(args, "--network="+base(pn))
				}
			}
			if bc := blk(st, "backup_configuration"); bc != nil && bv(bc, "enabled", false) {
				args = append(args, "--backup-start-time="+sv(bc, "start_time", "03:00"))
			}
			return s.tfExec(cfg.proj(d), args...)
		},
		destroy: func(s *Session, cfg *tfConfig, id string) error {
			parts := strings.Split(id, "/")
			return s.tfExec(parts[0], "sql", "instances", "delete", parts[1], "--quiet")
		},
	}
}

func bodyVars(b *hclsyntax.Body) []hcl.Traversal {
	var out []hcl.Traversal
	for _, a := range b.Attributes {
		out = append(out, a.Expr.Variables()...)
	}
	for _, bl := range b.Blocks {
		out = append(out, bodyVars(bl.Body)...)
	}
	return out
}
