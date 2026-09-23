package cef

import (
	"fmt"

	"github.com/bnema/dumber/internal/logging"
	cef2gtk "github.com/bnema/purego-cef2gtk"
	"github.com/rs/zerolog"
)

func clickDiagnosticPhaseName(phase cef2gtk.ClickDiagnosticPhase) string {
	switch phase {
	case cef2gtk.ClickDiagnosticPressed:
		return "pressed"
	case cef2gtk.ClickDiagnosticReleased:
		return "released"
	case cef2gtk.ClickDiagnosticCancelled:
		return "canceled"
	case cef2gtk.ClickDiagnosticForwarded:
		return "forwarded"
	case cef2gtk.ClickDiagnosticConsumed:
		return "consumed"
	case cef2gtk.ClickDiagnosticDropped:
		return "dropped"
	default:
		return fmt.Sprintf("unknown(%d)", phase)
	}
}

func clickDropReasonName(reason cef2gtk.ClickDropReason) string {
	switch reason {
	case cef2gtk.ClickDropReasonNone:
		return "none"
	case cef2gtk.ClickDropReasonMissingHost:
		return "missing_host"
	case cef2gtk.ClickDropReasonDetached:
		return "detached"
	case cef2gtk.ClickDropReasonMissingInputState:
		return "missing_input_state"
	default:
		return fmt.Sprintf("unknown(%d)", reason)
	}
}

func clickButtonName(button uint) string {
	switch button {
	case 1:
		return "primary"
	case 2:
		return "middle"
	case 3:
		return "secondary"
	default:
		return "other"
	}
}

func (wv *WebView) logClickDiagnostic(event cef2gtk.ClickDiagnosticEvent) {
	if wv == nil || wv.ctx == nil {
		return
	}
	wv.mu.RLock()
	browserID := int32(0)
	if wv.browser != nil {
		browserID = wv.browser.GetIdentifier()
	}
	inputAttached := wv.inputAttached
	visibilityKnown := wv.effectiveVisibilityKnown
	visible := wv.effectiveVisible
	bridge := wv.viewBridge
	bridgePresent := bridge != nil
	wv.mu.RUnlock()
	bridgeFocused := bridgePresent && bridge.HasFocus()

	logger := logging.FromContext(wv.ctx)
	var logEvent *zerolog.Event
	if event.Phase == cef2gtk.ClickDiagnosticDropped || event.Phase == cef2gtk.ClickDiagnosticCancelled {
		logEvent = logger.Warn()
	} else {
		logEvent = logger.Debug()
	}
	logEvent.
		Uint64("webview_id", uint64(wv.id)).
		Int32("browser_id", browserID).
		Str("phase", clickDiagnosticPhaseName(event.Phase)).
		Uint("button", event.Button).
		Str("button_type", clickButtonName(event.Button)).
		Int("click_count", event.ClickCount).
		Str("drop_reason", clickDropReasonName(event.DropReason)).
		Bool("input_attached", inputAttached).
		Bool("effective_visibility_known", visibilityKnown).
		Bool("effective_visible", visible).
		Bool("destroyed", wv.destroyed.Load()).
		Bool("bridge_present", bridgePresent).
		Bool("bridge_focused", bridgeFocused).
		Msg("cef: click routing diagnostic")
}
