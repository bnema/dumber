package webkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/javascriptcore"
	"github.com/bnema/puregotk-webkit/webkit"
)

// Message represents a JS -> Go message envelope sent via postMessage.
type Message struct {
	Type         string          `json:"type"`
	Payload      json.RawMessage `json:"payload"`
	WebViewID    uint64          `json:"webview_id,omitempty"`
	WebViewIDAlt uint64          `json:"webviewId,omitempty"`
}

// MessageHandler handles a decoded message payload.
type MessageHandler interface {
	Handle(ctx context.Context, webviewID WebViewID, payload json.RawMessage) (any, error)
}

// MessageHandlerFunc adapts a function to the MessageHandler interface.
type MessageHandlerFunc func(ctx context.Context, webviewID WebViewID, payload json.RawMessage) (any, error)

// Handle calls f(ctx, webviewID, payload).
func (f MessageHandlerFunc) Handle(ctx context.Context, webviewID WebViewID, payload json.RawMessage) (any, error) {
	return f(ctx, webviewID, payload)
}

type handlerEntry struct {
	handler       MessageHandler
	callback      string
	errorCallback string
	world         string
}

// MessageRouter dispatches script-message events to registered handlers.
type MessageRouter struct {
	handlers map[string]handlerEntry
	baseCtx  context.Context

	mu        sync.RWMutex
	callbacks []interface{}
	signals   []uint32

	idMu      sync.Mutex
	syncedIDs map[WebViewID]bool
}

// NewMessageRouter creates a new message router.
func NewMessageRouter(ctx context.Context) *MessageRouter {
	if ctx == nil {
		ctx = context.Background()
	}

	return &MessageRouter{
		handlers:  make(map[string]handlerEntry),
		baseCtx:   ctx,
		syncedIDs: make(map[WebViewID]bool),
	}
}

// SetBaseContext updates the base context used for logging and handler execution.
func (r *MessageRouter) SetBaseContext(ctx context.Context) {
	if r == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.baseCtx = ctx
}

// RegisterHandler registers a handler for a message type.
func (r *MessageRouter) RegisterHandler(msgType string, handler MessageHandler) error {
	if msgType == "" {
		return errors.New("message type cannot be empty")
	}
	if handler == nil {
		return errors.New("message handler cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[msgType] = handlerEntry{handler: handler}
	return nil
}

// RegisterHandlerWithCallbacks registers a handler and response callbacks.
// callback is invoked on success, errorCallback (optional) on failure.
// worldName allows targeting a specific script world (empty for main world).
func (r *MessageRouter) RegisterHandlerWithCallbacks(msgType, callback, errorCallback, worldName string, handler MessageHandler) error {
	if msgType == "" {
		return errors.New("message type cannot be empty")
	}
	if handler == nil {
		return errors.New("message handler cannot be nil")
	}
	if callback == "" {
		return errors.New("callback cannot be empty when registering with callbacks")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[msgType] = handlerEntry{
		handler:       handler,
		callback:      callback,
		errorCallback: errorCallback,
		world:         worldName,
	}
	return nil
}

// SetupMessageHandler wires the router into the given UserContentManager.
// It registers the script message handler in the MAIN world (not isolated).
// WebKit's messageHandlers is only available in main world.
// The isolated world GUI scripts dispatch CustomEvents to main world, which forwards to this handler.
func (r *MessageRouter) SetupMessageHandler(ucm *webkit.UserContentManager, _ string) (uint32, error) {
	log := logging.FromContext(r.baseCtx).With().Str("component", "message-router").Logger()

	log.Debug().Msg("SetupMessageHandler called")

	if ucm == nil {
		log.Warn().Msg("SetupMessageHandler: ucm is nil")
		return 0, errors.New("user content manager is nil")
	}

	log.Debug().Msg("SetupMessageHandler: connecting signal with detail")

	// Connect to signal BEFORE registering handler to avoid race conditions
	// (as recommended by WebKit documentation)
	cb := func(ucm webkit.UserContentManager, valuePtr uintptr) {
		log.Debug().Uint64("value_ptr", uint64(valuePtr)).Msg("signal callback invoked")
		r.handleScriptMessage(ucm, valuePtr)
	}

	r.mu.Lock()
	r.callbacks = append(r.callbacks, cb) // keep callback alive
	r.mu.Unlock()

	// Connect to "script-message-received::dumber" with handler name as signal detail
	signalID := ucm.ConnectScriptMessageReceivedWithDetail(MessageHandlerName, &cb)
	log.Debug().Uint32("signal_id", signalID).Str("handler", MessageHandlerName).Msg("signal connected with detail")

	r.mu.Lock()
	r.signals = append(r.signals, signalID)
	r.mu.Unlock()

	log.Debug().Msg("SetupMessageHandler: registering handler in main world")

	// Register handler in main world - webkit.messageHandlers is only available there
	// nil = main/default world (NULL in C), empty string "" would be a world named ""
	if ok := ucm.RegisterScriptMessageHandler(MessageHandlerName, nil); !ok {
		log.Warn().Str("handler", MessageHandlerName).Msg("RegisterScriptMessageHandler returned false")
		return 0, fmt.Errorf("failed to register script message handler %q in main world", MessageHandlerName)
	}

	log.Info().
		Str("handler", MessageHandlerName).
		Str("world", "main").
		Uint32("signal_id", signalID).
		Msg("script message handler connected")

	return signalID, nil
}

// handleScriptMessage decodes the JSC value and routes it to the correct handler.
func (r *MessageRouter) handleScriptMessage(senderUCM webkit.UserContentManager, valuePtr uintptr) {
	log := logging.FromContext(r.baseCtx).With().Str("component", "message-router").Logger()

	if valuePtr == 0 {
		log.Warn().Msg("received script message with nil value pointer")
		return
	}

	jscValue := javascriptcore.ValueNewFromInternalPtr(valuePtr)
	if jscValue == nil {
		log.Warn().Msg("failed to wrap script message JSC value")
		return
	}

	rawJSON := jscValue.ToJson(0)
	if rawJSON == "" {
		log.Warn().Msg("script message JSON is empty")
		return
	}

	var msg Message
	if err := json.Unmarshal([]byte(rawJSON), &msg); err != nil {
		log.Warn().Err(err).Str("json", rawJSON).Msg("failed to unmarshal script message")
		return
	}
	if msg.WebViewID == 0 && msg.WebViewIDAlt != 0 {
		msg.WebViewID = msg.WebViewIDAlt
	}
	if msg.WebViewID == 0 {
		if senderWV := lookupSenderWebView(senderUCM); senderWV != nil {
			msg.WebViewID = uint64(senderWV.ID())
			r.syncWebViewID(senderWV)
		}
	}

	if msg.Type == "" {
		log.Warn().Msg("script message missing type")
		return
	}

	entry, ok := r.getHandler(msg.Type)
	if !ok || entry.handler == nil {
		log.Warn().Str("type", msg.Type).Msg("no handler registered for message type")
		return
	}

	log.Info().
		Str("type", msg.Type).
		Uint64("webview_id", msg.WebViewID).
		Int("payload_len", len(msg.Payload)).
		Msg("received script message")

	resp, err := entry.handler.Handle(r.baseCtx, WebViewID(msg.WebViewID), msg.Payload)
	if err != nil {
		log.Error().Err(err).Str("type", msg.Type).Msg("message handler returned error")
		if entry.errorCallback != "" {
			if respErr := r.dispatchResponse(r.baseCtx, WebViewID(msg.WebViewID), entry.errorCallback, entry.world, err.Error()); respErr != nil {
				log.Warn().Err(respErr).Msg("failed to dispatch error callback")
			}
		}
		return
	}

	if entry.callback != "" {
		if err := r.dispatchResponse(r.baseCtx, WebViewID(msg.WebViewID), entry.callback, entry.world, resp); err != nil {
			log.Warn().Err(err).
				Str("callback", entry.callback).
				Uint64("webview_id", msg.WebViewID).
				Msg("failed to dispatch callback response")
		}
	}
}

func lookupSenderWebView(senderUCM webkit.UserContentManager) *WebView {
	ptr := senderUCM.GoPointer()
	if ptr == 0 {
		return nil
	}
	return LookupWebViewByUCMPointer(ptr)
}

func (r *MessageRouter) syncWebViewID(wv *WebView) {
	if wv == nil {
		return
	}
	if !r.markWebViewIDSynced(wv.ID()) {
		return
	}
	script := fmt.Sprintf("window.__dumber_webview_id=%d;", uint64(wv.ID()))
	wv.RunJavaScript(r.baseCtx, script, "")
}

func (r *MessageRouter) markWebViewIDSynced(id WebViewID) bool {
	r.idMu.Lock()
	defer r.idMu.Unlock()
	if r.syncedIDs[id] {
		return false
	}
	r.syncedIDs[id] = true
	return true
}

// getHandler retrieves a handler by message type.
func (r *MessageRouter) getHandler(msgType string) (handlerEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.handlers[msgType]
	return entry, ok
}

// dispatchResponse serializes the payload and invokes a window callback in JS.
func (r *MessageRouter) dispatchResponse(ctx context.Context, webviewID WebViewID, callback, world string, payload any) error {
	if ctx == nil {
		ctx = r.baseCtx
	}

	if callback == "" {
		return errors.New("callback name is empty")
	}

	if webviewID == 0 {
		return errors.New("webview id is required for JS callback dispatch")
	}

	wv := LookupWebView(webviewID)
	if wv == nil {
		return fmt.Errorf("webview %d not found", webviewID)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal callback payload: %w", err)
	}

	script := fmt.Sprintf(
		`(function(){try{if(window.%[1]s){window.%[1]s(%[2]s);}`+
			`else{console.warn("dumber callback missing: %[1]s");}}`+
			`catch(e){console.error("dumber callback %[1]s failed", e);}})();`,
		callback,
		string(data),
	)

	wv.RunJavaScript(ctx, script, world)
	return nil
}
