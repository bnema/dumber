package webutil

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemviewTrustAllows(t *testing.T) {
	trusted := func(raw string) bool { return raw == "dumb://history" }
	webkit := SystemviewTrust{IsTrustedURL: trusted, OpaqueOriginUsesReferrer: true}
	cef := SystemviewTrust{IsTrustedURL: trusted}

	tests := []struct {
		name             string
		origin, referrer string
		webkit, cef      bool
	}{
		{name: "trusted origin", origin: "dumb://history", webkit: true, cef: true},
		{name: "untrusted origin wins over trusted referrer", origin: "https://evil.example", referrer: "dumb://history"},
		{name: "missing origin uses referrer", referrer: "dumb://history", webkit: true, cef: true},
		{name: "opaque origin uses referrer only when allowed", origin: "null", referrer: "dumb://history", webkit: true},
		{name: "nothing is untrusted"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.webkit, webkit.Allows(tt.origin, tt.referrer), "webkit")
			assert.Equal(t, tt.cef, cef.Allows(tt.origin, tt.referrer), "cef")
		})
	}
	assert.False(t, SystemviewTrust{}.Allows("dumb://history", ""), "nil matcher denies")
}

func TestDumbURLHost(t *testing.T) {
	for raw, want := range map[string]string{
		"dumb://history":         "history",
		"dumb:config":            "config",
		"DUMB://favorites/x?y=1": "favorites",
	} {
		host, ok := DumbURLHost(raw)
		require.True(t, ok, raw)
		assert.Equal(t, want, host, raw)
	}
	_, ok := DumbURLHost("https://history")
	assert.False(t, ok)
}

func TestParseSystemviewFaviconRequest(t *testing.T) {
	domain, size, err := ParseSystemviewFaviconRequest("dumb://history/api/favicon?domain=%20example.com%20&size=32")
	require.NoError(t, err)
	assert.Equal(t, "example.com", domain)
	assert.Equal(t, SystemviewFaviconSize, size)

	_, _, err = ParseSystemviewFaviconRequest("dumb://history/api/favicon")
	require.ErrorIs(t, err, ErrFaviconMissingDomain)

	_, _, err = ParseSystemviewFaviconRequest("dumb://history/api/favicon?domain=a&size=64")
	require.ErrorIs(t, err, ErrFaviconUnsupportedSize)

	_, _, err = ParseSystemviewFaviconRequest("://bad")
	require.ErrorIs(t, err, ErrFaviconInvalidRequestURL)
}

func TestReadSystemviewAssetCorruptCompressedIsAnError(t *testing.T) {
	fsys := fstest.MapFS{
		"systemviews/systemviews.wasm.br": {Data: []byte("not-brotli")},
		"systemviews/systemviews.wasm":    {Data: []byte("\x00asm")},
	}

	_, err := ReadSystemviewAsset(fsys, "systemviews/systemviews.wasm", "systemviews.wasm")

	require.Error(t, err)
}

func TestSafeSystemviewsAssetPathRejectsTraversal(t *testing.T) {
	_, _, ok := SafeSystemviewsAssetPath(SystemviewsAssetDir, "../secret")
	assert.False(t, ok)

	full, rel, ok := SafeSystemviewsAssetPath(SystemviewsAssetDir, "a/../index.html")
	require.True(t, ok)
	assert.Equal(t, "systemviews/index.html", full)
	assert.Equal(t, "index.html", rel)
}
