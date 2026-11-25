package noctalia

import (
	"context"
	"errors"
	"fmt"
	"time"

	"bnema.dev/dumber/internal/domain/entity"
	"bnema.dev/dumber/internal/infrastructure/filewatch"
)

// FileWatcher watches one Noctalia external theme file.
type FileWatcher struct {
	watcher *filewatch.Watcher
}

// NewFileWatcher creates a file watcher with the default debounce delay.
func NewFileWatcher() *FileWatcher {
	return newFileWatcher(filewatch.DefaultDebounce)
}

func newFileWatcher(delay time.Duration) *FileWatcher {
	return &FileWatcher{watcher: filewatch.New(delay)}
}

// Start starts watching the configured external theme path.
//
// Calling Start with the same enabled path while already running is a no-op.
// Calling Start with a different enabled path restarts the underlying watcher.
// Disabled, unsupported, or empty-path config stops any existing watcher.
func (w *FileWatcher) Start(ctx context.Context, cfg entity.ExternalThemeConfig, onChange func()) error {
	path, enabled, err := watchPathFromConfig(cfg)
	if err != nil {
		if stopErr := w.Stop(); stopErr != nil {
			return errors.Join(err, fmt.Errorf("stop noctalia theme watcher: %w", stopErr))
		}
		return err
	}
	if !enabled || path == "" {
		return w.Stop()
	}
	if onChange == nil {
		return errors.New("noctalia file watcher callback is nil")
	}
	if err := w.watcher.Start(ctx, []string{path}, func(string) { onChange() }); err != nil {
		return fmt.Errorf("watch noctalia theme: %w", err)
	}
	return nil
}

// Stop stops the current watcher. It is safe to call repeatedly.
func (w *FileWatcher) Stop() error {
	return w.watcher.Stop()
}

func watchPathFromConfig(cfg entity.ExternalThemeConfig) (string, bool, error) {
	provider, format := normalizeProviderAndFormat(cfg.Provider, cfg.Format)
	enabled := cfg.Enabled && provider == providerName && isSupportedFormat(format)
	if !enabled {
		return "", false, nil
	}
	path, err := ExpandPath(cfg.Path)
	if err != nil {
		return "", false, err
	}
	return path, path != "", nil
}
