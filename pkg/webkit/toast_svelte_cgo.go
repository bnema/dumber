//go:build webkit_cgo

package webkit

import (
	"fmt"
	"log"
)

// getToastScript returns the unified GUI bundle-based toast notification script with GTK theme integration.
// This function is deprecated and should use the new unified approach via BrowserService.InjectToastSystem
func getToastScript(assets interface{ ReadFile(string) ([]byte, error) }) string {
	// Read the unified GUI bundle from assets
	var guiBundle string
	if assets != nil {
		bundleBytes, err := assets.ReadFile("gui/gui.min.js")
		if err != nil {
			// Fallback to error message if file not found
			guiBundle = `console.error("❌ Failed to load unified GUI bundle: ` + err.Error() + `");`
			log.Printf("[webkit] Failed to read GUI bundle: %v", err)
		} else {
			guiBundle = string(bundleBytes)
			log.Printf("[webkit] Successfully loaded unified GUI bundle (%d bytes)", len(bundleBytes))
		}
	} else {
		guiBundle = `console.error("❌ Failed to load unified GUI bundle: assets not provided");`
		log.Printf("[webkit] GUI bundle error: assets not provided")
	}

	// GTK Theme Integration with new unified approach
	themeIntegration := `
// Set up theme change listener for dynamic updates
window.__dumber_setTheme = (theme) => {
  window.__dumber_initial_theme = theme;
  if (theme === 'dark') {
    document.documentElement.classList.add('dark');
  } else {
    document.documentElement.classList.remove('dark');
  }
};
`

	// Use unified GUI bundle approach
	return fmt.Sprintf(`(function() {
  function initializeToast() {
    try {
      %s

      %s

      // Initialize toast system via unified GUI bundle
      if (window.__dumber_gui && window.__dumber_gui.initializeToast) {
        window.__dumber_gui.initializeToast().then(() => {
          console.log('✅ Toast system initialized via unified GUI bundle');
        }).catch((e) => {
          console.error('❌ Toast initialization failed:', e);
        });
      } else {
        console.error('❌ Unified GUI bundle not properly loaded');
      }
    } catch (e) {
      console.error('❌ Failed to initialize toast system:', e);
    }
  }

  // Ensure DOM is ready before initializing
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initializeToast);
  } else {
    initializeToast();
  }
})();`, themeIntegration, guiBundle)
}
