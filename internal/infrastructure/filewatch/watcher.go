// Package filewatch provides a debounced watcher for a set of files.
package filewatch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DefaultDebounce is the debounce delay used when New receives a non-positive delay.
const DefaultDebounce = 75 * time.Millisecond

// Watcher watches a set of files and reports debounced changes per file.
//
// It watches each file's parent directory so atomic writes and renames are
// observed, then filters events to the exact configured paths.
type Watcher struct {
	mu       sync.Mutex
	delay    time.Duration
	watcher  *fsnotify.Watcher
	paths    []string // sorted, deduplicated
	stopFunc context.CancelFunc
	running  bool
	// done is closed when the current run goroutine exits.
	done chan struct{}
}

// New creates a watcher. A non-positive delay selects DefaultDebounce.
func New(delay time.Duration) *Watcher {
	if delay <= 0 {
		delay = DefaultDebounce
	}
	return &Watcher{delay: delay}
}

// Start watches the given file paths and calls onChange(path) from the watcher
// goroutine after each file has been quiet for the debounce delay.
//
// Calling Start with the same path set (order-insensitive) while running is a
// no-op. A different set restarts the watcher. An empty set stops it.
func (w *Watcher) Start(ctx context.Context, paths []string, onChange func(path string)) error {
	normalized := normalizePaths(paths)
	if len(normalized) == 0 {
		return w.Stop()
	}
	if onChange == nil {
		return errors.New("file watcher callback is nil")
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running && slices.Equal(w.paths, normalized) {
		return nil
	}
	if err := w.stopLocked(); err != nil {
		return fmt.Errorf("stop previous file watcher: %w", err)
	}

	parents := parentDirs(normalized)
	for _, parent := range parents {
		info, err := os.Stat(parent)
		if err != nil {
			return fmt.Errorf("watch parent directory: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("watch parent directory: %s is not a directory", parent)
		}
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create file watcher: %w", err)
	}
	for _, parent := range parents {
		if addErr := watcher.Add(parent); addErr != nil {
			addErr = fmt.Errorf("watch parent directory %s: %w", parent, addErr)
			if closeErr := watcher.Close(); closeErr != nil {
				return errors.Join(addErr, fmt.Errorf("close file watcher after add failure: %w", closeErr))
			}
			return addErr
		}
	}

	runCtx, cancel := context.WithCancel(ctx)
	w.watcher = watcher
	w.paths = normalized
	w.stopFunc = cancel
	w.running = true
	w.done = make(chan struct{})

	go w.run(runCtx, watcher, normalized, onChange, w.done)
	return nil
}

// Stop stops the current watcher and waits for its goroutine to exit, so no
// onChange call runs after Stop returns. It is idempotent and safe for
// concurrent use.
//
// Stop must not be called from inside onChange (it would wait for itself),
// nor while holding a lock that onChange acquires.
func (w *Watcher) Stop() error {
	w.mu.Lock()
	done := w.done
	err := w.stopLocked()
	w.mu.Unlock()
	if done != nil {
		<-done
	}
	return err
}

func (w *Watcher) stopLocked() error {
	if !w.running {
		return nil
	}
	if w.stopFunc != nil {
		w.stopFunc()
	}
	err := w.watcher.Close()
	w.watcher = nil
	w.paths = nil
	w.stopFunc = nil
	w.running = false
	return err
}

func (w *Watcher) run(
	ctx context.Context,
	watcher *fsnotify.Watcher,
	paths []string,
	onChange func(string),
	done chan<- struct{},
) {
	defer func() {
		w.mu.Lock()
		if w.watcher == watcher {
			_ = w.stopLocked()
		}
		w.mu.Unlock()
		close(done)
	}()
	loop(ctx, watcher.Events, watcher.Errors, paths, w.delay, onChange)
}

// loop debounces relevant events per path into onChange calls. It returns
// when ctx is done or either channel is closed.
func loop(
	ctx context.Context,
	events <-chan fsnotify.Event,
	errs <-chan error,
	paths []string,
	delay time.Duration,
	onChange func(string),
) {
	watched := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		watched[p] = struct{}{}
	}
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
			if path, relevant := relevantPath(event, watched); relevant {
				d.trigger(path, time.Now())
			}
		case err, ok := <-errs:
			if !ok {
				return
			}
			slog.Warn("file watcher error", "error", err)
		case now := <-d.C():
			for _, path := range d.fired(now) {
				onChange(path)
			}
		}
	}
}

// debouncer coalesces bursts of triggers per key into one fire after delay of
// quiet for that key. It uses one timer armed for the earliest deadline.
// It is not safe for concurrent use; the owning loop drives it.
type debouncer struct {
	delay     time.Duration
	deadlines map[string]time.Time
	timer     *time.Timer
	c         <-chan time.Time
}

func newDebouncer(delay time.Duration) *debouncer {
	return &debouncer{delay: delay, deadlines: make(map[string]time.Time)}
}

// trigger (re)arms key so it fires delay after now.
func (d *debouncer) trigger(key string, now time.Time) {
	d.deadlines[key] = now.Add(d.delay)
	d.rearm(now)
}

// C returns the fire channel, or nil when nothing is pending.
func (d *debouncer) C() <-chan time.Time { return d.c }

// fired must be called after receiving from C. It returns the keys whose
// deadline has passed (sorted) and rearms for the remaining ones.
func (d *debouncer) fired(now time.Time) []string {
	d.c = nil
	var due []string
	for key, deadline := range d.deadlines {
		if !deadline.After(now) {
			due = append(due, key)
			delete(d.deadlines, key)
		}
	}
	slices.Sort(due)
	d.rearm(now)
	return due
}

func (d *debouncer) rearm(now time.Time) {
	if len(d.deadlines) == 0 {
		d.c = nil
		return
	}
	var earliest time.Time
	for _, deadline := range d.deadlines {
		if earliest.IsZero() || deadline.Before(earliest) {
			earliest = deadline
		}
	}
	wait := max(earliest.Sub(now), 0)
	if d.timer == nil {
		d.timer = time.NewTimer(wait)
	} else {
		d.timer.Stop()
		select {
		case <-d.timer.C:
		default:
		}
		d.timer.Reset(wait)
	}
	d.c = d.timer.C
}

func (d *debouncer) stop() {
	if d.timer != nil {
		d.timer.Stop()
	}
}

func relevantPath(event fsnotify.Event, watched map[string]struct{}) (string, bool) {
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
		return "", false
	}
	path := filepath.Clean(event.Name)
	_, ok := watched[path]
	return path, ok
}

func normalizePaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		out = append(out, filepath.Clean(p))
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func parentDirs(paths []string) []string {
	parents := make([]string, 0, len(paths))
	for _, p := range paths {
		parents = append(parents, filepath.Dir(p))
	}
	slices.Sort(parents)
	return slices.Compact(parents)
}

// Running reports whether the watcher is currently active.
func (w *Watcher) Running() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

func (w *Watcher) fsWatcherForTest() *fsnotify.Watcher {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.watcher
}

func (w *Watcher) doneForTest() <-chan struct{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.done
}
