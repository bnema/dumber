package filewatch

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/fsnotify/fsnotify"
)

const testDelay = 20 * time.Millisecond

func writeFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func waitFor(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for change")
		return ""
	}
}

func TestStartErrors(t *testing.T) {
	if got := New(0).delay; got != DefaultDebounce {
		t.Fatalf("New(0) delay = %v, want %v", got, DefaultDebounce)
	}
	w := New(testDelay)
	path := writeFile(t, t.TempDir(), "a")
	if err := w.Start(context.Background(), []string{path}, nil); err == nil {
		t.Fatal("expected error for nil callback")
	}
	missing := filepath.Join(t.TempDir(), "nope", "a")
	if err := w.Start(context.Background(), []string{missing}, func(string) {}); err == nil {
		t.Fatal("expected error for missing parent dir")
	}
	if w.Running() {
		t.Fatal("watcher running after failed Start")
	}
}

func TestStopIdempotentAndContextCancel(t *testing.T) {
	path := writeFile(t, t.TempDir(), "a")
	w := New(testDelay)
	ctx, cancel := context.WithCancel(context.Background())
	if err := w.Start(ctx, []string{path}, func(string) {}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if err := w.Stop(); err != nil {
				t.Errorf("Stop: %v", err)
			}
		})
	}
	wg.Wait()
	if w.Running() {
		t.Fatal("running after Stop")
	}

	if err := w.Start(ctx, []string{path}, func(string) {}); err != nil {
		t.Fatalf("Start after Stop: %v", err)
	}
	done := w.doneForTest()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("goroutine did not exit after ctx cancel")
	}
	if w.Running() {
		t.Fatal("running after ctx cancel")
	}
}

func TestStartSameSetIsNoOpAndNewSetRestarts(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a")
	b := writeFile(t, dir, "b")
	w := New(testDelay)
	t.Cleanup(func() { _ = w.Stop() })
	changes := make(chan string, 8)
	cb := func(p string) { changes <- p }

	if err := w.Start(context.Background(), []string{a, b}, cb); err != nil {
		t.Fatalf("Start: %v", err)
	}
	first := w.fsWatcherForTest()
	if err := w.Start(context.Background(), []string{b, a, a}, cb); err != nil {
		t.Fatalf("Start same set: %v", err)
	}
	if w.fsWatcherForTest() != first {
		t.Fatal("same set replaced the underlying watcher")
	}

	if err := w.Start(context.Background(), []string{b}, cb); err != nil {
		t.Fatalf("Start new set: %v", err)
	}
	if w.fsWatcherForTest() == first {
		t.Fatal("new set did not restart the watcher")
	}
	writeFile(t, dir, "b")
	if got := waitFor(t, changes); got != b {
		t.Fatalf("change = %q, want %q", got, b)
	}

	if err := w.Start(context.Background(), nil, cb); err != nil {
		t.Fatalf("Start empty: %v", err)
	}
	if w.Running() {
		t.Fatal("running after empty set")
	}
}

func TestAtomicRenameTriggers(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "theme.json")
	w := New(testDelay)
	t.Cleanup(func() { _ = w.Stop() })
	changes := make(chan string, 8)
	if err := w.Start(context.Background(), []string{path}, func(p string) { changes <- p }); err != nil {
		t.Fatalf("Start: %v", err)
	}
	tmp := writeFile(t, dir, ".theme.json.tmp")
	if err := os.Rename(tmp, path); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got := waitFor(t, changes); got != path {
		t.Fatalf("change = %q, want %q", got, path)
	}
}

func TestTwoFilesInDifferentDirs(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()
	a := writeFile(t, dirA, "a")
	b := writeFile(t, dirB, "b")
	w := New(testDelay)
	t.Cleanup(func() { _ = w.Stop() })
	changes := make(chan string, 8)
	if err := w.Start(context.Background(), []string{a, b}, func(p string) { changes <- p }); err != nil {
		t.Fatalf("Start: %v", err)
	}
	writeFile(t, dirB, "b")
	if got := waitFor(t, changes); got != b {
		t.Fatalf("change = %q, want %q", got, b)
	}
	writeFile(t, dirA, "a")
	if got := waitFor(t, changes); got != a {
		t.Fatalf("change = %q, want %q", got, a)
	}
}

// loop tests run in a synctest bubble with fake channels and fake time.

type loopHarness struct {
	events chan fsnotify.Event
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
	calls  []string
}

func startLoop(t *testing.T, paths []string) *loopHarness {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	h := &loopHarness{events: make(chan fsnotify.Event), cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(h.done)
		loop(ctx, h.events, make(chan error), paths, testDelay, func(p string) {
			h.mu.Lock()
			h.calls = append(h.calls, p)
			h.mu.Unlock()
		})
	}()
	return h
}

func (h *loopHarness) send(name string, op fsnotify.Op) {
	h.events <- fsnotify.Event{Name: name, Op: op}
}

func (h *loopHarness) snapshot() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.calls...)
}

func (h *loopHarness) close() {
	h.cancel()
	<-h.done
}

func TestLoopBurstOnOneFileCoalesces(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := startLoop(t, []string{"/d/a", "/d/b"})
		defer h.close()
		for range 5 {
			h.send("/d/a", fsnotify.Write)
			time.Sleep(testDelay / 2)
		}
		synctest.Wait()
		if got := h.snapshot(); len(got) != 0 {
			t.Fatalf("fired during burst: %v", got)
		}
		time.Sleep(testDelay)
		synctest.Wait()
		if got := h.snapshot(); len(got) != 1 || got[0] != "/d/a" {
			t.Fatalf("calls = %v, want [/d/a]", got)
		}
	})
}

func TestLoopDebouncesPerFile(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := startLoop(t, []string{"/d/a", "/d/b"})
		defer h.close()
		h.send("/d/a", fsnotify.Write)
		time.Sleep(testDelay / 2)
		h.send("/d/b", fsnotify.Create)
		// A's deadline passes; B's burst must not delay A.
		time.Sleep(testDelay/2 + time.Millisecond)
		synctest.Wait()
		if got := h.snapshot(); len(got) != 1 || got[0] != "/d/a" {
			t.Fatalf("calls = %v, want [/d/a]", got)
		}
		time.Sleep(testDelay)
		synctest.Wait()
		if got := h.snapshot(); len(got) != 2 || got[1] != "/d/b" {
			t.Fatalf("calls = %v, want [/d/a /d/b]", got)
		}
	})
}

func TestLoopIgnoresUnrelatedAndIrrelevantOps(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := startLoop(t, []string{"/d/a"})
		defer h.close()
		h.send("/d/other", fsnotify.Write)
		h.send("/d/a", fsnotify.Chmod)
		time.Sleep(2 * testDelay)
		synctest.Wait()
		if got := h.snapshot(); len(got) != 0 {
			t.Fatalf("calls = %v, want none", got)
		}
		h.send("/d/./a", fsnotify.Remove)
		time.Sleep(2 * testDelay)
		synctest.Wait()
		if got := h.snapshot(); len(got) != 1 {
			t.Fatalf("calls = %v, want one", got)
		}
	})
}

func TestLoopExitsOnClosedEvents(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := startLoop(t, []string{"/d/a"})
		close(h.events)
		<-h.done
		h.cancel()
	})
}

func TestStopWaitsForRunningCallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.toml")
	if err := os.WriteFile(path, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}

	w := New(10 * time.Millisecond)
	entered := make(chan struct{})
	release := make(chan struct{})
	var finished atomic.Bool
	var once sync.Once
	if err := w.Start(context.Background(), []string{path}, func(string) {
		once.Do(func() { close(entered) })
		<-release
		finished.Store(true)
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("callback never ran")
	}

	stopped := make(chan struct{})
	go func() {
		if err := w.Stop(); err != nil {
			t.Error(err)
		}
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("Stop returned while onChange was still running")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop never returned")
	}
	if !finished.Load() {
		t.Fatal("Stop returned before onChange finished")
	}
}

func TestRestartWaitsForRunningCallback(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.toml")
	second := filepath.Join(dir, "b.toml")
	for _, p := range []string{first, second} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	w := New(10 * time.Millisecond)
	t.Cleanup(func() { _ = w.Stop() })
	entered := make(chan struct{})
	release := make(chan struct{})
	var finished atomic.Bool
	var once sync.Once
	if err := w.Start(context.Background(), []string{first}, func(string) {
		once.Do(func() { close(entered) })
		<-release
		finished.Store(true)
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("callback never ran")
	}

	restarted := make(chan error, 1)
	go func() { restarted <- w.Start(context.Background(), []string{second}, func(string) {}) }()
	select {
	case <-restarted:
		t.Fatal("Start restarted while the old onChange was still running")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-restarted:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start never returned")
	}
	if !finished.Load() {
		t.Fatal("Start returned before the old onChange finished")
	}
	if !w.Running() {
		t.Fatal("watcher must run on the new path set")
	}
}
