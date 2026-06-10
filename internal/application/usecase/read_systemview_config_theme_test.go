package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/domain/entity"
)

type systemviewConfigReaderStub struct {
	current dto.SystemviewConfigPayload
	def     dto.SystemviewConfigPayload
}

func (s systemviewConfigReaderStub) Current(context.Context) (dto.SystemviewConfigPayload, error) {
	return s.current, nil
}

func (s systemviewConfigReaderStub) Default(context.Context) (dto.SystemviewConfigPayload, error) {
	return s.def, nil
}

type systemviewExternalThemeSource struct {
	theme   *entity.ExternalTheme
	enabled bool
}

func (s systemviewExternalThemeSource) Get(context.Context) (*entity.ExternalTheme, error) {
	return s.theme, nil
}

func (s systemviewExternalThemeSource) IsEnabled() bool {
	return s.enabled
}

func TestReadSystemviewConfigUseCase_CurrentResolvedAppearanceUsesPayloadSnapshot(t *testing.T) {
	payload := systemviewThemePayload("prefer-light")
	uc := NewReadSystemviewConfigUseCase(
		systemviewConfigReaderStub{current: payload},
		WithSystemviewResolvedAppearance(nil, nil, func() bool { return true }),
	)

	got, err := uc.Current(context.Background())
	require.NoError(t, err)

	require.NotNil(t, got.ResolvedAppearance)
	require.NotEmpty(t, got.ResolvedAppearance.ColorScheme, "resolved appearance should preserve color scheme metadata")
	require.Equal(t, "prefer-light", got.ResolvedAppearance.ColorScheme)
	require.Equal(t, "#ffffff", got.ResolvedAppearance.LightPalette.Background)
	require.Equal(t, "Inter", got.ResolvedAppearance.SansFont)
	require.Equal(t, 18, got.ResolvedAppearance.DefaultFontSize)
}

func TestReadSystemviewConfigUseCase_CurrentResolvedAppearanceKeepsEditableAppearanceUnchanged(t *testing.T) {
	payload := systemviewThemePayload("prefer-dark")
	external := &entity.ExternalTheme{
		Name:     "Noctalia",
		Provider: "noctalia",
		LightPalette: &entity.ColorPalette{
			Background: "#eeeeee",
			Accent:     "#112233",
		},
		DarkPalette: &entity.ColorPalette{
			Background: "#000000",
			Accent:     "#445566",
		},
	}
	uc := NewReadSystemviewConfigUseCase(
		systemviewConfigReaderStub{current: payload},
		WithSystemviewResolvedAppearance(systemviewExternalThemeSource{theme: external, enabled: true}, nil, nil),
	)

	got, err := uc.Current(context.Background())
	require.NoError(t, err)

	require.Equal(t, "#111111", got.Appearance.DarkPalette.Background)
	require.Equal(t, "#66aaff", got.Appearance.DarkPalette.Accent)
	require.NotNil(t, got.ResolvedAppearance)
	require.Equal(t, "#000000", got.ResolvedAppearance.DarkPalette.Background)
	require.Equal(t, "#445566", got.ResolvedAppearance.DarkPalette.Accent)
	require.Equal(t, got.Appearance.ExternalTheme, got.ResolvedAppearance.ExternalTheme)
}

func TestReadSystemviewConfigUseCase_DefaultDoesNotOverlayResolvedAppearance(t *testing.T) {
	payload := systemviewThemePayload("prefer-dark")
	uc := NewReadSystemviewConfigUseCase(
		systemviewConfigReaderStub{def: payload},
		WithSystemviewResolvedAppearance(systemviewExternalThemeSource{theme: validExternalTheme(), enabled: true}, nil, nil),
	)

	got, err := uc.Default(context.Background())
	require.NoError(t, err)
	require.Nil(t, got.ResolvedAppearance)
}

func systemviewThemePayload(colorScheme string) dto.SystemviewConfigPayload {
	return dto.SystemviewConfigPayload{
		DefaultUIScale: 1.25,
		Appearance: dto.WebUIAppearanceConfig{
			ColorScheme:     colorScheme,
			SansFont:        "Inter",
			SerifFont:       "Georgia",
			MonospaceFont:   "JetBrains Mono",
			GtkFont:         "Adwaita Sans",
			DefaultFontSize: 18,
			LightPalette: dto.ColorPalette{
				Background:     "#ffffff",
				Surface:        "#f8f8f8",
				SurfaceVariant: "#eeeeee",
				Text:           "#111111",
				Muted:          "#666666",
				Accent:         "#0055ff",
				Border:         "#dddddd",
			},
			DarkPalette: dto.ColorPalette{
				Background:     "#111111",
				Surface:        "#1a1a1a",
				SurfaceVariant: "#2a2a2a",
				Text:           "#f5f5f5",
				Muted:          "#a0a0a0",
				Accent:         "#66aaff",
				Border:         "#333333",
			},
		},
	}
}
