package component

import (
	"testing"

	"github.com/jwijenbergh/puregotk/v4/gdk"
)

func TestShouldPreferTypedURLNavigation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "domain", input: "x.com", want: true},
		{name: "domain with path", input: "github.com/bnema", want: true},
		{name: "http url", input: "http://example.com", want: true},
		{name: "search query", input: "hello world", want: false},
		{name: "empty", input: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldPreferTypedURLNavigation(tt.input); got != tt.want {
				t.Fatalf("shouldPreferTypedURLNavigation(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestResetSearchSessionState(t *testing.T) {
	o := &Omnibox{}
	o.lastQuery = "github"

	o.resetSearchSessionState()

	if o.lastQuery != "" {
		t.Fatalf("lastQuery should be reset, got %q", o.lastQuery)
	}
}

func TestShouldUpdateGhostImmediately(t *testing.T) {
	tests := []struct {
		name          string
		previousInput string
		entryText     string
		want          bool
	}{
		{name: "typing forward", previousInput: "goo", entryText: "goog", want: true},
		{name: "same length replace", previousInput: "goo", entryText: "gaa", want: true},
		{name: "backspace", previousInput: "goog", entryText: "goo", want: false},
		{name: "clear input", previousInput: "goo", entryText: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldUpdateGhostImmediately(tt.previousInput, tt.entryText); got != tt.want {
				t.Fatalf(
					"shouldUpdateGhostImmediately(%q, %q) = %v, want %v",
					tt.previousInput,
					tt.entryText,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestNextGhostSuppressionState(t *testing.T) {
	tests := []struct {
		name    string
		current bool
		key     uint
		want    bool
	}{
		{name: "backspace enables suppression", current: false, key: uint(gdk.KEY_BackSpace), want: true},
		{name: "delete enables suppression", current: false, key: uint(gdk.KEY_Delete), want: true},
		{name: "typing key disables suppression", current: true, key: uint(gdk.KEY_g), want: false},
		{name: "arrow key disables suppression", current: true, key: uint(gdk.KEY_Left), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextGhostSuppressionState(tt.current, tt.key); got != tt.want {
				t.Fatalf("nextGhostSuppressionState(%v, %d) = %v, want %v", tt.current, tt.key, got, tt.want)
			}
		})
	}
}
