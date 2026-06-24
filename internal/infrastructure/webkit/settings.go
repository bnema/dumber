package webkit

import (
	"context"
	"sync"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk/v4/webkit"
	"github.com/rs/zerolog"
)

const hardwareRequiredContentTypes = "video/av01;video/mp4;video/webm;video/x-h264;video/x-h265"

type mediaSettings interface {
	SetEnableWebaudio(bool)
	SetEnableWebgl(bool)
	SetEnableMedia(bool)
	SetEnableMediasource(bool)
	SetEnableMediaCapabilities(bool)
	SetEnableEncryptedMedia(bool)
	SetMediaPlaybackRequiresUserGesture(bool)
	SetMediaPlaybackAllowsInline(bool)
	SetHardwareAccelerationPolicy(webkit.HardwareAccelerationPolicy)
	SetMediaContentTypesRequiringHardwareSupport(*string)
}

// SettingsManager creates and manages WebKit Settings instances from payloads.
type SettingsManager struct {
	settings entity.EngineSettingsPayload
	mu       sync.RWMutex
}

// NewSettingsManager creates a new SettingsManager with the given payload.
func NewSettingsManager(ctx context.Context, settings entity.EngineSettingsPayload) *SettingsManager {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating settings manager")
	return &SettingsManager{settings: settings}
}

func (sm *SettingsManager) current() entity.EngineSettingsPayload {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.settings
}

// CreateSettings creates a new webkit.Settings instance configured from the current payload.
func (sm *SettingsManager) CreateSettings(ctx context.Context) *webkit.Settings {
	log := logging.FromContext(ctx)
	payload := sm.current()

	settings := webkit.NewSettings()
	if settings == nil {
		log.Error().Msg("failed to create webkit settings")
		return nil
	}

	sm.applySettings(ctx, settings, payload)
	return settings
}

// applySettings applies settings payload to a webkit.Settings instance.
func (sm *SettingsManager) applySettings(ctx context.Context, settings *webkit.Settings, payload entity.EngineSettingsPayload) {
	if sm == nil {
		return
	}
	log := logging.FromContext(ctx)
	applyJavaScriptSettings(settings)
	applyFontSettings(settings, payload.WebContent)
	applyDebugSettings(settings, payload.WebContent)
	applyBrowsingSettings(settings)
	applyMediaSettings(settings, payload.WebContent.HardwareDecoding, log)
	applyStorageSettings(settings)
	applyUISettings(settings)
	applyCanvasSettings(settings)
	applyWebRTCSettings(settings)

	webrtcEnabled := settings.GetPropertyEnableWebrtc()
	mediaStreamEnabled := settings.GetPropertyEnableMediaStream()

	log.Debug().
		Str("sans_font", payload.WebContent.SansFont).
		Bool("developer_extras", payload.WebContent.EnableDevTools).
		Bool("webrtc_enabled", webrtcEnabled).
		Bool("media_stream_enabled", mediaStreamEnabled).
		Msg("settings applied")
}

func applyJavaScriptSettings(settings *webkit.Settings) {
	settings.SetEnableJavascript(true)
	settings.SetEnableJavascriptMarkup(true)
}

func applyFontSettings(settings *webkit.Settings, payload entity.EngineWebContentSettingsPayload) {
	if payload.SansFont != "" {
		settings.SetDefaultFontFamily(payload.SansFont)
		settings.SetSansSerifFontFamily(payload.SansFont)
	}
	if payload.SerifFont != "" {
		settings.SetSerifFontFamily(payload.SerifFont)
	}
	if payload.MonospaceFont != "" {
		settings.SetMonospaceFontFamily(payload.MonospaceFont)
	}
	if payload.DefaultFontSize > 0 {
		settings.SetDefaultFontSize(uint32(payload.DefaultFontSize))
	}
}

func applyDebugSettings(settings *webkit.Settings, payload entity.EngineWebContentSettingsPayload) {
	settings.SetEnableDeveloperExtras(payload.EnableDevTools)
	settings.SetEnableWriteConsoleMessagesToStdout(payload.CaptureConsole)
	settings.SetDrawCompositingIndicators(payload.DrawCompositingIndicators)
}

func applyBrowsingSettings(settings *webkit.Settings) {
	settings.SetEnableSmoothScrolling(true)
	settings.SetEnablePageCache(true)
	settings.SetEnableSiteSpecificQuirks(true)
}

func applyMediaSettings(settings mediaSettings, mode entity.EngineHardwareDecodingMode, log *zerolog.Logger) {
	settings.SetEnableWebaudio(true)
	settings.SetEnableWebgl(true)
	settings.SetEnableMedia(true)
	settings.SetEnableMediasource(true)
	settings.SetEnableMediaCapabilities(true)
	settings.SetEnableEncryptedMedia(true)
	settings.SetMediaPlaybackRequiresUserGesture(true)
	settings.SetMediaPlaybackAllowsInline(true)

	switch mode {
	case entity.EngineHardwareDecodingForce:
		hwTypes := hardwareRequiredContentTypes
		settings.SetHardwareAccelerationPolicy(webkit.HardwareAccelerationPolicyAlwaysValue)
		settings.SetMediaContentTypesRequiringHardwareSupport(&hwTypes)
		log.Debug().Msg("hardware decoding: forced (may fail without hw support)")
	case entity.EngineHardwareDecodingDisable:
		emptyTypes := ""
		settings.SetHardwareAccelerationPolicy(webkit.HardwareAccelerationPolicyNeverValue)
		settings.SetMediaContentTypesRequiringHardwareSupport(&emptyTypes)
		log.Debug().Msg("hardware decoding: disabled (software only)")
	default:
		emptyTypes := ""
		settings.SetHardwareAccelerationPolicy(webkit.HardwareAccelerationPolicyAlwaysValue)
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
	// Set both direct API and gobject properties for robustness with generated bindings.
	settings.SetPropertyEnableMediaStream(true)
	settings.SetPropertyEnableWebrtc(true)

	settings.SetEnableMediaStream(true)
	settings.SetEnableWebrtc(true)
}

// UpdateFromPayload updates the manager with a new payload (for hot-reload).
// Note: This doesn't update already-created Settings instances.
// New WebViews will use the updated payload.
func (sm *SettingsManager) UpdateFromPayload(ctx context.Context, settings entity.EngineSettingsPayload) {
	log := logging.FromContext(ctx)
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.settings = settings
	log.Debug().Msg("settings payload updated")
}

// ApplyToWebView applies current settings to an existing WebView.
// This can be used to update a WebView's settings after config hot-reload.
func (sm *SettingsManager) ApplyToWebView(ctx context.Context, wv *webkit.WebView) {
	if sm == nil {
		return
	}
	if wv == nil {
		return
	}

	// Apply to the existing settings object attached to the WebView
	existingSettings := wv.GetSettings()
	if existingSettings != nil {
		payload := sm.current()
		sm.applySettings(ctx, existingSettings, payload)
	}
}
