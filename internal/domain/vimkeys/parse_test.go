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

	// Representative adjacent pairs that can spell modifier/named prefixes.
	// Trailing letters avoid exact legacy atom collisions (enter/escape/…).
	adjacentPairs := []Sequence{
		{{Sym: "c", Mods: ModShift}, {Sym: "-"}, {Sym: "d"}},
		{{Sym: "s", Mods: ModShift}, {Sym: "-"}, {Sym: "j"}},
		{{Sym: "a", Mods: ModShift}, {Sym: "-"}, {Sym: "k"}},
		{{Sym: "c", Mods: ModShift}, {Sym: "-"}, {Sym: "s", Mods: ModShift}, {Sym: "-"}, {Sym: "a"}},
		{{Sym: "s", Mods: ModShift}, {Sym: "p"}, {Sym: "a"}, {Sym: "c"}, {Sym: "e"}, {Sym: "j"}},
		{{Sym: "e", Mods: ModShift}, {Sym: "s"}, {Sym: "c"}, {Sym: "j"}},
		{{Sym: "t", Mods: ModShift}, {Sym: "a"}, {Sym: "b"}, {Sym: "x"}},
		{{Sym: "r", Mods: ModShift}, {Sym: "e"}, {Sym: "t"}, {Sym: "u"}, {Sym: "r"}, {Sym: "n"}, {Sym: "x"}},
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
