package ui

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/application/port/mocks"
	"bnema.dev/dumber/internal/shared/syncdispatch"
)

// mainThreadRecorder queues posted callbacks instead of running them and
// records any synchronous dispatch, which watcher paths must never use.
type mainThreadRecorder struct {
	mu         sync.Mutex
	posted     []func()
	dispatched []string
}

func recordMainThread(app *App) *mainThreadRecorder {
	r := &mainThreadRecorder{}
	app.postOnMainThread = func(_ string, fn func()) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.posted = append(r.posted, fn)
	}
	app.dispatchOnMainThread = func(label string, _ func()) syncdispatch.SyncDispatchResult {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.dispatched = append(r.dispatched, label)
		return syncdispatch.SyncDispatchResult{Label: label}
	}
	return r
}

func (r *mainThreadRecorder) snapshot() (posted []func(), dispatched []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]func(){}, r.posted...), append([]string{}, r.dispatched...)
}

// userStyleReturning is a user.css mock whose reads return contents in order.
func userStyleReturning(t *testing.T, contents ...string) *mocks.MockSystemViewUserStyle {
	t.Helper()
	style := mocks.NewMockSystemViewUserStyle(t)
	for _, css := range contents {
		style.EXPECT().Read().Return(css, nil).Once()
	}
	style.EXPECT().Path().Return("/c/user.css").Maybe()
	return style
}

func TestReloadSystemViewUserCSSPostsAndDropsOlderReads(t *testing.T) {
	ctx := context.Background()
	app := &App{deps: &Dependencies{SystemViewUserStyle: userStyleReturning(t, "old", "new")}}
	rec := recordMainThread(app)

	app.reloadSystemViewUserCSS(ctx) // seq 1, the startup read
	app.reloadSystemViewUserCSS(ctx) // seq 2, a watcher read
	posted, dispatched := rec.snapshot()
	require.Len(t, posted, 2)
	assert.Empty(t, dispatched)

	// The newer read is applied first; the older one must not win after it.
	posted[1]()
	assert.Equal(t, uint64(2), app.userCSSAppliedSeq)
	posted[0]()
	assert.Equal(t, uint64(2), app.userCSSAppliedSeq)
}

func TestNotifyHomeConfigChangedPostsWithoutWaiting(t *testing.T) {
	app := &App{deps: &Dependencies{}}
	rec := recordMainThread(app)
	app.notifyHomeConfigChanged(context.Background())
	posted, dispatched := rec.snapshot()
	require.Len(t, posted, 1)
	assert.Empty(t, dispatched)
	posted[0]() // no content coordinator: a no-op
}

func TestInitSystemViewFileWatcherCallbacksPost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		var handlers port.SystemViewFileHandlers
		watcher := mocks.NewMockSystemViewFileWatcher(t)
		watcher.EXPECT().Start(mock.Anything, mock.Anything).
			Run(func(_ context.Context, h port.SystemViewFileHandlers) { handlers = h }).
			Return(nil).Once()
		app := &App{deps: &Dependencies{
			SystemViewUserStyle:   userStyleReturning(t, "startup", "changed"),
			SystemViewFileWatcher: watcher,
		}}
		rec := recordMainThread(app)

		app.initSystemViewFileWatcher(ctx)
		require.NotNil(t, handlers.OnUserCSS)
		require.NotNil(t, handlers.OnHomeConfig)
		handlers.OnUserCSS()
		handlers.OnHomeConfig()

		// The startup read runs on its own goroutine, inside the bubble:
		// Wait returns once it has posted, and synctest.Test fails if it
		// were still running when the test ends.
		synctest.Wait()
		posted, dispatched := rec.snapshot()
		assert.Len(t, posted, 3, "startup read, user.css change and home.toml change")
		assert.Empty(t, dispatched, "watcher callbacks must not wait for the main thread")
		assert.Equal(t, uint64(2), app.userCSSReadSeq.Load())
	})
}
