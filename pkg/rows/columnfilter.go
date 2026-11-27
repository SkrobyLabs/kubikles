package rows

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"time"

	"kubikles/pkg/rows/query"
)

// FilterCondition is one condition of a column filter: a pattern for a regex filter, an
// operator and number for a numeric one. On the wire a regex condition is a bare string and a
// numeric one an object.
type FilterCondition struct {
	Pattern  string
	Operator string
	// Value is nil when the condition carried no number (JavaScript undefined).
	Value *float64
}

func (c *FilterCondition) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &c.Pattern)
	}
	var o struct {
		Operator string   `json:"operator"`
		Value    *float64 `json:"value"`
		Pattern  string   `json:"pattern"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return err
	}
	c.Operator, c.Value, c.Pattern = o.Operator, o.Value, o.Pattern
	return nil
}

func (c FilterCondition) MarshalJSON() ([]byte, error) {
	if c.Operator == "" && c.Value == nil {
		return json.Marshal(c.Pattern)
	}
	return json.Marshal(struct {
		Operator string   `json:"operator"`
		Value    *float64 `json:"value,omitempty"`
	}{c.Operator, c.Value})
}

// ColumnFilterSpec mirrors the ColumnFilter of the legacy list: select by value set, regex
// conditions or numeric conditions, combined with AND (the default) or OR.
type ColumnFilterSpec struct {
	Col  string `json:"col"`
	Type string `json:"type"`
	// Values is the set a select filter accepts. A nil or empty set accepts nothing.
	Values []string `json:"values,omitempty"`
	// Conditions are the regex patterns or numeric conditions. Absent (nil) conditions fall back
	// to Pattern or Operator and Value, exactly as the legacy compile step does.
	Conditions []FilterCondition `json:"conditions,omitempty"`
	Logic      string            `json:"logic,omitempty"`
	Pattern    *string           `json:"pattern,omitempty"`
	Operator   string            `json:"operator,omitempty"`
	Value      *float64          `json:"value,omitempty"`
}

// FilterValue is getColumnValue: the string select and regex filters test.
func (c *Column) FilterValue(r *Row) string {
	if c.Filter != nil {
		return c.Filter(r)
	}
	switch c.Key {
	case "name":
		return r.Name()
	case "namespace":
		return r.Namespace()
	}
	if s, ok := c.Cell(r).(string); ok {
		return s
	}
	return ""
}

// NumericValue is getNumericValue: ok=false stands for NaN, which no numeric filter matches.
func (c *Column) NumericValue(r *Row) (float64, bool) {
	if c.Numeric != nil {
		return c.Numeric(r)
	}
	if c.Key == "age" {
		ts := r.CreationTimestamp()
		if ts == "" {
			return 0, false
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return 0, false
		}
		return float64(r.Now.UnixMilli()-t.UnixMilli()) / 3600000, true
	}
	switch n := c.Cell(r).(type) {
	case float64:
		return n, !math.IsNaN(n)
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// usesNow reports whether the column's numeric value depends on the clock.
func (c *Column) usesNow() bool { return c.Numeric == nil && c.Key == "age" }

func checkNumeric(v float64, c FilterCondition) bool {
	target := math.NaN()
	if c.Value != nil {
		target = *c.Value
	}
	switch c.Operator {
	case ">":
		return v > target
	case ">=":
		return v >= target
	case "<":
		return v < target
	case "<=":
		return v <= target
	case "=":
		return v == target
	case "!=":
		return v != target
	}
	return true
}

func combine(results []bool, and bool) bool {
	if and {
		for _, r := range results {
			if !r {
				return false
			}
		}
		return true
	}
	for _, r := range results {
		if r {
			return true
		}
	}
	return false
}

// compileColumnFilters compiles filters once into a single predicate (compileColumnFilters in
// resourceListPipeline.ts). Filters on unknown columns are skipped and an invalid regex pattern
// matches everything. It returns nil when nothing remains to test. needsClock is true when a
// numeric filter depends on the current time.
func compileColumnFilters(spec *Spec, filters []ColumnFilterSpec) (pred func(*Row) bool, needsClock bool, err error) {
	// Later entries for the same column replace earlier ones, like keys of a JS object.
	order := make([]string, 0, len(filters))
	byCol := map[string]ColumnFilterSpec{}
	for _, f := range filters {
		if _, seen := byCol[f.Col]; !seen {
			order = append(order, f.Col)
		}
		byCol[f.Col] = f
	}
	var tests []func(*Row) bool
	for _, key := range order {
		f := byCol[key]
		col := spec.Column(key)
		if col == nil {
			continue
		}
		switch f.Type {
		case "select":
			set := make(map[string]struct{}, len(f.Values))
			for _, v := range f.Values {
				set[v] = struct{}{}
			}
			tests = append(tests, func(r *Row) bool {
				_, ok := set[col.FilterValue(r)]
				return ok
			})
		case "regex":
			patterns := f.regexPatterns()
			res := make([]*query.Regex, len(patterns))
			for i, p := range patterns {
				if re, cerr := query.CompileJS(p, "i"); cerr == nil {
					res[i] = re
				}
			}
			and := f.Logic != "or"
			tests = append(tests, func(r *Row) bool {
				val := col.FilterValue(r)
				results := make([]bool, len(res))
				for i, re := range res {
					results[i] = re == nil || re.Test(val)
				}
				return combine(results, and)
			})
		case "numeric":
			conds := f.Conditions
			if conds == nil {
				conds = []FilterCondition{{Operator: f.Operator, Value: f.Value}}
			}
			and := f.Logic != "or"
			if col.usesNow() {
				needsClock = true
			}
			tests = append(tests, func(r *Row) bool {
				v, ok := col.NumericValue(r)
				if !ok {
					return false
				}
				results := make([]bool, len(conds))
				for i, c := range conds {
					results[i] = checkNumeric(v, c)
				}
				return combine(results, and)
			})
		default:
			return nil, false, errors.New("unknown column filter type " + f.Type)
		}
	}
	if len(tests) == 0 {
		return nil, false, nil
	}
	return func(r *Row) bool {
		for _, t := range tests {
			if !t(r) {
				return false
			}
		}
		return true
	}, needsClock, nil
}

func (f ColumnFilterSpec) regexPatterns() []string {
	if f.Conditions != nil {
		out := make([]string, len(f.Conditions))
		for i, c := range f.Conditions {
			out[i] = c.Pattern
		}
		return out
	}
	if f.Pattern != nil {
		return []string{*f.Pattern}
	}
	// new RegExp(undefined, 'i') is the empty pattern, which matches everything.
	return []string{""}
}
