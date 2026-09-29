package sim

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	reFrom      = regexp.MustCompile("(?i)\\bFROM\\s+`?([a-zA-Z0-9_.:-]+)`?")
	reCreate    = regexp.MustCompile("(?is)^\\s*CREATE\\s+(?:OR\\s+REPLACE\\s+)?(TABLE|MODEL|VIEW|MATERIALIZED\\s+VIEW)\\s+`?([a-zA-Z0-9_.:-]+)`?(.*)$")
	rePartition = regexp.MustCompile("(?i)PARTITION\\s+BY\\s+(?:DATE\\s*\\(\\s*)?([a-zA-Z0-9_]+)")
	reCluster   = regexp.MustCompile("(?i)CLUSTER\\s+BY\\s+([a-zA-Z0-9_, ]+?)(?:\\s+OPTIONS|\\s+AS\\b|$)")
	reDate      = regexp.MustCompile(`'(\d{4}-\d{2}-\d{2})`)
	reInterval  = regexp.MustCompile(`(?i)INTERVAL\s+(\d+)\s+DAY`)
	reModelType = regexp.MustCompile(`(?i)model_type\s*=\s*'([a-z_]+)'`)
	reMLModel   = regexp.MustCompile("(?i)MODEL\\s+`?([a-zA-Z0-9_.:-]+)`?")
)

// QueryResult is the outcome of a BigQuery job.
type QueryResult struct {
	Bytes   int64
	Rows    int64
	Created string
	Columns []string
}

func (s *State) findTable(project, ref string) (*Dataset, *Table, string, string) {
	ref = strings.ReplaceAll(ref, ":", ".")
	parts := strings.Split(ref, ".")
	proj := project
	var ds, tb string
	switch len(parts) {
	case 2:
		ds, tb = parts[0], parts[1]
	case 3:
		proj, ds, tb = parts[0], parts[1], parts[2]
	default:
		return nil, nil, proj, ""
	}
	p := s.Projects[proj]
	if p == nil {
		return nil, nil, proj, ds
	}
	d := p.Datasets[ds]
	if d == nil {
		return nil, nil, proj, ds
	}
	return d, d.Tables[tb], proj, ds
}

// RunQuery estimates and (optionally) executes a GoogleSQL statement.
func (s *State) RunQuery(project, principal, sql string, dryRun bool, dest string, partitionField string, clustering []string) (QueryResult, error) {
	sql = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(sql), ";"))
	var res QueryResult
	if !s.Allowed(principal, "bigquery.jobs.create", ProjectResource(project)) {
		return res, fmt.Errorf("Access Denied: Project %s: User does not have bigquery.jobs.create permission in project %s.", project, project)
	}
	if m := reCreate.FindStringSubmatch(sql); m != nil {
		kind := strings.ToUpper(strings.Join(strings.Fields(m[1]), " "))
		rest := m[3]
		asIdx := regexp.MustCompile(`(?is)\bAS\s*\(?\s*SELECT\b`).FindStringIndex(rest)
		sel := ""
		if asIdx != nil {
			sel = strings.TrimPrefix(strings.TrimSpace(rest[asIdx[0]+2:]), "(")
			sel = strings.TrimSuffix(strings.TrimSpace(sel), ")")
		}
		var src *Table
		if sel != "" {
			r, err := s.estimateSelect(project, sel)
			if err != nil {
				return res, err
			}
			res.Bytes = r.Bytes
			res.Columns = r.Columns
			if fm := reFrom.FindStringSubmatch(sel); fm != nil {
				_, src, _, _ = s.findTable(project, fm[1])
			}
		}
		if dryRun {
			return res, nil
		}
		d, existing, _, _ := s.findTable(project, m[2])
		if d == nil {
			return res, fmt.Errorf("Not found: Dataset %s", strings.Split(m[2], ".")[0])
		}
		tname := m[2][strings.LastIndex(m[2], ".")+1:]
		_ = existing
		t := &Table{ID: tname, Kind: kind}
		if kind == "MATERIALIZED VIEW" {
			t.Kind = "MATERIALIZED_VIEW"
		}
		if src != nil {
			t.Rows = src.Rows
			t.PartitionDays = src.PartitionDays
			for _, f := range src.Schema {
				if len(res.Columns) == 0 || contains(res.Columns, f.Name) {
					t.Schema = append(t.Schema, f)
				}
			}
		}
		if kind == "MODEL" {
			t.ModelType = "linear_reg"
			if mm := reModelType.FindStringSubmatch(rest); mm != nil {
				t.ModelType = mm[1]
			}
		}
		if pm := rePartition.FindStringSubmatch(rest); pm != nil {
			t.PartitionField, t.PartitionType = pm[1], "DAY"
			if t.PartitionDays == 0 {
				t.PartitionDays = 365
			}
		}
		if cm := reCluster.FindStringSubmatch(rest); cm != nil {
			for _, c := range strings.Split(cm[1], ",") {
				if c = strings.TrimSpace(c); c != "" {
					t.Clustering = append(t.Clustering, c)
				}
			}
		}
		if strings.Contains(strings.ToLower(rest), "require_partition_filter") && strings.Contains(strings.ToLower(rest), "true") {
			t.RequirePartitionFilter = true
		}
		if kind == "VIEW" || kind == "MATERIALIZED VIEW" {
			t.Query = sel
		}
		if sel == "" && kind == "TABLE" {
			t.Schema = ParseColumnDefs(rest)
			for i := range t.Schema {
				t.Schema[i].Bytes = typeBytes(t.Schema[i].Type)
			}
		}
		if kind == "TABLE" && sel != "" {
			t.Sample = src != nil && src.Sample
			_ = s.FillFromSelect(project, sel, t)
		}
		d.Tables[tname] = t
		res.Created = tname
		s.recordJob(project, principal, sql, res.Bytes, false, m[2])
		return res, nil
	}
	if !regexp.MustCompile(`(?i)^\s*(WITH|SELECT)\b`).MatchString(sql) {
		return res, fmt.Errorf("Syntax error: Unexpected keyword at [1:1] (the simulator supports SELECT, CREATE TABLE/VIEW/MODEL AS SELECT)")
	}
	r, err := s.estimateSelect(project, sql)
	if err != nil {
		return res, err
	}
	res = r
	if dest != "" && !dryRun {
		d, _, _, _ := s.findTable(project, dest)
		if d == nil {
			return res, fmt.Errorf("Not found: Dataset for %s", dest)
		}
		tname := dest[strings.LastIndex(strings.ReplaceAll(dest, ":", "."), ".")+1:]
		t := &Table{ID: tname, Kind: "TABLE", PartitionField: partitionField, Clustering: clustering, Rows: r.Rows}
		if partitionField != "" {
			t.PartitionType, t.PartitionDays = "DAY", 365
		}
		if fm := reFrom.FindStringSubmatch(sql); fm != nil {
			if _, src, _, _ := s.findTable(project, fm[1]); src != nil {
				t.PartitionDays = max(src.PartitionDays, t.PartitionDays)
				t.Sample = src.Sample
				for _, f := range src.Schema {
					if len(r.Columns) == 0 || contains(r.Columns, f.Name) {
						t.Schema = append(t.Schema, f)
					}
				}
			}
		}
		_ = s.FillFromSelect(project, sql, t)
		d.Tables[tname] = t
		res.Created = tname
	}
	s.recordJob(project, principal, sql, res.Bytes, dryRun, dest)
	return res, nil
}

// typeBytes is the modelled size of one value of a BigQuery type.
func typeBytes(t string) int {
	switch strings.ToUpper(t) {
	case "BOOL", "BOOLEAN":
		return 1
	case "INT64", "INTEGER", "FLOAT64", "FLOAT", "TIMESTAMP", "DATETIME", "DATE":
		return 8
	case "NUMERIC":
		return 16
	}
	return 16
}

func contains(l []string, v string) bool {
	for _, x := range l {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

func (s *State) recordJob(project, principal, sql string, bytes int64, dry bool, dest string) {
	p := s.Projects[project]
	p.BQJobs = append(p.BQJobs, &BQJob{ID: "bquxjob_" + s.ID(8), Query: sql, BytesProcessed: bytes, DryRun: dry, User: principal, Destination: dest})
}

func (s *State) estimateSelect(project, sql string) (QueryResult, error) {
	var res QueryResult
	if m := regexp.MustCompile("(?i)ML\\.(PREDICT|EVALUATE)\\s*\\(").FindStringIndex(sql); m != nil {
		mm := reMLModel.FindStringSubmatch(sql[m[0]:])
		if mm == nil {
			return res, fmt.Errorf("Syntax error: expected MODEL")
		}
		_, t, _, _ := s.findTable(project, mm[1])
		if t == nil || t.Kind != "MODEL" {
			return res, fmt.Errorf("Not found: Model %s", mm[1])
		}
	}
	froms := reFrom.FindAllStringSubmatch(sql, -1)
	lower := strings.ToLower(sql)
	selectList := lower
	if i := strings.Index(lower, " from "); i > 0 {
		selectList = lower[:i]
	}
	star := regexp.MustCompile(`select\s+(distinct\s+)?\*|,\s*\*|\.\*`).MatchString(selectList)
	for _, fm := range froms {
		if strings.Contains(strings.ToUpper(fm[1]), "INFORMATION_SCHEMA") {
			res.Bytes += 10 << 20
			continue
		}
		_, t, _, ds := s.findTable(project, fm[1])
		if t == nil {
			if strings.Count(fm[1], ".") < 1 {
				continue // CTE reference
			}
			return res, fmt.Errorf("Not found: Table %s was not found in location EU", fm[1])
		}
		_ = ds
		if t.Kind == "VIEW" && t.Query != "" {
			r, err := s.estimateSelect(project, t.Query)
			if err != nil {
				return res, err
			}
			res.Bytes += r.Bytes
			continue
		}
		if t.Kind == "MATERIALIZED_VIEW" {
			res.Bytes += 50 << 20
			continue
		}
		var rowBytes int64
		for _, f := range t.Schema {
			if star || regexp.MustCompile(`(?i)\b`+regexp.QuoteMeta(f.Name)+`\b`).MatchString(sql) {
				rowBytes += int64(f.Bytes)
				if !contains(res.Columns, f.Name) {
					res.Columns = append(res.Columns, f.Name)
				}
			}
		}
		if rowBytes == 0 && len(t.Schema) > 0 {
			rowBytes = int64(t.Schema[0].Bytes)
		}
		frac := 1.0
		wherePart := ""
		if i := strings.Index(lower, " where "); i >= 0 {
			wherePart = lower[i:]
		}
		if t.PartitionField != "" {
			pf := strings.ToLower(t.PartitionField)
			if wherePart != "" && strings.Contains(wherePart, pf) {
				days := partitionDays(wherePart)
				if t.PartitionDays > 0 {
					frac = float64(days) / float64(t.PartitionDays)
					if frac > 1 {
						frac = 1
					}
				}
			} else if t.RequirePartitionFilter {
				return res, fmt.Errorf("Cannot query over table '%s' without a filter over column(s) '%s' that can be used for partition elimination", fm[1], t.PartitionField)
			}
		}
		if len(t.Clustering) > 0 && wherePart != "" && regexp.MustCompile(`\b`+strings.ToLower(t.Clustering[0])+`\s*(=|in\b)`).MatchString(wherePart) {
			frac *= 0.2
		}
		res.Bytes += int64(float64(t.Rows) * float64(rowBytes) * frac)
		res.Rows += int64(float64(t.Rows) * frac)
	}
	if res.Bytes < 10<<20 && len(froms) > 0 {
		res.Bytes = 10 << 20 // minimum billing
	}
	return res, nil
}

func partitionDays(where string) int {
	dates := reDate.FindAllStringSubmatch(where, -1)
	if m := reInterval.FindStringSubmatch(where); m != nil {
		n, _ := strconv.Atoi(m[1])
		return max(1, n)
	}
	if len(dates) >= 2 {
		a, e1 := time.Parse("2006-01-02", dates[0][1])
		b, e2 := time.Parse("2006-01-02", dates[1][1])
		if e1 == nil && e2 == nil {
			d := int(b.Sub(a).Hours()/24) + 1
			if d < 0 {
				d = -d
			}
			return max(1, d)
		}
	}
	if len(dates) == 1 {
		if strings.Contains(where, ">") {
			return 30
		}
		return 1
	}
	return 30
}

// FormatBytes renders a byte count like BigQuery.
func FormatBytes(b int64) string {
	switch {
	case b >= 1<<40:
		return fmt.Sprintf("%.2f TB", float64(b)/(1<<40))
	case b >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.2f MB", float64(b)/(1<<20))
	}
	return fmt.Sprintf("%d bytes", b)
}
