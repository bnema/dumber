package ceffilter

import (
	"context"
	"sync"

	"github.com/bnema/dumber/internal/infrastructure/filtering"
	"github.com/bnema/dumber/internal/logging"
)

// Backend stores the active immutable CEF matcher snapshot.
type Backend struct {
	mu      sync.RWMutex
	matcher *Matcher
}

var _ filtering.Backend = (*Backend)(nil)

func NewBackend() *Backend {
	return &Backend{}
}

// ActivateCached returns false because the CEF MVP has no compiled on-disk
// cache; the shared manager will fall back to cached JSON files or download.
func (*Backend) ActivateCached(context.Context) (bool, error) {
	return false, nil
}

// ActivateFiles parses existing Safari Content Blocker JSON files into a CEF
// network matcher and swaps it in atomically.
func (b *Backend) ActivateFiles(ctx context.Context, paths []string) error {
	matcher, err := NewMatcherFromFiles(paths)
	if err != nil {
		return err
	}
	if skipped := matcher.SkippedRuleCount(); skipped > 0 {
		logging.FromContext(ctx).
			Warn().
			Int("active_rules", matcher.RuleCount()).
			Int("skipped_rules", skipped).
			Msg("cef: skipped unsupported content filter rules")
	}
	b.mu.Lock()
	b.matcher = matcher
	b.mu.Unlock()
	return nil
}

func (b *Backend) HasActive() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.matcher != nil
}

func (b *Backend) Clear(context.Context) error {
	b.mu.Lock()
	b.matcher = nil
	b.mu.Unlock()
	return nil
}

func (b *Backend) ShouldBlock(req Request) bool {
	b.mu.RLock()
	matcher := b.matcher
	b.mu.RUnlock()
	return matcher != nil && matcher.ShouldBlock(req)
}
