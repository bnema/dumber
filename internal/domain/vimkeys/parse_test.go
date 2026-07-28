package vimkeys

import (
	"errors"
	"testing"
)

func TestParseBinding_SingleKeys(t *testing.T) {
	tests := []struct {
		input string
		want  Sequence
	}{
		{input: "j", want: Sequence{{Sym: "j"}}},
		{input: "shift+j", want: Sequence{{Sym: "j", Mods: ModShift}}},
		{input: "ctrl++", want: Sequence{{Sym: "+", Mods: ModCtrl}}},
		{input: "<C-d>", want: Sequence{{Sym: "d", Mods: ModCtrl}}},
		{input: "<CR>", want: Sequence{{Sym: "CR"}}},
		{input: "<Return>", want: Sequence{{Sym: "CR"}}},
		{input: "<Esc>", want: Sequence{{Sym: "Esc"}}},
		{input: "<Escape>", want: Sequence{{Sym: "Esc"}}},
		{input: "<Space>", want: Sequence{{Sym: "Space"}}},
		{input: "<lt>", want: Sequence{{Sym: "<"}}},
		{input: "<Plus>", want: Sequence{{Sym: "+"}}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseBinding(tt.input)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", tt.input, err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("ParseBinding(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseBinding_Sequences(t *testing.T) {
	tests := []struct {
		input string
		want  Sequence
	}{
		{input: "]]", want: Sequence{{Sym: "]"}, {Sym: "]"}}},
		{input: "yah", want: Sequence{{Sym: "y"}, {Sym: "a"}, {Sym: "h"}}},
		{input: "gO", want: Sequence{{Sym: "g"}, {Sym: "o", Mods: ModShift}}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseBinding(tt.input)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", tt.input, err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("ParseBinding(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseBinding_Malformed(t *testing.T) {
	tests := []struct {
		input string
		want  error
	}{
		{input: "", want: ErrEmptyBinding},
		{input: "   ", want: ErrEmptyBinding},
		{input: "<", want: ErrBadBinding},
		{input: "<>", want: ErrBadBinding},
		{input: "<Bad>", want: ErrBadBinding},
		{input: "shift+", want: ErrBadBinding},
		{input: "ctrl+", want: ErrBadBinding},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := ParseBinding(tt.input)
			if err == nil {
				t.Fatalf("ParseBinding(%q) expected error", tt.input)
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("ParseBinding(%q) error = %v, want %v", tt.input, err, tt.want)
			}
		})
	}
}

func TestParseBinding_CanonicalString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "j", want: "j"},
		{input: "shift+j", want: "J"},
		{input: "ctrl++", want: "<C-+>"},
		{input: "<C-d>", want: "<C-d>"},
		{input: "<CR>", want: "<Return>"},
		{input: "<Return>", want: "<Return>"},
		{input: "<Esc>", want: "<Escape>"},
		{input: "<Escape>", want: "<Escape>"},
		{input: "<Space>", want: "<Space>"},
		{input: "<lt>", want: "<lt>"},
		{input: "gO", want: "gO"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			seq, err := ParseBinding(tt.input)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", tt.input, err)
			}
			if got := seq.String(); got != tt.want {
				t.Fatalf("ParseBinding(%q).String() = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseBinding_RoundTrip(t *testing.T) {
	inputs := []string{
		"j",
		"shift+j",
		"ctrl++",
		"]]",
		"yah",
		"gO",
		"<C-d>",
		"<CR>",
		"<Esc>",
		"<Space>",
		"<lt>",
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			first, err := ParseBinding(input)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", input, err)
			}
			canonical := first.String()
			second, err := ParseBinding(canonical)
			if err != nil {
				t.Fatalf("ParseBinding(%q) from canonical %q error = %v", input, canonical, err)
			}
			if !first.Equal(second) {
				t.Fatalf("round trip %q -> %q -> %v, want %v", input, canonical, second, first)
			}
		})
	}
}
