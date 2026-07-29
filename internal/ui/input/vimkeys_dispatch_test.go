package input

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/vimkeys"
	"github.com/bnema/puregotk/v4/gdk"
)

func TestTrieOwnsBinding(t *testing.T) {
	cases := map[string]bool{
		"]]":      true,
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

func TestBuildPageModeTrie(t *testing.T) {
	cfg := &entity.PageModeConfig{
		Actions: map[string]entity.ActionBinding{
			"page-scroll-down": {Keys: []string{"j"}},
			"heading-next":     {Keys: []string{"]]"}},
			"yank-section":     {Keys: []string{"yah"}},
			"outline":          {Keys: []string{"gO"}},
			"half-page-down":   {Keys: []string{"<C-d>"}},
			"confirm":          {Keys: []string{"enter"}},
			"cancel":           {Keys: []string{"escape"}},
		},
	}

	trie, owned := buildPageModeTrie(cfg)
	if trie == nil {
		t.Fatal("buildPageModeTrie returned nil trie")
	}

	wantOwned := map[string]bool{
		"]]":    true,
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
}

func TestBuildPageModeTrie_ConflictKeepsFirstSortedWinner(t *testing.T) {
	// Same sequence under two raw aliases; first sorted action wins Insert.
	// Losing raw key must not be marked owned (insert failed).
	cfg := &entity.PageModeConfig{
		Actions: map[string]entity.ActionBinding{
			"zzz-later":        {Keys: []string{"C-d"}},
			"aaa-first":        {Keys: []string{"<C-d>"}},
			"page-scroll-down": {Keys: []string{"j"}},
		},
	}

	trie, owned := buildPageModeTrie(cfg)
	if !owned["<C-d>"] {
		t.Fatal("winning raw key <C-d> should be owned after successful Insert")
	}
	if owned["C-d"] {
		t.Fatal("conflicting raw key C-d must not be marked owned when Insert fails")
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

func pageModeSequencesOwnershipFixture(t *testing.T) *ShortcutSet {
	t.Helper()

	workspace := &entity.WorkspaceConfig{
		PageMode: entity.PageModeConfig{
			ActivationShortcut: "ctrl+y",
			Actions: map[string]entity.ActionBinding{
				"page-scroll-down":      {Keys: []string{"j"}},
				"page-scroll-down-fast": {Keys: []string{"shift+j"}},
				"confirm":               {Keys: []string{"enter"}},
				"cancel":                {Keys: []string{"escape"}},
				"heading-next":          {Keys: []string{"]]"}},
				"yank-section":          {Keys: []string{"yah"}},
				"outline":               {Keys: []string{"gO"}},
				"half-page-down":        {Keys: []string{"<C-d>"}},
			},
		},
	}

	set := NewShortcutSet(context.Background(), workspace, nil)
	if set.PageModeSequences() == nil {
		t.Fatal("PageModeSequences() is nil")
	}
	return set
}

func assertLegacyPageModeBindings(t *testing.T, set *ShortcutSet) {
	t.Helper()

	jBinding, jOK := ParseKeyString("j")
	if !jOK {
		t.Fatal("ParseKeyString(j) failed")
	}
	if action, found := set.PageMode[jBinding]; !found || action != ActionPageScrollDown {
		t.Fatalf("legacy j missing from PageMode: found=%v action=%q", found, action)
	}

	enterBinding := KeyBinding{Keyval: uint(gdk.KEY_Return), Modifiers: ModNone}
	if action, found := set.PageMode[enterBinding]; !found || action != ActionExitMode {
		t.Fatalf("legacy enter missing from PageMode: found=%v action=%q", found, action)
	}

	escapeBinding := KeyBinding{Keyval: uint(gdk.KEY_Escape), Modifiers: ModNone}
	if action, found := set.PageMode[escapeBinding]; !found || action != ActionExitMode {
		t.Fatalf("legacy escape missing from PageMode: found=%v action=%q", found, action)
	}
}

func assertOwnedBindingsAbsentFromPageMode(t *testing.T, set *ShortcutSet) {
	t.Helper()

	for _, key := range []string{"]]", "yah", "gO", "<C-d>"} {
		if binding, parsed := ParseKeyString(key); parsed {
			if action, found := set.PageMode[binding]; found {
				t.Fatalf("owned binding %q leaked into PageMode as %q", key, action)
			}
		}
	}
}

func assertPageModeTrieYankSection(t *testing.T, set *ShortcutSet) {
	t.Helper()

	seq, err := vimkeys.ParseBinding("yah")
	if err != nil {
		t.Fatalf("ParseBinding(yah) error = %v", err)
	}
	node, walked := set.PageModeSequences().Walk(seq)
	if !walked || !node.Exact || node.Action != "yank-section" {
		t.Fatalf("PageModeSequences Walk(yah) = (%#v, %v), want exact yank-section", node, walked)
	}
}

func TestShortcutSet_PageModeSequencesOwnership(t *testing.T) {
	set := pageModeSequencesOwnershipFixture(t)

	t.Run("legacy bindings", func(t *testing.T) {
		assertLegacyPageModeBindings(t, set)
	})
	t.Run("owned bindings absent from legacy table", func(t *testing.T) {
		assertOwnedBindingsAbsentFromPageMode(t, set)
	})
	t.Run("trie resolves owned sequence", func(t *testing.T) {
		assertPageModeTrieYankSection(t, set)
	})
}
