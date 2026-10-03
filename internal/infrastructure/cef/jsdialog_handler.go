package cef

import (
	"sync"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// CEF JavaScript dialog support (alert/confirm/prompt/beforeunload).
//
// With a nil JsdialogHandler CEF falls back to its built-in runner, which has
// no usable window in windowless (OSR) mode on Linux, so dialogs are never
// shown to the user. We therefore implement the handler, forward requests to
// the UI through port.WebViewCallbacks.OnJSDialog and answer the CEF callback
// exactly once.
//
// Anti-abuse: while a dialog is open for a WebView, further OnJsdialog requests
// are suppressed (suppress_message=true) instead of being queued, so a page
// cannot stack or spam dialogs.
//
// CEF reset semantics (javascript_dialog_manager.cc): OnResetDialogState is
// also delivered (a) right after a suppressed OnJsdialog and (b) at the end of
// every DialogClosed, i.e. inside/after each callback Cont. Those are not real
// resets and must not cancel the dialog the user is looking at, so
// jsDialogState carries a one-shot ignoreReset flag armed before suppressing,
// rejecting or continuing. Only WebView destruction and renderer termination
// cancel unconditionally.

// continueCEFJSDialog resolves a CEF dialog callback. It is a seam for tests.
// CEF's callback implementation re-posts to the CEF UI thread by itself.
var continueCEFJSDialog = func(callback purecef.JsdialogCallback, ok bool, input string) {
	success := int32(0)
	if ok {
		success = 1
	}
	callback.Cont(success, input)
}

var _ purecef.JsdialogHandler = (*handlerSet)(nil)

// jsDialogCall is one in-flight dialog. done is guarded by jsDialogState.mu.
type jsDialogCall struct {
	callback purecef.JsdialogCallback
	done     bool
}

// jsDialogState tracks the single pending dialog of a WebView.
type jsDialogState struct {
	mu          sync.Mutex
	current     *jsDialogCall
	ignoreReset bool // swallow the next OnResetDialogState (see file comment)
}

// armIgnoreReset makes the next OnResetDialogState a no-op.
func (s *jsDialogState) armIgnoreReset() {
	s.mu.Lock()
	s.ignoreReset = true
	s.mu.Unlock()
}

// consumeIgnoreReset reports (and clears) a pending ignoreReset.
func (s *jsDialogState) consumeIgnoreReset() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	was := s.ignoreReset
	s.ignoreReset = false
	return was
}

// begin registers a new pending dialog; false if one is already open.
func (s *jsDialogState) begin(cb purecef.JsdialogCallback) (*jsDialogCall, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != nil {
		return nil, false
	}
	call := &jsDialogCall{callback: cb}
	s.current = call
	// A new dialog means any reset owed by the previous one has been delivered
	// (the page is blocked until then); never carry a stale flag across.
	s.ignoreReset = false
	return call, true
}

func (s *jsDialogState) isDone(call *jsDialogCall) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return call.done
}

// resolve answers the CEF callback exactly once; later calls are no-ops.
func (s *jsDialogState) resolve(call *jsDialogCall, ok bool, input string) bool {
	s.mu.Lock()
	if call == nil || call.done {
		s.mu.Unlock()
		return false
	}
	call.done = true
	if s.current == call {
		s.current = nil
	}
	// Armed before Cont: CEF may deliver DialogClosed + OnResetDialogState
	// synchronously from inside it.
	s.ignoreReset = true
	s.mu.Unlock()
	continueCEFJSDialog(call.callback, ok, input)
	return true
}

// cancelPending resolves the pending dialog (if any) with a cancel.
func (s *jsDialogState) cancelPending() bool {
	s.mu.Lock()
	call := s.current
	s.mu.Unlock()
	if call == nil {
		return false
	}
	return s.resolve(call, false, "")
}

func (wv *WebView) jsDialogUICallbacks() *port.WebViewCallbacks {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.callbacks
}

// cancelJSDialogs resolves any pending dialog and tells the UI to hide it.
func (wv *WebView) cancelJSDialogs() {
	if wv == nil {
		return
	}
	if !wv.jsDialogs.cancelPending() {
		return
	}
	if cb := wv.jsDialogUICallbacks(); cb != nil && cb.OnJSDialogReset != nil {
		wv.runOnGTK(cb.OnJSDialogReset)
	}
}

func jsDialogTypeFromCEF(t purecef.JsdialogType) (port.JSDialogType, bool) {
	switch t {
	case purecef.JsdialogTypeJsdialogtypeAlert:
		return port.JSDialogAlert, true
	case purecef.JsdialogTypeJsdialogtypeConfirm:
		return port.JSDialogConfirm, true
	case purecef.JsdialogTypeJsdialogtypePrompt:
		return port.JSDialogPrompt, true
	default:
		return 0, false
	}
}

// present shows the dialog on the GTK thread and wires the user's answer back.
func (wv *WebView) presentJSDialog(call *jsDialogCall, req port.JSDialogRequest, ui *port.WebViewCallbacks) {
	wv.runOnGTK(func() {
		if wv.jsDialogs.isDone(call) {
			return // canceled before the UI got to run
		}
		respond := func(ok bool, input string) {
			if req.Type != port.JSDialogPrompt {
				input = ""
			}
			wv.jsDialogs.resolve(call, ok, input)
		}
		if !ui.OnJSDialog(req, respond) {
			respond(false, "")
		}
	})
}

// OnJsdialog implements purecef.JsdialogHandler.
func (h *handlerSet) OnJsdialog(
	_ purecef.Browser,
	originURL string,
	dialogType purecef.JsdialogType,
	messageText string,
	defaultPromptText string,
	callback purecef.JsdialogCallback,
	suppressMessage *int32,
) int32 {
	if h == nil || h.wv == nil || callback == nil {
		return 0
	}
	if h.wv.destroyed.Load() {
		// Shutting down: the GTK loop may be blocked. Let CEF dismiss it.
		return 0
	}
	typ, ok := jsDialogTypeFromCEF(dialogType)
	if !ok {
		return 0
	}
	ui := h.wv.jsDialogUICallbacks()
	if ui == nil || ui.OnJSDialog == nil {
		return 0 // no UI: let CEF apply its default behavior
	}
	call, ok := h.wv.jsDialogs.begin(callback)
	if !ok {
		// A dialog is already open: suppress rather than stack/spam. CEF will
		// follow up with OnResetDialogState, which must not cancel that dialog.
		h.wv.jsDialogs.armIgnoreReset()
		if suppressMessage != nil {
			*suppressMessage = 1
		}
		return 0
	}
	logging.FromContext(h.wv.ctx).Debug().Int32("type", dialogType).Msg("cef: js dialog requested")
	h.wv.presentJSDialog(call, port.JSDialogRequest{
		Type:          typ,
		Origin:        originURL,
		Message:       messageText,
		DefaultPrompt: defaultPromptText,
	}, ui)
	return 1
}

// OnBeforeUnloadDialog implements purecef.JsdialogHandler.
func (h *handlerSet) OnBeforeUnloadDialog(
	_ purecef.Browser,
	_ string, // CEF always passes a fixed string; the UI shows Dumber's own text.
	isReload int32,
	callback purecef.JsdialogCallback,
) bool {
	if h == nil || h.wv == nil || callback == nil {
		return false
	}
	if h.wv.destroyed.Load() {
		// CloseBrowser dispatches beforeunload; accept it instead of waiting
		// on UI that may never run. CEF accepts the unload when we return false.
		return false
	}
	ui := h.wv.jsDialogUICallbacks()
	if ui == nil || ui.OnJSDialog == nil {
		return false
	}
	call, ok := h.wv.jsDialogs.begin(callback)
	if !ok {
		// Another dialog is open: stay on the page rather than stacking.
		// The Cont below makes CEF send a reset that must not cancel it.
		h.wv.jsDialogs.armIgnoreReset()
		continueCEFJSDialog(callback, false, "")
		return true
	}
	h.wv.presentJSDialog(call, port.JSDialogRequest{
		Type:     port.JSDialogBeforeUnload,
		Origin:   h.wv.URI(),
		IsReload: isReload != 0,
	}, ui)
	return true
}

// OnResetDialogState implements purecef.JsdialogHandler: CEF canceled pending
// dialogs (navigation, close). Close the UI and resolve the callback, unless
// this is the echo of a suppress/reject/Cont (see file comment).
//
// The UI reset (OnJSDialogReset) carries no dialog identity: it hides whatever
// the pane shows. That is safe because a pane shows at most one dialog and a
// new one is only presented after the previous call was resolved.
func (h *handlerSet) OnResetDialogState(_ purecef.Browser) {
	if h == nil || h.wv == nil {
		return
	}
	if h.wv.jsDialogs.consumeIgnoreReset() {
		return
	}
	h.wv.cancelJSDialogs()
}

// OnDialogClosed implements purecef.JsdialogHandler. It intentionally does
// nothing: resolve already cleared the pending state, and a late notification
// must not clear a newer dialog.
func (h *handlerSet) OnDialogClosed(_ purecef.Browser) {}
