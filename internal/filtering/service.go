// Package filtering provides content blocking services for WebViews.
package filtering

import (
	"fmt"
	"net/url"
	"sync"

	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"

	"github.com/bnema/dumber/internal/filtering/cosmetic"
	"github.com/bnema/dumber/internal/logging"
	pkgwebkit "github.com/bnema/dumber/pkg/webkit"
)

// ContentBlockingService manages content blocking for all WebViews.
// It ensures network filters and cosmetic rules are applied consistently
// across all WebViews in the application.
type ContentBlockingService struct {
	mu sync.RWMutex

	// Filter data
	networkFilterJSON []byte
	filterManager     *FilterManager
	cosmeticInjector  *cosmetic.CosmeticInjector
	filtersReady      bool

	// WebView registry - tracks all active WebViews
	webViews map[*pkgwebkit.WebView]webViewState

	// Content blocking manager for compiling filters
	contentBlockingMgr *pkgwebkit.ContentBlockingManager

	// Data directory for filter storage
	dataDir string
}

// webViewState tracks per-WebView filter application state
type webViewState struct {
	networkApplied  bool
	cosmeticApplied bool
}

// NewContentBlockingService creates a new content blocking service.
func NewContentBlockingService(dataDir string, filterManager *FilterManager) (*ContentBlockingService, error) {
	if filterManager == nil {
		return nil, fmt.Errorf("filterManager is required")
	}

	cbm, err := pkgwebkit.NewContentBlockingManager(dataDir + "/content-filters")
	if err != nil {
		return nil, fmt.Errorf("failed to create content blocking manager: %w", err)
	}

	service := &ContentBlockingService{
		filterManager:      filterManager,
		cosmeticInjector:   filterManager.cosmeticInjector,
		webViews:           make(map[*pkgwebkit.WebView]webViewState),
		contentBlockingMgr: cbm,
		dataDir:            dataDir,
	}

	return service, nil
}

// SetFiltersReady is called when filters have been parsed and are ready to compile.
// It compiles filters asynchronously, then applies to all registered WebViews.
func (s *ContentBlockingService) SetFiltersReady(networkJSON []byte) {
	s.mu.Lock()
	s.networkFilterJSON = networkJSON
	s.filtersReady = true // Cosmetic rules are ready immediately
	webViews := make([]*pkgwebkit.WebView, 0, len(s.webViews))
	for wv := range s.webViews {
		webViews = append(webViews, wv)
	}
	s.mu.Unlock()

	// Inject cosmetic scripts IMMEDIATELY (don't wait for network filter compilation)
	// This hides ads visually within milliseconds while network filters compile in background
	for _, wv := range webViews {
		webView := wv // capture for closure
		pkgwebkit.RunOnMainThread(func() {
			s.injectCosmeticBaseScript(webView)
			if uri := webView.GetCurrentURL(); uri != "" {
				s.injectCosmeticRules(webView, uri)
			}
		})
	}
	logging.Info("[filtering] Cosmetic rules injected immediately to all WebViews")

	if len(networkJSON) == 0 {
		logging.Warn("[filtering] SetFiltersReady called with empty network filter JSON")
		return
	}

	// Compile network filters asynchronously - this happens in the background
	// and does not block the main thread (takes ~6 seconds)
	s.contentBlockingMgr.CompileFilters("adblock-filters", networkJSON, func(err error) {
		if err != nil {
			logging.Error(fmt.Sprintf("[filtering] Filter compilation failed: %v", err))
			return
		}

		logging.Info("[filtering] Network filter compilation complete, applying to WebViews")

		// Get current WebViews (may have changed since cosmetic injection)
		s.mu.RLock()
		currentWebViews := make([]*pkgwebkit.WebView, 0, len(s.webViews))
		for wv := range s.webViews {
			currentWebViews = append(currentWebViews, wv)
		}
		s.mu.RUnlock()

		// Apply network filters to all registered WebViews
		for _, wv := range currentWebViews {
			webView := wv // capture for closure
			pkgwebkit.RunOnMainThread(func() {
				s.applyFiltersToWebView(webView)
			})
		}
	})
}

// RegisterWebView registers a WebView for content blocking.
// If filters are already compiled, they will be applied immediately.
func (s *ContentBlockingService) RegisterWebView(wv *pkgwebkit.WebView) {
	if wv == nil {
		return
	}

	s.mu.Lock()
	if _, exists := s.webViews[wv]; exists {
		s.mu.Unlock()
		return // Already registered
	}
	s.webViews[wv] = webViewState{}
	filtersReady := s.filtersReady
	s.mu.Unlock()

	logging.Debug(fmt.Sprintf("[filtering] Registered WebView %d for content blocking", wv.ID()))

	// Setup cosmetic filtering hooks for domain-specific rules (navigation handler)
	s.setupCosmeticHooks(wv)

	// Only inject cosmetic base script if filters are loaded (rules available)
	// Otherwise, it will be injected when SetFiltersReady completes
	if filtersReady {
		s.injectCosmeticBaseScript(wv)
	}

	// Apply network filters if already compiled (instant)
	if s.contentBlockingMgr.IsFilterCompiled() {
		pkgwebkit.RunOnMainThread(func() {
			s.applyFiltersToWebView(wv)
		})
	}
}

// injectCosmeticBaseScript injects the base cosmetic filtering script into a WebView.
// This script sets up the cosmetic filtering infrastructure (MutationObserver, hide functions, etc.)
func (s *ContentBlockingService) injectCosmeticBaseScript(wv *pkgwebkit.WebView) {
	if wv == nil {
		return
	}

	gtkView := wv.GetWebView()
	if gtkView == nil {
		return
	}

	ucm := gtkView.UserContentManager()
	if ucm == nil {
		return
	}

	s.mu.RLock()
	injector := s.cosmeticInjector
	s.mu.RUnlock()

	if injector == nil || !injector.IsEnabled() {
		return
	}

	// Get the base cosmetic script (the infrastructure without domain-specific rules)
	// Pass empty domain to get just the base script
	baseScript := injector.GetScriptForDomain("")
	if baseScript == "" {
		logging.Debug(fmt.Sprintf("[filtering] No cosmetic base script available for WebView %d", wv.ID()))
		return
	}

	logging.Info(fmt.Sprintf("[filtering] Injecting cosmetic base script into WebView %d (%d bytes)", wv.ID(), len(baseScript)))

	// Inject at document-end when DOM is ready for querySelector/MutationObserver
	userScript := webkit.NewUserScript(
		baseScript,
		webkit.UserContentInjectAllFrames,
		webkit.UserScriptInjectAtDocumentEnd,
		nil, // allowList
		nil, // blockList
	)

	ucm.AddScript(userScript)
	logging.Debug(fmt.Sprintf("[filtering] Injected cosmetic base script into WebView %d", wv.ID()))
}

// UnregisterWebView removes a WebView from the registry.
func (s *ContentBlockingService) UnregisterWebView(wv *pkgwebkit.WebView) {
	if wv == nil {
		return
	}

	s.mu.Lock()
	delete(s.webViews, wv)
	s.mu.Unlock()
}

// applyFiltersToWebView applies both network and cosmetic filters to a WebView.
func (s *ContentBlockingService) applyFiltersToWebView(wv *pkgwebkit.WebView) {
	s.mu.RLock()
	networkJSON := s.networkFilterJSON
	state := s.webViews[wv]
	s.mu.RUnlock()

	// Apply network filters if not already applied
	if !state.networkApplied && len(networkJSON) > 0 {
		pkgwebkit.RunOnMainThread(func() {
			s.applyNetworkFilters(wv, networkJSON)
		})
	}
}

// applyNetworkFilters applies network blocking rules to a WebView.
func (s *ContentBlockingService) applyNetworkFilters(wv *pkgwebkit.WebView, filterJSON []byte) {
	if wv == nil {
		return
	}

	gtkView := wv.GetWebView()
	if gtkView == nil {
		return
	}

	ucm := gtkView.UserContentManager()
	if ucm == nil {
		return
	}

	// Use shared identifier so all WebViews reuse the same compiled filter
	const identifier = "adblock-filters"

	// Try to apply cached compiled filter first (instant)
	if s.contentBlockingMgr.IsFilterCompiled() {
		if err := s.contentBlockingMgr.ApplyCompiledFilter(ucm); err != nil {
			logging.Error(fmt.Sprintf("[filtering] Failed to apply cached filter to WebView %d: %v", wv.ID(), err))
			return
		}
	} else if len(filterJSON) > 0 {
		// Fallback: compile if not yet compiled (first WebView)
		err := s.contentBlockingMgr.ApplyFiltersFromJSON(ucm, identifier, filterJSON)
		if err != nil {
			logging.Error(fmt.Sprintf("[filtering] Failed to apply network filters to WebView %d: %v", wv.ID(), err))
			return
		}
	} else {
		return
	}

	// Mark as applied
	s.mu.Lock()
	if state, exists := s.webViews[wv]; exists {
		state.networkApplied = true
		s.webViews[wv] = state
	}
	s.mu.Unlock()

	logging.Info(fmt.Sprintf("[filtering] Applied network filters to WebView %d", wv.ID()))
}

// setupCosmeticHooks sets up hooks for cosmetic filtering on navigation events.
func (s *ContentBlockingService) setupCosmeticHooks(wv *pkgwebkit.WebView) {
	if wv == nil {
		return
	}

	// Register load-committed handler to inject domain-specific cosmetic rules
	wv.RegisterLoadCommittedHandler(func(uri string) {
		s.injectCosmeticRules(wv, uri)
	})
}

// injectCosmeticRules injects cosmetic filtering rules for the current page.
func (s *ContentBlockingService) injectCosmeticRules(wv *pkgwebkit.WebView, uri string) {
	if wv == nil || uri == "" {
		return
	}

	// Parse domain from URI
	parsedURL, err := url.Parse(uri)
	if err != nil || parsedURL.Host == "" {
		return
	}

	domain := parsedURL.Hostname()

	// Get cosmetic script for this domain
	s.mu.RLock()
	injector := s.cosmeticInjector
	s.mu.RUnlock()

	if injector == nil || !injector.IsEnabled() {
		return
	}

	script := injector.GetScriptForDomain(domain)
	if script == "" {
		return
	}

	// Inject the script
	logging.Info(fmt.Sprintf("[filtering] Injecting cosmetic rules for domain: %s (%d bytes)", domain, len(script)))
	pkgwebkit.RunOnMainThread(func() {
		if err := wv.InjectScript(script); err != nil {
			logging.Error(fmt.Sprintf("[filtering] Failed to inject cosmetic rules for %s: %v", domain, err))
		} else {
			logging.Info(fmt.Sprintf("[filtering] Successfully injected cosmetic rules for domain: %s", domain))
		}
	})
}

// GetFilterManager returns the underlying filter manager.
func (s *ContentBlockingService) GetFilterManager() *FilterManager {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.filterManager
}

// GetCosmeticInjector returns the cosmetic injector.
func (s *ContentBlockingService) GetCosmeticInjector() *cosmetic.CosmeticInjector {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cosmeticInjector
}

// IsReady returns whether filters have been loaded and are ready to apply.
func (s *ContentBlockingService) IsReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.filtersReady
}

// GetStats returns statistics about the content blocking service.
func (s *ContentBlockingService) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	webViewCount := len(s.webViews)
	appliedCount := 0
	for _, state := range s.webViews {
		if state.networkApplied {
			appliedCount++
		}
	}

	stats := map[string]interface{}{
		"ready":               s.filtersReady,
		"webview_count":       webViewCount,
		"filters_applied":     appliedCount,
		"network_rules_bytes": len(s.networkFilterJSON),
	}

	if s.cosmeticInjector != nil {
		cosmeticStats := s.cosmeticInjector.GetStats()
		for k, v := range cosmeticStats {
			stats["cosmetic_"+k] = v
		}
	}

	return stats
}
