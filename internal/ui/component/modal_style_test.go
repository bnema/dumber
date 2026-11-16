package component

import "testing"

func TestModalStyleClass(t *testing.T) {
	for input, want := range map[string]string{
		"":            "omnibox-style-command",
		"  ":          "omnibox-style-command",
		" command ":   "omnibox-style-command",
		"multiplexer": "omnibox-style-multiplexer",
		"minimal":     "omnibox-style-minimal",
	} {
		if got := modalStyleClass(input); got != want {
			t.Errorf("modalStyleClass(%q) = %q, want %q", input, got, want)
		}
	}
}
