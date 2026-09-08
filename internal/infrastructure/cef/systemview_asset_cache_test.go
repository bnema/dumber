package cef

// P2.1 tests: the bundle loads its WASM exactly once (success or failure),
// with the hardened semantics — compressed authoritative, raw fallback only
// on absence, raw reads bounded, failures cached.

import (
	"errors"
	"io"
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

// compressedReads counts compressed WASM reads through either entry
// point: the loader may Open the file directly (bounded reads) or use
// the ReadFileFS shortcut, depending on the hardened path.
func (c *countingFS) compressedReads() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	name := systemviewsWASMPath + ".br"
	return c.opens["readfile:"+name] + c.opens["open:"+name]
}

// denyFS fails every Open with a fixed error (e.g. permission denied).
type denyFS struct{ err error }

func (d denyFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: d.err}
}

func TestSystemviewAssetCache_DecodesOnceUnderConcurrency(t *testing.T) {
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
	require.Equal(t, 1, fsys.compressedReads(), "bundle must decode exactly once")
}

func TestSystemviewAssetCache_CachesFailureWithoutRetryStorm(t *testing.T) {
	t.Parallel()

	fsys := &countingFS{inner: fstest.MapFS{
		"systemviews/systemviews.wasm.br": {Data: []byte("not-brotli")},
		"systemviews/systemviews.wasm":    {Data: []byte("\x00asm-valid-raw")},
	}}
	b := newSystemviewAssetBundle(fsys)

	_, err := b.WASM()
	require.Error(t, err, "corrupt compressed WASM must fail even with valid raw present")
	firstReads := fsys.compressedReads()
	_, err = b.WASM()
	require.Error(t, err)
	require.Equal(t, firstReads, fsys.compressedReads(), "cached failure must not re-read")
}

func TestSystemviewAssetCache_CompressedReadErrorIsExplicit(t *testing.T) {
	t.Parallel()

	// Permission (or any non-absence) failures reading the compressed file
	// must not silently fall back to raw.
	b := newSystemviewAssetBundle(denyFS{err: fs.ErrPermission})

	_, err := b.WASM()
	require.Error(t, err)
	require.ErrorIs(t, err, fs.ErrPermission, "must surface the read failure")
	require.NotErrorIs(t, err, fs.ErrNotExist)
}

func TestSystemviewAssetCache_RawFallbackOnlyOnAbsence(t *testing.T) {
	t.Parallel()

	raw := []byte("\x00asm-raw-fallback")
	b := newSystemviewAssetBundle(fstest.MapFS{
		"systemviews/systemviews.wasm": {Data: raw},
	})

	data, err := b.WASM()
	require.NoError(t, err)
	require.Equal(t, raw, data)
}

func TestSystemviewAssetCache_RawOversizedRejected(t *testing.T) {
	t.Parallel()

	big := make([]byte, maxSystemviewsWASMBytes+1)
	b := newSystemviewAssetBundle(fstest.MapFS{
		"systemviews/systemviews.wasm": {Data: big},
	})

	_, err := b.WASM()
	require.Error(t, err, "raw WASM past the bound must fail like the compressed path")
}

func TestSystemviewAssetCache_MissingBothReportsNotExist(t *testing.T) {
	t.Parallel()

	b := newSystemviewAssetBundle(fstest.MapFS{})

	_, err := b.WASM()
	require.Error(t, err)
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestSystemviewAssetCache_NilFSFailsCleanly(t *testing.T) {
	t.Parallel()

	b := newSystemviewAssetBundle(nil)
	_, err := b.WASM()
	require.Error(t, err)
}

// zeroFile yields size bytes of zeros without allocating them, proving
// input bounds without a 64 MiB fixture in memory.
type zeroFile struct {
	remaining int64
}

func (f *zeroFile) Read(p []byte) (int, error) {
	if f.remaining <= 0 {
		return 0, io.EOF
	}
	n := min(int64(len(p)), f.remaining)
	clear(p[:n])
	f.remaining -= n
	return int(n), nil
}

func (f *zeroFile) Close() error { return nil }

func (f *zeroFile) Stat() (fs.FileInfo, error) { return nil, errors.New("zeroFile has no stat") }

// oversizedInputFS serves an unbounded-looking compressed file: the
// bundle must reject it on size before allocation.
type oversizedInputFS struct {
	fstest.MapFS
}

func (o oversizedInputFS) Open(name string) (fs.File, error) {
	if name == systemviewsWASMPath+".br" {
		return &zeroFile{remaining: int64(maxSystemviewsWASMBytes) + 1}, nil
	}
	return o.MapFS.Open(name)
}

func TestSystemviewAssetCache_OversizedCompressedInputRejected(t *testing.T) {
	t.Parallel()

	b := newSystemviewAssetBundle(oversizedInputFS{fstest.MapFS{
		"systemviews/systemviews.wasm": {Data: []byte("\x00asm-valid-raw")},
	}})
	_, err := b.WASM()
	require.Error(t, err, "oversized compressed input must fail before allocation")
	require.Contains(t, err.Error(), "exceeds")
}
