package vimkeys

import "strconv"

// ResultKind classifies matcher feed outcomes.
type ResultKind int

const (
	ResultPending ResultKind = iota
	ResultComplete
	ResultInvalid
)

// Result is the outcome of feeding a key into the matcher.
type Result struct {
	Kind    ResultKind
	Count   int
	Action  string
	Pending string
}

// Matcher incrementally matches key sequences against a trie.
type Matcher struct {
	trie      *Trie
	node      *trieNode
	pending   Sequence
	count     int
	inCount   bool
	ambiguous bool
}

// NewMatcher creates a matcher backed by trie.
func NewMatcher(trie *Trie) *Matcher {
	return &Matcher{trie: trie}
}

// Reset clears matcher state.
func (m *Matcher) Reset() {
	if m == nil {
		return
	}
	m.node = nil
	m.pending = nil
	m.count = 0
	m.inCount = false
	m.ambiguous = false
}

// Pending returns the count prefix and keys matched so far.
func (m *Matcher) Pending() string {
	if !m.hasTrie() {
		return ""
	}
	return formatPending(m.count, m.pending)
}

// Ambiguous reports whether the current prefix is both exact and extendable.
func (m *Matcher) Ambiguous() bool {
	return m.hasTrie() && m.ambiguous
}

// Feed consumes one key and returns the current match state.
func (m *Matcher) Feed(key Key) Result {
	if !m.hasTrie() {
		m.Reset()
		return Result{Kind: ResultInvalid}
	}

	if m.node == nil && len(m.pending) == 0 && m.count == 0 && !m.inCount {
		m.beginSequence()
	}

	if m.inCount {
		return m.feedCountDigit(key)
	}

	if len(m.pending) == 0 && isCountStartDigit(key) {
		m.inCount = true
		return m.feedCountDigit(key)
	}

	return m.feedSequenceKey(key)
}

func (m *Matcher) hasTrie() bool {
	return m != nil && m.trie != nil && m.trie.root != nil
}

func (m *Matcher) beginSequence() {
	if !m.hasTrie() {
		m.Reset()
		return
	}
	m.node = m.trie.root
	m.pending = nil
	m.count = 0
	m.inCount = false
	m.ambiguous = false
}

func (m *Matcher) feedCountDigit(key Key) Result {
	if !isCountDigit(key) {
		m.inCount = false
		m.node = m.trie.root
		return m.feedSequenceKey(key)
	}

	digit := int(key.Sym[0] - '0')
	next := m.count*10 + digit
	if next > maxCount {
		next = maxCount
	}
	m.count = next
	return Result{Kind: ResultPending, Count: normalizedCount(m.count), Pending: m.Pending()}
}

func (m *Matcher) feedSequenceKey(key Key) Result {
	if m.node == nil {
		result := Result{Kind: ResultInvalid, Count: normalizedCount(m.count)}
		m.Reset()
		return result
	}

	child, ok := m.node.children[key]
	if !ok {
		result := Result{Kind: ResultInvalid, Count: normalizedCount(m.count)}
		m.Reset()
		return result
	}

	m.pending = append(m.pending, key)
	m.node = child

	exact := m.node.hasAction
	children := len(m.node.children) > 0
	m.ambiguous = exact && children

	switch {
	case m.ambiguous:
		return Result{Kind: ResultPending, Count: normalizedCount(m.count), Pending: m.Pending()}
	case exact:
		result := Result{
			Kind:   ResultComplete,
			Count:  normalizedCount(m.count),
			Action: m.node.action,
		}
		m.Reset()
		return result
	case children:
		return Result{Kind: ResultPending, Count: normalizedCount(m.count), Pending: m.Pending()}
	default:
		result := Result{Kind: ResultInvalid, Count: normalizedCount(m.count)}
		m.Reset()
		return result
	}
}

// ResolveAmbiguity completes the current short exact binding.
func (m *Matcher) ResolveAmbiguity() (Result, bool) {
	if !m.hasTrie() || !m.ambiguous || m.node == nil || !m.node.hasAction {
		return Result{}, false
	}

	result := Result{
		Kind:   ResultComplete,
		Count:  normalizedCount(m.count),
		Action: m.node.action,
	}
	m.Reset()
	return result, true
}

func isCountStartDigit(key Key) bool {
	return key.Mods == 0 && len(key.Sym) == 1 && key.Sym[0] >= '1' && key.Sym[0] <= '9'
}

func isCountDigit(key Key) bool {
	return key.Mods == 0 && len(key.Sym) == 1 && key.Sym[0] >= '0' && key.Sym[0] <= '9'
}

func normalizedCount(count int) int {
	if count <= 0 {
		return 0
	}
	return count
}

func formatPending(count int, pending Sequence) string {
	var out string
	if count > 0 {
		out = strconv.Itoa(count)
	}
	if len(pending) > 0 {
		out += pending.String()
	}
	return out
}
