package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/shared/syncdispatch"
	uitheme "github.com/bnema/dumber/internal/ui/theme"
)

type recordingExternalThemeWatcher struct {
	starts  int
	stops   int
	cfg     entity.ExternalThemeConfig
	trigger func()
}

func (w *recordingExternalThemeWatcher) Start(_ context.Context, cfg entity.ExternalThemeConfig, onChange func()) error {
	w.starts++
	w.cfg = cfg
	w.trigger = onChange
	return nil
}

func (w *recordingExternalThemeWatcher) Stop() error {
	w.stops++
	return nil
}

type mutableExternalThemeSource struct {
	cfg   entity.ExternalThemeConfig
	theme *entity.ExternalTheme
	err   error
}

func (s *mutableExternalThemeSource) Configure(cfg entity.ExternalThemeConfig) {
	s.cfg = cfg
}

func (s *mutableExternalThemeSource) ExternalThemeIdentity() string {
	if !s.cfg.Enabled {
		return ""
	}
	return s.cfg.Provider + "|" + s.cfg.Format + "|" + s.cfg.Path
}

func (s *mutableExternalThemeSource) IsEnabled() bool {
	return s.cfg.Enabled
}

func (s *mutableExternalThemeSource) Get(context.Context) (*entity.ExternalTheme, error) {
	if !s.IsEnabled() {
		return nil, nil
	}
	return s.theme, s.err
}

func immediateDispatchForExternalThemeTest(label string, fn func()) syncdispatch.SyncDispatchResult {
	fn()
	return syncdispatch.SyncDispatchResult{Label: label, Status: syncdispatch.SyncDispatchCompleted}
}

func TestExternalThemeWatcherCallbackAppliesThemeThroughSharedPath(t *testing.T) {
	ctx := context.Background()
	cfg := config.DefaultConfig()
	cfg.Appearance.ColorScheme = "prefer-dark"
	cfg.Appearance.ExternalTheme = entity.ExternalThemeConfig{
		Enabled:  true,
		Provider: "noctalia",
		Format:   "dumber-json",
		Path:     "/tmp/theme.json",
	}
	source := &mutableExternalThemeSource{theme: externalThemeWithDarkBackground("#000000")}
	watcher := &recordingExternalThemeWatcher{}
	manager := uitheme.NewManager(ctx, resolvedThemeForUITest(cfg.Appearance.LightPalette, cfg.Appearance.DarkPalette))
	engine := portmocks.NewMockEngine(t)
	var backgroundUpdated bool
	engine.EXPECT().UpdateSettings(mock.Anything, mock.Anything).Return(nil).Once()
	engine.EXPECT().UpdateAppearance(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ context.Context, r, g, b, alpha float64) {
			backgroundUpdated = true
			require.InDelta(t, 0.0, r, 0.000001)
			require.InDelta(t, 0.0, g, 0.000001)
			require.InDelta(t, 0.0, b, 0.000001)
			require.InDelta(t, 1.0, alpha, 0.000001)
		}).
		Return(nil).
		Once()
	engine.EXPECT().ContentInjector().Return(nil).Once()
	app := &App{
		deps: &Dependencies{
			Config:               cfg,
			Theme:                manager,
			ResolveThemeUC:       usecase.NewResolveThemeUseCase(source),
			ExternalThemeSource:  source,
			ExternalThemeWatcher: watcher,
		},
		dispatchOnMainThread: immediateDispatchForExternalThemeTest,
		engine:               engine,
	}

	app.syncExternalThemeWatcher(ctx)
	require.Equal(t, 1, watcher.starts)
	require.Equal(t, cfg.Appearance.ExternalTheme, watcher.cfg)
	require.NotNil(t, watcher.trigger)

	watcher.trigger()

	require.Equal(t, "#000000", app.deps.Theme.GetCurrentPalette().Background)
	require.True(t, backgroundUpdated, "theme reload should refresh engine background color")
}

func TestExternalThemeReloadKeepsLastGoodAndDisablingClearsIt(t *testing.T) {
	ctx := context.Background()
	cfg := config.DefaultConfig()
	cfg.Appearance.ColorScheme = "prefer-dark"
	cfg.Appearance.ExternalTheme = entity.ExternalThemeConfig{
		Enabled:  true,
		Provider: "noctalia",
		Format:   "dumber-json",
		Path:     "/tmp/theme.json",
	}
	source := &mutableExternalThemeSource{theme: externalThemeWithDarkBackground("#000000")}
	manager := uitheme.NewManager(ctx, resolvedThemeForUITest(cfg.Appearance.LightPalette, cfg.Appearance.DarkPalette))
	app := &App{
		deps: &Dependencies{
			Config:              cfg,
			Theme:               manager,
			ResolveThemeUC:      usecase.NewResolveThemeUseCase(source),
			ExternalThemeSource: source,
		},
		dispatchOnMainThread: immediateDispatchForExternalThemeTest,
	}

	app.applyAppearanceConfig(ctx)
	require.Equal(t, "#000000", app.deps.Theme.GetCurrentPalette().Background)

	source.theme = &entity.ExternalTheme{
		Name:         "broken",
		Provider:     "noctalia",
		LightPalette: &entity.ColorPalette{Accent: "not-a-hex"},
		DarkPalette:  &entity.ColorPalette{Background: "also-bad"},
	}
	app.applyAppearanceConfig(ctx)
	require.Equal(t, "#000000", app.deps.Theme.GetCurrentPalette().Background)

	cfg.Appearance.ExternalTheme.Enabled = false
	app.applyAppearanceConfig(ctx)
	require.Equal(t, cfg.Appearance.DarkPalette.Background, app.deps.Theme.GetCurrentPalette().Background)
}

func TestExternalThemeReloadKeepsLastGoodOnReadError(t *testing.T) {
	ctx := context.Background()
	cfg := config.DefaultConfig()
	cfg.Appearance.ColorScheme = "prefer-dark"
	cfg.Appearance.ExternalTheme = entity.ExternalThemeConfig{
		Enabled:  true,
		Provider: "noctalia",
		Format:   "dumber-json",
		Path:     "/tmp/theme.json",
	}
	source := &mutableExternalThemeSource{theme: externalThemeWithDarkBackground("#000000")}
	manager := uitheme.NewManager(ctx, resolvedThemeForUITest(cfg.Appearance.LightPalette, cfg.Appearance.DarkPalette))
	app := &App{
		deps: &Dependencies{
			Config:              cfg,
			Theme:               manager,
			ResolveThemeUC:      usecase.NewResolveThemeUseCase(source),
			ExternalThemeSource: source,
		},
		dispatchOnMainThread: immediateDispatchForExternalThemeTest,
	}

	app.applyAppearanceConfig(ctx)
	require.Equal(t, "#000000", app.deps.Theme.GetCurrentPalette().Background)

	source.theme = nil
	source.err = errors.New("temporary read failure")
	app.applyAppearanceConfig(ctx)
	require.Equal(t, "#000000", app.deps.Theme.GetCurrentPalette().Background)
}

func TestExternalThemeReloadRecoversAfterMalformedUpdate(t *testing.T) {
	ctx := context.Background()
	cfg := config.DefaultConfig()
	cfg.Appearance.ColorScheme = "prefer-dark"
	cfg.Appearance.ExternalTheme = entity.ExternalThemeConfig{
		Enabled:  true,
		Provider: "noctalia",
		Format:   "dumber-json",
		Path:     "/tmp/theme.json",
	}
	source := &mutableExternalThemeSource{theme: externalThemeWithDarkBackground("#000000")}
	manager := uitheme.NewManager(ctx, resolvedThemeForUITest(cfg.Appearance.LightPalette, cfg.Appearance.DarkPalette))
	app := &App{
		deps: &Dependencies{
			Config:              cfg,
			Theme:               manager,
			ResolveThemeUC:      usecase.NewResolveThemeUseCase(source),
			ExternalThemeSource: source,
		},
		dispatchOnMainThread: immediateDispatchForExternalThemeTest,
	}

	app.applyAppearanceConfig(ctx)
	require.Equal(t, "#000000", app.deps.Theme.GetCurrentPalette().Background)

	source.theme = &entity.ExternalTheme{
		Name:         "broken",
		Provider:     "noctalia",
		LightPalette: &entity.ColorPalette{Accent: "not-a-hex"},
		DarkPalette:  &entity.ColorPalette{Background: "also-bad"},
	}
	app.applyAppearanceConfig(ctx)
	require.Equal(t, "#000000", app.deps.Theme.GetCurrentPalette().Background)

	source.theme = externalThemeWithDarkBackground("#123456")
	app.applyAppearanceConfig(ctx)
	require.Equal(t, "#123456", app.deps.Theme.GetCurrentPalette().Background)
}

func TestOnShutdownStopsExternalThemeWatcher(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	watcher := &recordingExternalThemeWatcher{}
	app := &App{
		deps:   &Dependencies{ExternalThemeWatcher: watcher},
		cancel: cancel,
	}

	app.onShutdown(ctx)

	require.Equal(t, 1, watcher.stops)
}

func resolvedThemeForUITest(light, dark entity.ColorPalette) entity.ResolvedTheme {
	return entity.ResolvedTheme{
		LightPalette:      light,
		DarkPalette:       dark,
		ActivePalette:     dark,
		PrefersDark:       true,
		ColorSchemeSource: "test",
		ThemeSource:       entity.ThemeSourceMetadata{Kind: entity.ThemeSourceConfig},
		Fonts:             entity.DefaultThemeFonts(),
		UIScale:           1,
		ModeColors:        entity.DefaultThemeModeColors(),
	}
}

func externalThemeWithDarkBackground(background string) *entity.ExternalTheme {
	return &entity.ExternalTheme{
		Name:     "Noctalia",
		Provider: "noctalia",
		LightPalette: &entity.ColorPalette{
			Background: "#eeeeee",
			Accent:     "#112233",
		},
		DarkPalette: &entity.ColorPalette{
			Background: background,
			Accent:     "#445566",
		},
	}
}
