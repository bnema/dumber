package cef

import (
	"context"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

var _ port.VimPageInteractor = (*WebView)(nil)

//go:embed vim_page_runtime.js
var vimPageRuntimeTemplateJS string

var errVimPageTokenUnavailable = errors.New("vim page: interaction token unavailable")

// vimPageBridgePayload is one result reported by the page runtime. Token is
// the per-interaction secret issued by StartVimPageInteraction.
type vimPageBridgePayload struct {
	Type  string `json:"type"`
	Token string `json:"token"`
	Text  string `json:"text,omitempty"`
	URL   string `json:"url,omitempty"`
	// Reason explains an early end (for example no visible targets).
	Reason string `json:"reason,omitempty"`
}

const (
	vimPageMessageCopy    = "copy"
	vimPageMessageOpenNew = "open-new"
	vimPageMessageEnd     = "end"
)

func decodeVimPageBridgePayload(body []byte) (vimPageBridgePayload, error) {
	var payload vimPageBridgePayload
	if len(body) > maxClipboardBytes {
		return payload, errors.New("payload too large")
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return payload, err
	}
	if payload.Token == "" {
		return payload, errors.New("missing token")
	}
	switch payload.Type {
	case vimPageMessageCopy:
		if payload.Text == "" {
			return payload, errors.New("missing text")
		}
	case vimPageMessageOpenNew:
		if payload.URL == "" {
			return payload, errors.New("missing url")
		}
	case vimPageMessageEnd:
	default:
		return payload, fmt.Errorf("unknown type %q", payload.Type)
	}
	return payload, nil
}

// isNavigableVimHintURL keeps hint-opened panes to web documents with a host
// so a page cannot smuggle javascript: or internal URLs through the bridge.
// file: targets are allowed only from a page that is itself a file: document,
// matching Chromium's own http(s)-to-file navigation block.
func isNavigableVimHintURL(raw, sourceURI string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed.Host != ""
	case "file":
		source, err := url.Parse(sourceURI)
		return err == nil && strings.EqualFold(source.Scheme, "file")
	default:
		return false
	}
}

func vimPageKindName(kind dto.VimPageInteractionKind) (string, bool) {
	switch kind {
	case dto.VimPageHintFollow:
		return "hint-follow", true
	case dto.VimPageHintFollowNew:
		return "hint-follow-new", true
	case dto.VimPageHintYankURL:
		return "hint-yank-url", true
	case dto.VimPageVisual:
		return "visual", true
	case dto.VimPageYankObject:
		return "yank", true
	default:
		return "", false
	}
}

func vimPageObjectName(object dto.VimPageTextObject) (string, bool) {
	switch object {
	case dto.VimPageTextObjectSection:
		return "section", true
	case dto.VimPageTextObjectParagraph:
		return "paragraph", true
	case dto.VimPageTextObjectCode:
		return "code", true
	case dto.VimPageTextObjectTable:
		return "table", true
	case dto.VimPageTextObjectList:
		return "list", true
	default:
		return "", false
	}
}

func vimPageRequestJSON(request dto.VimPageInteractionRequest) (string, error) {
	kind, ok := vimPageKindName(request.Kind)
	if !ok {
		return "", fmt.Errorf("unsupported vim page interaction: %d", request.Kind)
	}
	object := ""
	if request.Kind == dto.VimPageYankObject {
		if object, ok = vimPageObjectName(request.Object); !ok {
			return "", fmt.Errorf("unsupported vim text object: %d", request.Object)
		}
	}
	encoded, err := json.Marshal(struct {
		Kind   string `json:"kind"`
		Object string `json:"object,omitempty"`
		Color  string `json:"color,omitempty"`
	}{Kind: kind, Object: object, Color: request.HighlightColor})
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// jsString renders s as a JavaScript string literal (JSON is valid JS).
func jsString(s string) string {
	encoded, _ := json.Marshal(s)
	return string(encoded)
}

// vimPageScript installs the runtime when absent and then invokes one method
// with the interaction token. The token is a per-interaction secret, not the
// shared bridge nonce, so a page that intercepts the call only learns a token
// valid for the interaction the user just started.
func vimPageScript(method, token string, args ...string) string {
	call := append([]string{jsString(token)}, args...)
	return vimPageRuntimeTemplateJS + "\nwindow.__dumberVimPage." + method + "(" + strings.Join(call, ", ") + ");"
}

// armVimPageInteraction issues a fresh token for a new interaction and
// invalidates any previous one, so late results from it are rejected.
func (wv *WebView) armVimPageInteraction(kind dto.VimPageInteractionKind) (string, error) {
	token := newBridgeNonce()
	if token == "" {
		return "", errVimPageTokenUnavailable
	}
	wv.mu.Lock()
	wv.vimPageToken = token
	wv.vimPageKind = kind
	wv.vimPageGeneration++
	wv.mu.Unlock()
	return token, nil
}

func (wv *WebView) currentVimPageToken() string {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.vimPageToken
}

// disarmVimPageInteraction clears the active token. It reports whether a
// key-capturing interaction was active and the generation it belonged to.
func (wv *WebView) disarmVimPageInteraction() (bool, uint64) {
	wv.mu.Lock()
	defer wv.mu.Unlock()
	captured := wv.vimPageToken != "" && wv.vimPageKind.CapturesKeys()
	wv.vimPageToken = ""
	wv.vimPageKind = 0
	return captured, wv.vimPageGeneration
}

// notifyVimPageInteractionEnded runs the ended callback on GTK unless a newer
// interaction was armed in the meantime (keys queued ahead of the idle).
func (wv *WebView) notifyVimPageInteractionEnded(generation uint64) {
	wv.mu.RLock()
	cb := wv.callbacks
	wv.mu.RUnlock()
	if cb == nil || cb.OnVimPageInteractionEnded == nil {
		return
	}
	ended := cb.OnVimPageInteractionEnded
	wv.runOnGTK(func() {
		wv.mu.RLock()
		current := wv.vimPageGeneration
		wv.mu.RUnlock()
		if current == generation {
			ended()
		}
	})
}

// vimPageResultAllowed ties each result type to the interaction the user
// started, so a hijacked token only yields the kind of result requested.
func vimPageResultAllowed(kind dto.VimPageInteractionKind, messageType string) bool {
	switch messageType {
	case vimPageMessageEnd:
		return true
	case vimPageMessageOpenNew:
		return kind == dto.VimPageHintFollow || kind == dto.VimPageHintFollowNew
	case vimPageMessageCopy:
		return kind == dto.VimPageYankObject || kind == dto.VimPageHintYankURL || kind == dto.VimPageVisual
	default:
		return false
	}
}

// StartVimPageInteraction starts link hints, visual selection, or a text-object yank.
func (wv *WebView) StartVimPageInteraction(_ context.Context, request dto.VimPageInteractionRequest) error {
	if wv == nil || wv.destroyed.Load() {
		return errDestroyed
	}
	encoded, err := vimPageRequestJSON(request)
	if err != nil {
		return err
	}
	token, err := wv.armVimPageInteraction(request.Kind)
	if err != nil {
		return err
	}
	if err := wv.scheduleJavaScript(vimPageScript("start", token, encoded)); err != nil {
		wv.disarmVimPageInteraction()
		return err
	}
	return nil
}

// SendVimPageKey forwards one canonical key to the active page interaction.
func (wv *WebView) SendVimPageKey(_ context.Context, key string) error {
	if wv == nil || wv.destroyed.Load() {
		return errDestroyed
	}
	token := wv.currentVimPageToken()
	if key == "" || token == "" {
		return nil
	}
	return wv.scheduleJavaScript(vimPageScript("key", token, jsString(key)))
}

// CancelVimPageInteraction removes any hint overlay or visual selection.
func (wv *WebView) CancelVimPageInteraction(_ context.Context) error {
	if wv == nil || wv.destroyed.Load() {
		return errDestroyed
	}
	token := wv.currentVimPageToken()
	wv.disarmVimPageInteraction()
	if token == "" {
		return nil
	}
	return wv.scheduleJavaScript(vimPageScript("cancel", token))
}

// endVimPageInteractionOnNavigation drops an interaction whose document is
// being replaced; its runtime can no longer report the end itself.
func (wv *WebView) endVimPageInteractionOnNavigation() {
	if captured, generation := wv.disarmVimPageInteraction(); captured {
		wv.notifyVimPageInteractionEnded(generation)
	}
}

// handleVimPageBridge routes one runtime result from the trusted bridge.
func (e *Engine) handleVimPageBridge(browser purecef.Browser, payload vimPageBridgePayload) {
	e.withBridgeSourceWebView(browser, "", "vim-page", func(wv *WebView) {
		wv.handleVimPageResult(payload)
	})
}

// handleVimPageResult accepts a result only for the interaction the user
// armed; results from stale or forged interactions are dropped.
func (wv *WebView) handleVimPageResult(payload vimPageBridgePayload) {
	wv.mu.Lock()
	cb := wv.callbacks
	sourceURI := wv.uri
	armed := wv.vimPageToken
	kind := wv.vimPageKind
	generation := wv.vimPageGeneration
	valid := armed != "" && subtle.ConstantTimeCompare([]byte(armed), []byte(payload.Token)) == 1
	if valid {
		// Every runtime message is terminal: a token grants exactly one result.
		wv.vimPageToken = ""
		wv.vimPageKind = 0
	}
	wv.mu.Unlock()
	if !valid {
		logging.FromContext(wv.ctx).Debug().Str("type", payload.Type).Msg("cef: vim page result rejected — stale or unknown token")
		return
	}

	logging.FromContext(wv.ctx).Debug().
		Str("type", payload.Type).
		Str("reason", payload.Reason).
		Msg("cef: vim page interaction finished")
	if kind.CapturesKeys() {
		wv.notifyVimPageInteractionEnded(generation)
	}
	if !vimPageResultAllowed(kind, payload.Type) {
		logging.FromContext(wv.ctx).Debug().Str("type", payload.Type).Msg("cef: vim page result rejected — not allowed for interaction")
		return
	}

	switch payload.Type {
	case vimPageMessageCopy:
		if wv.engine != nil {
			wv.engine.handleExplicitClipboardBridgeText(wv.id, "copy", payload.Text)
		}
	case vimPageMessageOpenNew:
		if !isNavigableVimHintURL(payload.URL, sourceURI) {
			logging.FromContext(wv.ctx).Debug().Msg("cef: vim hint open rejected — unsupported url")
			return
		}
		if cb == nil || cb.OnLinkMiddleClick == nil {
			logging.FromContext(wv.ctx).Debug().Msg("cef: vim hint open skipped — handler not available")
			return
		}
		target := payload.URL
		wv.runOnGTK(func() { cb.OnLinkMiddleClick(target) })
	}
}
