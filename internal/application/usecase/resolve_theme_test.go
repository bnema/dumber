package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
)

// assertNoWarnings fails the test if there are any warnings.
func assertNoWarnings(t *testing.T, warnings []entity.ThemeWarning) {
	t.Helper()
	if len(warnings) > 0 {
		var msgs []string
		for _, w := range warnings {
			msgs = append(msgs, fmt.Sprintf("%s: %s", w.Field, w.Message))
		}
		t.Errorf("unexpected warnings: %s", strings.Join(msgs, "; "))
	}
}

// assertValidPaletteHex checks every color field in a palette is valid CSS-safe hex.
func assertValidPaletteHex(t *testing.T, label string, p entity.ResolvedThemePalette) {
	t.Helper()
	fields := map[string]string{
		"Background":     p.Background,
		"Surface":        p.Surface,
		"SurfaceVariant": p.SurfaceVariant,
		"Text":           p.Text,
		"Muted":          p.Muted,
		"Accent":         p.Accent,
		"Border":         p.Border,
	}
	for name, value := range fields {
		if !entity.IsValidHex(value) {
			t.Errorf("%s.%s = %q is not a valid CSS-safe hex color", label, name, value)
		}
	}
}

// newMockExternalSource creates a test double for the external theme source port.
type newMockExternalSource struct {
	theme    *entity.ExternalTheme
	err      error
	enabled  bool
	identity string
}

func (m *newMockExternalSource) Get(_ context.Context) (*entity.ExternalTheme, error) {
	return m.theme, m.err
}

func (m *newMockExternalSource) IsEnabled() bool {
	return m.enabled
}

func (m *newMockExternalSource) ExternalThemeIdentity() string {
	return m.identity
}

// configLightPalette returns a recognizable light palette for config.
func configLightPalette() *entity.ColorPalette {
	return &entity.ColorPalette{
		Background:     "#fafafa",
		Surface:        "#ffffff",
		SurfaceVariant: "#f0f0f0",
		Text:           "#1a1a1a",
		Muted:          "#666666",
		Accent:         "#22c55e",
		Border:         "#dddddd",
	}
}

// configDarkPalette returns a recognizable dark palette for config.
func configDarkPalette() *entity.ColorPalette {
	return &entity.ColorPalette{
		Background:     "#0a0a0b",
		Surface:        "#1a1a1b",
		SurfaceVariant: "#2d2d2d",
		Text:           "#ffffff",
		Muted:          "#909090",
		Accent:         "#4ade80",
		Border:         "#333333",
	}
}

// externalLightPalette returns a different light palette for external testing.
func externalLightPalette() *entity.ColorPalette {
	return &entity.ColorPalette{
		Background:     "#fff8e7",
		Surface:        "#fffef5",
		SurfaceVariant: "#f5ecd7",
		Text:           "#2d2a1e",
		Muted:          "#8c876e",
		Accent:         "#d4a017",
		Border:         "#e0d8b0",
	}
}

// externalDarkPalette returns a different dark palette for external testing.
func externalDarkPalette() *entity.ColorPalette {
	return &entity.ColorPalette{
		Background:     "#1a1a2e",
		Surface:        "#16213e",
		SurfaceVariant: "#0f3460",
		Text:           "#e0e0e0",
		Muted:          "#a0a0a0",
		Accent:         "#e94560",
		Border:         "#533483",
	}
}

// validExternalTheme returns a fully valid ExternalTheme for testing.
func validExternalTheme() *entity.ExternalTheme {
	return &entity.ExternalTheme{
		Name:         "Noctalia Test",
		Provider:     "noctalia",
		LightPalette: externalLightPalette(),
		DarkPalette:  externalDarkPalette(),
	}
}

// ============================================================
// Precedence: External disabled → config/default palettes
// ============================================================

// TestResolveTheme_ExternalDisabled_UsesConfigPalettes verifies that when the external
// source is disabled, config palettes (or defaults) are used.
func TestResolveTheme_ExternalDisabled_UsesConfigPalettes(t *testing.T) {
	uc := NewResolveThemeUseCase(nil)

	light := configLightPalette()
	dark := configDarkPalette()

	input := ResolveThemeInput{
		ColorScheme:       "default",
		PrefersDark:       true,
		ColorSchemeSource: "adwaita",
		LightPalette:      light,
		DarkPalette:       dark,
		UIScale:           1.0,
	}

	output, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertValidPaletteHex(t, "LightPalette", output.Theme.LightPalette)
	assertValidPaletteHex(t, "DarkPalette", output.Theme.DarkPalette)
	assertValidPaletteHex(t, "ActivePalette", output.Theme.ActivePalette)

	// With PrefersDark=true and no external, light palette should be config
	if output.Theme.LightPalette.Background != light.Background {
		t.Errorf("expected LightPalette.Background=%q, got %q", light.Background, output.Theme.LightPalette.Background)
	}
	// Active palette should be dark (PrefersDark=true)
	if output.Theme.ActivePalette.Background != dark.Background {
		t.Errorf("expected ActivePalette.Background=%q (dark), got %q", dark.Background, output.Theme.ActivePalette.Background)
	}
}

// TestResolveTheme_NoConfig_UsesDefaults verifies that when neither config nor external
// provide palettes, sensible defaults are returned.
func TestResolveTheme_NoConfig_UsesDefaults(t *testing.T) {
	uc := NewResolveThemeUseCase(nil)

	input := ResolveThemeInput{
		ColorScheme:       "default",
		PrefersDark:       true,
		ColorSchemeSource: "fallback",
		UIScale:           1.0,
		// LightPalette and DarkPalette are nil (no config)
	}

	output, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertValidPaletteHex(t, "LightPalette", output.Theme.LightPalette)
	assertValidPaletteHex(t, "DarkPalette", output.Theme.DarkPalette)
	assertValidPaletteHex(t, "ActivePalette", output.Theme.ActivePalette)

	// Defaults must be non-trivial (Background, Text non-empty)
	if output.Theme.DarkPalette.Background == "" {
		t.Error("expected non-empty DarkPalette.Background from defaults")
	}
	if output.Theme.DarkPalette.Text == "" {
		t.Error("expected non-empty DarkPalette.Text from defaults")
	}
	if output.Theme.LightPalette.Background == "" {
		t.Error("expected non-empty LightPalette.Background from defaults")
	}
}

// ============================================================
// Precedence: Valid external → external palettes override palette colors only
// ============================================================

// TestResolveTheme_ValidExternal_OverridesPaletteColors verifies that when a valid
// external theme is available, its palette colors override config palette colors,
// but other fields (fonts, mode colors, scale) come from config or defaults.
func TestResolveTheme_ValidExternal_OverridesPaletteColors(t *testing.T) {
	ext := validExternalTheme()
	mock := &newMockExternalSource{
		theme:   ext,
		enabled: true,
	}
	uc := NewResolveThemeUseCase(mock)

	input := ResolveThemeInput{
		ColorScheme:       "default",
		PrefersDark:       true,
		ColorSchemeSource: "adwaita",
		LightPalette:      configLightPalette(),
		DarkPalette:       configDarkPalette(),
		Fonts: &entity.ThemeFonts{
			SansFont: "config-sans",
		},
		UIScale: 1.5,
	}

	output, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertValidPaletteHex(t, "LightPalette", output.Theme.LightPalette)
	assertValidPaletteHex(t, "DarkPalette", output.Theme.DarkPalette)
	assertValidPaletteHex(t, "ActivePalette", output.Theme.ActivePalette)

	// External palette should override config palette
	if output.Theme.DarkPalette.Background != ext.DarkPalette.Background {
		t.Errorf("expected DarkPalette.Background from external (%q), got %q",
			ext.DarkPalette.Background, output.Theme.DarkPalette.Background)
	}
	if output.Theme.DarkPalette.Accent != ext.DarkPalette.Accent {
		t.Errorf("expected DarkPalette.Accent from external (%q), got %q",
			ext.DarkPalette.Accent, output.Theme.DarkPalette.Accent)
	}

	// ThemeSource should indicate external
	if output.Theme.ThemeSource.Kind != entity.ThemeSourceExternal {
		t.Errorf("expected ThemeSource.Kind=external, got %q", output.Theme.ThemeSource.Kind)
	}
	if output.Theme.ThemeSource.Provider != "noctalia" {
		t.Errorf("expected ThemeSource.Provider=noctalia, got %q", output.Theme.ThemeSource.Provider)
	}
	if output.Theme.ThemeSource.LastGood {
		t.Error("expected ThemeSource.LastGood=false for fresh valid external theme")
	}

	// ColorSchemeSource should still reflect the detector
	if output.Theme.ColorSchemeSource != "adwaita" {
		t.Errorf("expected ColorSchemeSource='adwaita', got %q", output.Theme.ColorSchemeSource)
	}
}

// TestResolveTheme_ValidExternal_DoesNotOverrideNonPaletteFields verifies that
// external theme only overrides palette colors, not fonts, scale, or mode colors.
func TestResolveTheme_ValidExternal_DoesNotOverrideNonPaletteFields(t *testing.T) {
	ext := validExternalTheme()
	mock := &newMockExternalSource{
		theme:   ext,
		enabled: true,
	}
	uc := NewResolveThemeUseCase(mock)

	input := ResolveThemeInput{
		ColorScheme:       "default",
		PrefersDark:       false,
		ColorSchemeSource: "env",
		Fonts: &entity.ThemeFonts{
			SansFont:      "MyConfigSans",
			SerifFont:     "MyConfigSerif",
			MonospaceFont: "MyConfigMono",
			GtkFont:       "MyConfigGtk",
			DefaultSize:   14,
		},
		UIScale: 2.0,
		ModeColors: &entity.ThemeModeColors{
			PaneMode:    "#111111",
			TabMode:     "#222222",
			SessionMode: "#333333",
			ResizeMode:  "#444444",
		},
	}

	output, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Fonts should come from config, not external
	if output.Theme.Fonts.SansFont != "MyConfigSans" {
		t.Errorf("expected SansFont from config (%q), got %q",
			"MyConfigSans", output.Theme.Fonts.SansFont)
	}
	if output.Theme.Fonts.DefaultSize != 14 {
		t.Errorf("expected DefaultFontSize from config (14), got %d",
			output.Theme.Fonts.DefaultSize)
	}

	// UI scale should come from config
	if output.Theme.UIScale != 2.0 {
		t.Errorf("expected UIScale=2.0 from config, got %f", output.Theme.UIScale)
	}

	// Mode colors should come from config
	if output.Theme.ModeColors.PaneMode != "#111111" {
		t.Errorf("expected PaneMode from config (#111111), got %q",
			output.Theme.ModeColors.PaneMode)
	}
}

// TestResolveTheme_PartialExternal_MergesOverConfig verifies that empty external
// fields are treated as "inherit" rather than malformed.
func TestResolveTheme_PartialExternal_MergesOverConfig(t *testing.T) {
	partialLight := &entity.ColorPalette{
		Accent: "#123456",
	}
	partialDark := &entity.ColorPalette{
		Background: "#010203",
		Border:     "#abcdef",
	}
	mock := &newMockExternalSource{
		theme: &entity.ExternalTheme{
			Name:         "Partial Noctalia",
			Provider:     "noctalia",
			LightPalette: partialLight,
			DarkPalette:  partialDark,
		},
		enabled: true,
	}
	uc := NewResolveThemeUseCase(mock)

	input := ResolveThemeInput{
		PrefersDark:       true,
		ColorSchemeSource: "adwaita",
		LightPalette:      configLightPalette(),
		DarkPalette:       configDarkPalette(),
		UIScale:           1.0,
	}

	output, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertValidPaletteHex(t, "LightPalette", output.Theme.LightPalette)
	assertValidPaletteHex(t, "DarkPalette", output.Theme.DarkPalette)
	assertNoWarnings(t, output.Theme.Warnings)

	if output.Theme.LightPalette.Accent != "#123456" {
		t.Errorf("expected partial external light accent override, got %q", output.Theme.LightPalette.Accent)
	}
	if output.Theme.LightPalette.Background != configLightPalette().Background {
		t.Errorf("expected empty external light background to inherit config value %q, got %q",
			configLightPalette().Background, output.Theme.LightPalette.Background)
	}
	if output.Theme.DarkPalette.Background != "#010203" {
		t.Errorf("expected partial external dark background override, got %q", output.Theme.DarkPalette.Background)
	}
	if output.Theme.DarkPalette.Text != configDarkPalette().Text {
		t.Errorf("expected empty external dark text to inherit config value %q, got %q",
			configDarkPalette().Text, output.Theme.DarkPalette.Text)
	}
	if output.Theme.ThemeSource.Kind != entity.ThemeSourceExternal || output.Theme.ThemeSource.Provider != "noctalia" {
		t.Errorf("expected noctalia external source metadata, got %+v", output.Theme.ThemeSource)
	}
}

// TestResolveTheme_PartialExternal_MergesOverDefaults verifies partial external
// palettes also merge over built-in defaults when config palettes are absent.
func TestResolveTheme_PartialExternal_MergesOverDefaults(t *testing.T) {
	mock := &newMockExternalSource{
		theme: &entity.ExternalTheme{
			Name: "Partial Defaults",
			LightPalette: &entity.ColorPalette{
				Accent: "#112233",
			},
			DarkPalette: &entity.ColorPalette{
				Accent: "#445566",
			},
		},
		enabled: true,
	}
	uc := NewResolveThemeUseCase(mock)

	output, err := uc.Execute(context.Background(), ResolveThemeInput{PrefersDark: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertValidPaletteHex(t, "LightPalette", output.Theme.LightPalette)
	assertValidPaletteHex(t, "DarkPalette", output.Theme.DarkPalette)
	if output.Theme.LightPalette.Accent != "#112233" {
		t.Errorf("expected external light accent, got %q", output.Theme.LightPalette.Accent)
	}
	if output.Theme.LightPalette.Background != entity.DefaultLightPalette().Background {
		t.Errorf("expected light background to inherit default %q, got %q",
			entity.DefaultLightPalette().Background, output.Theme.LightPalette.Background)
	}
	if output.Theme.DarkPalette.Accent != "#445566" {
		t.Errorf("expected external dark accent, got %q", output.Theme.DarkPalette.Accent)
	}
}

func TestResolveTheme_PartialExternal_RepairsInvalidConfigFallbackFields(t *testing.T) {
	partialLight := &entity.ColorPalette{Accent: "#112233"}
	partialDark := &entity.ColorPalette{Background: "#445566"}
	mock := &newMockExternalSource{
		theme: &entity.ExternalTheme{
			Name:         "Partial With Invalid Config",
			Provider:     "noctalia",
			LightPalette: partialLight,
			DarkPalette:  partialDark,
		},
		enabled: true,
	}
	uc := NewResolveThemeUseCase(mock)
	invalidLight := *configLightPalette()
	invalidLight.Background = "not-a-hex"
	invalidDark := *configDarkPalette()
	invalidDark.Text = "also-invalid"

	output, err := uc.Execute(context.Background(), ResolveThemeInput{
		PrefersDark:  true,
		LightPalette: &invalidLight,
		DarkPalette:  &invalidDark,
		UIScale:      1.0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertValidPaletteHex(t, "LightPalette", output.Theme.LightPalette)
	assertValidPaletteHex(t, "DarkPalette", output.Theme.DarkPalette)
	if output.Theme.LightPalette.Accent != "#112233" {
		t.Fatalf("expected valid external light accent to survive invalid config fallback, got %q", output.Theme.LightPalette.Accent)
	}
	if output.Theme.LightPalette.Background != entity.DefaultLightPalette().Background {
		t.Fatalf("expected invalid config light background to be repaired from defaults, got %q", output.Theme.LightPalette.Background)
	}
	if output.Theme.DarkPalette.Background != "#445566" {
		t.Fatalf("expected valid external dark background to survive invalid config fallback, got %q", output.Theme.DarkPalette.Background)
	}
	if output.Theme.DarkPalette.Text != entity.DefaultDarkPalette().Text {
		t.Fatalf("expected invalid config dark text to be repaired from defaults, got %q", output.Theme.DarkPalette.Text)
	}
	if output.Theme.ThemeSource.Kind != entity.ThemeSourceExternal || output.Theme.ThemeSource.LastGood {
		t.Fatalf("expected fresh external source metadata, got %+v", output.Theme.ThemeSource)
	}
}

// ============================================================
// PrefersDark selects ActivePalette
// ============================================================

// TestResolveTheme_PrefersDark_SelectsDarkPalette verifies that when PrefersDark=true,
// ActivePalette is the dark palette.
func TestResolveTheme_PrefersDark_SelectsDarkPalette(t *testing.T) {
	uc := NewResolveThemeUseCase(nil)

	input := ResolveThemeInput{
		PrefersDark:       true,
		ColorSchemeSource: "config",
		LightPalette:      configLightPalette(),
		DarkPalette:       configDarkPalette(),
		UIScale:           1.0,
	}

	output, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Theme.ActivePalette.Background != configDarkPalette().Background {
		t.Errorf("expected ActivePalette to be dark palette, got Background=%q",
			output.Theme.ActivePalette.Background)
	}
	if !output.Theme.PrefersDark {
		t.Error("expected PrefersDark=true in output")
	}
}

// TestResolveTheme_PrefersLight_SelectsLightPalette verifies that when PrefersDark=false,
// ActivePalette is the light palette.
func TestResolveTheme_PrefersLight_SelectsLightPalette(t *testing.T) {
	uc := NewResolveThemeUseCase(nil)

	input := ResolveThemeInput{
		PrefersDark:       false,
		ColorSchemeSource: "config",
		LightPalette:      configLightPalette(),
		DarkPalette:       configDarkPalette(),
		UIScale:           1.0,
	}

	output, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Theme.ActivePalette.Background != configLightPalette().Background {
		t.Errorf("expected ActivePalette to be light palette, got Background=%q",
			output.Theme.ActivePalette.Background)
	}
	if output.Theme.PrefersDark {
		t.Error("expected PrefersDark=false in output")
	}
}

// ============================================================
// Malformed external at startup → config/default palettes + warning
// ============================================================

// TestResolveTheme_MalformedExternalAtStartup_FallsBackToConfig tests that when the
// external theme has malformed palette colors on first resolution, the usecase falls
// back to config/defaults and includes a warning.
func TestResolveTheme_MalformedExternalAtStartup_FallsBackToConfig(t *testing.T) {
	malformed := &entity.ExternalTheme{
		Name: "Malformed Theme",
		LightPalette: &entity.ColorPalette{
			Background: "rgb(0,0,0)", // not hex
			Text:       "#ffffff",
		},
		DarkPalette: &entity.ColorPalette{
			Background: "#hijklm", // invalid hex
			Text:       "#ffffff",
		},
	}
	mock := &newMockExternalSource{
		theme:   malformed,
		enabled: true,
	}
	uc := NewResolveThemeUseCase(mock)

	input := ResolveThemeInput{
		PrefersDark:       true,
		ColorSchemeSource: "adwaita",
		LightPalette:      configLightPalette(),
		DarkPalette:       configDarkPalette(),
		UIScale:           1.0,
	}

	output, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertValidPaletteHex(t, "LightPalette", output.Theme.LightPalette)
	assertValidPaletteHex(t, "DarkPalette", output.Theme.DarkPalette)
	assertValidPaletteHex(t, "ActivePalette", output.Theme.ActivePalette)

	// Should fall back to config palette (not the malformed external)
	if output.Theme.DarkPalette.Background != configDarkPalette().Background {
		t.Errorf("expected fallback to config dark palette (%q), got %q",
			configDarkPalette().Background, output.Theme.DarkPalette.Background)
	}

	// Should have at least one warning about the malformed external
	if len(output.Theme.Warnings) == 0 {
		t.Error("expected at least one warning for malformed external theme")
	}
}

// TestResolveTheme_MissingExternalAtStartup_FallsBackWithWarning verifies that enabled
// external sources returning nil data are treated as missing, not silently ignored.
func TestResolveTheme_MissingExternalAtStartup_FallsBackWithWarning(t *testing.T) {
	mock := &newMockExternalSource{
		theme:   nil,
		enabled: true,
	}
	uc := NewResolveThemeUseCase(mock)

	input := ResolveThemeInput{
		PrefersDark:       true,
		ColorSchemeSource: "adwaita",
		LightPalette:      configLightPalette(),
		DarkPalette:       configDarkPalette(),
		UIScale:           1.0,
	}

	output, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Theme.DarkPalette.Background != configDarkPalette().Background {
		t.Errorf("expected missing external startup fallback to config dark palette %q, got %q",
			configDarkPalette().Background, output.Theme.DarkPalette.Background)
	}
	if len(output.Theme.Warnings) == 0 {
		t.Fatal("expected warning for missing external theme")
	}
	if output.Theme.ThemeSource.Kind == entity.ThemeSourceExternal {
		t.Errorf("expected non-external source for startup missing external, got %+v", output.Theme.ThemeSource)
	}
}

func TestResolveTheme_ReadErrorAtStartup_FallsBackWithWarning(t *testing.T) {
	mock := &newMockExternalSource{
		err:      errors.New("read failed"),
		enabled:  true,
		identity: "noctalia|colors-json|/tmp/theme-a.json",
	}
	uc := NewResolveThemeUseCase(mock)

	input := ResolveThemeInput{
		PrefersDark:       true,
		ColorSchemeSource: "adwaita",
		LightPalette:      configLightPalette(),
		DarkPalette:       configDarkPalette(),
		UIScale:           1.0,
	}

	output, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Theme.DarkPalette.Background != configDarkPalette().Background {
		t.Errorf("expected read-error startup fallback to config dark palette %q, got %q",
			configDarkPalette().Background, output.Theme.DarkPalette.Background)
	}
	if output.Theme.ThemeSource.Kind == entity.ThemeSourceExternal {
		t.Errorf("expected non-external source for startup read error, got %+v", output.Theme.ThemeSource)
	}
	if len(output.Theme.Warnings) == 0 {
		t.Fatal("expected warning for external read error")
	}
}

func TestResolveTheme_ReadErrorAfterValid_KeepsLastGoodForSameIdentity(t *testing.T) {
	ext := validExternalTheme()
	mock := &newMockExternalSource{
		theme:    ext,
		enabled:  true,
		identity: "noctalia|colors-json|/tmp/theme-a.json",
	}
	uc := NewResolveThemeUseCase(mock)

	input := ResolveThemeInput{
		PrefersDark:       true,
		ColorSchemeSource: "adwaita",
		LightPalette:      configLightPalette(),
		DarkPalette:       configDarkPalette(),
		UIScale:           1.0,
	}

	first, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("first resolution error: %v", err)
	}
	mock.theme = nil
	mock.err = errors.New("temporary read failure")

	second, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("second resolution error: %v", err)
	}
	if second.Theme.DarkPalette.Background != first.Theme.DarkPalette.Background {
		t.Errorf("expected same-identity read error to keep last-good dark background %q, got %q",
			first.Theme.DarkPalette.Background, second.Theme.DarkPalette.Background)
	}
	if !second.Theme.ThemeSource.LastGood {
		t.Fatal("expected LastGood=true for same-identity read error after valid external")
	}
}

func TestResolveTheme_ExternalIdentityChangeClearsLastGoodBeforeReadError(t *testing.T) {
	ext := validExternalTheme()
	mock := &newMockExternalSource{
		theme:    ext,
		enabled:  true,
		identity: "noctalia|colors-json|/tmp/theme-a.json",
	}
	uc := NewResolveThemeUseCase(mock)

	input := ResolveThemeInput{
		PrefersDark:       true,
		ColorSchemeSource: "adwaita",
		LightPalette:      configLightPalette(),
		DarkPalette:       configDarkPalette(),
		UIScale:           1.0,
	}

	first, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("first resolution error: %v", err)
	}
	if first.Theme.DarkPalette.Background == configDarkPalette().Background {
		t.Fatal("first resolution should use external palette, not config")
	}

	mock.theme = nil
	mock.err = errors.New("new path missing")
	mock.identity = "noctalia|colors-json|/tmp/theme-b.json"

	second, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("second resolution error: %v", err)
	}
	if second.Theme.DarkPalette.Background != configDarkPalette().Background {
		t.Errorf("expected changed-identity read error to fall back to config dark background %q, got %q",
			configDarkPalette().Background, second.Theme.DarkPalette.Background)
	}
	if second.Theme.ThemeSource.LastGood {
		t.Fatal("expected LastGood=false after external identity changed")
	}
	if second.Theme.ThemeSource.Kind == entity.ThemeSourceExternal {
		t.Errorf("expected non-external source after changed-identity read error, got %+v", second.Theme.ThemeSource)
	}
}

// TestResolveTheme_MissingExternalAfterValid_KeepsLastGood verifies missing nil data
// after a valid external theme keeps last-good with warning metadata.
func TestResolveTheme_MissingExternalAfterValid_KeepsLastGood(t *testing.T) {
	ext := validExternalTheme()
	mock := &newMockExternalSource{
		theme:   ext,
		enabled: true,
	}
	uc := NewResolveThemeUseCase(mock)

	input := ResolveThemeInput{
		PrefersDark:       true,
		ColorSchemeSource: "adwaita",
		LightPalette:      configLightPalette(),
		DarkPalette:       configDarkPalette(),
		UIScale:           1.0,
	}

	first, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("first resolution error: %v", err)
	}
	mock.theme = nil

	second, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("second resolution error: %v", err)
	}

	if second.Theme.DarkPalette.Background != first.Theme.DarkPalette.Background {
		t.Errorf("expected last-good dark background %q, got %q",
			first.Theme.DarkPalette.Background, second.Theme.DarkPalette.Background)
	}
	if !second.Theme.ThemeSource.LastGood {
		t.Error("expected LastGood=true when missing external follows valid external")
	}
	if second.Theme.ThemeSource.Provider != "noctalia" {
		t.Errorf("expected provider noctalia, got %q", second.Theme.ThemeSource.Provider)
	}
	if len(second.Theme.Warnings) == 0 {
		t.Fatal("expected warning for missing external after valid")
	}
}

// TestResolveTheme_MalformedExternalMissingPalette_FallsBack verifies fallback
// when the external theme has nil palettes.
func TestResolveTheme_MalformedExternalMissingPalette_FallsBack(t *testing.T) {
	malformed := &entity.ExternalTheme{
		Name:         "Missing Palettes",
		LightPalette: nil,
		DarkPalette:  nil,
	}
	mock := &newMockExternalSource{
		theme:   malformed,
		enabled: true,
	}
	uc := NewResolveThemeUseCase(mock)

	input := ResolveThemeInput{
		PrefersDark:       true,
		ColorSchemeSource: "adwaita",
		LightPalette:      configLightPalette(),
		DarkPalette:       configDarkPalette(),
		UIScale:           1.0,
	}

	output, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertValidPaletteHex(t, "LightPalette", output.Theme.LightPalette)
	assertValidPaletteHex(t, "DarkPalette", output.Theme.DarkPalette)

	// Should have warnings about missing palettes
	if len(output.Theme.Warnings) == 0 {
		t.Error("expected warnings for external theme with nil palettes")
	}
}

// ============================================================
// Malformed after valid external → keep last-good external + warning
// ============================================================

// TestResolveTheme_MalformedAfterValid_KeepsLastGood tests the "last-good" contract:
// when an external theme becomes malformed after previously being valid, the usecase
// keeps the last-good external palette colors and emits a warning.
func TestResolveTheme_MalformedAfterValid_KeepsLastGood(t *testing.T) {
	// Step 1: First resolution with valid external
	ext := validExternalTheme()
	mock := &newMockExternalSource{
		theme:   ext,
		enabled: true,
	}
	uc := NewResolveThemeUseCase(mock)

	input := ResolveThemeInput{
		PrefersDark:       true,
		ColorSchemeSource: "adwaita",
		LightPalette:      configLightPalette(),
		DarkPalette:       configDarkPalette(),
		UIScale:           1.0,
	}

	output1, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("first resolution error: %v", err)
	}

	firstDarkBg := output1.Theme.DarkPalette.Background
	// Verify first resolution uses external palette
	if firstDarkBg != ext.DarkPalette.Background {
		t.Fatalf("expected first resolution to use external dark palette (%q), got %q",
			ext.DarkPalette.Background, firstDarkBg)
	}

	// Step 2: External becomes malformed
	malformed := &entity.ExternalTheme{
		Name: "Now Malformed",
		LightPalette: &entity.ColorPalette{
			Background: "bad-color",
			Text:       "#ffffff",
		},
		DarkPalette: nil, // missing palette
	}
	mock.theme = malformed

	output2, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("second resolution error: %v", err)
	}

	assertValidPaletteHex(t, "DarkPalette", output2.Theme.DarkPalette)
	assertValidPaletteHex(t, "ActivePalette", output2.Theme.ActivePalette)

	// Should keep last-good colors (from first resolution)
	if output2.Theme.DarkPalette.Background != firstDarkBg {
		t.Errorf("expected to keep last-good dark palette background (%q), got %q",
			firstDarkBg, output2.Theme.DarkPalette.Background)
	}

	// Should have warnings about the malformed update
	if len(output2.Theme.Warnings) == 0 {
		t.Error("expected warnings when external becomes malformed after being valid")
	}

	// ThemeSource should still indicate external (last-good)
	if output2.Theme.ThemeSource.Kind != entity.ThemeSourceExternal {
		t.Errorf("expected ThemeSource.Kind='external' for last-good, got %q", output2.Theme.ThemeSource.Kind)
	}
	if output2.Theme.ThemeSource.Provider != "noctalia" {
		t.Errorf("expected ThemeSource.Provider='noctalia' for last-good, got %q", output2.Theme.ThemeSource.Provider)
	}
	if !output2.Theme.ThemeSource.LastGood {
		t.Error("expected ThemeSource.LastGood=true for last-good external palette")
	}
}

// ============================================================
// Disabling external after valid → clear last-good and use config/defaults
// ============================================================

// TestResolveTheme_DisablingExternalAfterValid_ClearsLastGood verifies that when the
// external source is disabled after having provided a valid theme, last-good is cleared
// and config/defaults are used instead.
func TestResolveTheme_DisablingExternalAfterValid_ClearsLastGood(t *testing.T) {
	// Step 1: First resolution with valid external
	ext := validExternalTheme()
	mock := &newMockExternalSource{
		theme:   ext,
		enabled: true,
	}
	uc := NewResolveThemeUseCase(mock)

	input := ResolveThemeInput{
		PrefersDark:       true,
		ColorSchemeSource: "adwaita",
		LightPalette:      configLightPalette(),
		DarkPalette:       configDarkPalette(),
		UIScale:           1.0,
	}

	output1, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("first resolution error: %v", err)
	}

	// Verify first resolution uses external palette
	if output1.Theme.DarkPalette.Background == configDarkPalette().Background {
		t.Fatal("first resolution should use external palette, not config")
	}

	// Step 2: External becomes disabled
	mock.theme = nil
	mock.enabled = false

	output2, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("second resolution error: %v", err)
	}

	assertValidPaletteHex(t, "DarkPalette", output2.Theme.DarkPalette)

	// Should fall back to config palette (NOT keep last-good)
	if output2.Theme.DarkPalette.Background != configDarkPalette().Background {
		t.Errorf("expected fallback to config dark palette (%q) after external disabled, got %q",
			configDarkPalette().Background, output2.Theme.DarkPalette.Background)
	}

	// ThemeSource should indicate config or default
	if output2.Theme.ThemeSource.Kind == entity.ThemeSourceExternal {
		t.Error("expected ThemeSource.Kind NOT to be 'external' after external is disabled")
	}
	if output2.Theme.ThemeSource.LastGood {
		t.Error("expected ThemeSource.LastGood=false after external is disabled")
	}
}

// TestResolveTheme_DisablingExternalTwice_Noop verifies that disabling external
// when it was already disabled is a no-op (no warnings, uses config).
func TestResolveTheme_DisablingExternalTwice_Noop(t *testing.T) {
	// External already disabled from start
	mock := &newMockExternalSource{
		enabled: false,
	}
	uc := NewResolveThemeUseCase(mock)

	input := ResolveThemeInput{
		PrefersDark:       true,
		ColorSchemeSource: "adwaita",
		LightPalette:      configLightPalette(),
		DarkPalette:       configDarkPalette(),
		UIScale:           1.0,
	}

	output1, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("first resolution error: %v", err)
	}

	// Should use config
	if output1.Theme.DarkPalette.Background != configDarkPalette().Background {
		t.Errorf("expected config dark palette, got %q", output1.Theme.DarkPalette.Background)
	}

	// Second resolution with same disabled state — should behave identically
	output2, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("second resolution error: %v", err)
	}

	if output2.Theme.DarkPalette.Background != configDarkPalette().Background {
		t.Errorf("expected config dark palette on second call, got %q", output2.Theme.DarkPalette.Background)
	}

	assertNoWarnings(t, output2.Theme.Warnings)
}

// ============================================================
// Output completeness: all ResolvedTheme fields
// ============================================================

// TestResolveTheme_OutputContainsAllFields verifies that every field of
// ResolvedTheme is populated in the output (no zero-value surprises).
func TestResolveTheme_OutputContainsAllFields(t *testing.T) {
	uc := NewResolveThemeUseCase(nil)

	fonts := entity.ThemeFonts{
		SansFont:      "config-sans",
		SerifFont:     "config-serif",
		MonospaceFont: "config-mono",
		GtkFont:       "config-gtk",
		DefaultSize:   12,
	}
	modeColors := entity.ThemeModeColors{
		PaneMode:    "#111111",
		TabMode:     "#222222",
		SessionMode: "#333333",
		ResizeMode:  "#444444",
	}

	input := ResolveThemeInput{
		ColorScheme:       "prefer-dark",
		PrefersDark:       true,
		ColorSchemeSource: "config",
		LightPalette:      configLightPalette(),
		DarkPalette:       configDarkPalette(),
		Fonts:             &fonts,
		UIScale:           1.25,
		ModeColors:        &modeColors,
	}

	output, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rt := output.Theme

	// LightPalette must be valid
	assertValidPaletteHex(t, "LightPalette", rt.LightPalette)
	// DarkPalette must be valid
	assertValidPaletteHex(t, "DarkPalette", rt.DarkPalette)
	// ActivePalette must be valid
	assertValidPaletteHex(t, "ActivePalette", rt.ActivePalette)
	// PrefersDark
	if !rt.PrefersDark {
		t.Error("expected PrefersDark=true")
	}
	// ColorSchemeSource
	if rt.ColorSchemeSource != "config" {
		t.Errorf("expected ColorSchemeSource='config', got %q", rt.ColorSchemeSource)
	}
	// ThemeSource
	if rt.ThemeSource.Kind == "" {
		t.Error("expected non-empty ThemeSource.Kind")
	}
	// Fonts
	if rt.Fonts.SansFont == "" {
		t.Error("expected non-empty Fonts.SansFont")
	}
	if rt.Fonts.SerifFont == "" {
		t.Error("expected non-empty Fonts.SerifFont")
	}
	if rt.Fonts.DefaultSize <= 0 {
		t.Errorf("expected positive Fonts.DefaultSize, got %d", rt.Fonts.DefaultSize)
	}
	// UIScale
	if rt.UIScale <= 0 {
		t.Errorf("expected positive UIScale, got %f", rt.UIScale)
	}
	// ModeColors
	if rt.ModeColors.PaneMode == "" {
		t.Error("expected non-empty ModeColors.PaneMode")
	}
	if rt.ModeColors.TabMode == "" {
		t.Error("expected non-empty ModeColors.TabMode")
	}
	if rt.ModeColors.SessionMode == "" {
		t.Error("expected non-empty ModeColors.SessionMode")
	}
	if rt.ModeColors.ResizeMode == "" {
		t.Error("expected non-empty ModeColors.ResizeMode")
	}
}

// ============================================================
// All palette colors must be valid CSS-safe hex
// ============================================================

// TestResolveTheme_AllOutputPalettesAreCSSSafeHex is the contract test: every color
// field in every palette of the output must be a valid #RRGGBB hex color.
func TestResolveTheme_AllOutputPalettesAreCSSSafeHex(t *testing.T) {
	uc := NewResolveThemeUseCase(nil)

	input := ResolveThemeInput{
		PrefersDark:       false,
		ColorSchemeSource: "fallback",
		UIScale:           1.0,
		// No config palettes — must get valid defaults
	}

	output, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertValidPaletteHex(t, "LightPalette", output.Theme.LightPalette)
	assertValidPaletteHex(t, "DarkPalette", output.Theme.DarkPalette)
	assertValidPaletteHex(t, "ActivePalette", output.Theme.ActivePalette)
}

// ============================================================
// Font defaults are used when no config or external fonts provided
// ============================================================

// TestResolveTheme_NoConfigFonts_UsesDefaults verifies that when fonts are not
// provided in config or external, sensible defaults are used.
func TestResolveTheme_NoConfigFonts_UsesDefaults(t *testing.T) {
	uc := NewResolveThemeUseCase(nil)

	input := ResolveThemeInput{
		PrefersDark:       true,
		ColorSchemeSource: "fallback",
		UIScale:           1.0,
		// Fonts is nil
	}

	output, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defaults := entity.DefaultThemeFonts()

	if output.Theme.Fonts.SansFont != defaults.SansFont {
		t.Errorf("expected default SansFont (%q), got %q",
			defaults.SansFont, output.Theme.Fonts.SansFont)
	}
	if output.Theme.Fonts.SerifFont != defaults.SerifFont {
		t.Errorf("expected default SerifFont (%q), got %q",
			defaults.SerifFont, output.Theme.Fonts.SerifFont)
	}
	if output.Theme.Fonts.DefaultSize != defaults.DefaultSize {
		t.Errorf("expected default DefaultSize (%d), got %d",
			defaults.DefaultSize, output.Theme.Fonts.DefaultSize)
	}
}

// ============================================================
// Mode color defaults are used when not provided
// ============================================================

// TestResolveTheme_NoConfigModeColors_UsesDefaults verifies that when mode colors
// are not provided, sensible defaults are used.
func TestResolveTheme_NoConfigModeColors_UsesDefaults(t *testing.T) {
	uc := NewResolveThemeUseCase(nil)

	input := ResolveThemeInput{
		PrefersDark:       true,
		ColorSchemeSource: "fallback",
		UIScale:           1.0,
		// ModeColors is nil
	}

	output, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Mode colors must be valid CSS-safe hex
	fields := map[string]string{
		"PaneMode":    output.Theme.ModeColors.PaneMode,
		"TabMode":     output.Theme.ModeColors.TabMode,
		"SessionMode": output.Theme.ModeColors.SessionMode,
		"ResizeMode":  output.Theme.ModeColors.ResizeMode,
	}
	for name, value := range fields {
		if value == "" {
			t.Errorf("expected non-empty ModeColors.%s", name)
		}
		if !entity.IsValidHex(value) {
			t.Errorf("ModeColors.%s = %q is not valid CSS-safe hex", name, value)
		}
	}
}

// TestResolveTheme_InvalidModeColors_FallsBackToDefaults verifies invalid config mode
// colors cannot pass through ResolvedTheme.
func TestResolveTheme_InvalidModeColors_FallsBackToDefaults(t *testing.T) {
	uc := NewResolveThemeUseCase(nil)
	invalid := &entity.ThemeModeColors{
		PaneMode:    "not-a-color",
		TabMode:     "#222222",
		SessionMode: "#333333",
		ResizeMode:  "#444444",
	}

	output, err := uc.Execute(context.Background(), ResolveThemeInput{
		PrefersDark:       true,
		ColorSchemeSource: "config",
		ModeColors:        invalid,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defaults := entity.DefaultThemeModeColors()
	if output.Theme.ModeColors != defaults {
		t.Errorf("expected invalid mode colors to fall back to defaults %+v, got %+v", defaults, output.Theme.ModeColors)
	}
	if len(output.Theme.Warnings) == 0 {
		t.Fatal("expected warning for invalid mode colors")
	}
}

// ============================================================
// UIScale defaults
// ============================================================

// TestResolveTheme_UIScaleDefaultsToOne verifies that when UIScale is zero or negative,
// it defaults to 1.0.
func TestResolveTheme_UIScaleDefaultsToOne(t *testing.T) {
	uc := NewResolveThemeUseCase(nil)

	tests := []struct {
		name     string
		uiScale  float64
		expected float64
	}{
		{"zero", 0.0, 1.0},
		{"negative", -0.5, 1.0},
		{"positive", 1.25, 1.25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := ResolveThemeInput{
				PrefersDark:       true,
				ColorSchemeSource: "fallback",
				UIScale:           tt.uiScale,
			}

			output, err := uc.Execute(context.Background(), input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if output.Theme.UIScale != tt.expected {
				t.Errorf("expected UIScale=%f, got %f", tt.expected, output.Theme.UIScale)
			}
		})
	}
}

// ============================================================
// Color scheme source is properly captured
// ============================================================

// TestResolveTheme_ColorSchemeSourceIsCaptured verifies that the color scheme
// source string is passed through to the output.
func TestResolveTheme_ColorSchemeSourceIsCaptured(t *testing.T) {
	uc := NewResolveThemeUseCase(nil)

	sources := []string{"config", "adwaita", "gsettings", "env", "fallback"}

	for _, source := range sources {
		input := ResolveThemeInput{
			PrefersDark:       true,
			ColorSchemeSource: source,
			UIScale:           1.0,
		}

		output, err := uc.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error for source %q: %v", source, err)
		}

		if output.Theme.ColorSchemeSource != source {
			t.Errorf("expected ColorSchemeSource=%q, got %q", source, output.Theme.ColorSchemeSource)
		}
	}
}

// ============================================================
// Refresh API
// ============================================================

// TestResolveTheme_RefreshDelegatesToExecute verifies the refresh/update API returns
// the same single result shape as Execute without adding CSS generation or I/O.
func TestResolveTheme_RefreshDelegatesToExecute(t *testing.T) {
	uc := NewResolveThemeUseCase(nil)
	input := ResolveThemeInput{
		PrefersDark:       true,
		ColorSchemeSource: "adwaita",
		LightPalette:      configLightPalette(),
		DarkPalette:       configDarkPalette(),
		UIScale:           1.25,
	}

	output, err := uc.Refresh(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !output.Theme.PrefersDark {
		t.Error("expected refreshed theme to preserve PrefersDark")
	}
	if output.Theme.ActivePalette.Background != configDarkPalette().Background {
		t.Errorf("expected refreshed active palette background %q, got %q",
			configDarkPalette().Background, output.Theme.ActivePalette.Background)
	}
	if output.Theme.UIScale != 1.25 {
		t.Errorf("expected refreshed UI scale 1.25, got %f", output.Theme.UIScale)
	}
}

// ============================================================
// Input mapping from domain config shapes
// ============================================================

// TestResolveThemeInputFromConfig_MapsAppearanceStylingAndPreference verifies
// app/bootstrap callers can build usecase input without duplicating fallback logic.
func TestResolveThemeInputFromConfig_MapsAppearanceStylingAndPreference(t *testing.T) {
	appearance := &entity.AppearanceConfig{
		SansFont:        "Inter",
		SerifFont:       "Georgia",
		MonospaceFont:   "JetBrains Mono",
		GtkFont:         "Adwaita Sans",
		DefaultFontSize: 18,
		ColorScheme:     "prefer-dark",
		LightPalette:    *configLightPalette(),
		DarkPalette:     *configDarkPalette(),
	}
	styling := &entity.WorkspaceStylingConfig{
		PaneModeColor:    "#111111",
		TabModeColor:     "#222222",
		SessionModeColor: "#333333",
		ResizeModeColor:  "#444444",
	}
	preference := port.ColorSchemePreference{PrefersDark: true, Source: "adwaita"}

	input := ResolveThemeInputFromConfig(appearance, 1.25, styling, preference)

	if input.ColorScheme != "prefer-dark" {
		t.Errorf("expected color scheme prefer-dark, got %q", input.ColorScheme)
	}
	if !input.PrefersDark || input.ColorSchemeSource != "adwaita" {
		t.Errorf("expected preference from detector, got PrefersDark=%v Source=%q", input.PrefersDark, input.ColorSchemeSource)
	}
	if input.LightPalette == nil || input.LightPalette.Background != configLightPalette().Background {
		t.Fatalf("expected light palette pointer from appearance, got %+v", input.LightPalette)
	}
	if input.DarkPalette == nil || input.DarkPalette.Background != configDarkPalette().Background {
		t.Fatalf("expected dark palette pointer from appearance, got %+v", input.DarkPalette)
	}
	if input.Fonts == nil || input.Fonts.SansFont != "Inter" || input.Fonts.DefaultSize != 18 {
		t.Fatalf("expected fonts from appearance, got %+v", input.Fonts)
	}
	if input.UIScale != 1.25 {
		t.Errorf("expected UI scale 1.25, got %f", input.UIScale)
	}
	if input.ModeColors == nil || input.ModeColors.PaneMode != "#111111" || input.ModeColors.ResizeMode != "#444444" {
		t.Fatalf("expected mode colors from styling, got %+v", input.ModeColors)
	}
}

// TestResolveThemeInputFromConfig_AllowsNilConfig verifies callers can resolve defaults
// when appearance/styling config is unavailable.
func TestResolveThemeInputFromConfig_AllowsNilConfig(t *testing.T) {
	preference := port.ColorSchemePreference{PrefersDark: false, Source: "fallback"}

	input := ResolveThemeInputFromConfig(nil, 0, nil, preference)

	if input.ColorScheme != "" {
		t.Errorf("expected empty color scheme for nil appearance, got %q", input.ColorScheme)
	}
	if input.PrefersDark || input.ColorSchemeSource != "fallback" {
		t.Errorf("expected fallback light preference, got PrefersDark=%v Source=%q", input.PrefersDark, input.ColorSchemeSource)
	}
	if input.LightPalette != nil || input.DarkPalette != nil || input.Fonts != nil || input.ModeColors != nil {
		t.Fatalf("expected nil optional inputs, got light=%v dark=%v fonts=%v modes=%v",
			input.LightPalette, input.DarkPalette, input.Fonts, input.ModeColors)
	}
}
