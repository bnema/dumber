package webkit

import (
	"context"
	"net/http"
	"testing"

	"bnema.dev/dumber/internal/application/port"
	"github.com/stretchr/testify/require"
)

type stubHomeIconResolver map[string][]byte

func (s stubHomeIconResolver) ResolveHomeIconByID(_ context.Context, id string) ([]byte, string, error) {
	if data, ok := s[id]; ok {
		return data, "image/png", nil
	}
	return nil, "", port.ErrHomeIconNotFound
}

func newHomeIconTestHandler() *DumbSchemeHandler {
	h := NewDumbSchemeHandler(context.Background())
	h.SetHomeIconResolver(stubHomeIconResolver{"jellyfin": []byte("png")})
	return h
}

func homeIconReq(query string) *SchemeRequest {
	return &SchemeRequest{URI: "dumb://home/api/home-icon" + query, Path: homeIconAPIPath, Method: httpGET, Origin: "dumb://home"}
}

func TestWebKitHomeIconServesPNG(t *testing.T) {
	immediate, resolve := newHomeIconTestHandler().homeIconRequest(homeIconReq("?service=jellyfin"))
	require.Nil(t, immediate)
	require.NotNil(t, resolve)
	resp := resolve(context.Background())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "image/png", resp.ContentType)
	require.Equal(t, []byte("png"), resp.Data)
	require.Equal(t, "max-age=3600", resp.Headers["Cache-Control"])
}

func TestWebKitHomeIconUnknownServiceIs404(t *testing.T) {
	_, resolve := newHomeIconTestHandler().homeIconRequest(homeIconReq("?service=nope"))
	require.Equal(t, http.StatusNotFound, resolve(context.Background()).StatusCode)
}

func TestWebKitHomeIconRejectsBadID(t *testing.T) {
	h := newHomeIconTestHandler()
	for _, q := range []string{"", "?service=", "?service=../etc", "?service=UPPER"} {
		immediate, resolve := h.homeIconRequest(homeIconReq(q))
		require.Nil(t, resolve, q)
		require.Equal(t, http.StatusBadRequest, immediate.StatusCode, q)
	}
}

func TestWebKitHomeIconAppliesTrustGate(t *testing.T) {
	req := homeIconReq("?service=jellyfin")
	req.Origin = "https://evil.example"
	immediate, resolve := newHomeIconTestHandler().homeIconRequest(req)
	require.Nil(t, resolve)
	require.Equal(t, http.StatusForbidden, immediate.StatusCode)
}

func TestWebKitHomeIconUnavailableWithoutResolver(t *testing.T) {
	immediate, resolve := NewDumbSchemeHandler(context.Background()).homeIconRequest(homeIconReq("?service=jellyfin"))
	require.Nil(t, resolve)
	require.Equal(t, http.StatusNotFound, immediate.StatusCode)
}

func TestWebKitHomeIconTrustsSystemviewWebViewWithoutHeaders(t *testing.T) {
	h := newHomeIconTestHandler()
	req := homeIconReq("?service=jellyfin")
	req.Origin = ""
	req.WebViewURI = "dumb://home"
	_, trusted := h.homeIconRequest(req)
	require.NotNil(t, trusted, "img loads from a dumb://home view carry no headers")

	req.WebViewURI = "https://evil.example/"
	immediate, resolve := h.homeIconRequest(req)
	require.Nil(t, resolve)
	require.Equal(t, http.StatusForbidden, immediate.StatusCode)
}

func TestSystemviewImageTrustGate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		req  *SchemeRequest
		want bool
	}{
		{"nil", nil, false},
		{"trusted origin", &SchemeRequest{Origin: "dumb://history"}, true},
		{"headerless from history view", &SchemeRequest{WebViewURI: "dumb://history"}, true},
		{"headerless from favorites view", &SchemeRequest{WebViewURI: "dumb://favorites"}, true},
		{"headerless from home view", &SchemeRequest{WebViewURI: "dumb://home"}, true},
		{"headerless, no view", &SchemeRequest{}, false},
		{"headerless from web page", &SchemeRequest{WebViewURI: "https://evil.example/"}, false},
		{"headerless from unknown dumb host", &SchemeRequest{WebViewURI: "dumb://evil"}, false},
		{"untrusted referer in systemview", &SchemeRequest{Referer: "https://evil.example/", WebViewURI: "dumb://history"}, false},
		{"untrusted origin in systemview", &SchemeRequest{Origin: "https://evil.example", WebViewURI: "dumb://history"}, false},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, isTrustedSystemviewImageRequest(tc.req), tc.name)
		require.Equal(t, tc.want, isTrustedSystemviewFaviconRequest(tc.req), tc.name)
	}
}

func TestSystemviewAPITrustIgnoresWebView(t *testing.T) {
	t.Parallel()
	// Non-image APIs must keep requiring a trusted Origin or Referer.
	require.False(t, isTrustedSystemviewAPIRequest(&SchemeRequest{WebViewURI: "dumb://history"}))
}
