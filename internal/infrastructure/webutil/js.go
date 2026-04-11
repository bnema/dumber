package webutil

import "strings"

// EscapeForJSString escapes a string for use inside a JS single-quoted string literal.
func EscapeForJSString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\u2028", "\\u2028")
	s = strings.ReplaceAll(s, "\u2029", "\\u2029")
	return s
}
