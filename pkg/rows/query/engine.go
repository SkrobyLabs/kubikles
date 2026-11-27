package query

import "strings"

// Field is one searchable field of a row type T: a primary name, the aliases that resolve to it
// and the extractor producing its text.
type Field[T any] struct {
	Name    string
	Aliases []string
	Extract func(T) string
}

// Lookup resolves a field by name or alias, case-insensitively. A direct name always wins; an
// alias resolves to the first field (in declaration order) that lists it. This is the lookup
// table getFieldByName builds in utils/search/fieldMappings/index.ts.
func Lookup[T any](fields []Field[T], name string) *Field[T] {
	name = strings.ToLower(name)
	for i := range fields {
		if fields[i].Name == name {
			return &fields[i]
		}
	}
	for i := range fields {
		for _, a := range fields[i].Aliases {
			if a == name {
				return &fields[i]
			}
		}
	}
	return nil
}

// Compile turns a parsed query into a predicate (createFilter in filterEngine.ts). Plain text
// matches the name only, an unknown field matches nothing, partial matches are
// case-insensitive, and regex conditions are tested against the raw text. An empty query
// matches everything.
func Compile[T any](q Query, fields []Field[T], name func(T) string) func(T) bool {
	if len(q.Groups) == 0 {
		return func(T) bool { return true }
	}
	groups := make([][]func(T) bool, len(q.Groups))
	for i, g := range q.Groups {
		for _, c := range g {
			groups[i] = append(groups[i], compileCondition(c, fields, name))
		}
	}
	return func(item T) bool {
		for _, g := range groups {
			all := true
			for _, test := range g {
				if !test(item) {
					all = false
					break
				}
			}
			if all {
				return true
			}
		}
		return false
	}
}

// CompileString parses and compiles in one step.
func CompileString[T any](q string, fields []Field[T], name func(T) string) func(T) bool {
	return Compile(ParseQuery(q), fields, name)
}

func compileCondition[T any](c Condition, fields []Field[T], name func(T) string) func(T) bool {
	switch c.Type {
	case "plain":
		if c.IsRegex {
			re := c.Regex
			return func(item T) bool { return re.Test(name(item)) }
		}
		needle := strings.ToLower(c.Value)
		return func(item T) bool { return strings.Contains(strings.ToLower(name(item)), needle) }
	case "field":
		f := Lookup(fields, c.Field)
		if f == nil {
			return func(T) bool { return false }
		}
		extract := f.Extract
		if c.IsRegex {
			re := c.Regex
			return func(item T) bool { return re.Test(extract(item)) }
		}
		needle := strings.ToLower(c.Value)
		return func(item T) bool { return strings.Contains(strings.ToLower(extract(item)), needle) }
	}
	return func(T) bool { return false }
}
