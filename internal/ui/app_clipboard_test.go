package ui

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
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
		Return(port.RuntimeConfigSnapshot{}).
		Once()

	deps := &Dependencies{
		Ctx:           ctx,
		Config:        &config.Config{},
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
		Return(port.RuntimeConfigSnapshot{}).
		Once()

	deps := &Dependencies{
		Ctx:           ctx,
		Config:        &config.Config{},
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
