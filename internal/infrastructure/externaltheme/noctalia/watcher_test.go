package noctalia

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bnema.dev/dumber/internal/domain/entity"
)

func TestFileWatcherDisabledConfigStops(t *testing.T) {
	path := writeThemeFile(t, t.TempDir(), "theme.json")
	watcher := newFastWatcher()
	if err := watcher.Start(context.Background(), watcherConfig(path), func() {}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	cfg := watcherConfig(path)
	cfg.Enabled = false
	if err := watcher.Start(context.Background(), cfg, func() {}); err != nil {
		t.Fatalf("Start(disabled) error = %v", err)
	}
	if watcher.watcher.Running() {
		t.Fatal("watcher running after disabled config")
	}
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
