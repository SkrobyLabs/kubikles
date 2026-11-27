package query

import (
	"strings"
	"unicode/utf8"
)

// Condition is one parsed search condition.
type Condition struct {
	// Type is "plain" (matches the name) or "field".
	Type  string
	Field string
	// Value is the text to look for; for a regex condition it is the original source.
	Value   string
	IsRegex bool
	// Regex is set when IsRegex is true.
	Regex *Regex
}

// Query is a parsed search: groups are OR-ed, conditions inside a group are AND-ed.
type Query struct {
	Groups [][]Condition
}

// ParseQuery ports parseQuery in utils/search/queryParser.ts.
func ParseQuery(q string) Query {
	q = trimJS(q)
	if q == "" {
		return Query{}
	}
	var out Query
	for _, seg := range splitOR(q) {
		conds := parseSegment(trimJS(seg))
		if len(conds) > 0 {
			out.Groups = append(out.Groups, conds)
		}
	}
	return out
}

// splitOR is q.split(/\s+OR\s+/i).
func splitOR(q string) []string {
	var parts []string
	start := 0
	i := 0
	for i < len(q) {
		r, n := utf8.DecodeRuneInString(q[i:])
		if !IsJSSpace(r) {
			i += n
			continue
		}
		j := i + n
		for j < len(q) {
			r2, n2 := utf8.DecodeRuneInString(q[j:])
			if !IsJSSpace(r2) {
				break
			}
			j += n2
		}
		// whitespace run is [i, j)
		if j+2 <= len(q) && strings.EqualFold(q[j:j+2], "or") && j+2 < len(q) {
			r3, n3 := utf8.DecodeRuneInString(q[j+2:])
			if IsJSSpace(r3) {
				k := j + 2 + n3
				for k < len(q) {
					r4, n4 := utf8.DecodeRuneInString(q[k:])
					if !IsJSSpace(r4) {
						break
					}
					k += n4
				}
				parts = append(parts, q[start:i])
				start = k
				i = k
				continue
			}
		}
		i = j
	}
	parts = append(parts, q[start:])
	return parts
}

func parseSegment(segment string) []Condition {
	if segment == "" {
		return nil
	}
	var conds []Condition
	remaining := segment
	for len(remaining) > 0 {
		remaining = trimStartJS(remaining)
		if remaining == "" {
			break
		}
		if c, n, ok := matchFieldCondition(remaining); ok {
			conds = append(conds, c)
			remaining = remaining[n:]
			continue
		}
		if tok, ok := matchRegexToken(remaining); ok {
			conds = append(conds, Condition{Type: "plain", Value: tok.value, IsRegex: tok.isRegex, Regex: tok.re})
			remaining = remaining[tok.consumed:]
			continue
		}
		if w, ok := matchPlainWord(remaining); ok {
			conds = append(conds, Condition{Type: "plain", Value: w})
			remaining = remaining[len(w):]
			continue
		}
		// Fallback: everything up to the next white space.
		end := 0
		for end < len(remaining) {
			r, n := utf8.DecodeRuneInString(remaining[end:])
			if IsJSSpace(r) {
				break
			}
			end += n
		}
		if end == 0 {
			break
		}
		conds = append(conds, Condition{Type: "plain", Value: remaining[:end]})
		remaining = remaining[end:]
	}
	return conds
}

// matchPlainWord is remaining.match(/^([^\s:]+)(?=\s|$|[a-zA-Z]+:)/), including the regex's
// backtracking when the greedy run is followed by a colon.
func matchPlainWord(s string) (string, bool) {
	runes := []rune(s)
	max := 0
	for max < len(runes) && !IsJSSpace(runes[max]) && runes[max] != ':' {
		max++
	}
	for n := max; n >= 1; n-- {
		if lookaheadWord(runes, n) {
			return string(runes[:n]), true
		}
	}
	return "", false
}

// lookaheadWord is (?=\s|$|[a-zA-Z]+:) at position pos.
func lookaheadWord(runes []rune, pos int) bool {
	if pos >= len(runes) {
		return true
	}
	if IsJSSpace(runes[pos]) {
		return true
	}
	i := pos
	for i < len(runes) && isASCIILetter(runes[i]) {
		i++
	}
	return i > pos && i < len(runes) && runes[i] == ':'
}

type regexToken struct {
	value    string
	isRegex  bool
	re       *Regex
	consumed int
}

// matchRegexToken is str.match(/^\/(.+?)\/([gimsuy]*)?(?=\s|,|$)/) followed by the RegExp
// construction. A token whose pattern does not compile is returned as literal text.
func matchRegexToken(str string) (regexToken, bool) {
	if !strings.HasPrefix(str, "/") {
		return regexToken{}, false
	}
	// body must be at least one character and cannot cross a line terminator.
	pos := 1
	first, n := utf8.DecodeRuneInString(str[pos:])
	if pos >= len(str) || isLineTerminator(first) {
		return regexToken{}, false
	}
	pos += n
	for pos < len(str) {
		// Candidate closing slash at pos.
		if str[pos] == '/' {
			flagsStart := pos + 1
			flagsEnd := flagsStart
			for flagsEnd < len(str) && strings.IndexByte("gimsuy", str[flagsEnd]) >= 0 {
				flagsEnd++
			}
			for fe := flagsEnd; fe >= flagsStart; fe-- {
				if lookaheadToken(str, fe) {
					pattern := str[1:pos]
					flags := str[flagsStart:fe]
					whole := str[:fe]
					re, err := CompileJS(pattern, flags)
					if err != nil {
						return regexToken{value: whole, consumed: fe}, true
					}
					return regexToken{value: whole, isRegex: true, re: re, consumed: fe}, true
				}
			}
		}
		r, n := utf8.DecodeRuneInString(str[pos:])
		if isLineTerminator(r) {
			return regexToken{}, false
		}
		pos += n
	}
	return regexToken{}, false
}

// lookaheadToken is (?=\s|,|$) at byte offset pos.
func lookaheadToken(s string, pos int) bool {
	if pos >= len(s) {
		return true
	}
	if s[pos] == ',' {
		return true
	}
	r, _ := utf8.DecodeRuneInString(s[pos:])
	return IsJSSpace(r)
}

// matchFieldCondition tries field:value at the start of str.
func matchFieldCondition(str string) (Condition, int, bool) {
	i := 0
	for i < len(str) && isWordChar(rune(str[i])) {
		i++
	}
	if i == 0 || i >= len(str) || str[i] != ':' {
		return Condition{}, 0, false
	}
	field := strings.ToLower(str[:i])
	pos := i + 1
	after := str[pos:]

	if tok, ok := matchRegexToken(after); ok {
		return Condition{Type: "field", Field: field, Value: tok.value, IsRegex: tok.isRegex, Regex: tok.re}, pos + tok.consumed, true
	}
	for _, q := range []byte{'"', '\''} {
		if len(after) > 0 && after[0] == q {
			if end := strings.IndexByte(after[1:], q); end >= 0 {
				return Condition{Type: "field", Field: field, Value: after[1 : 1+end]}, pos + end + 2, true
			}
		}
	}
	end := 0
	for end < len(after) {
		r, n := utf8.DecodeRuneInString(after[end:])
		if IsJSSpace(r) || r == ',' {
			break
		}
		end += n
	}
	if end > 0 {
		return Condition{Type: "field", Field: field, Value: after[:end]}, pos + end, true
	}
	return Condition{}, 0, false
}
