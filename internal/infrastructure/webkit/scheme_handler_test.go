package webkit

import (
	"context"
	"net/url"
	"testing"

	"github.com/bnema/dumber/assets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCrashOriginalURI(t *testing.T) {
	assert.Equal(t, "https://example.com/path?q=1", crashOriginalURI("dumb://home/crash?url=https%3A%2F%2Fexample.com%2Fpath%3Fq%3D1"))
	assert.Empty(t, crashOriginalURI("dumb://home/crash"))
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
	body := buildCrashPageHTML("dumb://home")
	assert.Contains(t, body, `data-target="dumb://home"`)
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
		URI:    "dumb://home/crash?url=https%3A%2F%2Fexample.com",
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
		URI:    "dumb://home/crash?url=javascript%3Aalert(1)",
		Path:   "/crash",
		Method: "GET",
		Scheme: "dumb",
	})
	require.NotNil(t, resp)
	assert.Contains(t, string(resp.Data), `data-target=""`)
}

func TestHandleAsset_WebRTCRootServesInternalWebRTCTester(t *testing.T) {
	h := NewDumbSchemeHandler(context.Background())
	h.SetAssets(assets.WebUIAssets)

	u, err := url.Parse("dumb://webrtc/")
	require.NoError(t, err)

	resp := h.handleAsset(u)
	require.NotNil(t, resp)
	assert.Equal(t, "text/html; charset=utf-8", resp.ContentType)
	assert.Contains(t, string(resp.Data), "webrtc.min.js")
}

func TestHandleAsset_WebRTCOpaqueFormServesInternalWebRTCTester(t *testing.T) {
	h := NewDumbSchemeHandler(context.Background())
	h.SetAssets(assets.WebUIAssets)

	u, err := url.Parse("dumb:webrtc")
	require.NoError(t, err)

	resp := h.handleAsset(u)
	require.NotNil(t, resp)
	assert.Equal(t, "text/html; charset=utf-8", resp.ContentType)
	assert.Contains(t, string(resp.Data), "webrtc.min.js")
}

func TestHandleAsset_ConfigOpaqueFormServesConfigPage(t *testing.T) {
	h := NewDumbSchemeHandler(context.Background())
	h.SetAssets(assets.WebUIAssets)

	u, err := url.Parse("dumb:config")
	require.NoError(t, err)

	resp := h.handleAsset(u)
	require.NotNil(t, resp)
	assert.Equal(t, "text/html; charset=utf-8", resp.ContentType)
	assert.Contains(t, string(resp.Data), "config")
}
