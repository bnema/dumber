package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	portmocks "bnema.dev/dumber/internal/application/port/mocks"
	"bnema.dev/dumber/internal/application/usecase"
	"bnema.dev/dumber/internal/cli"
	"bnema.dev/dumber/internal/cli/styles"
	"bnema.dev/dumber/internal/domain/entity"
	repomocks "bnema.dev/dumber/internal/domain/repository/mocks"
	"bnema.dev/dumber/internal/infrastructure/config"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSessionCommandsReturnManagementErrorWhenSessionUseCaseMissing(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "interactive sessions",
			run:  func() error { return runSessions(nil, nil) },
		},
		{
			name: "list sessions",
			run:  func() error { return runSessionsList(nil, nil) },
		},
		{
			name: "delete session",
			run:  func() error { return runSessionsDelete(nil, []string{"session-id"}) },
		},
		{
			name: "find session helper",
			run: func() error {
				_, err := findSessionByIDOrSuffix("session-id")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withApp(t, sessionCommandTestApp())
			discardStderr(t)

			err := tt.run()

			require.ErrorContains(t, err, "session management not available")
		})
	}
}

func TestSessionRestoreReturnsRestorationErrorWhenSessionUseCaseMissing(t *testing.T) {
	withApp(t, sessionCommandTestApp())
	discardStderr(t)

	err := runSessionsRestore(nil, []string{"session-id"})

	require.ErrorContains(t, err, "session restoration not available")
}

func TestFindSessionByIDOrSuffixSearchesBeyondDefaultLimit(t *testing.T) {
	targetID := entity.SessionID("20251224_000000_targetolder")
	sessions := make([]*entity.Session, 0, defaultSessionsLimit*5+1)
	startedAt := time.Date(2025, 12, 24, 12, 0, 0, 0, time.UTC)
	for i := range defaultSessionsLimit * 5 {
		sessions = append(sessions, &entity.Session{
			ID:        entity.SessionID(fmt.Sprintf("20251224_1200%02d_recent", i)),
			Type:      entity.SessionTypeBrowser,
			StartedAt: startedAt.Add(-time.Duration(i) * time.Minute),
		})
	}
	sessions = append(sessions, &entity.Session{
		ID:        targetID,
		Type:      entity.SessionTypeBrowser,
		StartedAt: startedAt.Add(-time.Hour),
	})

	sessionRepo := &sessionCommandSessionRepo{sessions: sessions}
	cfg := config.DefaultConfig()
	withApp(t, &cli.App{
		Config:         cfg,
		Theme:          styles.NewTheme(cfg),
		SessionUC:      usecase.NewManageSessionUseCase(sessionRepo, nil),
		ListSessionsUC: usecase.NewListSessionsUseCase(sessionRepo, sessionCommandStateRepo{}),
	})

	got, err := findSessionByIDOrSuffix("targetolder")

	require.NoError(t, err)
	require.Equal(t, targetID, got.Session.ID)
}

func sessionCommandTestApp() *cli.App {
	cfg := config.DefaultConfig()
	return &cli.App{
		Config:          cfg,
		Theme:           styles.NewTheme(cfg),
		ListSessionsUC:  &usecase.ListSessionsUseCase{},
		RestoreUC:       &usecase.RestoreSessionUseCase{},
		DeleteSessionUC: &usecase.DeleteSessionUseCase{},
	}
}

func withApp(t *testing.T, testApp *cli.App) {
	t.Helper()
	oldApp := app
	app = testApp
	t.Cleanup(func() { app = oldApp })
}

func discardStderr(t *testing.T) {
	t.Helper()
	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = writer
	t.Cleanup(func() {
		os.Stderr = oldStderr
		require.NoError(t, writer.Close())
		_, _ = io.Copy(io.Discard, reader)
		require.NoError(t, reader.Close())
	})
}

type sessionCommandSessionRepo struct {
	sessions []*entity.Session
}

func (*sessionCommandSessionRepo) Save(context.Context, *entity.Session) error {
	return nil
}

func (r *sessionCommandSessionRepo) FindByID(_ context.Context, id entity.SessionID) (*entity.Session, error) {
	for _, session := range r.sessions {
		if session.ID == id {
			return session, nil
		}
	}
	return nil, nil
}

func (*sessionCommandSessionRepo) GetActive(context.Context) (*entity.Session, error) {
	return nil, nil
}

func (*sessionCommandSessionRepo) ListActiveBrowser(context.Context) ([]*entity.Session, error) {
	return nil, nil
}

func (r *sessionCommandSessionRepo) GetRecent(_ context.Context, limit int) ([]*entity.Session, error) {
	if limit < 0 || limit > len(r.sessions) {
		limit = len(r.sessions)
	}
	out := make([]*entity.Session, limit)
	copy(out, r.sessions[:limit])
	return out, nil
}

func (*sessionCommandSessionRepo) MarkEndedAtLastActivity(context.Context, entity.SessionID) error {
	return nil
}

func (*sessionCommandSessionRepo) MarkEnded(context.Context, entity.SessionID, time.Time) error {
	return nil
}

func (*sessionCommandSessionRepo) Delete(context.Context, entity.SessionID) error {
	return nil
}

func (*sessionCommandSessionRepo) DeleteOldestExited(context.Context, int) (int64, error) {
	return 0, nil
}

func (*sessionCommandSessionRepo) DeleteExitedBefore(context.Context, time.Time) (int64, error) {
	return 0, nil
}

type sessionCommandStateRepo struct{}

func (sessionCommandStateRepo) SaveSnapshot(context.Context, *entity.SessionState) error {
	return nil
}

func (sessionCommandStateRepo) GetSnapshot(context.Context, entity.SessionID) (*entity.SessionState, error) {
	return nil, nil
}

func (sessionCommandStateRepo) DeleteSnapshot(context.Context, entity.SessionID) error {
	return nil
}

func (sessionCommandStateRepo) GetAllSnapshots(context.Context) ([]*entity.SessionState, error) {
	return nil, nil
}

func (sessionCommandStateRepo) GetTotalSnapshotsSize(context.Context) (int64, error) {
	return 0, nil
}

func TestSessionRestoreDeliveryFailureIsPrintedAndReturned(t *testing.T) {
	cfg := config.DefaultConfig()
	ended := time.Now()
	sessions := &sessionCommandSessionRepo{sessions: []*entity.Session{{ID: "saved", Type: entity.SessionTypeBrowser, StartedAt: time.Now(), EndedAt: &ended}}}
	states := repomocks.NewMockSessionStateRepository(t)
	states.EXPECT().GetAllSnapshots(mock.Anything).Return([]*entity.SessionState{}, nil)
	states.EXPECT().GetSnapshot(mock.Anything, entity.SessionID("saved")).Return(&entity.SessionState{Version: entity.SessionStateVersion}, nil)
	launcher := portmocks.NewMockSessionRestoreLauncher(t)
	launcher.EXPECT().RestoreSession(mock.Anything, entity.SessionID("saved")).Return(false, fmt.Errorf("relay unavailable"))
	withApp(t, &cli.App{Config: cfg, Theme: styles.NewTheme(cfg), SessionUC: usecase.NewManageSessionUseCase(sessions, nil), ListSessionsUC: usecase.NewListSessionsUseCase(sessions, states), RestoreUC: usecase.NewRestoreSessionUseCase(states, sessions), SessionRestoreLauncher: launcher})
	old := os.Stderr
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = writer
	t.Cleanup(func() { os.Stderr = old; _ = reader.Close(); _ = writer.Close() })
	err = runSessionsRestore(nil, []string{"saved"})
	require.ErrorContains(t, err, "relay unavailable")
	require.NoError(t, writer.Close())
	output, readErr := io.ReadAll(reader)
	require.NoError(t, readErr)
	require.Contains(t, string(output), "relay unavailable")
	var printed *printedError
	require.ErrorAs(t, err, &printed)
}

func TestSessionRestoreRejectsActiveSourceBeforeColdDestination(t *testing.T) {
	cfg := config.DefaultConfig()
	sessions := &sessionCommandSessionRepo{sessions: []*entity.Session{{ID: "active-source", Type: entity.SessionTypeBrowser, StartedAt: time.Now()}}}
	states := repomocks.NewMockSessionStateRepository(t)
	states.EXPECT().GetAllSnapshots(mock.Anything).Return([]*entity.SessionState{}, nil)
	// The cold-launch-capable service must never be called for an active source.
	launcher := portmocks.NewMockSessionRestoreLauncher(t)
	withApp(t, &cli.App{Config: cfg, Theme: styles.NewTheme(cfg), SessionUC: usecase.NewManageSessionUseCase(sessions, nil), ListSessionsUC: usecase.NewListSessionsUseCase(sessions, states), RestoreUC: usecase.NewRestoreSessionUseCase(states, sessions), SessionRestoreLauncher: launcher})
	discardStderr(t)
	err := runSessionsRestore(nil, []string{"active-source"})
	require.ErrorIs(t, err, usecase.ErrSessionAlreadyOpen)
}
