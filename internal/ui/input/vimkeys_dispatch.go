package input

import (
	"sort"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/vimkeys"
	"github.com/rs/zerolog/log"
)

// trieOwnsBinding reports whether a raw config binding belongs to the Vim
// sequence trie rather than the legacy single-chord Page Mode table.
// Multi-key sequences and Vim-only chords that ParseKeyString cannot read are owned.
func trieOwnsBinding(raw string) (vimkeys.Sequence, bool) {
	seq, err := vimkeys.ParseBinding(raw)
	if err != nil {
		return nil, false
	}
	if len(seq) >= 2 {
		return seq, true
	}
	if _, ok := ParseKeyString(raw); !ok {
		return seq, true
	}
	return nil, false
}

// buildVimModeTrie inserts trie-owned Vim mode bindings. Action names are
// sorted so the first sorted name wins on Insert conflicts (validation already
// rejects duplicates). Ownership is marked only after a successful Insert.
func buildVimModeTrie(cfg *entity.VimModeConfig) (*vimkeys.Trie, map[string]bool) {
	trie := vimkeys.NewTrie()
	owned := make(map[string]bool)
	if cfg == nil {
		return trie, owned
	}

	actions := make([]string, 0, len(cfg.Actions))
	for action := range cfg.Actions {
		actions = append(actions, action)
	}
	sort.Strings(actions)

	for _, action := range actions {
		binding := cfg.Actions[action]
		keys := append([]string(nil), binding.Keys...)
		sort.Strings(keys)
		for _, key := range keys {
			seq, ok := trieOwnsBinding(key)
			if !ok {
				continue
			}
			if err := trie.Insert(seq, action); err != nil {
				log.Debug().
					Err(err).
					Str("key", key).
					Str("action", action).
					Msg("page mode trie insert skipped")
				continue
			}
			owned[key] = true
		}
	}
	return trie, owned
}

// VimModeSequences returns the Vim sequence trie, if any.
func (s *ShortcutSet) VimModeSequences() *vimkeys.Trie {
	if s == nil {
		return nil
	}
	return s.vimModeSequences
}

// filterLegacyVimModeBindings copies GetKeyBindings entries that the trie does not own.
func filterLegacyVimModeBindings(bindings map[string]string, owned map[string]bool) map[string]string {
	legacy := make(map[string]string, len(bindings))
	for key, action := range bindings {
		if owned[key] {
			continue
		}
		legacy[key] = action
	}
	return legacy
}
