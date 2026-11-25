package cef

import (
	"context"
	"net/http"
	"testing"

	purecef "bnema.dev/purego-cef/cef"
	cefmocks "bnema.dev/purego-cef/cef/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Transition values observed at runtime (CEF 0.15 / Chromium):
// Dumber LoadURL = explicit|direct-load, back = reload|forward-back,
// reload = reload, link / location= / initial subframe = link or auto-subframe.
const (
	ttLoadURL  = purecef.TransitionTypeTtExplicit | purecef.TransitionTypeTtDirectLoadFlag
	ttBack     = purecef.TransitionTypeTtReload | purecef.TransitionTypeTtForwardBackFlag
	ttReload   = purecef.TransitionTypeTtReload
	ttLink     = purecef.TransitionTypeTtLink
	ttSubframe = purecef.TransitionTypeTtAutoSubframe
	ttForm     = purecef.TransitionTypeTtFormSubmit
)

func TestIsInternalTargetURL(t *testing.T) {
	for raw, want := range map[string]bool{
		"dumb://history":                      true,
		"DUMB://config":                       true,
		"dumb:home":                           true,
		"https://dumber.invalid/history":      true,
		"https://DUMBER.INVALID/config":       true,
		"https://dumber.invalid:443/home":     true,
		"https://x@dumber.invalid/home":       true,
		"https://dumber.invalid.evil.com/":    false,
		"https://evil.com/dumber.invalid/":    false,
		"https://evil.com/?u=dumb://history":  false,
		"http://dumber.invalid/history":       false,
		"https://dumber.invalid@evil.com/":    false,
		"about:blank":                         false,
		"":                                    false,
		"http://127.0.0.1:8766/index.html":    false,
		"https://subdomain.dumber.invalid/x":  false,
		"javascript:location='dumb://config'": false,
	} {
		assert.Equal(t, want, isInternalTargetURL(raw), raw)
	}
}

func TestBlockInternalNavigation(t *testing.T) {
	const web = "http://127.0.0.1:8766/index.html"
	const sys = "https://dumber.invalid/history"
	for _, tc := range []struct {
		name     string
		main     bool
		current  string
		target   string
		tt       purecef.TransitionType
		redirect bool
		block    bool
	}{
		{"non-internal target is never blocked", false, web, "https://example.com/", ttLink, false, false},
		{"subframe initial load of system view", false, "", sys, ttSubframe, false, true},
		{"subframe self-navigation", false, web, "dumb://history/", ttLink, false, true},
		{"subframe redirect to system view", false, web, sys, ttLink, true, true},
		{"subframe inside a system view", false, sys, sys, ttLink, false, true},
		{"page link from web page", true, web, sys, ttLink, false, true},
		{"form post from web page", true, web, sys, ttForm, false, true},
		{"server redirect from typed web URL", true, web, sys, ttLoadURL, true, true},
		{"popup first load (empty current)", true, "", sys, ttLink, false, true},
		{"omnibox from web page", true, web, sys, ttLoadURL, false, false},
		{"CLI start (empty current)", true, "", "https://dumber.invalid/home", ttLoadURL, false, false},
		{"new pane (about:blank)", true, "about:blank", sys, ttLoadURL, false, false},
		{"back from web page", true, web, "https://dumber.invalid/config", ttBack, false, false},
		{"reload of a system view", true, sys, sys, ttReload, false, false},
		{"link between system views", true, sys, "dumb://config/", ttLink, false, false},
		{"dumb: to https redirect inside system view", true, sys, "https://dumber.invalid/config", ttLink, true, false},
	} {
		got := blockInternalNavigation(tc.main, tc.current, tc.target, tc.tt, tc.redirect)
		assert.Equal(t, tc.block, got, tc.name)
	}
}

func TestBlockInternalPopup(t *testing.T) {
	assert.False(t, blockInternalPopup(false, "http://x/", "https://example.com/"))
	assert.True(t, blockInternalPopup(false, "http://x/", "https://dumber.invalid/history"))
	assert.True(t, blockInternalPopup(false, "http://x/", "dumb://history"))
	assert.True(t, blockInternalPopup(true, "http://x/", "dumb://history"), "web page main frame")
	assert.True(t, blockInternalPopup(false, "https://dumber.invalid/home", "dumb://history"), "subframe on internal origin")
	assert.True(t, blockInternalPopup(false, "", "dumb://config"), "no source frame")
	assert.False(t, blockInternalPopup(true, "https://dumber.invalid/home", "dumb://history"))
}

func TestOnBeforeBrowseCancelsSubframeSystemViewNavigation(t *testing.T) {
	frame := cefmocks.NewMockFrame(t)
	frame.EXPECT().IsMain().Return(false)
	frame.EXPECT().GetURL().Return("http://127.0.0.1:8766/index.html")
	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetURL().Return("https://dumber.invalid/history")
	request.EXPECT().GetTransitionType().Return(ttLink)
	h := &handlerSet{wv: &WebView{ctx: context.Background(), scrollCancelSeam: &recordingScrollBridge{}}}

	require.True(t, h.OnBeforeBrowse(nil, frame, request, 1, 0))
}

func TestOnBeforePopupRefusesSystemViewFromWebPage(t *testing.T) {
	frame := cefmocks.NewMockFrame(t)
	frame.EXPECT().IsMain().Return(false)
	frame.EXPECT().GetURL().Return("http://127.0.0.1:8766/index.html")
	h := &handlerSet{wv: &WebView{ctx: context.Background()}}

	require.True(t, h.OnBeforePopup(nil, frame, 1, "https://dumber.invalid/config", "", 0, 1, nil, nil, nil, nil, nil, nil))
}

func TestCreateRefusesSystemViewDocumentInSubframe(t *testing.T) {
	useRawResourceHandlers(t)
	h := newTestDumbSchemeHandler(t)

	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetURL().Return("dumb://history").Once()
	request.EXPECT().GetMethod().Return(http.MethodGet).Once()
	request.EXPECT().GetResourceType().Return(purecef.ResourceTypeRtSubFrame).Once()
	frame := cefmocks.NewMockFrame(t)
	frame.EXPECT().IsMain().Return(false).Once()

	got, ok := h.Create(nil, frame, "dumb", request).(*staticResourceHandler)
	require.True(t, ok)
	require.Equal(t, http.StatusForbidden, got.statusCode)
}
