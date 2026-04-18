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

	wv := &WebView{ctx: context.Background(), browser: browser, pendingURI: "https://github.com/bnema"}
	wv.replayPendingNavigation(0)

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

	wv := &WebView{ctx: context.Background(), browser: browser, pendingURI: "https://github.com/bnema"}
	wv.replayPendingNavigation(0)

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

	wv := &WebView{ctx: context.Background(), browser: browser, pendingURI: "https://github.com/bnema"}
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

	wv := &WebView{ctx: context.Background(), browser: staleBrowser, pendingURI: "https://github.com/bnema"}
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
