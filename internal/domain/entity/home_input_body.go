package entity

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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

// HomeURLOriginEnd returns the length of the scheme://host:port prefix of a
// URL template: everything before the first '/', '?' or '#' that follows
// "://". Without "://" the whole template is the prefix.
func HomeURLOriginEnd(raw string) int {
	i := strings.Index(raw, "://")
	if i < 0 {
		return len(raw)
	}
	rest := raw[i+3:]
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		return i + 3 + j
	}
	return len(raw)
}

// HomeInputsInURLOrigin reports whether an {{input.X}} placeholder appears
// in the scheme, host or port of a URL template.
func HomeInputsInURLOrigin(raw string) bool {
	end := HomeURLOriginEnd(raw)
	for _, m := range homeInputPattern.FindAllStringIndex(raw, -1) {
		if m[0] < end {
			return true
		}
	}
	return false
}

// HomeBodyIsJSON reports whether a body template is treated as JSON: it
// starts with '{' or '['.
func HomeBodyIsJSON(body string) bool {
	t := strings.TrimSpace(body)
	return strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[")
}

// ExpandHomeJSONBodyEnv replaces {{env.NAME}} in a JSON body template. A
// placeholder inside a JSON string gets the JSON-escaped value, so a secret
// containing quotes or backslashes cannot break out of the string. Outside
// strings the value must be a single JSON value and is inserted raw. The
// result must still be valid JSON once input placeholders are filled with
// empty strings. Errors only
// name variables, never values.
func ExpandHomeJSONBodyEnv(body string, lookup func(string) (string, bool)) (string, error) {
	if lookup == nil {
		return "", errors.New("home: environment lookup is nil")
	}
	inString := homeJSONStringMask(body)
	var b strings.Builder
	var missing []string
	last := 0
	for _, m := range homeEnvPattern.FindAllStringSubmatchIndex(body, -1) {
		name := body[m[2]:m[3]]
		b.WriteString(body[last:m[0]])
		last = m[1]
		value, ok := lookup(name)
		if !ok {
			if !slices.Contains(missing, name) {
				missing = append(missing, name)
			}
			continue
		}
		if inString[m[0]] {
			b.WriteString(homeJSONStringEscape(value))
			continue
		}
		// Outside a string the value must be one JSON value on its own
		// (a number, true, a quoted string...), so it cannot add fields.
		if !json.Valid([]byte(value)) {
			return "", fmt.Errorf("home: environment variable %s is not a single JSON value", name)
		}
		b.WriteString(value)
	}
	b.WriteString(body[last:])
	if len(missing) > 0 {
		return "", fmt.Errorf("home: environment variable %s is not set", strings.Join(missing, ", "))
	}
	out := b.String()
	probe := homeInputPattern.ReplaceAllString(out, "")
	if !json.Valid([]byte(probe)) {
		return "", errors.New("home: body is not valid JSON after environment expansion")
	}
	return out, nil
}

// homeJSONStringEscape escapes s for use inside a JSON string literal.
func homeJSONStringEscape(s string) string {
	data, _ := json.Marshal(s)
	return string(data[1 : len(data)-1])
}
