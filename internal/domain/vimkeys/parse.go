package vimkeys

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func ParseBinding(s string) (Sequence, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, ErrEmptyBinding
	}

	if key, ok := tryParseLegacyAtom(s); ok {
		return Sequence{key}, nil
	}

	if key, ok, err := tryParseAsLegacyChord(s); ok {
		if err != nil {
			return nil, err
		}
		return Sequence{key}, nil
	}
	return parseRawSequence(s)
}

// tryParseLegacyAtom treats raw UI key names enter/escape (and aliases) as single
// keys so they remain compatible with the GTK shortcut table contract.
func tryParseLegacyAtom(s string) (Key, bool) {
	switch strings.ToLower(s) {
	case "enter", "return":
		return Key{Sym: "CR"}, true
	case "escape", "esc":
		return Key{Sym: "Esc"}, true
	default:
		return Key{}, false
	}
}

func legacyChordParts(s string) []string {
	parts := strings.Split(s, "+")
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			tokens = append(tokens, part)
		}
	}
	return tokens
}

func isModifierName(part string) bool {
	switch strings.ToLower(part) {
	case "ctrl", "control", "shift", "alt":
		return true
	default:
		return false
	}
}

func tryParseAsLegacyChord(s string) (Key, bool, error) {
	if s == "+" {
		return Key{Sym: "+"}, true, nil
	}

	parts := legacyChordParts(s)
	if !legacyChordLooksLikeAttempt(s, parts) {
		return Key{}, false, nil
	}

	key, err := buildLegacyChordKey(s, parts)
	return key, true, err
}

func legacyChordLooksLikeAttempt(s string, parts []string) bool {
	if strings.HasSuffix(s, "++") {
		return true
	}
	for _, part := range parts {
		if isModifierName(part) {
			return true
		}
	}
	return false
}

func validateLegacyChordLayout(s string, parts []string) error {
	if !strings.Contains(s, "+") {
		return ErrBadBinding
	}
	if strings.HasSuffix(s, "++") {
		return nil
	}
	for i, part := range parts {
		if isModifierName(part) {
			continue
		}
		if i != len(parts)-1 {
			return ErrBadBinding
		}
	}
	return nil
}

func modsFromLegacyPart(part string) (Mods, bool) {
	switch strings.ToLower(part) {
	case "ctrl", "control":
		return ModCtrl, true
	case "shift":
		return ModShift, true
	case "alt":
		return ModAlt, true
	default:
		return 0, false
	}
}

func collectLegacyChordModsAndKey(s string, parts []string) (Mods, string, error) {
	if err := validateLegacyChordLayout(s, parts); err != nil {
		return 0, "", err
	}

	var mods Mods
	keyPart := ""
	for _, part := range parts {
		if mod, ok := modsFromLegacyPart(part); ok {
			mods |= mod
			continue
		}
		if keyPart != "" {
			return 0, "", ErrBadBinding
		}
		keyPart = part
	}

	if keyPart == "" && strings.HasSuffix(s, "++") {
		keyPart = "+"
	}
	if keyPart == "" {
		return 0, "", ErrBadBinding
	}
	return mods, keyPart, nil
}

func legacyKeyPartWithShift(keyPart string, mods Mods) (string, Mods) {
	if len(keyPart) == 1 && keyPart[0] >= 'A' && keyPart[0] <= 'Z' {
		return strings.ToLower(keyPart), mods | ModShift
	}
	return keyPart, mods
}

func buildLegacyChordKey(s string, parts []string) (Key, error) {
	mods, keyPart, err := collectLegacyChordModsAndKey(s, parts)
	if err != nil {
		return Key{}, err
	}
	keyPart, mods = legacyKeyPartWithShift(keyPart, mods)
	sym, err := symFromToken(keyPart, false)
	if err != nil {
		return Key{}, err
	}
	return Key{Sym: sym, Mods: mods}, nil
}

func parseAngleToken(inner string) (Key, error) {
	if strings.Contains(inner, "-") {
		parts := strings.Split(inner, "-")
		if len(parts) < 2 {
			return Key{}, ErrBadBinding
		}
		symPart := strings.TrimSpace(parts[len(parts)-1])
		if symPart == "" {
			return Key{}, ErrBadBinding
		}

		var mods Mods
		for _, modPart := range parts[:len(parts)-1] {
			modPart = strings.TrimSpace(modPart)
			if modPart == "" {
				return Key{}, ErrBadBinding
			}
			partMods, err := modsFromCompactModString(modPart)
			if err != nil {
				return Key{}, err
			}
			mods |= partMods
		}

		sym, err := symFromToken(symPart, true)
		if err != nil {
			return Key{}, err
		}
		return Key{Sym: sym, Mods: mods}, nil
	}

	sym, err := symFromAlias(inner)
	if err != nil {
		return Key{}, err
	}
	return Key{Sym: sym}, nil
}

func modsFromCompactModString(modPart string) (Mods, error) {
	var mods Mods
	for _, ch := range modPart {
		switch ch {
		case 'C', 'c':
			mods |= ModCtrl
		case 'S', 's':
			mods |= ModShift
		case 'A', 'a':
			mods |= ModAlt
		default:
			return 0, ErrBadBinding
		}
	}
	return mods, nil
}

func parseRawSequence(s string) (Sequence, error) {
	seq := make(Sequence, 0, len(s))
	for i := 0; i < len(s); {
		if s[i] == '<' {
			key, width, err := parseAngleAt(s, i)
			if err != nil {
				return nil, err
			}
			seq = append(seq, key)
			i += width
			continue
		}
		// Named keys (Space, Esc, Tab, …) are canonical only inside <...>.
		// Bare prefixes must not be greedily consumed so printable runs like
		// "Spacej" (Shift+s,p,a,c,e,j) round-trip via Sequence.String().
		if key, width, ok := tryParseCanonicalAt(s, i); ok {
			seq = append(seq, key)
			i += width
			continue
		}

		r, width := utf8.DecodeRuneInString(s[i:])
		key, err := keyFromRune(r)
		if err != nil {
			return nil, err
		}
		seq = append(seq, key)
		i += width
	}
	return seq, nil
}

func parseAngleAt(s string, i int) (Key, int, error) {
	if i >= len(s) || s[i] != '<' {
		return Key{}, 0, ErrBadBinding
	}
	closeIdx := strings.IndexByte(s[i+1:], '>')
	if closeIdx < 0 {
		return Key{}, 0, ErrBadBinding
	}
	closeIdx += i + 1
	inner := s[i+1 : closeIdx]
	if inner == "" {
		return Key{}, 0, ErrBadBinding
	}
	key, err := parseAngleToken(inner)
	if err != nil {
		return Key{}, 0, err
	}
	return key, closeIdx - i + 1, nil
}

func tryParseCanonicalAt(s string, i int) (Key, int, bool) {
	if i >= len(s) {
		return Key{}, 0, false
	}
	switch s[i] {
	case 'C', 'S', 'A':
	default:
		return Key{}, 0, false
	}
	if i+1 >= len(s) || s[i+1] != '-' {
		return Key{}, 0, false
	}

	end := len(s)
	nextDash := strings.IndexByte(s[i:], '-')
	if nextDash >= 0 {
		nextDash += i
		if nextDash+1 < len(s) {
			switch s[nextDash+1] {
			case 'C', 'S', 'A':
				end = nextDash
			}
		}
	}

	token := s[i:end]
	key, err := parseCanonicalToken(token)
	if err != nil {
		return Key{}, 0, false
	}
	return key, end - i, true
}

func parseCanonicalToken(token string) (Key, error) {
	parts := strings.Split(token, "-")
	if len(parts) < 2 {
		return Key{}, ErrBadBinding
	}

	var mods Mods
	for i := 0; i < len(parts)-1; i++ {
		switch parts[i] {
		case "C":
			mods |= ModCtrl
		case "S":
			mods |= ModShift
		case "A":
			mods |= ModAlt
		default:
			return Key{}, ErrBadBinding
		}
	}

	symPart := parts[len(parts)-1]
	sym, err := symFromToken(symPart, true)
	if err != nil {
		return Key{}, err
	}
	return Key{Sym: sym, Mods: mods}, nil
}

func keyFromRune(r rune) (Key, error) {
	if r >= 'A' && r <= 'Z' {
		return Key{Sym: strings.ToLower(string(r)), Mods: ModShift}, nil
	}
	if shiftedGlyph, ok := shiftedGlyphForRune(r); ok {
		return Key{Sym: shiftedGlyph}, nil
	}
	if r >= 'a' && r <= 'z' {
		return Key{Sym: string(r)}, nil
	}
	if unicode.IsPrint(r) {
		return Key{Sym: string(r)}, nil
	}
	return Key{}, ErrBadBinding
}

func shiftedGlyphForRune(r rune) (string, bool) {
	switch r {
	case '!', '@', '#', '$', '%', '^', '&', '*', '(', ')', '_', '+', '{', '}', '|', ':', '"', '~', '<', '>', '?':
		return string(r), true
	default:
		return "", false
	}
}

func symFromSpecialAlias(token string) (string, bool) {
	lower := strings.ToLower(token)
	switch lower {
	case "cr", "return":
		return "CR", true
	case "esc", "escape":
		return "Esc", true
	case "space":
		return "Space", true
	case "lt":
		return "<", true
	case "plus":
		return "+", true
	default:
		return namedSymFromAlias(lower)
	}
}

func namedSymFromAlias(lower string) (string, bool) {
	switch lower {
	case "tab":
		return "Tab", true
	case "backspace":
		return "BackSpace", true
	case "delete":
		return "Delete", true
	case "left":
		return "Left", true
	case "right":
		return "Right", true
	case "up":
		return "Up", true
	case "down":
		return "Down", true
	case "home":
		return "Home", true
	case "end":
		return "End", true
	case "pageup":
		return "PageUp", true
	case "pagedown":
		return "PageDown", true
	default:
		return "", false
	}
}

func symFromToken(token string, allowSingleLetter bool) (string, error) {
	if sym, ok := symFromSpecialAlias(token); ok {
		return sym, nil
	}
	if allowSingleLetter && len(token) == 1 {
		ch := token[0]
		if ch >= 'A' && ch <= 'Z' {
			return strings.ToLower(token), nil
		}
		if ch >= 'a' && ch <= 'z' {
			return token, nil
		}
	}
	if len(token) == 1 && unicode.IsPrint(rune(token[0])) {
		return token, nil
	}
	return "", ErrBadBinding
}

func symFromAlias(token string) (string, error) {
	if sym, ok := symFromSpecialAlias(token); ok {
		return sym, nil
	}
	return "", ErrBadBinding
}
