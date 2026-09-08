package cef

// P2.1 tests: the bundle loads its WASM exactly once (success or failure),
// with the hardened semantics — compressed authoritative, raw fallback only
// on absence, raw reads bounded, failures cached.

import (
	"errors"
	"io/fs"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

// countingFS counts Open and ReadFile calls per name. Note MapFS
// implements ReadFileFS, so fs.ReadFile bypasses Open: both entry points
// must be counted explicitly.
type countingFS struct {
	inner fstest.MapFS
	mu    sync.Mutex
	opens map[string]int
}

func (c *countingFS) record(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.opens == nil {
		c.opens = map[string]int{}
	}
	c.opens[name]++
}

func (c *countingFS) Open(name string) (fs.File, error) {
	c.record("open:" + name)
	return c.inner.Open(name)
}

func (c *countingFS) ReadFile(name string) ([]byte, error) {
	c.record("readfile:" + name)
	return c.inner.ReadFile(name)
}

func (c *countingFS) count(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.opens[name]
}

// denyFS fails every Open with a fixed error (e.g. permission denied).
type denyFS struct{ err error }

func (d denyFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: d.err}
}

func TestSystemviewBundle_DecodesOnceUnderConcurrency(t *testing.T) {
	t.Parallel()

	wasm := []byte("\x00asm-bundle-once")
	fsys := &countingFS{inner: fstest.MapFS{
		"systemviews/systemviews.wasm.br": {Data: brotliCompressForTest(t, wasm)},
	}}
	b := newSystemviewAssetBundle(fsys)

	const readers = 8
	got := make([][]byte, readers)
	errs := make([]error, readers)
	var wg sync.WaitGroup
	for i := range got {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got[i], errs[i] = b.WASM()
		}(i)
	}
	wg.Wait()
	for i := range got {
		require.NoError(t, errs[i])
		require.Equal(t, wasm, got[i])
		require.Same(t, &got[0][0], &got[i][0], "waiters must share one immutable buffer")
	}
	require.Equal(t, 1, fsys.count("readfile:systemviews/systemviews.wasm.br"), "bundle must decode exactly once")
}

func TestSystemviewBundle_CachesFailureWithoutRetryStorm(t *testing.T) {
	t.Parallel()

	fsys := &countingFS{inner: fstest.MapFS{
		"systemviews/systemviews.wasm.br": {Data: []byte("not-brotli")},
		"systemviews/systemviews.wasm":    {Data: []byte("\x00asm-valid-raw")},
	}}
	b := newSystemviewAssetBundle(fsys)

	_, err := b.WASM()
	require.Error(t, err, "corrupt compressed WASM must fail even with valid raw present")
	firstReads := fsys.count("readfile:systemviews/systemviews.wasm.br")
	_, err = b.WASM()
	require.Error(t, err)
	require.Equal(t, firstReads, fsys.count("readfile:systemviews/systemviews.wasm.br"), "cached failure must not re-read")
}

func TestSystemviewBundle_CompressedReadErrorIsExplicit(t *testing.T) {
	t.Parallel()

	// Permission (or any non-absence) failures reading the compressed file
	// must not silently fall back to raw.
	b := newSystemviewAssetBundle(denyFS{err: fs.ErrPermission})

	_, err := b.WASM()
	require.Error(t, err)
	require.True(t, errors.Is(err, fs.ErrPermission), "must surface the read failure, got %v", err)
	require.False(t, errors.Is(err, fs.ErrNotExist))
}

func TestSystemviewBundle_RawFallbackOnlyOnAbsence(t *testing.T) {
	t.Parallel()

	raw := []byte("\x00asm-raw-fallback")
	b := newSystemviewAssetBundle(fstest.MapFS{
		"systemviews/systemviews.wasm": {Data: raw},
	})

	data, err := b.WASM()
	require.NoError(t, err)
	require.Equal(t, raw, data)
}

func TestSystemviewBundle_RawOversizedRejected(t *testing.T) {
	t.Parallel()

	big := make([]byte, maxSystemviewsWASMBytes+1)
	b := newSystemviewAssetBundle(fstest.MapFS{
		"systemviews/systemviews.wasm": {Data: big},
	})

	_, err := b.WASM()
	require.Error(t, err, "raw WASM past the bound must fail like the compressed path")
}

func TestSystemviewBundle_MissingBothReportsNotExist(t *testing.T) {
	t.Parallel()

	b := newSystemviewAssetBundle(fstest.MapFS{})

	_, err := b.WASM()
	require.Error(t, err)
	require.True(t, errors.Is(err, fs.ErrNotExist), "got %v", err)
}

func TestSystemviewBundle_NilFSFailsCleanly(t *testing.T) {
	t.Parallel()

	b := newSystemviewAssetBundle(nil)
	_, err := b.WASM()
	require.Error(t, err)
}
