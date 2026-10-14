package keychord

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalize_SameBindingSameString(t *testing.T) {
	groups := [][]string{
		{"right", "arrowright", "Right", "ArrowRight"},
		{"left", "arrowleft"},
		{"escape", "esc", "Escape"},
		{"enter", "return", "Return"},
		{"delete", "del"},
		{"pageup", "page_up"},
		{"ctrl+t", "control+t", "CTRL+t", " Ctrl + t"},
		{"ctrl+shift+t", "Ctrl+Shift+T", "shift+ctrl+t", "control+shift+T", "Control+T"},
		{"M", "shift+m", "shift+M"},
		{"ctrl+plus", "ctrl++", "ctrl+Plus"},
		{"plus", "+"},
		{"minus", "-"},
		{"alt+[", "alt+bracketleft"},
		{"alt+shift+tab", "shift+alt+tab"},
		{" ctrl + t ", "ctrl+t"},
	}
	for _, group := range groups {
		want, ok := Canonicalize(group[0])
		require.True(t, ok, group[0])
		for _, spelling := range group[1:] {
			got, ok := Canonicalize(spelling)
			require.True(t, ok, spelling)
			assert.Equal(t, want, got, "%q should equal %q", spelling, group[0])
		}
	}
}

func TestCanonicalize_DifferentBindingsDiffer(t *testing.T) {
	pairs := [][2]string{
		{"m", "M"},
		{"ctrl+t", "alt+t"},
		{"ctrl+t", "t"},
		{"left", "right"},
		{"ctrl+shift+t", "ctrl+t"},
		{"1", "f1"},
	}
	for _, pair := range pairs {
		a, ok := Canonicalize(pair[0])
		require.True(t, ok, pair[0])
		b, ok := Canonicalize(pair[1])
		require.True(t, ok, pair[1])
		assert.NotEqual(t, a, b, "%q vs %q", pair[0], pair[1])
	}
}

func TestCanonicalize_CanonicalForm(t *testing.T) {
	tests := map[string]string{
		"arrowright":          "right",
		"Control+Shift+T":     "ctrl+shift+t",
		"shift+alt+ctrl+Esc":  "ctrl+alt+shift+escape",
		"M":                   "shift+m",
		"ctrl++":              "ctrl+plus",
		"%":                   "%",
		"shift+%":             "shift+%",
		"f5":                  "f5",
		"0":                   "0",
		"alt+bracketleft":     "alt+bracketleft",
		"ctrl+arrowleft":      "ctrl+left",
		"  Alt + Page_Down  ": "alt+pagedown",
	}
	for in, want := range tests {
		got, ok := Canonicalize(in)
		require.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
}

func TestParse_Invalid(t *testing.T) {
	for _, in := range []string{"", "   ", "ctrl+", "ctrl", "unknownkey", "ctrl+unknownkey", "a+b", "é"} {
		_, ok := Parse(in)
		assert.False(t, ok, "%q", in)
	}
}

func TestKeyNames_AreCanonicalAndStable(t *testing.T) {
	names := KeyNames()
	require.Contains(t, names, "left")
	require.Contains(t, names, "plus")
	require.NotContains(t, names, "arrowleft")
	for _, name := range names {
		got, ok := canonicalKey(name)
		require.True(t, ok, name)
		assert.Equal(t, name, got)
	}
}
