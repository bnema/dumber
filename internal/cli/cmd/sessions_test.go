package cmd

import (
	"io"
	"os"
	"testing"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/cli"
	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/infrastructure/config"
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
