package cli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

// Col is a table column: header and field path.
type Col struct{ H, P string }

// Table is a list result rendered according to --format.
type Table struct {
	Cols []Col
	Rows []any
}

// Obj is a single resource rendered as YAML by default.
type Obj struct{ V any }

// Flags holds parsed command-line flags.
type Flags map[string][]string

var boolFlags = map[string]bool{
	"quiet": true, "table": true, "dataset": true, "q": true, "global": true, "async": true, "uniform-bucket-level-access": true, "allow-unauthenticated": true,
	"auto-allocate-nat-external-ips": true, "nat-all-subnet-ip-ranges": true, "versioning": true, "auto-ack": true,
	"dry-run": true, "dry_run": true, "tunnel-through-iap": true, "spot": true, "preemptible": true, "all": true, "recursive": true,
	"r": true, "R": true, "require-ssl": true, "to-latest": true, "preview": true, "deletion-protection": true, "shielded-secure-boot": true,
	"autopilot": true, "use_legacy_sql": true, "nouse_legacy_sql": true, "force": true, "d": true, "t": true, "enable-cdn": true,
	"internal-ip": true, "log-http": true, "no-user-output-enabled": true, "watch": true, "w": true, "A": true, "all-namespaces": true,
	"auto-approve": true, "sort-by-created": true, "verbose": true, "include-deleted": true, "json": true, "stream": true,
	"enable-autorepair": true, "enable-autoupgrade": true, "private-network-only": true, "disable-default-snat": false,
	"set-as-default": true, "can-ip-forward": true, "headless": true, "update-credentials": true, "rm": true, "stdin": true,
	"assign-ip": true, "retain-backups-on-delete": true, "no-traffic": true, "iap": true, "ha": true, "default-internet-gateway": false,
	"include-all-scopes": true, "cloud-sql-proxy": true, "insecure": true, "k": true, "s": true, "silent": true, "i": true, "sS": true,
	"v": true, "I": true, "fsSL": true, "fsS": true, "L": true, "use-http2": true, "load-balancing-scheme-internal": true,
	"expand-ip-range": false, "raw": true, "no-color": true, "input": false, "bgp-routing": false, "enable-logging": true, "wait": true, "p": true,
}

var valueFlagsWithBoolPrefix = map[string]bool{"remove-env-vars": true, "remove-secrets": true, "remove-labels": true, "remove-tags": true, "remove-metadata": true, "clear-labels": true, "remove-cloudsql-instances": true, "remove-flags": false, "enable-private-endpoint": true}

func isBoolFlag(name string) bool {
	if valueFlagsWithBoolPrefix[name] && name != "clear-labels" && name != "enable-private-endpoint" {
		return false
	}
	if v, ok := boolFlags[name]; ok {
		return v
	}
	for _, p := range []string{"no-", "enable-", "disable-", "clear-"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// parseArgs splits positional arguments and flags.
func parseArgs(args []string) ([]string, Flags) {
	var pos []string
	f := Flags{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "--") && len(a) > 2 {
			name := a[2:]
			if k, v, ok := strings.Cut(name, "="); ok {
				f[k] = append(f[k], v)
				continue
			}
			if isBoolFlag(name) || i+1 >= len(args) || (strings.HasPrefix(args[i+1], "--") && len(args[i+1]) > 2) {
				f[name] = append(f[name], "true")
				continue
			}
			f[name] = append(f[name], args[i+1])
			i++
			continue
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 && !isNumber(a) {
			name := a[1:]
			if k, v, ok := strings.Cut(name, "="); ok {
				f[k] = append(f[k], v)
				continue
			}
			if isBoolFlag(name) || i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") && !isNumber(args[i+1]) {
				f[name] = append(f[name], "true")
				continue
			}
			f[name] = append(f[name], args[i+1])
			i++
			continue
		}
		pos = append(pos, a)
	}
	return pos, f
}

func isNumber(s string) bool { _, err := strconv.ParseFloat(s, 64); return err == nil }

// toAny converts structs to generic JSON values.
func toAny(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

// getPath resolves a dotted path, flattening lists.
func getPath(v any, path string) []any {
	path = strings.TrimSpace(path)
	if path == "" {
		return []any{v}
	}
	fn := ""
	if m := regexp.MustCompile(`^(.*)\.(basename|scope|len|join|list)\(\)$`).FindStringSubmatch(path); m != nil {
		path, fn = m[1], m[2]
	}
	parts := strings.Split(path, ".")
	cur := []any{v}
	for _, p := range parts {
		p = strings.TrimSuffix(p, "[]")
		idx := -1
		if m := regexp.MustCompile(`^(.*)\[(\d+)\]$`).FindStringSubmatch(p); m != nil {
			p = m[1]
			idx, _ = strconv.Atoi(m[2])
		}
		var next []any
		for _, c := range cur {
			switch cv := c.(type) {
			case map[string]any:
				x, ok := cv[p]
				if !ok {
					continue
				}
				if arr, ok := x.([]any); ok {
					if idx >= 0 {
						if idx < len(arr) {
							next = append(next, arr[idx])
						}
					} else {
						next = append(next, arr...)
					}
				} else {
					next = append(next, x)
				}
			case []any:
				for _, e := range cv {
					if m, ok := e.(map[string]any); ok {
						if x, ok := m[p]; ok {
							next = append(next, x)
						}
					}
				}
			}
		}
		cur = next
	}
	if fn == "basename" {
		for i, c := range cur {
			s := fmt.Sprint(c)
			cur[i] = s[strings.LastIndex(s, "/")+1:]
		}
	}
	if fn == "list" || fn == "join" {
		return []any{scalar(cur, ",")}
	}
	if fn == "len" {
		return []any{len(cur)}
	}
	return cur
}

func scalar(vals []any, sep string) string {
	var parts []string
	for _, v := range vals {
		switch x := v.(type) {
		case nil:
		case map[string]any, []any:
			b, _ := json.Marshal(x)
			parts = append(parts, string(b))
		case float64:
			if x == float64(int64(x)) {
				parts = append(parts, strconv.FormatInt(int64(x), 10))
			} else {
				parts = append(parts, strconv.FormatFloat(x, 'f', -1, 64))
			}
		default:
			parts = append(parts, fmt.Sprint(x))
		}
	}
	return strings.Join(parts, sep)
}

// applyFilter evaluates a gcloud --filter expression on an item.
func applyFilter(item any, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	if strings.Contains(filter, " OR ") {
		for _, p := range strings.Split(filter, " OR ") {
			if applyFilter(item, p) {
				return true
			}
		}
		return false
	}
	for _, term := range regexp.MustCompile(`\s+AND\s+|\s+`).Split(filter, -1) {
		if term == "" {
			continue
		}
		neg := false
		if strings.HasPrefix(term, "NOT") || strings.HasPrefix(term, "-") {
			neg = true
			term = strings.TrimPrefix(strings.TrimPrefix(term, "NOT"), "-")
		}
		ok := false
		matched := false
		for _, op := range []string{"!=", ">=", "<=", "~", "=", ":", ">", "<"} {
			i := strings.Index(term, op)
			if i <= 0 {
				continue
			}
			matched = true
			key, want := term[:i], strings.Trim(term[i+len(op):], `"'()`)
			vals := getPath(item, key)
			for _, v := range vals {
				got := scalar([]any{v}, "")
				switch op {
				case "=":
					ok = ok || strings.EqualFold(got, want) || strings.HasSuffix(got, "/"+want)
				case "!=":
					ok = ok || !strings.EqualFold(got, want)
				case ":":
					ok = ok || strings.Contains(strings.ToLower(got), strings.ToLower(want))
				case "~":
					if re, err := regexp.Compile(want); err == nil {
						ok = ok || re.MatchString(got)
					}
				case ">", "<", ">=", "<=":
					a, e1 := strconv.ParseFloat(got, 64)
					b, e2 := strconv.ParseFloat(want, 64)
					if e1 == nil && e2 == nil {
						ok = ok || (op == ">" && a > b) || (op == "<" && a < b) || (op == ">=" && a >= b) || (op == "<=" && a <= b)
					}
				}
			}
			if len(vals) == 0 && op == "!=" {
				ok = true
			}
			break
		}
		if !matched {
			b, _ := json.Marshal(item)
			ok = strings.Contains(strings.ToLower(string(b)), strings.ToLower(term))
		}
		if ok == neg {
			return false
		}
	}
	return true
}

// flatten expands a list field into one row per element (--flatten).
func flatten(rows []any, path string) []any {
	var out []any
	for _, part := range strings.Split(path, ",") {
		part = strings.TrimSpace(part)
		segs := strings.Split(strings.ReplaceAll(part, "[]", ""), ".")
		rows2 := rows
		for depth := 1; depth <= len(segs); depth++ {
			var next []any
			for _, r := range rows2 {
				next = append(next, flattenOne(r, segs[:depth])...)
			}
			rows2 = next
		}
		out = rows2
		rows = rows2
	}
	return out
}

func flattenOne(row any, segs []string) []any {
	m, ok := row.(map[string]any)
	if !ok {
		return []any{row}
	}
	if len(segs) == 1 {
		arr, ok := m[segs[0]].([]any)
		if !ok {
			return []any{row}
		}
		var out []any
		for _, e := range arr {
			c := copyMapAny(m)
			c[segs[0]] = e
			out = append(out, c)
		}
		return out
	}
	child := m[segs[0]]
	var out []any
	for _, e := range flattenOne(child, segs[1:]) {
		c := copyMapAny(m)
		c[segs[0]] = e
		out = append(out, c)
	}
	return out
}

func copyMapAny(m map[string]any) map[string]any {
	c := map[string]any{}
	for k, v := range m {
		c[k] = v
	}
	return c
}

var reFormat = regexp.MustCompile(`^(json|yaml|value|table|csv|list|text|get|default|none)(?:\[([^\]]*)\])?(?:\((.*)\))?$`)

// render outputs a handler result according to --format.
func render(v any, f Flags) (string, error) {
	format := last(f["format"])
	switch x := v.(type) {
	case nil:
		return "", nil
	case string:
		return x, nil
	case Obj:
		item := toAny(x.V)
		if format == "" {
			format = "yaml"
		}
		return renderItems([]any{item}, nil, format, true)
	case Table:
		rows := x.Rows
		for i := range rows {
			rows[i] = toAny(rows[i])
		}
		if fl := last(f["flatten"]); fl != "" {
			rows = flatten(rows, fl)
		}
		if flt := last(f["filter"]); flt != "" {
			var kept []any
			for _, r := range rows {
				if applyFilter(r, flt) {
					kept = append(kept, r)
				}
			}
			rows = kept
		}
		if sb := last(f["sort-by"]); sb != "" {
			desc := strings.HasPrefix(sb, "~")
			key := strings.TrimPrefix(sb, "~")
			sort.SliceStable(rows, func(i, j int) bool {
				a, b := scalar(getPath(rows[i], key), ""), scalar(getPath(rows[j], key), "")
				if desc {
					return a > b
				}
				return a < b
			})
		}
		if l := last(f["limit"]); l != "" {
			n, _ := strconv.Atoi(l)
			if n > 0 && n < len(rows) {
				rows = rows[:n]
			}
		}
		if len(rows) == 0 && (format == "" || strings.HasPrefix(format, "table")) {
			return "Listed 0 items.\n", nil
		}
		if format == "" {
			format = "table"
		}
		return renderItems(rows, x.Cols, format, false)
	}
	return fmt.Sprint(v), nil
}

func last(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[len(v)-1]
}

func renderItems(rows []any, cols []Col, format string, single bool) (string, error) {
	m := reFormat.FindStringSubmatch(strings.TrimSpace(format))
	if m == nil {
		return "", fail(1, "ERROR: (gcloud) Format [%s] not supported by the simulator.", format)
	}
	kind, attrs, spec := m[1], m[2], m[3]
	var specCols []Col
	if spec != "" {
		for _, p := range splitSpec(spec) {
			h := p
			if i := strings.Index(p, ":label="); i >= 0 {
				h = p[i+7:]
				p = p[:i]
			} else if i := strings.Index(p, ":"); i >= 0 {
				p = p[:i]
				h = p
			}
			h = strings.ToUpper(strings.ReplaceAll(regexp.MustCompile(`\(.*\)|\.basename`).ReplaceAllString(h, ""), ".", "_"))
			specCols = append(specCols, Col{H: h, P: p})
		}
	}
	switch kind {
	case "none":
		return "", nil
	case "json":
		var b []byte
		if single {
			b, _ = json.MarshalIndent(rows[0], "", "  ")
		} else {
			if rows == nil {
				rows = []any{}
			}
			b, _ = json.MarshalIndent(rows, "", "  ")
		}
		return string(b) + "\n", nil
	case "yaml", "default", "list", "text":
		var sb strings.Builder
		for i, r := range rows {
			if i > 0 || !single {
				sb.WriteString("---\n")
			}
			b, _ := yaml.Marshal(r)
			sb.WriteString(string(b))
		}
		return sb.String(), nil
	case "value", "get", "csv":
		use := specCols
		if len(use) == 0 {
			use = cols
		}
		sep := "\t"
		if kind == "csv" {
			sep = ","
		}
		if strings.Contains(attrs, "separator=") {
			sep = strings.Trim(strings.SplitN(attrs, "separator=", 2)[1], `"'`)
			sep = strings.SplitN(sep, ",", 2)[0]
		}
		var sb strings.Builder
		if kind == "csv" && !strings.Contains(attrs, "no-heading") {
			var hs []string
			for _, c := range use {
				hs = append(hs, strings.ToLower(c.H))
			}
			sb.WriteString(strings.Join(hs, ",") + "\n")
		}
		for _, r := range rows {
			var vals []string
			for _, c := range use {
				vals = append(vals, scalar(getPath(r, c.P), ";"))
			}
			sb.WriteString(strings.Join(vals, sep) + "\n")
		}
		return sb.String(), nil
	case "table":
		use := specCols
		if len(use) == 0 {
			use = cols
		}
		if len(use) == 0 {
			use = []Col{{"NAME", "name"}}
		}
		var sb strings.Builder
		tw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
		var hs []string
		for _, c := range use {
			hs = append(hs, c.H)
		}
		if !strings.Contains(attrs, "no-heading") {
			fmt.Fprintln(tw, strings.Join(hs, "\t"))
		}
		for _, r := range rows {
			var vals []string
			for _, c := range use {
				vals = append(vals, scalar(getPath(r, c.P), ","))
			}
			fmt.Fprintln(tw, strings.Join(vals, "\t"))
		}
		tw.Flush()
		return sb.String(), nil
	}
	return "", nil
}

func splitSpec(s string) []string {
	var out []string
	depth := 0
	start := 0
	for i, c := range s {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	out = append(out, strings.TrimSpace(s[start:]))
	return out
}

// jq implements a small subset of jq.
func jq(args []string, in string) (string, error) {
	raw := false
	expr := "."
	for _, a := range args {
		if a == "-r" || a == "--raw-output" {
			raw = true
		} else if a == "-c" {
		} else {
			expr = a
		}
	}
	var v any
	if err := json.Unmarshal([]byte(in), &v); err != nil {
		return "", fail(2, "jq: error (at <stdin>:0): Cannot parse input as JSON")
	}
	vals := []any{v}
	pipes := strings.Split(expr, "|")
	for _, p := range pipes {
		p = strings.TrimSpace(p)
		if p == "length" {
			var out []any
			for _, x := range vals {
				switch t := x.(type) {
				case []any:
					out = append(out, float64(len(t)))
				case map[string]any:
					out = append(out, float64(len(t)))
				default:
					out = append(out, float64(len(fmt.Sprint(t))))
				}
			}
			vals = out
			continue
		}
		var next []any
		for _, x := range vals {
			next = append(next, jqPath(x, p)...)
		}
		vals = next
	}
	var sb strings.Builder
	for _, x := range vals {
		if s, ok := x.(string); ok && raw {
			sb.WriteString(s + "\n")
			continue
		}
		b, _ := json.MarshalIndent(x, "", "  ")
		sb.WriteString(string(b) + "\n")
	}
	return sb.String(), nil
}

func jqPath(v any, p string) []any {
	if p == "." || p == "" {
		return []any{v}
	}
	cur := []any{v}
	re := regexp.MustCompile(`\.([A-Za-z_][A-Za-z0-9_-]*)|\[(\d*)\]|\["([^"]+)"\]`)
	for _, m := range re.FindAllStringSubmatch(p, -1) {
		var next []any
		for _, c := range cur {
			switch {
			case m[1] != "" || m[3] != "":
				k := m[1] + m[3]
				if mm, ok := c.(map[string]any); ok {
					next = append(next, mm[k])
				}
			case m[2] == "":
				if arr, ok := c.([]any); ok {
					next = append(next, arr...)
				} else if mm, ok := c.(map[string]any); ok {
					for _, k := range sortedAnyKeys(mm) {
						next = append(next, mm[k])
					}
				}
			default:
				i, _ := strconv.Atoi(m[2])
				if arr, ok := c.([]any); ok && i < len(arr) {
					next = append(next, arr[i])
				}
			}
		}
		cur = next
	}
	return cur
}

func sortedAnyKeys(m map[string]any) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	sort.Strings(k)
	return k
}
