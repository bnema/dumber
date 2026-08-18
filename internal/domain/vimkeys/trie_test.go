package vimkeys

import (
	"errors"
	"testing"
)

func TestTrie_InsertWalkPrefixes(t *testing.T) {
	trie := NewTrie()
	bindings := []struct {
		seq    Sequence
		action string
	}{
		{seq: Sequence{{Sym: "]"}, {Sym: "]"}}, action: "double-bracket"},
		{seq: Sequence{{Sym: "]"}, {Sym: "c"}}, action: "bracket-c"},
		{seq: Sequence{{Sym: "g"}, {Sym: "o", Mods: ModShift}}, action: "gO"},
		{seq: Sequence{{Sym: "y"}, {Sym: "a"}, {Sym: "h"}}, action: "yah"},
	}
	for _, binding := range bindings {
		if err := trie.Insert(binding.seq, binding.action); err != nil {
			t.Fatalf("Insert(%v) error = %v", binding.seq, err)
		}
	}

	prefixCases := []struct {
		name         string
		seq          Sequence
		wantExact    bool
		wantChildren bool
		wantAction   string
	}{
		{name: "]] prefix ]", seq: Sequence{{Sym: "]"}}, wantChildren: true},
		{name: "]] full", seq: Sequence{{Sym: "]"}, {Sym: "]"}}, wantExact: true, wantAction: "double-bracket"},
		{name: "]c prefix ]", seq: Sequence{{Sym: "]"}}, wantChildren: true},
		{name: "]c full", seq: Sequence{{Sym: "]"}, {Sym: "c"}}, wantExact: true, wantAction: "bracket-c"},
		{name: "gO prefix g", seq: Sequence{{Sym: "g"}}, wantChildren: true},
		{name: "gO full", seq: Sequence{{Sym: "g"}, {Sym: "o", Mods: ModShift}}, wantExact: true, wantAction: "gO"},
		{name: "yah prefix y", seq: Sequence{{Sym: "y"}}, wantChildren: true},
		{name: "yah prefix ya", seq: Sequence{{Sym: "y"}, {Sym: "a"}}, wantChildren: true},
		{name: "yah full", seq: Sequence{{Sym: "y"}, {Sym: "a"}, {Sym: "h"}}, wantExact: true, wantAction: "yah"},
	}
	for _, tt := range prefixCases {
		t.Run(tt.name, func(t *testing.T) {
			node, ok := trie.Walk(tt.seq)
			if !ok {
				t.Fatalf("Walk(%v) ok = false, want true", tt.seq)
			}
			if node.Exact != tt.wantExact {
				t.Errorf("Walk(%v) Exact = %v, want %v", tt.seq, node.Exact, tt.wantExact)
			}
			if node.HasChildren != tt.wantChildren {
				t.Errorf("Walk(%v) HasChildren = %v, want %v", tt.seq, node.HasChildren, tt.wantChildren)
			}
			if tt.wantExact && node.Action != tt.wantAction {
				t.Errorf("Walk(%v) Action = %q, want %q", tt.seq, node.Action, tt.wantAction)
			}
		})
	}
}

func TestTrie_InsertDuplicateReturnsConflict(t *testing.T) {
	trie := NewTrie()
	seq := Sequence{{Sym: "j"}}
	if err := trie.Insert(seq, "first"); err != nil {
		t.Fatalf("first Insert error = %v", err)
	}
	err := trie.Insert(seq, "second")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate Insert error = %v, want %v", err, ErrConflict)
	}
}

func TestTrie_InsertEmptySequenceReturnsErrEmptyBinding(t *testing.T) {
	trie := NewTrie()
	for _, seq := range []Sequence{nil, {}} {
		err := trie.Insert(seq, "root-action")
		if !errors.Is(err, ErrEmptyBinding) {
			t.Fatalf("Insert(%v) error = %v, want %v", seq, err, ErrEmptyBinding)
		}
	}
	node, ok := trie.Walk(nil)
	if !ok {
		t.Fatal("Walk(nil) ok = false, want true for empty path")
	}
	if node.Exact || node.Action != "" {
		t.Fatalf("Walk(nil) after rejected empty Insert = %+v, want no root action", node)
	}
	if trie.MaxDepth() != 0 {
		t.Fatalf("MaxDepth() = %d after rejected empty Insert, want 0", trie.MaxDepth())
	}
}

func TestTrie_MaxDepth(t *testing.T) {
	trie := NewTrie()
	if got := trie.MaxDepth(); got != 0 {
		t.Fatalf("empty MaxDepth() = %d, want 0", got)
	}
	if err := trie.Insert(Sequence{{Sym: "y"}, {Sym: "a"}, {Sym: "h"}}, "yah"); err != nil {
		t.Fatalf("Insert(yah) error = %v", err)
	}
	if err := trie.Insert(Sequence{{Sym: "]"}, {Sym: "]"}}, "brackets"); err != nil {
		t.Fatalf("Insert(]]) error = %v", err)
	}
	if got := trie.MaxDepth(); got != 3 {
		t.Fatalf("MaxDepth() = %d, want 3", got)
	}
}

func TestTrie_WalkMissingPath(t *testing.T) {
	trie := NewTrie()
	if err := trie.Insert(Sequence{{Sym: "j"}}, "down"); err != nil {
		t.Fatalf("Insert(j) error = %v", err)
	}
	_, ok := trie.Walk(Sequence{{Sym: "k"}})
	if ok {
		t.Fatal("Walk missing path ok = true, want false")
	}
}
