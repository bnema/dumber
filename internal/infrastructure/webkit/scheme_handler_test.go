package webkit

import (
	"context"
	"testing"

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
	assert.Contains(t, body, "window.location.href = targetUrl;")
	assert.Contains(t, body, "https://example.com/foo?a=1&amp;b=2")
}

func TestBuildCrashPageHTMLFallbackReloadWithoutTarget(t *testing.T) {
	body := buildCrashPageHTML("")
	assert.Contains(t, body, "window.location.reload();")
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
