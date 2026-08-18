package vimkeys

import (
	"errors"
	"strings"
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
		{input: "ctrl+-", want: Sequence{{Sym: "-", Mods: ModCtrl}}},
		{input: "<C-d>", want: Sequence{{Sym: "d", Mods: ModCtrl}}},
		{input: "<CR>", want: Sequence{{Sym: "CR"}}},
		{input: "<Return>", want: Sequence{{Sym: "CR"}}},
		{input: "<Esc>", want: Sequence{{Sym: "Esc"}}},
		{input: "<Escape>", want: Sequence{{Sym: "Esc"}}},
		{input: "<Space>", want: Sequence{{Sym: "Space"}}},
		{input: "<Tab>", want: Sequence{{Sym: "Tab"}}},
		{input: "<BackSpace>", want: Sequence{{Sym: "BackSpace"}}},
		{input: "<Delete>", want: Sequence{{Sym: "Delete"}}},
		{input: "<Left>", want: Sequence{{Sym: "Left"}}},
		{input: "<Right>", want: Sequence{{Sym: "Right"}}},
		{input: "<Up>", want: Sequence{{Sym: "Up"}}},
		{input: "<Down>", want: Sequence{{Sym: "Down"}}},
		{input: "<Home>", want: Sequence{{Sym: "Home"}}},
		{input: "<End>", want: Sequence{{Sym: "End"}}},
		{input: "<PageUp>", want: Sequence{{Sym: "PageUp"}}},
		{input: "<PageDown>", want: Sequence{{Sym: "PageDown"}}},
		{input: "<C-Tab>", want: Sequence{{Sym: "Tab", Mods: ModCtrl}}},
		{input: "<S-BackSpace>", want: Sequence{{Sym: "BackSpace", Mods: ModShift}}},
		{input: "<A-Delete>", want: Sequence{{Sym: "Delete", Mods: ModAlt}}},
		{input: "<C-Left>", want: Sequence{{Sym: "Left", Mods: ModCtrl}}},
		{input: "<lt>", want: Sequence{{Sym: "<"}}},
		{input: "<gt>", want: Sequence{{Sym: ">"}}},
		{input: "<C-gt>", want: Sequence{{Sym: ">", Mods: ModCtrl}}},
		{input: "<Plus>", want: Sequence{{Sym: "+"}}},
		{input: "enter", want: Sequence{{Sym: "CR"}}},
		{input: "escape", want: Sequence{{Sym: "Esc"}}},
		{input: "Enter", want: Sequence{{Sym: "CR"}}},
		{input: "Escape", want: Sequence{{Sym: "Esc"}}},
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
		{input: "ctrl+-", want: "<C-->"},
		{input: "<C-->", want: "<C-->"},
		{input: "<C-d>", want: "<C-d>"},
		{input: "<Plus>", want: "<Plus>"},
		{input: "+", want: "<Plus>"},
		{input: "<CR>", want: "<Return>"},
		{input: "<Return>", want: "<Return>"},
		{input: "<Esc>", want: "<Escape>"},
		{input: "<Escape>", want: "<Escape>"},
		{input: "<Space>", want: "<Space>"},
		{input: "<Tab>", want: "<Tab>"},
		{input: "<BackSpace>", want: "<BackSpace>"},
		{input: "<Delete>", want: "<Delete>"},
		{input: "<Left>", want: "<Left>"},
		{input: "<Right>", want: "<Right>"},
		{input: "<Up>", want: "<Up>"},
		{input: "<Down>", want: "<Down>"},
		{input: "<Home>", want: "<Home>"},
		{input: "<End>", want: "<End>"},
		{input: "<PageUp>", want: "<PageUp>"},
		{input: "<PageDown>", want: "<PageDown>"},
		{input: "<C-Tab>", want: "<C-Tab>"},
		{input: "<S-BackSpace>", want: "<S-BackSpace>"},
		{input: "<A-Delete>", want: "<A-Delete>"},
		{input: "<C-Home>", want: "<C-Home>"},
		{input: "<A-PageDown>", want: "<A-PageDown>"},
		{input: "<lt>", want: "<lt>"},
		{input: "<gt>", want: "<gt>"},
		{input: ">", want: "<gt>"},
		{input: "ctrl+>", want: "<C-gt>"},
		{input: "<C-gt>", want: "<C-gt>"},
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

func TestParseBinding_BareModifierLikeIsLiteral(t *testing.T) {
	// Modifiers are canonical only inside <...>; bare "C-d" is Shift+c, '-', d.
	tests := []struct {
		input string
		want  Sequence
	}{
		{
			input: "C-d",
			want: Sequence{
				{Sym: "c", Mods: ModShift},
				{Sym: "-"},
				{Sym: "d"},
			},
		},
		{
			input: "S-j",
			want: Sequence{
				{Sym: "s", Mods: ModShift},
				{Sym: "-"},
				{Sym: "j"},
			},
		},
		{
			input: "A-k",
			want: Sequence{
				{Sym: "a", Mods: ModShift},
				{Sym: "-"},
				{Sym: "k"},
			},
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
			if got := got.String(); got != tt.input {
				t.Fatalf("ParseBinding(%q).String() = %q, want %q", tt.input, got, tt.input)
			}
		})
	}
}

func TestParseBinding_CtrlGtRoundTrip(t *testing.T) {
	tests := []struct {
		input      string
		want       Sequence
		wantString string
	}{
		{
			input:      "ctrl+>",
			want:       Sequence{{Sym: ">", Mods: ModCtrl}},
			wantString: "<C-gt>",
		},
		{
			input:      "<C-gt>",
			want:       Sequence{{Sym: ">", Mods: ModCtrl}},
			wantString: "<C-gt>",
		},
		{
			input:      "<S-gt>",
			want:       Sequence{{Sym: ">", Mods: ModShift}},
			wantString: "<S-gt>",
		},
		{
			input:      "<A-gt>",
			want:       Sequence{{Sym: ">", Mods: ModAlt}},
			wantString: "<A-gt>",
		},
		{
			input:      ">",
			want:       Sequence{{Sym: ">"}},
			wantString: "<gt>",
		},
		{
			input:      "<gt>",
			want:       Sequence{{Sym: ">"}},
			wantString: "<gt>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			first, err := ParseBinding(tt.input)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", tt.input, err)
			}
			if !first.Equal(tt.want) {
				t.Fatalf("ParseBinding(%q) = %v, want %v", tt.input, first, tt.want)
			}
			if got := first.String(); got != tt.wantString {
				t.Fatalf("ParseBinding(%q).String() = %q, want %q", tt.input, got, tt.wantString)
			}
			second, err := ParseBinding(first.String())
			if err != nil {
				t.Fatalf("ParseBinding(%q) from canonical %q error = %v", tt.input, first.String(), err)
			}
			if !first.Equal(second) {
				t.Fatalf("round trip %q -> %q -> %v, want %v", tt.input, first.String(), second, first)
			}
		})
	}
}

func TestParseBinding_ShiftCHyphenDRoundTrip(t *testing.T) {
	input := "<S-c>-d"
	want := Sequence{
		{Sym: "c", Mods: ModShift},
		{Sym: "-"},
		{Sym: "d"},
	}
	first, err := ParseBinding(input)
	if err != nil {
		t.Fatalf("ParseBinding(%q) error = %v", input, err)
	}
	if !first.Equal(want) {
		t.Fatalf("ParseBinding(%q) = %v, want %v", input, first, want)
	}
	canonical := first.String()
	if canonical != "C-d" {
		t.Fatalf("String() = %q, want %q", canonical, "C-d")
	}
	second, err := ParseBinding(canonical)
	if err != nil {
		t.Fatalf("ParseBinding(%q) error = %v", canonical, err)
	}
	if !first.Equal(second) {
		t.Fatalf("round trip %q -> %q -> %v, want %v", input, canonical, second, first)
	}
}

func TestParseBinding_SpecialNamesInSequence(t *testing.T) {
	tests := []struct {
		input string
		want  Sequence
	}{
		{input: "j<Esc>", want: Sequence{{Sym: "j"}, {Sym: "Esc"}}},
		{input: "<Space>j", want: Sequence{{Sym: "Space"}, {Sym: "j"}}},
		{input: "<CR>j", want: Sequence{{Sym: "CR"}, {Sym: "j"}}},
		{input: "<lt>j", want: Sequence{{Sym: "<"}, {Sym: "j"}}},
		{input: "<gt>j", want: Sequence{{Sym: ">"}, {Sym: "j"}}},
		{input: "<Plus>j", want: Sequence{{Sym: "+"}, {Sym: "j"}}},
		{input: "j<Tab>", want: Sequence{{Sym: "j"}, {Sym: "Tab"}}},
		{input: "<BackSpace>j", want: Sequence{{Sym: "BackSpace"}, {Sym: "j"}}},
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

func TestParseBinding_BareNamedPrefixIsLiteral(t *testing.T) {
	// Named keys are canonical only in <...>; bare "Spacej" is Shift+s + p + a + c + e + j.
	seq := Sequence{
		{Sym: "s", Mods: ModShift},
		{Sym: "p"},
		{Sym: "a"},
		{Sym: "c"},
		{Sym: "e"},
		{Sym: "j"},
	}
	if got := seq.String(); got != "Spacej" {
		t.Fatalf("seq.String() = %q, want %q", got, "Spacej")
	}
	got, err := ParseBinding("Spacej")
	if err != nil {
		t.Fatalf("ParseBinding(%q) error = %v", "Spacej", err)
	}
	if !got.Equal(seq) {
		t.Fatalf("ParseBinding(%q) = %v, want %v", "Spacej", got, seq)
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

func TestParseBinding_CtrlHyphenRoundTrip(t *testing.T) {
	tests := []struct {
		input      string
		want       Sequence
		wantString string
	}{
		{
			input:      "ctrl+-",
			want:       Sequence{{Sym: "-", Mods: ModCtrl}},
			wantString: "<C-->",
		},
		{
			input:      "<C-->",
			want:       Sequence{{Sym: "-", Mods: ModCtrl}},
			wantString: "<C-->",
		},
		{
			input:      "<C-S-->",
			want:       Sequence{{Sym: "-", Mods: ModCtrl | ModShift}},
			wantString: "<C-S-->",
		},
		{
			input:      "<C-A-->",
			want:       Sequence{{Sym: "-", Mods: ModCtrl | ModAlt}},
			wantString: "<C-A-->",
		},
		{
			input:      "<C-S-A-->",
			want:       Sequence{{Sym: "-", Mods: ModCtrl | ModShift | ModAlt}},
			wantString: "<C-S-A-->",
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			first, err := ParseBinding(tt.input)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", tt.input, err)
			}
			if !first.Equal(tt.want) {
				t.Fatalf("ParseBinding(%q) = %v, want %v", tt.input, first, tt.want)
			}
			if got := first.String(); got != tt.wantString {
				t.Fatalf("ParseBinding(%q).String() = %q, want %q", tt.input, got, tt.wantString)
			}
			second, err := ParseBinding(first.String())
			if err != nil {
				t.Fatalf("ParseBinding(%q) from canonical %q error = %v", tt.input, first.String(), err)
			}
			if !first.Equal(second) {
				t.Fatalf("round trip %q -> %q -> %v, want %v", tt.input, first.String(), second, first)
			}
		})
	}
}

func TestParseBinding_CtrlPlusRoundTrip(t *testing.T) {
	tests := []struct {
		input      string
		want       Sequence
		wantString string
	}{
		{
			input:      "ctrl++",
			want:       Sequence{{Sym: "+", Mods: ModCtrl}},
			wantString: "<C-+>",
		},
		{
			input:      "<C-+>",
			want:       Sequence{{Sym: "+", Mods: ModCtrl}},
			wantString: "<C-+>",
		},
		{
			input:      "<C-S-+>",
			want:       Sequence{{Sym: "+", Mods: ModCtrl | ModShift}},
			wantString: "<C-S-+>",
		},
		{
			input:      "<C-A-+>",
			want:       Sequence{{Sym: "+", Mods: ModCtrl | ModAlt}},
			wantString: "<C-A-+>",
		},
		{
			input:      "<C-S-A-+>",
			want:       Sequence{{Sym: "+", Mods: ModCtrl | ModShift | ModAlt}},
			wantString: "<C-S-A-+>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			first, err := ParseBinding(tt.input)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", tt.input, err)
			}
			if !first.Equal(tt.want) {
				t.Fatalf("ParseBinding(%q) = %v, want %v", tt.input, first, tt.want)
			}
			if got := first.String(); got != tt.wantString {
				t.Fatalf("ParseBinding(%q).String() = %q, want %q", tt.input, got, tt.wantString)
			}
			second, err := ParseBinding(first.String())
			if err != nil {
				t.Fatalf("ParseBinding(%q) from canonical %q error = %v", tt.input, first.String(), err)
			}
			if !first.Equal(second) {
				t.Fatalf("round trip %q -> %q -> %v, want %v", tt.input, first.String(), second, first)
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
		"ctrl+-",
		"<C-->",
		"<C-S-->",
		"<C-A-->",
		"]]",
		"yah",
		"gO",
		"<C-d>",
		"<CR>",
		"<Esc>",
		"<Space>",
		"<Tab>",
		"<BackSpace>",
		"<Delete>",
		"<Left>",
		"<Right>",
		"<Up>",
		"<Down>",
		"<Home>",
		"<End>",
		"<PageUp>",
		"<PageDown>",
		"<C-Tab>",
		"<S-BackSpace>",
		"<A-Delete>",
		"<C-Left>",
		"<S-Home>",
		"<A-PageUp>",
		"<lt>",
		"<gt>",
		">",
		"ctrl+>",
		"<C-gt>",
		"<C-d>j",
		"<Escape>j",
		"<C-S-a>",
		"<S-c>-d",
		"C-d",
		"S-j",
		"A-k",
		"Spacej",
		"Escj",
		"Tabx",
		"enter",
		"escape",
		"return",
		"esc",
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

// TestParseBinding_DeterministicRoundTripProperty asserts ParseBinding(seq.String()) == seq
// over a fixed representative set of printable, named, and modifier sequences.
// The corpus is deterministic (no flaky randomness).
func TestParseBinding_DeterministicRoundTripProperty(t *testing.T) {
	corpus := []Sequence{
		{{Sym: "j"}},
		{{Sym: "j", Mods: ModShift}},
		{{Sym: "]"}, {Sym: "]"}},
		{{Sym: "y"}, {Sym: "a"}, {Sym: "h"}},
		{{Sym: "g"}, {Sym: "o", Mods: ModShift}},
		{{Sym: "d", Mods: ModCtrl}},
		{{Sym: "a", Mods: ModCtrl | ModShift}},
		{{Sym: "k", Mods: ModAlt}},
		{{Sym: "CR"}},
		{{Sym: "Esc"}},
		{{Sym: "Space"}},
		{{Sym: "Tab"}},
		{{Sym: "BackSpace"}},
		{{Sym: "Delete"}},
		{{Sym: "Left"}},
		{{Sym: "Right"}},
		{{Sym: "Up"}},
		{{Sym: "Down"}},
		{{Sym: "Home"}},
		{{Sym: "End"}},
		{{Sym: "PageUp"}},
		{{Sym: "PageDown"}},
		{{Sym: "Tab", Mods: ModCtrl}},
		{{Sym: "BackSpace", Mods: ModShift}},
		{{Sym: "Delete", Mods: ModAlt}},
		{{Sym: "<"}},
		{{Sym: ">"}},
		{{Sym: ">", Mods: ModCtrl}},
		{{Sym: "+"}},
		{{Sym: "+", Mods: ModCtrl}},
		{{Sym: "+", Mods: ModCtrl | ModShift}},
		{{Sym: "-", Mods: ModCtrl}},
		{{Sym: "-", Mods: ModCtrl | ModShift}},
		{{Sym: "-", Mods: ModCtrl | ModAlt}},
		{{Sym: "!"}},
		{{Sym: "@"}},
		{{Sym: "#"}},
		{{Sym: "Space"}, {Sym: "j"}},
		{{Sym: "d", Mods: ModCtrl}, {Sym: "j"}},
		{{Sym: "Esc"}, {Sym: "j"}},
		// Literal printable run that formerly collided with greedy "Space" recognition.
		{
			{Sym: "s", Mods: ModShift},
			{Sym: "p"},
			{Sym: "a"},
			{Sym: "c"},
			{Sym: "e"},
			{Sym: "j"},
		},
		{
			{Sym: "e", Mods: ModShift},
			{Sym: "s"},
			{Sym: "c"},
			{Sym: "j"},
		},
		{
			{Sym: "t", Mods: ModShift},
			{Sym: "a"},
			{Sym: "b"},
			{Sym: "x"},
		},
		// Bare modifier-like text must stay literal (no C-/S-/A- stealing).
		{
			{Sym: "c", Mods: ModShift},
			{Sym: "-"},
			{Sym: "d"},
		},
		// Representative multi-byte printable runes (accented BMP + emoji).
		{{Sym: "é"}},
		{{Sym: "é", Mods: ModCtrl}},
		{{Sym: "ñ", Mods: ModAlt}},
		{{Sym: "😀"}},
		{{Sym: "😀", Mods: ModCtrl}},
	}
	for _, seq := range corpus {
		canonical := seq.String()
		t.Run(canonical, func(t *testing.T) {
			got, err := ParseBinding(canonical)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", canonical, err)
			}
			if !got.Equal(seq) {
				t.Fatalf("ParseBinding(%q) = %v, want %v", canonical, got, seq)
			}
		})
	}
}

// TestParseBinding_LegacyAtomCollisionRoundTrip ensures rune sequences whose
// naive Key.String concatenation would equal a legacy atom are emitted with
// enough angle-literal tokens to round-trip without becoming CR/Esc.
func TestParseBinding_LegacyAtomCollisionRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		seq        Sequence
		wantString string
	}{
		{
			name: "Shift+e n t e r",
			seq: Sequence{
				{Sym: "e", Mods: ModShift},
				{Sym: "n"}, {Sym: "t"}, {Sym: "e"}, {Sym: "r"},
			},
			wantString: "<S-e>nter",
		},
		{
			name: "e n t e r",
			seq: Sequence{
				{Sym: "e"}, {Sym: "n"}, {Sym: "t"}, {Sym: "e"}, {Sym: "r"},
			},
			wantString: "<e>nter",
		},
		{
			name: "Shift+r e t u r n",
			seq: Sequence{
				{Sym: "r", Mods: ModShift},
				{Sym: "e"}, {Sym: "t"}, {Sym: "u"}, {Sym: "r"}, {Sym: "n"},
			},
			wantString: "<S-r>eturn",
		},
		{
			name: "r e t u r n",
			seq: Sequence{
				{Sym: "r"}, {Sym: "e"}, {Sym: "t"}, {Sym: "u"}, {Sym: "r"}, {Sym: "n"},
			},
			wantString: "<r>eturn",
		},
		{
			name: "Shift+e s c",
			seq: Sequence{
				{Sym: "e", Mods: ModShift},
				{Sym: "s"}, {Sym: "c"},
			},
			wantString: "<S-e>sc",
		},
		{
			name:       "e s c",
			seq:        Sequence{{Sym: "e"}, {Sym: "s"}, {Sym: "c"}},
			wantString: "<e>sc",
		},
		{
			name: "Shift+e s c a p e",
			seq: Sequence{
				{Sym: "e", Mods: ModShift},
				{Sym: "s"}, {Sym: "c"}, {Sym: "a"}, {Sym: "p"}, {Sym: "e"},
			},
			wantString: "<S-e>scape",
		},
		{
			name: "e s c a p e",
			seq: Sequence{
				{Sym: "e"}, {Sym: "s"}, {Sym: "c"}, {Sym: "a"}, {Sym: "p"}, {Sym: "e"},
			},
			wantString: "<e>scape",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.seq.String(); got != tt.wantString {
				t.Fatalf("Sequence.String() = %q, want %q", got, tt.wantString)
			}
			got, err := ParseBinding(tt.wantString)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", tt.wantString, err)
			}
			if !got.Equal(tt.seq) {
				t.Fatalf("ParseBinding(%q) = %v, want %v", tt.wantString, got, tt.seq)
			}
			if again := got.String(); again != tt.wantString {
				t.Fatalf("canonical unstable %q -> %q", tt.wantString, again)
			}
		})
	}
}

// TestParseBinding_LegacyChordCollisionRoundTrip ensures a rune sequence that
// would spell a legacy chord via a literal '+' is emitted with <Plus> so it
// cannot reparse as that chord.
func TestParseBinding_LegacyChordCollisionRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		seq        Sequence
		wantString string
	}{
		{
			name: "ctrl + d runes",
			seq: Sequence{
				{Sym: "c"}, {Sym: "t"}, {Sym: "r"}, {Sym: "l"},
				{Sym: "+"},
				{Sym: "d"},
			},
			wantString: "ctrl<Plus>d",
		},
		{
			name: "Ctrl + d mixed case start",
			seq: Sequence{
				{Sym: "c", Mods: ModShift},
				{Sym: "t"}, {Sym: "r"}, {Sym: "l"},
				{Sym: "+"},
				{Sym: "d"},
			},
			wantString: "Ctrl<Plus>d",
		},
		{
			name: "control + j runes",
			seq: Sequence{
				{Sym: "c"}, {Sym: "o"}, {Sym: "n"}, {Sym: "t"}, {Sym: "r"}, {Sym: "o"}, {Sym: "l"},
				{Sym: "+"},
				{Sym: "j"},
			},
			wantString: "control<Plus>j",
		},
		{
			name: "shift + j runes",
			seq: Sequence{
				{Sym: "s"}, {Sym: "h"}, {Sym: "i"}, {Sym: "f"}, {Sym: "t"},
				{Sym: "+"},
				{Sym: "j"},
			},
			wantString: "shift<Plus>j",
		},
		{
			name: "alt + k runes",
			seq: Sequence{
				{Sym: "a"}, {Sym: "l"}, {Sym: "t"},
				{Sym: "+"},
				{Sym: "k"},
			},
			wantString: "alt<Plus>k",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.seq.String(); got != tt.wantString {
				t.Fatalf("Sequence.String() = %q, want %q", got, tt.wantString)
			}
			got, err := ParseBinding(tt.wantString)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", tt.wantString, err)
			}
			if !got.Equal(tt.seq) {
				t.Fatalf("ParseBinding(%q) = %v, want %v", tt.wantString, got, tt.seq)
			}
		})
	}
}

// TestParseBinding_LegacyAtomsAndChordsStillAccepted preserves exact raw
// legacy config inputs as named atoms / chords.
func TestParseBinding_LegacyAtomsAndChordsStillAccepted(t *testing.T) {
	tests := []struct {
		input string
		want  Sequence
	}{
		{input: "enter", want: Sequence{{Sym: "CR"}}},
		{input: "return", want: Sequence{{Sym: "CR"}}},
		{input: "Enter", want: Sequence{{Sym: "CR"}}},
		{input: "Return", want: Sequence{{Sym: "CR"}}},
		{input: "ENTER", want: Sequence{{Sym: "CR"}}},
		{input: "escape", want: Sequence{{Sym: "Esc"}}},
		{input: "esc", want: Sequence{{Sym: "Esc"}}},
		{input: "Escape", want: Sequence{{Sym: "Esc"}}},
		{input: "Esc", want: Sequence{{Sym: "Esc"}}},
		{input: "ESC", want: Sequence{{Sym: "Esc"}}},
		{input: "ctrl+d", want: Sequence{{Sym: "d", Mods: ModCtrl}}},
		{input: "control+j", want: Sequence{{Sym: "j", Mods: ModCtrl}}},
		{input: "shift+j", want: Sequence{{Sym: "j", Mods: ModShift}}},
		{input: "alt+k", want: Sequence{{Sym: "k", Mods: ModAlt}}},
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

// TestParseBinding_OneCharAngleLiteral accepts <e>/<S-e> while rejecting <Nope>.
func TestParseBinding_OneCharAngleLiteral(t *testing.T) {
	got, err := ParseBinding("<e>")
	if err != nil {
		t.Fatalf("ParseBinding(<e>) error = %v", err)
	}
	if want := (Sequence{{Sym: "e"}}); !got.Equal(want) {
		t.Fatalf("ParseBinding(<e>) = %v, want %v", got, want)
	}
	got, err = ParseBinding("<S-e>")
	if err != nil {
		t.Fatalf("ParseBinding(<S-e>) error = %v", err)
	}
	if want := (Sequence{{Sym: "e", Mods: ModShift}}); !got.Equal(want) {
		t.Fatalf("ParseBinding(<S-e>) = %v, want %v", got, want)
	}
	if _, err := ParseBinding("<Nope>"); !errors.Is(err, ErrBadBinding) {
		t.Fatalf("ParseBinding(<Nope>) error = %v, want %v", err, ErrBadBinding)
	}
}

// TestParseBinding_CtrlEAcuteRoundTrip covers the runtime Unicode canonical
// invariant: ctrl+é / <C-é> parse to Key{Sym:"é", ModCtrl} and String
// re-parses identically.
func TestParseBinding_CtrlEAcuteRoundTrip(t *testing.T) {
	want := Sequence{{Sym: "é", Mods: ModCtrl}}
	for _, input := range []string{"ctrl+é", "<C-é>"} {
		t.Run(input, func(t *testing.T) {
			got, err := ParseBinding(input)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", input, err)
			}
			if !got.Equal(want) {
				t.Fatalf("ParseBinding(%q) = %v, want %v", input, got, want)
			}
			canonical := got.String()
			if canonical != "<C-é>" {
				t.Fatalf("String() = %q, want %q", canonical, "<C-é>")
			}
			again, err := ParseBinding(canonical)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", canonical, err)
			}
			if !again.Equal(want) {
				t.Fatalf("round trip %q -> %q -> %v, want %v", input, canonical, again, want)
			}
		})
	}
}

// TestParseBinding_ReplacementCharRoundTrip ensures valid printable U+FFFD is not
// rejected by the single-rune token path (r == utf8.RuneError is U+FFFD itself).
func TestParseBinding_ReplacementCharRoundTrip(t *testing.T) {
	const repl = "\uFFFD"
	want := Sequence{{Sym: repl, Mods: ModCtrl}}
	for _, input := range []string{"ctrl+" + repl, "<C-" + repl + ">"} {
		t.Run(input, func(t *testing.T) {
			got, err := ParseBinding(input)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", input, err)
			}
			if !got.Equal(want) {
				t.Fatalf("ParseBinding(%q) = %v, want %v", input, got, want)
			}
			canonical := got.String()
			wantCanonical := "<C-" + repl + ">"
			if canonical != wantCanonical {
				t.Fatalf("String() = %q, want %q", canonical, wantCanonical)
			}
			again, err := ParseBinding(canonical)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", canonical, err)
			}
			if !again.Equal(want) {
				t.Fatalf("round trip %q -> %q -> %v, want %v", input, canonical, again, want)
			}
		})
	}
	modifiedWant := Sequence{{Sym: repl}}
	got, err := ParseBinding("<" + repl + ">")
	if err != nil {
		t.Fatalf("ParseBinding angle literal error = %v", err)
	}
	if !got.Equal(modifiedWant) {
		t.Fatalf("ParseBinding(<U+FFFD>) = %v, want %v", got, modifiedWant)
	}
}

// TestParseBinding_LegacyCollisionModel systematically covers every exact
// legacy atom alias, modifier+plus chord spellings, case variants, and
// adjacent non-colliding sequences while retaining round-trip.
func TestParseBinding_LegacyCollisionModel(t *testing.T) {
	type atomCase struct {
		alias string
		seq   Sequence
	}
	atoms := []atomCase{
		{alias: "enter", seq: Sequence{{Sym: "e"}, {Sym: "n"}, {Sym: "t"}, {Sym: "e"}, {Sym: "r"}}},
		{alias: "Enter", seq: Sequence{{Sym: "e", Mods: ModShift}, {Sym: "n"}, {Sym: "t"}, {Sym: "e"}, {Sym: "r"}}},
		{alias: "ENTER", seq: Sequence{
			{Sym: "e", Mods: ModShift}, {Sym: "n", Mods: ModShift}, {Sym: "t", Mods: ModShift},
			{Sym: "e", Mods: ModShift}, {Sym: "r", Mods: ModShift},
		}},
		{alias: "return", seq: Sequence{{Sym: "r"}, {Sym: "e"}, {Sym: "t"}, {Sym: "u"}, {Sym: "r"}, {Sym: "n"}}},
		{alias: "Return", seq: Sequence{{Sym: "r", Mods: ModShift}, {Sym: "e"}, {Sym: "t"}, {Sym: "u"}, {Sym: "r"}, {Sym: "n"}}},
		{alias: "escape", seq: Sequence{{Sym: "e"}, {Sym: "s"}, {Sym: "c"}, {Sym: "a"}, {Sym: "p"}, {Sym: "e"}}},
		{alias: "Escape", seq: Sequence{{Sym: "e", Mods: ModShift}, {Sym: "s"}, {Sym: "c"}, {Sym: "a"}, {Sym: "p"}, {Sym: "e"}}},
		{alias: "esc", seq: Sequence{{Sym: "e"}, {Sym: "s"}, {Sym: "c"}}},
		{alias: "Esc", seq: Sequence{{Sym: "e", Mods: ModShift}, {Sym: "s"}, {Sym: "c"}}},
		{alias: "ESC", seq: Sequence{{Sym: "e", Mods: ModShift}, {Sym: "s", Mods: ModShift}, {Sym: "c", Mods: ModShift}}},
	}
	for _, tt := range atoms {
		t.Run("atom/"+tt.alias, func(t *testing.T) {
			canonical := tt.seq.String()
			if strings.EqualFold(canonical, tt.alias) {
				t.Fatalf("Sequence.String() still collides with legacy atom %q", tt.alias)
			}
			got, err := ParseBinding(canonical)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", canonical, err)
			}
			if !got.Equal(tt.seq) {
				t.Fatalf("ParseBinding(%q) = %v, want %v", canonical, got, tt.seq)
			}
			raw, err := ParseBinding(tt.alias)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", tt.alias, err)
			}
			if raw.Equal(tt.seq) {
				t.Fatalf("legacy atom %q must not equal rune sequence", tt.alias)
			}
		})
	}

	chordSpellings := []struct {
		legacy string
		seq    Sequence
	}{
		{
			legacy: "ctrl+d",
			seq: Sequence{
				{Sym: "c"}, {Sym: "t"}, {Sym: "r"}, {Sym: "l"}, {Sym: "+"}, {Sym: "d"},
			},
		},
		{
			legacy: "control+j",
			seq: Sequence{
				{Sym: "c"}, {Sym: "o"}, {Sym: "n"}, {Sym: "t"}, {Sym: "r"}, {Sym: "o"}, {Sym: "l"},
				{Sym: "+"}, {Sym: "j"},
			},
		},
		{
			legacy: "shift+a",
			seq: Sequence{
				{Sym: "s"}, {Sym: "h"}, {Sym: "i"}, {Sym: "f"}, {Sym: "t"},
				{Sym: "+"}, {Sym: "a"},
			},
		},
		{
			legacy: "alt+b",
			seq: Sequence{
				{Sym: "a"}, {Sym: "l"}, {Sym: "t"}, {Sym: "+"}, {Sym: "b"},
			},
		},
	}
	for _, tt := range chordSpellings {
		t.Run("chord/"+tt.legacy, func(t *testing.T) {
			canonical := tt.seq.String()
			if canonical == tt.legacy {
				t.Fatalf("Sequence.String() still equals legacy chord %q", tt.legacy)
			}
			got, err := ParseBinding(canonical)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", canonical, err)
			}
			if !got.Equal(tt.seq) {
				t.Fatalf("ParseBinding(%q) = %v, want %v", canonical, got, tt.seq)
			}
			chord, err := ParseBinding(tt.legacy)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", tt.legacy, err)
			}
			if chord.Equal(tt.seq) {
				t.Fatalf("legacy chord %q must not equal rune sequence", tt.legacy)
			}
		})
	}

	// Adjacent sequences that share prefixes but are not exact atoms.
	adjacent := []Sequence{
		{{Sym: "e"}, {Sym: "n"}, {Sym: "t"}, {Sym: "e"}, {Sym: "r"}, {Sym: "j"}},
		{{Sym: "e", Mods: ModShift}, {Sym: "s"}, {Sym: "c"}, {Sym: "j"}},
		{{Sym: "c"}, {Sym: "t"}, {Sym: "r"}, {Sym: "l"}, {Sym: "+"}, {Sym: "d"}, {Sym: "j"}},
	}
	for _, seq := range adjacent {
		canonical := seq.String()
		t.Run("adjacent/"+canonical, func(t *testing.T) {
			got, err := ParseBinding(canonical)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", canonical, err)
			}
			if !got.Equal(seq) {
				t.Fatalf("ParseBinding(%q) = %v, want %v", canonical, got, seq)
			}
		})
	}
}

// TestParseBinding_ExhaustiveCanonicalModelRoundTrip covers the full legal Key
// surface that Sequence.String can emit: letters × modifier combos, printable
// ASCII punctuation in runtime-canonical modifier states, named symbols ×
// modifier combos, and adjacent pairs that can spell modifier/named prefixes.
func TestParseBinding_ExhaustiveCanonicalModelRoundTrip(t *testing.T) {
	allMods := []Mods{
		0,
		ModCtrl,
		ModShift,
		ModAlt,
		ModCtrl | ModShift,
		ModCtrl | ModAlt,
		ModShift | ModAlt,
		ModCtrl | ModShift | ModAlt,
	}

	var corpus []Sequence

	for c := 'a'; c <= 'z'; c++ {
		sym := string(c)
		for _, mods := range allMods {
			corpus = append(corpus, Sequence{{Sym: sym, Mods: mods}})
		}
	}

	punctuation := []string{
		"!", `"`, "#", "$", "%", "&", "'", "(", ")", "*", "+", ",", "-", ".", "/",
		":", ";", "<", "=", ">", "?", "@", "[", `\`, "]", "^", "_", "`", "{", "|", "}", "~",
	}
	for _, sym := range punctuation {
		for _, mods := range allMods {
			// Unmodified punctuation is always legal; modified forms are emitted
			// as angle keys and must round-trip.
			corpus = append(corpus, Sequence{{Sym: sym, Mods: mods}})
		}
	}

	named := []string{
		"CR", "Esc", "Space", "Tab", "BackSpace", "Delete",
		"Left", "Right", "Up", "Down", "Home", "End", "PageUp", "PageDown",
	}
	for _, sym := range named {
		for _, mods := range allMods {
			corpus = append(corpus, Sequence{{Sym: sym, Mods: mods}})
		}
	}

	// Representative adjacent pairs that can spell modifier/named prefixes,
	// including exact legacy atom/chord collision spellings.
	adjacentPairs := []Sequence{
		{{Sym: "c", Mods: ModShift}, {Sym: "-"}, {Sym: "d"}},
		{{Sym: "s", Mods: ModShift}, {Sym: "-"}, {Sym: "j"}},
		{{Sym: "a", Mods: ModShift}, {Sym: "-"}, {Sym: "k"}},
		{{Sym: "c", Mods: ModShift}, {Sym: "-"}, {Sym: "s", Mods: ModShift}, {Sym: "-"}, {Sym: "a"}},
		{{Sym: "s", Mods: ModShift}, {Sym: "p"}, {Sym: "a"}, {Sym: "c"}, {Sym: "e"}, {Sym: "j"}},
		{{Sym: "e", Mods: ModShift}, {Sym: "s"}, {Sym: "c"}, {Sym: "j"}},
		{{Sym: "t", Mods: ModShift}, {Sym: "a"}, {Sym: "b"}, {Sym: "x"}},
		{{Sym: "r", Mods: ModShift}, {Sym: "e"}, {Sym: "t"}, {Sym: "u"}, {Sym: "r"}, {Sym: "n"}, {Sym: "x"}},
		{{Sym: "e"}, {Sym: "n"}, {Sym: "t"}, {Sym: "e"}, {Sym: "r"}},
		{{Sym: "e", Mods: ModShift}, {Sym: "n"}, {Sym: "t"}, {Sym: "e"}, {Sym: "r"}},
		{{Sym: "e"}, {Sym: "s"}, {Sym: "c"}},
		{{Sym: "e", Mods: ModShift}, {Sym: "s"}, {Sym: "c"}, {Sym: "a"}, {Sym: "p"}, {Sym: "e"}},
		{{Sym: "r"}, {Sym: "e"}, {Sym: "t"}, {Sym: "u"}, {Sym: "r"}, {Sym: "n"}},
		{{Sym: "c"}, {Sym: "t"}, {Sym: "r"}, {Sym: "l"}, {Sym: "+"}, {Sym: "d"}},
		{{Sym: "d", Mods: ModCtrl}, {Sym: "j"}},
		{{Sym: ">"}, {Sym: "j"}},
		{{Sym: "<"}, {Sym: "j"}},
		{{Sym: "g"}, {Sym: "o", Mods: ModShift}},
		{{Sym: "]"}, {Sym: "]"}},
	}
	corpus = append(corpus, adjacentPairs...)

	seen := make(map[string]struct{}, len(corpus))
	for _, seq := range corpus {
		canonical := seq.String()
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}
		t.Run(canonical, func(t *testing.T) {
			got, err := ParseBinding(canonical)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", canonical, err)
			}
			if !got.Equal(seq) {
				t.Fatalf("ParseBinding(%q) = %v, want %v", canonical, got, seq)
			}
			if again := got.String(); again != canonical {
				t.Fatalf("canonical unstable %q -> %q", canonical, again)
			}
		})
	}
}
