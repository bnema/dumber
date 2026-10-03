package cef

import (
	"context"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
)

type stubJSDialogCallback struct {
	calls []jsDialogAnswer
	// onCont mimics CEF's DialogClosed ordering: OnDialogClosed and
	// OnResetDialogState are delivered from inside Cont.
	onCont func()
}

type jsDialogAnswer struct {
	ok    bool
	input string
}

func (s *stubJSDialogCallback) Cont(success int32, userInput string) {
	s.calls = append(s.calls, jsDialogAnswer{ok: success != 0, input: userInput})
	if s.onCont != nil {
		s.onCont()
	}
}

// echoCEFClose makes cb deliver CEF's DialogClosed + reset echo from Cont.
func echoCEFClose(h *handlerSet, cb *stubJSDialogCallback) {
	cb.onCont = func() {
		h.OnDialogClosed(nil)
		h.OnResetDialogState(nil)
	}
}

// useDirectJSDialogContinue answers stub callbacks inline (no CEF UI thread).
func useDirectJSDialogContinue(t *testing.T) {
	t.Helper()
	prev := continueCEFJSDialog
	continueCEFJSDialog = func(cb purecef.JsdialogCallback, ok bool, input string) {
		success := int32(0)
		if ok {
			success = 1
		}
		cb.Cont(success, input)
	}
	t.Cleanup(func() { continueCEFJSDialog = prev })
}

type jsDialogUIRecorder struct {
	reqs    []port.JSDialogRequest
	respond []func(bool, string)
	resets  int
	handled bool
}

func newJSDialogWebView(ui *jsDialogUIRecorder) *WebView {
	return &WebView{
		ctx: context.Background(),
		callbacks: &port.WebViewCallbacks{
			OnJSDialog: func(req port.JSDialogRequest, respond func(bool, string)) bool {
				ui.reqs = append(ui.reqs, req)
				ui.respond = append(ui.respond, respond)
				return ui.handled
			},
			OnJSDialogReset: func() { ui.resets++ },
		},
	}
}

func TestGetJsdialogHandlerEnabled(t *testing.T) {
	h := &handlerSet{}
	require.Same(t, h, h.GetJsdialogHandler())
}

func TestJSDialogTypeMapping(t *testing.T) {
	cases := map[purecef.JsdialogType]port.JSDialogType{
		purecef.JsdialogTypeJsdialogtypeAlert:   port.JSDialogAlert,
		purecef.JsdialogTypeJsdialogtypeConfirm: port.JSDialogConfirm,
		purecef.JsdialogTypeJsdialogtypePrompt:  port.JSDialogPrompt,
	}
	for in, want := range cases {
		got, ok := jsDialogTypeFromCEF(in)
		require.True(t, ok)
		require.Equal(t, want, got)
	}
	_, ok := jsDialogTypeFromCEF(purecef.JsdialogTypeJsdialogtypeNumValues)
	require.False(t, ok)
}

func TestOnJsdialogForwardsRequestAndRespondsOnce(t *testing.T) {
	useDirectJSDialogContinue(t)
	ui := &jsDialogUIRecorder{handled: true}
	h := &handlerSet{wv: newJSDialogWebView(ui)}
	cb := &stubJSDialogCallback{}
	var suppress int32

	handled := h.OnJsdialog(nil, "https://example.com/p", purecef.JsdialogTypeJsdialogtypePrompt, "Name?", "bob", cb, &suppress)

	require.Equal(t, int32(1), handled)
	require.Zero(t, suppress)
	require.Equal(t, []port.JSDialogRequest{{
		Type: port.JSDialogPrompt, Origin: "https://example.com/p", Message: "Name?", DefaultPrompt: "bob",
	}}, ui.reqs)
	require.Empty(t, cb.calls)

	ui.respond[0](true, "alice")
	ui.respond[0](false, "late") // ignored: exactly once
	require.Equal(t, []jsDialogAnswer{{ok: true, input: "alice"}}, cb.calls)
}

func TestOnJsdialogDropsInputForNonPrompt(t *testing.T) {
	useDirectJSDialogContinue(t)
	ui := &jsDialogUIRecorder{handled: true}
	h := &handlerSet{wv: newJSDialogWebView(ui)}
	cb := &stubJSDialogCallback{}

	h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeConfirm, "ok?", "", cb, new(int32))
	ui.respond[0](true, "ignored")

	require.Equal(t, []jsDialogAnswer{{ok: true}}, cb.calls)
}

func TestOnJsdialogWithoutUICallbackUsesDefault(t *testing.T) {
	cb := &stubJSDialogCallback{}
	var suppress int32

	for _, h := range []*handlerSet{
		nil,
		{},
		{wv: &WebView{ctx: context.Background()}},
		{wv: &WebView{ctx: context.Background(), callbacks: &port.WebViewCallbacks{}}},
	} {
		require.Zero(t, h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeAlert, "m", "", cb, &suppress))
		require.False(t, h.OnBeforeUnloadDialog(nil, "m", 0, cb))
	}
	require.Zero(t, suppress)
	require.Empty(t, cb.calls)
}

func TestOnJsdialogRejectsNilCallbackAndUnknownType(t *testing.T) {
	ui := &jsDialogUIRecorder{handled: true}
	h := &handlerSet{wv: newJSDialogWebView(ui)}

	require.Zero(t, h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeAlert, "m", "", nil, new(int32)))
	require.Zero(t, h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeNumValues, "m", "", &stubJSDialogCallback{}, new(int32)))
	require.Empty(t, ui.reqs)
}

func TestOnJsdialogUINotHandledCancels(t *testing.T) {
	useDirectJSDialogContinue(t)
	ui := &jsDialogUIRecorder{handled: false}
	h := &handlerSet{wv: newJSDialogWebView(ui)}
	cb := &stubJSDialogCallback{}

	require.Equal(t, int32(1), h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeConfirm, "m", "", cb, new(int32)))
	require.Equal(t, []jsDialogAnswer{{ok: false}}, cb.calls)
}

func TestOnJsdialogSuppressesWhileDialogOpen(t *testing.T) {
	useDirectJSDialogContinue(t)
	ui := &jsDialogUIRecorder{handled: true}
	h := &handlerSet{wv: newJSDialogWebView(ui)}
	first := &stubJSDialogCallback{}
	second := &stubJSDialogCallback{}
	var suppress int32

	require.Equal(t, int32(1), h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeAlert, "1", "", first, new(int32)))
	require.Zero(t, h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeAlert, "2", "", second, &suppress))
	require.Equal(t, int32(1), suppress)
	require.Len(t, ui.reqs, 1)
	require.Empty(t, second.calls)

	// Once answered, new dialogs are allowed again.
	ui.respond[0](true, "")
	require.Equal(t, int32(1), h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeAlert, "3", "", second, new(int32)))
	require.Len(t, ui.reqs, 2)
}

func TestOnBeforeUnloadDialog(t *testing.T) {
	useDirectJSDialogContinue(t)
	ui := &jsDialogUIRecorder{handled: true}
	wv := newJSDialogWebView(ui)
	wv.uri = "https://example.com/edit"
	h := &handlerSet{wv: wv}
	cb := &stubJSDialogCallback{}

	require.True(t, h.OnBeforeUnloadDialog(nil, "unsaved", 1, cb))
	// CEF's fixed message text is not forwarded.
	require.Equal(t, []port.JSDialogRequest{{
		Type: port.JSDialogBeforeUnload, Origin: "https://example.com/edit", IsReload: true,
	}}, ui.reqs)

	// A second beforeunload while open is answered "stay" immediately.
	other := &stubJSDialogCallback{}
	require.True(t, h.OnBeforeUnloadDialog(nil, "again", 0, other))
	require.Equal(t, []jsDialogAnswer{{ok: false}}, other.calls)
	require.Len(t, ui.reqs, 1)

	ui.respond[0](true, "")
	require.Equal(t, []jsDialogAnswer{{ok: true}}, cb.calls)
}

func TestOnResetDialogStateResolvesPendingAndClosesUI(t *testing.T) {
	useDirectJSDialogContinue(t)
	ui := &jsDialogUIRecorder{handled: true}
	h := &handlerSet{wv: newJSDialogWebView(ui)}
	cb := &stubJSDialogCallback{}
	h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypePrompt, "m", "d", cb, new(int32))

	h.OnResetDialogState(nil)

	require.Equal(t, []jsDialogAnswer{{ok: false}}, cb.calls)
	require.Equal(t, 1, ui.resets)

	// A late UI answer after reset must not resolve twice.
	ui.respond[0](true, "late")
	require.Len(t, cb.calls, 1)

	// Reset with nothing pending is a no-op and does not touch the UI.
	h.OnResetDialogState(nil)
	require.Equal(t, 1, ui.resets)
	h.OnDialogClosed(nil)
}

func TestDestroyCancelsPendingJSDialog(t *testing.T) {
	useDirectJSDialogContinue(t)
	ui := &jsDialogUIRecorder{handled: true}
	wv := newJSDialogWebView(ui)
	h := &handlerSet{wv: wv}
	cb := &stubJSDialogCallback{}
	h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeAlert, "m", "", cb, new(int32))

	wv.Destroy()

	require.Equal(t, []jsDialogAnswer{{ok: false}}, cb.calls)
	require.Equal(t, 1, ui.resets)
}

func TestJSDialogCancelledBeforeUIRunsIsNotShown(t *testing.T) {
	useDirectJSDialogContinue(t)
	ui := &jsDialogUIRecorder{handled: true}
	wv := newJSDialogWebView(ui)
	cb := &stubJSDialogCallback{}
	call, begin := wv.jsDialogs.begin(cb)
	require.Equal(t, jsDialogBeginOK, begin)
	wv.jsDialogs.resolve(call, false, "")

	wv.presentJSDialog(call, port.JSDialogRequest{}, wv.callbacks)

	require.Empty(t, ui.reqs)
	require.Len(t, cb.calls, 1)
}

func TestResetEchoAfterSuppressDoesNotCancelOpenDialog(t *testing.T) {
	useDirectJSDialogContinue(t)
	ui := &jsDialogUIRecorder{handled: true}
	h := &handlerSet{wv: newJSDialogWebView(ui)}
	first := &stubJSDialogCallback{}
	h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeAlert, "1", "", first, new(int32))

	// A spammer is suppressed; CEF then sends OnResetDialogState.
	h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeAlert, "2", "", &stubJSDialogCallback{}, new(int32))
	h.OnResetDialogState(nil)

	require.Empty(t, first.calls, "dialog #1 must stay pending")
	require.Zero(t, ui.resets)

	// A real reset afterwards still cancels it.
	h.OnResetDialogState(nil)
	require.Equal(t, []jsDialogAnswer{{ok: false}}, first.calls)
	require.Equal(t, 1, ui.resets)
}

func TestResetEchoAfterRejectedBeforeUnloadDoesNotCancelOpenDialog(t *testing.T) {
	useDirectJSDialogContinue(t)
	ui := &jsDialogUIRecorder{handled: true}
	h := &handlerSet{wv: newJSDialogWebView(ui)}
	first := &stubJSDialogCallback{}
	h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeConfirm, "1", "", first, new(int32))

	other := &stubJSDialogCallback{}
	echoCEFClose(h, other)
	require.True(t, h.OnBeforeUnloadDialog(nil, "", 0, other))

	require.Empty(t, first.calls)
	require.Zero(t, ui.resets)
}

func TestCEFCloseEchoDoesNotCancelNewerDialog(t *testing.T) {
	useDirectJSDialogContinue(t)
	ui := &jsDialogUIRecorder{handled: true}
	h := &handlerSet{wv: newJSDialogWebView(ui)}
	first := &stubJSDialogCallback{}
	echoCEFClose(h, first)
	h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeAlert, "1", "", first, new(int32))

	ui.respond[0](true, "") // Cont -> OnDialogClosed + OnResetDialogState (echo)
	require.Equal(t, []jsDialogAnswer{{ok: true}}, first.calls)
	require.Zero(t, ui.resets, "echo must not hide the UI")

	// The echo consumed the flag, so a genuine reset on a newer dialog cancels it.
	second := &stubJSDialogCallback{}
	h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeAlert, "2", "", second, new(int32))
	h.OnResetDialogState(nil)
	require.Equal(t, []jsDialogAnswer{{ok: false}}, second.calls)
}

func TestDeferredResetEchoIsConsumed(t *testing.T) {
	useDirectJSDialogContinue(t)
	ui := &jsDialogUIRecorder{handled: true}
	h := &handlerSet{wv: newJSDialogWebView(ui)}
	first := &stubJSDialogCallback{} // no synchronous echo
	h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeAlert, "1", "", first, new(int32))
	ui.respond[0](true, "")

	// The echo arrives later, while no dialog is open: consumed, UI untouched.
	h.OnDialogClosed(nil)
	h.OnResetDialogState(nil)
	require.Zero(t, ui.resets)
	require.Len(t, first.calls, 1)
	require.False(t, h.wv.jsDialogs.consumeIgnoreReset(), "flag must have been consumed")
}

func TestMainFrameLoadStartCancelsPendingDialog(t *testing.T) {
	useDirectJSDialogContinue(t)
	ui := &jsDialogUIRecorder{handled: true}
	wv := newJSDialogWebView(ui)
	h := &handlerSet{wv: wv}
	first := &stubJSDialogCallback{}
	h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeAlert, "1", "", first, new(int32))
	// A suppressed spammer makes CEF drop its handler_, so no reset will follow.
	h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeAlert, "2", "", &stubJSDialogCallback{}, new(int32))

	h.OnLoadStart(nil, stubFrame{main: false, url: "https://sub.example.com"}, 0)
	require.Empty(t, first.calls, "subframe loads must not cancel")

	h.OnLoadStart(nil, stubFrame{main: true, url: "https://example.com/next"}, 0)
	require.Equal(t, []jsDialogAnswer{{ok: false}}, first.calls)
	require.Equal(t, 1, ui.resets)
}

func TestDestroyedStateRejectsNewDialog(t *testing.T) {
	useDirectJSDialogContinue(t)
	ui := &jsDialogUIRecorder{handled: true}
	wv := newJSDialogWebView(ui)
	h := &handlerSet{wv: wv}
	wv.cancelJSDialogsClosing(true) // Destroy raced ahead of the destroyed check
	cb := &stubJSDialogCallback{}
	var suppress int32

	require.Zero(t, h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeAlert, "m", "", cb, &suppress))
	require.False(t, h.OnBeforeUnloadDialog(nil, "", 0, cb))
	require.Zero(t, suppress)
	require.Empty(t, ui.reqs)
}

func TestNoStaleIgnoreAcrossDialogs(t *testing.T) {
	useDirectJSDialogContinue(t)
	ui := &jsDialogUIRecorder{handled: true}
	h := &handlerSet{wv: newJSDialogWebView(ui)}
	// Answered without a CEF echo (e.g. reset-then-late-answer path).
	first := &stubJSDialogCallback{}
	h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeAlert, "1", "", first, new(int32))
	ui.respond[0](true, "")

	second := &stubJSDialogCallback{}
	h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeAlert, "2", "", second, new(int32))
	h.OnResetDialogState(nil)

	require.Equal(t, []jsDialogAnswer{{ok: false}}, second.calls, "real reset must cancel the new dialog")
}

func TestDestroyedWebViewDoesNotShowDialogs(t *testing.T) {
	ui := &jsDialogUIRecorder{handled: true}
	wv := newJSDialogWebView(ui)
	wv.destroyed.Store(true)
	h := &handlerSet{wv: wv}
	cb := &stubJSDialogCallback{}
	var suppress int32

	require.Zero(t, h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeAlert, "m", "", cb, &suppress))
	require.False(t, h.OnBeforeUnloadDialog(nil, "", 0, cb))
	require.Zero(t, suppress)
	require.Empty(t, ui.reqs)
	require.Empty(t, cb.calls)
}

func TestRenderProcessTerminatedCancelsPendingJSDialog(t *testing.T) {
	useDirectJSDialogContinue(t)
	ui := &jsDialogUIRecorder{handled: true}
	wv := newJSDialogWebView(ui)
	h := &handlerSet{wv: wv}
	cb := &stubJSDialogCallback{}
	h.OnJsdialog(nil, "", purecef.JsdialogTypeJsdialogtypeAlert, "m", "", cb, new(int32))

	h.OnRenderProcessTerminated(nil, purecef.TerminationStatusTsProcessCrashed, 0, "")

	require.Equal(t, []jsDialogAnswer{{ok: false}}, cb.calls)
	require.Equal(t, 1, ui.resets)
}
