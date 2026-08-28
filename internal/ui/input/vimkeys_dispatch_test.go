package input

import (
	"context"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/vimkeys"
	"github.com/bnema/puregotk/v4/gdk"
)

type stubSequenceTimer struct {
	onStop func()
}

func (s stubSequenceTimer) Stop() bool {
	if s.onStop != nil {
		s.onStop()
	}
	return true
}

func vimModeSequenceWorkspace(actions map[string]entity.ActionBinding) *entity.WorkspaceConfig {
	ws := newTestWorkspace()
	ws.VimMode = entity.VimModeConfig{
		ActivationShortcut:          "ctrl+y",
		TimeoutMilliseconds:         0,
		SequenceTimeoutMilliseconds: 500,
		Actions:                     actions,
	}
	return ws
}

func enterVimMode(t *testing.T, h *KeyboardHandler) {
	t.Helper()
	if !h.handleKeyPress(uint('y'), 0, gdk.ControlMaskValue) {
		t.Fatal("ctrl+y should enter vim mode")
	}
	if h.Mode() != ModeVim {
		t.Fatalf("mode = %v, want ModeVim", h.Mode())
	}
}

func capturePending(h *KeyboardHandler) *[]string {
	pending := &[]string{}
	h.SetOnPendingSequenceChange(func(p string) {
		*pending = append(*pending, p)
	})
	return pending
}

func captureSequenceActions(h *KeyboardHandler) *[]struct {
	action string
	count  int
} {
	actions := &[]struct {
		action string
		count  int
	}{}
	h.SetOnSequenceAction(func(action string, count int) {
		*actions = append(*actions, struct {
			action string
			count  int
		}{action: action, count: count})
	})
	return actions
}

func TestNewVimModeSequenceState_NilTrie(t *testing.T) {
	state := newVimModeSequenceState(nil, 0)
	if state.matcher == nil {
		t.Fatal("matcher is nil")
	}
	if got := state.matcher.Pending(); got != "" {
		t.Fatalf("Pending() = %q, want empty", got)
	}
	if state.matcher.Ambiguous() {
		t.Fatal("Ambiguous() = true, want false")
	}
	result := state.matcher.Feed(vimkeys.Key{Sym: "x"})
	if result.Kind != vimkeys.ResultInvalid {
		t.Fatalf("Feed kind = %v, want %v", result.Kind, vimkeys.ResultInvalid)
	}
}

func TestTrieOwnsBinding(t *testing.T) {
	cases := map[string]bool{
		"]]":      true,
		"gi":      true,
		"yah":     true,
		"gO":      true,
		"<C-d>":   true,
		"j":       false,
		"shift+j": false,
		"}":       false,
		"enter":   false,
		"escape":  false,
		"<Nope>":  false,
	}

	for raw, wantOwned := range cases {
		t.Run(raw, func(t *testing.T) {
			seq, owned := trieOwnsBinding(raw)
			if owned != wantOwned {
				t.Fatalf("trieOwnsBinding(%q) owned = %v, want %v (seq=%v)", raw, owned, wantOwned, seq)
			}
			if !wantOwned {
				return
			}
			if len(seq) == 0 {
				t.Fatalf("trieOwnsBinding(%q) returned empty sequence", raw)
			}
		})
	}
}

func TestTrieOwnsBinding_CrossContractAtoms(t *testing.T) {
	tests := []struct {
		raw  string
		want vimkeys.Sequence
	}{
		{raw: "}", want: vimkeys.Sequence{{Sym: "}"}}},
		{raw: "<C-d>", want: vimkeys.Sequence{{Sym: "d", Mods: vimkeys.ModCtrl}}},
		{raw: "<CR>", want: vimkeys.Sequence{{Sym: "CR"}}},
		{raw: "J", want: vimkeys.Sequence{{Sym: "j", Mods: vimkeys.ModShift}}},
		{raw: "enter", want: vimkeys.Sequence{{Sym: "CR"}}},
		{raw: "escape", want: vimkeys.Sequence{{Sym: "Esc"}}},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := vimkeys.ParseBinding(tt.raw)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", tt.raw, err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("ParseBinding(%q) = %v, want %v", tt.raw, got, tt.want)
			}
			_, owned := trieOwnsBinding(tt.raw)
			switch tt.raw {
			case "<C-d>", "<CR>":
				if !owned {
					t.Fatalf("trieOwnsBinding(%q) = false, want true", tt.raw)
				}
			default:
				if owned {
					t.Fatalf("trieOwnsBinding(%q) = true, want false", tt.raw)
				}
			}
		})
	}
}

func TestBuildVimModeTrie(t *testing.T) {
	cfg := &entity.VimModeConfig{
		Actions: map[string]entity.ActionBinding{
			"vim-scroll-down": {Keys: []string{"j"}},
			"heading-next":    {Keys: []string{"]]"}},
			"focus-input":     {Keys: []string{"gi"}},
			"yank-section":    {Keys: []string{"yah"}},
			"outline":         {Keys: []string{"gO"}},
			"half-page-down":  {Keys: []string{"<C-d>"}},
			"confirm":         {Keys: []string{"enter"}},
			"cancel":          {Keys: []string{"escape"}},
		},
	}

	trie, owned := buildVimModeTrie(cfg)
	if trie == nil {
		t.Fatal("buildVimModeTrie returned nil trie")
	}

	wantOwned := map[string]bool{
		"]]":    true,
		"gi":    true,
		"yah":   true,
		"gO":    true,
		"<C-d>": true,
	}
	for key, want := range wantOwned {
		if owned[key] != want {
			t.Fatalf("owned[%q] = %v, want %v", key, owned[key], want)
		}
	}
	for _, legacy := range []string{"j", "enter", "escape"} {
		if owned[legacy] {
			t.Fatalf("legacy binding %q should not be owned", legacy)
		}
	}

	seq, err := vimkeys.ParseBinding("]]")
	if err != nil {
		t.Fatalf("ParseBinding(]]) error = %v", err)
	}
	node, ok := trie.Walk(seq)
	if !ok || !node.Exact || node.Action != "heading-next" {
		t.Fatalf("Walk(]]) = (%#v, %v), want exact heading-next", node, ok)
	}

	seq, err = vimkeys.ParseBinding("gi")
	if err != nil {
		t.Fatalf("ParseBinding(gi) error = %v", err)
	}
	node, ok = trie.Walk(seq)
	if !ok || !node.Exact || node.Action != "focus-input" {
		t.Fatalf("Walk(gi) = (%#v, %v), want exact focus-input", node, ok)
	}
}

func TestBuildVimModeTrie_ConflictKeepsFirstSortedWinner(t *testing.T) {
	// Same sequence under two raw aliases; first sorted action wins Insert.
	// Losing raw key must not be marked owned (insert failed).
	cfg := &entity.VimModeConfig{
		Actions: map[string]entity.ActionBinding{
			"zzz-later":       {Keys: []string{"<c-d>"}},
			"aaa-first":       {Keys: []string{"<C-d>"}},
			"vim-scroll-down": {Keys: []string{"j"}},
		},
	}

	trie, owned := buildVimModeTrie(cfg)
	if !owned["<C-d>"] {
		t.Fatal("winning raw key <C-d> should be owned after successful Insert")
	}
	if owned["<c-d>"] {
		t.Fatal("conflicting raw key <c-d> must not be marked owned when Insert fails")
	}

	seq, err := vimkeys.ParseBinding("<C-d>")
	if err != nil {
		t.Fatalf("ParseBinding(<C-d>) error = %v", err)
	}
	node, ok := trie.Walk(seq)
	if !ok || !node.Exact || node.Action != "aaa-first" {
		t.Fatalf("Walk(<C-d>) = (%#v, %v), want exact aaa-first", node, ok)
	}
}

func vimModeSequencesOwnershipFixture(t *testing.T) *ShortcutSet {
	t.Helper()

	workspace := &entity.WorkspaceConfig{
		VimMode: entity.VimModeConfig{
			ActivationShortcut: "ctrl+y",
			Actions: map[string]entity.ActionBinding{
				"vim-scroll-down":      {Keys: []string{"j"}},
				"vim-scroll-down-fast": {Keys: []string{"shift+j"}},
				"confirm":              {Keys: []string{"enter"}},
				"cancel":               {Keys: []string{"escape"}},
				"heading-next":         {Keys: []string{"]]"}},
				"yank-section":         {Keys: []string{"yah"}},
				"outline":              {Keys: []string{"gO"}},
				"half-page-down":       {Keys: []string{"<C-d>"}},
			},
		},
	}

	set := NewShortcutSet(context.Background(), workspace, nil)
	if set.VimModeSequences() == nil {
		t.Fatal("VimModeSequences() is nil")
	}
	return set
}

func assertLegacyVimModeBindings(t *testing.T, set *ShortcutSet) {
	t.Helper()

	jBinding, jOK := ParseKeyString("j")
	if !jOK {
		t.Fatal("ParseKeyString(j) failed")
	}
	if action, found := set.VimMode[jBinding]; !found || action != ActionVimScrollDown {
		t.Fatalf("legacy j missing from VimMode: found=%v action=%q", found, action)
	}

	enterBinding := KeyBinding{Keyval: uint(gdk.KEY_Return), Modifiers: ModNone}
	if action, found := set.VimMode[enterBinding]; !found || action != ActionVimConfirm {
		t.Fatalf("legacy Vim confirm missing from VimMode: found=%v action=%q", found, action)
	}

	escapeBinding := KeyBinding{Keyval: uint(gdk.KEY_Escape), Modifiers: ModNone}
	if action, found := set.VimMode[escapeBinding]; !found || action != ActionExitMode {
		t.Fatalf("legacy escape missing from VimMode: found=%v action=%q", found, action)
	}
}

func assertOwnedBindingsAbsentFromVimMode(t *testing.T, set *ShortcutSet) {
	t.Helper()

	for _, key := range []string{"]]", "yah", "gO", "<C-d>"} {
		if binding, parsed := ParseKeyString(key); parsed {
			if action, found := set.VimMode[binding]; found {
				t.Fatalf("owned binding %q leaked into VimMode as %q", key, action)
			}
		}
	}
}

func assertVimModeTrieYankSection(t *testing.T, set *ShortcutSet) {
	t.Helper()

	seq, err := vimkeys.ParseBinding("yah")
	if err != nil {
		t.Fatalf("ParseBinding(yah) error = %v", err)
	}
	node, walked := set.VimModeSequences().Walk(seq)
	if !walked || !node.Exact || node.Action != "yank-section" {
		t.Fatalf("VimModeSequences Walk(yah) = (%#v, %v), want exact yank-section", node, walked)
	}
}

func TestShortcutSet_VimModeSequencesOwnership(t *testing.T) {
	set := vimModeSequencesOwnershipFixture(t)

	t.Run("legacy bindings", func(t *testing.T) {
		assertLegacyVimModeBindings(t, set)
	})
	t.Run("owned bindings absent from legacy table", func(t *testing.T) {
		assertOwnedBindingsAbsentFromVimMode(t, set)
	})
	t.Run("trie resolves owned sequence", func(t *testing.T) {
		assertVimModeTrieYankSection(t, set)
	})
}

func TestFeedVimModeSequence_CompleteDoubleBracket(t *testing.T) {
	ctx := context.Background()
	ws := vimModeSequenceWorkspace(map[string]entity.ActionBinding{
		"heading-next":    {Keys: []string{"]]"}},
		"vim-scroll-down": {Keys: []string{"j"}},
		"cancel":          {Keys: []string{"escape"}},
	})
	h := NewKeyboardHandler(ctx, ws, newTestSession())
	pending := capturePending(h)
	actions := captureSequenceActions(h)
	enterVimMode(t, h)

	if !h.handleKeyPress(uint(']'), 0, 0) {
		t.Fatal("] prefix should be consumed")
	}
	if h.PendingSequence() != "]" {
		t.Fatalf("PendingSequence() = %q, want %q", h.PendingSequence(), "]")
	}
	if len(*pending) != 1 || (*pending)[0] != "]" {
		t.Fatalf("pending notifications = %#v, want [\"]\"]", *pending)
	}

	if !h.handleKeyPress(uint(']'), 0, 0) {
		t.Fatal("]] should be consumed")
	}
	if h.PendingSequence() != "" {
		t.Fatalf("PendingSequence after complete = %q, want empty", h.PendingSequence())
	}
	if len(*actions) != 1 || (*actions)[0].action != "heading-next" || (*actions)[0].count != 0 {
		t.Fatalf("actions = %#v, want heading-next count 0", *actions)
	}
	if len(*pending) < 2 || (*pending)[len(*pending)-1] != "" {
		t.Fatalf("pending after complete = %#v, want trailing clear", *pending)
	}
}

func TestFeedVimModeSequence_ConfiguredConfirmActivatesAndExits(t *testing.T) {
	ctx := context.Background()
	ws := vimModeSequenceWorkspace(map[string]entity.ActionBinding{
		"confirm": {Keys: []string{"zz"}},
	})
	h := NewKeyboardHandler(ctx, ws, newTestSession())
	actions := captureSequenceActions(h)
	enterVimMode(t, h)

	if !h.handleKeyPress(uint('z'), 0, 0) {
		t.Fatal("configured confirm prefix should be consumed")
	}
	if !h.handleKeyPress(uint('z'), 0, 0) {
		t.Fatal("configured confirm completion should be consumed")
	}
	if len(*actions) != 1 || (*actions)[0].action != "confirm" || (*actions)[0].count != 0 {
		t.Fatalf("actions = %#v, want confirm count 0", *actions)
	}
	if h.Mode() != ModeNormal {
		t.Fatalf("mode after configured confirm sequence = %v, want ModeNormal", h.Mode())
	}
}

func TestFeedVimModeSequence_PendingYankNotifications(t *testing.T) {
	ctx := context.Background()
	ws := vimModeSequenceWorkspace(map[string]entity.ActionBinding{
		"yank-section":    {Keys: []string{"yah"}},
		"vim-scroll-down": {Keys: []string{"j"}},
	})
	h := NewKeyboardHandler(ctx, ws, newTestSession())
	pending := capturePending(h)
	actions := captureSequenceActions(h)
	enterVimMode(t, h)

	for _, step := range []struct {
		key  uint
		want string
	}{
		{uint('y'), "y"},
		{uint('a'), "ya"},
		{uint('h'), ""},
	} {
		if !h.handleKeyPress(step.key, 0, 0) {
			t.Fatalf("key %c should be consumed", step.key)
		}
		if got := h.PendingSequence(); got != step.want {
			t.Fatalf("after %c PendingSequence() = %q, want %q", step.key, got, step.want)
		}
	}
	if len(*actions) != 1 || (*actions)[0].action != "yank-section" {
		t.Fatalf("actions = %#v, want yank-section", *actions)
	}
	if len(*pending) < 3 || (*pending)[0] != "y" || (*pending)[1] != "ya" {
		t.Fatalf("pending = %#v, want y then ya", *pending)
	}
}

func TestFeedVimModeSequence_UnknownFallsThrough(t *testing.T) {
	ctx := context.Background()
	ws := vimModeSequenceWorkspace(map[string]entity.ActionBinding{
		"heading-next":    {Keys: []string{"]]"}},
		"vim-scroll-down": {Keys: []string{"j"}},
	})
	h := NewKeyboardHandler(ctx, ws, newTestSession())
	var scrollCalls int
	h.SetOnAction(func(_ context.Context, action Action) error {
		if action == ActionVimScrollDown {
			scrollCalls++
		}
		return nil
	})
	enterVimMode(t, h)

	// Unknown sequence key falls through; legacy j still works.
	if !h.handleKeyPress(uint('j'), 0, 0) {
		t.Fatal("legacy j should be handled")
	}
	if scrollCalls != 1 {
		t.Fatalf("scrollCalls = %d, want 1", scrollCalls)
	}
	if h.PendingSequence() != "" {
		t.Fatalf("PendingSequence = %q, want empty", h.PendingSequence())
	}
}

func TestFeedVimModeSequence_IgnoresOutsideVimMode(t *testing.T) {
	ctx := context.Background()
	ws := vimModeSequenceWorkspace(map[string]entity.ActionBinding{
		"heading-next": {Keys: []string{"]]"}},
	})
	h := NewKeyboardHandler(ctx, ws, newTestSession())
	pending := capturePending(h)

	if h.feedVimModeSequence(uint(']'), 0) {
		t.Fatal("feed outside vim mode should return false")
	}
	if len(*pending) != 0 {
		t.Fatalf("pending = %#v, want none", *pending)
	}
}

func TestFeedVimModeSequence_ModeExitResetsSilently(t *testing.T) {
	ctx := context.Background()
	ws := vimModeSequenceWorkspace(map[string]entity.ActionBinding{
		"yank-section": {Keys: []string{"yah"}},
		"cancel":       {Keys: []string{"escape"}},
	})
	h := NewKeyboardHandler(ctx, ws, newTestSession())
	pending := capturePending(h)
	enterVimMode(t, h)

	if !h.handleKeyPress(uint('y'), 0, 0) {
		t.Fatal("y should be consumed")
	}
	before := len(*pending)

	if !h.handleKeyPress(uint(gdk.KEY_Escape), 0, 0) {
		t.Fatal("escape should exit")
	}
	if h.Mode() != ModeNormal {
		t.Fatalf("mode = %v, want ModeNormal", h.Mode())
	}
	if h.PendingSequence() != "" {
		t.Fatalf("PendingSequence after exit = %q, want empty", h.PendingSequence())
	}
	if len(*pending) != before {
		t.Fatalf("silent exit must not notify pending clear: before=%d after=%d (%#v)", before, len(*pending), *pending)
	}
}

func TestFeedVimModeSequence_CountPrefix(t *testing.T) {
	ctx := context.Background()
	ws := vimModeSequenceWorkspace(map[string]entity.ActionBinding{
		"heading-next": {Keys: []string{"]]"}},
	})
	h := NewKeyboardHandler(ctx, ws, newTestSession())
	actions := captureSequenceActions(h)
	enterVimMode(t, h)

	if !h.handleKeyPress(uint('2'), 0, 0) {
		t.Fatal("count digit should be consumed")
	}
	if h.PendingSequence() != "2" {
		t.Fatalf("PendingSequence = %q, want 2", h.PendingSequence())
	}
	if !h.handleKeyPress(uint(']'), 0, 0) {
		t.Fatal("first ] after count should be consumed")
	}
	if !h.handleKeyPress(uint(']'), 0, 0) {
		t.Fatal("2]] should complete")
	}
	if len(*actions) != 1 || (*actions)[0].action != "heading-next" || (*actions)[0].count != 2 {
		t.Fatalf("actions = %#v, want heading-next count 2", *actions)
	}
}

func TestFeedVimModeSequence_AmbiguityTimeout(t *testing.T) {
	ctx := context.Background()
	ws := vimModeSequenceWorkspace(map[string]entity.ActionBinding{
		"bracket-c":  {Keys: []string{"]c"}},
		"bracket-cc": {Keys: []string{"]cc"}},
	})
	h := NewKeyboardHandler(ctx, ws, newTestSession())
	actions := captureSequenceActions(h)

	var fired func()
	var stops int
	h.seq.afterFunc = func(d time.Duration, fn func()) sequenceTimer {
		if d != 500*time.Millisecond {
			t.Fatalf("ambiguity timeout = %v, want 500ms", d)
		}
		fired = fn
		return stubSequenceTimer{onStop: func() { stops++ }}
	}
	var queued []func()
	h.SetSequenceMainThreadScheduler(func(fn func()) {
		queued = append(queued, fn)
	})

	enterVimMode(t, h)
	if !h.handleKeyPress(uint(']'), 0, 0) || !h.handleKeyPress(uint('c'), 0, 0) {
		t.Fatal("]c prefix should be consumed")
	}
	if !h.seq.matcher.Ambiguous() {
		t.Fatal("]c should be ambiguous against ]cc")
	}
	if fired == nil {
		t.Fatal("expected ambiguity timer arm")
	}

	fired() // timer expiry queues resolve on scheduler
	if len(queued) != 1 {
		t.Fatalf("queued resolves = %d, want 1", len(queued))
	}
	queued[0]()
	if len(*actions) != 1 || (*actions)[0].action != "bracket-c" {
		t.Fatalf("actions = %#v, want bracket-c", *actions)
	}
	if h.PendingSequence() != "" {
		t.Fatalf("PendingSequence after resolve = %q, want empty", h.PendingSequence())
	}
	_ = stops
}

func TestFeedVimModeSequence_TimeoutZeroResolvesImmediately(t *testing.T) {
	ctx := context.Background()
	ws := vimModeSequenceWorkspace(map[string]entity.ActionBinding{
		"bracket-c":  {Keys: []string{"]c"}},
		"bracket-cc": {Keys: []string{"]cc"}},
	})
	ws.VimMode.SequenceTimeoutMilliseconds = 0
	h := NewKeyboardHandler(ctx, ws, newTestSession())
	actions := captureSequenceActions(h)

	var armed time.Duration = -1
	h.seq.afterFunc = func(d time.Duration, fn func()) sequenceTimer {
		armed = d
		fn() // AfterFunc(0) runs promptly; stub runs inline before schedule wrap
		return stubSequenceTimer{}
	}
	var queued []func()
	h.SetSequenceMainThreadScheduler(func(fn func()) {
		queued = append(queued, fn)
	})

	enterVimMode(t, h)
	if !h.handleKeyPress(uint(']'), 0, 0) || !h.handleKeyPress(uint('c'), 0, 0) {
		t.Fatal("]c should be consumed")
	}
	if armed != 0 {
		t.Fatalf("armed timeout = %v, want 0", armed)
	}
	if len(queued) != 1 {
		t.Fatalf("queued = %d, want 1", len(queued))
	}
	queued[0]()
	if len(*actions) != 1 || (*actions)[0].action != "bracket-c" {
		t.Fatalf("actions = %#v, want bracket-c", *actions)
	}
}

func TestFeedVimModeSequence_NextKeyStopsTimer(t *testing.T) {
	ctx := context.Background()
	ws := vimModeSequenceWorkspace(map[string]entity.ActionBinding{
		"bracket-c":  {Keys: []string{"]c"}},
		"bracket-cc": {Keys: []string{"]cc"}},
	})
	h := NewKeyboardHandler(ctx, ws, newTestSession())
	actions := captureSequenceActions(h)

	var fired func()
	var stops int
	h.seq.afterFunc = func(_ time.Duration, fn func()) sequenceTimer {
		fired = fn
		return stubSequenceTimer{onStop: func() { stops++ }}
	}
	h.SetSequenceMainThreadScheduler(func(fn func()) { fn() })

	enterVimMode(t, h)
	if !h.handleKeyPress(uint(']'), 0, 0) || !h.handleKeyPress(uint('c'), 0, 0) {
		t.Fatal("]c should be consumed")
	}
	if fired == nil {
		t.Fatal("expected timer")
	}
	if !h.handleKeyPress(uint('c'), 0, 0) {
		t.Fatal("]cc should complete")
	}
	if stops == 0 {
		t.Fatal("next key must Stop ambiguity timer")
	}
	if len(*actions) != 1 || (*actions)[0].action != "bracket-cc" {
		t.Fatalf("actions = %#v, want bracket-cc", *actions)
	}
	// Stale timer must no-op.
	fired()
	if len(*actions) != 1 {
		t.Fatalf("stale timer mutated actions: %#v", *actions)
	}
}

func TestFeedVimModeSequence_StaleQueuedTimeoutNoOp(t *testing.T) {
	ctx := context.Background()
	ws := vimModeSequenceWorkspace(map[string]entity.ActionBinding{
		"bracket-c":    {Keys: []string{"]c"}},
		"bracket-cc":   {Keys: []string{"]cc"}},
		"yank-section": {Keys: []string{"yah"}},
	})
	h := NewKeyboardHandler(ctx, ws, newTestSession())
	actions := captureSequenceActions(h)
	pending := capturePending(h)

	var fired func()
	h.seq.afterFunc = func(_ time.Duration, fn func()) sequenceTimer {
		fired = fn
		return stubSequenceTimer{}
	}
	var queued []func()
	h.SetSequenceMainThreadScheduler(func(fn func()) {
		queued = append(queued, fn)
	})

	enterVimMode(t, h)
	if !h.handleKeyPress(uint(']'), 0, 0) || !h.handleKeyPress(uint('c'), 0, 0) {
		t.Fatal("]c should be consumed")
	}
	fired() // enqueue stale resolve
	if len(queued) != 1 {
		t.Fatalf("queued = %d, want 1", len(queued))
	}
	stale := queued[0]
	queued = nil

	// Interleave: feed/reset/reload before stale closure runs.
	h.ResetPendingSequence()
	if len(*pending) == 0 || (*pending)[len(*pending)-1] != "" {
		t.Fatalf("ResetPendingSequence should notify clear: %#v", *pending)
	}
	stale()
	if len(*actions) != 0 {
		t.Fatalf("stale resolve after reset must no-op, actions=%#v", *actions)
	}

	// Reload path also invalidates queued resolve.
	fired = nil
	queued = nil
	if !h.handleKeyPress(uint(']'), 0, 0) || !h.handleKeyPress(uint('c'), 0, 0) {
		t.Fatal("]c again")
	}
	fired()
	stale = queued[0]
	h.ReloadShortcuts(ctx, ws, newTestSession())
	stale()
	if len(*actions) != 0 {
		t.Fatalf("stale resolve after reload must no-op, actions=%#v", *actions)
	}
}

func TestFeedVimModeSequence_ReloadReplacesMatcher(t *testing.T) {
	ctx := context.Background()
	wsA := vimModeSequenceWorkspace(map[string]entity.ActionBinding{
		"action-a":        {Keys: []string{"yah"}},
		"vim-scroll-down": {Keys: []string{"j"}},
	})
	h := NewKeyboardHandler(ctx, wsA, newTestSession())
	actions := captureSequenceActions(h)
	enterVimMode(t, h)

	if !h.handleKeyPress(uint('y'), 0, 0) {
		t.Fatal("y should pending under trie A")
	}
	if h.PendingSequence() != "y" {
		t.Fatalf("PendingSequence = %q, want y", h.PendingSequence())
	}

	wsB := vimModeSequenceWorkspace(map[string]entity.ActionBinding{
		"action-b":        {Keys: []string{"gO"}},
		"vim-scroll-down": {Keys: []string{"j"}},
	})
	h.ReloadShortcuts(ctx, wsB, newTestSession())
	if h.PendingSequence() != "" {
		t.Fatalf("PendingSequence after reload = %q, want empty", h.PendingSequence())
	}

	// Old A sequence must not complete; fall through / invalid.
	if h.feedVimModeSequence(uint('a'), 0) {
		t.Fatal("stale A continuation must not be consumed after reload")
	}
	if len(*actions) != 0 {
		t.Fatalf("no A action after reload, got %#v", *actions)
	}

	// New B sequence works.
	if !h.handleKeyPress(uint('g'), 0, 0) {
		t.Fatal("g should pending under trie B")
	}
	if h.PendingSequence() != "g" {
		t.Fatalf("PendingSequence = %q, want g", h.PendingSequence())
	}
	if !h.handleKeyPress(uint('O'), 0, gdk.ShiftMaskValue) {
		t.Fatal("gO should complete under trie B")
	}
	if len(*actions) != 1 || (*actions)[0].action != "action-b" {
		t.Fatalf("actions = %#v, want action-b", *actions)
	}
}
