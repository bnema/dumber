package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/bnema/dumber/internal/logging"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func testContext() context.Context {
	logger := logging.NewFromConfigValues("debug", "console")
	return logging.WithContext(context.Background(), logger)
}

func TestManageSessionUseCase_StartSession_SavesAndReturnsLogger(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)
	loggerPort := portmocks.NewMockSessionLogger(t)

	cfg := port.SessionLogConfig{Level: "info", Format: "json", TimeFormat: "15:04:05", EnableFileLog: false}
	returnedLogger := zerolog.New(zerolog.NewConsoleWriter())

	loggerPort.EXPECT().CreateLogger(mock.Anything, mock.Anything, cfg).
		Return(returnedLogger, func() {}, nil)

	sessionRepo.EXPECT().Save(mock.Anything, mock.AnythingOfType("*entity.Session")).
		Run(func(_ context.Context, s *entity.Session) {
			require.Equal(t, entity.SessionTypeBrowser, s.Type)
			require.NotEmpty(t, s.ID)
			require.False(t, s.StartedAt.IsZero())
		}).
		Return(nil)

	uc := usecase.NewManageSessionUseCase(sessionRepo, loggerPort)

	out, err := uc.StartSession(ctx, usecase.StartSessionInput{Type: entity.SessionTypeBrowser, Now: time.Now(), LogConfig: cfg})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.NotNil(t, out.Session)
	assert.Equal(t, returnedLogger, out.Logger)
}

func TestManageSessionUseCase_StartSession_WhenSaveFails_CleansUp(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)
	loggerPort := portmocks.NewMockSessionLogger(t)

	cfg := port.SessionLogConfig{Level: "info", Format: "json", EnableFileLog: false}
	returnedLogger := zerolog.New(zerolog.NewConsoleWriter())

	cleanupCalled := false
	cleanupFn := func() { cleanupCalled = true }

	loggerPort.EXPECT().CreateLogger(mock.Anything, mock.Anything, cfg).
		Return(returnedLogger, cleanupFn, nil)

	sessionRepo.EXPECT().Save(mock.Anything, mock.AnythingOfType("*entity.Session")).
		Return(errors.New("db down"))

	uc := usecase.NewManageSessionUseCase(sessionRepo, loggerPort)

	_, err := uc.StartSession(ctx, usecase.StartSessionInput{Type: entity.SessionTypeBrowser, Now: time.Now(), LogConfig: cfg})
	require.Error(t, err)
	assert.True(t, cleanupCalled)
}
