package ui

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
)

func TestDependenciesValidateAcceptsRuntimeConfigOnly(t *testing.T) {
	runtimeConfig := portmocks.NewMockRuntimeConfigProvider(t)
	engine := portmocks.NewMockEngine(t)
	deps := &Dependencies{
		Ctx:           context.Background(),
		RuntimeConfig: runtimeConfig,
		Engine:        engine,
		HandlerDeps: port.HandlerDeps{
			SaveConfig: func(context.Context, dto.WebUIConfig) error {
				return nil
			},
			SaveOmniboxInitialBehavior: func(context.Context, entity.OmniboxInitialBehavior) error {
				return nil
			},
		},
	}

	if err := deps.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestAppRuntimeConfigReturnsProviderSnapshot(t *testing.T) {
	provider := portmocks.NewMockRuntimeConfigProvider(t)
	provider.EXPECT().
		Current().
		Return(port.RuntimeConfigSnapshot{
			UI: port.RuntimeUIConfig{DefaultUIScale: 1.6},
		}).
		Once()
	app := &App{deps: &Dependencies{RuntimeConfig: provider}}

	got := app.runtimeConfigSnapshot()

	if got.UI.DefaultUIScale != 1.6 {
		t.Fatalf("DefaultUIScale=%v, want 1.6", got.UI.DefaultUIScale)
	}
}

func TestAppUpdateRuntimeConfigStoresLatestSnapshot(t *testing.T) {
	app := &App{}
	app.updateRuntimeConfig(port.RuntimeConfigSnapshot{
		UI: port.RuntimeUIConfig{SidebarWidth: 340},
	})

	got := app.runtimeConfigSnapshot()
	if got.UI.SidebarWidth != 340 {
		t.Fatalf("SidebarWidth=%d, want 340", got.UI.SidebarWidth)
	}
}

func TestAppUpdateRuntimeConfigRefreshesStablePopupConfigPointer(t *testing.T) {
	app := &App{}
	app.updateRuntimeConfig(port.RuntimeConfigSnapshot{
		UI: port.RuntimeUIConfig{
			Workspace: entity.WorkspaceConfig{
				BrowsingContexts: entity.BrowsingContextConfig{OpenInNewPane: true},
			},
		},
	})
	ptr := app.popupBrowsingContextConfig()

	app.updateRuntimeConfig(port.RuntimeConfigSnapshot{
		UI: port.RuntimeUIConfig{
			Workspace: entity.WorkspaceConfig{
				BrowsingContexts: entity.BrowsingContextConfig{OpenInNewPane: false, OAuthAutoClose: true},
			},
		},
	})

	if ptr != app.popupBrowsingContextConfig() {
		t.Fatal("popup config pointer must remain stable for content coordinator")
	}
	if ptr.OpenInNewPane {
		t.Fatal("popup config pointer did not observe updated OpenInNewPane")
	}
	if !ptr.OAuthAutoClose {
		t.Fatal("popup config pointer did not observe updated OAuthAutoClose")
	}
}
