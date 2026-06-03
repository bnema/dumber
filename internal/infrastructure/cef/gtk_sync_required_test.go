package cef

import (
	"context"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/shared/syncdispatch"
	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/require"
)

func TestConfigureInitialBrowserCreationClearsPendingCreateWhenGTKDispatchTimesOut(t *testing.T) {
	factory := &WebViewFactory{}
	wv := &WebView{
		ctx:            context.Background(),
		viewBridge:     &Cef2gtkAdapter{},
		gtkSyncTimeout: 5 * time.Millisecond,
		gtkSyncIsOwner: func() bool { return false },
		gtkSyncDispatch: func(func()) {
			// Simulate a GTK main loop that does not service the callback.
		},
	}
	windowInfo := purecef.NewWindowInfo()
	settings := purecef.NewBrowserSettings()

	err := factory.configureInitialBrowserCreation(context.Background(), wv, nil, &windowInfo, &settings, nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "configure initial browser creation")
	require.Nil(t, wv.pendingCreate)
}

func TestInstallViewportSyncHooksReportsIncompleteGTKDispatch(t *testing.T) {
	wv := &WebView{
		ctx:            context.Background(),
		viewBridge:     &Cef2gtkAdapter{},
		gtkSyncTimeout: 5 * time.Millisecond,
		gtkSyncIsOwner: func() bool { return false },
		gtkSyncDispatch: func(func()) {
			// Simulate a GTK main loop that does not service the callback.
		},
	}

	result := wv.installViewportSyncHooks()

	require.False(t, result.Completed())
	require.Equal(t, syncdispatch.SyncDispatchTimedOut, result.Status)
}

func TestDestroyViewBridgeOnGTKSyncLeavesCleanupQueuedAfterTimeout(t *testing.T) {
	delayed := make(chan func(), 1)
	wv := &WebView{
		ctx:            context.Background(),
		engine:         &Engine{},
		viewBridge:     &Cef2gtkAdapter{},
		gtkSyncTimeout: 5 * time.Millisecond,
		gtkSyncIsOwner: func() bool { return false },
		gtkSyncDispatch: func(fn func()) {
			delayed <- fn
		},
	}

	wv.destroyViewBridgeOnGTKSync()

	require.NotNil(t, wv.viewBridge)
	select {
	case fn := <-delayed:
		fn()
	case <-time.After(time.Second):
		t.Fatal("cleanup callback was not captured")
	}
	require.Nil(t, wv.viewBridge)
}

func TestRunOnGTKSyncCompletedAfterTimeoutStillCountsAsCompleted(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	wv := &WebView{
		ctx:            context.Background(),
		engine:         &Engine{},
		gtkSyncTimeout: 5 * time.Millisecond,
		gtkSyncIsOwner: func() bool { return false },
		gtkSyncDispatch: func(fn func()) {
			go fn()
		},
	}

	resultCh := make(chan syncdispatch.SyncDispatchResult, 1)
	go func() {
		resultCh <- wv.runOnGTKSync(func() {
			close(started)
			<-release
		})
	}()

	<-started
	time.Sleep(20 * time.Millisecond)
	close(release)

	result := <-resultCh
	require.Equal(t, syncdispatch.SyncDispatchCompletedAfterTimeout, result.Status)
	require.True(t, result.Completed())
}
