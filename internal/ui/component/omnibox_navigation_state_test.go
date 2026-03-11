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
		{name: "localhost", input: "localhost", want: true},
		{name: "ipv4", input: "127.0.0.1", want: true},
		{name: "ipv4 with port", input: "192.168.1.1:8080", want: true},
		{name: "ipv6 bracketed", input: "[::1]", want: true},
		{name: "http ipv6 bracketed", input: "http://[::1]", want: true},
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
			if got := nextGhostSuppressionState(tt.key); got != tt.want {
				t.Fatalf("nextGhostSuppressionState(%v, %d) = %v, want %v", tt.current, tt.key, got, tt.want)
			}
		})
	}
}

func TestEffectiveSearchQuery(t *testing.T) {
	tests := []struct {
		name      string
		entryText string
		realInput string
		hasGhost  bool
		want      string
	}{
		{name: "uses typed input when ghost visible", entryText: "dumber.bnema.dev", realInput: "dumb", hasGhost: true, want: "dumb"},
		{name: "falls back to entry when no ghost", entryText: "dumb", realInput: "dumb", hasGhost: false, want: "dumb"},
		{name: "uses entry when real input unavailable", entryText: "dumb", realInput: "", hasGhost: true, want: "dumb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveSearchQuery(tt.entryText, tt.realInput, tt.hasGhost); got != tt.want {
				t.Fatalf("effectiveSearchQuery(%q, %q, %v) = %q, want %q", tt.entryText, tt.realInput, tt.hasGhost, got, tt.want)
			}
		})
	}
}

func TestUpdateGhostFromSelectionUsesRealInputWhenGhostVisible(t *testing.T) {
	entryText := "github.com/bnema/dumber"
	realInput := "upl"

	got := effectiveSearchQuery(entryText, realInput, true)
	if got != realInput {
		t.Fatalf("effectiveSearchQuery should prefer real input when ghost is visible, got %q want %q", got, realInput)
	}
}

func TestResolveTargetURLForSelection(t *testing.T) {
	suggestions := []Suggestion{
		{URL: "https://github.com/bnema/dumber"},
		{URL: "https://github.com/bnema/dumber/pulls"},
	}
	favorites := []Favorite{
		{URL: "https://dumber.bnema.dev"},
	}

	tests := []struct {
		name    string
		mode    ViewMode
		index   int
		wantURL string
	}{
		{name: "history index", mode: ViewModeHistory, index: 1, wantURL: "https://github.com/bnema/dumber/pulls"},
		{name: "favorites index", mode: ViewModeFavorites, index: 0, wantURL: "https://dumber.bnema.dev"},
		{name: "invalid index", mode: ViewModeHistory, index: 99, wantURL: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveTargetURLForSelection(tt.mode, tt.index, suggestions, favorites)
			if got != tt.wantURL {
				t.Fatalf("resolveTargetURLForSelection(%s, %d) = %q, want %q", tt.mode, tt.index, got, tt.wantURL)
			}
		})
	}
}

func TestSelectedTargetURL(t *testing.T) {
	suggestions := []Suggestion{
		{URL: "https://github.com/bnema/dumber"},
	}
	favorites := []Favorite{
		{URL: "https://dumber.bnema.dev"},
	}

	tests := []struct {
		name     string
		mode     ViewMode
		index    int
		wantURL  string
		wantBool bool
	}{
		{name: "negative index is not explicit selection", mode: ViewModeHistory, index: -1, wantURL: "", wantBool: false},
		{name: "history selection is explicit", mode: ViewModeHistory, index: 0, wantURL: "https://github.com/bnema/dumber", wantBool: true},
		{name: "favorites selection is explicit", mode: ViewModeFavorites, index: 0, wantURL: "https://dumber.bnema.dev", wantBool: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotBool := selectedTargetURL(tt.mode, tt.index, suggestions, favorites)
			if gotURL != tt.wantURL || gotBool != tt.wantBool {
				t.Fatalf("selectedTargetURL(%s, %d) = (%q, %v), want (%q, %v)", tt.mode, tt.index, gotURL, gotBool, tt.wantURL, tt.wantBool)
			}
		})
	}
}

func TestVisibleGhostSuggestion(t *testing.T) {
	suggestions := []Suggestion{
		{URL: "https://github.com/bnema/dumber"},
		{URL: "https://gitlab.com/team/project"},
	}

	tests := []struct {
		name                 string
		input                string
		selectedURL          string
		hasExplicitSelection bool
		wantFull             string
		wantSuffix           string
		wantOK               bool
	}{
		{
			name:       "top visible candidate drives ghost without selection",
			input:      "git",
			wantFull:   "github.com",
			wantSuffix: "hub.com",
			wantOK:     true,
		},
		{
			name:                 "explicit selection wins over top candidate",
			input:                "git",
			selectedURL:          "https://gitlab.com/team/project",
			hasExplicitSelection: true,
			wantFull:             "gitlab.com/team/project",
			wantSuffix:           "lab.com/team/project",
			wantOK:               true,
		},
		{
			name:       "no visible prefix match means no ghost",
			input:      "xyz",
			wantFull:   "",
			wantSuffix: "",
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFull, gotSuffix, gotOK := visibleGhostSuggestion(
				tt.input,
				tt.selectedURL,
				tt.hasExplicitSelection,
				ViewModeHistory,
				suggestions,
				nil,
			)
			if gotFull != tt.wantFull || gotSuffix != tt.wantSuffix || gotOK != tt.wantOK {
				t.Fatalf(
					"visibleGhostSuggestion(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.input,
					gotFull,
					gotSuffix,
					gotOK,
					tt.wantFull,
					tt.wantSuffix,
					tt.wantOK,
				)
			}
		})
	}
}

func TestShouldPromoteHoverSelection(t *testing.T) {
	tests := []struct {
		name         string
		realInput    string
		hasGhostText bool
		hasNavigated bool
		want         bool
	}{
		{
			name: "initial open allows hover selection",
			want: true,
		},
		{
			name:      "typed input keeps hover from stealing authority",
			realInput: "git",
			want:      false,
		},
		{
			name:         "visible ghost keeps hover from stealing authority",
			hasGhostText: true,
			want:         false,
		},
		{
			name:         "keyboard navigation keeps hover from stealing authority",
			hasNavigated: true,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldPromoteHoverSelection(tt.realInput, tt.hasGhostText, tt.hasNavigated)
			if got != tt.want {
				t.Fatalf(
					"shouldPromoteHoverSelection(%q, %v, %v) = %v, want %v",
					tt.realInput,
					tt.hasGhostText,
					tt.hasNavigated,
					got,
					tt.want,
				)
			}
		})
	}
}
