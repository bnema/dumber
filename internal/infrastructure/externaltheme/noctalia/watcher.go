package noctalia

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"bnema.dev/dumber/internal/domain/entity"
	"github.com/fsnotify/fsnotify"
)

const defaultDebounceDelay = 75 * time.Millisecond

// FileWatcher watches one Noctalia external theme file.
//
// It watches the file's parent directory so atomic writes and renames are
// observed, then filters events to the configured file path.
type FileWatcher struct {
	mu       sync.Mutex
	delay    time.Duration
	watcher  *fsnotify.Watcher
	path     string
	stopCtx  context.Context
	stopFunc context.CancelFunc
	running  bool
	// done is closed when the current run goroutine exits.
	done chan struct{}
}

// NewFileWatcher creates a file watcher with the default debounce delay.
func NewFileWatcher() *FileWatcher {
	return &FileWatcher{delay: defaultDebounceDelay}
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

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running && w.path == path {
		return nil
	}
	if stopErr := w.stopLocked(); stopErr != nil {
		return fmt.Errorf("stop previous noctalia theme watcher: %w", stopErr)
	}

	parent := filepath.Dir(path)
	info, statErr := os.Stat(parent)
	if statErr != nil {
		return fmt.Errorf("watch noctalia theme parent directory: %w", statErr)
	} else if !info.IsDir() {
		return fmt.Errorf("watch noctalia theme parent directory: %s is not a directory", parent)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create noctalia theme watcher: %w", err)
	}
	if err := watcher.Add(parent); err != nil {
		if closeErr := watcher.Close(); closeErr != nil {
			return errors.Join(
				fmt.Errorf("watch noctalia theme parent directory: %w", err),
				fmt.Errorf("close noctalia theme watcher after add failure: %w", closeErr),
			)
		}
		return fmt.Errorf("watch noctalia theme parent directory: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	w.watcher = watcher
	w.path = path
	w.stopCtx = runCtx
	w.stopFunc = cancel
	w.running = true
	w.done = make(chan struct{})

	go w.run(runCtx, watcher, path, onChange, w.done)
	return nil
}

// Stop stops the current watcher. It is safe to call repeatedly.
func (w *FileWatcher) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stopLocked()
}

func (w *FileWatcher) stopLocked() error {
	if !w.running {
		return nil
	}
	if w.stopFunc != nil {
		w.stopFunc()
	}
	err := w.watcher.Close()
	w.watcher = nil
	w.path = ""
	w.stopCtx = nil
	w.stopFunc = nil
	w.running = false
	return err
}

func (w *FileWatcher) run(ctx context.Context, watcher *fsnotify.Watcher, path string, onChange func(), done chan<- struct{}) {
	defer func() {
		w.mu.Lock()
		if w.watcher == watcher {
			_ = w.stopLocked()
		}
		w.mu.Unlock()
		close(done)
	}()
	loop(ctx, watcher.Events, watcher.Errors, path, w.delay, onChange)
}

// loop debounces relevant events for path into onChange calls. It returns when
// ctx is done or either channel is closed.
func loop(
	ctx context.Context,
	events <-chan fsnotify.Event,
	errs <-chan error,
	path string,
	delay time.Duration,
	onChange func(),
) {
	d := newDebouncer(delay)
	defer d.stop()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if isRelevantFileEvent(event, path) {
				d.trigger()
			}
		case err, ok := <-errs:
			if !ok {
				return
			}
			slog.Warn("noctalia theme watcher error", "error", err)
		case <-d.C():
			d.fired()
			onChange()
		}
	}
}

// debouncer coalesces bursts of triggers into one fire after delay of quiet.
// It is not safe for concurrent use; the owning loop drives it.
type debouncer struct {
	delay time.Duration
	timer *time.Timer
	c     <-chan time.Time
}

func newDebouncer(delay time.Duration) *debouncer {
	return &debouncer{delay: delay}
}

// trigger (re)arms the timer so it fires delay after the latest trigger.
func (d *debouncer) trigger() {
	if d.timer == nil {
		d.timer = time.NewTimer(d.delay)
	} else {
		if !d.timer.Stop() {
			select {
			case <-d.timer.C:
			default:
			}
		}
		d.timer.Reset(d.delay)
	}
	d.c = d.timer.C
}

// C returns the fire channel, or nil when nothing is pending.
func (d *debouncer) C() <-chan time.Time { return d.c }

// fired must be called after receiving from C.
func (d *debouncer) fired() { d.c = nil }

func (d *debouncer) stop() {
	if d.timer != nil {
		d.timer.Stop()
	}
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

func isRelevantFileEvent(event fsnotify.Event, path string) bool {
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
		return false
	}
	return filepath.Clean(event.Name) == path
}

func (w *FileWatcher) isRunningForTest() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

func (w *FileWatcher) fsWatcherForTest() *fsnotify.Watcher {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.watcher
}

func (w *FileWatcher) doneForTest() <-chan struct{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.done
}
