package webkit

import (
	"context"
	"sync"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
)

// SettingsManager creates and manages WebKit Settings instances from config.
type SettingsManager struct {
	cfg *config.Config
	ctx context.Context
	mu  sync.RWMutex
}

// NewSettingsManager creates a new SettingsManager with the given config.
func NewSettingsManager(ctx context.Context, cfg *config.Config) *SettingsManager {
	return &SettingsManager{
		cfg: cfg,
		ctx: logging.WithComponent(ctx, "webkit-settings"),
	}
}

// CreateSettings creates a new webkit.Settings instance configured from the current config.
func (sm *SettingsManager) CreateSettings() *webkit.Settings {
	log := logging.FromContext(sm.ctx)

	sm.mu.RLock()
	cfg := sm.cfg
	sm.mu.RUnlock()

	settings := webkit.NewSettings()
	if settings == nil {
		log.Error().Msg("failed to create webkit settings")
		return nil
	}

	sm.applySettings(settings, cfg)
	return settings
}

// applySettings applies configuration to a webkit.Settings instance.
func (sm *SettingsManager) applySettings(settings *webkit.Settings, cfg *config.Config) {
	// JavaScript settings
	settings.SetEnableJavascript(true)
	settings.SetEnableJavascriptMarkup(true)

	// Font settings from config
	if cfg.Appearance.SansFont != "" {
		settings.SetDefaultFontFamily(cfg.Appearance.SansFont)
		settings.SetSansSerifFontFamily(cfg.Appearance.SansFont)
	}
	if cfg.Appearance.SerifFont != "" {
		settings.SetSerifFontFamily(cfg.Appearance.SerifFont)
	}
	if cfg.Appearance.MonospaceFont != "" {
		settings.SetMonospaceFontFamily(cfg.Appearance.MonospaceFont)
	}
	if cfg.Appearance.DefaultFontSize > 0 {
		settings.SetDefaultFontSize(uint32(cfg.Appearance.DefaultFontSize))
	}

	// Hardware acceleration based on rendering mode
	switch cfg.RenderingMode {
	case config.RenderingModeGPU:
		settings.SetHardwareAccelerationPolicy(webkit.HardwareAccelerationPolicyAlwaysValue)
	case config.RenderingModeCPU:
		settings.SetHardwareAccelerationPolicy(webkit.HardwareAccelerationPolicyNeverValue)
	case config.RenderingModeAuto:
		// Default to always for auto mode (WebKit handles capability detection)
		settings.SetHardwareAccelerationPolicy(webkit.HardwareAccelerationPolicyAlwaysValue)
	}

	// Debug settings
	settings.SetEnableDeveloperExtras(cfg.Debug.EnableDevTools)
	settings.SetEnableWriteConsoleMessagesToStdout(cfg.Logging.CaptureConsole)

	// Browsing experience
	settings.SetEnableSmoothScrolling(true)
	// Note: DNS prefetching setting deprecated in WebKitGTK 6 - use NetworkSession.PrefetchDns() instead
	settings.SetEnablePageCache(true)
	settings.SetEnableSiteSpecificQuirks(true)

	// Media settings
	settings.SetEnableWebaudio(true)
	settings.SetEnableWebgl(true)
	settings.SetEnableMedia(true)
	settings.SetEnableMediasource(true)
	settings.SetEnableMediaCapabilities(true)
	settings.SetEnableMediaStream(true)
	settings.SetEnableEncryptedMedia(true)
	settings.SetMediaPlaybackRequiresUserGesture(true)

	// HTML5 storage
	settings.SetEnableHtml5LocalStorage(true)
	settings.SetEnableHtml5Database(true)

	// UI behavior - keyboard-driven browser doesn't need swipe gestures
	settings.SetEnableBackForwardNavigationGestures(false)
	settings.SetEnableFullscreen(true)

	// Canvas acceleration
	settings.SetEnable2dCanvasAcceleration(true)

	// WebRTC
	settings.SetEnableWebrtc(true)

	log := logging.FromContext(sm.ctx)
	log.Debug().
		Str("sans_font", cfg.Appearance.SansFont).
		Str("rendering_mode", string(cfg.RenderingMode)).
		Bool("developer_extras", cfg.Debug.EnableDevTools).
		Msg("settings applied")
}

// UpdateFromConfig updates the manager with a new config (for hot-reload).
// Note: This doesn't update already-created Settings instances.
// New WebViews will use the updated config.
func (sm *SettingsManager) UpdateFromConfig(cfg *config.Config) {
	log := logging.FromContext(sm.ctx)
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.cfg = cfg
	log.Debug().Msg("settings config updated")
}

// ApplyToWebView applies current settings to an existing WebView.
// This can be used to update a WebView's settings after config hot-reload.
func (sm *SettingsManager) ApplyToWebView(wv *webkit.WebView) {
	if wv == nil {
		return
	}

	settings := sm.CreateSettings()
	if settings == nil {
		return
	}

	// WebView.GetSettings() returns the current settings - we can modify them directly
	// For a full replacement, we'd need webkit_web_view_set_settings which may not be available
	// Instead, we apply to the existing settings object
	existingSettings := wv.GetSettings()
	if existingSettings != nil {
		sm.mu.RLock()
		cfg := sm.cfg
		sm.mu.RUnlock()
		sm.applySettings(existingSettings, cfg)
	}
}
