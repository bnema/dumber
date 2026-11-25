package entity

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// homePathStep is one key or index of a field path.
type homePathStep struct {
	key   string
	index int
	isIdx bool
}

// parseHomeFieldPath splits a path such as `data.items[0].count`,
// `[2].name` or `"a.b".c` into steps.
func parseHomeFieldPath(path string) ([]homePathStep, bool) {
	var steps []homePathStep
	i := 0
	expectKey := true // a key may start here (start of path or after '.')
	for i < len(path) {
		switch path[i] {
		case '[':
			end := strings.IndexByte(path[i:], ']')
			if end < 0 {
				return nil, false
			}
			n, err := strconv.Atoi(path[i+1 : i+end])
			if err != nil || n < 0 {
				return nil, false
			}
			steps = append(steps, homePathStep{index: n, isIdx: true})
			i += end + 1
			expectKey = false
		case '.':
			if expectKey {
				return nil, false // empty key
			}
			i++
			expectKey = true
		case '"':
			if !expectKey {
				return nil, false
			}
			end := strings.IndexByte(path[i+1:], '"')
			if end < 0 {
				return nil, false
			}
			steps = append(steps, homePathStep{key: path[i+1 : i+1+end]})
			i += end + 2
			expectKey = false
		default:
			if !expectKey {
				return nil, false
			}
			end := strings.IndexAny(path[i:], ".[")
			if end < 0 {
				end = len(path) - i
			}
			steps = append(steps, homePathStep{key: path[i : i+end]})
			i += end
			expectKey = false
		}
	}
	if len(steps) == 0 || expectKey {
		return nil, false
	}
	return steps, true
}

// ExtractHomeField returns the value at path in doc, a json.Unmarshal result.
// Path syntax: dot-separated keys, `[n]` indexes, and `"quoted.keys"`.
func ExtractHomeField(doc any, path string) (any, bool) {
	steps, ok := parseHomeFieldPath(strings.TrimSpace(path))
	if !ok {
		return nil, false
	}
	cur := doc
	for _, step := range steps {
		if step.isIdx {
			arr, isArr := cur.([]any)
			if !isArr || step.index >= len(arr) {
				return nil, false
			}
			cur = arr[step.index]
			continue
		}
		obj, isObj := cur.(map[string]any)
		if !isObj {
			return nil, false
		}
		cur, ok = obj[step.key]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// FormatHomeField formats v for display. Non-numeric values fall back to
// text for numeric formats.
func FormatHomeField(v any, format HomeFieldFormat) string {
	if format == "" || format == HomeFormatText {
		return homeText(v)
	}
	n, ok := homeNumber(v)
	if !ok {
		return homeText(v)
	}
	switch format {
	case HomeFormatNumber:
		return formatHomeNumber(n)
	case HomeFormatPercent:
		if math.Abs(n) <= 1 {
			n *= 100
		}
		return formatHomeNumber(n) + "%"
	case HomeFormatBytes:
		return formatHomeBytes(n)
	case HomeFormatDuration:
		return formatHomeDuration(n)
	default:
		return homeText(v)
	}
}

func homeText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case map[string]any, []any:
		data, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		return string(data)
	default:
		return fmt.Sprint(t)
	}
}

func homeNumber(v any) (float64, bool) {
	var n float64
	switch t := v.(type) {
	case float64:
		n = t
	case float32:
		n = float64(t)
	case int:
		n = float64(t)
	case int64:
		n = float64(t)
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return 0, false
		}
		n = f
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err != nil {
			return 0, false
		}
		n = f
	default:
		return 0, false
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return n, true
}

// formatHomeNumber renders n with thousands separators and at most 2 decimals.
func formatHomeNumber(n float64) string {
	s := strconv.FormatFloat(math.Round(n*100)/100, 'f', -1, 64)
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	}
	intPart, frac, hasFrac := strings.Cut(s, ".")
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	out := sign + b.String()
	if hasFrac {
		out += "." + frac
	}
	if out == "-0" {
		return "0"
	}
	return out
}

func formatHomeBytes(n float64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	abs := math.Abs(n)
	i := 0
	for abs >= 1024 && i < len(units)-1 {
		abs /= 1024
		n /= 1024
		i++
	}
	if i == 0 {
		return formatHomeNumber(math.Round(n)) + " B"
	}
	return strconv.FormatFloat(math.Round(n*10)/10, 'f', -1, 64) + " " + units[i]
}

// formatHomeDuration renders seconds as the two largest units, e.g. `1h 2m`.
func formatHomeDuration(seconds float64) string {
	sign := ""
	if seconds < 0 {
		sign, seconds = "-", -seconds
	}
	total := int64(math.Round(seconds))
	parts := []struct {
		unit string
		size int64
	}{{"d", 86400}, {"h", 3600}, {"m", 60}, {"s", 1}}
	var out []string
	for _, p := range parts {
		if q := total / p.size; q > 0 {
			out = append(out, strconv.FormatInt(q, 10)+p.unit)
			total -= q * p.size
		}
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return "0s"
	}
	return sign + strings.Join(out, " ")
}

var (
	homeEnvPattern   = regexp.MustCompile(`\{\{\s*env\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)
	homeInputPattern = regexp.MustCompile(`\{\{\s*input\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)
)

// ExpandHomeEnv replaces `{{env.NAME}}` with lookup(NAME). An unknown
// variable is an error naming the variable, never any value.
func ExpandHomeEnv(s string, lookup func(string) (string, bool)) (string, error) {
	if lookup == nil {
		return "", errors.New("home: environment lookup is nil")
	}
	var missing []string
	out := homeEnvPattern.ReplaceAllStringFunc(s, func(m string) string {
		name := homeEnvPattern.FindStringSubmatch(m)[1]
		value, ok := lookup(name)
		if !ok {
			if !slices.Contains(missing, name) {
				missing = append(missing, name)
			}
			return ""
		}
		return value
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("home: environment variable %s is not set", strings.Join(missing, ", "))
	}
	return out, nil
}

// ExpandHomeInputs replaces `{{input.NAME}}` with encode(inputs[NAME]). Only
// declared names are allowed; a declared name without a value expands to
// encode("").
func ExpandHomeInputs(
	tmpl string,
	inputs map[string]string,
	declared []string,
	encode func(string) string,
) (string, error) {
	if encode == nil {
		encode = func(s string) string { return s }
	}
	var undeclared []string
	out := homeInputPattern.ReplaceAllStringFunc(tmpl, func(m string) string {
		name := homeInputPattern.FindStringSubmatch(m)[1]
		if !slices.Contains(declared, name) {
			if !slices.Contains(undeclared, name) {
				undeclared = append(undeclared, name)
			}
			return ""
		}
		return encode(inputs[name])
	})
	if len(undeclared) > 0 {
		return "", fmt.Errorf("home: input %s is not declared", strings.Join(undeclared, ", "))
	}
	return out, nil
}
