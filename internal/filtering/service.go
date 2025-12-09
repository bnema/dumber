// Package filtering provides content blocking services for WebViews.
package filtering

import (
	"fmt"
	"net/url"
	"strings"
	"sync"

	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"

	"github.com/bnema/dumber/internal/filtering/cosmetic"
	"github.com/bnema/dumber/internal/logging"
	pkgwebkit "github.com/bnema/dumber/pkg/webkit"
)

// localSchemes are URI schemes that don't need content filtering.
var localSchemes = []string{"about:", "dumb://", "file://", "data:"}

// ContentBlockingService manages content blocking for all WebViews.
// Simplified architecture:
// - Compile filter once at startup
// - Apply to each WebView's UCM directly (AddFilter is idempotent)
// - Inject cosmetic rules on navigation
type ContentBlockingService struct {
	mu sync.RWMutex

	filterManager      *FilterManager
	cosmeticInjector   *cosmetic.CosmeticInjector
	contentBlockingMgr *pkgwebkit.ContentBlockingManager
	networkFilterJSON  []byte
	filtersReady       bool
	dataDir            string
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

	return &ContentBlockingService{
		filterManager:      filterManager,
		cosmeticInjector:   filterManager.cosmeticInjector,
		contentBlockingMgr: cbm,
		dataDir:            dataDir,
	}, nil
}

// SetFiltersReady is called when filters have been parsed and are ready to compile.
// Compiles filters asynchronously - callback runs on GTK main thread.
func (s *ContentBlockingService) SetFiltersReady(networkJSON []byte) {
	s.mu.Lock()
	s.networkFilterJSON = networkJSON
	s.filtersReady = true
	s.mu.Unlock()

	if len(networkJSON) == 0 {
		logging.Warn("[filtering] SetFiltersReady called with empty network filter JSON")
		return
	}

	// Compile network filters asynchronously
	// The callback runs on GTK main thread (GIO async pattern)
	s.contentBlockingMgr.CompileFilters("adblock-filters", networkJSON, func(err error) {
		if err != nil {
			logging.Error(fmt.Sprintf("[filtering] Filter compilation failed: %v", err))
			return
		}
		logging.Info("[filtering] Network filter compilation complete")
	})
}

// RegisterWebView registers a WebView for content blocking.
// If filters are compiled, applies them immediately.
// Sets up cosmetic filtering hooks for navigation.
func (s *ContentBlockingService) RegisterWebView(wv *pkgwebkit.WebView) {
	if wv == nil {
		return
	}

	logging.Debug(fmt.Sprintf("[filtering] Registered WebView %d for content blocking", wv.ID()))

	// Setup cosmetic filtering hook for navigation
	s.setupCosmeticHook(wv)

	// Inject cosmetic base script if filters are ready
	s.mu.RLock()
	filtersReady := s.filtersReady
	s.mu.RUnlock()

	if filtersReady {
		s.injectCosmeticBaseScript(wv)
	}

	// Apply network filter if already compiled (idempotent - safe to call multiple times)
	if s.contentBlockingMgr.IsFilterCompiled() {
		s.applyNetworkFilter(wv)
	}
}

// UnregisterWebView is a no-op in simplified architecture.
// WebKit manages filter lifetime via UCM.
func (s *ContentBlockingService) UnregisterWebView(wv *pkgwebkit.WebView) {
	// No-op: UCM handles filter cleanup when WebView is destroyed
}

// setupCosmeticHook sets up navigation hook for cosmetic filtering.
func (s *ContentBlockingService) setupCosmeticHook(wv *pkgwebkit.WebView) {
	if wv == nil {
		return
	}

	// Load committed handler runs on GTK main thread
	wv.RegisterLoadCommittedHandler(func(uri string) {
		if wv.IsDestroyed() {
			return
		}
		s.injectCosmeticRules(wv, uri)
	})
}

// applyNetworkFilter applies the compiled network filter to a WebView.
// AddFilter is idempotent - safe to call multiple times.
func (s *ContentBlockingService) applyNetworkFilter(wv *pkgwebkit.WebView) {
	if wv == nil || wv.IsDestroyed() {
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

	if err := s.contentBlockingMgr.ApplyCompiledFilter(ucm); err != nil {
		logging.Error(fmt.Sprintf("[filtering] Failed to apply filter to WebView %d: %v", wv.ID(), err))
		return
	}

	logging.Debug(fmt.Sprintf("[filtering] Applied network filters to WebView %d", wv.ID()))
}

// injectCosmeticBaseScript injects the base cosmetic filtering script.
func (s *ContentBlockingService) injectCosmeticBaseScript(wv *pkgwebkit.WebView) {
	if wv == nil || wv.IsDestroyed() {
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

	injector := s.cosmeticInjector
	if injector == nil || !injector.IsEnabled() {
		return
	}

	// Get base script (empty domain = infrastructure only)
	baseScript := injector.GetScriptForDomain("")
	if baseScript == "" {
		return
	}

	// Inject at document-end when DOM is ready
	userScript := webkit.NewUserScript(
		baseScript,
		webkit.UserContentInjectAllFrames,
		webkit.UserScriptInjectAtDocumentEnd,
		nil, nil,
	)

	ucm.AddScript(userScript)
	logging.Debug(fmt.Sprintf("[filtering] Injected cosmetic base script into WebView %d", wv.ID()))
}

// isLocalScheme returns true if the URI uses a local/internal scheme
// that doesn't need content filtering.
func isLocalScheme(uri string) bool {
	for _, scheme := range localSchemes {
		if strings.HasPrefix(uri, scheme) {
			return true
		}
	}
	return false
}

// injectCosmeticRules injects domain-specific cosmetic rules.
// Called from load-committed handler (already on GTK main thread).
func (s *ContentBlockingService) injectCosmeticRules(wv *pkgwebkit.WebView, uri string) {
	if wv == nil || wv.IsDestroyed() || uri == "" {
		return
	}

	// Skip local/internal schemes
	if isLocalScheme(uri) {
		return
	}

	parsedURL, err := url.Parse(uri)
	if err != nil || parsedURL.Host == "" {
		return
	}

	domain := parsedURL.Hostname()

	injector := s.cosmeticInjector
	if injector == nil || !injector.IsEnabled() {
		return
	}

	script := injector.GetScriptForDomain(domain)
	if script == "" {
		return
	}

	logging.Debug(fmt.Sprintf("[filtering] Injecting cosmetic rules for domain: %s (%d bytes)", domain, len(script)))

	// Already on GTK main thread from load-committed handler
	if err := wv.InjectScript(script); err != nil {
		logging.Error(fmt.Sprintf("[filtering] Failed to inject cosmetic rules for %s: %v", domain, err))
	} else {
		logging.Debug(fmt.Sprintf("[filtering] Successfully injected cosmetic rules for domain: %s", domain))
	}
}

// GetFilterManager returns the underlying filter manager.
func (s *ContentBlockingService) GetFilterManager() *FilterManager {
	return s.filterManager
}

// GetCosmeticInjector returns the cosmetic injector.
func (s *ContentBlockingService) GetCosmeticInjector() *cosmetic.CosmeticInjector {
	return s.cosmeticInjector
}

// IsReady returns whether filters have been loaded.
func (s *ContentBlockingService) IsReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.filtersReady
}

// GetStats returns statistics about the content blocking service.
func (s *ContentBlockingService) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := map[string]interface{}{
		"ready":               s.filtersReady,
		"filter_compiled":     s.contentBlockingMgr.IsFilterCompiled(),
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
