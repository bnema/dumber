package noctalia

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/fsnotify/fsnotify"
)

func TestFileWatcherStartStopAndContextCancellation(t *testing.T) {
	path := writeThemeFile(t, t.TempDir(), "theme.json")
	watcher := newFastWatcher()

	ctx, cancel := context.WithCancel(context.Background())
	if err := watcher.Start(ctx, watcherConfig(path), func() {}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !watcher.isRunningForTest() {
		t.Fatal("watcher is not running after Start")
	}
	if err := watcher.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := watcher.Stop(); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}

	if err := watcher.Start(ctx, watcherConfig(path), func() {}); err != nil {
		t.Fatalf("Start() after Stop error = %v", err)
	}
	done := watcher.doneForTest()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher goroutine did not exit after context cancellation")
	}
	if watcher.isRunningForTest() {
		t.Fatal("watcher still running after context cancellation")
	}
}

func TestFileWatcherStartSamePathDoesNotDuplicateWatcher(t *testing.T) {
	dir := t.TempDir()
	path := writeThemeFile(t, dir, "theme.json")
	watcher := newFastWatcher()
	changes := make(chan struct{}, 4)

	if err := watcher.Start(context.Background(), watcherConfig(path), func() { changes <- struct{}{} }); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	first := watcher.fsWatcherForTest()
	if err := watcher.Start(context.Background(), watcherConfig(path), func() { changes <- struct{}{} }); err != nil {
		t.Fatalf("second Start() same path error = %v", err)
	}
	if got := watcher.fsWatcherForTest(); got != first {
		t.Fatal("second Start() with same path replaced the underlying watcher")
	}

	writeThemeFile(t, dir, "theme.json")
	waitForChange(t, changes)
}

func TestFileWatcherPathChangeRestartsAndDisabledStops(t *testing.T) {
	dir := t.TempDir()
	first := writeThemeFile(t, dir, "first.json")
	second := writeThemeFile(t, dir, "second.json")
	watcher := newFastWatcher()
	changes := make(chan string, 8)

	if err := watcher.Start(context.Background(), watcherConfig(first), func() { changes <- "first" }); err != nil {
		t.Fatalf("Start(first) error = %v", err)
	}
	if err := watcher.Start(context.Background(), watcherConfig(second), func() { changes <- "second" }); err != nil {
		t.Fatalf("Start(second) error = %v", err)
	}

	writeThemeFile(t, dir, "second.json")
	if got := waitForStringChange(t, changes); got != "second" {
		t.Fatalf("callback = %q, want second", got)
	}

	cfg := watcherConfig(second)
	cfg.Enabled = false
	if err := watcher.Start(context.Background(), cfg, func() { changes <- "disabled" }); err != nil {
		t.Fatalf("Start(disabled) error = %v", err)
	}
	if watcher.isRunningForTest() {
		t.Fatal("watcher running after disabled config")
	}
}

const loopTestPath = "/themes/theme.json"

// loopHarness drives loop with fake channels inside a synctest bubble.
type loopHarness struct {
	events chan fsnotify.Event
	errs   chan error
	cancel context.CancelFunc
	done   chan struct{}
	calls  atomic.Int32
}

func startLoop(t *testing.T, delay time.Duration) *loopHarness {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	h := &loopHarness{
		events: make(chan fsnotify.Event),
		errs:   make(chan error),
		cancel: cancel,
		done:   make(chan struct{}),
	}
	go func() {
		defer close(h.done)
		loop(ctx, h.events, h.errs, loopTestPath, delay, func() { h.calls.Add(1) })
	}()
	return h
}

func (h *loopHarness) assertCalls(t *testing.T, want int32) {
	t.Helper()
	synctest.Wait()
	if got := h.calls.Load(); got != want {
		t.Fatalf("callbacks = %d, want %d", got, want)
	}
}

func TestLoopDebouncesBurstToOneCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const delay = 100 * time.Millisecond
		h := startLoop(t, delay)

		for range 5 {
			h.events <- fsnotify.Event{Name: loopTestPath, Op: fsnotify.Write}
			time.Sleep(delay / 2)
		}
		h.assertCalls(t, 0)

		time.Sleep(delay)
		h.assertCalls(t, 1)

		time.Sleep(10 * delay)
		h.assertCalls(t, 1)

		h.events <- fsnotify.Event{Name: loopTestPath, Op: fsnotify.Create}
		time.Sleep(delay)
		h.assertCalls(t, 2)

		h.cancel()
		<-h.done
	})
}

func TestLoopIgnoresIrrelevantEventsAndErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const delay = 100 * time.Millisecond
		h := startLoop(t, delay)

		h.events <- fsnotify.Event{Name: "/themes/other.json", Op: fsnotify.Write}
		h.events <- fsnotify.Event{Name: loopTestPath, Op: fsnotify.Chmod}
		h.errs <- errors.New("boom")
		time.Sleep(10 * delay)
		h.assertCalls(t, 0)

		h.cancel()
		<-h.done
	})
}

func TestLoopCancelDropsPendingCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const delay = 100 * time.Millisecond
		h := startLoop(t, delay)

		h.events <- fsnotify.Event{Name: loopTestPath, Op: fsnotify.Write}
		h.cancel()
		<-h.done
		time.Sleep(10 * delay)
		h.assertCalls(t, 0)
	})
}

func TestLoopReturnsWhenChannelsClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := startLoop(t, time.Millisecond)
		close(h.events)
		<-h.done
		h.cancel()

		h = startLoop(t, time.Millisecond)
		close(h.errs)
		<-h.done
		h.cancel()
	})
}

func TestIsRelevantFileEventFiltersPathAndOps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.json")
	other := filepath.Join(filepath.Dir(path), "other.json")
	tests := []struct {
		name  string
		event fsnotify.Event
		want  bool
	}{
		{"write", fsnotify.Event{Name: path, Op: fsnotify.Write}, true},
		{"create", fsnotify.Event{Name: path, Op: fsnotify.Create}, true},
		{"rename", fsnotify.Event{Name: path, Op: fsnotify.Rename}, true},
		{"remove", fsnotify.Event{Name: path, Op: fsnotify.Remove}, true},
		{"chmod", fsnotify.Event{Name: path, Op: fsnotify.Chmod}, false},
		{"other file", fsnotify.Event{Name: other, Op: fsnotify.Write}, false},
		{"unclean path", fsnotify.Event{Name: filepath.Dir(path) + "/./theme.json", Op: fsnotify.Write}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRelevantFileEvent(tt.event, path); got != tt.want {
				t.Fatalf("isRelevantFileEvent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFileWatcherAtomicRenameWriteEmitsCallback(t *testing.T) {
	dir := t.TempDir()
	path := writeThemeFile(t, dir, "theme.json")
	watcher := newFastWatcher()
	changes := make(chan struct{}, 4)

	if err := watcher.Start(context.Background(), watcherConfig(path), func() { changes <- struct{}{} }); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	tmp := filepath.Join(dir, "theme.json.tmp")
	if err := os.WriteFile(tmp, []byte(`{"light":{},"dark":{}}`), 0o600); err != nil {
		t.Fatalf("WriteFile(tmp) error = %v", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}

	waitForChange(t, changes)
}

func TestFileWatcherInvalidOrEmptyPathBehavior(t *testing.T) {
	watcher := newFastWatcher()
	if err := watcher.Start(context.Background(), watcherConfig(""), nil); err != nil {
		t.Fatalf("Start(empty path) error = %v", err)
	}
	if watcher.isRunningForTest() {
		t.Fatal("watcher running for empty path")
	}
	missingParent := filepath.Join(t.TempDir(), "missing", "theme.json")
	if err := watcher.Start(context.Background(), watcherConfig(missingParent), func() {}); err == nil {
		t.Fatal("Start(missing parent) error = nil, want error")
	}
}

func TestWatchPathFromConfigSupportsNoctaliaFormats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.json")
	for _, format := range []string{"colors-json", "dumber-json"} {
		t.Run(format, func(t *testing.T) {
			got, enabled, err := watchPathFromConfig(entity.ExternalThemeConfig{
				Enabled:  true,
				Provider: " NOCTALIA ",
				Format:   " " + format + " ",
				Path:     path,
			})
			if err != nil {
				t.Fatalf("watchPathFromConfig() error = %v", err)
			}
			if !enabled {
				t.Fatal("enabled = false, want true")
			}
			if got != path {
				t.Fatalf("path = %q, want %q", got, path)
			}
		})
	}

	_, enabled, err := watchPathFromConfig(entity.ExternalThemeConfig{
		Enabled:  true,
		Provider: "noctalia",
		Format:   "toml",
		Path:     path,
	})
	if err != nil {
		t.Fatalf("watchPathFromConfig(unsupported) error = %v", err)
	}
	if enabled {
		t.Fatal("enabled = true for unsupported format, want false")
	}
}

func TestFileWatcherInvalidPathStopsPreviousWatcher(t *testing.T) {
	dir := t.TempDir()
	path := writeThemeFile(t, dir, "theme.json")
	watcher := newFastWatcher()
	changes := make(chan struct{}, 4)

	if err := watcher.Start(context.Background(), watcherConfig(path), func() { changes <- struct{}{} }); err != nil {
		t.Fatalf("Start(valid) error = %v", err)
	}
	missingParent := filepath.Join(dir, "missing", "theme.json")
	if err := watcher.Start(context.Background(), watcherConfig(missingParent), func() { changes <- struct{}{} }); err == nil {
		t.Fatal("Start(missing parent) error = nil, want error")
	}
	if watcher.isRunningForTest() {
		t.Fatal("watcher running after invalid path")
	}
}

func newFastWatcher() *FileWatcher {
	watcher := NewFileWatcher()
	watcher.delay = 20 * time.Millisecond
	return watcher
}

func watcherConfig(path string) entity.ExternalThemeConfig {
	return entity.ExternalThemeConfig{Enabled: true, Provider: providerName, Format: formatName, Path: path}
}

func writeThemeFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(`{"light":{},"dark":{}}`), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", name, err)
	}
	return path
}

func waitForChange(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for callback")
	}
}

func waitForStringChange(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for callback")
	}
	panic("unreachable")
}
