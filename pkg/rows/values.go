package rows

import (
	"encoding/json"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"kubikles/pkg/rows/query"
)

// CompareUTF16 orders two strings by UTF-16 code units, which is what the JavaScript relational
// operators do. It differs from Go's byte order only when a supplementary-plane character meets
// a BMP character above the surrogate range.
func CompareUTF16(a, b string) int {
	if a == b {
		return 0
	}
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ra, na := utf8.DecodeRuneInString(a[i:])
		rb, nb := utf8.DecodeRuneInString(b[j:])
		if ra != rb {
			ua, ub := firstUnit(ra), firstUnit(rb)
			if ua != ub {
				if ua < ub {
					return -1
				}
				return 1
			}
			// Same lead surrogate: compare the trail surrogates.
			_, ta := utf16.EncodeRune(ra)
			_, tb := utf16.EncodeRune(rb)
			if ta < tb {
				return -1
			}
			return 1
		}
		i += na
		j += nb
	}
	switch {
	case i < len(a):
		return 1
	case j < len(b):
		return -1
	}
	return 0
}

func firstUnit(r rune) rune {
	if r >= 0x10000 {
		hi, _ := utf16.EncodeRune(r)
		return hi
	}
	return r
}

// cmpResult is the outcome of the JavaScript comparison of two cells.
type cmpResult int

const (
	cmpLess cmpResult = iota - 1
	cmpEqual
	cmpGreater
)

// compareCells orders two projected cells with the semantics of sortItems' primary key: strings
// by UTF-16 code units, numbers numerically, booleans as 0 and 1. undefined/null (nil) is not
// comparable in JavaScript, so the legacy comparator treats it as a tie; because a tie against
// every value is not an order, nil sorts before every value here, which is identical whenever a
// column is all-defined or all-undefined (the only cases real clusters produce).
func compareCells(a, b interface{}) cmpResult {
	if a == nil || b == nil {
		switch {
		case a == nil && b == nil:
			return cmpEqual
		case a == nil:
			return cmpLess
		}
		return cmpGreater
	}
	switch av := a.(type) {
	case string:
		switch bv := b.(type) {
		case string:
			return cmpResult(CompareUTF16(av, bv))
		default:
			if f, ok := toNumber(b); ok {
				return cmpFloat(jsNumber(av), f)
			}
		}
	default:
		if af, ok := toNumber(a); ok {
			switch bv := b.(type) {
			case string:
				return cmpFloat(af, jsNumber(bv))
			default:
				if bf, ok := toNumber(b); ok {
					return cmpFloat(af, bf)
				}
			}
		}
	}
	return cmpEqual
}

func cmpFloat(a, b float64) cmpResult {
	switch {
	case math.IsNaN(a) || math.IsNaN(b):
		return cmpEqual
	case a < b:
		return cmpLess
	case a > b:
		return cmpGreater
	}
	return cmpEqual
}

func toNumber(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	case bool:
		if n {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// jsNumber is Number(string) for the common cases (decimal, empty means 0, otherwise NaN).
func jsNumber(s string) float64 {
	s = strings.TrimFunc(s, query.IsJSSpace)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || strings.ContainsAny(s, "_xXpP") {
		return math.NaN()
	}
	return f
}

// cellsEqual reports whether two projected rows are identical. Cells are JSON-shaped (strings,
// numbers, booleans, nil); anything else falls back to deep equality.
func cellsEqual(a, b []interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !cellEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func cellEqual(a, b interface{}) bool {
	switch av := a.(type) {
	case nil:
		return b == nil
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case float64:
		bv, ok := b.(float64)
		return ok && (av == bv || (math.IsNaN(av) && math.IsNaN(bv)))
	case int:
		bv, ok := b.(int)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	}
	return reflect.DeepEqual(a, b)
}

// jsKeyOrder returns the keys in the order JavaScript's Object.keys lists the properties of an
// object parsed from JSON text whose keys appear in sorted order (which is how encoding/json
// writes maps): array-index keys first in ascending numeric order, then the others in text
// order.
func jsKeyOrder(sortedKeys []string) []string {
	var idx, rest []string
	for _, k := range sortedKeys {
		if isArrayIndex(k) {
			idx = append(idx, k)
		} else {
			rest = append(rest, k)
		}
	}
	if len(idx) == 0 {
		return sortedKeys
	}
	sort.Slice(idx, func(i, j int) bool {
		a, _ := strconv.ParseUint(idx[i], 10, 64)
		b, _ := strconv.ParseUint(idx[j], 10, 64)
		return a < b
	})
	return append(idx, rest...)
}

// isArrayIndex reports whether k is a canonical array index: "0" or a decimal without leading
// zeros below 2^32-1.
func isArrayIndex(k string) bool {
	if k == "" || len(k) > 10 {
		return false
	}
	if k == "0" {
		return true
	}
	if k[0] < '1' || k[0] > '9' {
		return false
	}
	for i := 1; i < len(k); i++ {
		if k[i] < '0' || k[i] > '9' {
			return false
		}
	}
	n, err := strconv.ParseUint(k, 10, 64)
	return err == nil && n < 4294967295
}

// sortedKeys returns the keys of a string-keyed map in byte order (encoding/json's order).
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// jsKeys is Object.keys of a JSON-decoded map.
func jsKeys[V any](m map[string]V) []string { return jsKeyOrder(sortedKeys(m)) }

// jsStringLen is the length in UTF-16 units of JSON.stringify(s).
func jsStringLen(s string) int {
	n := 2
	for _, r := range s {
		switch {
		case r == '"' || r == '\\' || r == '\b' || r == '\f' || r == '\n' || r == '\r' || r == '\t':
			n += 2
		case r < 0x20:
			n += 6
		case r >= 0x10000:
			n += 2
		case r == utf8.RuneError:
			n += 1
		default:
			n++
		}
	}
	return n
}

// jsStringifyMapLen is the length of JSON.stringify(obj) for an object of string values.
// valueLen yields the stringified length of each value.
func jsStringifyMapLen[V any](m map[string]V, valueLen func(V) int) int {
	if len(m) == 0 {
		return 2
	}
	n := 2 + (len(m) - 1)
	for k, v := range m {
		n += jsStringLen(k) + 1 + valueLen(v)
	}
	return n
}

// marshalCell renders any cell for the wire; used by tests.
func marshalCell(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
