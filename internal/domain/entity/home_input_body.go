package entity

import (
	"encoding/json"
	"strings"
)

// CheckHomeInputBody checks an action body that references inputs. Inputs
// may only appear inside JSON string literals, and the body with every
// placeholder replaced must be valid JSON. It returns a message, or "" when
// the body is acceptable.
func CheckHomeInputBody(body string) string {
	t := strings.TrimSpace(body)
	if !strings.HasPrefix(t, "{") && !strings.HasPrefix(t, "[") {
		return "inputs are only allowed in a JSON body, inside a string such as {\"q\": \"{{input.q}}\"}"
	}
	inString := homeJSONStringMask(body)
	var b strings.Builder
	last := 0
	for _, loc := range homePlaceholderLocs(body) {
		start, end, isInput := loc[0], loc[1], loc[2] == 1
		b.WriteString(body[last:start])
		switch {
		case inString[start]:
			// Replaced by an empty string inside the literal.
		case isInput:
			return "input " + body[start:end] + " must be inside a JSON string (between double quotes)"
		default:
			// An env value outside a string stands for a JSON value.
			b.WriteString("0")
		}
		last = end
	}
	b.WriteString(body[last:])
	if !json.Valid([]byte(b.String())) {
		return "must be valid JSON once inputs are filled in"
	}
	return ""
}

// homePlaceholderLocs returns [start, end, isInput] for every env and input
// placeholder in s, in order.
func homePlaceholderLocs(s string) [][3]int {
	var out [][3]int
	for _, m := range homeInputPattern.FindAllStringIndex(s, -1) {
		out = append(out, [3]int{m[0], m[1], 1})
	}
	for _, m := range homeEnvPattern.FindAllStringIndex(s, -1) {
		out = append(out, [3]int{m[0], m[1], 0})
	}
	// Insertion sort by start; there are only a few placeholders.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j][0] < out[j-1][0]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// homeJSONStringMask marks, for each byte of s, whether it is inside a JSON
// string literal (between its quotes, escapes included).
func homeJSONStringMask(s string) []bool {
	mask := make([]bool, len(s))
	in, escaped := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case !in:
			if c == '"' {
				in = true
			}
		case escaped:
			escaped = false
			mask[i] = true
		case c == '\\':
			escaped = true
			mask[i] = true
		case c == '"':
			in = false
		default:
			mask[i] = true
		}
	}
	return mask
}
