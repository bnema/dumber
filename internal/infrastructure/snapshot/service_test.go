package snapshot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type testProvider struct {
	sessionID entity.SessionID
	tabList   *entity.TabList
}

func (p *testProvider) GetTabList() *entity.TabList {
	return p.tabList
}

func (p *testProvider) GetSessionID() entity.SessionID {
	return p.sessionID
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
	svc := NewService(uc, &testProvider{
		sessionID: "20260207_120000_fk_retry_ok",
		tabList:   entity.NewTabList(),
	}, 1)
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
	svc := NewService(uc, &testProvider{
		sessionID: "20260207_120000_fk_retry_fail",
		tabList:   entity.NewTabList(),
	}, 1)
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
	svc := NewService(uc, &testProvider{
		sessionID: "20260207_120000_non_fk",
		tabList:   entity.NewTabList(),
	}, 1)
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
	svc := NewService(uc, &testProvider{
		sessionID: "20260207_120000_ready_flush",
		tabList:   entity.NewTabList(),
	}, 1)
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
