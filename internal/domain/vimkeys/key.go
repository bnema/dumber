package vimkeys

import (
	"errors"
	"strings"
)

// Mods holds keyboard modifier flags for a parsed key.
type Mods uint8

const (
	ModCtrl Mods = 1 << iota
	ModShift
	ModAlt
)

const (
	symCR  = "CR"
	symEsc = "Esc"
)

// Key is a single key event with symbol and modifiers.
type Key struct {
	Sym  string
	Mods Mods
}

// Sequence is an ordered list of keys forming a Vim-style binding.
type Sequence []Key

var (
	ErrEmptyBinding = errors.New("vimkeys: empty binding")
	ErrBadBinding   = errors.New("vimkeys: malformed binding")
)

// String renders a key in canonical form.
func (k Key) String() string {
	if k.Sym == "" {
		return ""
	}

	if k.Mods == ModShift && len(k.Sym) == 1 {
		if c := k.Sym[0]; c >= 'a' && c <= 'z' {
			return strings.ToUpper(k.Sym)
		}
	}

	if k.Mods == 0 {
		if k.Sym == "+" {
			return "<Plus>"
		}
		if alias := angleAliasForSym(k.Sym); alias != "" {
			return "<" + alias + ">"
		}
		return k.Sym
	}

	return formatAngleKey(k)
}

// String renders the full binding sequence in canonical form.
// When the naive concatenation would case-insensitively equal an exact legacy
// atom (enter/return/escape/esc), the first key is forced to an angle literal
// so ParseBinding cannot reabsorb the sequence as that atom.
func (s Sequence) String() string {
	rendered := s.render(false)
	if collidesWithLegacyAtom(rendered) {
		return s.render(true)
	}
	return rendered
}

func (s Sequence) render(forceAngleFirst bool) string {
	var b strings.Builder
	for i, key := range s {
		if forceAngleFirst && i == 0 {
			b.WriteString(key.angleLiteralString())
			continue
		}
		b.WriteString(key.String())
	}
	return b.String()
}

func collidesWithLegacyAtom(s string) bool {
	switch strings.ToLower(s) {
	case "enter", "return", "escape", "esc":
		return true
	default:
		return false
	}
}

// angleLiteralString forces angle-bracket form for keys that Key.String would
// otherwise emit as bare or shifted letters (e.g. "e" / "E").
func (k Key) angleLiteralString() string {
	if len(k.Sym) == 1 {
		c := k.Sym[0]
		if c >= 'a' && c <= 'z' {
			switch k.Mods {
			case 0:
				return "<" + k.Sym + ">"
			case ModShift:
				return formatAngleKey(k)
			}
		}
	}
	return k.String()
}

// Equal reports whether two sequences contain the same keys.
func (s Sequence) Equal(other Sequence) bool {
	if len(s) != len(other) {
		return false
	}
	for i := range s {
		if s[i] != other[i] {
			return false
		}
	}
	return true
}

func angleAliasForSym(sym string) string {
	switch sym {
	case symCR:
		return "Return"
	case symEsc:
		return "Escape"
	case "Space", "Tab", "BackSpace", "Delete",
		"Left", "Right", "Up", "Down",
		"Home", "End", "PageUp", "PageDown":
		return sym
	case "<":
		return "lt"
	case ">":
		return "gt"
	default:
		return ""
	}
}

func formatAngleKey(k Key) string {
	var b strings.Builder
	b.WriteByte('<')
	if k.Mods&ModCtrl != 0 {
		b.WriteByte('C')
	}
	if k.Mods&ModShift != 0 {
		if b.Len() > 1 {
			b.WriteByte('-')
		}
		b.WriteByte('S')
	}
	if k.Mods&ModAlt != 0 {
		if b.Len() > 1 {
			b.WriteByte('-')
		}
		b.WriteByte('A')
	}
	b.WriteByte('-')
	if alias := angleAliasForSym(k.Sym); alias != "" {
		b.WriteString(alias)
	} else {
		b.WriteString(k.Sym)
	}
	b.WriteByte('>')
	return b.String()
}
