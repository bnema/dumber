package webkit

import (
	"context"
	"net/http"
	"net/url"
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
		URI:    "dumb://config/api/config",
		Path:   "/api/config",
		Method: http.MethodGet,
		Scheme: "dumb",
	})
	require.NotNil(t, currentResp)
	assert.Equal(t, http.StatusOK, currentResp.StatusCode)
	assert.Equal(t, "application/json", currentResp.ContentType)
	assert.Equal(t, []byte(`{"current":true}`), currentResp.Data)

	defaultResp := defaultHandler.Handle(&SchemeRequest{
		URI:    "dumb://config/api/config/default",
		Path:   "/api/config/default",
		Method: http.MethodGet,
		Scheme: "dumb",
	})
	require.NotNil(t, defaultResp)
	assert.Equal(t, http.StatusOK, defaultResp.StatusCode)
	assert.Equal(t, "application/json", defaultResp.ContentType)
	assert.Equal(t, []byte(`{"default":true}`), defaultResp.Data)
}
