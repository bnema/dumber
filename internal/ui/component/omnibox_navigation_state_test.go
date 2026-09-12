package component

import (
	"context"
	"testing"

	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gtk"
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

func TestPasteSubmissionStateDefersEnterUntilTextArrives(t *testing.T) {
	state := pasteSubmissionState{}
	state.beginPaste("")

	if submit, _ := state.requestSubmit(""); submit {
		t.Fatal("Enter must not submit while the paste buffer is still empty")
	}
	if !state.textChanged() {
		t.Fatal("the completed paste must consume the deferred submission")
	}
	if state.textChanged() {
		t.Fatal("the deferred submission must be consumed exactly once")
	}
}

func TestPasteSubmissionStateCompletesWhenPasteKeepsTheSameText(t *testing.T) {
	state := pasteSubmissionState{}
	state.beginPaste("https://example.com")

	if submit, _ := state.requestSubmit("https://example.com"); submit {
		t.Fatal("Enter must be deferred while the clipboard read is still in flight")
	}
	if !state.textChanged() {
		t.Fatal("an identical replacement must consume the deferred submission")
	}
	if state.textChanged() {
		t.Fatal("the deferred submission must be consumed exactly once")
	}
}

func TestPasteSubmissionStateRecognizesPasteThatArrivesBeforeEnter(t *testing.T) {
	state := pasteSubmissionState{}
	state.beginPaste("old")

	submit, pastedTextReady := state.requestSubmit("new")
	if !submit || !pastedTextReady {
		t.Fatalf("completed paste should submit immediately, got submit=%v ready=%v", submit, pastedTextReady)
	}
}

func TestPasteSubmissionStateResetCancelsDeferredEnter(t *testing.T) {
	state := pasteSubmissionState{}
	state.beginPaste("")
	state.requestSubmit("")
	state.reset()

	if state.textChanged() {
		t.Fatal("a later edit must not consume a canceled paste submission")
	}
}

// omniboxPasteHarness drives the omnibox paste path against a real GTK entry.
type omniboxPasteHarness struct {
	omnibox   *Omnibox
	navigated []string
}

func newOmniboxPasteHarness(t *testing.T) *omniboxPasteHarness {
	t.Helper()
	if !gtk.InitCheck() {
		t.Skip("GTK native display prerequisite unavailable (gtk.InitCheck returned false)")
	}
	entry := gtk.NewSearchEntry()
	if entry == nil {
		t.Fatal("failed to create a GTK search entry")
	}
	harness := &omniboxPasteHarness{}
	harness.omnibox = &Omnibox{
		ctx:   context.Background(),
		entry: entry,
		onNavigate: func(_ context.Context, targetURL string) error {
			harness.navigated = append(harness.navigated, targetURL)
			return nil
		},
	}
	return harness
}

// pasteShortcut reproduces what the capture-phase key controller does on Ctrl+V.
func (h *omniboxPasteHarness) pasteShortcut() {
	h.omnibox.mu.Lock()
	defer h.omnibox.mu.Unlock()
	h.omnibox.pasteSubmit.beginPaste(h.omnibox.entry.GetText())
}

// showGhostSuffix displays input+suffix with the suffix selected, as the ghost
// completion does, without emitting entry notifications.
func (h *omniboxPasteHarness) showGhostSuffix(input, suffix string) {
	h.omnibox.entry.SetText(input + suffix)
	h.omnibox.mu.Lock()
	defer h.omnibox.mu.Unlock()
	h.omnibox.realInput = input
	h.omnibox.ghostSuffix = suffix
	h.omnibox.selectedIndex = -1
}

func TestPasteSubmissionStateReplaysEnterWhenPasteKeepsGhostSuffixText(t *testing.T) {
	state := pasteSubmissionState{}
	fullText := "https://example.com/guide"
	state.beginPaste(fullText)

	if submit, _ := state.requestSubmit(fullText); submit {
		t.Fatal("Enter must be deferred while the clipboard replacement is pending")
	}
	if !state.textChanged() {
		t.Fatal("an unchanged replacement matching ghost text must replay deferred Enter")
	}
	if state.textChanged() {
		t.Fatal("the replay must be consumed exactly once")
	}
}

func TestOmniboxGhostEchoAloneKeepsItsBehavior(t *testing.T) {
	harness := newOmniboxPasteHarness(t)
	harness.showGhostSuffix("https://example.com/gu", "ide")

	harness.omnibox.onEntryChanged()

	if len(harness.navigated) != 0 {
		t.Fatalf("a self-generated ghost echo must not navigate, got %v", harness.navigated)
	}
	harness.omnibox.mu.RLock()
	defer harness.omnibox.mu.RUnlock()
	if harness.omnibox.realInput != "https://example.com/gu" || harness.omnibox.ghostSuffix != "ide" {
		t.Fatalf(
			"a ghost echo must keep the ghost state, got input=%q suffix=%q",
			harness.omnibox.realInput,
			harness.omnibox.ghostSuffix,
		)
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

func TestIsDeletionKey(t *testing.T) {
	tests := []struct {
		name string
		key  uint
		want bool
	}{
		{name: "backspace is deletion", key: uint(gdk.KEY_BackSpace), want: true},
		{name: "delete is deletion", key: uint(gdk.KEY_Delete), want: true},
		{name: "typing key is not deletion", key: uint(gdk.KEY_g), want: false},
		{name: "arrow key is not deletion", key: uint(gdk.KEY_Left), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDeletionKey(tt.key); got != tt.want {
				t.Fatalf("isDeletionKey(%d) = %v, want %v", tt.key, got, tt.want)
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
		{name: "uses typed input when ghost visible", entryText: "bnema.dev/dumber", realInput: "dumb", hasGhost: true, want: "dumb"},
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

func TestResultsContainerState(t *testing.T) {
	tests := []struct {
		name            string
		rowCount        int
		wantVisible     bool
		wantExpand      bool
		wantListVisible bool
	}{
		{name: "negative rows stays hidden", rowCount: -1, wantVisible: false, wantExpand: false, wantListVisible: false},
		{name: "no rows stays hidden", rowCount: 0, wantVisible: false, wantExpand: false, wantListVisible: false},
		{name: "rows show and expand", rowCount: 3, wantVisible: true, wantExpand: true, wantListVisible: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVisible, gotExpand, gotListVisible := resultsContainerState(tt.rowCount)
			if gotVisible != tt.wantVisible || gotExpand != tt.wantExpand || gotListVisible != tt.wantListVisible {
				t.Fatalf(
					"resultsContainerState(%d) = (%v, %v, %v), want (%v, %v, %v)",
					tt.rowCount,
					gotVisible,
					gotExpand,
					gotListVisible,
					tt.wantVisible,
					tt.wantExpand,
					tt.wantListVisible,
				)
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
		{URL: "https://github.com/bnema/dumber/issues"},
	}
	favorites := []Favorite{
		{URL: "https://bnema.dev/dumber"},
		{URL: "https://bnema.dev/dumber/docs"},
	}

	tests := []struct {
		name    string
		mode    ViewMode
		index   int
		limit   int
		wantURL string
	}{
		{name: "history index", mode: ViewModeHistory, index: 1, limit: 10, wantURL: "https://github.com/bnema/dumber/pulls"},
		{name: "favorites index", mode: ViewModeFavorites, index: 0, limit: 10, wantURL: "https://bnema.dev/dumber"},
		{name: "invalid index", mode: ViewModeHistory, index: 99, limit: 10, wantURL: ""},
		{name: "history index beyond visible limit is hidden", mode: ViewModeHistory, index: 2, limit: 2, wantURL: ""},
		{name: "favorites index beyond visible limit is hidden", mode: ViewModeFavorites, index: 1, limit: 1, wantURL: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveTargetURLForSelection(tt.mode, tt.index, tt.limit, suggestions, favorites)
			if got != tt.wantURL {
				t.Fatalf("resolveTargetURLForSelection(%s, %d) = %q, want %q", tt.mode, tt.index, got, tt.wantURL)
			}
		})
	}
}

func TestSelectedTargetURL(t *testing.T) {
	suggestions := []Suggestion{
		{URL: "https://github.com/bnema/dumber"},
		{URL: "https://github.com/bnema/dumber/pulls"},
	}
	favorites := []Favorite{
		{URL: "https://bnema.dev/dumber"},
		{URL: "https://bnema.dev/dumber/docs"},
	}

	tests := []struct {
		name     string
		mode     ViewMode
		index    int
		limit    int
		wantURL  string
		wantBool bool
	}{
		{name: "negative index is not explicit selection", mode: ViewModeHistory, index: -1, limit: 10, wantURL: "", wantBool: false},
		{name: "history selection is explicit", mode: ViewModeHistory, index: 0, limit: 10, wantURL: "https://github.com/bnema/dumber", wantBool: true},
		{name: "favorites selection is explicit", mode: ViewModeFavorites, index: 0, limit: 10, wantURL: "https://bnema.dev/dumber", wantBool: true},
		{name: "hidden history selection is ignored", mode: ViewModeHistory, index: 1, limit: 1, wantURL: "", wantBool: true},
		{name: "hidden favorites selection is ignored", mode: ViewModeFavorites, index: 1, limit: 1, wantURL: "", wantBool: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotBool := selectedTargetURL(tt.mode, tt.index, tt.limit, suggestions, favorites)
			if gotURL != tt.wantURL || gotBool != tt.wantBool {
				t.Fatalf("selectedTargetURL(%s, %d) = (%q, %v), want (%q, %v)", tt.mode, tt.index, gotURL, gotBool, tt.wantURL, tt.wantBool)
			}
		})
	}
}

func TestBangSuggestionTextAt(t *testing.T) {
	bangSuggestions := []BangSuggestion{{Key: "g"}, {Key: "gh"}}

	if got, ok := bangSuggestionTextAt(1, bangSuggestions); !ok || got != "!gh " {
		t.Fatalf("bangSuggestionTextAt(1) = (%q, %v), want (%q, %v)", got, ok, "!gh ", true)
	}
	if got, ok := bangSuggestionTextAt(99, bangSuggestions); ok || got != "" {
		t.Fatalf("bangSuggestionTextAt(out of range) = (%q, %v), want (%q, %v)", got, ok, "", false)
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
				10,
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

func TestVisibleGhostSuggestionRespectsVisibleLimit(t *testing.T) {
	suggestions := []Suggestion{
		{URL: "https://github.com/bnema/dumber"},
		{URL: "https://gitlab.com/team/project"},
	}

	gotFull, gotSuffix, gotOK := visibleGhostSuggestion("gitl", "", false, ViewModeHistory, 1, suggestions, nil)
	if gotFull != "" || gotSuffix != "" || gotOK {
		t.Fatalf("visibleGhostSuggestion should ignore hidden suggestions, got (%q, %q, %v)", gotFull, gotSuffix, gotOK)
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
