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
}

// NewMessageRouter creates a new message router.
func NewMessageRouter(ctx context.Context) *MessageRouter {
	if ctx == nil {
		ctx = context.Background()
	}

	return &MessageRouter{
		handlers: make(map[string]handlerEntry),
		baseCtx:  ctx,
	}
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
// It registers the script message handler and connects the signal.
func (r *MessageRouter) SetupMessageHandler(ucm *webkit.UserContentManager, worldName string) (uint32, error) {
	log := logging.FromContext(r.baseCtx).With().Str("component", "message-router").Logger()

	if ucm == nil {
		return 0, errors.New("user content manager is nil")
	}
	if worldName == "" {
		worldName = ScriptWorldName
	}

	if ok := ucm.RegisterScriptMessageHandler(MessageHandlerName, worldName); !ok {
		return 0, fmt.Errorf("failed to register script message handler %q in world %q", MessageHandlerName, worldName)
	}

	cb := func(_ webkit.UserContentManager, valuePtr uintptr) {
		r.handleScriptMessage(valuePtr)
	}

	r.mu.Lock()
	r.callbacks = append(r.callbacks, cb) // keep callback alive
	r.mu.Unlock()

	signalID := ucm.ConnectScriptMessageReceived(&cb)
	r.mu.Lock()
	r.signals = append(r.signals, signalID)
	r.mu.Unlock()

	log.Debug().
		Str("handler", MessageHandlerName).
		Str("world", worldName).
		Uint32("signal_id", signalID).
		Msg("script message handler connected")

	return signalID, nil
}

// handleScriptMessage decodes the JSC value and routes it to the correct handler.
func (r *MessageRouter) handleScriptMessage(valuePtr uintptr) {
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

	if msg.Type == "" {
		log.Warn().Msg("script message missing type")
		return
	}

	entry, ok := r.getHandler(msg.Type)
	if !ok || entry.handler == nil {
		log.Warn().Str("type", msg.Type).Msg("no handler registered for message type")
		return
	}

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

	script := fmt.Sprintf(`(function(){try{if(window.%[1]s){window.%[1]s(%[2]s);}else{console.warn("dumber callback missing: %[1]s");}}catch(e){console.error("dumber callback %[1]s failed", e);}})();`, callback, string(data))

	// Use async to avoid blocking the GTK main loop (signal handler context)
	wv.RunJavaScriptAsync(ctx, script, world)
	return nil
}
