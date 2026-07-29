package cef

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/shared/syncdispatch"
	purecef "github.com/bnema/purego-cef/cef"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccessibilityCapture_WritesNumberedJSONWithSecurePerms(t *testing.T) {
	dir := t.TempDir()
	capture, err := newAccessibilityCapture(dir)
	require.NoError(t, err)

	require.NoError(t, capture.Write(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: `{"a":1}`}))
	require.NoError(t, capture.Write(accessibilityPayload{Kind: accessibilityCaptureKindLocation, JSON: `{"b":2}`}))
	require.NoError(t, capture.Write(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: `{"c":3}`}))

	want := []string{"000001-tree.json", "000002-location.json", "000003-tree.json"}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 3)
	for i, name := range want {
		assert.Equal(t, name, entries[i].Name())
		info, err := entries[i].Info()
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestAccessibilityCapture_IgnoresEmptyPayload(t *testing.T) {
	dir := t.TempDir()
	capture, err := newAccessibilityCapture(dir)
	require.NoError(t, err)
	require.NoError(t, capture.Write(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: ""}))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestAccessibilityCapture_RejectsUnsafePaths(t *testing.T) {
	for _, dir := range []string{"", ".", "foo/../bar", "../escape", "a/../../b"} {
		_, err := newAccessibilityCapture(dir)
		require.Error(t, err, "dir=%q", dir)
	}
}

func TestAccessibilityCapture_RejectsPathLikeAndUnknownKinds(t *testing.T) {
	dir := t.TempDir()
	capture, err := newAccessibilityCapture(dir)
	require.NoError(t, err)

	for _, kind := range []string{
		"../escape",
		"tree/../../etc/passwd",
		"tree/foo",
		"/tmp/x",
		"unknown",
		"TREE",
		"",
		"tree\x00",
	} {
		writeErr := capture.Write(accessibilityPayload{Kind: kind, JSON: `{"x":1}`})
		require.Error(t, writeErr, "kind=%q", kind)
	}

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestAccessibilityCapture_NeverOverwritesExistingNumberedFile(t *testing.T) {
	dir := t.TempDir()
	preexisting := filepath.Join(dir, "000001-tree.json")
	const original = `{"keep":true}`
	require.NoError(t, os.WriteFile(preexisting, []byte(original), 0o600))

	capture, err := newAccessibilityCapture(dir)
	require.NoError(t, err)
	require.Error(t, capture.Write(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: `{"overwrite":true}`}))

	got, err := os.ReadFile(preexisting)
	require.NoError(t, err)
	assert.Equal(t, original, string(got))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "000001-tree.json", entries[0].Name())
}

func TestAccessibilityCapture_Namespace0700AndChmodExisting(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "webview-9")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	info, err := os.Stat(dir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o755), info.Mode().Perm())

	capture, err := newAccessibilityCapture(dir)
	require.NoError(t, err)
	require.NotNil(t, capture)

	info, err = os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

func TestAccessibilityCapture_ConcurrentCounterNoCollision(t *testing.T) {
	dir := t.TempDir()
	capture, err := newAccessibilityCapture(dir)
	require.NoError(t, err)

	const n = 32
	var wg sync.WaitGroup
	var writeErrs atomic.Int64
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if writeErr := capture.Write(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: `{}`}); writeErr != nil {
				writeErrs.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(0), writeErrs.Load())

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, n)
	seen := make(map[string]struct{}, n)
	for _, e := range entries {
		_, dup := seen[e.Name()]
		assert.False(t, dup, "duplicate %s", e.Name())
		seen[e.Name()] = struct{}{}
	}
}

func TestAccessibilityCapture_TwoWebViewIDsIsolated(t *testing.T) {
	root := t.TempDir()
	dir1 := filepath.Join(root, "webview-1")
	dir2 := filepath.Join(root, "webview-2")
	c1, err := newAccessibilityCapture(dir1)
	require.NoError(t, err)
	c2, err := newAccessibilityCapture(dir2)
	require.NoError(t, err)

	var wg sync.WaitGroup
	var writeErrs atomic.Int64
	wg.Add(2)
	go func() {
		defer wg.Done()
		if writeErr := c1.Write(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: `{"id":1}`}); writeErr != nil {
			writeErrs.Add(1)
		}
	}()
	go func() {
		defer wg.Done()
		if writeErr := c2.Write(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: `{"id":2}`}); writeErr != nil {
			writeErrs.Add(1)
		}
	}()
	wg.Wait()
	require.Equal(t, int64(0), writeErrs.Load())

	b1, err := os.ReadFile(filepath.Join(dir1, "000001-tree.json"))
	require.NoError(t, err)
	b2, err := os.ReadFile(filepath.Join(dir2, "000001-tree.json"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":1}`, string(b1))
	assert.JSONEq(t, `{"id":2}`, string(b2))
}

func TestAccessibilityCaptureDir_UsesLogDirNamespace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ENV", "")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	dir, err := accessibilityCaptureDir(42)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".local", "state", "dumber", "logs", "a11y-capture", "webview-42"), dir)
}

func TestAccessibilityCaptureEnabled_RequiresExactOne(t *testing.T) {
	t.Setenv("DUMBER_A11Y_CAPTURE", "")
	assert.False(t, accessibilityCaptureEnabled())
	t.Setenv("DUMBER_A11Y_CAPTURE", "0")
	assert.False(t, accessibilityCaptureEnabled())
	t.Setenv("DUMBER_A11Y_CAPTURE", "true")
	assert.False(t, accessibilityCaptureEnabled())
	t.Setenv("DUMBER_A11Y_CAPTURE", "1")
	assert.True(t, accessibilityCaptureEnabled())
}

func TestAccessibilityCaptureWorker_DropsWhenFullThenCloseDrains(t *testing.T) {
	var (
		mu  sync.Mutex
		got []accessibilityPayload
	)
	w := &accessibilityCaptureWorker{
		queue: make(chan accessibilityPayload, 2),
		consume: func(p accessibilityPayload) {
			mu.Lock()
			got = append(got, p)
			mu.Unlock()
		},
	}
	require.True(t, w.Submit(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: "1"}))
	require.True(t, w.Submit(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: "2"}))
	require.False(t, w.Submit(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: "3"}))
	assert.Equal(t, uint64(1), w.Dropped())

	w.wg.Add(1)
	go w.loop()
	w.Close()

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, got, 2)
	assert.Equal(t, "1", got[0].JSON)
	assert.Equal(t, "2", got[1].JSON)
	require.False(t, w.Submit(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: "4"}))
}

func TestAccessibilityCaptureWorker_NonBlockingUnderBackpressure(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{})
	var consumed atomic.Int64
	w := newAccessibilityCaptureWorker(2, func(accessibilityPayload) {
		select {
		case <-started:
		default:
			close(started)
		}
		<-block
		consumed.Add(1)
	})

	require.True(t, w.Submit(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: "a"}))
	<-started
	require.True(t, w.Submit(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: "b"}))
	require.True(t, w.Submit(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: "c"}))

	done := make(chan struct{})
	go func() {
		defer close(done)
		assert.False(t, w.Submit(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: "d"}))
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Submit blocked under backpressure")
	}
	assert.Equal(t, uint64(1), w.Dropped())

	close(block)
	w.Close()
	assert.Equal(t, int64(3), consumed.Load())
}

func TestAccessibilityStats_ExactAverages(t *testing.T) {
	var stats accessibilityStats
	stats.Observe(accessibilityPayload{Kind: accessibilityCaptureKindTree, Bytes: 1000, SerializeNanos: 2_000_000})
	stats.Observe(accessibilityPayload{Kind: accessibilityCaptureKindTree, Bytes: 3000, SerializeNanos: 4_000_000})
	stats.Observe(accessibilityPayload{Kind: accessibilityCaptureKindLocation, Bytes: 100, SerializeNanos: 500_000})

	snap := stats.Snapshot()
	assert.Equal(t, 3, snap.Count)
	assert.Equal(t, 2, snap.TreeCount)
	assert.Equal(t, 1, snap.LocationCount)
	assert.Equal(t, int64(4100), snap.TotalBytes)
	assert.Equal(t, 100, snap.MinBytes)
	assert.Equal(t, 3000, snap.MaxBytes)
	assert.InDelta(t, 1366.6666666666667, snap.AverageBytes, 1e-9)
	assert.Equal(t, int64(6_500_000), snap.TotalSerializeNanos)
	assert.InDelta(t, 2_166_666.6666666665, snap.AverageSerializeNanos, 1e-9)
	assert.Equal(t, int64(4_000_000), snap.MaxSerializeNanos)
}

func TestAccessibilityStats_EmptySnapshotZero(t *testing.T) {
	var stats accessibilityStats
	snap := stats.Snapshot()
	assert.Equal(t, accessibilityStatsSnapshot{}, snap)
}

func TestAccessibilityCapture_InactiveWithoutEnvZeroWork(t *testing.T) {
	t.Setenv("DUMBER_A11Y_CAPTURE", "")
	var output bytes.Buffer
	logger := zerolog.New(&output).Level(zerolog.DebugLevel)
	ctx := logging.WithContext(context.Background(), logger)

	wv := &WebView{ctx: ctx, id: port.WebViewID(7), viewBridge: &Cef2gtkAdapter{}}
	require.Nil(t, wv.a11yWorker)
	require.Nil(t, wv.a11yCapture)

	rh := newDumberRenderHandler(wv)
	require.NotNil(t, rh)
	handler, ok := rh.GetAccessibilityHandler().(*accessibilityHandler)
	require.True(t, ok)
	require.Nil(t, handler.sink)

	writeCalls := 0
	handler.writeJSON = func(purecef.Value, purecef.JsonWriterOptions) string {
		writeCalls++
		return `{"x":1}`
	}
	handler.OnAccessibilityTreeChange(stubCEFValue{})
	handler.OnAccessibilityLocationChange(stubCEFValue{})
	assert.Equal(t, 0, writeCalls)
	assert.Equal(t, accessibilityStatsSnapshot{}, wv.a11yStats.Snapshot())
	assert.Empty(t, output.String())
	assert.False(t, wv.enqueueAccessibilityPayload(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: `{}`}))
}

func TestAccessibilityCapture_ConsumeLogsAndWritesOffCallback(t *testing.T) {
	dir := t.TempDir()
	capture, err := newAccessibilityCapture(dir)
	require.NoError(t, err)

	var output bytes.Buffer
	logger := zerolog.New(&output).Level(zerolog.InfoLevel)
	ctx := logging.WithContext(context.Background(), logger)

	wv := &WebView{
		ctx:         ctx,
		id:          port.WebViewID(3),
		a11yCapture: capture,
	}
	wv.a11yWorker = newAccessibilityCaptureWorker(8, wv.consumeAccessibilityPayload)

	require.True(t, wv.enqueueAccessibilityPayload(accessibilityPayload{
		Kind: accessibilityCaptureKindTree, JSON: `{"ok":true}`, Bytes: 11, SerializeNanos: 123,
	}))
	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(dir, "000001-tree.json"))
		return err == nil
	}, 2*time.Second, 10*time.Millisecond)
	wv.a11yWorker.Close()

	snap := wv.a11yStats.Snapshot()
	assert.Equal(t, 1, snap.Count)
	assert.Equal(t, 1, snap.TreeCount)
	assert.Equal(t, 11, snap.MaxBytes)

	var found bool
	dec := json.NewDecoder(bytes.NewReader(output.Bytes()))
	for dec.More() {
		var rec map[string]any
		require.NoError(t, dec.Decode(&rec))
		if rec["message"] == "cef: accessibility update" {
			found = true
			assert.InDelta(t, float64(3), rec["webview_id"], 0)
			assert.Equal(t, accessibilityCaptureKindTree, rec["kind"])
			assert.InDelta(t, float64(11), rec["bytes"], 0)
			assert.InDelta(t, float64(123), rec["serialize_ns"], 0)
		}
	}
	assert.True(t, found)
}

func TestAccessibilityCapture_DestroyEmitsSummary(t *testing.T) {
	dir := t.TempDir()
	capture, err := newAccessibilityCapture(dir)
	require.NoError(t, err)

	var output bytes.Buffer
	logger := zerolog.New(&output).Level(zerolog.InfoLevel)
	ctx := logging.WithContext(context.Background(), logger)

	wv := &WebView{
		ctx:         ctx,
		id:          port.WebViewID(5),
		a11yCapture: capture,
	}
	wv.a11yWorker = newAccessibilityCaptureWorker(4, wv.consumeAccessibilityPayload)
	require.True(t, wv.enqueueAccessibilityPayload(accessibilityPayload{
		Kind: accessibilityCaptureKindLocation, JSON: `{}`, Bytes: 2, SerializeNanos: 50,
	}))
	require.Eventually(t, func() bool {
		return wv.a11yStats.Snapshot().Count == 1
	}, 2*time.Second, 10*time.Millisecond)

	worker := wv.a11yWorker
	wv.shutdownAccessibilityCapture()

	require.True(t, wv.a11yCaptureFinalized.Load())
	require.Same(t, worker, wv.a11yWorker)
	require.NotNil(t, wv.a11yCapture)

	var found bool
	dec := json.NewDecoder(bytes.NewReader(output.Bytes()))
	for dec.More() {
		var rec map[string]any
		require.NoError(t, dec.Decode(&rec))
		if rec["message"] == "cef: accessibility summary" {
			found = true
			assert.InDelta(t, float64(5), rec["webview_id"], 0)
			assert.InDelta(t, float64(1), rec["count"], 0)
			assert.InDelta(t, float64(1), rec["location_count"], 0)
			assert.InDelta(t, float64(0), rec["dropped"], 0)
		}
	}
	assert.True(t, found)
	require.False(t, wv.enqueueAccessibilityPayload(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: `{}`}))
}

func TestInstallNewWebViewViewportHooksAfterCapture_FailureClosesWorkerOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("DUMBER_A11Y_CAPTURE", "1")

	prev := newWebViewInstallViewportSyncHooks
	t.Cleanup(func() { newWebViewInstallViewportSyncHooks = prev })
	newWebViewInstallViewportSyncHooks = func(*WebView) syncdispatch.SyncDispatchResult {
		return syncdispatch.SyncDispatchResult{
			Label:  "cef.install_viewport_sync_hooks",
			Status: syncdispatch.SyncDispatchTimedOut,
		}
	}

	var output bytes.Buffer
	logger := zerolog.New(&output).Level(zerolog.InfoLevel)
	ctx := logging.WithContext(context.Background(), logger)

	wv := &WebView{
		ctx: ctx,
		id:  port.WebViewID(99),
		gtkSyncDispatch: func(fn func()) {
			if fn != nil {
				fn()
			}
		},
		gtkSyncIsOwner: func() bool { return true },
		viewBridge:     &Cef2gtkAdapter{},
	}
	require.NoError(t, wv.enableAccessibilityCaptureIfRequested())
	worker := wv.a11yWorker
	require.NotNil(t, worker)

	err := wv.installNewWebViewViewportHooksAfterCapture()
	require.Error(t, err)
	require.Contains(t, err.Error(), "install CEF viewport sync hooks")
	require.True(t, wv.a11yCaptureFinalized.Load())
	require.Same(t, worker, wv.a11yWorker, "published worker pointer must stay immutable")
	require.NotNil(t, wv.a11yCapture, "published capture pointer must stay immutable")
	require.True(t, worker.closed)
	require.False(t, worker.Submit(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: `{}`}))
	require.False(t, wv.enqueueAccessibilityPayload(accessibilityPayload{Kind: accessibilityCaptureKindTree, JSON: `{}`}))

	wv.shutdownAccessibilityCapture()
	wv.Destroy()

	summaryCount := 0
	dec := json.NewDecoder(bytes.NewReader(output.Bytes()))
	for dec.More() {
		var rec map[string]any
		require.NoError(t, dec.Decode(&rec))
		if rec["message"] == "cef: accessibility summary" {
			summaryCount++
		}
	}
	assert.Equal(t, 0, summaryCount, "failed init must not emit Destroy summary")
}

func TestShutdownAccessibilityCapture_IdempotentAfterAbort(t *testing.T) {
	dir := t.TempDir()
	capture, err := newAccessibilityCapture(dir)
	require.NoError(t, err)

	var output bytes.Buffer
	logger := zerolog.New(&output).Level(zerolog.InfoLevel)
	ctx := logging.WithContext(context.Background(), logger)

	wv := &WebView{ctx: ctx, id: port.WebViewID(7), a11yCapture: capture}
	wv.a11yWorker = newAccessibilityCaptureWorker(2, wv.consumeAccessibilityPayload)

	wv.abortAccessibilityCaptureSetup()
	require.True(t, wv.a11yCaptureFinalized.Load())
	require.NotNil(t, wv.a11yWorker)
	require.True(t, wv.a11yWorker.closed)
	require.NotNil(t, wv.a11yCapture)
	wv.shutdownAccessibilityCapture()
	wv.shutdownAccessibilityCapture()

	summaryCount := 0
	dec := json.NewDecoder(bytes.NewReader(output.Bytes()))
	for dec.More() {
		var rec map[string]any
		require.NoError(t, dec.Decode(&rec))
		if rec["message"] == "cef: accessibility summary" {
			summaryCount++
		}
	}
	assert.Equal(t, 0, summaryCount)
}

func TestEnqueueAccessibilityPayload_ConcurrentWithShutdownRaceFree(t *testing.T) {
	dir := t.TempDir()
	capture, err := newAccessibilityCapture(dir)
	require.NoError(t, err)

	var output bytes.Buffer
	logger := zerolog.New(&output).Level(zerolog.InfoLevel)
	ctx := logging.WithContext(context.Background(), logger)

	wv := &WebView{ctx: ctx, id: port.WebViewID(11), a11yCapture: capture}
	wv.a11yWorker = newAccessibilityCaptureWorker(64, wv.consumeAccessibilityPayload)
	worker := wv.a11yWorker

	const producers = 8
	const perProducer = 200
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range producers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for range perProducer {
				_ = wv.enqueueAccessibilityPayload(accessibilityPayload{
					Kind: accessibilityCaptureKindTree, JSON: `{}`, Bytes: 2, SerializeNanos: 1,
				})
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		wv.shutdownAccessibilityCapture()
	}()
	close(start)
	wg.Wait()

	require.True(t, wv.a11yCaptureFinalized.Load())
	require.Same(t, worker, wv.a11yWorker)
	require.NotNil(t, wv.a11yCapture)
	require.True(t, worker.closed)
	assert.False(t, wv.enqueueAccessibilityPayload(accessibilityPayload{
		Kind: accessibilityCaptureKindTree, JSON: `{}`,
	}))

	wv.shutdownAccessibilityCapture()
	wv.abortAccessibilityCaptureSetup()

	summaryCount := 0
	dec := json.NewDecoder(bytes.NewReader(output.Bytes()))
	for dec.More() {
		var rec map[string]any
		require.NoError(t, dec.Decode(&rec))
		if rec["message"] == "cef: accessibility summary" {
			summaryCount++
		}
	}
	assert.Equal(t, 1, summaryCount, "successful shutdown must emit exactly one summary")
}
