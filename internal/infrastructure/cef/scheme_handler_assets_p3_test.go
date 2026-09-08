package cef

// P3.2 tests: the served shell carries content-versioned references from
// its own captured bundle while the on-disk shell keeps usable relative
// URLs. The shell itself stays noncacheable; an invalid manifest fails
// closed with a clear noncacheable error.

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	cefmocks "github.com/bnema/purego-cef/cef/mocks"
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
	require.Equal(t, noStoreCacheControl, rh.headers["Cache-Control"],
		"P3.3 policy: the shell itself stays noncacheable")
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

// ---------------------------------------------------------------------------
// P3.3: immutable headers bound to served bytes
// ---------------------------------------------------------------------------

// trueManifestFS builds a bundle whose manifest pins the true digests of
// the given files, plus a compressed WASM agreeing with the raw bytes.
func trueManifestFS(t *testing.T, wasm, js, css []byte) fstest.MapFS {
	t.Helper()
	entries := make(map[string]assets.FileEntry, len(assets.ManifestFiles))
	for name, data := range map[string][]byte{
		"systemviews.wasm": wasm,
		"wasm_exec.js":     js,
		"systemviews.css":  css,
	} {
		sum := sha256.Sum256(data)
		entries[name] = assets.FileEntry{SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data))}
	}
	manifest, err := assets.Manifest{Version: assets.ManifestVersion, Files: entries}.Bytes()
	require.NoError(t, err)
	return fstest.MapFS{
		"systemviews/systemviews.wasm":       {Data: wasm},
		"systemviews/systemviews.wasm.br":    {Data: brotliCompressForTest(t, wasm)},
		"systemviews/wasm_exec.js":           {Data: js},
		"systemviews/systemviews.css":        {Data: css},
		"systemviews/" + assets.ManifestName: {Data: manifest},
	}
}

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestAssetP33_VersionedCSSServesImmutable(t *testing.T) {
	stubIdentityResourceHandler(t)
	css := []byte("/* immutable css */")
	h := newTestDumbSchemeHandler(t)
	h.setAssets(trueManifestFS(t, []byte("\x00asm"), []byte("/* js */"), css))

	rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, "https://dumber.invalid/systemviews.css?v="+digestOf(css))))
	require.Equal(t, http.StatusOK, rh.statusCode)
	require.Equal(t, css, rh.data)
	require.Equal(t, immutableCacheControl, rh.headers["Cache-Control"])
}

func TestAssetP33_UnversionedCSSServesNoStore(t *testing.T) {
	stubIdentityResourceHandler(t)
	css := []byte("/* plain css */")
	h := newTestDumbSchemeHandler(t)
	h.setAssets(trueManifestFS(t, []byte("\x00asm"), []byte("/* js */"), css))

	rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, "https://dumber.invalid/systemviews.css")))
	require.Equal(t, http.StatusOK, rh.statusCode)
	require.Equal(t, noStoreCacheControl, rh.headers["Cache-Control"])
	require.NotContains(t, rh.headers["Cache-Control"], "immutable")
}

func TestAssetP33_WrongVersionRejectedNoncacheable(t *testing.T) {
	stubIdentityResourceHandler(t)
	css := []byte("/* real css */")
	h := newTestDumbSchemeHandler(t)
	h.setAssets(trueManifestFS(t, []byte("\x00asm"), []byte("/* js */"), css))

	rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, "https://dumber.invalid/systemviews.css?v="+strings.Repeat("0", 64))))
	require.Equal(t, http.StatusNotFound, rh.statusCode)
	require.Equal(t, noStoreCacheControl, rh.headers["Cache-Control"])
}

func TestAssetP33_DuplicateAndEmptyVersionsRejected(t *testing.T) {
	stubIdentityResourceHandler(t)
	css := []byte("/* css */")
	fsys := trueManifestFS(t, []byte("\x00asm"), []byte("/* js */"), css)
	h := newTestDumbSchemeHandler(t)
	h.setAssets(fsys)
	digest := digestOf(css)

	for _, raw := range []string{
		"https://dumber.invalid/systemviews.css?v=" + digest + "&v=" + digest,
		"https://dumber.invalid/systemviews.css?v=",
		"https://dumber.invalid/systemviews.css?v=a&v=b",
		// Malformed escapes fail closed instead of degrading to
		// unversioned or single-version handling.
		"https://dumber.invalid/systemviews.css?v=" + digest + "&v=%ZZ",
		"https://dumber.invalid/systemviews.css?v=%ZZ",
	} {
		rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, raw)))
		require.Equal(t, http.StatusNotFound, rh.statusCode, raw)
		require.Equal(t, noStoreCacheControl, rh.headers["Cache-Control"], raw)
	}

	// A malformed escape on an unrelated parameter is not a version:
	// the request serves unversioned and noncacheable.
	plain := staticHandlerOf(t, h.handleAsset(mustParseURL(t, "https://dumber.invalid/systemviews.css?x=%ZZ")))
	require.Equal(t, http.StatusOK, plain.statusCode)
	require.Equal(t, noStoreCacheControl, plain.headers["Cache-Control"])
}

func TestAssetP33_StaleManifestRejectedNoncacheable(t *testing.T) {
	stubIdentityResourceHandler(t)
	// The manifest pins dummy digests while the files carry real bytes:
	// v matches the pin, but the served bytes do not. URL-to-manifest
	// equality alone must never earn immutable headers.
	h := newTestDumbSchemeHandler(t)
	h.setAssets(fstest.MapFS{
		"systemviews/systemviews.css":        {Data: []byte("/* real css */")},
		"systemviews/systemviews.wasm":       {Data: []byte("\x00asm")},
		"systemviews/systemviews.wasm.br":    {Data: brotliCompressForTest(t, []byte("\x00asm"))},
		"systemviews/wasm_exec.js":           {Data: []byte("/* js */")},
		"systemviews/" + assets.ManifestName: {Data: testManifestBytes(t)},
	})

	rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, "https://dumber.invalid/systemviews.css?v="+strings.Repeat("a", 64))))
	require.Equal(t, http.StatusNotFound, rh.statusCode, "stale pin must fail closed")
	require.Equal(t, noStoreCacheControl, rh.headers["Cache-Control"])
}

func TestAssetP33_MissingVersionedArtifactRejected(t *testing.T) {
	stubIdentityResourceHandler(t)
	h := newTestDumbSchemeHandler(t)
	h.setAssets(trueManifestFS(t, []byte("\x00asm"), []byte("/* js */"), []byte("/* css */")))

	// wasm_exec.js is pinned but absent from this bundle variant.
	bundle := fstest.MapFS{
		"systemviews/systemviews.css": {Data: []byte("/* css */")},
	}
	manifest, err := assets.Manifest{Version: assets.ManifestVersion, Files: map[string]assets.FileEntry{
		"systemviews.css":  {SHA256: digestOf([]byte("/* css */")), Size: int64(len("/* css */"))},
		"systemviews.wasm": {SHA256: strings.Repeat("b", 64), Size: 2},
		"wasm_exec.js":     {SHA256: strings.Repeat("c", 64), Size: 3},
	}}.Bytes()
	require.NoError(t, err)
	bundle["systemviews/"+assets.ManifestName] = &fstest.MapFile{Data: manifest}
	h.setAssets(bundle)

	rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, "https://dumber.invalid/wasm_exec.js?v="+strings.Repeat("c", 64))))
	require.Equal(t, http.StatusNotFound, rh.statusCode)
	require.Equal(t, noStoreCacheControl, rh.headers["Cache-Control"])
}

func TestAssetP33_VersionedWASMServesImmutableOnWire(t *testing.T) {
	stubIdentityResourceHandler(t)
	wasm := []byte("\x00asm-wire-immutable")
	h := newTestDumbSchemeHandler(t)
	h.setAssets(trueManifestFS(t, wasm, []byte("/* js */"), []byte("/* css */")))

	wh := awaitWASMHandler(t, h.handleAsset(mustParseURL(t, wasmURL(t)+"?v="+digestOf(wasm))))
	require.Equal(t, http.StatusOK, wh.statusCode)
	require.Equal(t, wasm, wh.data)

	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusOK)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusOK)).Once()
	response.EXPECT().SetMimeType("application/wasm").Once()
	response.EXPECT().SetHeaderByName("Cache-Control", immutableCacheControl, int32(1)).Once()
	var responseLength int64
	wh.GetResponseHeaders(response, &responseLength, 0)
	require.Equal(t, int64(len(wasm)), responseLength)
}

func TestAssetP33_WrongVersionWASMDecodesNothing(t *testing.T) {
	stubIdentityResourceHandler(t)
	wasm := []byte("\x00asm-no-decode")
	fsys := &countingFS{inner: fstest.MapFS{
		"systemviews/systemviews.wasm.br": {Data: brotliCompressForTest(t, wasm)},
	}}
	_ = fsys
	h := newTestDumbSchemeHandler(t)
	inner := fstest.MapFS{
		"systemviews/systemviews.wasm.br": {Data: brotliCompressForTest(t, wasm)},
	}
	manifest, err := assets.Manifest{Version: assets.ManifestVersion, Files: map[string]assets.FileEntry{
		"systemviews.css":  {SHA256: strings.Repeat("a", 64), Size: 1},
		"systemviews.wasm": {SHA256: digestOf(wasm), Size: int64(len(wasm))},
		"wasm_exec.js":     {SHA256: strings.Repeat("c", 64), Size: 3},
	}}.Bytes()
	require.NoError(t, err)
	inner["systemviews/"+assets.ManifestName] = &fstest.MapFile{Data: manifest}
	counting := &countingFS{inner: inner}
	h.setAssets(counting)

	rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, wasmURL(t)+"?v="+strings.Repeat("0", 64))))
	require.Equal(t, http.StatusNotFound, rh.statusCode)
	require.Equal(t, noStoreCacheControl, rh.headers["Cache-Control"])
	require.Zero(t, counting.compressedReads(), "rejected version must not start a decode")
}

func TestAssetP33_StaleWASMBytesFailClosedOnWire(t *testing.T) {
	stubIdentityResourceHandler(t)
	wasm := []byte("\x00asm-stale-wire")
	other := []byte("\x00asm-other-wire")
	otherManifest, err := assets.Manifest{Version: assets.ManifestVersion, Files: map[string]assets.FileEntry{
		"systemviews.css":  {SHA256: strings.Repeat("a", 64), Size: 1},
		"systemviews.wasm": {SHA256: digestOf(other), Size: int64(len(other))},
		"wasm_exec.js":     {SHA256: strings.Repeat("c", 64), Size: 3},
	}}.Bytes()
	require.NoError(t, err)
	h := newTestDumbSchemeHandler(t)
	h.setAssets(fstest.MapFS{
		"systemviews/systemviews.wasm":       {Data: wasm},
		"systemviews/systemviews.wasm.br":    {Data: brotliCompressForTest(t, wasm)},
		"systemviews/" + assets.ManifestName: {Data: otherManifest},
	})

	// v matches the stale pin, but the decoded bytes hash differently.
	wh := awaitWASMHandler(t, h.handleAsset(mustParseURL(t, wasmURL(t)+"?v="+digestOf(other))))
	require.Equal(t, http.StatusNotFound, wh.statusCode)

	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusNotFound)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusNotFound)).Once()
	response.EXPECT().SetMimeType("text/html").Once()
	response.EXPECT().SetCharset("utf-8").Once()
	response.EXPECT().SetHeaderByName("Cache-Control", noStoreCacheControl, int32(1)).Once()
	var responseLength int64
	wh.GetResponseHeaders(response, &responseLength, 0)
	require.Equal(t, int64(len(wh.data)), responseLength)
}

func TestAssetP33_ReplacementSwapsValidatedVersions(t *testing.T) {
	stubIdentityResourceHandler(t)
	cssA := []byte("/* css a */")
	h := newTestDumbSchemeHandler(t)
	h.setAssets(trueManifestFS(t, []byte("\x00asm"), []byte("/* js */"), cssA))
	u := "https://dumber.invalid/systemviews.css?v=" + digestOf(cssA)
	require.Equal(t, immutableCacheControl, staticHandlerOf(t, h.handleAsset(mustParseURL(t, u))).headers["Cache-Control"])

	cssB := []byte("/* css b */")
	h.setAssets(trueManifestFS(t, []byte("\x00asm"), []byte("/* js */"), cssB))
	// The old version no longer validates against the captured bundle.
	old := staticHandlerOf(t, h.handleAsset(mustParseURL(t, u)))
	require.Equal(t, http.StatusNotFound, old.statusCode)
	require.Equal(t, noStoreCacheControl, old.headers["Cache-Control"])
	fresh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, "https://dumber.invalid/systemviews.css?v="+digestOf(cssB))))
	require.Equal(t, http.StatusOK, fresh.statusCode)
	require.Equal(t, immutableCacheControl, fresh.headers["Cache-Control"])
}

func TestAssetP33_ManifestItselfServesNoStore(t *testing.T) {
	stubIdentityResourceHandler(t)
	css := []byte("/* css */")
	h := newTestDumbSchemeHandler(t)
	h.setAssets(trueManifestFS(t, []byte("\x00asm"), []byte("/* js */"), css))

	// The manifest is servable but never allowlisted: it stays fresh and
	// noncacheable even with a version query.
	for _, raw := range []string{
		"https://dumber.invalid/asset-manifest.json",
		"https://dumber.invalid/asset-manifest.json?v=anything",
	} {
		rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, raw)))
		require.Equal(t, http.StatusOK, rh.statusCode, raw)
		require.Equal(t, noStoreCacheControl, rh.headers["Cache-Control"], raw)
	}
}

func TestAssetP33_ShellVersionQueryIgnored(t *testing.T) {
	stubIdentityResourceHandler(t)
	h := newTestDumbSchemeHandler(t)
	h.setAssets(shellBundleFS(t, versionedShellFixture))

	// The shell document is never immutable: a ?v= on the shell serves the
	// normal noncacheable shell instead of erroring or pinning.
	rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, "https://dumber.invalid/history?v=whatever")))
	require.Equal(t, http.StatusOK, rh.statusCode)
	require.Equal(t, noStoreCacheControl, rh.headers["Cache-Control"])
	require.Contains(t, string(rh.data), "?v=")
}
