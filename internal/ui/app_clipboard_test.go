package ui

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/stretchr/testify/require"
)

type fakeClipboardEngine struct {
	registerDeps port.HandlerDependencies
}

var _ port.Engine = (*fakeClipboardEngine)(nil)

func (f *fakeClipboardEngine) Factory() port.WebViewFactory          { return nil }
func (f *fakeClipboardEngine) Pool() port.WebViewPool                { return nil }
func (f *fakeClipboardEngine) ContentInjector() port.ContentInjector { return nil }
func (f *fakeClipboardEngine) SettingsApplier() port.SettingsApplier { return nil }
func (f *fakeClipboardEngine) FilterApplier() port.FilterApplier     { return nil }
func (f *fakeClipboardEngine) FaviconDatabase() port.FaviconDatabase { return nil }
func (f *fakeClipboardEngine) InternalSchemePath() string            { return "" }
func (f *fakeClipboardEngine) Close() error                          { return nil }
func (f *fakeClipboardEngine) RegisterHandlers(_ context.Context, deps port.HandlerDependencies) error {
	f.registerDeps = deps
	return nil
}
func (f *fakeClipboardEngine) RegisterAccentHandlers(context.Context, port.AccentKeyHandler) error {
	return nil
}
func (f *fakeClipboardEngine) ConfigureDownloads(context.Context, string, port.DownloadEventHandler, port.DownloadPreparer) error {
	return nil
}
func (f *fakeClipboardEngine) OnToolkitReady(context.Context) error { return nil }
func (f *fakeClipboardEngine) UpdateAppearance(context.Context, float64, float64, float64, float64) error {
	return nil
}
func (f *fakeClipboardEngine) UpdateSettings(context.Context, port.EngineSettingsUpdate) error {
	return nil
}
func (f *fakeClipboardEngine) SetHandlerContext(context.Context) {}

func TestNew_DoesNotWireClipboardOrchestratorWhenClipboardMissing(t *testing.T) {
	ctx := context.Background()
	engine := &fakeClipboardEngine{}

	deps := &Dependencies{
		Ctx:    ctx,
		Config: &config.Config{},
		Engine: engine,
		HandlerDeps: port.HandlerDeps{
			SaveConfig:                 func(context.Context, port.WebUIConfig) error { return nil },
			SaveOmniboxInitialBehavior: func(context.Context, entity.OmniboxInitialBehavior) error { return nil },
		},
		Clipboard: nil,
	}

	app, err := New(deps)
	require.NoError(t, err)
	require.NotNil(t, app)
	require.Nil(t, engine.registerDeps.ClipboardTextOrchestrator)
}
