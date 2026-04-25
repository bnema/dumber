package webkit

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/bnema/dumber/assets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCrashOriginalURI(t *testing.T) {
	assert.Equal(t, "https://example.com/path?q=1", crashOriginalURI("dumb://history/crash?url=https%3A%2F%2Fexample.com%2Fpath%3Fq%3D1"))
	assert.Empty(t, crashOriginalURI("dumb://history/crash"))
	assert.Empty(t, crashOriginalURI("%%%"))
}

func TestBuildCrashPageHTMLIncludesReloadTarget(t *testing.T) {
	body := buildCrashPageHTML("https://example.com/foo?a=1&b=2")
	assert.Contains(t, body, `data-target="https://example.com/foo?a=1&amp;b=2"`)
	assert.Contains(t, body, "https://example.com/foo?a=1&amp;b=2")
}

func TestBuildCrashPageHTMLFallbackReloadWithoutTarget(t *testing.T) {
	body := buildCrashPageHTML("")
	assert.Contains(t, body, "window.location.reload();")
}

func TestBuildCrashPageHTMLRejectsUnsafeReloadScheme(t *testing.T) {
	// buildCrashPageHTML receives already-sanitized input from the caller.
	// sanitizeCrashPageOriginalURI strips unsafe schemes to "".
	sanitized := sanitizeCrashPageOriginalURI("javascript:alert(1)")
	assert.Empty(t, sanitized)
	body := buildCrashPageHTML(sanitized)
	assert.Contains(t, body, `data-target=""`)
}

func TestBuildCrashPageHTMLAllowsDumbSchemeTarget(t *testing.T) {
	body := buildCrashPageHTML("dumb://history")
	assert.Contains(t, body, `data-target="dumb://history"`)
}

func TestBuildCrashPageHTMLEscapesScriptBreakoutPayload(t *testing.T) {
	payload := `https://example.com/?q=</script><script>alert(1)</script>`
	body := buildCrashPageHTML(payload)
	assert.NotContains(t, body, "</script><script>alert(1)</script>")
	assert.Contains(t, body, "&lt;/script&gt;&lt;script&gt;alert(1)&lt;/script&gt;")
}

func TestIsTrustedSystemviewURL(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"dumb://history",
		"dumb://favorites",
		"dumb:config",
	} {
		require.True(t, isTrustedSystemviewURL(raw), raw)
	}
	for _, raw := range []string{
		"",
		"https://dumber.invalid/history",
		"dumb://api/favicon",
		"dumb://evil/history",
	} {
		require.False(t, isTrustedSystemviewURL(raw), raw)
	}
}

func TestIsTrustedSystemviewFaviconRequestRequiresTrustedRefererWhenOriginAbsent(t *testing.T) {
	t.Parallel()

	require.True(t, isTrustedSystemviewFaviconRequest(&SchemeRequest{Referer: "dumb://history"}))
	require.False(t, isTrustedSystemviewFaviconRequest(&SchemeRequest{Referer: ""}))
}

func TestIsTrustedSystemviewFaviconRequestHonorsOriginBeforeReferer(t *testing.T) {
	t.Parallel()

	require.True(t, isTrustedSystemviewFaviconRequest(&SchemeRequest{Origin: "dumb://history"}))
	require.True(t, isTrustedSystemviewFaviconRequest(&SchemeRequest{Origin: "dumb://history", Referer: "https://evil.example"}))
	require.False(t, isTrustedSystemviewFaviconRequest(&SchemeRequest{Origin: "https://evil.example", Referer: "dumb://history"}))
}

func TestRegisterDefaultsIncludesCrashHandler(t *testing.T) {
	handler := NewDumbSchemeHandler(context.Background())
	require.NotNil(t, handler)

	handler.mu.RLock()
	crashHandler, ok := handler.handlers["/crash"]
	handler.mu.RUnlock()
	require.True(t, ok)
	require.NotNil(t, crashHandler)

	resp := crashHandler.Handle(&SchemeRequest{
		URI:    "dumb://history/crash?url=https%3A%2F%2Fexample.com",
		Path:   "/crash",
		Method: "GET",
		Scheme: "dumb",
	})
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "text/html; charset=utf-8", resp.ContentType)
	assert.Contains(t, string(resp.Data), "Renderer process ended")
}

func TestCrashHandlerSanitizesUnsafeURLQuery(t *testing.T) {
	handler := NewDumbSchemeHandler(context.Background())
	require.NotNil(t, handler)

	handler.mu.RLock()
	crashHandler, ok := handler.handlers["/crash"]
	handler.mu.RUnlock()
	require.True(t, ok)
	require.NotNil(t, crashHandler)

	resp := crashHandler.Handle(&SchemeRequest{
		URI:    "dumb://history/crash?url=javascript%3Aalert(1)",
		Path:   "/crash",
		Method: "GET",
		Scheme: "dumb",
	})
	require.NotNil(t, resp)
	assert.Contains(t, string(resp.Data), `data-target=""`)
}

func TestHandleAsset_ConfigOpaqueFormServesSystemviewsShell(t *testing.T) {
	h := NewDumbSchemeHandler(context.Background())
	h.SetAssets(assets.WebUIAssets)

	u, err := url.Parse("dumb:config")
	require.NoError(t, err)

	resp := h.handleAsset(u)
	require.NotNil(t, resp)
	assert.Equal(t, "text/html; charset=utf-8", resp.ContentType)
	assert.Contains(t, string(resp.Data), "systemviews.wasm")
}

func TestHandleAsset_SystemviewsCSSIsServed(t *testing.T) {
	h := NewDumbSchemeHandler(context.Background())
	h.SetAssets(assets.WebUIAssets)

	u, err := url.Parse("dumb://history/systemviews.css")
	require.NoError(t, err)

	resp := h.handleAsset(u)
	require.NotNil(t, resp)
	// Charset suffixes are acceptable; assert the MIME type contract only.
	assert.Equal(t, "text/css", strings.Split(resp.ContentType, ";")[0])
	assert.Contains(t, string(resp.Data), ".sv-app")
}

func TestHandleAsset_SystemviewsRootsServeShell(t *testing.T) {
	h := NewDumbSchemeHandler(context.Background())
	h.SetAssets(assets.WebUIAssets)

	tests := []string{
		"dumb://history",
		"dumb:history",
		"dumb://favorites",
		"dumb:favorites",
		"dumb://config",
		"dumb:config",
		"dumb://error",
		"dumb:error",
	}

	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			u, err := url.Parse(raw)
			require.NoError(t, err)

			resp := h.handleAsset(u)
			require.NotNil(t, resp)
			assert.Equal(t, "text/html; charset=utf-8", resp.ContentType)
			assert.Contains(t, string(resp.Data), "systemviews.wasm")
		})
	}
}

func TestConfigHandlersUseInjectedPayloadBuilders(t *testing.T) {
	h := NewDumbSchemeHandler(context.Background())
	h.SetConfigPayloadBuilders(
		func() ([]byte, error) { return []byte(`{"current":true}`), nil },
		func() ([]byte, error) { return []byte(`{"default":true}`), nil },
	)

	h.mu.RLock()
	currentHandler := h.handlers["/api/config"]
	defaultHandler := h.handlers["/api/config/default"]
	h.mu.RUnlock()
	require.NotNil(t, currentHandler)
	require.NotNil(t, defaultHandler)

	currentResp := currentHandler.Handle(&SchemeRequest{
		URI:     "dumb://config/api/config",
		Path:    "/api/config",
		Method:  http.MethodGet,
		Scheme:  "dumb",
		Referer: "dumb://config",
	})
	require.NotNil(t, currentResp)
	assert.Equal(t, http.StatusOK, currentResp.StatusCode)
	assert.Equal(t, "application/json", currentResp.ContentType)
	assert.Equal(t, []byte(`{"current":true}`), currentResp.Data)

	defaultResp := defaultHandler.Handle(&SchemeRequest{
		URI:     "dumb://config/api/config/default",
		Path:    "/api/config/default",
		Method:  http.MethodGet,
		Scheme:  "dumb",
		Referer: "dumb://config",
	})
	require.NotNil(t, defaultResp)
	assert.Equal(t, http.StatusOK, defaultResp.StatusCode)
	assert.Equal(t, "application/json", defaultResp.ContentType)
	assert.Equal(t, []byte(`{"default":true}`), defaultResp.Data)
}

func TestConfigHandlersRejectUntrustedRequests(t *testing.T) {
	h := NewDumbSchemeHandler(context.Background())
	h.SetConfigPayloadBuilders(
		func() ([]byte, error) { return []byte(`{"current":true}`), nil },
		func() ([]byte, error) { return []byte(`{"default":true}`), nil },
	)

	h.mu.RLock()
	currentHandler := h.handlers["/api/config"]
	defaultHandler := h.handlers["/api/config/default"]
	h.mu.RUnlock()

	for _, handler := range []PageHandler{currentHandler, defaultHandler} {
		resp := handler.Handle(&SchemeRequest{Method: http.MethodGet, Origin: "https://evil.example"})
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.True(t, resp.SuppressDefaultHeaders)
		assert.NotContains(t, resp.Headers, "Access-Control-Allow-Origin")
	}
}

func TestShouldAddCORSHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "private api route", path: "/api/config", want: false},
		{name: "wasm asset", path: "/systemviews.wasm", want: true},
		{name: "html shell", path: "/", want: false},
		{name: "js asset", path: "/wasm_exec.js", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, shouldAddCORSHeaders(tt.path))
		})
	}
}

func TestResponseHeadersForPath_WASMIncludesContentTypeAndCORS(t *testing.T) {
	t.Parallel()

	headers := responseHeadersForPath("/systemviews.wasm", "application/wasm")
	assert.Equal(t, "application/wasm", headers["Content-Type"])
	assert.Equal(t, "*", headers["Access-Control-Allow-Origin"])
	assert.Equal(t, "GET, POST, OPTIONS", headers["Access-Control-Allow-Methods"])
}

func TestReadAssetWithEncodingDecompressesEmbeddedBrotliWASM(t *testing.T) {
	t.Parallel()

	data, headers, err := readAssetWithEncoding(assets.WebUIAssets, "systemviews/systemviews.wasm", "systemviews.wasm")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(data), 4)
	assert.Equal(t, "\x00asm", string(data[:4]))
	assert.NotContains(t, headers, "Content-Encoding")
}

func TestHandleAssetRejectsTraversalOutsideSystemviews(t *testing.T) {
	h := NewDumbSchemeHandler(context.Background())
	h.SetAssets(assets.WebUIAssets)

	for _, raw := range []string{
		"dumb://history/../logo.svg",
		"dumb://history/nested/../../logo.svg",
	} {
		t.Run(raw, func(t *testing.T) {
			u, err := url.Parse(raw)
			require.NoError(t, err)
			assert.Nil(t, h.handleAsset(u))
		})
	}
}

func TestSafeSystemviewsAssetPathRejectsTraversal(t *testing.T) {
	t.Parallel()

	fullPath, relPath, ok := safeSystemviewsAssetPath(systemviewsAssetDir, "nested/../systemviews.css")
	require.True(t, ok)
	assert.Equal(t, "systemviews/systemviews.css", fullPath)
	assert.Equal(t, "systemviews.css", relPath)

	invalid := []struct {
		name     string
		assetDir string
		relPath  string
	}{
		{name: "parent escape", assetDir: systemviewsAssetDir, relPath: "../logo.svg"},
		{name: "nested parent escape", assetDir: systemviewsAssetDir, relPath: "nested/../../logo.svg"},
		{name: "absolute parent escape", assetDir: systemviewsAssetDir, relPath: "/../logo.svg"},
		{name: "null byte", assetDir: systemviewsAssetDir, relPath: "systemviews.css\x00"},
		{name: "wrong asset dir", assetDir: "logos", relPath: "logo.svg"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, ok := safeSystemviewsAssetPath(tt.assetDir, tt.relPath)
			assert.False(t, ok)
		})
	}
}
