package vimkeys

import (
	"testing"
)

func testTrie(t *testing.T) *Trie {
	t.Helper()
	trie := NewTrie()
	bindings := []struct {
		seq    Sequence
		action string
	}{
		{seq: Sequence{{Sym: "]"}, {Sym: "]"}}, action: "double-bracket"},
		{seq: Sequence{{Sym: "]"}, {Sym: "c"}}, action: "bracket-c"},
		{seq: Sequence{{Sym: "g"}, {Sym: "o", Mods: ModShift}}, action: "gO"},
		{seq: Sequence{{Sym: "y"}, {Sym: "a"}, {Sym: "h"}}, action: "yah"},
		{seq: Sequence{{Sym: "]"}}, action: "single-bracket"},
		{seq: Sequence{{Sym: "j"}}, action: "down"},
		{seq: Sequence{{Sym: "0"}}, action: "zero"},
	}
	for _, binding := range bindings {
		if err := trie.Insert(binding.seq, binding.action); err != nil {
			t.Fatalf("Insert(%v) error = %v", binding.seq, err)
		}
	}
	return trie
}

func TestMatcher_FeedPrefixesPending(t *testing.T) {
	m := NewMatcher(testTrie(t))

	cases := []struct {
		name    string
		keys    []Key
		want    ResultKind
		pending string
	}{
		{
			name:    "]] first bracket",
			keys:    []Key{{Sym: "]"}},
			want:    ResultPending,
			pending: "]",
		},
		{
			name:    "gO prefix g",
			keys:    []Key{{Sym: "g"}},
			want:    ResultPending,
			pending: "g",
		},
		{
			name:    "yah prefix ya",
			keys:    []Key{{Sym: "y"}, {Sym: "a"}},
			want:    ResultPending,
			pending: "ya",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			m.Reset()
			var result Result
			for _, key := range tt.keys {
				result = m.Feed(key)
			}
			if result.Kind != tt.want {
				t.Fatalf("Feed kind = %v, want %v", result.Kind, tt.want)
			}
			if result.Pending != tt.pending {
				t.Fatalf("Pending = %q, want %q", result.Pending, tt.pending)
			}
			if m.Pending() != tt.pending {
				t.Fatalf("Matcher.Pending() = %q, want %q", m.Pending(), tt.pending)
			}
			if result.Count != 0 {
				t.Fatalf("Count = %d, want 0 (absent)", result.Count)
			}
		})
	}
}

func TestMatcher_FeedComplete(t *testing.T) {
	m := NewMatcher(testTrie(t))

	cases := []struct {
		name   string
		keys   []Key
		action string
	}{
		{name: "]]", keys: []Key{{Sym: "]"}, {Sym: "]"}}, action: "double-bracket"},
		{name: "gO", keys: []Key{{Sym: "g"}, {Sym: "o", Mods: ModShift}}, action: "gO"},
		{name: "yah", keys: []Key{{Sym: "y"}, {Sym: "a"}, {Sym: "h"}}, action: "yah"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			m.Reset()
			var result Result
			for _, key := range tt.keys {
				result = m.Feed(key)
			}
			if result.Kind != ResultComplete {
				t.Fatalf("Feed kind = %v, want %v", result.Kind, ResultComplete)
			}
			if result.Action != tt.action {
				t.Fatalf("Action = %q, want %q", result.Action, tt.action)
			}
			if result.Count != 0 {
				t.Fatalf("Count = %d, want 0", result.Count)
			}
		})
	}
}

func TestMatcher_Counts(t *testing.T) {
	m := NewMatcher(testTrie(t))

	cases := []struct {
		name   string
		keys   []Key
		count  int
		action string
	}{
		{
			name:   "2]]",
			keys:   []Key{{Sym: "2"}, {Sym: "]"}, {Sym: "]"}},
			count:  2,
			action: "double-bracket",
		},
		{
			name:   "3j",
			keys:   []Key{{Sym: "3"}, {Sym: "j"}},
			count:  3,
			action: "down",
		},
		{
			name:   "10j",
			keys:   []Key{{Sym: "1"}, {Sym: "0"}, {Sym: "j"}},
			count:  10,
			action: "down",
		},
		{
			name:   "03j",
			keys:   []Key{{Sym: "0"}, {Sym: "3"}, {Sym: "j"}},
			count:  3,
			action: "down",
		},
		{
			name:   "0 binding",
			keys:   []Key{{Sym: "0"}},
			count:  0,
			action: "zero",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			m.Reset()
			var result Result
			for _, key := range tt.keys {
				result = m.Feed(key)
			}
			if result.Kind != ResultComplete {
				t.Fatalf("Feed kind = %v, want %v", result.Kind, ResultComplete)
			}
			if result.Count != tt.count {
				t.Fatalf("Count = %d, want %d", result.Count, tt.count)
			}
			if result.Action != tt.action {
				t.Fatalf("Action = %q, want %q", result.Action, tt.action)
			}
		})
	}
}

func TestMatcher_CountPendingString(t *testing.T) {
	m := NewMatcher(testTrie(t))
	m.Reset()
	result := m.Feed(Key{Sym: "3"})
	if result.Kind != ResultPending {
		t.Fatalf("Feed kind = %v, want %v", result.Kind, ResultPending)
	}
	if result.Pending != "3" {
		t.Fatalf("Pending = %q, want %q", result.Pending, "3")
	}
	if m.Pending() != "3" {
		t.Fatalf("Matcher.Pending() = %q, want %q", m.Pending(), "3")
	}
}

func TestMatcher_CountCap9999(t *testing.T) {
	m := NewMatcher(testTrie(t))
	digits := make([]Key, 0, 5)
	for _, ch := range "10000" {
		digits = append(digits, Key{Sym: string(ch)})
	}
	digits = append(digits, Key{Sym: "j"})

	m.Reset()
	var result Result
	for _, key := range digits {
		result = m.Feed(key)
	}
	if result.Kind != ResultComplete {
		t.Fatalf("Feed kind = %v, want %v", result.Kind, ResultComplete)
	}
	if result.Count != 9999 {
		t.Fatalf("Count = %d, want 9999", result.Count)
	}
}

func TestMatcher_AutomaticReset(t *testing.T) {
	m := NewMatcher(testTrie(t))

	m.Reset()
	complete := m.Feed(Key{Sym: "j"})
	if complete.Kind != ResultComplete {
		t.Fatalf("first Feed kind = %v, want %v", complete.Kind, ResultComplete)
	}

	next := m.Feed(Key{Sym: "g"})
	if next.Kind != ResultPending {
		t.Fatalf("after complete Feed kind = %v, want %v", next.Kind, ResultPending)
	}
	if m.Pending() != "g" {
		t.Fatalf("Pending after reset = %q, want g", m.Pending())
	}

	m.Reset()
	invalid := m.Feed(Key{Sym: "x"})
	if invalid.Kind != ResultInvalid {
		t.Fatalf("invalid Feed kind = %v, want %v", invalid.Kind, ResultInvalid)
	}

	afterInvalid := m.Feed(Key{Sym: "j"})
	if afterInvalid.Kind != ResultComplete {
		t.Fatalf("after invalid Feed kind = %v, want %v", afterInvalid.Kind, ResultComplete)
	}
}

func TestMatcher_AmbiguousShortLong(t *testing.T) {
	m := NewMatcher(testTrie(t))

	m.Reset()
	result := m.Feed(Key{Sym: "]"})
	if result.Kind != ResultPending {
		t.Fatalf("Feed ] kind = %v, want %v", result.Kind, ResultPending)
	}
	if !m.Ambiguous() {
		t.Fatal("Ambiguous() = false, want true after ]")
	}

	short, ok := m.ResolveAmbiguity()
	if !ok {
		t.Fatal("ResolveAmbiguity() ok = false, want true")
	}
	if short.Kind != ResultComplete {
		t.Fatalf("ResolveAmbiguity() kind = %v, want %v", short.Kind, ResultComplete)
	}
	if short.Action != "single-bracket" {
		t.Fatalf("short action = %q, want %q", short.Action, "single-bracket")
	}

	m.Reset()
	_ = m.Feed(Key{Sym: "]"})
	finish := m.Feed(Key{Sym: "]"})
	if finish.Kind != ResultComplete {
		t.Fatalf("finish ]] kind = %v, want %v", finish.Kind, ResultComplete)
	}
	if finish.Action != "double-bracket" {
		t.Fatalf("long action = %q, want %q", finish.Action, "double-bracket")
	}
}

func TestMatcher_Reset(t *testing.T) {
	m := NewMatcher(testTrie(t))
	_ = m.Feed(Key{Sym: "y"})
	_ = m.Feed(Key{Sym: "a"})
	m.Reset()
	if m.Pending() != "" {
		t.Fatalf("Pending after Reset = %q, want empty", m.Pending())
	}
	if m.Ambiguous() {
		t.Fatal("Ambiguous() after Reset = true, want false")
	}
}

func TestMatcher_InvalidMidSequence(t *testing.T) {
	m := NewMatcher(testTrie(t))
	m.Reset()
	_ = m.Feed(Key{Sym: "y"})
	result := m.Feed(Key{Sym: "x"})
	if result.Kind != ResultInvalid {
		t.Fatalf("Feed kind = %v, want %v", result.Kind, ResultInvalid)
	}
}

func TestMatcher_ResolveAmbiguityInvalid(t *testing.T) {
	m := NewMatcher(testTrie(t))
	m.Reset()
	_, ok := m.ResolveAmbiguity()
	if ok {
		t.Fatal("ResolveAmbiguity without prefix ok = true, want false")
	}
}

func TestMatcher_InvalidDeadEnd(t *testing.T) {
	trie := NewTrie()
	trie.root.children[Key{Sym: "x"}] = newTrieNode()
	m := NewMatcher(trie)
	result := m.Feed(Key{Sym: "x"})
	if result.Kind != ResultInvalid {
		t.Fatalf("Feed kind = %v, want %v", result.Kind, ResultInvalid)
	}
}

func TestMatcher_CountExactCap(t *testing.T) {
	m := NewMatcher(testTrie(t))
	m.Reset()
	var result Result
	for _, ch := range "9999" {
		result = m.Feed(Key{Sym: string(ch)})
	}
	result = m.Feed(Key{Sym: "j"})
	if result.Count != 9999 {
		t.Fatalf("Count = %d, want 9999", result.Count)
	}
}
