package webkit

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"

	"bnema.dev/dumber/internal/infrastructure/webutil"
)

// systemviewBridgeTokenField is the envelope field carrying the bridge token.
// The top-frame wrapper adds it; the native side strips it before routing.
const systemviewBridgeTokenField = "__dumber_bridge_token"

// SystemviewBridgeToken returns the token that authenticates trusted
// system-view script messages for this WebView.
func (wv *WebView) SystemviewBridgeToken() string {
	if wv == nil {
		return ""
	}
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.systemviewBridgeToken
}

func (wv *WebView) setSystemviewBridgeToken(token string) {
	if wv == nil {
		return
	}
	wv.mu.Lock()
	wv.systemviewBridgeToken = token
	wv.mu.Unlock()
}

// allowsSystemviewBridgeMessage reports whether a script message may reach the
// privileged handlers. WebKitGTK exposes window.webkit.messageHandlers to
// every frame and the signal does not say which frame sent a message, so the
// top-level URI alone cannot tell the dumb:// page from an iframe embedded in
// it. Messages must also carry the per-WebView token, which is injected only
// into the top frame of dumb:// pages; a cross-origin iframe cannot read it.
func allowsSystemviewBridgeMessage(topURI, wantToken, gotToken string) bool {
	if !isTrustedBridgeURI(topURI) || wantToken == "" || gotToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(wantToken), []byte(gotToken)) == 1
}

// parseScriptMessageJSON decodes a script message envelope and returns the
// bridge token it carries, if any.
func parseScriptMessageJSON(raw []byte) (Message, string, error) {
	var envelope struct {
		Message
		Token string `json:"__dumber_bridge_token"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Message{}, "", err
	}
	return envelope.Message, envelope.Token, nil
}

// systemviewBridgeTokenScriptTemplate wraps UserMessageHandler.prototype.postMessage
// in the top frame so every object message carries the token. Frames have
// their own prototypes, so embedded frames keep the unwrapped postMessage, and
// the token lives only in this closure.
//nolint:gosec // G101 false positive: a JS template; the token is filled in at runtime.
const systemviewBridgeTokenScriptTemplate = `(function() {
  var token = '%s';
  var handlers = window.webkit && window.webkit.messageHandlers;
  var handler = handlers && handlers.dumber;
  if (!handler) { return; }
  var proto = Object.getPrototypeOf(handler);
  var original = proto && proto.postMessage;
  if (typeof original !== 'function') { return; }
  Object.defineProperty(proto, 'postMessage', {
    value: function(message) {
      if (message && typeof message === 'object' && !Array.isArray(message)) {
        message = Object.assign({}, message);
        message['%s'] = token;
      }
      return original.call(this, message);
    },
    writable: false,
    configurable: false,
  });
})();`

func buildSystemviewBridgeTokenScript(token string) string {
	return fmt.Sprintf(systemviewBridgeTokenScriptTemplate,
		webutil.EscapeForJSString(token), systemviewBridgeTokenField)
}
