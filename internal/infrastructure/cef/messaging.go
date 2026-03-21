package cef

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// Compile-time check: MessageRouter implements port.WebUIHandlerRouter.
var _ port.WebUIHandlerRouter = (*MessageRouter)(nil)

// Message represents a JS -> Go message envelope sent via fetch to /api/message.
type Message struct {
	Type         string          `json:"type"`
	Payload      json.RawMessage `json:"payload"`
	WebViewID    uint64          `json:"webview_id,omitempty"`
	WebViewIDAlt uint64          `json:"webviewId,omitempty"`
}

// MessageHandler handles a decoded message payload.
type MessageHandler interface {
	Handle(ctx context.Context, webviewID uint64, payload json.RawMessage) (any, error)
}

// MessageHandlerFunc adapts a function to the MessageHandler interface.
type MessageHandlerFunc func(ctx context.Context, webviewID uint64, payload json.RawMessage) (any, error)

// Handle calls f(ctx, webviewID, payload).
func (f MessageHandlerFunc) Handle(ctx context.Context, webviewID uint64, payload json.RawMessage) (any, error) {
	return f(ctx, webviewID, payload)
}

type handlerEntry struct {
	handler       MessageHandler
	callback      string
	errorCallback string
	world         string
}

// MessageRouter dispatches fetch-based message events to registered handlers.
type MessageRouter struct {
	handlers map[string]handlerEntry
	baseCtx  context.Context
	mu       sync.RWMutex
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

// SetBaseContext updates the base context used for handler execution.
func (r *MessageRouter) SetBaseContext(ctx context.Context) {
	if r == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	r.baseCtx = ctx
	r.mu.Unlock()
}

// RegisterHandler registers a handler for a message type.
// Satisfies port.WebUIHandlerRouter.
func (r *MessageRouter) RegisterHandler(msgType string, handler port.WebUIMessageHandler) error {
	if msgType == "" {
		return errors.New("message type cannot be empty")
	}
	if handler == nil {
		return errors.New("message handler cannot be nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[msgType] = handlerEntry{handler: portHandlerAdapter{handler}}
	return nil
}

// RegisterHandlerWithCallbacks registers a handler with response callback names.
// callback is invoked on success, errorCallback (optional) on failure.
// worldName allows targeting a specific script world (unused in CEF fetch bridge).
// Satisfies port.WebUIHandlerRouter.
func (r *MessageRouter) RegisterHandlerWithCallbacks(
	msgType, callback, errorCallback, worldName string, handler port.WebUIMessageHandler,
) error {
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
		handler:       portHandlerAdapter{handler},
		callback:      callback,
		errorCallback: errorCallback,
		world:         worldName,
	}
	return nil
}

// registerInternalHandler registers a CEF-internal handler that works with bare
// uint64 webview IDs (e.g. accent key handlers in engine.go). External callers
// should use RegisterHandler/RegisterHandlerWithCallbacks (port.WebUIHandlerRouter).
func (r *MessageRouter) registerInternalHandler(msgType string, handler MessageHandler) error {
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

// portHandlerAdapter wraps a port.WebUIMessageHandler as a cef.MessageHandler,
// converting the WebViewID type.
type portHandlerAdapter struct {
	inner port.WebUIMessageHandler
}

func (a portHandlerAdapter) Handle(ctx context.Context, webviewID uint64, payload json.RawMessage) (any, error) {
	return a.inner.Handle(ctx, port.WebViewID(webviewID), payload)
}

// HandleMessage decodes a raw JSON message body and routes it to the correct handler.
// Returns the JSON-encoded response. Called by the scheme handler for /api/message POST.
func (r *MessageRouter) HandleMessage(ctx context.Context, webviewID uint64, body []byte) ([]byte, error) {
	r.mu.RLock()
	baseCtx := r.baseCtx
	r.mu.RUnlock()
	log := logging.FromContext(baseCtx).With().Str("component", "message-router").Logger()

	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal message: %w", err)
	}
	if msg.WebViewID == 0 && msg.WebViewIDAlt != 0 {
		msg.WebViewID = msg.WebViewIDAlt
	}
	if msg.WebViewID == 0 {
		msg.WebViewID = webviewID
	}
	if msg.Type == "" {
		return nil, errors.New("message missing type")
	}

	r.mu.RLock()
	entry, ok := r.handlers[msg.Type]
	r.mu.RUnlock()

	if !ok || entry.handler == nil {
		log.Warn().Str("type", msg.Type).Msg("no handler registered for message type")
		return json.Marshal(map[string]string{"error": "unknown message type: " + msg.Type})
	}

	log.Debug().
		Str("type", msg.Type).
		Uint64("webview_id", msg.WebViewID).
		Int("payload_len", len(msg.Payload)).
		Msg("dispatching message to handler")

	if ctx == nil {
		ctx = baseCtx
	}
	resp, err := entry.handler.Handle(ctx, msg.WebViewID, msg.Payload)
	if err != nil {
		log.Error().Err(err).Str("type", msg.Type).Msg("message handler returned error")
		result := map[string]any{"error": err.Error()}
		if entry.errorCallback != "" {
			result["_callback"] = entry.errorCallback
		}
		return json.Marshal(result)
	}

	result := map[string]any{"data": resp}
	if entry.callback != "" {
		result["_callback"] = entry.callback
	}
	return json.Marshal(result)
}

// MessageBridgeJS is the JavaScript shim injected into dumb:// pages to provide
// the window.dumber.postMessage() bridge via fetch.
//
// The message body is sent via the X-Dumber-Body header (base64-encoded) rather
// than the fetch body, because CEF's PostData element API (GetElements/GetBytes)
// requires unexported wrappers in purego-cef. Using a header avoids that limitation
// while keeping the bridge simple and reliable.
const MessageBridgeJS = `
window.dumber = {
    postMessage: async function(msg) {
        const encoded = btoa(unescape(encodeURIComponent(JSON.stringify(msg))));
        const r = await fetch('/api/message', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-Dumber-Body': encoded
            }
        });
        const resp = await r.json();
        if (resp && resp._callback && window[resp._callback]) {
            window[resp._callback](resp.data);
        }
        return resp;
    }
};
`
