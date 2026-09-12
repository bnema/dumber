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
	return newInjectionHarnessWithID(t, uri, 7)
}

func newInjectionHarnessWithID(t *testing.T, uri string, browserID int32) *injectionHarness {
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
	h.browser.EXPECT().GetIdentifier().Return(browserID).Maybe()
	h.wv = &WebView{ctx: context.Background(), browser: h.browser, documentSeq: 1}
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
	testInjector().onLoadEndForEvent(h.wv, h.wv.captureInjectionEvent(h.browser, h.wv.uri))

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
	testInjector().onLoadEndForEvent(h.wv, h.wv.captureInjectionEvent(h.browser, h.wv.uri))

	require.Equal(t, 7, h.scriptCount(), "internal install must stay a fixed small script set")
}

// TestInjectionEvent_StaleBrowserSkipped reproduces an old main frame firing
// during process swap: the GTK-dispatched callback must install nothing.
func TestInjectionEvent_StaleBrowserSkipped(t *testing.T) {
	h := newInjectionHarnessWithID(t, "https://example.com/a", 7)
	event := h.wv.captureInjectionEvent(h.browser, h.wv.uri)

	replacement := cefmocks.NewMockBrowser(t)
	replacement.EXPECT().GetIdentifier().Return(int32(8)).Maybe()
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
	event := h.wv.captureInjectionEvent(h.browser, h.wv.uri)

	h.wv.mu.Lock()
	h.wv.pendingIntentID++
	h.wv.mu.Unlock()

	testInjector().onLoadEndForEvent(h.wv, event)
	require.Zero(t, h.scriptCount(), "superseded intent must not install")
}

// TestInjectionEvent_CurrentInstalls verifies matching identity installs once
// for a committed document. Repeated load-end notifications for that document
// must not repeat the GTK-to-CEF script round trips.
func TestInjectionEvent_CurrentInstalls(t *testing.T) {
	h := newInjectionHarness(t, "https://example.com/a")
	ci := testInjector()

	ci.onLoadEndForEvent(h.wv, h.wv.captureInjectionEvent(h.browser, h.wv.uri))
	first := h.scriptCount()
	require.Positive(t, first)

	ci.onLoadEndForEvent(h.wv, h.wv.captureInjectionEvent(h.browser, h.wv.uri))
	require.Equal(t, first, h.scriptCount(), "repeated same-document events must be coalesced")

	h.wv.mu.Lock()
	h.wv.documentSeq++
	h.wv.mu.Unlock()
	ci.onLoadEndForEvent(h.wv, h.wv.captureInjectionEvent(h.browser, h.wv.uri))
	require.Equal(t, 2*first, h.scriptCount(), "a new committed document must receive scripts")
}

// TestInjectionEvent_NilCaptureSkips verifies the conservative fallback: a
// capture from a nil callback browser cannot prove identity, so it installs
// nothing; the current browser gets its own load-end installation.
func TestInjectionEvent_NilCaptureSkips(t *testing.T) {
	h := newInjectionHarness(t, "https://example.com/a")
	event := h.wv.captureInjectionEvent(nil, h.wv.uri)
	require.Equal(t, noBrowserID, event.browserID)

	testInjector().onLoadEndForEvent(h.wv, event)
	require.Zero(t, h.scriptCount(), "identity-less capture must not install blind")
}

// TestInjectionEvent_NilCurrentBrowserSkips verifies dispatch against a
// destroyed browser installs nothing instead of dereferencing it.
func TestInjectionEvent_NilCurrentBrowserSkips(t *testing.T) {
	h := newInjectionHarness(t, "https://example.com/a")
	event := h.wv.captureInjectionEvent(h.browser, h.wv.uri)

	h.wv.mu.Lock()
	h.wv.browser = nil
	h.wv.mu.Unlock()
	testInjector().onLoadEndForEvent(h.wv, event)
	require.Zero(t, h.scriptCount())
}

// TestInjectionEvent_StaleCallbackBrowserSkipped reproduces the exact
// reviewer scenario: WebView state advances to a replacement browser, then
// the OLD callback browser fires. Identity must come from the callback, so
// the event keeps the old identifier and is rejected. Stamping the current
// wv.browser instead would bless it with the replacement identity.
func TestInjectionEvent_StaleCallbackBrowserSkipped(t *testing.T) {
	h := newInjectionHarnessWithID(t, "https://example.com/a", 7)
	oldCallbackBrowser := h.browser

	replacement := cefmocks.NewMockBrowser(t)
	replacement.EXPECT().GetIdentifier().Return(int32(8)).Maybe()
	h.wv.mu.Lock()
	h.wv.browser = replacement
	h.wv.mu.Unlock()

	event := h.wv.captureInjectionEvent(oldCallbackBrowser, h.wv.uri)
	require.Equal(t, int32(7), event.browserID, "callback-sourced capture must keep the old identity")
	testInjector().onLoadEndForEvent(h.wv, event)
	require.Zero(t, h.scriptCount(), "old-callback event must not install against the replacement browser")
}
