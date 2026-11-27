package query

import "testing"

// Patterns RE2 accepts but JavaScript reads differently or rejects. Expected values are what
// `new RegExp(pattern, flags).test(input)` yields in a browser (non-unicode unless noted).
func TestCompileJSMatchesJavaScriptSemantics(t *testing.T) {
	cases := []struct {
		pattern, flags, input string
		reject                bool
		want                  bool
	}{
		{pattern: `(?i)foo`, reject: true},
		{pattern: `(?P<x>a)`, reject: true},
		{pattern: `(?s:a)`, reject: true},
		{pattern: `\pL`, input: "FOO", want: false}, // literal "pL"
		{pattern: `\pL`, input: "xpL", want: true},
		{pattern: `^\p{L}`, flags: "u", input: "abc", want: true},
		{pattern: `^\p{L}`, flags: "u", input: "1bc", want: false},
		{pattern: `[[:alpha:]]`, input: "a", want: false},
		{pattern: `[[:alpha:]]`, input: "a]", want: true},
		{pattern: `\s`, input: "a b", want: true},
		{pattern: `\s`, input: "a\vb", want: true},
		{pattern: `\s`, input: "ab", want: false},
		{pattern: `^\S+$`, input: "ab", want: true},
		{pattern: `^\S+$`, input: "a b", want: false},
		{pattern: `[\s,]+`, input: " ", want: true},
		{pattern: `a.c`, input: "a\rc", want: false},
		{pattern: `a.c`, input: "a c", want: false},
		{pattern: `a.c`, input: "abc", want: true},
		{pattern: `a.c`, flags: "s", input: "a\nc", want: true},
		{pattern: `\Aabc`, input: "Aabc", want: true}, // \A is the letter A
		{pattern: `\Aabc`, input: "abc", want: false},
		{pattern: `a\z`, input: "az", want: true},
		{pattern: `(?:ab)+c`, input: "ababc", want: true},
		{pattern: `(?=a)a`, input: "a", want: true},
		{pattern: `(a)\1`, input: "aa", want: true},
		{pattern: `abc`, flags: "y", input: "xabc", want: false},
		{pattern: `abc`, flags: "i", input: "ABC", want: true},
	}
	for _, c := range cases {
		re, err := CompileJS(c.pattern, c.flags)
		if c.reject {
			if err == nil {
				t.Errorf("/%s/%s must be rejected like JavaScript", c.pattern, c.flags)
			}
			continue
		}
		if err != nil {
			t.Errorf("/%s/%s: %v", c.pattern, c.flags, err)
			continue
		}
		if got := re.Test(c.input); got != c.want {
			t.Errorf("/%s/%s on %q = %v (%s), want %v", c.pattern, c.flags, c.input, got, re.Engine(), c.want)
		}
	}
}
