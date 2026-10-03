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

// continueCEFJSDialog resolves a CEF dialog callback. It is a seam for tests.
var continueCEFJSDialog = continueCEFJSDialogOnUIThread

var _ purecef.JsdialogHandler = (*handlerSet)(nil)

// jsDialogCall is one in-flight dialog. done is guarded by jsDialogState.mu.
type jsDialogCall struct {
	callback purecef.JsdialogCallback
	done     bool
}

// jsDialogState tracks the single pending dialog of a WebView.
type jsDialogState struct {
	mu      sync.Mutex
	current *jsDialogCall
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
		// A dialog is already open: suppress rather than stack/spam.
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
	messageText string,
	isReload int32,
	callback purecef.JsdialogCallback,
) bool {
	if h == nil || h.wv == nil || callback == nil {
		return false
	}
	ui := h.wv.jsDialogUICallbacks()
	if ui == nil || ui.OnJSDialog == nil {
		return false
	}
	call, ok := h.wv.jsDialogs.begin(callback)
	if !ok {
		// Another dialog is open: stay on the page rather than stacking.
		continueCEFJSDialog(callback, false, "")
		return true
	}
	h.wv.presentJSDialog(call, port.JSDialogRequest{
		Type:     port.JSDialogBeforeUnload,
		Origin:   h.wv.URI(),
		Message:  messageText,
		IsReload: isReload != 0,
	}, ui)
	return true
}

// OnResetDialogState implements purecef.JsdialogHandler: CEF canceled pending
// dialogs (navigation, close). Close the UI and resolve the callback.
func (h *handlerSet) OnResetDialogState(_ purecef.Browser) {
	if h == nil || h.wv == nil {
		return
	}
	h.wv.cancelJSDialogs()
}

// OnDialogClosed implements purecef.JsdialogHandler. It intentionally does
// nothing: resolve already cleared the pending state, and a late notification
// must not clear a newer dialog.
func (h *handlerSet) OnDialogClosed(_ purecef.Browser) {}

// continueCEFJSDialogOnUIThread answers the callback on the CEF UI thread.
func continueCEFJSDialogOnUIThread(callback purecef.JsdialogCallback, ok bool, input string) {
	if callback == nil {
		return
	}
	success := int32(0)
	if ok {
		success = 1
	}
	if purecef.CurrentlyOn(purecef.ThreadIDTidUi) == 1 {
		callback.Cont(success, input)
		return
	}
	task := cefNewTask(cefTaskFunc(func() { callback.Cont(success, input) }))
	if cefPostTask(purecef.ThreadIDTidUi, task) != 1 {
		callback.Cont(success, input)
	}
}
