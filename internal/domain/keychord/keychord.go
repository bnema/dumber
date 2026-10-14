// Package keychord parses and canonicalizes single key chords such as
// "ctrl+shift+t" or "arrowright".
//
// It is the one place that knows which spellings name the same key (aliases,
// modifier names, letter case), so every consumer agrees on when two config
// strings are the same binding. It has no toolkit dependency: the input layer
// maps the canonical key names to its own key values.
package keychord

import (
	"slices"
	"strings"
)

// Mods holds the modifier flags of a chord.
type Mods uint8

// Modifier flags.
const (
	Ctrl Mods = 1 << iota
	Alt
	Shift
)

// Chord is a parsed key chord. Key is the canonical key name: one of the
// names returned by KeyNames, or a single printable ASCII character.
type Chord struct {
	Mods Mods
	Key  string
}

// String renders the chord in canonical form: modifiers in the fixed order
// ctrl, alt, shift, followed by the key.
func (c Chord) String() string {
	var b strings.Builder
	if c.Mods&Ctrl != 0 {
		b.WriteString("ctrl+")
	}
	if c.Mods&Alt != 0 {
		b.WriteString("alt+")
	}
	if c.Mods&Shift != 0 {
		b.WriteString("shift+")
	}
	b.WriteString(c.Key)
	return b.String()
}

// keyAliases maps every accepted lower-case spelling of a named key to its
// canonical name. Canonical names map to themselves.
var keyAliases = map[string]string{
	"escape":       "escape",
	"esc":          "escape",
	"return":       "return",
	"enter":        "return",
	"tab":          "tab",
	"space":        "space",
	"backspace":    "backspace",
	"delete":       "delete",
	"del":          "delete",
	"home":         "home",
	"end":          "end",
	"pageup":       "pageup",
	"page_up":      "pageup",
	"pagedown":     "pagedown",
	"page_down":    "pagedown",
	"left":         "left",
	"arrowleft":    "left",
	"right":        "right",
	"arrowright":   "right",
	"up":           "up",
	"arrowup":      "up",
	"down":         "down",
	"arrowdown":    "down",
	"f1":           "f1",
	"f2":           "f2",
	"f3":           "f3",
	"f4":           "f4",
	"f5":           "f5",
	"f6":           "f6",
	"f7":           "f7",
	"f8":           "f8",
	"f9":           "f9",
	"f10":          "f10",
	"f11":          "f11",
	"f12":          "f12",
	"plus":         "plus",
	"+":            "plus",
	"minus":        "minus",
	"-":            "minus",
	"equal":        "equal",
	"=":            "equal",
	"bracketleft":  "bracketleft",
	"[":            "bracketleft",
	"bracketright": "bracketright",
	"]":            "bracketright",
	"braceleft":    "braceleft",
	"{":            "braceleft",
	"braceright":   "braceright",
	"}":            "braceright",
}

// KeyNames returns the canonical names of the named keys, sorted. Printable
// ASCII characters are also valid keys but are not listed.
func KeyNames() []string {
	seen := make(map[string]struct{})
	for _, canonical := range keyAliases {
		seen[canonical] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// canonicalKey resolves a key spelling to its canonical name.
func canonicalKey(name string) (string, bool) {
	if canonical, ok := keyAliases[strings.ToLower(name)]; ok {
		return canonical, true
	}
	// Single printable ASCII keys (letters, digits, symbols such as %, ", &).
	if len(name) == 1 && name[0] > ' ' && name[0] <= '~' {
		return name, true
	}
	return "", false
}

// Parse parses a config key string such as "ctrl+t", "Control+Shift+T" or
// "alt+left". Modifier names and named keys are case-insensitive; an upper-case
// single letter implies Shift. It returns false if the string is not a valid
// single chord.
func Parse(s string) (Chord, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Chord{}, false
	}
	if s == "+" {
		return Chord{Key: "plus"}, true
	}

	var mods Mods
	var keyPart string
	for _, part := range strings.Split(s, "+") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		switch strings.ToLower(part) {
		case "ctrl", "control":
			mods |= Ctrl
		case "shift":
			mods |= Shift
		case "alt":
			mods |= Alt
		default:
			if keyPart != "" {
				return Chord{}, false
			}
			keyPart = part
		}
	}

	// Allow "ctrl++" / "alt+shift++" where the key is "+".
	if keyPart == "" && strings.HasSuffix(s, "++") {
		keyPart = "+"
	}
	if keyPart == "" {
		return Chord{}, false
	}

	// An upper-case single letter is Shift+<letter>.
	if len(keyPart) == 1 && keyPart[0] >= 'A' && keyPart[0] <= 'Z' {
		mods |= Shift
		keyPart = strings.ToLower(keyPart)
	}

	key, ok := canonicalKey(keyPart)
	if !ok {
		return Chord{}, false
	}
	return Chord{Mods: mods, Key: key}, true
}

// Canonicalize returns the canonical spelling of a key string, so that two
// strings that name the same binding compare equal. It returns false if the
// string is not a valid single chord.
func Canonicalize(s string) (string, bool) {
	chord, ok := Parse(s)
	if !ok {
		return "", false
	}
	return chord.String(), true
}
