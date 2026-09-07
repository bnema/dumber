package cef

import (
	"context"
	"sync"
	"testing"

	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// injectionHarness builds a WebView with a recording main frame. Engine is
// nil so RunJavaScript executes synchronously without GTK dispatch.
type injectionHarness struct {
	wv      *WebView
	browser *cefmocks.MockBrowser
	mu      sync.Mutex
	scripts []string
}

func newInjectionHarness(t *testing.T, uri string) *injectionHarness {
	t.Helper()
	h := &injectionHarness{}
	frame := cefmocks.NewMockFrame(t)
	frame.EXPECT().ExecuteJavaScript(mock.Anything, "", int32(0)).Run(func(script string, _ string, _ int32) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.scripts = append(h.scripts, script)
	}).Maybe()
	h.browser = cefmocks.NewMockBrowser(t)
	h.browser.EXPECT().GetMainFrame().Return(frame).Maybe()
	h.wv = &WebView{ctx: context.Background(), browser: h.browser}
	h.wv.uri = uri
	return h
}

func (h *injectionHarness) scriptCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.scripts)
}

func testInjector() *contentInjector {
	return &contentInjector{themeCSS: ":root{--t:1}", findHighlightCSS: ".find{--f:1}"}
}

// TestInjectionEvent_ExternalPageScriptSet counts the installation for an
// external document: scrollbar styling, its auto-hide behavior, the
// clipboard bridge, plus configured find CSS. No dark-mode or message
// bridge shim belongs to external pages.
func TestInjectionEvent_ExternalPageScriptSet(t *testing.T) {
	h := newInjectionHarness(t, "https://example.com/page")
	testInjector().onLoadEndForEvent(h.wv, h.wv.captureInjectionEvent())

	require.Equal(t, 4, h.scriptCount(), "external install must stay a fixed small script set")
	h.mu.Lock()
	defer h.mu.Unlock()
	joined := ""
	for _, script := range h.scripts {
		joined += script
	}
	require.Contains(t, joined, "dumber-scrollbar")
	require.Contains(t, joined, "dumber-find-highlight")
	require.NotContains(t, joined, "dumber-theme-vars")
	require.NotContains(t, joined, "__dumber_cef_prefers_dark")
}

// TestInjectionEvent_InternalPageScriptSet counts the installation for an
// internal document, including dark-mode handling and the message bridge.
func TestInjectionEvent_InternalPageScriptSet(t *testing.T) {
	h := newInjectionHarness(t, "dumb://home")
	testInjector().onLoadEndForEvent(h.wv, h.wv.captureInjectionEvent())

	require.Equal(t, 7, h.scriptCount(), "internal install must stay a fixed small script set")
}

// TestInjectionEvent_StaleBrowserSkipped reproduces an old main frame firing
// during process swap: the GTK-dispatched callback must install nothing.
func TestInjectionEvent_StaleBrowserSkipped(t *testing.T) {
	h := newInjectionHarness(t, "https://example.com/a")
	event := h.wv.captureInjectionEvent()

	replacement := cefmocks.NewMockBrowser(t)
	h.wv.mu.Lock()
	h.wv.browser = replacement
	h.wv.mu.Unlock()

	testInjector().onLoadEndForEvent(h.wv, event)
	require.Zero(t, h.scriptCount(), "old-frame event must not install against the replacement browser")
}

// TestInjectionEvent_StaleIntentSkipped reproduces a queued callback
// outlived by a newer navigation: installation must not run for the
// superseded intent.
func TestInjectionEvent_StaleIntentSkipped(t *testing.T) {
	h := newInjectionHarness(t, "https://example.com/a")
	event := h.wv.captureInjectionEvent()

	h.wv.mu.Lock()
	h.wv.pendingIntentID++
	h.wv.mu.Unlock()

	testInjector().onLoadEndForEvent(h.wv, event)
	require.Zero(t, h.scriptCount(), "superseded intent must not install")
}

// TestInjectionEvent_CurrentInstalls verifies matching identity installs
// exactly as the legacy path, including repeated same-document load-ends
// whose scripts are idempotent by element ID.
func TestInjectionEvent_CurrentInstalls(t *testing.T) {
	h := newInjectionHarness(t, "https://example.com/a")
	ci := testInjector()

	ci.onLoadEndForEvent(h.wv, h.wv.captureInjectionEvent())
	first := h.scriptCount()
	require.Positive(t, first)

	ci.onLoadEndForEvent(h.wv, h.wv.captureInjectionEvent())
	require.Equal(t, 2*first, h.scriptCount(), "repeated same-document events reinstall idempotent scripts")
}

// TestInjectionEvent_NilCaptureFallsBackToLegacy verifies the conservative
// fallback: a capture without a browser cannot prove staleness and installs
// as before.
func TestInjectionEvent_NilCaptureFallsBackToLegacy(t *testing.T) {
	h := newInjectionHarness(t, "https://example.com/a")
	testInjector().onLoadEndForEvent(h.wv, injectionEvent{})
	require.Positive(t, h.scriptCount())
}
