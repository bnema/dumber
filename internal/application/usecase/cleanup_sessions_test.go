package usecase_test

import (
	"errors"
	"testing"
	"time"

	"bnema.dev/dumber/internal/application/port"
	portmocks "bnema.dev/dumber/internal/application/port/mocks"
	"bnema.dev/dumber/internal/application/usecase"
	"bnema.dev/dumber/internal/domain/entity"
	repomocks "bnema.dev/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCleanupSessionsUseCase_ReconcileActiveBrowserSessions(t *testing.T) {
	now := time.Date(2026, 6, 18, 20, 0, 0, 0, time.UTC)
	sessionStart := now.Add(-time.Hour)
	pid := 222

	tests := []struct {
		name     string
		status   port.SessionProcessStatus
		probeErr error
		wantEnd  bool
	}{
		{name: "pid not held", status: port.SessionProcessStatus{}, wantEnd: true},
		{name: "holder started before session", status: port.SessionProcessStatus{Running: true, StartedAt: sessionStart.Add(-time.Second)}, wantEnd: false},
		{name: "holder started within reuse margin", status: port.SessionProcessStatus{Running: true, StartedAt: sessionStart.Add(30 * time.Second)}, wantEnd: false},
		{name: "holder started exactly at reuse margin", status: port.SessionProcessStatus{Running: true, StartedAt: sessionStart.Add(time.Minute)}, wantEnd: false},
		{name: "holder started just past reuse margin", status: port.SessionProcessStatus{Running: true, StartedAt: sessionStart.Add(time.Minute + time.Nanosecond)}, wantEnd: true},
		{name: "holder started after session reuses pid", status: port.SessionProcessStatus{Running: true, StartedAt: sessionStart.Add(2 * time.Minute)}, wantEnd: true},
		{name: "same executable survives clock step", status: port.SessionProcessStatus{Running: true, StartedAt: sessionStart.Add(time.Hour), SameExecutable: true}, wantEnd: false},
		{name: "probe error keeps session", probeErr: errors.New("probe failed"), wantEnd: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := testContext()
			sessionRepo := repomocks.NewMockSessionRepository(t)
			processProbe := portmocks.NewMockSessionProcessProbe(t)

			current := &entity.Session{ID: "current", Type: entity.SessionTypeBrowser, StartedAt: now}
			unknown := &entity.Session{ID: "unknown", Type: entity.SessionTypeBrowser, StartedAt: sessionStart}
			target := &entity.Session{ID: "target", Type: entity.SessionTypeBrowser, StartedAt: sessionStart, ProcessID: &pid}

			sessionRepo.EXPECT().
				ListActiveBrowser(ctx).
				Return([]*entity.Session{current, unknown, nil, target}, nil)
			processProbe.EXPECT().
				ProcessStatus(ctx, pid).
				Return(tt.status, tt.probeErr)
			if tt.wantEnd {
				sessionRepo.EXPECT().MarkEndedAtLastActivity(ctx, target.ID).Return(true, nil)
			}

			uc := usecase.NewCleanupSessionsUseCase(sessionRepo, processProbe, nil)
			output, err := uc.ReconcileActiveBrowserSessions(ctx, usecase.ReconcileActiveBrowserSessionsInput{
				CurrentSessionID: current.ID,
			})

			require.NoError(t, err)
			wantEnded := int64(0)
			if tt.wantEnd {
				wantEnded = 1
			}
			assert.Equal(t, wantEnded, output.EndedDeadSessions)
		})
	}
}

func TestCleanupSessionsUseCase_ReconcileActiveBrowserSessionsWithLeases(t *testing.T) {
	now := time.Date(2026, 6, 18, 20, 0, 0, 0, time.UTC)
	pid := 333
	reused := port.SessionProcessStatus{Running: true, StartedAt: now.Add(time.Hour)}

	tests := []struct {
		name      string
		state     port.SessionLeaseState
		leaseErr  error
		status    *port.SessionProcessStatus // nil: PID probe not expected
		removeErr error
		wantEnd   bool
	}{
		{name: "held lease keeps session without pid probe", state: port.SessionLeaseHeld},
		{name: "free lease ends session without pid probe", state: port.SessionLeaseFree, wantEnd: true},
		{name: "lease removal failure still ends session", state: port.SessionLeaseFree, removeErr: errors.New("busy"), wantEnd: true},
		{name: "lease probe error keeps session", leaseErr: errors.New("io")},
		{name: "missing lease falls back to pid probe", state: port.SessionLeaseMissing, status: &reused, wantEnd: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := testContext()
			sessionRepo := repomocks.NewMockSessionRepository(t)
			processProbe := portmocks.NewMockSessionProcessProbe(t)
			leases := portmocks.NewMockSessionLeaseStore(t)

			current := &entity.Session{ID: "current", Type: entity.SessionTypeBrowser, StartedAt: now}
			target := &entity.Session{ID: "target", Type: entity.SessionTypeBrowser, StartedAt: now.Add(-time.Hour), ProcessID: &pid}

			sessionRepo.EXPECT().ListActiveBrowser(ctx).Return([]*entity.Session{current, target}, nil)
			leases.EXPECT().Probe(ctx, target.ID).Return(tt.state, tt.leaseErr)
			if tt.status != nil {
				processProbe.EXPECT().ProcessStatus(ctx, pid).Return(*tt.status, nil)
			}
			if tt.wantEnd {
				sessionRepo.EXPECT().MarkEndedAtLastActivity(ctx, target.ID).Return(true, nil)
				leases.EXPECT().Remove(ctx, target.ID).Return(tt.removeErr)
			}

			uc := usecase.NewCleanupSessionsUseCase(sessionRepo, processProbe, leases)
			output, err := uc.ReconcileActiveBrowserSessions(ctx, usecase.ReconcileActiveBrowserSessionsInput{
				CurrentSessionID: current.ID,
			})

			require.NoError(t, err)
			wantEnded := int64(0)
			if tt.wantEnd {
				wantEnded = 1
			}
			assert.Equal(t, wantEnded, output.EndedDeadSessions)
		})
	}
}

func TestCleanupSessionsUseCase_ReconcileSkipsSessionEndedSinceListing(t *testing.T) {
	ctx := testContext()
	sessionRepo := repomocks.NewMockSessionRepository(t)
	leases := portmocks.NewMockSessionLeaseStore(t)
	target := &entity.Session{ID: "target", Type: entity.SessionTypeBrowser, StartedAt: time.Now()}

	sessionRepo.EXPECT().ListActiveBrowser(ctx).Return([]*entity.Session{target}, nil)
	leases.EXPECT().Probe(ctx, target.ID).Return(port.SessionLeaseFree, nil)
	sessionRepo.EXPECT().MarkEndedAtLastActivity(ctx, target.ID).Return(false, nil)

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil, leases)
	output, err := uc.ReconcileActiveBrowserSessions(ctx, usecase.ReconcileActiveBrowserSessionsInput{})

	require.NoError(t, err)
	assert.Equal(t, int64(0), output.EndedDeadSessions)
}

func TestCleanupSessionsUseCase_ReconcileMissingLeaseWithoutPIDProbeKeepsSession(t *testing.T) {
	ctx := testContext()
	sessionRepo := repomocks.NewMockSessionRepository(t)
	leases := portmocks.NewMockSessionLeaseStore(t)
	pid := 444
	withPID := &entity.Session{ID: "with-pid", Type: entity.SessionTypeBrowser, StartedAt: time.Now(), ProcessID: &pid}

	sessionRepo.EXPECT().ListActiveBrowser(ctx).Return([]*entity.Session{withPID}, nil)
	leases.EXPECT().Probe(ctx, withPID.ID).Return(port.SessionLeaseMissing, nil)

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil, leases)
	output, err := uc.ReconcileActiveBrowserSessions(ctx, usecase.ReconcileActiveBrowserSessionsInput{})

	require.NoError(t, err)
	assert.Equal(t, int64(0), output.EndedDeadSessions)
}

func TestCleanupSessionsUseCase_SweepOrphanLeases(t *testing.T) {
	ctx := testContext()
	sessionRepo := repomocks.NewMockSessionRepository(t)
	leases := portmocks.NewMockSessionLeaseStore(t)
	active := &entity.Session{ID: "active", Type: entity.SessionTypeBrowser, StartedAt: time.Now()}

	leases.EXPECT().List(ctx).Return([]entity.SessionID{"current", "active", "orphan-free", "orphan-held", "probe-error", "remove-error"}, nil)
	sessionRepo.EXPECT().ListActiveBrowser(ctx).Return([]*entity.Session{active, nil}, nil)
	leases.EXPECT().Probe(ctx, entity.SessionID("orphan-free")).Return(port.SessionLeaseFree, nil)
	leases.EXPECT().Probe(ctx, entity.SessionID("orphan-held")).Return(port.SessionLeaseHeld, nil)
	leases.EXPECT().Probe(ctx, entity.SessionID("probe-error")).Return(port.SessionLeaseMissing, errors.New("io"))
	leases.EXPECT().Probe(ctx, entity.SessionID("remove-error")).Return(port.SessionLeaseFree, nil)
	leases.EXPECT().Remove(ctx, entity.SessionID("orphan-free")).Return(nil)
	leases.EXPECT().Remove(ctx, entity.SessionID("remove-error")).Return(errors.New("busy"))

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil, leases)
	removed, err := uc.SweepOrphanLeases(ctx, "current")

	require.NoError(t, err)
	assert.Equal(t, 1, removed)
}

func TestCleanupSessionsUseCase_SweepOrphanLeasesWithoutLeasesSkipsRepository(t *testing.T) {
	ctx := testContext()
	leases := portmocks.NewMockSessionLeaseStore(t)
	leases.EXPECT().List(ctx).Return(nil, nil)

	removed, err := usecase.NewCleanupSessionsUseCase(repomocks.NewMockSessionRepository(t), nil, leases).
		SweepOrphanLeases(ctx, "current")

	require.NoError(t, err)
	assert.Zero(t, removed)
}

func TestCleanupSessionsUseCase_Execute_AgeAndCountCleanup(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)

	// Expect age-based deletion
	sessionRepo.EXPECT().
		DeleteExitedBefore(ctx, mock.MatchedBy(func(cutoff time.Time) bool {
			// Should be approximately 7 days ago
			expected := time.Now().AddDate(0, 0, -7)
			diff := expected.Sub(cutoff)
			return diff > -time.Minute && diff < time.Minute
		})).
		Return(int64(3), nil)

	// Expect count-based deletion
	sessionRepo.EXPECT().
		DeleteOldestExited(ctx, 50).
		Return(int64(2), nil)

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil, nil)

	output, err := uc.Execute(ctx, usecase.CleanupSessionsInput{
		MaxExitedSessions:       50,
		MaxExitedSessionAgeDays: 7,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(3), output.DeletedByAge)
	assert.Equal(t, int64(2), output.DeletedByCount)
	assert.Equal(t, int64(5), output.TotalDeleted)
}

func TestCleanupSessionsUseCase_Execute_AgeOnlyCleanup(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)

	// Expect age-based deletion
	sessionRepo.EXPECT().
		DeleteExitedBefore(ctx, mock.Anything).
		Return(int64(5), nil)

	// Count-based with -1 should be skipped (disabled)
	// MaxExitedSessions >= 0 triggers count cleanup, so use -1 to disable
	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil, nil)

	output, err := uc.Execute(ctx, usecase.CleanupSessionsInput{
		MaxExitedSessions:       -1, // Disabled
		MaxExitedSessionAgeDays: 30,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(5), output.DeletedByAge)
	assert.Equal(t, int64(0), output.DeletedByCount)
	assert.Equal(t, int64(5), output.TotalDeleted)
}

func TestCleanupSessionsUseCase_Execute_CountOnlyCleanup(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)

	// Count-based deletion only (age disabled with 0)
	sessionRepo.EXPECT().
		DeleteOldestExited(ctx, 10).
		Return(int64(4), nil)

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil, nil)

	output, err := uc.Execute(ctx, usecase.CleanupSessionsInput{
		MaxExitedSessions:       10,
		MaxExitedSessionAgeDays: 0, // Disabled
	})

	require.NoError(t, err)
	assert.Equal(t, int64(0), output.DeletedByAge)
	assert.Equal(t, int64(4), output.DeletedByCount)
	assert.Equal(t, int64(4), output.TotalDeleted)
}

func TestCleanupSessionsUseCase_Execute_NoCleanupNeeded(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)

	// Nothing to delete
	sessionRepo.EXPECT().
		DeleteExitedBefore(ctx, mock.Anything).
		Return(int64(0), nil)

	sessionRepo.EXPECT().
		DeleteOldestExited(ctx, 50).
		Return(int64(0), nil)

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil, nil)

	output, err := uc.Execute(ctx, usecase.CleanupSessionsInput{
		MaxExitedSessions:       50,
		MaxExitedSessionAgeDays: 7,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(0), output.DeletedByAge)
	assert.Equal(t, int64(0), output.DeletedByCount)
	assert.Equal(t, int64(0), output.TotalDeleted)
}

func TestCleanupSessionsUseCase_Execute_AgeDeleteError_ContinuesToCount(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)

	// Age-based deletion fails
	sessionRepo.EXPECT().
		DeleteExitedBefore(ctx, mock.Anything).
		Return(int64(0), errors.New("db error"))

	// Should still attempt count-based deletion
	sessionRepo.EXPECT().
		DeleteOldestExited(ctx, 50).
		Return(int64(3), nil)

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil, nil)

	output, err := uc.Execute(ctx, usecase.CleanupSessionsInput{
		MaxExitedSessions:       50,
		MaxExitedSessionAgeDays: 7,
	})

	// Should not return error, just log warning
	require.NoError(t, err)
	assert.Equal(t, int64(0), output.DeletedByAge)
	assert.Equal(t, int64(3), output.DeletedByCount)
	assert.Equal(t, int64(3), output.TotalDeleted)
}

func TestCleanupSessionsUseCase_Execute_CountDeleteError(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)

	// Age-based deletion succeeds
	sessionRepo.EXPECT().
		DeleteExitedBefore(ctx, mock.Anything).
		Return(int64(2), nil)

	// Count-based deletion fails
	sessionRepo.EXPECT().
		DeleteOldestExited(ctx, 50).
		Return(int64(0), errors.New("db error"))

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil, nil)

	output, err := uc.Execute(ctx, usecase.CleanupSessionsInput{
		MaxExitedSessions:       50,
		MaxExitedSessionAgeDays: 7,
	})

	// Should not return error, just log warning
	require.NoError(t, err)
	assert.Equal(t, int64(2), output.DeletedByAge)
	assert.Equal(t, int64(0), output.DeletedByCount)
	assert.Equal(t, int64(2), output.TotalDeleted)
}

func TestCleanupSessionsUseCase_Execute_BothDisabled(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)

	// No expectations - nothing should be called
	// Actually, count with 0 means "keep 0" which will trigger cleanup
	// So we need -1 for count and 0 for age to fully disable

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil, nil)

	output, err := uc.Execute(ctx, usecase.CleanupSessionsInput{
		MaxExitedSessions:       -1, // Disabled
		MaxExitedSessionAgeDays: 0,  // Disabled
	})

	require.NoError(t, err)
	assert.Equal(t, int64(0), output.DeletedByAge)
	assert.Equal(t, int64(0), output.DeletedByCount)
	assert.Equal(t, int64(0), output.TotalDeleted)
}

func TestCleanupSessionsUseCase_Execute_ZeroMaxSessions(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)

	// MaxExitedSessions = 0 means keep 0 sessions (delete all)
	sessionRepo.EXPECT().
		DeleteOldestExited(ctx, 0).
		Return(int64(10), nil)

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil, nil)

	output, err := uc.Execute(ctx, usecase.CleanupSessionsInput{
		MaxExitedSessions:       0, // Keep 0 = delete all
		MaxExitedSessionAgeDays: 0, // Disabled
	})

	require.NoError(t, err)
	assert.Equal(t, int64(0), output.DeletedByAge)
	assert.Equal(t, int64(10), output.DeletedByCount)
	assert.Equal(t, int64(10), output.TotalDeleted)
}
