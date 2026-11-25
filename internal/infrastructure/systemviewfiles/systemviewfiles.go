// Package systemviewfiles reads and watches the user files that customize
// system view pages (user.css, home.toml).
package systemviewfiles

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/infrastructure/filewatch"
)

const (
	userCSSFile    = "user.css"
	homeConfigFile = "home.toml"

	// MaxUserCSSBytes caps how much of user.css is read.
	MaxUserCSSBytes = 1 << 20
)

// UserStyle reads <config dir>/user.css.
type UserStyle struct {
	path string
}

// NewUserStyle returns a reader for user.css inside configDir.
func NewUserStyle(configDir string) *UserStyle {
	return &UserStyle{path: filepath.Join(configDir, userCSSFile)}
}

// Path returns the user.css path.
func (s *UserStyle) Path() string { return s.path }

// Read returns at most MaxUserCSSBytes of user.css. A missing file yields "".
func (s *UserStyle) Read() (string, error) {
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("open user css: %w", err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, MaxUserCSSBytes))
	if err != nil {
		return "", fmt.Errorf("read user css: %w", err)
	}
	return string(data), nil
}

// Watcher watches user.css and home.toml in one filewatch.Watcher and
// dispatches changes by path.
type Watcher struct {
	configDir string
	watcher   *filewatch.Watcher
}

// NewWatcher creates a watcher for the system view files in configDir.
func NewWatcher(configDir string) *Watcher {
	return &Watcher{configDir: configDir, watcher: filewatch.New(filewatch.DefaultDebounce)}
}

// Start watches the files. Handlers run on the watcher goroutine.
func (w *Watcher) Start(ctx context.Context, h port.SystemViewFileHandlers) error {
	handlers := make(map[string]func(), 2)
	if h.OnUserCSS != nil {
		handlers[absPath(filepath.Join(w.configDir, userCSSFile))] = h.OnUserCSS
	}
	if h.OnHomeConfig != nil {
		handlers[absPath(filepath.Join(w.configDir, homeConfigFile))] = h.OnHomeConfig
	}
	if len(handlers) == 0 {
		return errors.New("no system view file handlers")
	}
	paths := make([]string, 0, len(handlers))
	for path := range handlers {
		paths = append(paths, path)
	}
	if err := w.watcher.Start(ctx, paths, func(path string) {
		if handler, ok := handlers[absPath(path)]; ok {
			handler()
		}
	}); err != nil {
		return fmt.Errorf("watch system view files: %w", err)
	}
	return nil
}

// Stop stops the watcher. It is safe to call repeatedly.
func (w *Watcher) Stop() error {
	return w.watcher.Stop()
}

func absPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}
