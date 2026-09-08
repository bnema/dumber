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
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sync"

	"github.com/andybalholm/brotli"
)

// systemviewsWASMPath is the fixed bundle-relative WASM served to internal
// pages. Only this file is cached; nothing else enters this cache.
const systemviewsWASMPath = "systemviews/systemviews.wasm"

// systemviewAssetBundle is one immutable asset state: an asset filesystem
// plus its once-loaded WASM outcome. In-flight requests retain their
// captured bundle; installing a different bundle replaces the whole state.
type systemviewAssetBundle struct {
	assets fs.FS
	mu     sync.Mutex
	loaded bool
	data   []byte
	err    error
}

// newSystemviewAssetBundle captures one immutable asset filesystem. A nil
// FS is rejected at load time with a plain error, never a panic.
func newSystemviewAssetBundle(assets fs.FS) *systemviewAssetBundle {
	return &systemviewAssetBundle{assets: assets}
}

// WASM returns the bundle's decoded WASM, loading it exactly once. Success
// and failure are both cached: repeated callers share the first outcome
// without re-reading or re-decoding.
func (b *systemviewAssetBundle) WASM() ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.loaded {
		b.data, b.err = loadSystemviewWASM(b.assets)
		b.loaded = true
	}
	return b.data, b.err
}

// loadSystemviewWASM reads and decodes the bundle WASM with the hardened
// semantics documented above. It performs no caching itself.
func loadSystemviewWASM(assets fs.FS) ([]byte, error) {
	if assets == nil {
		return nil, errors.New("systemview assets not configured")
	}
	compressed, err := fs.ReadFile(assets, systemviewsWASMPath+".br")
	if err == nil {
		data, err := io.ReadAll(io.LimitReader(brotli.NewReader(bytes.NewReader(compressed)), maxSystemviewsWASMBytes+1))
		if err != nil {
			return nil, fmt.Errorf("decompress systemview WASM: %w", err)
		}
		if len(data) > maxSystemviewsWASMBytes {
			return nil, fmt.Errorf("decompressed systemview WASM exceeds %d bytes", maxSystemviewsWASMBytes)
		}
		return data, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read compressed systemview WASM: %w", err)
	}
	return readBoundedAsset(assets, systemviewsWASMPath)
}

// readBoundedAsset reads one raw asset with the same size bound as the
// decompressed path, enforced before unbounded allocation.
func readBoundedAsset(assets fs.FS, name string) ([]byte, error) {
	f, err := assets.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxSystemviewsWASMBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read systemview asset %s: %w", name, err)
	}
	if len(data) > maxSystemviewsWASMBytes {
		return nil, fmt.Errorf("systemview asset %s exceeds %d bytes", name, maxSystemviewsWASMBytes)
	}
	return data, nil
}
