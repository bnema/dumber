package vimkeys

import "errors"

// Node describes the trie state at the end of a walked sequence.
type Node struct {
	Action      string
	Exact       bool
	HasChildren bool
}

// ErrConflict is returned when inserting a duplicate binding.
var ErrConflict = errors.New("vimkeys: duplicate binding")

const maxCount = 9999

type trieNode struct {
	children  map[Key]*trieNode
	action    string
	hasAction bool
}

// Trie stores Vim key sequence bindings.
type Trie struct {
	root  *trieNode
	depth int
}

// NewTrie creates an empty binding trie.
func NewTrie() *Trie {
	return &Trie{root: newTrieNode()}
}

func newTrieNode() *trieNode {
	return &trieNode{children: make(map[Key]*trieNode)}
}

// Insert adds a binding for seq. Duplicate sequences return ErrConflict.
// Nil or empty sequences return ErrEmptyBinding.
func (t *Trie) Insert(seq Sequence, action string) error {
	if len(seq) == 0 {
		return ErrEmptyBinding
	}
	node := t.root
	for _, key := range seq {
		child, ok := node.children[key]
		if !ok {
			child = newTrieNode()
			node.children[key] = child
		}
		node = child
	}
	if node.hasAction {
		return ErrConflict
	}
	node.hasAction = true
	node.action = action
	if depth := len(seq); depth > t.depth {
		t.depth = depth
	}
	return nil
}

// Walk reports the trie node reached by seq and whether the path exists.
func (t *Trie) Walk(seq Sequence) (Node, bool) {
	node := t.root
	for _, key := range seq {
		child, ok := node.children[key]
		if !ok {
			return Node{}, false
		}
		node = child
	}
	return Node{
		Action:      node.action,
		Exact:       node.hasAction,
		HasChildren: len(node.children) > 0,
	}, true
}

// MaxDepth returns the longest inserted sequence length.
func (t *Trie) MaxDepth() int {
	return t.depth
}
