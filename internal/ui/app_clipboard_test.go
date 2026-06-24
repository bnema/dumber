package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNew_DoesNotWireClipboardOrchestratorWhenClipboardMissing(t *testing.T) {
	ctx := context.Background()
	engine := portmocks.NewMockEngine(t)
	var registerDeps port.HandlerDependencies
	engine.EXPECT().RegisterHandlers(mock.Anything, mock.Anything).
		Run(func(_ context.Context, deps port.HandlerDependencies) {
			registerDeps = deps
		}).
		Return(nil).
		Once()
	runtimeConfig := portmocks.NewMockRuntimeConfigProvider(t)
	runtimeConfig.EXPECT().
		Current().
		Return(entity.RuntimeConfigSnapshot{}).
		Once()

	deps := &Dependencies{
		Ctx:           ctx,
		RuntimeConfig: runtimeConfig,
		Engine:        engine,
		HandlerDeps: port.HandlerDeps{
			SaveConfig:                 func(context.Context, dto.WebUIConfig) error { return nil },
			SaveOmniboxInitialBehavior: func(context.Context, entity.OmniboxInitialBehavior) error { return nil },
		},
		Clipboard: nil,
	}

	app, err := New(deps)
	require.NoError(t, err)
	require.NotNil(t, app)
	require.Nil(t, registerDeps.ClipboardTextOrchestrator)
}

func TestNew_ClosesHistoryRecorderWhenRegisterHandlersFails(t *testing.T) {
	ctx := context.Background()
	engine := portmocks.NewMockEngine(t)
	registerErr := errors.New("register failed")
	engine.EXPECT().RegisterHandlers(mock.Anything, mock.Anything).Return(registerErr).Once()
	runtimeConfig := portmocks.NewMockRuntimeConfigProvider(t)
	runtimeConfig.EXPECT().Current().Return(entity.RuntimeConfigSnapshot{}).Once()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRecorder := usecase.NewHistoryRecorderUseCase(historyRepo, nil)

	deps := &Dependencies{
		Ctx:               ctx,
		RuntimeConfig:     runtimeConfig,
		Engine:            engine,
		HistoryRecorderUC: historyRecorder,
		HandlerDeps: port.HandlerDeps{
			SaveConfig:                 func(context.Context, dto.WebUIConfig) error { return nil },
			SaveOmniboxInitialBehavior: func(context.Context, entity.OmniboxInitialBehavior) error { return nil },
		},
	}

	app, err := New(deps)
	require.Nil(t, app)
	require.ErrorIs(t, err, registerErr)

	historyRecorder.RecordHistory(ctx, "pane-1", "https://example.com/leak-check")
	historyRecorder.Close()
}

func TestNew_WiresClipboardOrchestratorWhenClipboardPresent(t *testing.T) {
	ctx := context.Background()
	engine := portmocks.NewMockEngine(t)
	clipboard := portmocks.NewMockClipboard(t)
	var registerDeps port.HandlerDependencies
	engine.EXPECT().RegisterHandlers(mock.Anything, mock.Anything).
		Run(func(_ context.Context, deps port.HandlerDependencies) {
			registerDeps = deps
		}).
		Return(nil).
		Once()
	runtimeConfig := portmocks.NewMockRuntimeConfigProvider(t)
	runtimeConfig.EXPECT().
		Current().
		Return(entity.RuntimeConfigSnapshot{}).
		Once()

	deps := &Dependencies{
		Ctx:           ctx,
		RuntimeConfig: runtimeConfig,
		Engine:        engine,
		HandlerDeps: port.HandlerDeps{
			SaveConfig:                 func(context.Context, dto.WebUIConfig) error { return nil },
			SaveOmniboxInitialBehavior: func(context.Context, entity.OmniboxInitialBehavior) error { return nil },
		},
		Clipboard: clipboard,
	}

	app, err := New(deps)
	require.NoError(t, err)
	require.NotNil(t, app)
	require.NotNil(t, registerDeps.ClipboardTextOrchestrator)
}
