package webkitfilter

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/infrastructure/filtering"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk/v4/webkit"
)

const storeDirPerm = 0o755

// StoreOps abstracts WebKit's UserContentFilterStore for tests.
type StoreOps interface {
	Compile(ctx context.Context, identifier string, jsonPath string) (*webkit.UserContentFilter, error)
	Load(ctx context.Context, identifier string) (*webkit.UserContentFilter, error)
	Remove(ctx context.Context, identifier string) error
	FetchIdentifiers(ctx context.Context) ([]string, error)
	Path() string
}

// Backend activates Safari Content Blocker JSON files for WebKitGTK.
type Backend struct {
	store StoreOps

	filterMu sync.RWMutex
	filters  []*webkit.UserContentFilter
}

// Config configures the WebKit filter backend.
type Config struct {
	StoreDir string
	Store    StoreOps
	NewStore func(string) StoreOps
}

var _ filtering.Backend = (*Backend)(nil)

// NewBackend creates a WebKit filter backend.
func NewBackend(cfg Config) (*Backend, error) {
	store := cfg.Store
	if store == nil {
		if err := os.MkdirAll(cfg.StoreDir, storeDirPerm); err != nil {
			return nil, fmt.Errorf("failed to create filter store dir: %w", err)
		}
		newStore := cfg.NewStore
		if newStore == nil {
			newStore = func(path string) StoreOps { return NewStore(path) }
		}
		store = newStore(cfg.StoreDir)
		if store == nil {
			return nil, fmt.Errorf("failed to create filter store at %s", cfg.StoreDir)
		}
	}
	return &Backend{store: store}, nil
}

// ActivateCached loads compiled WebKit filters from UserContentFilterStore.
func (b *Backend) ActivateCached(ctx context.Context) (bool, error) {
	log := logging.FromContext(ctx).With().
		Str("component", "webkit-filter-backend").
		Logger()

	identifiers, err := b.store.FetchIdentifiers(ctx)
	if err != nil {
		return false, err
	}

	partIDs := make([]string, 0, len(identifiers))
	for _, id := range identifiers {
		if strings.HasPrefix(id, filtering.FilterIdentifierPrefix) {
			partIDs = append(partIDs, id)
		}
	}
	if len(partIDs) == 0 {
		return false, nil
	}

	log.Debug().Int("parts", len(partIDs)).Msg("found compiled filters, loading from cache")
	filters := make([]*webkit.UserContentFilter, 0, len(partIDs))
	for _, id := range partIDs {
		filter, loadErr := b.store.Load(ctx, id)
		if loadErr != nil || filter == nil {
			log.Warn().Err(loadErr).Str("id", id).Msg("failed to load cached filter part")
			return false, loadErr
		}
		filters = append(filters, filter)
	}

	b.setFilters(filters)
	return true, nil
}

// ActivateFiles compiles downloaded Safari Content Blocker JSON files and makes them active.
func (b *Backend) ActivateFiles(ctx context.Context, paths []string) error {
	log := logging.FromContext(ctx).With().
		Str("component", "webkit-filter-backend").
		Logger()

	filters := make([]*webkit.UserContentFilter, 0, len(paths))
	for i, path := range paths {
		identifier := fmt.Sprintf("%s-%d", filtering.FilterIdentifierPrefix, i)
		log.Debug().Str("id", identifier).Str("path", path).Msg("compiling filter part")

		filter, err := b.store.Compile(ctx, identifier, path)
		if err != nil {
			return fmt.Errorf("compile %s: %w", identifier, err)
		}
		if filter == nil {
			return fmt.Errorf("compile %s returned nil filter", identifier)
		}
		filters = append(filters, filter)
	}

	b.setFilters(filters)
	return nil
}

// HasActive reports whether WebKit filters are currently loaded.
func (b *Backend) HasActive() bool {
	b.filterMu.RLock()
	defer b.filterMu.RUnlock()
	return len(b.filters) > 0
}

// Clear removes active WebKit filters and compiled filter cache entries.
func (b *Backend) Clear(ctx context.Context) error {
	log := logging.FromContext(ctx).With().
		Str("component", "webkit-filter-backend").
		Logger()

	b.setFilters(nil)

	identifiers, err := b.store.FetchIdentifiers(ctx)
	if err != nil {
		return fmt.Errorf("fetch identifiers: %w", err)
	}
	for _, id := range identifiers {
		if strings.HasPrefix(id, filtering.FilterIdentifierPrefix) {
			if removeErr := b.store.Remove(ctx, id); removeErr != nil {
				log.Warn().Err(removeErr).Str("id", id).Msg("failed to remove compiled filter")
			}
		}
	}
	return nil
}

// ApplyTo adds active filters to a WebKit UserContentManager.
func (b *Backend) ApplyTo(ctx context.Context, ucm *webkit.UserContentManager) {
	log := logging.FromContext(ctx).With().
		Str("component", "webkit-filter-backend").
		Logger()

	if ucm == nil {
		return
	}

	b.filterMu.RLock()
	filters := append([]*webkit.UserContentFilter(nil), b.filters...)
	b.filterMu.RUnlock()

	if len(filters) == 0 {
		log.Debug().Msg("no filter available to apply (filters still loading?)")
		return
	}
	for _, filter := range filters {
		ucm.AddFilter(filter)
	}
	log.Debug().Int("parts", len(filters)).Msg("content filters applied to webview")
}

// GetFilters returns the currently loaded filters, or nil if none.
func (b *Backend) GetFilters() []*webkit.UserContentFilter {
	b.filterMu.RLock()
	defer b.filterMu.RUnlock()
	return append([]*webkit.UserContentFilter(nil), b.filters...)
}

func (b *Backend) setFilters(filters []*webkit.UserContentFilter) {
	b.filterMu.Lock()
	defer b.filterMu.Unlock()
	b.filters = append([]*webkit.UserContentFilter(nil), filters...)
}
