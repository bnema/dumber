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
		if alias := angleAliasForSym(k.Sym); alias != "" {
			return "<" + alias + ">"
		}
		return k.Sym
	}

	return formatAngleKey(k)
}

// String renders the full binding sequence in canonical form.
func (s Sequence) String() string {
	var b strings.Builder
	for _, key := range s {
		b.WriteString(key.String())
	}
	return b.String()
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
	case "CR":
		return "Return"
	case "Esc":
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
