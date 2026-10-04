package cef

import (
	"context"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

var _ port.VimPageInteractor = (*WebView)(nil)

//go:embed vim_page_runtime.js
var vimPageRuntimeTemplateJS string

var (
	errVimPageTokenUnavailable = errors.New("vim page: interaction token unavailable")
	errVimPagePayloadTooLarge  = errors.New("vim page: payload too large")
	// vimPageAccentColor admits the CSS hex colors the runtime paints with;
	// anything else is replaced by the runtime default before reaching JS.
	vimPageAccentColor = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
)

// vimPageBridgePayload is one result reported by the page runtime. Token is
// the per-interaction secret issued by StartVimPageInteraction.
type vimPageBridgePayload struct {
	Type  string `json:"type"`
	Token string `json:"token"`
	Text  string `json:"text,omitempty"`
	URL   string `json:"url,omitempty"`
	// Reason explains an early end (for example no visible targets).
	Reason string `json:"reason,omitempty"`
	// Mode names the sub-mode of a "mode" message.
	Mode string `json:"mode,omitempty"`
}

const (
	vimPageMessageCopy    = "copy"
	vimPageMessageMode    = "mode"
	vimPageMessageOpenNew = "open-new"
	vimPageMessageEnd     = "end"
)

func decodeVimPageBridgePayload(body []byte) (vimPageBridgePayload, error) {
	var payload vimPageBridgePayload
	if len(body) > maxClipboardBytes {
		return payload, errVimPagePayloadTooLarge
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
	case vimPageMessageMode:
		if !dto.VimPageMode(payload.Mode).Valid() {
			return payload, fmt.Errorf("unknown mode %q", payload.Mode)
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
	color := request.HighlightColor
	if !vimPageAccentColor.MatchString(color) {
		color = ""
	}
	encoded, err := json.Marshal(struct {
		Kind   string `json:"kind"`
		Object string `json:"object,omitempty"`
		Color  string `json:"color,omitempty"`
	}{Kind: kind, Object: object, Color: color})
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

// vimPageStartScript installs the runtime when absent and starts an
// interaction. The token is a per-interaction secret, not the shared bridge
// nonce, so a page that intercepts the call only learns a token valid for the
// interaction the user just started.
func vimPageStartScript(token, request string) string {
	return vimPageRuntimeTemplateJS + "\nwindow.__dumberVimPage.start(" + jsString(token) + ", " + request + ");"
}

// vimPageCallScript invokes key or cancel on the installed runtime without
// shipping it again. When the runtime is gone (the document was replaced), it
// reports the end of the token so Go releases the key capture.
func vimPageCallScript(method, token string, args ...string) string {
	call := append([]string{jsString(token)}, args...)
	return "window.__dumberVimPage ? window.__dumberVimPage." + method + "(" + strings.Join(call, ", ") + ") : " + vimPageEndScript(token)
}

// vimPageEndScript posts an end message for token over the same bridge the
// runtime uses.
func vimPageEndScript(token string) string {
	body, _ := json.Marshal(map[string]string{"token": token, "type": vimPageMessageEnd})
	return `window.fetch("dumb:///api/vim-page", {method: "POST", headers: {"X-Dumber-Body": ` +
		jsString(base64.StdEncoding.EncodeToString(body)) + `}}).catch(() => {});`
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
	wv.vimPageCopyAllowance = 0
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
	wv.vimPageCopyAllowance = 0
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

// vimPageResultEndsInteraction reports whether a result spends the token.
// The caret and visual flow is long-lived: its mode changes and copies leave
// the interaction running so copies can be chained. Copies stay bounded by
// the user's own y/Enter presses (see vimPageCopyAllowance), not by the
// token. Every other result is terminal, so a token grants exactly one.
func vimPageResultEndsInteraction(kind dto.VimPageInteractionKind, messageType string) bool {
	if kind != dto.VimPageVisual {
		return true
	}
	return messageType != vimPageMessageMode && messageType != vimPageMessageCopy
}

// notifyVimPageModeChanged reports a sub-mode change on GTK unless a newer
// interaction was armed in the meantime.
func (wv *WebView) notifyVimPageModeChanged(generation uint64, mode dto.VimPageMode) {
	wv.mu.RLock()
	cb := wv.callbacks
	wv.mu.RUnlock()
	if cb == nil || cb.OnVimPageModeChanged == nil {
		return
	}
	changed := cb.OnVimPageModeChanged
	wv.runOnGTK(func() {
		wv.mu.RLock()
		current := wv.vimPageGeneration
		wv.mu.RUnlock()
		if current == generation {
			changed(mode)
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
	case vimPageMessageMode:
		return kind == dto.VimPageVisual
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
	if err := wv.scheduleJavaScript(vimPageStartScript(token, encoded)); err != nil {
		wv.disarmVimPageInteraction()
		return err
	}
	return nil
}

// vimPageKeyRequestsCopy reports whether a forwarded key may make a visual
// selection copy: y and Enter yank in the caret and visual flow.
func vimPageKeyRequestsCopy(key string) bool {
	return key == "y" || key == "<Return>"
}

// grantVimPageCopyAllowance records that the user pressed a copy key for the
// armed visual interaction, so one visual copy result will be accepted. The
// allowance is capped at one: y/Enter presses the page ignores (caret or hint
// phase, empty selection) must not bank copies for a page holding the token.
func (wv *WebView) grantVimPageCopyAllowance(token, key string) {
	if !vimPageKeyRequestsCopy(key) {
		return
	}
	wv.mu.Lock()
	if wv.vimPageToken == token && wv.vimPageKind == dto.VimPageVisual {
		wv.vimPageCopyAllowance = 1
	}
	wv.mu.Unlock()
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
	wv.grantVimPageCopyAllowance(token, key)
	return wv.scheduleJavaScript(vimPageCallScript("key", token, jsString(key)))
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
	return wv.scheduleJavaScript(vimPageCallScript("cancel", token))
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

// spendVimPageResultLocked validates the token of one result and applies its
// effect on the armed state: a terminal result disarms the interaction, and a
// visual copy spends one allowance. A visual copy is non-terminal, so the
// token alone would let a page that read it write the clipboard repeatedly;
// each accepted copy needs a y/Enter key the user pressed. wv.mu must be held.
func (wv *WebView) spendVimPageResultLocked(payload vimPageBridgePayload) (valid, ends, copyDenied bool) {
	armed := wv.vimPageToken
	valid = armed != "" && subtle.ConstantTimeCompare([]byte(armed), []byte(payload.Token)) == 1
	if !valid {
		return false, false, false
	}
	kind := wv.vimPageKind
	if payload.Type == vimPageMessageCopy && kind == dto.VimPageVisual {
		if wv.vimPageCopyAllowance <= 0 {
			return true, false, true
		}
		wv.vimPageCopyAllowance--
	}
	ends = vimPageResultEndsInteraction(kind, payload.Type)
	if ends {
		wv.vimPageToken = ""
		wv.vimPageKind = 0
		wv.vimPageCopyAllowance = 0
	}
	return true, ends, false
}

// handleVimPageResult accepts a result only for the interaction the user
// armed; results from stale or forged interactions are dropped.
func (wv *WebView) handleVimPageResult(payload vimPageBridgePayload) {
	if wv == nil || wv.destroyed.Load() {
		return
	}
	wv.mu.Lock()
	cb := wv.callbacks
	sourceURI := wv.uri
	kind := wv.vimPageKind
	generation := wv.vimPageGeneration
	valid, ends, copyDenied := wv.spendVimPageResultLocked(payload)
	wv.mu.Unlock()
	if !valid {
		logging.FromContext(wv.ctx).Debug().Str("type", payload.Type).Msg("cef: vim page result rejected — stale or unknown token")
		return
	}
	if copyDenied {
		logging.FromContext(wv.ctx).Debug().Msg("cef: vim page copy rejected — no user copy key pending")
		return
	}

	if ends {
		logging.FromContext(wv.ctx).Debug().
			Str("type", payload.Type).
			Str("reason", payload.Reason).
			Msg("cef: vim page interaction finished")
		if kind.CapturesKeys() {
			wv.notifyVimPageInteractionEnded(generation)
		}
	}
	if !vimPageResultAllowed(kind, payload.Type) {
		logging.FromContext(wv.ctx).Debug().Str("type", payload.Type).Msg("cef: vim page result rejected — not allowed for interaction")
		return
	}

	switch payload.Type {
	case vimPageMessageMode:
		wv.notifyVimPageModeChanged(generation, dto.VimPageMode(payload.Mode))
	case vimPageMessageCopy:
		// Results arrive on the CEF IO thread; the clipboard write belongs on GTK.
		if engine := wv.engine; engine != nil {
			id, text := wv.id, payload.Text
			wv.runOnGTK(func() { engine.handleExplicitClipboardBridgeText(id, "copy", text) })
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
