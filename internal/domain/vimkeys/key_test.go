package vimkeys

import (
	"testing"
)

func TestKeyString(t *testing.T) {
	tests := []struct {
		name string
		key  Key
		want string
	}{
		{name: "letter", key: Key{Sym: "j"}, want: "j"},
		{name: "shift letter", key: Key{Sym: "j", Mods: ModShift}, want: "J"},
		{name: "ctrl letter", key: Key{Sym: "d", Mods: ModCtrl}, want: "<C-d>"},
		{name: "ctrl plus", key: Key{Sym: "+", Mods: ModCtrl}, want: "<C-+>"},
		{name: "shift glyph", key: Key{Sym: "!"}, want: "!"},
		{name: "special CR", key: Key{Sym: "CR"}, want: "<Return>"},
		{name: "special Esc", key: Key{Sym: "Esc"}, want: "<Escape>"},
		{name: "special Space", key: Key{Sym: "Space"}, want: "<Space>"},
		{name: "literal lt", key: Key{Sym: "<"}, want: "<lt>"},
		{name: "alt only", key: Key{Sym: "b", Mods: ModAlt}, want: "<A-b>"},
		{name: "ctrl alt", key: Key{Sym: "c", Mods: ModCtrl | ModAlt}, want: "<C-A-c>"},
		{name: "ctrl shift", key: Key{Sym: "a", Mods: ModCtrl | ModShift}, want: "<C-S-a>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.key.String(); got != tt.want {
				t.Errorf("Key.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSequenceString(t *testing.T) {
	tests := []struct {
		name string
		seq  Sequence
		want string
	}{
		{name: "yah", seq: Sequence{{Sym: "y"}, {Sym: "a"}, {Sym: "h"}}, want: "yah"},
		{name: "double bracket", seq: Sequence{{Sym: "]"}, {Sym: "]"}}, want: "]]"},
		{name: "gO", seq: Sequence{{Sym: "g"}, {Sym: "o", Mods: ModShift}}, want: "gO"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.seq.String(); got != tt.want {
				t.Errorf("Sequence.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSequenceEqual(t *testing.T) {
	a := Sequence{{Sym: "j"}, {Sym: "k"}}
	b := Sequence{{Sym: "j"}, {Sym: "k"}}
	c := Sequence{{Sym: "j"}}
	if !a.Equal(b) {
		t.Fatal("equal sequences reported unequal")
	}
	if a.Equal(c) {
		t.Fatal("unequal length sequences reported equal")
	}
	if a.Equal(Sequence{{Sym: "j"}, {Sym: "x"}}) {
		t.Fatal("different keys reported equal")
	}
}

func TestKeyStringEmpty(t *testing.T) {
	if got := (Key{}).String(); got != "" {
		t.Fatalf("empty Key.String() = %q, want empty", got)
	}
}

func TestErrorSentinels(t *testing.T) {
	if ErrEmptyBinding.Error() != "vimkeys: empty binding" {
		t.Errorf("ErrEmptyBinding = %q, want %q", ErrEmptyBinding.Error(), "vimkeys: empty binding")
	}
	if ErrBadBinding.Error() != "vimkeys: malformed binding" {
		t.Errorf("ErrBadBinding = %q, want %q", ErrBadBinding.Error(), "vimkeys: malformed binding")
	}
}
