package webkit

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/bnema/dumber/internal/logging"
	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// ContentBlockingManager handles WebKit content filtering
type ContentBlockingManager struct {
	filterStore *webkit.UserContentFilterStore
	storagePath string

	// Cache compiled filter to avoid recompilation
	mu             sync.RWMutex
	compiledFilter *webkit.UserContentFilter
	filterID       string
}

// NewContentBlockingManager creates a new content blocking manager
func NewContentBlockingManager(storagePath string) (*ContentBlockingManager, error) {
	// Create the filter store
	filterStore := webkit.NewUserContentFilterStore(storagePath)
	if filterStore == nil {
		return nil, fmt.Errorf("failed to create UserContentFilterStore")
	}

	logging.Debug(fmt.Sprintf("[webkit] Created UserContentFilterStore at: %s", storagePath))

	return &ContentBlockingManager{
		filterStore: filterStore,
		storagePath: storagePath,
	}, nil
}

// CompileFilters compiles JSON filter rules and caches the result for reuse.
// This should be called once during startup. Subsequent WebViews use ApplyCompiledFilter.
func (cbm *ContentBlockingManager) CompileFilters(identifier string, jsonRules []byte, onComplete func(error)) {
	if len(jsonRules) == 0 {
		if onComplete != nil {
			onComplete(fmt.Errorf("empty filter rules"))
		}
		return
	}

	logging.Debug(fmt.Sprintf("[webkit] Compiling content filters (identifier: %s, rules size: %d bytes)", identifier, len(jsonRules)))

	// Store identifier for later use
	cbm.mu.Lock()
	cbm.filterID = identifier
	cbm.mu.Unlock()

	// Convert []byte to *glib.Bytes for WebKit API
	gBytes := glib.NewBytesWithGo(jsonRules)

	// Compile asynchronously - does not block main thread
	ctx := context.Background()
	cbm.filterStore.Save(ctx, identifier, gBytes, func(result gio.AsyncResulter) {
		filter, err := cbm.filterStore.SaveFinish(result)
		if err != nil {
			logging.Error(fmt.Sprintf("[webkit] Failed to compile content filter: %v", err))
			if onComplete != nil {
				onComplete(fmt.Errorf("failed to compile filter: %w", err))
			}
			return
		}

		if filter == nil {
			logging.Error(fmt.Sprintf("[webkit] Filter compilation returned nil"))
			if onComplete != nil {
				onComplete(fmt.Errorf("filter compilation returned nil"))
			}
			return
		}

		// Cache the compiled filter
		cbm.mu.Lock()
		cbm.compiledFilter = filter
		cbm.mu.Unlock()

		logging.Debug(fmt.Sprintf("[webkit] Filter compilation complete: %s", identifier))
		if onComplete != nil {
			onComplete(nil)
		}
	})
}

// ApplyCompiledFilter applies the pre-compiled filter to a UserContentManager.
// This is instant since the filter is already compiled.
func (cbm *ContentBlockingManager) ApplyCompiledFilter(ucm *webkit.UserContentManager) error {
	if ucm == nil {
		return fmt.Errorf("UserContentManager is nil")
	}

	cbm.mu.RLock()
	filter := cbm.compiledFilter
	cbm.mu.RUnlock()

	if filter == nil {
		return fmt.Errorf("no compiled filter available")
	}

	ucm.AddFilter(filter)
	return nil
}

// IsFilterCompiled returns true if filters have been compiled and are ready to apply.
func (cbm *ContentBlockingManager) IsFilterCompiled() bool {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()
	return cbm.compiledFilter != nil
}

// ApplyFiltersFromJSON compiles and applies WebKit JSON filter rules to a UserContentManager.
// DEPRECATED: Use CompileFilters + ApplyCompiledFilter for better performance.
// This method is kept for backward compatibility.
func (cbm *ContentBlockingManager) ApplyFiltersFromJSON(ucm *webkit.UserContentManager, identifier string, jsonRules []byte) error {
	if ucm == nil {
		return fmt.Errorf("UserContentManager is nil")
	}

	// Check if already compiled
	cbm.mu.RLock()
	if cbm.compiledFilter != nil {
		filter := cbm.compiledFilter
		cbm.mu.RUnlock()
		ucm.AddFilter(filter)
		logging.Debug(fmt.Sprintf("[webkit] Applied cached content filter to UCM"))
		return nil
	}
	cbm.mu.RUnlock()

	if len(jsonRules) == 0 {
		return fmt.Errorf("empty filter rules")
	}

	logging.Debug(fmt.Sprintf("[webkit] Applying content filters (identifier: %s, rules size: %d bytes)", identifier, len(jsonRules)))

	// Convert []byte to *glib.Bytes for WebKit API
	gBytes := glib.NewBytesWithGo(jsonRules)

	ctx := context.Background()
	done := make(chan error, 1)

	cbm.filterStore.Save(ctx, identifier, gBytes, func(result gio.AsyncResulter) {
		filter, err := cbm.filterStore.SaveFinish(result)
		if err != nil {
			logging.Error(fmt.Sprintf("[webkit] Failed to compile content filter: %v", err))
			done <- fmt.Errorf("failed to compile filter: %w", err)
			return
		}

		if filter == nil {
			logging.Error(fmt.Sprintf("[webkit] Filter compilation returned nil"))
			done <- fmt.Errorf("filter compilation returned nil")
			return
		}

		// Cache for future use
		cbm.mu.Lock()
		cbm.compiledFilter = filter
		cbm.filterID = identifier
		cbm.mu.Unlock()

		RunOnMainThread(func() {
			ucm.AddFilter(filter)
			logging.Debug(fmt.Sprintf("[webkit] Successfully added content filter: %s", identifier))
			done <- nil
		})
	})

	return <-done
}

// RemoveAllFilters removes all content filters from a UserContentManager
func (cbm *ContentBlockingManager) RemoveAllFilters(ucm *webkit.UserContentManager) {
	if ucm != nil {
		ucm.RemoveAllFilters()
		logging.Debug(fmt.Sprintf("[webkit] Removed all content filters"))
	}
}

// GetFilterStore returns the underlying UserContentFilterStore
func (cbm *ContentBlockingManager) GetFilterStore() *webkit.UserContentFilterStore {
	return cbm.filterStore
}

// ApplyFiltersToWebView applies content blocking filters to a WebView
func ApplyFiltersToWebView(view *WebView, filterJSON []byte) error {
	if view == nil || view.view == nil {
		return fmt.Errorf("invalid WebView")
	}

	// Get the UserContentManager
	ucm := view.view.UserContentManager()
	if ucm == nil {
		return fmt.Errorf("UserContentManager is nil")
	}

	// Create content blocking manager
	// Use a subdirectory of the cache dir for filter storage
	dataDir := view.config.DataDir
	if dataDir == "" {
		return fmt.Errorf("WebView has no data directory configured")
	}

	filterStorePath := filepath.Join(dataDir, "content-filters")
	cbm, err := NewContentBlockingManager(filterStorePath)
	if err != nil {
		return fmt.Errorf("failed to create content blocking manager: %w", err)
	}

	// Apply the filters
	return cbm.ApplyFiltersFromJSON(ucm, "adblock-filters", filterJSON)
}
