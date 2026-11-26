package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	portmocks "bnema.dev/dumber/internal/application/port/mocks"
	"bnema.dev/dumber/internal/application/usecase"
	"bnema.dev/dumber/internal/domain/entity"
	repomocks "bnema.dev/dumber/internal/domain/repository/mocks"
)

// countingCloser records Close calls; io.Closer is a stdlib interface with no generated mock.
type countingCloser struct{ closes int }

func (c *countingCloser) Close() error {
	c.closes++
	return nil
}

func newLifetimeSession() *entity.Session {
	return &entity.Session{ID: "20260618_200000_life", Type: entity.SessionTypeBrowser, StartedAt: time.Now()}
}

func TestBrowserSessionLifetime_BeginAcquiresLeaseBeforeSaving(t *testing.T) {
	ctx := testContext()
	repo := repomocks.NewMockSessionRepository(t)
	leases := portmocks.NewMockSessionLeaseStore(t)
	session := newLifetimeSession()
	closer := &countingCloser{}

	acquire := leases.EXPECT().Acquire(ctx, session.ID).Return(closer, nil).Call
	repo.EXPECT().Save(ctx, session).Return(nil).NotBefore(acquire)

	lease, err := usecase.NewBrowserSessionLifetimeUseCase(repo, leases).Begin(ctx, session)

	require.NoError(t, err)
	require.NotNil(t, lease)
	assert.Zero(t, closer.closes)
}

func TestBrowserSessionLifetime_BeginSavesWithoutLeaseWhenAcquireFails(t *testing.T) {
	ctx := testContext()
	repo := repomocks.NewMockSessionRepository(t)
	leases := portmocks.NewMockSessionLeaseStore(t)
	session := newLifetimeSession()

	leases.EXPECT().Acquire(ctx, session.ID).Return(nil, errors.New("read-only"))
	repo.EXPECT().Save(ctx, session).Return(nil)

	uc := usecase.NewBrowserSessionLifetimeUseCase(repo, leases)
	lease, err := uc.Begin(ctx, session)
	require.NoError(t, err)

	endedAt := time.Date(2026, 6, 18, 21, 0, 0, 0, time.UTC)
	repo.EXPECT().MarkEnded(ctx, session.ID, endedAt).Return(nil)
	require.NoError(t, uc.End(ctx, session.ID, lease, endedAt), "no lease to release or remove")
}

func TestBrowserSessionLifetime_BeginReleasesAndRemovesLeaseWhenSaveFails(t *testing.T) {
	ctx := testContext()
	repo := repomocks.NewMockSessionRepository(t)
	leases := portmocks.NewMockSessionLeaseStore(t)
	session := newLifetimeSession()
	closer := &countingCloser{}

	leases.EXPECT().Acquire(ctx, session.ID).Return(closer, nil)
	repo.EXPECT().Save(ctx, session).Return(errors.New("disk full"))
	leases.EXPECT().Remove(ctx, session.ID).Return(nil)

	_, err := usecase.NewBrowserSessionLifetimeUseCase(repo, leases).Begin(ctx, session)

	require.ErrorContains(t, err, "disk full")
	assert.Equal(t, 1, closer.closes)
}

func TestBrowserSessionLifetime_End(t *testing.T) {
	tests := []struct {
		name       string
		markErr    error
		wantRemove bool
	}{
		{name: "ended session releases and removes its lease", wantRemove: true},
		{name: "failed end keeps the released lease file for the next instance", markErr: errors.New("database is locked")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := testContext()
			repo := repomocks.NewMockSessionRepository(t)
			leases := portmocks.NewMockSessionLeaseStore(t)
			session := newLifetimeSession()
			closer := &countingCloser{}
			endedAt := time.Date(2026, 6, 18, 21, 0, 0, 0, time.UTC)

			leases.EXPECT().Acquire(ctx, session.ID).Return(closer, nil)
			repo.EXPECT().Save(ctx, session).Return(nil)
			uc := usecase.NewBrowserSessionLifetimeUseCase(repo, leases)
			lease, err := uc.Begin(ctx, session)
			require.NoError(t, err)

			mark := repo.EXPECT().MarkEnded(ctx, session.ID, endedAt).Return(tt.markErr).Call
			if tt.wantRemove {
				leases.EXPECT().Remove(ctx, session.ID).Return(nil).NotBefore(mark)
			}

			err = uc.End(ctx, session.ID, lease, endedAt)

			if tt.markErr != nil {
				require.ErrorIs(t, err, tt.markErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, 1, closer.closes)
		})
	}
}

func TestBrowserSessionLifetime_NilLeaseStore(t *testing.T) {
	ctx := testContext()
	repo := repomocks.NewMockSessionRepository(t)
	session := newLifetimeSession()
	endedAt := time.Date(2026, 6, 18, 21, 0, 0, 0, time.UTC)

	repo.EXPECT().Save(ctx, session).Return(nil)
	repo.EXPECT().MarkEnded(ctx, session.ID, endedAt).Return(nil)

	uc := usecase.NewBrowserSessionLifetimeUseCase(repo, nil)
	lease, err := uc.Begin(ctx, session)
	require.NoError(t, err)
	require.NoError(t, uc.End(ctx, session.ID, lease, endedAt))
}
