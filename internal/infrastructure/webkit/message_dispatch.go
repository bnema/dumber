package webkit

import (
	"context"
	"encoding/json"
	"fmt"

	"bnema.dev/dumber/internal/infrastructure/webutil"
	"bnema.dev/dumber/internal/logging"
	"bnema.dev/puregotk/v4/glib"
)

// runOnGTKMain schedules f on the GTK main loop.
func runOnGTKMain(f func()) {
	// glib copies the callback into its own registry and dispatches it
	// through one shared trampoline, so no per-call callback slot leaks.
	cb := glib.SourceFunc(func(_ uintptr) bool {
		f()
		return false
	})
	glib.IdleAdd(&cb, 0)
}

// runHandler executes entry for msg and delivers its callback. Handlers run
// on a goroutine and their response is delivered on the GTK main thread, so
// a slow handler (home widgets call remote services for up to 60s) never
// blocks the UI. Handlers must not touch GTK directly; the inline types
// listed by webutil.IsInlineBridgeMessage stay on the main thread.
func (r *MessageRouter) runHandler(baseCtx context.Context, entry handlerEntry, msg Message) {
	webviewID := WebViewID(msg.WebViewID)
	requestID := bridgeRequestID(msg.Payload)
	if webutil.IsInlineBridgeMessage(msg.Type) {
		resp, err := entry.handler.Handle(baseCtx, webviewID, msg.Payload)
		r.deliver(baseCtx, entry, msg.Type, webviewID, requestID, resp, err)
		return
	}
	stamp, ok := r.stampFor(webviewID)
	if !ok {
		return
	}
	r.goAsync(func() {
		resp, err := safeHandle(baseCtx, entry, webviewID, msg.Payload)
		r.onMain(func() {
			if now, ok := r.stampFor(webviewID); !ok || now != stamp {
				logging.FromContext(baseCtx).Debug().
					Str("type", msg.Type).
					Uint64("webview_id", uint64(webviewID)).
					Msg("dropping bridge response: the requesting page is gone")
				return
			}
			r.deliver(baseCtx, entry, msg.Type, webviewID, requestID, resp, err)
		})
	})
}

// deliveryStamp identifies one page load in one WebView.
type deliveryStamp struct {
	view       any
	generation uint64
	navigation uint64
}

// webViewDeliveryStamp returns the stamp of a WebView that shows a trusted
// system view. It runs on the GTK main thread.
func webViewDeliveryStamp(id WebViewID) (deliveryStamp, bool) {
	wv := LookupWebView(id)
	if wv == nil || wv.destroyed.Load() || !isTrustedBridgeWebView(wv) {
		return deliveryStamp{}, false
	}
	return deliveryStamp{view: wv, generation: wv.Generation(), navigation: wv.NavigationSeq()}, true
}

func safeHandle(ctx context.Context, entry handlerEntry, webviewID WebViewID, payload json.RawMessage) (resp any, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("message handler panicked: %v", p)
		}
	}()
	return entry.handler.Handle(ctx, webviewID, payload)
}

// deliver sends the handler result to the page callbacks. Responses to
// requests that carry a requestId are wrapped in the bridge envelope so the
// page matches them by id: asynchronous responses can arrive out of order.
func (r *MessageRouter) deliver(
	ctx context.Context,
	entry handlerEntry,
	msgType string,
	webviewID WebViewID,
	requestID string,
	resp any,
	err error,
) {
	log := logging.FromContext(ctx).With().Str("component", "message-router").Logger()
	log.Debug().Str("type", msgType).Err(err).Msg("handler execution completed")
	if err != nil {
		log.Error().Err(err).Str("type", msgType).Msg("message handler returned error")
		if entry.errorCallback == "" {
			return
		}
		var payload any = err.Error()
		if requestID != "" {
			payload = bridgeEnvelope{RequestID: requestID, Error: err.Error()}
		}
		if respErr := r.respond(ctx, webviewID, entry.errorCallback, entry.world, payload); respErr != nil {
			log.Warn().Err(respErr).Msg("failed to dispatch error callback")
		}
		return
	}
	if entry.callback == "" {
		return
	}
	payload := resp
	if requestID != "" && !hasRequestID(resp) {
		payload = bridgeEnvelope{RequestID: requestID, Success: true, Data: resp}
	}
	if respErr := r.respond(ctx, webviewID, entry.callback, entry.world, payload); respErr != nil {
		log.Warn().Err(respErr).
			Str("callback", entry.callback).
			Uint64("webview_id", uint64(webviewID)).
			Msg("failed to dispatch callback response")
	}
}

// bridgeEnvelope matches the systemviews bridge response envelope.
type bridgeEnvelope struct {
	RequestID string `json:"requestId"`
	Success   bool   `json:"success,omitempty"`
	Data      any    `json:"data,omitempty"`
	Error     string `json:"error,omitempty"`
}

// hasRequestID reports whether resp already is a bridge envelope (many
// handlers answer with their own {requestId, success, data} envelope).
func hasRequestID(resp any) bool {
	if resp == nil {
		return false
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return false
	}
	return bridgeRequestID(data) != ""
}

func bridgeRequestID(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var v struct {
		RequestID string `json:"requestId"`
	}
	if len(payload) == 0 || payload[0] != '{' || json.Unmarshal(payload, &v) != nil {
		return ""
	}
	return v.RequestID
}
