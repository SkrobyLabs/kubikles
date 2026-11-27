// Package query ports the list search language (frontend/src/utils/search): the query parser,
// the filter engine's compile step and the JavaScript-compatible regular expressions both use.
package query

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/dlclark/regexp2"
)

// matchTimeout bounds one backtracking match so a pathological pattern cannot stall a view.
const matchTimeout = 100 * time.Millisecond

// Regex is a compiled JavaScript-style regular expression. RE2 runs everything it can compile;
// patterns RE2 rejects (lookaround, backreferences) fall back to regexp2 in ECMAScript mode so
// patterns that are valid in the browser keep matching.
type Regex struct {
	re2 *regexp.Regexp
	bt  *regexp2.Regexp
}

var errBadFlags = errors.New("invalid regular expression flags")

// CompileJS compiles pattern with JavaScript flags (any of gimsuy, each at most once). The error
// is non-nil exactly where `new RegExp(pattern, flags)` would throw, give or take syntax that
// only one engine understands.
func CompileJS(pattern, flags string) (*Regex, error) {
	var ignoreCase, multiline, dotAll, sticky bool
	seen := map[rune]bool{}
	for _, f := range flags {
		if seen[f] {
			return nil, errBadFlags
		}
		seen[f] = true
		switch f {
		case 'i':
			ignoreCase = true
		case 'm':
			multiline = true
		case 's':
			dotAll = true
		case 'y':
			sticky = true
		case 'g', 'u':
		default:
			return nil, errBadFlags
		}
	}

	// Rewrite the constructs whose meaning differs between JavaScript and the engines (\s, ., \p
	// without the u flag, POSIX classes, identity escapes) and reject what JavaScript rejects
	// (inline flag groups), once per engine because the escape syntax differs.
	flagsOpt := translateOpts{dotAll: dotAll, unicode: seen['u']}
	srcRE2, err := translateJS(pattern, flagsOpt, escRE2)
	if err != nil {
		return nil, err
	}
	src, err := translateJS(pattern, flagsOpt, escBT)
	if err != nil {
		return nil, err
	}
	if sticky {
		// test() resets lastIndex to 0, so a sticky regexp can only match at the very start.
		srcRE2 = `\A(?:` + srcRE2 + `)`
		src = `\A(?:` + src + `)`
	}
	var prefix strings.Builder
	if ignoreCase {
		prefix.WriteString("i")
	}
	if multiline {
		prefix.WriteString("m")
	}
	if dotAll {
		prefix.WriteString("s")
	}
	re2src := srcRE2
	if prefix.Len() > 0 {
		re2src = "(?" + prefix.String() + ")" + srcRE2
	}
	if re, err := regexp.Compile(re2src); err == nil {
		return &Regex{re2: re}, nil
	}

	opts := regexp2.RegexOptions(regexp2.ECMAScript)
	if ignoreCase {
		opts |= regexp2.IgnoreCase
	}
	if multiline {
		opts |= regexp2.Multiline
	}
	re, err := regexp2.Compile(src, opts)
	if err != nil && dotAll {
		// ECMAScript mode has no dotAll; the plain engine does.
		re, err = regexp2.Compile(src, regexp2.Singleline|optsIf(ignoreCase, regexp2.IgnoreCase)|optsIf(multiline, regexp2.Multiline))
	} else if err == nil && dotAll {
		re, err = regexp2.Compile(src, regexp2.Singleline|optsIf(ignoreCase, regexp2.IgnoreCase)|optsIf(multiline, regexp2.Multiline))
	}
	if err != nil {
		return nil, err
	}
	re.MatchTimeout = matchTimeout
	return &Regex{bt: re}, nil
}

func optsIf(cond bool, o regexp2.RegexOptions) regexp2.RegexOptions {
	if cond {
		return o
	}
	return 0
}

type translateOpts struct{ dotAll, unicode bool }

func escRE2(r rune) string { return fmt.Sprintf(`\x{%04x}`, r) }
func escBT(r rune) string  { return fmt.Sprintf(`\u%04X`, r) }

// jsSpace is the set JavaScript's \s matches: WhiteSpace and LineTerminator code points.
var jsSpace = [][2]rune{
	{0x09, 0x0d}, {0x20, 0x20}, {0xa0, 0xa0}, {0x1680, 0x1680}, {0x2000, 0x200a},
	{0x2028, 0x2029}, {0x202f, 0x202f}, {0x205f, 0x205f}, {0x3000, 0x3000}, {0xfeff, 0xfeff},
}

func jsSpaceItems(esc func(rune) string) string {
	var b strings.Builder
	for _, r := range jsSpace {
		b.WriteString(esc(r[0]))
		if r[1] != r[0] {
			b.WriteByte('-')
			b.WriteString(esc(r[1]))
		}
	}
	return b.String()
}

// jsKnownEscapes are the letter escapes JavaScript gives a meaning in non-unicode mode and that
// both engines read the same way. Every other letter escape is an identity escape in JavaScript
// (\A is the letter A) while RE2 gives it a meaning (\A, \z, \Q, \h and so on).
const jsKnownEscapes = "dDwWbBnrtfvcxuk"

var errBadGroup = errors.New("invalid regular expression: invalid group")

// translateJS rewrites a JavaScript pattern so that the engine reads it with JavaScript
// semantics. It returns an error exactly where JavaScript would reject the pattern for a group
// the engines would otherwise accept.
func translateJS(p string, o translateOpts, esc func(rune) string) (string, error) {
	rs := []rune(p)
	var b strings.Builder
	inClass := false
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '\\' && i+1 < len(rs):
			i++
			n := rs[i]
			switch {
			case n == 's':
				if inClass {
					b.WriteString(jsSpaceItems(esc))
				} else {
					b.WriteString("[" + jsSpaceItems(esc) + "]")
				}
			case n == 'S' && !inClass:
				b.WriteString("[^" + jsSpaceItems(esc) + "]")
			case (n == 'p' || n == 'P') && o.unicode:
				b.WriteRune('\\')
				b.WriteRune(n)
			case (n == 'p' || n == 'P') && !o.unicode:
				b.WriteRune(n)
			case n < 128 && unicode.IsLetter(n) && !strings.ContainsRune(jsKnownEscapes, n):
				b.WriteRune(n)
			default:
				b.WriteRune('\\')
				b.WriteRune(n)
			}
		case inClass:
			switch c {
			case ']':
				inClass = false
				b.WriteRune(c)
			case '[':
				b.WriteString(`\[`)
			default:
				b.WriteRune(c)
			}
		case c == '[':
			inClass = true
			b.WriteRune(c)
		case c == '.' && !o.dotAll:
			b.WriteString(`[^\n\r` + esc(0x2028) + esc(0x2029) + `]`)
		case c == '(' && i+1 < len(rs) && rs[i+1] == '?':
			rest := string(rs[i+1:])
			switch {
			case strings.HasPrefix(rest, "?:"), strings.HasPrefix(rest, "?="), strings.HasPrefix(rest, "?!"),
				strings.HasPrefix(rest, "?<="), strings.HasPrefix(rest, "?<!"):
			case strings.HasPrefix(rest, "?<") && len(rest) > 2:
			default:
				return "", errBadGroup
			}
			b.WriteRune(c)
		default:
			b.WriteRune(c)
		}
	}
	return b.String(), nil
}

// Test reports whether s contains a match. A timeout counts as no match.
func (r *Regex) Test(s string) bool {
	if r == nil {
		return false
	}
	if r.re2 != nil {
		return r.re2.MatchString(s)
	}
	ok, err := r.bt.MatchString(s)
	return err == nil && ok
}

// Engine names the engine in use ("re2" or "backtracking"), for tests and diagnostics.
func (r *Regex) Engine() string {
	if r.re2 != nil {
		return "re2"
	}
	return "backtracking"
}
