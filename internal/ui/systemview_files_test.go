package ui

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/shared/syncdispatch"
)

// queuedUserStyle returns its contents in order, one per Read.
type queuedUserStyle struct {
	mu       sync.Mutex
	contents []string
}

func (s *queuedUserStyle) Path() string { return "/c/user.css" }

func (s *queuedUserStyle) Read() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	css := s.contents[0]
	s.contents = s.contents[1:]
	return css, nil
}

// postRecorder queues posted callbacks instead of running them, and fails
// the test on any synchronous dispatch.
func postRecorder(t *testing.T, app *App) *[]func() {
	t.Helper()
	var (
		mu     sync.Mutex
		posted []func()
	)
	app.postOnMainThread = func(_ string, fn func()) {
		mu.Lock()
		defer mu.Unlock()
		posted = append(posted, fn)
	}
	app.dispatchOnMainThread = func(label string, _ func()) syncdispatch.SyncDispatchResult {
		t.Fatalf("watcher path %q must not wait for the main thread", label)
		return syncdispatch.SyncDispatchResult{}
	}
	return &posted
}

func TestReloadSystemViewUserCSSPostsAndDropsOlderReads(t *testing.T) {
	ctx := context.Background()
	app := &App{deps: &Dependencies{SystemViewUserStyle: &queuedUserStyle{contents: []string{"old", "new"}}}}
	posted := postRecorder(t, app)

	app.reloadSystemViewUserCSS(ctx) // seq 1, the startup read
	app.reloadSystemViewUserCSS(ctx) // seq 2, a watcher read
	require.Len(t, *posted, 2)

	// The newer read is applied first; the older one must not win after it.
	(*posted)[1]()
	assert.Equal(t, uint64(2), app.userCSSAppliedSeq)
	(*posted)[0]()
	assert.Equal(t, uint64(2), app.userCSSAppliedSeq)
}

func TestNotifyHomeConfigChangedPostsWithoutWaiting(t *testing.T) {
	app := &App{deps: &Dependencies{}}
	posted := postRecorder(t, app)
	app.notifyHomeConfigChanged(context.Background())
	require.Len(t, *posted, 1)
	(*posted)[0]() // no content coordinator: a no-op
}

func TestInitSystemViewFileWatcherCallbacksPost(t *testing.T) {
	ctx := context.Background()
	watcher := &recordingSystemViewWatcher{}
	app := &App{deps: &Dependencies{
		SystemViewUserStyle:   &queuedUserStyle{contents: []string{"a", "b", "c"}},
		SystemViewFileWatcher: watcher,
	}}
	posted := postRecorder(t, app)
	app.initSystemViewFileWatcher(ctx)
	require.NotNil(t, watcher.handlers.OnUserCSS)
	require.NotNil(t, watcher.handlers.OnHomeConfig)
	watcher.handlers.OnHomeConfig()
	watcher.handlers.OnUserCSS()
	// The startup read runs on its own goroutine: wait for both reads.
	assert.Eventually(t, func() bool { return app.userCSSReadSeq.Load() == 2 }, time.Second, 5*time.Millisecond)
	_ = posted
}

type recordingSystemViewWatcher struct{ handlers port.SystemViewFileHandlers }

func (w *recordingSystemViewWatcher) Start(_ context.Context, h port.SystemViewFileHandlers) error {
	w.handlers = h
	return nil
}

func (*recordingSystemViewWatcher) Stop() error { return nil }
