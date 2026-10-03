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
}

type jsDialogAnswer struct {
	ok    bool
	input string
}

func (s *stubJSDialogCallback) Cont(success int32, userInput string) {
	s.calls = append(s.calls, jsDialogAnswer{ok: success != 0, input: userInput})
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
	require.Equal(t, []port.JSDialogRequest{{
		Type: port.JSDialogBeforeUnload, Origin: "https://example.com/edit", Message: "unsaved", IsReload: true,
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

	wv.cancelJSDialogs()

	require.Equal(t, []jsDialogAnswer{{ok: false}}, cb.calls)
	require.Equal(t, 1, ui.resets)
}

func TestJSDialogCancelledBeforeUIRunsIsNotShown(t *testing.T) {
	useDirectJSDialogContinue(t)
	ui := &jsDialogUIRecorder{handled: true}
	wv := newJSDialogWebView(ui)
	cb := &stubJSDialogCallback{}
	call, ok := wv.jsDialogs.begin(cb)
	require.True(t, ok)
	wv.jsDialogs.resolve(call, false, "")

	wv.presentJSDialog(call, port.JSDialogRequest{}, wv.callbacks)

	require.Empty(t, ui.reqs)
	require.Len(t, cb.calls, 1)
}
