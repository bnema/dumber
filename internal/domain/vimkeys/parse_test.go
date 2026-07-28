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

func TestParseBinding_LegacyChords(t *testing.T) {
	tests := []struct {
		input string
		want  Sequence
	}{
		{input: "ctrl+d", want: Sequence{{Sym: "d", Mods: ModCtrl}}},
		{input: "control+j", want: Sequence{{Sym: "j", Mods: ModCtrl}}},
		{input: "alt+k", want: Sequence{{Sym: "k", Mods: ModAlt}}},
		{input: "ctrl+J", want: Sequence{{Sym: "j", Mods: ModCtrl | ModShift}}},
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

func TestParseBinding_MoreMalformed(t *testing.T) {
	tests := []string{
		"ctrl+shift+j+k",
		"<C->",
		"<C--d>",
		"<Bad>",
		"ctrl+",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := ParseBinding(input)
			if err == nil {
				t.Fatalf("ParseBinding(%q) expected error", input)
			}
			if !errors.Is(err, ErrBadBinding) {
				t.Fatalf("ParseBinding(%q) error = %v, want %v", input, err, ErrBadBinding)
			}
		})
	}
}

func TestParseBinding_PlusAlone(t *testing.T) {
	got, err := ParseBinding("+")
	if err != nil {
		t.Fatalf("ParseBinding error = %v", err)
	}
	want := Sequence{{Sym: "+"}}
	if !got.Equal(want) {
		t.Fatalf("ParseBinding = %v, want %v", got, want)
	}
}

func TestParseBinding_LooksLikeChordButRaw(t *testing.T) {
	got, err := ParseBinding("]c")
	if err != nil {
		t.Fatalf("ParseBinding error = %v", err)
	}
	want := Sequence{{Sym: "]"}, {Sym: "c"}}
	if !got.Equal(want) {
		t.Fatalf("ParseBinding = %v, want %v", got, want)
	}
}

func TestParseBinding_CanonicalInlineModifiers(t *testing.T) {
	tests := []struct {
		input string
		want  Sequence
	}{
		{input: "C-d", want: Sequence{{Sym: "d", Mods: ModCtrl}}},
		{input: "S-j", want: Sequence{{Sym: "j", Mods: ModShift}}},
		{input: "A-k", want: Sequence{{Sym: "k", Mods: ModAlt}}},
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

func TestParseBinding_SpecialNamesInSequence(t *testing.T) {
	tests := []struct {
		input string
		want  Sequence
	}{
		{input: "jEsc", want: Sequence{{Sym: "j"}, {Sym: "Esc"}}},
		{input: "Spacej", want: Sequence{{Sym: "Space"}, {Sym: "j"}}},
		{input: "CRj", want: Sequence{{Sym: "CR"}, {Sym: "j"}}},
		{input: "ltj", want: Sequence{{Sym: "<"}, {Sym: "j"}}},
		{input: "Plusj", want: Sequence{{Sym: "+"}, {Sym: "j"}}},
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

func TestParseBinding_PrintableGlyphs(t *testing.T) {
	got, err := ParseBinding("!@#")
	if err != nil {
		t.Fatalf("ParseBinding error = %v", err)
	}
	want := Sequence{{Sym: "!"}, {Sym: "@"}, {Sym: "#"}}
	if !got.Equal(want) {
		t.Fatalf("ParseBinding = %v, want %v", got, want)
	}
}

func TestParseBinding_MixedAngleSequences(t *testing.T) {
	tests := []struct {
		input string
		want  Sequence
	}{
		{
			input: "<C-d>j",
			want:  Sequence{{Sym: "d", Mods: ModCtrl}, {Sym: "j"}},
		},
		{
			input: "<Escape>j",
			want:  Sequence{{Sym: "Esc"}, {Sym: "j"}},
		},
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

func TestParseBinding_MultiModifierRoundTrip(t *testing.T) {
	input := "<C-S-a>"
	first, err := ParseBinding(input)
	if err != nil {
		t.Fatalf("ParseBinding(%q) error = %v", input, err)
	}
	want := Sequence{{Sym: "a", Mods: ModCtrl | ModShift}}
	if !first.Equal(want) {
		t.Fatalf("ParseBinding(%q) = %v, want %v", input, first, want)
	}

	canonical := first.String()
	if canonical != "<C-S-a>" {
		t.Fatalf("String() = %q, want %q", canonical, "<C-S-a>")
	}

	second, err := ParseBinding(canonical)
	if err != nil {
		t.Fatalf("ParseBinding(%q) error = %v", canonical, err)
	}
	if !first.Equal(second) {
		t.Fatalf("round trip %q -> %q -> %v, want %v", input, canonical, second, first)
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
		"<C-d>j",
		"<Escape>j",
		"<C-S-a>",
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
