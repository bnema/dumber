package snapshot

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newWindowStateProvider(
	t *testing.T,
	sessionID entity.SessionID,
	windows []entity.WindowTabListState,
	activeWindowIndex int,
) *mocks.MockWindowStateProvider {
	provider := mocks.NewMockWindowStateProvider(t)
	provider.EXPECT().GetSessionID().Return(sessionID).Once()
	provider.EXPECT().GetWindowSnapshotState().Return(windows, activeWindowIndex).Once()
	return provider
}

func TestService_DrainSettlesAfterDebounceAndSave(t *testing.T) {
	repo := repomocks.NewMockSessionStateRepository(t)
	repo.EXPECT().
		SaveSnapshot(mock.Anything, mock.AnythingOfType("*entity.SessionState")).
		Return(nil).Once()
	uc := usecase.NewSnapshotSessionUseCase(repo)
	svc := NewService(uc, newWindowStateProvider(t, "20260207_120000_drain", nil, 0), 60000)
	svc.ready = true
	require.False(t, svc.Active())

	svc.MarkDirty()
	require.True(t, svc.Active(), "pending debounce is outstanding work")

	require.NoError(t, svc.SaveNow(context.Background()))
	require.False(t, svc.Active())
	require.NoError(t, svc.LastError())
}

func TestService_FailedSaveIsSettledButFailed(t *testing.T) {
	closedErr := errors.New("database is closed")
	repo := repomocks.NewMockSessionStateRepository(t)
	calls := 0
	repo.EXPECT().
		SaveSnapshot(mock.Anything, mock.AnythingOfType("*entity.SessionState")).
		RunAndReturn(func(_ context.Context, _ *entity.SessionState) error {
			calls++
			return closedErr
		})
	uc := usecase.NewSnapshotSessionUseCase(repo)
	svc := NewService(uc, newWindowStateProvider(t, "20260207_120000_closed", nil, 0), 60000)
	svc.ready = true
	svc.dirty = true

	err := svc.SaveNow(context.Background())
	require.ErrorIs(t, err, closedErr)
	require.Equal(t, 1, calls, "no write retries after terminal database failure")
	require.True(t, svc.dirty, "dirty preserved for reporting and later retry")
	require.ErrorIs(t, svc.LastError(), closedErr)
	require.False(t, svc.Active(), "terminal failure settles the drain")
	require.NoError(t, svc.WaitSettled(context.Background()))
}

func TestService_WaitSettledExpiry(t *testing.T) {
	repo := repomocks.NewMockSessionStateRepository(t)
	uc := usecase.NewSnapshotSessionUseCase(repo)
	svc := NewService(uc, mocks.NewMockWindowStateProvider(t), 60000)
	svc.MarkDirty()
	require.True(t, svc.Active())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.Error(t, svc.WaitSettled(ctx), "drain must expire instead of looping forever")
}

func TestService_OnSettledFiresAfterSave(t *testing.T) {
	repo := repomocks.NewMockSessionStateRepository(t)
	repo.EXPECT().
		SaveSnapshot(mock.Anything, mock.AnythingOfType("*entity.SessionState")).
		Return(nil).Once()
	uc := usecase.NewSnapshotSessionUseCase(repo)
	svc := NewService(uc, newWindowStateProvider(t, "20260207_120000_settled", nil, 0), 60000)
	svc.ready = true
	svc.dirty = true
	settled := make(chan struct{}, 1)
	svc.OnSettled(func() { settled <- struct{}{} })

	require.NoError(t, svc.SaveNow(context.Background()))
	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("settled callback must fire after save completes")
	}
}

func TestService_SaveSnapshot_RetriesTransientFKAndSucceeds(t *testing.T) {
	repo := repomocks.NewMockSessionStateRepository(t)
	calls := 0
	repo.EXPECT().
		SaveSnapshot(mock.Anything, mock.AnythingOfType("*entity.SessionState")).
		RunAndReturn(func(_ context.Context, _ *entity.SessionState) error {
			calls++
			if calls == 1 {
				return errors.New("FOREIGN KEY constraint failed")
			}
			return nil
		})

	uc := usecase.NewSnapshotSessionUseCase(repo)
	svc := NewService(uc, newWindowStateProvider(t, "20260207_120000_fk_retry_ok", nil, 0), 1)
	svc.retryDelay = time.Millisecond
	svc.ready = true
	svc.dirty = true

	err := svc.saveSnapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	assert.False(t, svc.dirty)
}

func TestService_SaveSnapshot_RetriesTransientFKAndFails(t *testing.T) {
	repo := repomocks.NewMockSessionStateRepository(t)
	calls := 0
	fkErr := errors.New("save session snapshot: FOREIGN KEY constraint failed")
	repo.EXPECT().
		SaveSnapshot(mock.Anything, mock.AnythingOfType("*entity.SessionState")).
		RunAndReturn(func(_ context.Context, _ *entity.SessionState) error {
			calls++
			return fkErr
		})

	uc := usecase.NewSnapshotSessionUseCase(repo)
	svc := NewService(uc, newWindowStateProvider(t, "20260207_120000_fk_retry_fail", nil, 0), 1)
	svc.retryDelay = time.Millisecond
	svc.ready = true
	svc.dirty = true

	err := svc.saveSnapshot(context.Background())
	require.Error(t, err)
	require.ErrorIs(t, err, fkErr)
	assert.Equal(t, svc.retries+1, calls)
	assert.True(t, svc.dirty)
}

func TestService_SaveSnapshot_DoesNotRetryNonFKError(t *testing.T) {
	repo := repomocks.NewMockSessionStateRepository(t)
	calls := 0
	nonFKErr := errors.New("database is read-only")
	repo.EXPECT().
		SaveSnapshot(mock.Anything, mock.AnythingOfType("*entity.SessionState")).
		RunAndReturn(func(_ context.Context, _ *entity.SessionState) error {
			calls++
			return nonFKErr
		})

	uc := usecase.NewSnapshotSessionUseCase(repo)
	svc := NewService(uc, newWindowStateProvider(t, "20260207_120000_non_fk", nil, 0), 1)
	svc.retryDelay = time.Millisecond
	svc.ready = true
	svc.dirty = true

	err := svc.saveSnapshot(context.Background())
	require.Error(t, err)
	require.ErrorIs(t, err, nonFKErr)
	assert.Equal(t, 1, calls)
	assert.True(t, svc.dirty)
}

func TestService_SetReady_SavesPendingDirtySnapshot(t *testing.T) {
	repo := repomocks.NewMockSessionStateRepository(t)
	saved := make(chan struct{}, 1)
	repo.EXPECT().
		SaveSnapshot(mock.Anything, mock.AnythingOfType("*entity.SessionState")).
		RunAndReturn(func(_ context.Context, _ *entity.SessionState) error {
			saved <- struct{}{}
			return nil
		})

	uc := usecase.NewSnapshotSessionUseCase(repo)
	svc := NewService(uc, newWindowStateProvider(t, "20260207_120000_ready_flush", nil, 0), 1)
	svc.Start(context.Background())
	svc.dirty = true

	svc.SetReady()

	select {
	case <-saved:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected pending snapshot to be saved after SetReady")
	}
	svc.mu.Lock()
	dirty := svc.dirty
	svc.mu.Unlock()
	assert.False(t, dirty)
}

func TestService_SaveNowPassesWindowSnapshots(t *testing.T) {
	repo := repomocks.NewMockSessionStateRepository(t)

	pane1 := entity.NewPane("p_w1")
	pane1.URI = "https://win1.com"
	tab1_1 := entity.NewTab("t1_w1", "ws1_w1", pane1)
	tab1_1.Name = "Win1Tab"
	tabs1 := entity.NewTabList()
	tabs1.Add(tab1_1)

	pane2 := entity.NewPane("p_w2")
	pane2.URI = "https://win2.com"
	tab2_1 := entity.NewTab("t1_w2", "ws1_w2", pane2)
	tab2_1.Name = "Win2Tab"
	tabs2 := entity.NewTabList()
	tabs2.Add(tab2_1)

	windows := []entity.WindowTabListState{
		{WindowID: "w1", Tabs: tabs1},
		{WindowID: "w2", Tabs: tabs2},
	}

	sessionID := entity.SessionID("20260501_window_snap")

	repo.EXPECT().
		SaveSnapshot(mock.Anything, mock.AnythingOfType("*entity.SessionState")).
		Run(func(_ context.Context, state *entity.SessionState) {
			require.Equal(t, sessionID, state.SessionID)
			require.Equal(t, 2, state.Version)
			require.Len(t, state.Windows, 2)
			assert.Equal(t, entity.WindowID("w1"), state.Windows[0].ID)
			assert.Equal(t, entity.WindowID("w2"), state.Windows[1].ID)
			assert.Equal(t, 1, state.ActiveWindowIndex)
			assert.Empty(t, state.Tabs)
		}).
		Return(nil)

	uc := usecase.NewSnapshotSessionUseCase(repo)
	provider := newWindowStateProvider(t, sessionID, windows, 1)
	svc := NewService(uc, provider, 1)
	svc.ready = true
	svc.dirty = true

	err := svc.saveSnapshot(context.Background())
	require.NoError(t, err)
	assert.False(t, svc.dirty)
}

func TestService_SaveSnapshotKeepsDirtyWhenWindowSnapshotUnavailable(t *testing.T) {
	repo := repomocks.NewMockSessionStateRepository(t)
	uc := usecase.NewSnapshotSessionUseCase(repo)
	provider := mocks.NewMockWindowStateProvider(t)
	provider.EXPECT().GetSessionID().Return(entity.SessionID("20260501_snapshot_unavailable")).Once()
	provider.EXPECT().GetWindowSnapshotState().Return(nil, -1).Once()

	svc := NewService(uc, provider, 1)
	svc.ready = true
	svc.dirty = true

	err := svc.saveSnapshot(context.Background())
	require.NoError(t, err)
	assert.True(t, svc.dirty)
}

func TestService_SaveNowPersistsEmptyWindowSnapshotAsV2(t *testing.T) {
	repo := repomocks.NewMockSessionStateRepository(t)
	sessionID := entity.SessionID("20260501_empty_window_snap")

	repo.EXPECT().
		SaveSnapshot(mock.Anything, mock.AnythingOfType("*entity.SessionState")).
		Run(func(_ context.Context, state *entity.SessionState) {
			require.Equal(t, sessionID, state.SessionID)
			require.Equal(t, entity.SessionStateVersion, state.Version)
			require.Empty(t, state.Windows)
			require.Empty(t, state.Tabs)
			assert.Equal(t, 0, state.ActiveWindowIndex)
		}).
		Return(nil)

	uc := usecase.NewSnapshotSessionUseCase(repo)
	provider := newWindowStateProvider(t, sessionID, []entity.WindowTabListState{}, 2)
	svc := NewService(uc, provider, 1)
	svc.ready = true
	svc.dirty = true

	err := svc.saveSnapshot(context.Background())
	require.NoError(t, err)
	assert.False(t, svc.dirty)
}

func TestService_SaveNowPreservesLegacySingleEmptyWindowSentinel(t *testing.T) {
	repo := repomocks.NewMockSessionStateRepository(t)
	sessionID := entity.SessionID("20260501_legacy_sentinel")

	pane := entity.NewPane("p_legacy")
	pane.URI = "https://legacy.example"
	tab := entity.NewTab("t_legacy", "ws_legacy", pane)
	tab.Name = "Legacy Tab"
	tabs := entity.NewTabList()
	tabs.Add(tab)

	repo.EXPECT().
		SaveSnapshot(mock.Anything, mock.AnythingOfType("*entity.SessionState")).
		Run(func(_ context.Context, state *entity.SessionState) {
			require.Equal(t, sessionID, state.SessionID)
			require.Equal(t, entity.LegacySessionStateVersion, state.Version)
			require.Len(t, state.Tabs, 1)
			assert.Equal(t, "Legacy Tab", state.Tabs[0].Name)
			assert.Empty(t, state.Windows)
			assert.Equal(t, 0, state.ActiveWindowIndex)
		}).
		Return(nil)

	uc := usecase.NewSnapshotSessionUseCase(repo)
	provider := newWindowStateProvider(t, sessionID, []entity.WindowTabListState{{WindowID: "", Tabs: tabs}}, 3)
	svc := NewService(uc, provider, 1)
	svc.ready = true
	svc.dirty = true

	err := svc.saveSnapshot(context.Background())
	require.NoError(t, err)
	assert.False(t, svc.dirty)
}

func TestService_DebounceTimerClearsOnFire(t *testing.T) {
	repo := repomocks.NewMockSessionStateRepository(t)
	repo.EXPECT().
		SaveSnapshot(mock.Anything, mock.AnythingOfType("*entity.SessionState")).
		Return(nil).Once()
	uc := usecase.NewSnapshotSessionUseCase(repo)
	svc := NewService(uc, newWindowStateProvider(t, "20260207_120000_fire", nil, 0), 1)
	svc.ready = true
	svc.ctx = context.Background()
	svc.MarkDirty()
	require.True(t, svc.Active())
	// The fired timer retires itself; the save settles the drain.
	require.Eventually(t, func() bool { return !svc.Active() }, 5*time.Second, 5*time.Millisecond)
}

func TestService_StopJoinsInFlightSave(t *testing.T) {
	released := make(chan struct{})
	var calls atomic.Int32
	repo := repomocks.NewMockSessionStateRepository(t)
	repo.EXPECT().
		SaveSnapshot(mock.Anything, mock.AnythingOfType("*entity.SessionState")).
		RunAndReturn(func(_ context.Context, _ *entity.SessionState) error {
			calls.Add(1)
			<-released
			return nil
		})
	uc := usecase.NewSnapshotSessionUseCase(repo)
	svc := NewService(uc, newWindowStateProvider(t, "20260207_120000_join", nil, 0), 1)
	svc.ready = true
	svc.dirty = true
	saveDone := make(chan error, 1)
	go func() { saveDone <- svc.saveSnapshot(context.Background()) }()
	// The in-flight save reached the database: dirty is cleared and the
	// provider was consumed exactly once, so Stop cannot start a second.
	require.Eventually(t, func() bool { return calls.Load() == 1 }, 5*time.Second, 5*time.Millisecond)
	stopDone := make(chan error, 1)
	go func() { stopDone <- svc.Stop(context.Background()) }()
	select {
	case err := <-stopDone:
		t.Fatalf("Stop returned while a save was still in flight: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(released)
	select {
	case err := <-saveDone:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight save never completed")
	}
	select {
	case err := <-stopDone:
		require.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("Stop did not join the in-flight save")
	}
	require.False(t, svc.Active())
	require.Equal(t, int32(1), calls.Load(), "Stop must join, not duplicate, the save")
}

func TestService_MarkDirtyNoOpAfterStop(t *testing.T) {
	repo := repomocks.NewMockSessionStateRepository(t)
	uc := usecase.NewSnapshotSessionUseCase(repo)
	svc := NewService(uc, mocks.NewMockWindowStateProvider(t), 1)
	require.NoError(t, svc.Stop(context.Background()))
	svc.MarkDirty()
	require.False(t, svc.Active(), "stopped service schedules nothing")
	time.Sleep(20 * time.Millisecond)
	require.False(t, svc.Active())
}

func TestService_SupersededTimerSavesNothing(t *testing.T) {
	var calls atomic.Int32
	repo := repomocks.NewMockSessionStateRepository(t)
	repo.EXPECT().
		SaveSnapshot(mock.Anything, mock.AnythingOfType("*entity.SessionState")).
		RunAndReturn(func(_ context.Context, _ *entity.SessionState) error {
			calls.Add(1)
			return nil
		})
	uc := usecase.NewSnapshotSessionUseCase(repo)
	svc := NewService(uc, newWindowStateProvider(t, "20260207_120000_superseded", nil, 0), 1)
	svc.ready = true
	svc.MarkDirty()
	// Stop consumes the pending work in its own final save and retires
	// the debounce timer; the stopped timer callback must save nothing.
	require.NoError(t, svc.Stop(context.Background()))
	require.Equal(t, int32(1), calls.Load())
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, int32(1), calls.Load(), "stopped timer must not save")
	require.False(t, svc.Active())
}
