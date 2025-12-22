package webkit

import (
	"context"
	"sync"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
	"github.com/rs/zerolog"
)

// SettingsManager creates and manages WebKit Settings instances from config.
type SettingsManager struct {
	cfg *config.Config
	mu  sync.RWMutex
}

// NewSettingsManager creates a new SettingsManager with the given config.
func NewSettingsManager(ctx context.Context, cfg *config.Config) *SettingsManager {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating settings manager")
	return &SettingsManager{
		cfg: cfg,
	}
}

// CreateSettings creates a new webkit.Settings instance configured from the current config.
func (sm *SettingsManager) CreateSettings(ctx context.Context) *webkit.Settings {
	log := logging.FromContext(ctx)

	sm.mu.RLock()
	cfg := sm.cfg
	sm.mu.RUnlock()

	settings := webkit.NewSettings()
	if settings == nil {
		log.Error().Msg("failed to create webkit settings")
		return nil
	}

	sm.applySettings(ctx, settings, cfg)
	return settings
}

// applySettings applies configuration to a webkit.Settings instance.
func (sm *SettingsManager) applySettings(ctx context.Context, settings *webkit.Settings, cfg *config.Config) {
	if sm == nil {
		return
	}
	log := logging.FromContext(ctx)
	applyJavaScriptSettings(settings)
	applyFontSettings(settings, cfg)
	applyRenderingSettings(settings, cfg)
	applyDebugSettings(settings, cfg)
	applyBrowsingSettings(settings)
	applyMediaSettings(settings, cfg, log)
	applyStorageSettings(settings)
	applyUISettings(settings)
	applyCanvasSettings(settings)
	applyWebRTCSettings(settings)

	log.Debug().
		Str("sans_font", cfg.Appearance.SansFont).
		Str("rendering_mode", string(cfg.Rendering.Mode)).
		Bool("developer_extras", cfg.Debug.EnableDevTools).
		Msg("settings applied")
}

func applyJavaScriptSettings(settings *webkit.Settings) {
	settings.SetEnableJavascript(true)
	settings.SetEnableJavascriptMarkup(true)
}

func applyFontSettings(settings *webkit.Settings, cfg *config.Config) {
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
}

func applyRenderingSettings(settings *webkit.Settings, cfg *config.Config) {
	switch cfg.Rendering.Mode {
	case config.RenderingModeGPU:
		settings.SetHardwareAccelerationPolicy(webkit.HardwareAccelerationPolicyAlwaysValue)
	case config.RenderingModeCPU:
		settings.SetHardwareAccelerationPolicy(webkit.HardwareAccelerationPolicyNeverValue)
	case config.RenderingModeAuto:
		settings.SetHardwareAccelerationPolicy(webkit.HardwareAccelerationPolicyAlwaysValue)
	}
}

func applyDebugSettings(settings *webkit.Settings, cfg *config.Config) {
	settings.SetEnableDeveloperExtras(cfg.Debug.EnableDevTools)
	settings.SetEnableWriteConsoleMessagesToStdout(cfg.Logging.CaptureConsole)
	settings.SetDrawCompositingIndicators(cfg.Rendering.DrawCompositingIndicators)
}

func applyBrowsingSettings(settings *webkit.Settings) {
	settings.SetEnableSmoothScrolling(true)
	settings.SetEnablePageCache(true)
	settings.SetEnableSiteSpecificQuirks(true)
}

func applyMediaSettings(settings *webkit.Settings, cfg *config.Config, log *zerolog.Logger) {
	settings.SetEnableWebaudio(true)
	settings.SetEnableWebgl(true)
	settings.SetEnableMedia(true)
	settings.SetEnableMediasource(true)
	settings.SetEnableMediaCapabilities(true)
	settings.SetEnableMediaStream(true)
	settings.SetEnableEncryptedMedia(true)
	settings.SetMediaPlaybackRequiresUserGesture(true)
	settings.SetMediaPlaybackAllowsInline(true)

	switch cfg.Media.HardwareDecodingMode {
	case config.HardwareDecodingForce:
		hwTypes := "video/av01;video/mp4;video/webm;video/x-h264;video/x-h265"
		settings.SetMediaContentTypesRequiringHardwareSupport(&hwTypes)
		log.Debug().Msg("hardware decoding: forced (may fail without hw support)")
	case config.HardwareDecodingDisable:
		settings.SetHardwareAccelerationPolicy(webkit.HardwareAccelerationPolicyNeverValue)
		log.Debug().Msg("hardware decoding: disabled (software only)")
	default:
		emptyTypes := ""
		settings.SetMediaContentTypesRequiringHardwareSupport(&emptyTypes)
		log.Debug().Msg("hardware decoding: auto (hw preferred, software fallback)")
	}
}

func applyStorageSettings(settings *webkit.Settings) {
	settings.SetEnableHtml5LocalStorage(true)
	settings.SetEnableHtml5Database(true)
}

func applyUISettings(settings *webkit.Settings) {
	settings.SetEnableBackForwardNavigationGestures(true)
	settings.SetEnableFullscreen(true)
}

func applyCanvasSettings(settings *webkit.Settings) {
	settings.SetEnable2dCanvasAcceleration(true)
}

func applyWebRTCSettings(settings *webkit.Settings) {
	settings.SetEnableWebrtc(true)
}

// UpdateFromConfig updates the manager with a new config (for hot-reload).
// Note: This doesn't update already-created Settings instances.
// New WebViews will use the updated config.
func (sm *SettingsManager) UpdateFromConfig(ctx context.Context, cfg *config.Config) {
	log := logging.FromContext(ctx)
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.cfg = cfg
	log.Debug().Msg("settings config updated")
}

// ApplyToWebView applies current settings to an existing WebView.
// This can be used to update a WebView's settings after config hot-reload.
func (sm *SettingsManager) ApplyToWebView(ctx context.Context, wv *webkit.WebView) {
	if wv == nil {
		return
	}

	settings := sm.CreateSettings(ctx)
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
		sm.applySettings(ctx, existingSettings, cfg)
	}
}
