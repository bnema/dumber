package cef

// P2.2/P2.3 tests: the handler serves the WASM from its captured bundle
// through a deferred handler that never blocks CEF's IO thread.
// Replacement installs a fresh isolated state; in-flight requests keep the
// old bytes; canceled waiters suppress only their own continuation.

import (
	"bytes"
	"context"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/andybalholm/brotli"
	purecef "github.com/bnema/purego-cef/cef"
	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/assets"
)

// blockingReadFileFS blocks ReadFile of one name until release is closed,
// proving non-blocking Create and replacement isolation during a decode.
type blockingReadFileFS struct {
	inner     fstest.MapFS
	blockName string
	entered   chan struct{}
	release   chan struct{}
	once      sync.Once
}

func (b *blockingReadFileFS) Open(name string) (fs.File, error) {
	return b.inner.Open(name)
}

func (b *blockingReadFileFS) ReadFile(name string) ([]byte, error) {
	if name == b.blockName {
		b.once.Do(func() { close(b.entered) })
		<-b.release
	}
	return b.inner.ReadFile(name)
}

// contStubCallback counts native continuations without mock machinery.
type contStubCallback struct {
	mu    sync.Mutex
	conts int
}

func (c *contStubCallback) Cont() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conts++
}

func (c *contStubCallback) Cancel() {}

func (c *contStubCallback) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conts
}

// testManifestBytes builds a parseable manifest fixture pinning dummy
// digests. Serving validates presence and format, not truth: digest truth
// is P3.3's job against real artifacts.
func testManifestBytes(t *testing.T) []byte {
	t.Helper()
	data, err := assets.Manifest{Version: assets.ManifestVersion, Files: map[string]assets.FileEntry{
		"systemviews.css":  {SHA256: strings.Repeat("a", 64), Size: 1},
		"systemviews.wasm": {SHA256: strings.Repeat("b", 64), Size: 2},
		"wasm_exec.js":     {SHA256: strings.Repeat("c", 64), Size: 3},
	}}.Bytes()
	require.NoError(t, err)
	return data
}

func wasmURL(t *testing.T) string {
	t.Helper()
	return "https://dumber.invalid/systemviews.wasm"
}

// awaitWASMHandler drives one deferred WASM response to completion on the
// test goroutine. Open must return promptly even while the decode is
// blocked: that prompt return is the P2.3 IO-thread guarantee.
func awaitWASMHandler(t *testing.T, rh purecef.ResourceHandler) *systemviewWASMResourceHandler {
	t.Helper()
	wh, ok := rh.(*systemviewWASMResourceHandler)
	require.True(t, ok, "expected deferred WASM handler, got %T", rh)
	opened := make(chan int32, 1)
	go func() { opened <- wh.Open(nil, nil, nil) }()
	select {
	case v := <-opened:
		require.Equal(t, int32(1), v)
	case <-time.After(10 * time.Second):
		t.Fatal("Open blocked: the CEF IO thread would stall")
	}
	select {
	case <-wh.done:
	case <-time.After(30 * time.Second):
		t.Fatal("WASM load did not complete")
	}
	return wh
}

func TestAssetP2_HandlerServesWASMFromBundleOnce(t *testing.T) {
	stubIdentityResourceHandler(t)
	wasm := []byte("\x00asm-handler-once")
	fsys := &countingFS{inner: fstest.MapFS{
		"systemviews/systemviews.wasm.br": {Data: brotliCompressForTest(t, wasm)},
	}}
	h := newTestDumbSchemeHandler(t)
	h.setAssets(fsys)

	for i := range 2 {
		wh := awaitWASMHandler(t, h.handleAsset(mustParseURL(t, wasmURL(t))))
		require.Equal(t, http.StatusOK, wh.statusCode, "request %d", i)
		require.Equal(t, wasm, wh.data)
	}
	require.Equal(t, 1, fsys.compressedReads(),
		"handler must share the bundle decode across requests")
}

func TestAssetP2_ReplacementDuringBlockedDecode(t *testing.T) {
	stubIdentityResourceHandler(t)
	wasmA := []byte("\x00asm-bundle-a")
	fsA := &blockingReadFileFS{
		inner: fstest.MapFS{
			"systemviews/systemviews.wasm.br": {Data: brotliCompressForTest(t, wasmA)},
		},
		blockName: "systemviews/systemviews.wasm.br",
		entered:   make(chan struct{}),
		release:   make(chan struct{}),
	}
	h := newTestDumbSchemeHandler(t)
	h.setAssets(fsA)

	u := mustParseURL(t, wasmURL(t))
	created := make(chan purecef.ResourceHandler, 1)
	go func() { created <- h.handleAsset(u) }()

	var oldRH purecef.ResourceHandler
	select {
	case oldRH = <-created:
	case <-time.After(10 * time.Second):
		t.Fatal("Create blocked on decode: the CEF IO thread would stall")
	}
	oldWH, ok := oldRH.(*systemviewWASMResourceHandler)
	require.True(t, ok)
	opened := make(chan int32, 1)
	go func() { opened <- oldWH.Open(nil, nil, nil) }()
	select {
	case <-opened:
	case <-time.After(10 * time.Second):
		t.Fatal("Open blocked on decode")
	}
	<-fsA.entered

	// Replace the bundle while A is still decoding: the in-flight request
	// must keep A's bytes and never publish into the new cache.
	wasmB := []byte("\x00asm-bundle-b")
	h.setAssets(fstest.MapFS{
		"systemviews/systemviews.wasm.br": {Data: brotliCompressForTest(t, wasmB)},
	})
	close(fsA.release)

	select {
	case <-oldWH.done:
	case <-time.After(30 * time.Second):
		t.Fatal("old load did not complete after release")
	}
	require.Equal(t, http.StatusOK, oldWH.statusCode)
	require.Equal(t, wasmA, oldWH.data, "in-flight request must retain the replaced bundle's bytes")

	fresh := awaitWASMHandler(t, h.handleAsset(u))
	require.Equal(t, http.StatusOK, fresh.statusCode)
	require.Equal(t, wasmB, fresh.data, "post-replacement requests must serve the new bundle")
}

func TestAssetP2_UnconfiguredAssetsServe500(t *testing.T) {
	stubIdentityResourceHandler(t)
	h := newTestDumbSchemeHandler(t)

	rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, wasmURL(t))))
	require.Equal(t, http.StatusInternalServerError, rh.statusCode)

	// Installing nil stays a clean 500, never a panic.
	h.setAssets(nil)
	rh = staticHandlerOf(t, h.handleAsset(mustParseURL(t, wasmURL(t))))
	require.Equal(t, http.StatusInternalServerError, rh.statusCode)
}

func TestAssetP2_NonWASMAssetsKeepLegacyPath(t *testing.T) {
	stubIdentityResourceHandler(t)
	h := newTestDumbSchemeHandler(t)
	h.setAssets(fstest.MapFS{
		"systemviews/index.html":             {Data: []byte(`<html><script src="./wasm_exec.js"></script></html>`)},
		"systemviews/" + assets.ManifestName: {Data: testManifestBytes(t)},
	})

	rh := staticHandlerOf(t, h.handleAsset(mustParseURL(t, "https://dumber.invalid/history")))
	require.Equal(t, http.StatusOK, rh.statusCode)
	require.Contains(t, string(rh.data), "./wasm_exec.js?v="+strings.Repeat("c", 64))
}

func TestAssetP23_ManifestReadableDuringBlockedDecode(t *testing.T) {
	stubIdentityResourceHandler(t)
	wasm := []byte("\x00asm-manifest-race")
	blocking := &blockingReadFileFS{
		inner: fstest.MapFS{
			"systemviews/systemviews.wasm.br":    {Data: brotliCompressForTest(t, wasm)},
			"systemviews/index.html":             {Data: []byte(versionedShellFixture)},
			"systemviews/" + assets.ManifestName: {Data: testManifestBytes(t)},
		},
		blockName: "systemviews/systemviews.wasm.br",
		entered:   make(chan struct{}),
		release:   make(chan struct{}),
	}
	h := newTestDumbSchemeHandler(t)
	h.setAssets(blocking)

	// Start a wasm decode and leave it blocked off-thread.
	firstWH := h.handleAsset(mustParseURL(t, wasmURL(t))).(*systemviewWASMResourceHandler)
	require.Equal(t, int32(1), firstWH.Open(nil, nil, nil))
	<-blocking.entered

	// The shell consults the manifest synchronously on the request path:
	// it must serve promptly instead of deadlocking behind the decode.
	served := make(chan purecef.ResourceHandler, 1)
	go func() { served <- h.handleAsset(mustParseURL(t, "https://dumber.invalid/history")) }()
	select {
	case rh := <-served:
		shell := staticHandlerOf(t, rh)
		require.Equal(t, http.StatusOK, shell.statusCode)
		require.Contains(t, string(shell.data), "?v=")
	case <-time.After(10 * time.Second):
		t.Fatal("shell deadlocked behind in-flight decode")
	}

	close(blocking.release)
	select {
	case <-firstWH.done:
	case <-time.After(30 * time.Second):
		t.Fatal("WASM load did not complete after release")
	}
	require.Equal(t, wasm, firstWH.data)
}

func TestAssetP23_BlockedDecodeLeavesUnrelatedWorkUnaffected(t *testing.T) {
	stubIdentityResourceHandler(t)
	wasm := []byte("\x00asm-unrelated")
	blocking := &blockingReadFileFS{
		inner: fstest.MapFS{
			"systemviews/systemviews.wasm.br":    {Data: brotliCompressForTest(t, wasm)},
			"systemviews/index.html":             {Data: []byte("<html>shell</html>")},
			"systemviews/" + assets.ManifestName: {Data: testManifestBytes(t)},
		},
		blockName: "systemviews/systemviews.wasm.br",
		entered:   make(chan struct{}),
		release:   make(chan struct{}),
	}
	h := newTestDumbSchemeHandler(t)
	h.setAssets(blocking)

	// Shell traffic before any WASM request must not decode.
	shell := staticHandlerOf(t, h.handleAsset(mustParseURL(t, "https://dumber.invalid/history")))
	require.Equal(t, http.StatusOK, shell.statusCode)

	u := mustParseURL(t, wasmURL(t))
	created := make(chan purecef.ResourceHandler, 1)
	go func() { created <- h.handleAsset(u) }()
	var firstRH purecef.ResourceHandler
	select {
	case firstRH = <-created:
	case <-time.After(10 * time.Second):
		t.Fatal("Create blocked on decode")
	}
	firstWH := firstRH.(*systemviewWASMResourceHandler)
	opened := make(chan int32, 1)
	go func() { opened <- firstWH.Open(nil, nil, nil) }()
	select {
	case <-opened:
	case <-time.After(10 * time.Second):
		t.Fatal("Open blocked on decode")
	}
	<-blocking.entered

	// A second waiter joins the same in-flight decode while an unrelated
	// API request completes without touching the decoder.
	secondWH := h.handleAsset(u).(*systemviewWASMResourceHandler)
	require.Equal(t, int32(1), secondWH.Open(nil, nil, nil))
	apiRH := staticHandlerOf(t, h.handleAPI(nil, http.MethodOptions, "/api/message", nil))
	require.Equal(t, http.StatusNoContent, apiRH.statusCode)

	select {
	case <-firstWH.done:
		t.Fatal("decode completed while still blocked")
	case <-time.After(100 * time.Millisecond):
	}

	close(blocking.release)
	for _, wh := range []*systemviewWASMResourceHandler{firstWH, secondWH} {
		select {
		case <-wh.done:
		case <-time.After(30 * time.Second):
			t.Fatal("WASM load did not complete after release")
		}
		require.Equal(t, http.StatusOK, wh.statusCode)
		require.Equal(t, wasm, wh.data, "both waiters share the one decode")
	}
}

func TestAssetP23_CancelSuppressesContinuation(t *testing.T) {
	stubIdentityResourceHandler(t)
	wasm := []byte("\x00asm-cancel")
	blocking := &blockingReadFileFS{
		inner: fstest.MapFS{
			"systemviews/systemviews.wasm.br": {Data: brotliCompressForTest(t, wasm)},
		},
		blockName: "systemviews/systemviews.wasm.br",
		entered:   make(chan struct{}),
		release:   make(chan struct{}),
	}
	h := newTestDumbSchemeHandler(t)
	h.setAssets(blocking)

	cb := &contStubCallback{}
	wh := h.handleAsset(mustParseURL(t, wasmURL(t))).(*systemviewWASMResourceHandler)
	require.Equal(t, int32(1), wh.Open(nil, nil, cb))
	<-blocking.entered

	// Canceling one waiter suppresses only its own continuation; the shared
	// decode still completes for the bundle.
	wh.Cancel()
	close(blocking.release)
	select {
	case <-wh.done:
	case <-time.After(30 * time.Second):
		t.Fatal("canceled load did not finish")
	}
	require.Equal(t, http.StatusOK, wh.statusCode)
	require.Equal(t, wasm, wh.data)
	require.Zero(t, cb.count(), "canceled waiter must not resume CEF")

	// A later waiter on the same bundle is served from the completed decode
	// and resumes normally.
	cb2 := &contStubCallback{}
	wh2 := h.handleAsset(mustParseURL(t, wasmURL(t))).(*systemviewWASMResourceHandler)
	require.Equal(t, int32(1), wh2.Open(nil, nil, cb2))
	select {
	case <-wh2.done:
	case <-time.After(30 * time.Second):
		t.Fatal("post-cancel waiter did not complete")
	}
	require.Equal(t, wasm, wh2.data)
	require.Equal(t, 1, cb2.count(), "later waiter must resume CEF normally")
}

func TestAssetP23_ShutdownCompletesSafely(t *testing.T) {
	stubIdentityResourceHandler(t)
	wasm := []byte("\x00asm-shutdown")
	blocking := &blockingReadFileFS{
		inner: fstest.MapFS{
			"systemviews/systemviews.wasm.br": {Data: brotliCompressForTest(t, wasm)},
		},
		blockName: "systemviews/systemviews.wasm.br",
		entered:   make(chan struct{}),
		release:   make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	h, err := newDumbSchemeHandler(ctx, nil, noopConfigPayload(), noopConfigPayload())
	require.NoError(t, err)
	h.setAssets(blocking)

	wh := h.handleAsset(mustParseURL(t, wasmURL(t))).(*systemviewWASMResourceHandler)
	require.Equal(t, int32(1), wh.Open(nil, nil, nil))
	<-blocking.entered

	// Engine shutdown mid-decode: the bounded CPU decode still finishes
	// without hanging or panicking; nobody waits on it past milliseconds.
	cancel()
	close(blocking.release)
	select {
	case <-wh.done:
	case <-time.After(30 * time.Second):
		t.Fatal("load hung across shutdown")
	}
	require.Equal(t, http.StatusOK, wh.statusCode)
	require.Equal(t, wasm, wh.data)
}

func TestAssetP23_WireUnversionedCarriesNoStore(t *testing.T) {
	stubIdentityResourceHandler(t)
	wasm := []byte("\x00asm-wire")
	h := newTestDumbSchemeHandler(t)
	h.setAssets(fstest.MapFS{
		"systemviews/systemviews.wasm.br": {Data: brotliCompressForTest(t, wasm)},
	})

	wh := awaitWASMHandler(t, h.handleAsset(mustParseURL(t, wasmURL(t))))

	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusOK)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusOK)).Once()
	response.EXPECT().SetMimeType("application/wasm").Once()
	// Unversioned successes stay explicitly noncacheable; no immutable
	// header may appear without a verified version.
	response.EXPECT().SetHeaderByName("Cache-Control", noStoreCacheControl, int32(1)).Once()
	var responseLength int64
	wh.GetResponseHeaders(response, &responseLength, 0)
	require.Equal(t, int64(len(wasm)), responseLength)
}

func TestAssetP23_WireFailureMatchesErrorPage(t *testing.T) {
	stubIdentityResourceHandler(t)
	h := newTestDumbSchemeHandler(t)
	h.setAssets(fstest.MapFS{
		"systemviews/systemviews.wasm.br": {Data: []byte("not-brotli")},
	})

	wh := awaitWASMHandler(t, h.handleAsset(mustParseURL(t, wasmURL(t))))
	require.Equal(t, http.StatusNotFound, wh.statusCode)
	require.Equal(t, errorPageBody(http.StatusNotFound, "Asset not found"), wh.data,
		"deferred failure must stay byte-identical to the synchronous error page")

	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusNotFound)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusNotFound)).Once()
	response.EXPECT().SetMimeType("text/html").Once()
	response.EXPECT().SetCharset("utf-8").Once()
	var responseLength int64
	wh.GetResponseHeaders(response, &responseLength, 0)
	require.Equal(t, int64(len(wh.data)), responseLength)
}

// wasmBenchFixture builds a 2 MiB pseudo-WASM payload; zeros keep brotli
// fast so benchmarks measure plumbing, not compression ratios.
func wasmBenchFixture() []byte {
	return bytes.Repeat([]byte("\x00asm-bench"), 256*1024)
}

func brotliCompressForBench(b *testing.B, data []byte) []byte {
	b.Helper()
	var buf bytes.Buffer
	w := brotli.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		b.Fatalf("brotli write: %v", err)
	}
	if err := w.Close(); err != nil {
		b.Fatalf("brotli close: %v", err)
	}
	return buf.Bytes()
}

// BenchmarkSystemviewWASMFirstLoad measures one cold bundle decode; the
// cached benchmark below measures the shared path every request takes.
func BenchmarkSystemviewWASMFirstLoad(b *testing.B) {
	wasm := wasmBenchFixture()
	compressed := brotliCompressForBench(b, wasm)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		bundle := newSystemviewAssetBundle(fstest.MapFS{
			"systemviews/systemviews.wasm.br": {Data: compressed},
		})
		b.StartTimer()
		data, err := bundle.WASM()
		if err != nil || len(data) != len(wasm) {
			b.Fatalf("load: %v", err)
		}
	}
}

func BenchmarkSystemviewWASMCached(b *testing.B) {
	wasm := wasmBenchFixture()
	bundle := newSystemviewAssetBundle(fstest.MapFS{
		"systemviews/systemviews.wasm.br": {Data: brotliCompressForBench(b, wasm)},
	})
	if _, err := bundle.WASM(); err != nil {
		b.Fatalf("warmup load: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := bundle.WASM()
		if err != nil || len(data) != len(wasm) {
			b.Fatalf("load: %v", err)
		}
	}
}
