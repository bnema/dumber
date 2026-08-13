package input

import (
	"sort"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/vimkeys"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/rs/zerolog/log"
)

// trieOwnsBinding reports whether a raw config binding belongs to the Vim
// sequence trie rather than the legacy single-chord Vim Mode table.
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
					Msg("vim mode trie insert skipped")
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

// sequenceTimer is the injectable ambiguity-timeout seam.
type sequenceTimer interface {
	Stop() bool
}

// vimModeSequenceState holds per-handler matcher state and ambiguity timing.
type vimModeSequenceState struct {
	mu                   sync.Mutex
	matcher              *vimkeys.Matcher
	onPending            func(string)
	onAction             func(string, int)
	timeout              time.Duration
	timer                sequenceTimer
	afterFunc            func(time.Duration, func()) sequenceTimer
	scheduleOnMainThread func(func())
	generation           uint64
}

func newVimModeSequenceState(trie *vimkeys.Trie, timeout time.Duration) vimModeSequenceState {
	return vimModeSequenceState{
		matcher: vimkeys.NewMatcher(trie),
		timeout: timeout,
		afterFunc: func(d time.Duration, fn func()) sequenceTimer {
			return time.AfterFunc(d, fn)
		},
		scheduleOnMainThread: func(fn func()) { fn() },
	}
}

// SetOnPendingSequenceChange sets the callback for pending sequence text updates.
func (h *KeyboardHandler) SetOnPendingSequenceChange(fn func(string)) {
	h.seq.mu.Lock()
	defer h.seq.mu.Unlock()
	h.seq.onPending = fn
}

// SetOnSequenceAction sets the callback for completed sequence actions.
func (h *KeyboardHandler) SetOnSequenceAction(fn func(string, int)) {
	h.seq.mu.Lock()
	defer h.seq.mu.Unlock()
	h.seq.onAction = fn
}

// SetSequenceTimeout sets the ambiguity resolution timeout.
func (h *KeyboardHandler) SetSequenceTimeout(d time.Duration) {
	h.seq.mu.Lock()
	defer h.seq.mu.Unlock()
	h.seq.timeout = d
}

// SetSequenceMainThreadScheduler sets the main-thread dispatcher for ambiguity
// resolution. Timer goroutines must not touch matcher state directly.
func (h *KeyboardHandler) SetSequenceMainThreadScheduler(fn func(func())) {
	h.seq.mu.Lock()
	defer h.seq.mu.Unlock()
	if fn == nil {
		return
	}
	h.seq.scheduleOnMainThread = fn
}

// PendingSequence returns the current pending count+keys string.
func (h *KeyboardHandler) PendingSequence() string {
	h.seq.mu.Lock()
	defer h.seq.mu.Unlock()
	if h.seq.matcher == nil {
		return ""
	}
	return h.seq.matcher.Pending()
}

// ResetPendingSequence clears matcher state and notifies pending listeners.
func (h *KeyboardHandler) ResetPendingSequence() {
	h.resetPendingSequence(true)
}

// resetPendingSequence clears matcher/timer state. When notify is false (mode
// exit under ModalState lock), pending callbacks are skipped.
func (h *KeyboardHandler) resetPendingSequence(notify bool) {
	h.seq.mu.Lock()
	h.stopSequenceTimerLocked()
	h.seq.generation++
	if h.seq.matcher != nil {
		h.seq.matcher.Reset()
	}
	onPending := h.seq.onPending
	h.seq.mu.Unlock()

	if notify && onPending != nil {
		onPending("")
	}
}

// teardownSequenceState linearizably cancels sequence timing and clears
// UI-retaining callbacks without notifying. Used by Detach/DetachForDestroy.
// Lock order: call without holding h.mu (same as resetPendingSequence).
func (h *KeyboardHandler) teardownSequenceState() {
	h.seq.mu.Lock()
	defer h.seq.mu.Unlock()
	h.stopSequenceTimerLocked()
	h.seq.generation++
	if h.seq.matcher != nil {
		h.seq.matcher.Reset()
	}
	h.seq.onPending = nil
	h.seq.onAction = nil
	h.seq.scheduleOnMainThread = nil
	log.Debug().Msg("vim mode sequence state torn down without notify")
}

// InvalidateSequenceMatcher stops timing, bumps generation, and replaces the
// matcher from the current VimModeSequences trie (hot reload).
func (h *KeyboardHandler) InvalidateSequenceMatcher() {
	h.mu.RLock()
	var trie *vimkeys.Trie
	if h.shortcuts != nil {
		trie = h.shortcuts.VimModeSequences()
	}
	h.mu.RUnlock()

	h.seq.mu.Lock()
	defer h.seq.mu.Unlock()
	h.stopSequenceTimerLocked()
	h.seq.generation++
	h.seq.matcher = vimkeys.NewMatcher(trie)
	log.Debug().Msg("vim mode sequence matcher replaced")
}

// feedVimModeSequence feeds a key into the vim-mode sequence matcher.
// Pending/Complete consume the event (true). Invalid clears and returns false
// so the legacy shortcut path can handle the key.
func (h *KeyboardHandler) feedVimModeSequence(keyval uint, state gdk.ModifierType) bool {
	if h.modal.Mode() != ModeVim {
		return false
	}
	key, ok := KeyvalToVimKey(keyval, state)
	if !ok {
		return false
	}

	h.seq.mu.Lock()
	if h.seq.matcher == nil {
		h.seq.mu.Unlock()
		return false
	}

	h.stopSequenceTimerLocked()
	h.seq.generation++
	hadPending := h.seq.matcher.Pending() != ""

	result := h.seq.matcher.Feed(key)
	onPending := h.seq.onPending
	onAction := h.seq.onAction

	switch result.Kind {
	case vimkeys.ResultPending:
		pending := result.Pending
		if pending == "" {
			pending = h.seq.matcher.Pending()
		}
		ambiguous := h.seq.matcher.Ambiguous()
		if ambiguous {
			h.armAmbiguityTimeoutLocked()
		}
		h.seq.mu.Unlock()
		if onPending != nil {
			onPending(pending)
		}
		log.Debug().Str("pending", pending).Bool("ambiguous", ambiguous).Msg("vim mode sequence pending")
		return true

	case vimkeys.ResultComplete:
		action := result.Action
		count := result.Count
		h.seq.mu.Unlock()
		if onPending != nil {
			onPending("")
		}
		if onAction != nil {
			onAction(action, count)
		}
		log.Debug().Str("action", action).Int("count", count).Msg("vim mode sequence complete")
		return true

	default: // ResultInvalid
		h.seq.mu.Unlock()
		if hadPending && onPending != nil {
			onPending("")
		}
		log.Debug().Msg("vim mode sequence invalid; falling through to legacy")
		return false
	}
}

func (h *KeyboardHandler) stopSequenceTimerLocked() {
	if h.seq.timer != nil {
		h.seq.timer.Stop()
		h.seq.timer = nil
	}
}

func (h *KeyboardHandler) armAmbiguityTimeoutLocked() {
	gen := h.seq.generation
	timeout := h.seq.timeout
	schedule := h.seq.scheduleOnMainThread
	afterFunc := h.seq.afterFunc
	if afterFunc == nil {
		afterFunc = func(d time.Duration, fn func()) sequenceTimer {
			return time.AfterFunc(d, fn)
		}
	}
	if schedule == nil {
		schedule = func(fn func()) { fn() }
	}
	h.seq.timer = afterFunc(timeout, func() {
		schedule(func() {
			h.resolveAmbiguity(gen)
		})
	})
	log.Debug().Dur("timeout", timeout).Uint64("generation", gen).Msg("vim mode ambiguity timer armed")
}

func (h *KeyboardHandler) resolveAmbiguity(gen uint64) {
	h.seq.mu.Lock()
	if h.seq.generation != gen {
		h.seq.mu.Unlock()
		log.Debug().Uint64("generation", gen).Msg("stale vim mode ambiguity resolve ignored")
		return
	}
	if h.seq.matcher == nil {
		h.seq.mu.Unlock()
		return
	}
	result, ok := h.seq.matcher.ResolveAmbiguity()
	if !ok {
		h.seq.mu.Unlock()
		return
	}
	h.seq.timer = nil
	h.seq.generation++
	onPending := h.seq.onPending
	onAction := h.seq.onAction
	action := result.Action
	count := result.Count
	h.seq.mu.Unlock()

	if onPending != nil {
		onPending("")
	}
	if onAction != nil {
		onAction(action, count)
	}
	log.Debug().Str("action", action).Int("count", count).Msg("vim mode ambiguity resolved")
}
