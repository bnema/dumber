package usecase_test

import (
	"context"
	"math"
	"testing"

	"github.com/bnema/dumber/internal/application/dto"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSaveWebUIConfigUseCase_Validation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*dto.WebUIConfig)
		wantErr string
	}{
		{
			name: "rejects invalid default search URL",
			mutate: func(cfg *dto.WebUIConfig) {
				cfg.DefaultSearchEngine = "not-a-url-%s"
			},
			wantErr: "default_search_engine",
		},
		{
			name: "rejects invalid performance profile",
			mutate: func(cfg *dto.WebUIConfig) {
				cfg.Performance.Profile = "turbo"
			},
			wantErr: "performance.profile",
		},
		{
			name: "rejects NaN UI scale",
			mutate: func(cfg *dto.WebUIConfig) {
				cfg.DefaultUIScale = math.NaN()
			},
			wantErr: "default_ui_scale must be a finite value",
		},
		{
			name: "rejects infinite UI scale",
			mutate: func(cfg *dto.WebUIConfig) {
				cfg.DefaultUIScale = math.Inf(1)
			},
			wantErr: "default_ui_scale must be a finite value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validWebUIConfig()
			tt.mutate(&cfg)
			saver := portmocks.NewMockWebUIConfigSaver(t)
			uc := usecase.NewSaveWebUIConfigUseCase(saver)

			err := uc.Execute(context.Background(), cfg)

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestSaveWebUIConfigUseCase_NormalizesAndSavesValidConfig(t *testing.T) {
	saver := portmocks.NewMockWebUIConfigSaver(t)
	var saved dto.WebUIConfig
	saver.EXPECT().SaveWebUIConfig(mock.Anything, mock.AnythingOfType("dto.WebUIConfig")).Run(func(_ context.Context, cfg dto.WebUIConfig) {
		saved = cfg
	}).Return(nil).Once()
	uc := usecase.NewSaveWebUIConfigUseCase(saver)
	cfg := validWebUIConfig()
	cfg.SearchShortcuts[" ddg "] = dto.SearchShortcut{URL: " https://duckduckgo.com/?q=%s ", Description: " DuckDuckGo "}
	cfg.SearchShortcuts[" "] = dto.SearchShortcut{URL: "https://example.com?q=%s", Description: "Example"}

	err := uc.Execute(context.Background(), cfg)

	require.NoError(t, err)
	require.NotContains(t, saved.SearchShortcuts, " ddg ")
	require.NotContains(t, saved.SearchShortcuts, "")
	require.Contains(t, saved.SearchShortcuts, "ddg")
	require.Equal(t, "https://duckduckgo.com/?q=%s", saved.SearchShortcuts["ddg"].URL)
	require.Equal(t, "DuckDuckGo", saved.SearchShortcuts["ddg"].Description)
}

func validWebUIConfig() dto.WebUIConfig {
	palette := dto.ColorPalette{
		Background:     "#ffffff",
		Surface:        "#f8f8f8",
		SurfaceVariant: "#eeeeee",
		Text:           "#111111",
		Muted:          "#666666",
		Accent:         "#0055ff",
		Border:         "#dddddd",
	}
	return dto.WebUIConfig{
		Appearance: dto.WebUIAppearanceConfig{
			SansFont:        "Inter",
			SerifFont:       "Georgia",
			MonospaceFont:   "JetBrains Mono",
			DefaultFontSize: 16,
			ColorScheme:     "prefer-dark",
			LightPalette:    palette,
			DarkPalette:     palette,
		},
		Performance: dto.WebUIPerformanceConfig{
			Profile: "default",
		},
		DefaultUIScale:      1,
		DefaultSearchEngine: "https://duckduckgo.com/?q=%s",
		SearchShortcuts:     map[string]dto.SearchShortcut{},
	}
}
