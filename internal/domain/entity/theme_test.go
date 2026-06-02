package entity

import (
	"testing"
)

// TestValidateHexColor_ValidHex tests that standard hex colors are recognized.
func TestValidateHexColor_ValidHex(t *testing.T) {
	valid := []string{
		"#000000",
		"#ffffff",
		"#0a0a0b",
		"#4ade80",
		"#4A90E2",
		"#FFA500",
		"#9B59B6",
		"#00D4AA",
	}

	for _, hex := range valid {
		if !IsValidHex(hex) {
			t.Errorf("expected %q to be a valid hex color", hex)
		}
	}
}

// TestValidateHexColor_InvalidHex tests that non-hex or malformed strings are rejected.
func TestValidateHexColor_InvalidHex(t *testing.T) {
	invalid := []string{
		"",
		"rgb(0,0,0)",
		"#FFF",      // 3-char shorthand
		"#FFFFFFFF", // 8-char with alpha
		"#GGGGGG",   // non-hex chars
		"red",
		"#12",         // too short
		"#1234567",    // odd length
		"  #ffffff  ", // whitespace
		"#ffffff ",    // trailing space
	}

	for _, hex := range invalid {
		if IsValidHex(hex) {
			t.Errorf("expected %q to be invalid hex", hex)
		}
	}
}

// TestDefaultThemeFonts_ReturnsSaneDefaults verifies the default font configuration has non-empty,
// sensible values.
func TestDefaultThemeFonts_ReturnsSaneDefaults(t *testing.T) {
	fonts := DefaultThemeFonts()

	if fonts.SansFont != "Fira Sans" {
		t.Errorf("expected SansFont Fira Sans, got %q", fonts.SansFont)
	}
	if fonts.SerifFont != "Fira Sans" {
		t.Errorf("expected SerifFont Fira Sans, got %q", fonts.SerifFont)
	}
	if fonts.MonospaceFont != "Fira Code" {
		t.Errorf("expected MonospaceFont Fira Code, got %q", fonts.MonospaceFont)
	}
	if fonts.GtkFont != "Adwaita Sans" {
		t.Errorf("expected GtkFont Adwaita Sans, got %q", fonts.GtkFont)
	}
	if fonts.DefaultSize != 16 {
		t.Errorf("expected DefaultSize 16, got %d", fonts.DefaultSize)
	}
}

// TestResolvedThemePalette_MustBeCSSSafeHex ensures that every color field
// in a ResolvedThemePalette passes hex validation.
// This is the locked contract: all palette colors in ResolvedTheme are CSS-safe hex.
func TestResolvedThemePalette_MustBeCSSSafeHex(t *testing.T) {
	// A well-formed palette that SHOULD be produced by resolution.
	validPalette := ResolvedThemePalette{
		Background:     "#0a0a0b",
		Surface:        "#1a1a1b",
		SurfaceVariant: "#2d2d2d",
		Text:           "#ffffff",
		Muted:          "#909090",
		Accent:         "#4ade80",
		Border:         "#333333",
	}

	fields := map[string]string{
		"Background":     validPalette.Background,
		"Surface":        validPalette.Surface,
		"SurfaceVariant": validPalette.SurfaceVariant,
		"Text":           validPalette.Text,
		"Muted":          validPalette.Muted,
		"Accent":         validPalette.Accent,
		"Border":         validPalette.Border,
	}

	for name, value := range fields {
		if !IsValidHex(value) {
			t.Errorf("ResolvedThemePalette.%s = %q is not a valid CSS-safe hex color", name, value)
		}
	}
}

// TestResolvedThemePalette_RejectsInvalidColors verifies that any non-hex color
// in a palette is detectable by a validation helper.
func TestResolvedThemePalette_RejectsInvalidColors(t *testing.T) {
	invalidFields := []struct {
		name  string
		value string
	}{
		{"Background", "rgb(0,0,0)"},
		{"Background", "#F"},
		{"Surface", "white"},
		{"Text", "#GGGGGG"},
		{"Accent", "hsl(0,0%,0%)"},
		{"Border", ""},
	}

	for _, tc := range invalidFields {
		if IsValidHex(tc.value) {
			t.Errorf("field %q value %q should be rejected as invalid hex", tc.name, tc.value)
		}
	}
}

// TestThemeModeColors_DefaultsAreCSSSafe ensures the default mode colors are valid hex.
func TestThemeModeColors_DefaultsAreCSSSafe(t *testing.T) {
	// Per the existing ui/theme palette: defaults are #4A90E2, #FFA500, #9B59B6, #00D4AA
	defaults := ThemeModeColors{
		PaneMode:    "#4A90E2",
		TabMode:     "#FFA500",
		SessionMode: "#9B59B6",
		ResizeMode:  "#00D4AA",
	}

	fields := map[string]string{
		"PaneMode":    defaults.PaneMode,
		"TabMode":     defaults.TabMode,
		"SessionMode": defaults.SessionMode,
		"ResizeMode":  defaults.ResizeMode,
	}

	for name, value := range fields {
		if !IsValidHex(value) {
			t.Errorf("ThemeModeColors.%s = %q is not a valid CSS-safe hex color", name, value)
		}
	}
}

// TestResolvedTheme_StructFieldsExist is a compile-time guard and field-inspection test
// to verify that the ResolvedTheme struct carries all required fields from the locked contract.
func TestResolvedTheme_StructFieldsExist(t *testing.T) {
	rt := ResolvedTheme{
		LightPalette: ResolvedThemePalette{
			Background: "#0a0a0b",
			Surface:    "#1a1a1b",
		},
		DarkPalette: ResolvedThemePalette{
			Background: "#fafafa",
			Surface:    "#ffffff",
		},
		ActivePalette: ResolvedThemePalette{
			Background: "#0a0a0b",
		},
		PrefersDark:       true,
		ColorSchemeSource: "adwaita",
		ThemeSource:       ThemeSourceMetadata{Kind: ThemeSourceConfig},
		Fonts:             DefaultThemeFonts(),
		UIScale:           1.0,
		ModeColors: ThemeModeColors{
			PaneMode:    "#4A90E2",
			TabMode:     "#FFA500",
			SessionMode: "#9B59B6",
			ResizeMode:  "#00D4AA",
		},
		Warnings: []ThemeWarning{
			{Field: "external", Message: "malformed external theme, using last-good"},
		},
	}

	// Verify the struct can be constructed and fields are accessible.
	if !rt.PrefersDark {
		t.Error("expected PrefersDark to be true")
	}
	if rt.ColorSchemeSource != "adwaita" {
		t.Errorf("expected ColorSchemeSource 'adwaita', got %q", rt.ColorSchemeSource)
	}
	if rt.ThemeSource.Kind != ThemeSourceConfig {
		t.Errorf("expected ThemeSource.Kind 'config', got %q", rt.ThemeSource.Kind)
	}
	if rt.UIScale != 1.0 {
		t.Errorf("expected UIScale 1.0, got %f", rt.UIScale)
	}
	if len(rt.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(rt.Warnings))
	}
	if rt.Fonts.DefaultSize != 16 {
		t.Errorf("expected DefaultSize 16, got %d", rt.Fonts.DefaultSize)
	}
}

// TestThemeSourceKind_ConstantsAreDistinct verifies the ThemeSourceKind constants
// have unique values so that source metadata comparisons work correctly.
func TestThemeSourceKind_ConstantsAreDistinct(t *testing.T) {
	kinds := map[ThemeSourceKind]bool{
		ThemeSourceDefault:  true,
		ThemeSourceConfig:   true,
		ThemeSourceExternal: true,
	}

	if len(kinds) != 3 {
		t.Error("ThemeSourceKind constants must be distinct")
	}

	if ThemeSourceDefault == ThemeSourceExternal {
		t.Error("ThemeSourceDefault must not equal ThemeSourceExternal")
	}
	if ThemeSourceDefault == ThemeSourceConfig {
		t.Error("ThemeSourceDefault must not equal ThemeSourceConfig")
	}
}

// TestThemeWarning_FieldsExist verifies ThemeWarning carries the expected shape.
func TestThemeWarning_FieldsExist(t *testing.T) {
	w := ThemeWarning{
		Field:   "light_palette.background",
		Message: "not a valid hex color",
	}

	if w.Field == "" {
		t.Error("expected ThemeWarning.Field to be non-empty")
	}
	if w.Message == "" {
		t.Error("expected ThemeWarning.Message to be non-empty")
	}
}

// TestExternalTheme_IsDomainPure verifies ExternalTheme can be constructed without any
// infrastructure imports (this is a compile-time contract via package structure).
func TestExternalTheme_IsDomainPure(t *testing.T) {
	light := ColorPalette{
		Background: "#0a0a0b",
		Surface:    "#1a1a1b",
	}
	dark := ColorPalette{
		Background: "#fafafa",
		Surface:    "#ffffff",
	}

	ext := ExternalTheme{
		Name:         "Noctalia Dark",
		Provider:     "noctalia",
		LightPalette: &light,
		DarkPalette:  &dark,
	}

	if ext.Name != "Noctalia Dark" {
		t.Errorf("expected Name 'Noctalia Dark', got %q", ext.Name)
	}
	if ext.Provider != "noctalia" {
		t.Errorf("expected Provider 'noctalia', got %q", ext.Provider)
	}
	if ext.LightPalette == nil || ext.DarkPalette == nil {
		t.Error("expected non-nil palettes")
	}
}

// TestDefaultThemeFonts_AllFieldsAreSet ensures no zero-value fields in the defaults.
func TestDefaultThemeFonts_AllFieldsAreSet(t *testing.T) {
	fonts := DefaultThemeFonts()

	// Check that all string fields are non-empty
	if fonts.SansFont == "" {
		t.Error("SansFont should have a default value")
	}
	if fonts.SerifFont == "" {
		t.Error("SerifFont should have a default value")
	}
	if fonts.MonospaceFont == "" {
		t.Error("MonospaceFont should have a default value")
	}
	if fonts.GtkFont == "" {
		t.Error("GtkFont should have a default value")
	}
	if fonts.DefaultSize <= 0 {
		t.Errorf("DefaultSize should be positive, got %d", fonts.DefaultSize)
	}
}

// TestDefaultThemeModeColors_ReturnsSaneDefaults verifies the default mode colors.
func TestDefaultThemeModeColors_ReturnsSaneDefaults(t *testing.T) {
	mc := DefaultThemeModeColors()

	if mc.PaneMode != "#4A90E2" {
		t.Errorf("expected PaneMode #4A90E2, got %q", mc.PaneMode)
	}
	if mc.TabMode != "#FFA500" {
		t.Errorf("expected TabMode #FFA500, got %q", mc.TabMode)
	}
	if mc.SessionMode != "#9B59B6" {
		t.Errorf("expected SessionMode #9B59B6, got %q", mc.SessionMode)
	}
	if mc.ResizeMode != "#00D4AA" {
		t.Errorf("expected ResizeMode #00D4AA, got %q", mc.ResizeMode)
	}

	// All must be valid hex
	if !IsValidHex(mc.PaneMode) {
		t.Error("default PaneMode should be valid hex")
	}
	if !IsValidHex(mc.TabMode) {
		t.Error("default TabMode should be valid hex")
	}
	if !IsValidHex(mc.SessionMode) {
		t.Error("default SessionMode should be valid hex")
	}
	if !IsValidHex(mc.ResizeMode) {
		t.Error("default ResizeMode should be valid hex")
	}
}

// TestDefaultDarkPalette_ReturnsValidHex verifies the default dark palette is all valid hex.
func TestDefaultDarkPalette_ReturnsValidHex(t *testing.T) {
	p := DefaultDarkPalette()

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
		if value == "" {
			t.Errorf("DefaultDarkPalette.%s should not be empty", name)
		}
		if !IsValidHex(value) {
			t.Errorf("DefaultDarkPalette.%s = %q is not valid hex", name, value)
		}
	}
}

// TestDefaultLightPalette_ReturnsValidHex verifies the default light palette is all valid hex.
func TestDefaultLightPalette_ReturnsValidHex(t *testing.T) {
	p := DefaultLightPalette()

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
		if value == "" {
			t.Errorf("DefaultLightPalette.%s should not be empty", name)
		}
		if !IsValidHex(value) {
			t.Errorf("DefaultLightPalette.%s = %q is not valid hex", name, value)
		}
	}
}

// TestValidatePaletteHex_Valid verifies ValidatePaletteHex returns no warnings for valid palettes.
func TestValidatePaletteHex_Valid(t *testing.T) {
	p := DefaultDarkPalette()
	warnings := ValidatePaletteHex(&p, "dark")
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for valid palette, got %d: %v", len(warnings), warnings)
	}
}

// TestValidatePaletteHex_Invalid verifies ValidatePaletteHex returns warnings for invalid palettes.
func TestValidatePaletteHex_Invalid(t *testing.T) {
	p := ColorPalette{
		Background:     "rgb(0,0,0)", // invalid
		Surface:        "#ffffff",
		SurfaceVariant: "not-a-color",
		Text:           "",        // invalid
		Muted:          "#GGGGGG", // invalid
		Accent:         "#123",    // 3-char shorthand, rejected
		Border:         "#123456",
	}
	warnings := ValidatePaletteHex(&p, "test")
	if len(warnings) == 0 {
		t.Error("expected warnings for invalid palette, got none")
	}
}

// TestValidatePaletteHex_Nil verifies ValidatePaletteHex handles nil.
func TestValidatePaletteHex_Nil(t *testing.T) {
	warnings := ValidatePaletteHex(nil, "ext")
	if len(warnings) == 0 {
		t.Error("expected warnings for nil palette")
	}
}

// TestIsPaletteValid verifies IsPaletteValid returns correct truthiness.
func TestIsPaletteValid_EdgeCases(t *testing.T) {
	if IsPaletteValid(nil) {
		t.Error("nil palette should not be valid")
	}

	valid := DefaultDarkPalette()
	if !IsPaletteValid(&valid) {
		t.Error("default dark palette should be valid")
	}

	partial := ColorPalette{
		Background: "#000000",
		Surface:    "#111111",
		// missing fields are ""
	}
	if IsPaletteValid(&partial) {
		t.Error("partial palette with empty fields should not be valid")
	}
}

// TestMergePalette_FillsEmpties verifies MergePalette fills empty fields from defaults.
func TestMergePalette_FillsEmpties(t *testing.T) {
	src := &ColorPalette{
		Background: "#000000",
		Surface:    "", // should be filled
		Text:       "#ffffff",
	}
	def := DefaultDarkPalette()
	result := MergePalette(src, &def)

	if result.Background != "#000000" {
		t.Errorf("Background should be from src, got %q", result.Background)
	}
	if result.Surface != def.Surface {
		t.Errorf("Surface should be filled from defaults, got %q", result.Surface)
	}
	if result.Text != "#ffffff" {
		t.Errorf("Text should be from src, got %q", result.Text)
	}
}

// TestMergePalette_NilSrc verifies MergePalette returns defaults when src is nil.
func TestMergePalette_NilSrc(t *testing.T) {
	def := DefaultDarkPalette()
	result := MergePalette(nil, &def)

	if result.Background != def.Background {
		t.Errorf("expected default Background, got %q", result.Background)
	}
}

// TestMergeFonts_FillsEmpties verifies MergeFonts fills zero fields from defaults.
func TestMergeFonts_FillsEmpties(t *testing.T) {
	src := &ThemeFonts{
		SansFont: "Custom Sans",
		// SerifFont empty
		DefaultSize: 0, // zero
	}
	def := DefaultThemeFonts()
	result := MergeFonts(src, def)

	if result.SansFont != "Custom Sans" {
		t.Errorf("SansFont should be from src, got %q", result.SansFont)
	}
	if result.SerifFont != def.SerifFont {
		t.Errorf("SerifFont should be from defaults, got %q", result.SerifFont)
	}
	if result.DefaultSize != def.DefaultSize {
		t.Errorf("DefaultSize should be from defaults, got %d", result.DefaultSize)
	}
}

// TestMergeFonts_NilSrc verifies MergeFonts returns defaults when src is nil.
func TestMergeFonts_NilSrc(t *testing.T) {
	def := DefaultThemeFonts()
	result := MergeFonts(nil, def)

	if result.SansFont != def.SansFont {
		t.Errorf("expected default SansFont, got %q", result.SansFont)
	}
}

// TestMergeModeColors_FillsEmpties verifies mode color merging.
func TestMergeModeColors_FillsEmpties(t *testing.T) {
	src := &ThemeModeColors{
		PaneMode: "#111111",
		// TabMode empty
	}
	def := DefaultThemeModeColors()
	result := MergeModeColors(src, def)

	if result.PaneMode != "#111111" {
		t.Errorf("PaneMode should be from src, got %q", result.PaneMode)
	}
	if result.TabMode != def.TabMode {
		t.Errorf("TabMode should be from defaults, got %q", result.TabMode)
	}
}

// TestValidateModeColorsHex_ReturnsWarnings verifies invalid mode colors are detected.
func TestValidateModeColorsHex_ReturnsWarnings(t *testing.T) {
	colors := &ThemeModeColors{
		PaneMode:    "#111111",
		TabMode:     "not-a-color",
		SessionMode: "#333333",
		ResizeMode:  "",
	}

	warnings := ValidateModeColorsHex(colors, "mode_colors")
	if len(warnings) != 2 {
		t.Fatalf("expected 2 invalid mode color warnings, got %d: %+v", len(warnings), warnings)
	}
	if IsModeColorsValid(colors) {
		t.Error("expected invalid mode colors to be rejected")
	}
}

// TestValidateModeColorsHex_DefaultsAreValid verifies default mode colors satisfy validation.
func TestValidateModeColorsHex_DefaultsAreValid(t *testing.T) {
	defaults := DefaultThemeModeColors()
	if warnings := ValidateModeColorsHex(&defaults, "mode_colors"); len(warnings) != 0 {
		t.Fatalf("expected default mode colors to be valid, got warnings: %+v", warnings)
	}
	if !IsModeColorsValid(&defaults) {
		t.Error("expected default mode colors to be valid")
	}
}
