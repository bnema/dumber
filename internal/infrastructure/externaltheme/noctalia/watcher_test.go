package noctalia

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bnema.dev/dumber/internal/domain/entity"
)

func TestFileWatcherStartStopAndContextCancellation(t *testing.T) {
	path := writeThemeFile(t, t.TempDir(), "theme.json")
	watcher := newFastWatcher()

	ctx, cancel := context.WithCancel(context.Background())
	if err := watcher.Start(ctx, watcherConfig(path), func() {}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !watcher.watcher.Running() {
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
	cancel()
	deadline := time.Now().Add(5 * time.Second)
	for watcher.watcher.Running() {
		if time.Now().After(deadline) {
			t.Fatal("watcher still running after context cancellation")
		}
		time.Sleep(5 * time.Millisecond)
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
	if err := watcher.Start(context.Background(), watcherConfig(path), func() { changes <- struct{}{} }); err != nil {
		t.Fatalf("second Start() same path error = %v", err)
	}
	if !watcher.watcher.Running() {
		t.Fatal("watcher not running after second Start() with same path")
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
	if watcher.watcher.Running() {
		t.Fatal("watcher running after disabled config")
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
	if watcher.watcher.Running() {
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
	if watcher.watcher.Running() {
		t.Fatal("watcher running after invalid path")
	}
}

func newFastWatcher() *FileWatcher {
	return newFileWatcher(20 * time.Millisecond)
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
