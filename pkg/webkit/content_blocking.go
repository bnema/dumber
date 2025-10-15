package webkit

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// ContentBlockingManager handles WebKit content filtering
type ContentBlockingManager struct {
	filterStore *webkit.UserContentFilterStore
	storagePath string
}

// NewContentBlockingManager creates a new content blocking manager
func NewContentBlockingManager(storagePath string) (*ContentBlockingManager, error) {
	// Create the filter store
	filterStore := webkit.NewUserContentFilterStore(storagePath)
	if filterStore == nil {
		return nil, fmt.Errorf("failed to create UserContentFilterStore")
	}

	log.Printf("[webkit] Created UserContentFilterStore at: %s", storagePath)

	return &ContentBlockingManager{
		filterStore: filterStore,
		storagePath: storagePath,
	}, nil
}

// ApplyFiltersFromJSON compiles and applies WebKit JSON filter rules to a UserContentManager
// The jsonRules parameter should be a JSON array of WebKit content blocker rules
func (cbm *ContentBlockingManager) ApplyFiltersFromJSON(ucm *webkit.UserContentManager, identifier string, jsonRules []byte) error {
	if ucm == nil {
		return fmt.Errorf("UserContentManager is nil")
	}

	if len(jsonRules) == 0 {
		return fmt.Errorf("empty filter rules")
	}

	log.Printf("[webkit] Applying content filters (identifier: %s, rules size: %d bytes)", identifier, len(jsonRules))

	// Convert []byte to *glib.Bytes for WebKit API
	// NewBytesWithGo keeps the Go slice alive for the GBytes lifetime
	gBytes := glib.NewBytesWithGo(jsonRules)

	// Save filters to store asynchronously
	// This compiles the JSON rules into WebKit's internal format
	ctx := context.Background()

	// Use a channel to wait for the async operation
	done := make(chan error, 1)

	cbm.filterStore.Save(ctx, identifier, gBytes, func(result gio.AsyncResulter) {
		// This callback runs when compilation is complete
		filter, err := cbm.filterStore.SaveFinish(result)
		if err != nil {
			log.Printf("[webkit] Failed to compile content filter: %v", err)
			done <- fmt.Errorf("failed to compile filter: %w", err)
			return
		}

		if filter == nil {
			log.Printf("[webkit] Filter compilation returned nil")
			done <- fmt.Errorf("filter compilation returned nil")
			return
		}

		// Add the compiled filter to the UserContentManager
		// This must be done on the main GTK thread
		RunOnMainThread(func() {
			ucm.AddFilter(filter)
			log.Printf("[webkit] Successfully added content filter: %s", identifier)
			done <- nil
		})
	})

	// Wait for completion
	return <-done
}

// RemoveAllFilters removes all content filters from a UserContentManager
func (cbm *ContentBlockingManager) RemoveAllFilters(ucm *webkit.UserContentManager) {
	if ucm != nil {
		ucm.RemoveAllFilters()
		log.Printf("[webkit] Removed all content filters")
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
