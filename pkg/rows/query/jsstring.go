package query

import (
	"unicode/utf8"
)

// IsJSSpace reports whether r is JavaScript white space or a line terminator, the set matched by
// the regular expression escape \s and removed by String.prototype.trim.
func IsJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00a0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

func isLineTerminator(r rune) bool {
	return r == '\n' || r == '\r' || r == 0x2028 || r == 0x2029
}

func isWordChar(r rune) bool {
	return r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isASCIILetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// trimStartJS removes leading JavaScript white space.
func trimStartJS(s string) string {
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		if !IsJSSpace(r) {
			break
		}
		s = s[n:]
	}
	return s
}

// trimJS is String.prototype.trim.
func trimJS(s string) string {
	s = trimStartJS(s)
	for len(s) > 0 {
		r, n := utf8.DecodeLastRuneInString(s)
		if !IsJSSpace(r) {
			break
		}
		s = s[:len(s)-n]
	}
	return s
}
