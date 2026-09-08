package cef

// Bundle-scoped immutable systemviews WASM cache (Plan 05 P2.1).
//
// Only the successfully decoded immutable systemviews WASM is cached, one
// entry per asset bundle. This is deliberately NOT a general web response
// cache: shell HTML, JS/CSS, API responses, and error pages are never
// cached here.
//
// Hardened load semantics (tighter than the legacy readAssetWithEncoding):
//   - A present compressed artifact is authoritative. Corrupt or oversized
//     output is an explicit error with no raw fallback.
//   - The raw fallback applies ONLY when the compressed file is absent
//     (errors.Is(err, fs.ErrNotExist)). Permission/I/O failures reading
//     the compressed file are explicit errors even if valid raw data
//     exists.
//   - Raw reads are size-bounded before allocation, like the decompressed
//     path.
//   - The first outcome (success or failure) is cached for the bundle
//     lifetime, so a corrupt bundle fails fast instead of storming
//     retries. Production embeds are immutable, so a cached failure cannot
//     go stale; bundle replacement installs a fresh cache (P2.2).
//
// Returned bytes are immutable: callers must not mutate them.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/andybalholm/brotli"
	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/assets"
)

// systemviewsWASMPath is the fixed bundle-relative WASM served to internal
// pages. Only this file is cached; nothing else enters this cache.
const systemviewsWASMPath = "systemviews/systemviews.wasm"

// systemviewAssetBundle is one immutable asset state: an asset filesystem
// plus its once-loaded WASM outcome. In-flight requests retain their
// captured bundle; installing a different bundle replaces the whole state.
// WASM bytes and manifest load under independent mutexes: a stalled decode
// must never stall manifest reads on the request path, since handleAsset
// consults the manifest synchronously while decodes run off-thread.
type systemviewAssetBundle struct {
	assets fs.FS
	wasmMu sync.Mutex
	loaded bool
	data   []byte
	err    error
	// manifestLoaded caches the parsed content manifest alongside the
	// WASM: shell versioning and immutable headers always validate
	// against the same captured bundle state, never a newer install.
	manifestMu     sync.Mutex
	manifestLoaded bool
	manifest       assets.Manifest
	manifestErr    error
}

// newSystemviewAssetBundle captures one immutable asset filesystem. A nil
// FS is rejected at load time with a plain error, never a panic.
func newSystemviewAssetBundle(assetsFS fs.FS) *systemviewAssetBundle {
	return &systemviewAssetBundle{assets: assetsFS}
}

// WASM returns the bundle's decoded WASM, loading it exactly once. Success
// and failure are both cached: repeated callers share the first outcome
// without re-reading or re-decoding.
func (b *systemviewAssetBundle) WASM() ([]byte, error) {
	b.wasmMu.Lock()
	defer b.wasmMu.Unlock()
	if !b.loaded {
		b.data, b.err = loadSystemviewWASM(b.assets)
		b.loaded = true
	}
	return b.data, b.err
}

// Manifest returns the bundle's parsed content manifest, loading it
// exactly once. Success and failure are both cached like the WASM bytes:
// an invalid manifest fails closed for the bundle lifetime instead of
// flapping between error and unverified content.
func (b *systemviewAssetBundle) Manifest() (assets.Manifest, error) {
	b.manifestMu.Lock()
	defer b.manifestMu.Unlock()
	if !b.manifestLoaded {
		b.manifest, b.manifestErr = loadSystemviewManifest(b.assets)
		b.manifestLoaded = true
	}
	return b.manifest, b.manifestErr
}

// loadSystemviewManifest reads and strictly validates the bundle manifest.
// A missing, corrupt, or stale manifest is a plain error: callers serve a
// clear noncacheable error, never unverified immutable content.
func loadSystemviewManifest(assetsFS fs.FS) (assets.Manifest, error) {
	if assetsFS == nil {
		return assets.Manifest{}, errors.New("systemview assets not configured")
	}
	data, err := fs.ReadFile(assetsFS, "systemviews/"+assets.ManifestName)
	if err != nil {
		return assets.Manifest{}, fmt.Errorf("read asset manifest: %w", err)
	}
	return assets.ParseManifest(data)
}

// loadSystemviewWASM reads and decodes the bundle WASM with the hardened
// semantics documented above. It performs no caching itself.
func loadSystemviewWASM(assetsFS fs.FS) ([]byte, error) {
	if assetsFS == nil {
		return nil, errors.New("systemview assets not configured")
	}
	compressed, err := fs.ReadFile(assetsFS, systemviewsWASMPath+".br")
	if err == nil {
		data, decodeErr := io.ReadAll(io.LimitReader(brotli.NewReader(bytes.NewReader(compressed)), maxSystemviewsWASMBytes+1))
		if decodeErr != nil {
			return nil, fmt.Errorf("decompress systemview WASM: %w", decodeErr)
		}
		if len(data) > maxSystemviewsWASMBytes {
			return nil, fmt.Errorf("decompressed systemview WASM exceeds %d bytes", maxSystemviewsWASMBytes)
		}
		return data, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read compressed systemview WASM: %w", err)
	}
	return readBoundedAsset(assetsFS, systemviewsWASMPath)
}

// readBoundedAsset reads one raw asset with the same size bound as the
// decompressed path, enforced before unbounded allocation.
func readBoundedAsset(assetsFS fs.FS, name string) ([]byte, error) {
	f, err := assetsFS.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxSystemviewsWASMBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read systemview asset %s: %w", name, err)
	}
	if len(data) > maxSystemviewsWASMBytes {
		return nil, fmt.Errorf("systemview asset %s exceeds %d bytes", name, maxSystemviewsWASMBytes)
	}
	return data, nil
}

// ---------------------------------------------------------------------------
// Deferred WASM resource handler (Plan 05 P2.3)
// ---------------------------------------------------------------------------

// systemviewWASMResourceHandler serves one WASM request from a captured
// bundle without blocking CEF's IO thread, following the existing async
// favicon handler pattern: Open/ProcessRequest return immediately, the
// bundle-owned once-loading runs on a worker goroutine, and CEF is resumed
// via the retained callback. The decode itself is bounded CPU work (at most
// maxSystemviewsWASMBytes of brotli output), so no handler-wide lock is
// ever held across it and shutdown never waits on it beyond milliseconds.
// Canceling one request suppresses only its own continuation; other bundle
// waiters share the same decoded bytes unaffected. A versioned request pins
// exact bytes: only output hashing to wantDigest earns immutable headers.
type systemviewWASMResourceHandler struct {
	cancel      context.CancelFunc
	cancelOnce  sync.Once
	canceled    atomic.Bool
	bundle      *systemviewAssetBundle
	versioned   bool
	wantDigest  string
	once        sync.Once
	done        chan struct{}
	data        []byte
	contentType string
	statusCode  int
	offset      int
}

func newSystemviewWASMResourceHandler(
	ctx context.Context, bundle *systemviewAssetBundle, versioned bool, wantDigest string,
) purecef.ResourceHandler {
	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithCancel(ctx) //nolint:gosec // canceled by load completion or CEF Cancel callback
	_ = reqCtx                                // Decode is bounded CPU work, not context-aware: request
	// cancellation suppresses the native continuation via canceled.
	return cefNewResourceHandler(&systemviewWASMResourceHandler{
		cancel:     cancel,
		bundle:     bundle,
		versioned:  versioned,
		wantDigest: wantDigest,
		done:       make(chan struct{}),
	})
}

func (rh *systemviewWASMResourceHandler) load() {
	defer close(rh.done)
	defer rh.cancelRequest()
	if rh.bundle == nil {
		rh.fail()
		return
	}
	data, err := rh.bundle.WASM()
	if err != nil {
		rh.fail()
		return
	}
	// A versioned request pins exact bytes: hashing the served output
	// (reusing the decoded cache) is what earns immutable headers, not
	// the URL matching the manifest.
	if rh.versioned && sha256Hex(data) != rh.wantDigest {
		rh.fail()
		return
	}
	rh.statusCode = http.StatusOK
	// MIME matches the synchronous asset path exactly: the bundle serves
	// the same validated systemviews.wasm bytes getMimeType classifies.
	rh.contentType = getMimeType(systemviewsWASMPath)
	rh.data = data
}

// fail serves the same 404 error document as the synchronous error path,
// including its HTML content type.
func (rh *systemviewWASMResourceHandler) fail() {
	rh.statusCode = http.StatusNotFound
	rh.contentType = "text/html; charset=utf-8"
	rh.data = errorPageBody(http.StatusNotFound, "Asset not found")
}

func (rh *systemviewWASMResourceHandler) start(callback purecef.Callback) {
	rh.once.Do(func() {
		go func() {
			rh.load()
			if callback != nil && !rh.canceled.Load() {
				callback.Cont()
			}
		}()
	})
}

func (rh *systemviewWASMResourceHandler) Open(_ purecef.Request, handleRequest *int32, callback purecef.Callback) int32 {
	if handleRequest != nil {
		*handleRequest = 0
	}
	rh.start(callback)
	return 1
}

func (rh *systemviewWASMResourceHandler) ProcessRequest(_ purecef.Request, callback purecef.Callback) int32 {
	rh.start(callback)
	return 1
}

func (rh *systemviewWASMResourceHandler) GetResponseHeaders(response purecef.Response, responseLength *int64, _ uintptr) {
	<-rh.done
	response.SetStatus(int32(rh.statusCode))
	if text := http.StatusText(rh.statusCode); text != "" {
		response.SetStatusText(text)
	}
	mimeType, charset := splitMimeCharset(rh.contentType)
	response.SetMimeType(mimeType)
	if charset != "" {
		response.SetCharset(charset)
	}
	// Only version-verified bytes earn immutable headers. Versioned
	// failures are noncacheable errors; unversioned successes stay
	// explicitly noncacheable; unversioned failures keep the legacy
	// headerless error shape.
	switch {
	case rh.statusCode == http.StatusOK && rh.versioned:
		response.SetHeaderByName("Cache-Control", immutableCacheControl, 1)
	case rh.versioned || rh.statusCode == http.StatusOK:
		response.SetHeaderByName("Cache-Control", noStoreCacheControl, 1)
	}
	if responseLength != nil {
		*responseLength = int64(len(rh.data))
	}
}

func (rh *systemviewWASMResourceHandler) Skip(_ int64, _ *int64, _ purecef.ResourceSkipCallback) int32 {
	return 0
}

func (rh *systemviewWASMResourceHandler) Read(
	dataOut unsafe.Pointer, bytesToRead int32, bytesRead *int32, _ purecef.ResourceReadCallback,
) int32 {
	<-rh.done
	if rh.offset >= len(rh.data) {
		return 0
	}
	remaining := len(rh.data) - rh.offset
	toRead := min(int(bytesToRead), remaining)
	dst := unsafe.Slice((*byte)(dataOut), toRead)
	copy(dst, rh.data[rh.offset:rh.offset+toRead])
	rh.offset += toRead
	if bytesRead != nil {
		*bytesRead = int32(toRead)
	}
	return 1
}

func (rh *systemviewWASMResourceHandler) ReadResponse(_ unsafe.Pointer, _ int32, _ *int32, _ purecef.Callback) int32 {
	return 0
}

func (rh *systemviewWASMResourceHandler) Cancel() {
	rh.canceled.Store(true)
	rh.cancelRequest()
}

func (rh *systemviewWASMResourceHandler) cancelRequest() {
	if rh.cancel != nil {
		rh.cancelOnce.Do(rh.cancel)
	}
}
