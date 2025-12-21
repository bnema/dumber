package filtering

import (
	"context"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
	"github.com/jwijenbergh/puregotk/v4/gio"
)

// Store wraps webkit.UserContentFilterStore with Go-friendly async operations.
type Store struct {
	inner *webkit.UserContentFilterStore
	path  string
	mu    sync.Mutex
}

// NewStore creates a new Store at the given path.
// The path is where WebKit will store compiled filter bytecode.
func NewStore(storagePath string) *Store {
	inner := webkit.NewUserContentFilterStore(storagePath)
	if inner == nil {
		return nil
	}
	return &Store{
		inner: inner,
		path:  storagePath,
	}
}

// Path returns the storage path for compiled filters.
func (s *Store) Path() string {
	return s.path
}

// CompileResult holds the result of a filter compilation.
type CompileResult struct {
	Filter *webkit.UserContentFilter
	Err    error
}

// Compile compiles a JSON filter file and stores it with the given identifier.
// This is an async operation that may take several seconds for large filter sets.
// The JSON file must be in Safari Content Blocker format.
func (s *Store) Compile(ctx context.Context, identifier, jsonPath string) (*webkit.UserContentFilter, error) {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-store").
		Str("identifier", identifier).
		Str("json_path", jsonPath).
		Logger()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Create GFile from path
	file := gio.FileNewForPath(jsonPath)
	if file == nil {
		return nil, fmt.Errorf("failed to create GFile for path: %s", jsonPath)
	}

	log.Debug().Msg("starting filter compilation")

	// Use a channel to receive the async result
	resultCh := make(chan CompileResult, 1)

	// Create async callback
	cb := gio.AsyncReadyCallback(func(_ uintptr, resPtr uintptr, _ uintptr) {
		if resPtr == 0 {
			resultCh <- CompileResult{Err: fmt.Errorf("nil async result")}
			return
		}

		res := &gio.AsyncResultBase{Ptr: resPtr}
		filter, err := s.inner.SaveFromFileFinish(res)
		resultCh <- CompileResult{Filter: filter, Err: err}
	})

	// Start async compilation
	s.inner.SaveFromFile(identifier, file, nil, &cb, 0)

	// Wait for result or context cancellation
	select {
	case result := <-resultCh:
		if result.Err != nil {
			log.Error().Err(result.Err).Msg("filter compilation failed")
			return nil, result.Err
		}
		log.Info().Msg("filter compilation completed")
		return result.Filter, nil
	case <-ctx.Done():
		log.Warn().Msg("filter compilation canceled")
		return nil, ctx.Err()
	}
}

// LoadResult holds the result of loading a compiled filter.
type LoadResult struct {
	Filter *webkit.UserContentFilter
	Err    error
}

// Load loads a previously compiled filter by its identifier.
// This is fast (~50ms) as it loads pre-compiled bytecode.
func (s *Store) Load(ctx context.Context, identifier string) (*webkit.UserContentFilter, error) {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-store").
		Str("identifier", identifier).
		Logger()

	s.mu.Lock()
	defer s.mu.Unlock()

	log.Debug().Msg("loading compiled filter")

	// Use a channel to receive the async result
	resultCh := make(chan LoadResult, 1)

	// Create async callback
	cb := gio.AsyncReadyCallback(func(_ uintptr, resPtr uintptr, _ uintptr) {
		if resPtr == 0 {
			resultCh <- LoadResult{Err: fmt.Errorf("nil async result")}
			return
		}

		res := &gio.AsyncResultBase{Ptr: resPtr}
		filter, err := s.inner.LoadFinish(res)
		resultCh <- LoadResult{Filter: filter, Err: err}
	})

	// Start async load
	s.inner.Load(identifier, nil, &cb, 0)

	// Wait for result or context cancellation
	select {
	case result := <-resultCh:
		if result.Err != nil {
			log.Debug().Err(result.Err).Msg("filter load failed (may not exist)")
			return nil, result.Err
		}
		log.Debug().Msg("filter loaded successfully")
		return result.Filter, nil
	case <-ctx.Done():
		log.Warn().Msg("filter load canceled")
		return nil, ctx.Err()
	}
}

// FetchIdentifiersResult holds the result of fetching filter identifiers.
type FetchIdentifiersResult struct {
	Identifiers []string
	Err         error
}

// FetchIdentifiers returns all stored filter identifiers.
func (s *Store) FetchIdentifiers(ctx context.Context) ([]string, error) {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-store").
		Logger()

	s.mu.Lock()
	defer s.mu.Unlock()

	log.Debug().Msg("fetching filter identifiers")

	// Use a channel to receive the async result
	resultCh := make(chan FetchIdentifiersResult, 1)

	// Create async callback
	cb := gio.AsyncReadyCallback(func(_ uintptr, resPtr uintptr, _ uintptr) {
		if resPtr == 0 {
			resultCh <- FetchIdentifiersResult{Err: fmt.Errorf("nil async result")}
			return
		}

		res := &gio.AsyncResultBase{Ptr: resPtr}
		identifiers := s.inner.FetchIdentifiersFinish(res)
		resultCh <- FetchIdentifiersResult{Identifiers: identifiers}
	})

	// Start async fetch
	s.inner.FetchIdentifiers(nil, &cb, 0)

	// Wait for result or context cancellation
	select {
	case result := <-resultCh:
		if result.Err != nil {
			log.Error().Err(result.Err).Msg("failed to fetch identifiers")
			return nil, result.Err
		}
		log.Debug().Int("count", len(result.Identifiers)).Msg("fetched filter identifiers")
		return result.Identifiers, nil
	case <-ctx.Done():
		log.Warn().Msg("fetch identifiers canceled")
		return nil, ctx.Err()
	}
}

// Remove removes a compiled filter by its identifier.
func (s *Store) Remove(ctx context.Context, identifier string) error {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-store").
		Str("identifier", identifier).
		Logger()

	s.mu.Lock()
	defer s.mu.Unlock()

	log.Debug().Msg("removing filter")

	// Use a channel to receive the async result
	resultCh := make(chan error, 1)

	// Create async callback
	cb := gio.AsyncReadyCallback(func(_ uintptr, resPtr uintptr, _ uintptr) {
		if resPtr == 0 {
			resultCh <- fmt.Errorf("nil async result")
			return
		}

		res := &gio.AsyncResultBase{Ptr: resPtr}
		_, err := s.inner.RemoveFinish(res)
		resultCh <- err
	})

	// Start async remove
	s.inner.Remove(identifier, nil, &cb, 0)

	// Wait for result or context cancellation
	select {
	case err := <-resultCh:
		if err != nil {
			log.Error().Err(err).Msg("failed to remove filter")
			return err
		}
		log.Info().Msg("filter removed")
		return nil
	case <-ctx.Done():
		log.Warn().Msg("remove filter canceled")
		return ctx.Err()
	}
}

// HasCompiledFilter checks if a compiled filter exists for the given identifier.
func (s *Store) HasCompiledFilter(ctx context.Context, identifier string) bool {
	identifiers, err := s.FetchIdentifiers(ctx)
	if err != nil {
		return false
	}
	for _, id := range identifiers {
		if id == identifier {
			return true
		}
	}
	return false
}
