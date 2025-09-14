//go:build !webkit_cgo

package webkit

import (
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/logging"
)

// SetupWebKitDebugLogging is a stub for non-CGO builds
func SetupWebKitDebugLogging(cfg *config.Config) {
	logging.Info("[webkit] Debug logging not available in stub build")
}

// CheckWebViewState always returns false in stub builds
func (w *WebView) CheckWebViewState() bool {
	return false
}

// LogDebugInfo is a stub for non-CGO builds
func (w *WebView) LogDebugInfo() {
	logging.Debug("[webkit] Debug info not available in stub build")
}
