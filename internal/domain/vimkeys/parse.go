package vimkeys

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

var specialAliases = map[string]string{
	"cr":     "CR",
	"return": "CR",
	"esc":    "Esc",
	"escape": "Esc",
	"space":  "Space",
	"lt":     "<",
	"plus":   "+",
}

func ParseBinding(s string) (Sequence, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, ErrEmptyBinding
	}

	if looksLikeChordAttempt(s) {
		if !isLegacyChord(s) {
			return nil, ErrBadBinding
		}
		key, err := parseLegacyChord(s)
		if err != nil {
			return nil, err
		}
		return Sequence{key}, nil
	}
	return parseRawSequence(s)
}

func looksLikeChordAttempt(s string) bool {
	if s == "+" {
		return true
	}
	if strings.HasSuffix(s, "++") {
		return true
	}
	for _, part := range strings.Split(s, "+") {
		if isModifierName(part) {
			return true
		}
	}
	return false
}

func isLegacyChord(s string) bool {
	if s == "+" {
		return true
	}
	if !strings.Contains(s, "+") {
		return false
	}
	if strings.HasSuffix(s, "++") {
		return true
	}
	parts := strings.Split(s, "+")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if isModifierName(part) {
			continue
		}
		if i == len(parts)-1 {
			return true
		}
		return false
	}
	return false
}

func isModifierName(part string) bool {
	switch strings.ToLower(part) {
	case "ctrl", "control", "shift", "alt":
		return true
	default:
		return false
	}
}

func parseLegacyChord(s string) (Key, error) {
	if s == "+" {
		return Key{Sym: "+"}, nil
	}

	parts := strings.Split(s, "+")
	var mods Mods
	var keyPart string

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lower := strings.ToLower(part)
		switch lower {
		case "ctrl", "control":
			mods |= ModCtrl
		case "shift":
			mods |= ModShift
		case "alt":
			mods |= ModAlt
		default:
			if keyPart != "" {
				return Key{}, ErrBadBinding
			}
			keyPart = part
		}
	}

	if keyPart == "" && strings.HasSuffix(s, "++") {
		keyPart = "+"
	}
	if keyPart == "" {
		return Key{}, ErrBadBinding
	}

	if len(keyPart) == 1 && keyPart[0] >= 'A' && keyPart[0] <= 'Z' {
		mods |= ModShift
		keyPart = strings.ToLower(keyPart)
	}

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
			for _, ch := range modPart {
				switch ch {
				case 'C', 'c':
					mods |= ModCtrl
				case 'S', 's':
					mods |= ModShift
				case 'A', 'a':
					mods |= ModAlt
				default:
					return Key{}, ErrBadBinding
				}
			}
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

var sequenceSpecialNames = []string{"Space", "Esc", "CR", "lt", "Plus"}

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
		if key, width, ok := tryParseSpecialAt(s, i); ok {
			seq = append(seq, key)
			i += width
			continue
		}
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

func tryParseSpecialAt(s string, i int) (Key, int, bool) {
	for _, name := range sequenceSpecialNames {
		if !strings.HasPrefix(s[i:], name) {
			continue
		}
		sym, err := symFromAlias(name)
		if err != nil {
			continue
		}
		return Key{Sym: sym}, len(name), true
	}
	return Key{}, 0, false
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

func symFromToken(token string, allowSingleLetter bool) (string, error) {
	if sym, ok := specialAliases[strings.ToLower(token)]; ok {
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
	if sym, ok := specialAliases[strings.ToLower(token)]; ok {
		return sym, nil
	}
	return "", ErrBadBinding
}
