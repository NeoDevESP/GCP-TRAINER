package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
	"github.com/neodevesp/gcp-trainer/internal/sqlengine"
)

// Helpers of the BigQuery data commands (bq query/head/load/insert/extract).

// formatBQ renders query results like bq: a boxed table, or JSON/CSV with --format.
func formatBQ(c *Cmd, r sim.BQResult) string {
	if len(r.Columns) == 0 {
		return ""
	}
	switch c.Str("format", "pretty") {
	case "json", "prettyjson":
		var out []map[string]any
		for _, row := range r.Rows {
			m := map[string]any{}
			for i, cn := range r.Columns {
				v := row[i]
				if r.BoolCols[cn] {
					v = sqlengine.Display(v) == "1"
				}
				m[cn] = v
			}
			out = append(out, m)
		}
		if out == nil {
			out = []map[string]any{}
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		if c.Str("format", "") == "json" {
			b, _ = json.Marshal(out)
		}
		return string(b) + "\n"
	case "csv":
		var b strings.Builder
		w := csv.NewWriter(&b)
		w.Write(r.Columns)
		for _, row := range r.Rows {
			rec := make([]string, len(row))
			for i, v := range row {
				if v != nil {
					rec[i] = sqlengine.Display(v)
				}
			}
			w.Write(rec)
		}
		w.Flush()
		return b.String()
	}
	return sqlengine.FormatTable(r.Columns, r.Rows, r.BoolCols)
}

// syntheticRows renders placeholder rows (ML.PREDICT, INFORMATION_SCHEMA).
func syntheticRows(s *Session, cols []string) string {
	if len(cols) == 0 {
		cols = []string{"f0_"}
	}
	rows := make([][]any, 3)
	for i := range rows {
		for range cols {
			rows[i] = append(rows[i], int64(s.State.Rand().Intn(9000)+100))
		}
	}
	return sqlengine.FormatTable(cols, rows, nil)
}

// readSource reads a load source: gs://bucket/object, a Cloud Shell file or - (stdin).
func (s *Session) readSource(src, stdin string) (string, error) {
	if src == "-" {
		return stdin, nil
	}
	if strings.HasPrefix(src, "gs://") {
		c := &Cmd{S: s}
		b, _, err := c.bucket(src)
		if err != nil {
			return "", err
		}
		obj := objectName(src)
		if err := c.Need("storage.objects.get", s.State.BucketResource(b, obj)); err != nil {
			return "", err
		}
		o := b.Objects[obj]
		if o == nil {
			return "", fmt.Errorf("Not found: URI %s", src)
		}
		return o.Content, nil
	}
	v, ok := s.Files[s.path(src)]
	if !ok {
		return "", fmt.Errorf("file %s not found", src)
	}
	return v, nil
}

// writeGCS stores content as an object (bq extract).
func (s *Session) writeGCS(dst, content string) (string, error) {
	c := &Cmd{S: s, Stdin: content}
	return s.storageCp(c, "-", dst)
}

// parseRecords parses CSV or newline-delimited JSON.
func parseRecords(content, format string, skip int) ([]string, [][]any, error) {
	if strings.Contains(format, "JSON") {
		var cols []string
		seen := map[string]bool{}
		var objs []map[string]any
		for i, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				return nil, nil, fmt.Errorf("line %d: %v", i+1, err)
			}
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if !seen[k] {
					seen[k] = true
					cols = append(cols, k)
				}
			}
			objs = append(objs, m)
		}
		rows := make([][]any, len(objs))
		for i, m := range objs {
			for _, cn := range cols {
				rows[i] = append(rows[i], m[cn])
			}
		}
		return cols, rows, nil
	}
	r := csv.NewReader(strings.NewReader(content))
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	var cols []string
	if skip > 0 && len(recs) > 0 {
		cols = recs[0]
		if skip > len(recs) {
			skip = len(recs)
		}
		recs = recs[skip:]
	}
	rows := make([][]any, len(recs))
	for i, rec := range recs {
		for _, v := range rec {
			rows[i] = append(rows[i], v)
		}
	}
	return cols, rows, nil
}

// schemaFromSpec parses "name:TYPE,name:TYPE".
func schemaFromSpec(spec string) []sim.Field {
	var out []sim.Field
	for _, fd := range strings.Split(spec, ",") {
		parts := strings.Split(strings.TrimSpace(fd), ":")
		if parts[0] == "" {
			continue
		}
		f := sim.Field{Name: parts[0], Type: "STRING", Bytes: 16}
		if len(parts) > 1 {
			f.Type = strings.ToUpper(parts[1])
		}
		out = append(out, f)
	}
	return out
}

// autodetect infers a schema from the header and the values.
func autodetect(cols []string, rows [][]any) []sim.Field {
	header := cols
	data := rows
	if len(header) == 0 && len(rows) > 0 {
		for _, v := range rows[0] {
			header = append(header, fmt.Sprint(v))
		}
		data = rows[1:]
	}
	var out []sim.Field
	for i, cn := range header {
		typ := ""
		for _, r := range data {
			if i >= len(r) || r[i] == nil {
				continue
			}
			t := valueType(r[i])
			switch {
			case typ == "":
				typ = t
			case typ != t:
				if (typ == "INT64" && t == "FLOAT64") || (typ == "FLOAT64" && t == "INT64") {
					typ = "FLOAT64"
				} else {
					typ = "STRING"
				}
			}
		}
		if typ == "" {
			typ = "STRING"
		}
		out = append(out, sim.Field{Name: strings.TrimSpace(cn), Type: typ, Bytes: 8})
	}
	return out
}

func valueType(v any) string {
	switch x := v.(type) {
	case bool:
		return "BOOL"
	case float64:
		if x == float64(int64(x)) {
			return "INT64"
		}
		return "FLOAT64"
	case string:
		var n int64
		var f float64
		switch {
		case x == "true" || x == "false":
			return "BOOL"
		case isInt64(x, &n):
			return "INT64"
		case isFloat(x, &f):
			return "FLOAT64"
		case len(x) == 10 && x[4] == '-' && x[7] == '-':
			return "DATE"
		case len(x) >= 19 && x[4] == '-' && (x[10] == ' ' || x[10] == 'T'):
			return "TIMESTAMP"
		}
	}
	return "STRING"
}

func isInt64(s string, n *int64) bool {
	_, err := fmt.Sscanf(s, "%d", n)
	return err == nil && fmt.Sprint(*n) == strings.TrimLeft(s, "+")
}

func isFloat(s string, f *float64) bool {
	_, err := fmt.Sscanf(s, "%g", f)
	return err == nil && strings.ContainsAny(s, ".eE") && !strings.ContainsAny(s, " /:")
}
