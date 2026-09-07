package cef

import (
	"context"
	"testing"
	"time"

	purecef "github.com/bnema/purego-cef/cef"
	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/require"
)

func TestWebViewReplayPendingNavigation_LoadsQueuedURIWhenMainFrameAvailable(t *testing.T) {
	browser := cefmocks.NewMockBrowser(t)
	frame := cefmocks.NewMockFrame(t)
	frame.EXPECT().GetURL().Return("").Once()
	frame.EXPECT().LoadURL("https://github.com/bnema").Once()
	browser.EXPECT().GetMainFrame().Return(frame).Once()
	browser.EXPECT().GetIdentifier().Return(int32(1)).Twice()

	wv := &WebView{ctx: context.Background(), browser: browser}
	wv.setPendingNavigationLocked("https://github.com/bnema", time.Now())
	wv.replayPendingNavigationForIntent(0, wv.pendingIntentID)

	wv.mu.RLock()
	defer wv.mu.RUnlock()
	require.Equal(t, "https://github.com/bnema", wv.pendingURI)
}

func TestWebViewReplayPendingNavigation_RetriesWhenMainFrameUnavailable(t *testing.T) {
	browser := cefmocks.NewMockBrowser(t)
	browser.EXPECT().GetMainFrame().Return(purecef.Frame(nil)).Once()

	oldTask := cefNewTask
	oldDelayed := cefPostDelayedTask
	defer func() {
		cefNewTask = oldTask
		cefPostDelayedTask = oldDelayed
	}()
	cefNewTask = func(task purecef.Task) purecef.Task { return task }

	scheduled := false
	cefPostDelayedTask = func(threadID purecef.ThreadID, task purecef.Task, delayMs int64) int32 {
		require.Equal(t, purecef.ThreadIDTidUi, threadID)
		require.NotNil(t, task)
		require.Equal(t, int64(pendingNavigationRetryDelay/time.Millisecond), delayMs)
		scheduled = true
		return 1
	}

	wv := &WebView{ctx: context.Background(), browser: browser}
	wv.setPendingNavigationLocked("https://github.com/bnema", time.Now())
	wv.replayPendingNavigationForIntent(0, wv.pendingIntentID)

	require.True(t, scheduled)
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	require.Equal(t, "https://github.com/bnema", wv.pendingURI)
}

func TestWebViewSchedulePendingNavigationReplay_RetriesWhenTaskPostFails(t *testing.T) {
	browser := cefmocks.NewMockBrowser(t)

	oldTask := cefNewTask
	oldPost := cefPostTask
	oldDelayed := cefPostDelayedTask
	oldAfter := cefScheduleAfter
	defer func() {
		cefNewTask = oldTask
		cefPostTask = oldPost
		cefPostDelayedTask = oldDelayed
		cefScheduleAfter = oldAfter
	}()
	cefNewTask = func(task purecef.Task) purecef.Task { return task }

	retried := false
	cefPostTask = func(threadID purecef.ThreadID, task purecef.Task) int32 {
		require.Equal(t, purecef.ThreadIDTidUi, threadID)
		require.NotNil(t, task)
		return 0
	}
	cefScheduleAfter = func(delay time.Duration, fn func()) {
		require.Equal(t, pendingNavigationRetryDelay, delay)
		fn()
	}
	cefPostDelayedTask = func(threadID purecef.ThreadID, task purecef.Task, delayMs int64) int32 {
		require.Equal(t, purecef.ThreadIDTidUi, threadID)
		require.NotNil(t, task)
		require.Equal(t, int64(pendingNavigationRetryDelay/time.Millisecond), delayMs)
		retried = true
		return 1
	}

	wv := &WebView{ctx: context.Background(), browser: browser}
	wv.setPendingNavigationLocked("https://github.com/bnema", time.Now())
	wv.schedulePendingNavigationReplay(0)

	require.True(t, retried)
}

func TestWebViewReplayPendingNavigation_UsesCurrentBrowserAtExecutionTime(t *testing.T) {
	staleBrowser := cefmocks.NewMockBrowser(t)
	activeBrowser := cefmocks.NewMockBrowser(t)
	frame := cefmocks.NewMockFrame(t)
	frame.EXPECT().GetURL().Return("").Once()
	frame.EXPECT().LoadURL("https://github.com/bnema").Once()
	activeBrowser.EXPECT().GetMainFrame().Return(frame).Once()
	activeBrowser.EXPECT().GetIdentifier().Return(int32(2)).Twice()

	oldTask := cefNewTask
	oldPost := cefPostTask
	defer func() {
		cefNewTask = oldTask
		cefPostTask = oldPost
	}()
	var scheduled purecef.Task
	cefNewTask = func(task purecef.Task) purecef.Task { return task }
	cefPostTask = func(threadID purecef.ThreadID, task purecef.Task) int32 {
		require.Equal(t, purecef.ThreadIDTidUi, threadID)
		scheduled = task
		return 1
	}

	wv := &WebView{ctx: context.Background(), browser: staleBrowser}
	wv.setPendingNavigationLocked("https://github.com/bnema", time.Now())
	wv.schedulePendingNavigationReplay(0)
	wv.mu.Lock()
	wv.browser = activeBrowser
	wv.mu.Unlock()

	require.NotNil(t, scheduled)
	scheduled.Execute()
}

func TestWebViewLoadURI_QueuesPendingNavigationReplayForExistingBrowser(t *testing.T) {
	browser := cefmocks.NewMockBrowser(t)
	frame := cefmocks.NewMockFrame(t)
	browser.EXPECT().GetMainFrame().Return(frame).Once()
	browser.EXPECT().GetIdentifier().Return(int32(1)).Twice()
	frame.EXPECT().GetURL().Return("").Once()
	frame.EXPECT().LoadURL("github.com/bnema").Once()

	oldTask := cefNewTask
	oldPost := cefPostTask
	defer func() {
		cefNewTask = oldTask
		cefPostTask = oldPost
	}()
	cefNewTask = func(task purecef.Task) purecef.Task { return task }
	cefPostTask = func(threadID purecef.ThreadID, task purecef.Task) int32 {
		require.Equal(t, purecef.ThreadIDTidUi, threadID)
		require.NotNil(t, task)
		task.Execute()
		return 1
	}

	wv := &WebView{ctx: context.Background(), browser: browser}
	require.NoError(t, wv.LoadURI(context.Background(), "github.com/bnema"))

	wv.mu.RLock()
	defer wv.mu.RUnlock()
	require.Equal(t, "github.com/bnema", wv.pendingURI)
}

func TestWebViewUpdateURI_ClearsMatchingPendingNavigation(t *testing.T) {
	wv := &WebView{ctx: context.Background(), pendingURI: "https://github.com/bnema"}

	wv.updateURI("https://github.com/bnema")

	require.Empty(t, wv.pendingNavigationURI())
}

func TestWebViewUpdateLoadState_ClearsStalePendingNavigationAfterStartedRedirectCompletes(t *testing.T) {
	wv := &WebView{ctx: context.Background()}
	pending := "https://duckduckgo.com/?q=pi+agent+reddit"
	startedAt := time.Now().Add(-time.Second)
	wv.mu.Lock()
	wv.setPendingNavigationLocked(pending, startedAt.Add(-time.Millisecond))
	wv.markPendingNavigationStartedLocked(pending, startedAt)
	wv.mu.Unlock()
	wv.updateURI("https://www.reddit.com/r/somewhere/")

	wv.updateLoadState(false, true, false)

	require.Empty(t, wv.pendingNavigationURI())
}

func TestWebViewUpdateLoadState_KeepsPendingNavigationBeforeReplayStarts(t *testing.T) {
	wv := &WebView{ctx: context.Background()}
	pending := "https://duckduckgo.com/?q=pi+agent+reddit"
	wv.updateURI("https://old.example/still-finishing")
	wv.mu.Lock()
	wv.setPendingNavigationLocked(pending, time.Now())
	wv.mu.Unlock()

	wv.updateLoadState(false, true, false)

	require.Equal(t, pending, wv.pendingNavigationURI())
}

func TestWebViewUpdateLoadState_DoesNotClearPendingNavigationWhenOnlyOlderAddressWasObserved(t *testing.T) {
	wv := &WebView{ctx: context.Background()}
	pending := "https://duckduckgo.com/?q=pi+agent+reddit"
	observedAt := time.Now()
	wv.mu.Lock()
	wv.uri = "https://old.example/final"
	wv.loadDiagLastAddressAt = observedAt
	wv.setPendingNavigationLocked(pending, observedAt.Add(time.Millisecond))
	wv.markPendingNavigationStartedLocked(pending, observedAt.Add(time.Millisecond))
	wv.mu.Unlock()

	wv.updateLoadState(false, true, false)

	require.Equal(t, pending, wv.pendingNavigationURI())
}

func TestWebViewUpdateLoadState_KeepsPendingNavigationWhileLoading(t *testing.T) {
	wv := &WebView{ctx: context.Background()}
	pending := "https://duckduckgo.com/?q=pi+agent+reddit"
	startedAt := time.Now().Add(-time.Second)
	wv.mu.Lock()
	wv.setPendingNavigationLocked(pending, startedAt.Add(-time.Millisecond))
	wv.markPendingNavigationStartedLocked(pending, startedAt)
	wv.mu.Unlock()
	wv.updateURI("https://www.reddit.com/r/somewhere/")

	wv.updateLoadState(true, true, false)

	require.Equal(t, pending, wv.pendingNavigationURI())
}

// TestWebViewReplayPendingNavigation_SubmitsOncePerIntent reproduces the
// fresh-window race: queue an intent, run OnAfterCreated replay with the
// frame URL still blank, then deliver blank OnLoadEnd before commit. Exactly
// one LoadURL must be issued for the intent.
func TestWebViewReplayPendingNavigation_SubmitsOncePerIntent(t *testing.T) {
	browser := cefmocks.NewMockBrowser(t)
	frame := cefmocks.NewMockFrame(t)
	frame.EXPECT().GetURL().Return("about:blank")
	frame.EXPECT().LoadURL("https://example.com/target").Once()
	browser.EXPECT().GetMainFrame().Return(frame)
	browser.EXPECT().GetIdentifier().Return(int32(1)).Twice()

	oldTask := cefNewTask
	oldPost := cefPostTask
	defer func() {
		cefNewTask = oldTask
		cefPostTask = oldPost
	}()
	cefNewTask = func(task purecef.Task) purecef.Task { return task }
	var scheduled []purecef.Task
	cefPostTask = func(_ purecef.ThreadID, task purecef.Task) int32 {
		scheduled = append(scheduled, task)
		return 1
	}

	wv := &WebView{ctx: context.Background(), browser: browser}
	wv.setPendingNavigationLocked("https://example.com/target", time.Now())
	// OnAfterCreated schedules replay...
	wv.schedulePendingNavigationReplay(0)
	// ...and blank OnLoadEnd schedules a second replay before commit.
	wv.schedulePendingNavigationReplay(0)
	require.Len(t, scheduled, 2)
	for _, task := range scheduled {
		task.Execute()
	}
}

// TestWebViewReplayPendingNavigation_StaleIntentDoesNotResubmit verifies a
// rapid replacement URL invalidates the previously scheduled task.
func TestWebViewReplayPendingNavigation_StaleIntentDoesNotResubmit(t *testing.T) {
	browser := cefmocks.NewMockBrowser(t)
	frame := cefmocks.NewMockFrame(t)
	frame.EXPECT().GetURL().Return("").Once()
	frame.EXPECT().LoadURL("https://example.com/second").Once()
	browser.EXPECT().GetMainFrame().Return(frame).Once()
	browser.EXPECT().GetIdentifier().Return(int32(1)).Twice()

	oldTask := cefNewTask
	oldPost := cefPostTask
	defer func() {
		cefNewTask = oldTask
		cefPostTask = oldPost
	}()
	cefNewTask = func(task purecef.Task) purecef.Task { return task }
	var scheduled []purecef.Task
	cefPostTask = func(_ purecef.ThreadID, task purecef.Task) int32 {
		scheduled = append(scheduled, task)
		return 1
	}

	wv := &WebView{ctx: context.Background(), browser: browser}
	wv.setPendingNavigationLocked("https://example.com/first", time.Now())
	wv.schedulePendingNavigationReplay(0)
	require.Len(t, scheduled, 1)
	stale := scheduled[0]
	// Replacement installs a new intent; the stale task must not submit.
	wv.setPendingNavigationLocked("https://example.com/second", time.Now())
	stale.Execute()
	// Current intent still unissued; a fresh replay submits it once.
	wv.replayPendingNavigationForIntent(0, wv.pendingIntentID)
}

// TestWebViewReplayPendingNavigation_RetriesSameIntentOnFailedPost ensures a
// failed task post retries without consuming or duplicating the intent.
func TestWebViewReplayPendingNavigation_RetriesSameIntentOnFailedPost(t *testing.T) {
	browser := cefmocks.NewMockBrowser(t)

	oldTask := cefNewTask
	oldPost := cefPostTask
	oldAfter := cefScheduleAfter
	defer func() {
		cefNewTask = oldTask
		cefPostTask = oldPost
		cefScheduleAfter = oldAfter
	}()
	cefNewTask = func(task purecef.Task) purecef.Task { return task }
	cefPostTask = func(_ purecef.ThreadID, _ purecef.Task) int32 {
		return 0
	}
	scheduledAfter := false
	cefScheduleAfter = func(delay time.Duration, _ func()) {
		require.Equal(t, pendingNavigationRetryDelay, delay)
		scheduledAfter = true
	}

	wv := &WebView{ctx: context.Background(), browser: browser}
	wv.setPendingNavigationLocked("https://example.com/target", time.Now())
	wv.mu.RLock()
	intent := wv.pendingIntentID
	wv.mu.RUnlock()
	wv.schedulePendingNavigationReplay(0)
	require.True(t, scheduledAfter)
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	require.Equal(t, intent, wv.pendingIntentID)
	require.False(t, wv.pendingIssued)
	require.Equal(t, "https://example.com/target", wv.pendingURI)
}

// TestWebViewSetPendingNavigation_RepeatedSameURLInstallsNewIntent ensures
// explicit reloads and repeated navigations still work: each request is a
// new intent even for an identical URL.
func TestWebViewSetPendingNavigation_RepeatedSameURLInstallsNewIntent(t *testing.T) {
	wv := &WebView{ctx: context.Background()}
	wv.setPendingNavigationLocked("https://example.com/page", time.Now())
	first := wv.pendingIntentID
	require.NotZero(t, first)
	wv.mu.Lock()
	wv.pendingIssued = true
	wv.mu.Unlock()
	wv.setPendingNavigationLocked("https://example.com/page", time.Now())
	require.NotEqual(t, first, wv.pendingIntentID)
	require.False(t, wv.pendingIssued)
}

// TestWebViewReplayPendingNavigation_SameURLReloadSubmits verifies an
// unissued explicit intent for the currently displayed URL proceeds to
// LoadURL instead of being cleared as already active.
func TestWebViewReplayPendingNavigation_SameURLReloadSubmits(t *testing.T) {
	browser := cefmocks.NewMockBrowser(t)
	frame := cefmocks.NewMockFrame(t)
	frame.EXPECT().GetURL().Return("https://example.com/page").Once()
	frame.EXPECT().LoadURL("https://example.com/page").Once()
	browser.EXPECT().GetMainFrame().Return(frame).Once()
	browser.EXPECT().GetIdentifier().Return(int32(1)).Twice()

	wv := &WebView{ctx: context.Background(), browser: browser}
	wv.setPendingNavigationLocked("https://example.com/page", time.Now())
	wv.replayPendingNavigationForIntent(0, wv.pendingIntentID)

	wv.mu.RLock()
	defer wv.mu.RUnlock()
	require.True(t, wv.pendingIssued)
}
