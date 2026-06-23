package ui

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/shared/syncdispatch"
	"github.com/bnema/dumber/internal/ui/coordinator/content"
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

func TestAppRuntimeConfigStateStoresLatestSnapshot(t *testing.T) {
	app := &App{}
	setRuntimeConfigSnapshotForTest(app, port.RuntimeConfigSnapshot{
		UI: port.RuntimeUIConfig{SidebarWidth: 340},
	})

	got := app.runtimeConfigSnapshot()
	if got.UI.SidebarWidth != 340 {
		t.Fatalf("SidebarWidth=%d, want 340", got.UI.SidebarWidth)
	}
}

func TestConfigWatcherOnChangeUpdatesRuntimeConfigState(t *testing.T) {
	ctx := context.Background()
	provider := portmocks.NewMockRuntimeConfigProvider(t)
	provider.EXPECT().Watch().Return(nil).Once()
	var onChange func(port.RuntimeConfigSnapshot)
	provider.EXPECT().
		OnChange(mock.Anything).
		Run(func(fn func(port.RuntimeConfigSnapshot)) {
			onChange = fn
		}).
		Once()

	initial := port.RuntimeConfigSnapshot{
		UI: port.RuntimeUIConfig{
			Workspace: entity.WorkspaceConfig{
				BrowsingContexts: entity.BrowsingContextConfig{OpenInNewPane: true},
			},
		},
	}
	app := &App{
		deps:           &Dependencies{RuntimeConfig: provider},
		runtimeConfig:  runtimeConfigStateFromSnapshotForTest(initial),
		contentCoord:   &content.Coordinator{},
		browserWindows: map[string]*browserWindow{},
		dispatchOnMainThread: func(label string, fn func()) syncdispatch.SyncDispatchResult {
			if label != "ui.runtime_config_reload" {
				t.Fatalf("dispatch label=%q, want ui.runtime_config_reload", label)
			}
			fn()
			return syncdispatch.SyncDispatchResult{Label: label, Status: syncdispatch.SyncDispatchCompleted}
		},
	}
	app.contentCoord.SetPopupConfig(nil, &initial.UI.Workspace.BrowsingContexts, nil)

	app.initConfigWatcher(ctx)
	if onChange == nil {
		t.Fatal("runtime config OnChange callback was not registered")
	}

	onChange(port.RuntimeConfigSnapshot{
		UI: port.RuntimeUIConfig{
			SidebarWidth: 360,
			Workspace: entity.WorkspaceConfig{
				BrowsingContexts: entity.BrowsingContextConfig{OpenInNewPane: false, OAuthAutoClose: true},
			},
		},
	})

	got := app.runtimeConfigSnapshot()
	if got.UI.SidebarWidth != 360 {
		t.Fatalf("SidebarWidth=%d, want 360", got.UI.SidebarWidth)
	}
	if got.UI.Workspace.BrowsingContexts.OpenInNewPane {
		t.Fatal("runtime config state did not observe updated popup OpenInNewPane")
	}
	if !got.UI.Workspace.BrowsingContexts.OAuthAutoClose {
		t.Fatal("runtime config state did not observe updated popup OAuthAutoClose")
	}
}
