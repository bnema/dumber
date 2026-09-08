package cef

// P3.2 tests: the served shell carries content-versioned references from
// its own captured bundle while the on-disk shell keeps usable relative
// URLs. The shell itself stays noncacheable; an invalid manifest fails
// closed with a clear noncacheable error.

import (
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/assets"
)

const versionedShellFixture = `<html><head><link rel="stylesheet" href="./systemviews.css"><script src="./wasm_exec.js"></script></head><body><script>instantiateWasm(go, './systemviews.wasm')</script><img src="./other.png"></body></html>`

func shellBundleFS(t *testing.T, shell string) fstest.MapFS {
	t.Helper()
	return fstest.MapFS{
		"systemviews/index.html":             {Data: []byte(shell)},
		"systemviews/" + assets.ManifestName: {Data: testManifestBytes(t)},
	}
}

func TestAssetP32_ShellRefsVersionedFromBundleManifest(t *testing.T) {
	stubIdentityResourceHandler(t)
	h := newTestDumbSchemeHandler(t)
	h.setAssets(shellBundleFS(t, versionedShellFixture))

	rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, "https://dumber.invalid/history")))
	require.Equal(t, http.StatusOK, rh.statusCode)
	require.Equal(t, "text/html; charset=utf-8", rh.contentType)
	require.Empty(t, rh.headers, "shell stays noncacheable; only versioned targets gain immutable headers")
	served := string(rh.data)
	require.Contains(t, served, "./systemviews.wasm?v="+strings.Repeat("b", 64))
	require.Contains(t, served, "./wasm_exec.js?v="+strings.Repeat("c", 64))
	require.Contains(t, served, "./systemviews.css?v="+strings.Repeat("a", 64))
	require.Contains(t, served, `"./other.png"`, "unpinned references pass through untouched")
}

func TestAssetP32_ShellVersionedAcrossRoutes(t *testing.T) {
	stubIdentityResourceHandler(t)
	h := newTestDumbSchemeHandler(t)
	h.setAssets(shellBundleFS(t, versionedShellFixture))

	for _, raw := range []string{
		"https://dumber.invalid/history",
		"https://dumber.invalid/history/",
		"https://dumber.invalid/favorites",
		"https://dumber.invalid/favorites/",
		"https://dumber.invalid/config",
		"https://dumber.invalid/config/",
		"https://dumber.invalid/index.html",
	} {
		rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, raw)))
		require.Equal(t, http.StatusOK, rh.statusCode, raw)
		require.Contains(t, string(rh.data), "?v=", "%s must serve the versioned shell", raw)
	}

	// The bare domain root has no page mapping: existing routing rejects
	// it before the asset layer, unchanged by versioning.
	bare := staticHandlerOf(t, h.handleAsset(mustParseURL(t, "https://dumber.invalid/")))
	require.Equal(t, http.StatusNotFound, bare.statusCode)
}

func TestAssetP32_AlreadyVersionedRefsUntouched(t *testing.T) {
	out := versionShellRefs(
		[]byte(`<script src="./wasm_exec.js?v=old"></script><script src="./wasm_exec.js"></script>`),
		assets.Manifest{Version: assets.ManifestVersion, Files: map[string]assets.FileEntry{
			"systemviews.css":  {SHA256: "a", Size: 1},
			"systemviews.wasm": {SHA256: "b", Size: 2},
			"wasm_exec.js":     {SHA256: "c", Size: 3},
		}},
	)
	require.NotContains(t, string(out), "?v=c?v=", "must not double-version")
	require.Contains(t, string(out), `"./wasm_exec.js?v=old"`, "pre-versioned reference wins")
}

func TestAssetP32_InvalidManifestFailsClosedNoncacheable(t *testing.T) {
	stubIdentityResourceHandler(t)
	h := newTestDumbSchemeHandler(t)
	h.setAssets(fstest.MapFS{
		"systemviews/index.html":             {Data: []byte("<html>shell</html>")},
		"systemviews/" + assets.ManifestName: {Data: []byte("not json")},
	})

	rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, "https://dumber.invalid/history")))
	require.Equal(t, http.StatusInternalServerError, rh.statusCode)
	require.Equal(t, "no-store", rh.headers["Cache-Control"], "manifest failure must be noncacheable")
	require.Contains(t, string(rh.data), "Asset manifest invalid")
}

func TestAssetP32_MissingManifestFailsClosed(t *testing.T) {
	stubIdentityResourceHandler(t)
	h := newTestDumbSchemeHandler(t)
	h.setAssets(fstest.MapFS{
		"systemviews/index.html": {Data: []byte("<html>shell</html>")},
	})

	rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, "https://dumber.invalid/history")))
	require.Equal(t, http.StatusInternalServerError, rh.statusCode)
	require.Equal(t, "no-store", rh.headers["Cache-Control"])
}

func TestAssetP32_ReplacementSwapsShellVersions(t *testing.T) {
	stubIdentityResourceHandler(t)
	digestA := strings.Repeat("a", 64)
	manifestA, err := assets.Manifest{Version: assets.ManifestVersion, Files: map[string]assets.FileEntry{
		"systemviews.css":  {SHA256: digestA, Size: 1},
		"systemviews.wasm": {SHA256: digestA, Size: 2},
		"wasm_exec.js":     {SHA256: digestA, Size: 3},
	}}.Bytes()
	require.NoError(t, err)
	h := newTestDumbSchemeHandler(t)
	h.setAssets(fstest.MapFS{
		"systemviews/index.html":             {Data: []byte(versionedShellFixture)},
		"systemviews/" + assets.ManifestName: {Data: manifestA},
	})
	u := mustParseURL(t, "https://dumber.invalid/history")
	require.Contains(t, string(staticHandlerOf(t, h.handleAsset(u)).data), "?v="+digestA)

	digestB := strings.Repeat("b", 64)
	manifestB, err := assets.Manifest{Version: assets.ManifestVersion, Files: map[string]assets.FileEntry{
		"systemviews.css":  {SHA256: digestB, Size: 1},
		"systemviews.wasm": {SHA256: digestB, Size: 2},
		"wasm_exec.js":     {SHA256: digestB, Size: 3},
	}}.Bytes()
	require.NoError(t, err)
	h.setAssets(fstest.MapFS{
		"systemviews/index.html":             {Data: []byte(versionedShellFixture)},
		"systemviews/" + assets.ManifestName: {Data: manifestB},
	})
	served := string(staticHandlerOf(t, h.handleAsset(u)).data)
	require.Contains(t, served, "?v="+digestB)
	require.NotContains(t, served, "?v="+digestA, "shell versions follow the captured bundle")
}

func TestAssetP32_OnDiskShellKeepsRelativeURLs(t *testing.T) {
	// The committed shell must stay directly usable by alternate shells:
	// concrete relative URLs, no template placeholders, no baked versions.
	data, err := assets.WebUIAssets.ReadFile("systemviews/index.html")
	require.NoError(t, err)
	shell := string(data)
	for _, ref := range []string{"./systemviews.wasm", "./wasm_exec.js", "./systemviews.css"} {
		require.Contains(t, shell, ref)
		require.NotContains(t, shell, ref+"?v=", "on-disk shell must not bake versions")
	}
	require.NotContains(t, shell, "{{", "no unresolved template placeholders")
}
