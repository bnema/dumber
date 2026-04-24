package usecase_test

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/stretchr/testify/require"
)

func TestSaveWebUIConfigUseCase_Validation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*port.WebUIConfig)
		wantErr string
	}{
		{
			name: "rejects invalid default search URL",
			mutate: func(cfg *port.WebUIConfig) {
				cfg.DefaultSearchEngine = "not-a-url-%s"
			},
			wantErr: "default_search_engine",
		},
		{
			name: "rejects blank search shortcut key",
			mutate: func(cfg *port.WebUIConfig) {
				cfg.SearchShortcuts[" "] = port.SearchShortcut{URL: "https://example.com?q=%s", Description: "Example"}
			},
			wantErr: "shortcut key cannot be empty",
		},
		{
			name: "rejects invalid performance profile",
			mutate: func(cfg *port.WebUIConfig) {
				cfg.Performance.Profile = "turbo"
			},
			wantErr: "performance.profile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validWebUIConfig()
			tt.mutate(&cfg)
			uc := usecase.NewSaveWebUIConfigUseCase(fakeWebUIConfigSaver{})

			err := uc.Execute(context.Background(), cfg)

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestSaveWebUIConfigUseCase_NormalizesAndSavesValidConfig(t *testing.T) {
	saver := &capturingWebUIConfigSaver{}
	uc := usecase.NewSaveWebUIConfigUseCase(saver)
	cfg := validWebUIConfig()
	cfg.SearchShortcuts[" ddg "] = port.SearchShortcut{URL: " https://duckduckgo.com/?q=%s ", Description: " DuckDuckGo "}

	err := uc.Execute(context.Background(), cfg)

	require.NoError(t, err)
	require.Contains(t, saver.saved.SearchShortcuts, "ddg")
	require.Equal(t, "https://duckduckgo.com/?q=%s", saver.saved.SearchShortcuts["ddg"].URL)
	require.Equal(t, "DuckDuckGo", saver.saved.SearchShortcuts["ddg"].Description)
}

func validWebUIConfig() port.WebUIConfig {
	palette := port.ColorPalette{
		Background:     "#ffffff",
		Surface:        "#f8f8f8",
		SurfaceVariant: "#eeeeee",
		Text:           "#111111",
		Muted:          "#666666",
		Accent:         "#0055ff",
		Border:         "#dddddd",
	}
	return port.WebUIConfig{
		Appearance: port.WebUIAppearanceConfig{
			SansFont:        "Inter",
			SerifFont:       "Georgia",
			MonospaceFont:   "JetBrains Mono",
			DefaultFontSize: 16,
			ColorScheme:     "prefer-dark",
			LightPalette:    palette,
			DarkPalette:     palette,
		},
		Performance: port.WebUIPerformanceConfig{
			Profile: "default",
		},
		DefaultUIScale:      1,
		DefaultSearchEngine: "https://duckduckgo.com/?q=%s",
		SearchShortcuts:     map[string]port.SearchShortcut{},
	}
}

// Handwritten fake to preserve stateful save assertions without mock generation.
type fakeWebUIConfigSaver struct{}

func (fakeWebUIConfigSaver) SaveWebUIConfig(context.Context, port.WebUIConfig) error {
	return nil
}

// Handwritten fake to capture the saved config state for assertions.
type capturingWebUIConfigSaver struct {
	saved port.WebUIConfig
}

func (s *capturingWebUIConfigSaver) SaveWebUIConfig(_ context.Context, cfg port.WebUIConfig) error {
	s.saved = cfg
	return nil
}
