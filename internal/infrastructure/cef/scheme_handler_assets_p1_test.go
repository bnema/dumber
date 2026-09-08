package cef

// P1 characterization for Plan 05 (internal systemview startup).
//
// These tests lock the CURRENT asset behavior before any caching work:
// every WASM request fully re-reads and re-decompresses, and most response
// classes carry no Cache-Control at all. They are the failing regressions
// the P2 bundle cache and P3 versioned headers must turn green-side up
// (decode-once, immutable-where-versioned) without changing the error,
// traversal, or privacy semantics asserted here.

import (
	"io/fs"
	"net/http"
	"sync"
	"testing"
	"testing/fstest"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/assets"
)

// decodeCountingFS counts full asset reads: with no cache, one served WASM
// request equals one decompression. The counter lives outside production
// code; P2 replaces call-counting with a real decode-once assertion.
type decodeCountingFS struct {
	fstest.MapFS
	mu    sync.Mutex
	reads map[string]int
}

func (d *decodeCountingFS) record(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.reads == nil {
		d.reads = map[string]int{}
	}
	d.reads[name]++
}

func (d *decodeCountingFS) count(name string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.reads[name]
}

func countingWASMFS(t *testing.T, wasm []byte) *decodeCountingFS {
	t.Helper()
	return &decodeCountingFS{
		MapFS: fstest.MapFS{
			"systemviews/systemviews.wasm.br": {Data: brotliCompressForTest(t, wasm)},
		},
	}
}

func TestAssetP1_RepeatedReadsRedundantlyDecode(t *testing.T) {
	t.Parallel()

	wasm := []byte("\x00asm-redundant-fixture-p1")
	fsys := countingWASMFS(t, wasm)

	const requests = 3
	got := make([][]byte, requests)
	for i := range got {
		data, err := readAssetWithEncoding(fsys, "systemviews/systemviews.wasm", "systemviews.wasm")
		require.NoError(t, err)
		fsys.record("systemviews/systemviews.wasm.br")
		got[i] = data
	}
	for _, data := range got[1:] {
		require.Equal(t, got[0], data, "repeated requests must serve identical bytes")
	}
	require.Equal(t, requests, fsys.count("systemviews/systemviews.wasm.br"),
		"P1 baseline: every request re-reads the compressed asset; P2 must decode once")
}

func TestAssetP1_ConcurrentReadsServeIdenticalBytes(t *testing.T) {
	t.Parallel()

	wasm := []byte("\x00asm-concurrent-fixture-p1")
	fsys := countingWASMFS(t, wasm)

	const readers = 8
	got := make([][]byte, readers)
	errs := make([]error, readers)
	var wg sync.WaitGroup
	for i := range got {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got[i], errs[i] = readAssetWithEncoding(fsys, "systemviews/systemviews.wasm", "systemviews.wasm")
		}(i)
	}
	wg.Wait()
	for i := range got {
		require.NoError(t, errs[i])
		require.Equal(t, wasm, got[i])
	}
}

func TestAssetP1_CompressedPreferredOverRaw(t *testing.T) {
	t.Parallel()

	fromCompressed := []byte("\x00asm-from-br")
	fromRaw := []byte("\x00asm-from-raw")
	fsys := fstest.MapFS{
		"systemviews/systemviews.wasm.br": {Data: brotliCompressForTest(t, fromCompressed)},
		"systemviews/systemviews.wasm":    {Data: fromRaw},
	}

	data, err := readAssetWithEncoding(fsys, "systemviews/systemviews.wasm", "systemviews.wasm")
	require.NoError(t, err)
	require.Equal(t, fromCompressed, data, "compressed artifact wins when present")
}

func TestAssetP1_RawOnlyFallback(t *testing.T) {
	t.Parallel()

	raw := []byte("\x00asm-raw-only")
	fsys := fstest.MapFS{
		"systemviews/systemviews.wasm": {Data: raw},
	}

	data, err := readAssetWithEncoding(fsys, "systemviews/systemviews.wasm", "systemviews.wasm")
	require.NoError(t, err)
	require.Equal(t, raw, data)
}

func TestAssetP1_CorruptCompressedHasNoRawFallback(t *testing.T) {
	t.Parallel()

	// Current semantics: any successfully read .br is authoritative. A
	// corrupt compressed artifact is an explicit error even when valid raw
	// data exists. P2 keeps this tightened behavior.
	fsys := fstest.MapFS{
		"systemviews/systemviews.wasm.br": {Data: []byte("not-brotli-data")},
		"systemviews/systemviews.wasm":    {Data: []byte("\x00asm-valid-raw")},
	}

	_, err := readAssetWithEncoding(fsys, "systemviews/systemviews.wasm", "systemviews.wasm")
	require.Error(t, err, "corrupt compressed WASM must fail explicitly, not fall back to raw")
}

func TestAssetP1_OversizedDecompressedRejected(t *testing.T) {
	t.Parallel()

	// Zeros compress to almost nothing, so this stays fast while proving
	// the decompressed-size bound.
	big := make([]byte, maxSystemviewsWASMBytes+1)
	fsys := fstest.MapFS{
		"systemviews/systemviews.wasm.br": {Data: brotliCompressForTest(t, big)},
	}

	_, err := readAssetWithEncoding(fsys, "systemviews/systemviews.wasm", "systemviews.wasm")
	require.Error(t, err, "decompressed output past the 64 MiB bound must fail")
}

func TestAssetP1_RawReadCurrentlyUnbounded(t *testing.T) {
	t.Parallel()

	// P1 baseline: the raw path has no size bound at all. P2.1 deliberately
	// tightens this alongside the compressed path.
	big := make([]byte, maxSystemviewsWASMBytes+1)
	fsys := fstest.MapFS{
		"systemviews/systemviews.wasm": {Data: big},
	}

	data, err := readAssetWithEncoding(fsys, "systemviews/systemviews.wasm", "systemviews.wasm")
	require.NoError(t, err)
	require.Len(t, data, maxSystemviewsWASMBytes+1, "raw reads are unbounded at baseline")
}

func TestAssetP1_MissingAssetReportsNotExist(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{}

	_, err := readAssetWithEncoding(fsys, "systemviews/systemviews.wasm", "systemviews.wasm")
	require.Error(t, err)
	require.ErrorIs(t, err, fs.ErrNotExist, "missing asset must surface fs.ErrNotExist")
}

// staticHandlerOf unwraps the identity-stubbed resource handler. Callers
// must stub cefNewResourceHandler to the identity function first.
func staticHandlerOf(t *testing.T, rh any) *staticResourceHandler {
	t.Helper()
	h, ok := rh.(*staticResourceHandler)
	require.True(t, ok, "expected *staticResourceHandler, got %T", rh)
	return h
}

func stubIdentityResourceHandler(t *testing.T) {
	t.Helper()
	old := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	t.Cleanup(func() { cefNewResourceHandler = old })
}

// P1.2 response classes: exactly where no-store is applied today. Asset
// success, error pages, and the crash shell carry NO Cache-Control header
// at baseline; P3 grants immutable headers only to version-validated
// static bytes.

func TestAssetP1_ShellServes200WithoutCacheHeaders(t *testing.T) {
	stubIdentityResourceHandler(t)
	h := newTestDumbSchemeHandler(t)
	h.setAssets(assets.WebUIAssets)

	rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, "https://dumber.invalid/history")))
	require.Equal(t, http.StatusOK, rh.statusCode)
	require.Equal(t, "text/html; charset=utf-8", rh.contentType)
	require.NotEmpty(t, rh.data)
	require.Empty(t, rh.headers, "P1 baseline: shell carries no Cache-Control; P3 versions it")
}

func TestAssetP1_CSSServes200WithoutCacheHeaders(t *testing.T) {
	stubIdentityResourceHandler(t)
	h := newTestDumbSchemeHandler(t)
	h.setAssets(assets.WebUIAssets)

	rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, "https://dumber.invalid/systemviews.css")))
	require.Equal(t, http.StatusOK, rh.statusCode)
	require.Equal(t, "text/css; charset=utf-8", rh.contentType, "Go mime table carries the charset suffix")
	require.Empty(t, rh.headers, "P1 baseline: CSS carries no Cache-Control; P3 versions it")
}

func TestAssetP1_MissingAssetServes404WithoutCacheHeaders(t *testing.T) {
	stubIdentityResourceHandler(t)
	h := newTestDumbSchemeHandler(t)
	h.setAssets(assets.WebUIAssets)

	rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, "https://dumber.invalid/does-not-exist.js")))
	require.Equal(t, http.StatusNotFound, rh.statusCode)
	require.Empty(t, rh.headers, "error pages carry no Cache-Control at baseline")
}

func TestAssetP1_RedirectCarriesNoStore(t *testing.T) {
	stubIdentityResourceHandler(t)
	h := newTestDumbSchemeHandler(t)

	rh := staticHandlerOf(t, h.newRedirectResourceHandler(http.StatusTemporaryRedirect, "https://dumber.invalid/history"))
	require.Equal(t, http.StatusTemporaryRedirect, rh.statusCode)
	require.Equal(t, "https://dumber.invalid/history", rh.headers["Location"])
	require.Equal(t, "no-store", rh.headers["Cache-Control"])
}

func TestAssetP1_RawAndErrorHandlersCarryNoCacheHeaders(t *testing.T) {
	stubIdentityResourceHandler(t)
	h := newTestDumbSchemeHandler(t)

	require.Empty(t, staticHandlerOf(t, h.newRawResourceHandler(http.StatusOK, "text/html; charset=utf-8", []byte("<p>x</p>"))).headers)
	require.Empty(t, staticHandlerOf(t, h.newErrorResourceHandler(http.StatusInternalServerError, "boom")).headers,
		"error pages carry no Cache-Control at baseline")
}

func TestAssetP1_APIRawAndJSONCarryNoStoreCORS(t *testing.T) {
	stubIdentityResourceHandler(t)
	h := newTestDumbSchemeHandler(t)

	raw := staticHandlerOf(t, h.newAPIRawResourceHandler(http.StatusOK, "application/json", []byte(`{}`)))
	require.Equal(t, "no-store", raw.headers["Cache-Control"])
	require.Equal(t, "*", raw.headers["Access-Control-Allow-Origin"])

	js := staticHandlerOf(t, h.newAPIJSONResourceHandler(http.StatusNotFound, map[string]string{"error": "not found"}))
	require.Equal(t, http.StatusNotFound, js.statusCode)
	require.Equal(t, "application/json", js.contentType)
	require.Equal(t, "no-store", js.headers["Cache-Control"])
	require.Equal(t, "*", js.headers["Access-Control-Allow-Origin"])
	require.JSONEq(t, `{"error":"not found"}`, string(js.data))
}

func TestAssetP1_PrivateAPICarriesNoStoreWithoutCORS(t *testing.T) {
	stubIdentityResourceHandler(t)
	h := newTestDumbSchemeHandler(t)

	raw := staticHandlerOf(t, h.newPrivateAPIRawResourceHandler(http.StatusNoContent, "text/plain; charset=utf-8", nil))
	require.Equal(t, "no-store", raw.headers["Cache-Control"])
	require.NotContains(t, raw.headers, "Access-Control-Allow-Origin")

	js := staticHandlerOf(t, h.newPrivateAPIJSONResourceHandler(http.StatusForbidden, map[string]string{"error": "forbidden"}))
	require.Equal(t, "no-store", js.headers["Cache-Control"])
	require.NotContains(t, js.headers, "Access-Control-Allow-Origin")
}

func TestAssetP1_FaviconHandlerCarriesNoStore(t *testing.T) {
	stubIdentityResourceHandler(t)

	rh := newFaviconResourceHandler(t.Context(), nil, "example.com", 32)
	fav, ok := rh.(*faviconResourceHandler)
	require.True(t, ok, "expected *faviconResourceHandler, got %T", rh)
	require.Equal(t, "no-store", fav.headers["Cache-Control"])
}

func TestAssetP1_TopLevelAPIPathNeverResolvesAsAsset(t *testing.T) {
	for _, raw := range []string{
		"https://dumber.invalid/api/config",
		"https://dumber.invalid/api/message",
	} {
		_, _, ok := resolveAssetPath(mustParseURL(t, raw))
		require.False(t, ok, "%s must not resolve as an asset", raw)
	}
}

func TestAssetP1_NestedAPIPathFallsThroughTo404(t *testing.T) {
	stubIdentityResourceHandler(t)
	h := newTestDumbSchemeHandler(t)
	h.setAssets(assets.WebUIAssets)

	// handleAPI routes real API traffic before handleAsset; a nested api/
	// path reaching the asset layer must 404, never serve asset bytes.
	rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, "https://dumber.invalid/history/api/message")))
	require.Equal(t, http.StatusNotFound, rh.statusCode)
}
