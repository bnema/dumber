package theme

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateHistorySidebarCSS_ContainsExpectedSelectors(t *testing.T) {
	css := generateHistorySidebarCSS(DefaultDarkPalette())

	selectors := []string{
		".history-sidebar-outer",
		".history-sidebar-search-box",
		".history-sidebar-search",
		".history-sidebar-groups",
		".history-sidebar-group-header",
		".history-sidebar-row",
		".history-sidebar-row:hover",
		".history-sidebar-row:selected",
		".history-sidebar-row:focus",
		".history-sidebar-row-title",
		".history-sidebar-row-subtitle",
		".history-sidebar-row-time",
		".history-sidebar-empty",
		".history-sidebar-loading",
	}

	for _, sel := range selectors {
		assert.Contains(t, css, sel+" {",
			"expected selector %s { in generated CSS", sel)
	}
}

func TestGenerateHistorySidebarCSS_AccentAlphaInterpolation(t *testing.T) {
	darkPalette := DefaultDarkPalette()
	css := generateHistorySidebarCSS(darkPalette)

	expectedAlpha := fmt.Sprintf("alpha(%s, 0.18)", darkPalette.Accent)
	assert.Contains(t, css, expectedAlpha,
		"expected accent alpha value %q in generated CSS", expectedAlpha)

	// The accent alpha should appear in hover, selected, and focus blocks
	assert.GreaterOrEqual(t, strings.Count(css, expectedAlpha), 3,
		"expected accent alpha to appear at least 3 times (hover/selected/focus)")
}

func TestGenerateHistorySidebarCSS_DeterministicOutput(t *testing.T) {
	palette := DefaultDarkPalette()

	css1 := generateHistorySidebarCSS(palette)
	css2 := generateHistorySidebarCSS(palette)

	hash1 := sha256.Sum256([]byte(css1))
	hash2 := sha256.Sum256([]byte(css2))

	assert.Equal(t, hash1, hash2, "CSS output must be deterministic for the same palette")
}

func TestGenerateHistorySidebarCSS_DifferentPalettesProduceDifferentCSS(t *testing.T) {
	darkCSS := generateHistorySidebarCSS(DefaultDarkPalette())
	lightCSS := generateHistorySidebarCSS(DefaultLightPalette())

	hashDark := sha256.Sum256([]byte(darkCSS))
	hashLight := sha256.Sum256([]byte(lightCSS))

	assert.NotEqual(t, hashDark, hashLight,
		"dark and light palettes should produce different CSS")
}

func TestGenerateHistorySidebarCSS_LightPaletteContainsAccentAlpha(t *testing.T) {
	lightPalette := DefaultLightPalette()
	css := generateHistorySidebarCSS(lightPalette)

	expectedAlpha := fmt.Sprintf("alpha(%s, 0.18)", lightPalette.Accent)
	assert.Contains(t, css, expectedAlpha,
		"expected light palette accent alpha value %q in generated CSS", expectedAlpha)
}

func TestGenerateHistorySidebarCSS_ThroughGenerateCSS(t *testing.T) {
	fullCSS := GenerateCSS(DefaultDarkPalette())

	assert.Contains(t, fullCSS, ".history-sidebar-outer {",
		"full CSS must contain history sidebar selectors")
	assert.Contains(t, fullCSS, ".history-sidebar-group-header {",
		"full CSS must contain group header selector")
	assert.Contains(t, fullCSS, ".history-sidebar-row-title {",
		"full CSS must contain row title selector")
	assert.Contains(t, fullCSS, "History Sidebar Styling",
		"full CSS must contain the history sidebar comment marker")
}

func TestGenerateHistorySidebarCSS_NoEmptyCSSBlocks(t *testing.T) {
	css := generateHistorySidebarCSS(DefaultDarkPalette())

	sections := strings.Split(css, "{")
	for i, section := range sections {
		if i == 0 {
			continue
		}
		closeIdx := strings.Index(section, "}")
		if closeIdx < 0 {
			continue
		}
		blockContent := strings.TrimSpace(section[:closeIdx])
		assert.NotEmpty(t, blockContent, "CSS block must not be empty: section %d", i)
	}
}

func TestGenerateHistorySidebarCSS_CustomPaletteValuesInterpolated(t *testing.T) {
	customPalette := Palette{
		Background:     "#111111",
		Surface:        "#222222",
		SurfaceVariant: "#333333",
		Text:           "#eeeeee",
		Muted:          "#aaaaaa",
		Accent:         "#ff6600",
		Border:         "#444444",
		Success:        "#00cc44",
		Warning:        "#ffaa00",
		Destructive:    "#cc2222",
	}

	css := generateHistorySidebarCSS(customPalette)

	assert.Contains(t, css, "alpha(#ff6600, 0.18)",
		"custom accent should be interpolated into CSS")

	assert.Contains(t, css, ".history-sidebar-outer {")
	assert.Contains(t, css, ".history-sidebar-row-title {")
	assert.Contains(t, css, ".history-sidebar-row-subtitle {")
	assert.Contains(t, css, ".history-sidebar-row-time {")
	assert.Contains(t, css, ".history-sidebar-empty {")
}

func TestGenerateHistorySidebarCSS_ContainsTransition(t *testing.T) {
	css := generateHistorySidebarCSS(DefaultDarkPalette())

	assert.Contains(t, css, "transition:",
		"CSS should include transition property for smooth hover effects")
	assert.Contains(t, css, "background-color 100ms ease",
		"row transition should specify background-color 100ms ease")
}

func TestGenerateHistorySidebarCSS_ContainsUppercaseGroupHeader(t *testing.T) {
	css := generateHistorySidebarCSS(DefaultDarkPalette())

	assert.Contains(t, css, "text-transform: uppercase;",
		"group header should use text-transform uppercase")
	assert.Contains(t, css, "letter-spacing: 0.04em;",
		"group header should have letter-spacing")
}

func TestGenerateHistorySidebarCSS_InGenerateCSSFull_DarkPalette(t *testing.T) {
	fullCSS := GenerateCSSFull(DefaultDarkPalette(), 1.0, DefaultFontConfig(), DefaultModeColors())

	assert.Contains(t, fullCSS, ".history-sidebar-outer {",
		"GenerateCSSFull must include history sidebar outer selector for dark palette")
	assert.Contains(t, fullCSS, ".history-sidebar-row-title {",
		"GenerateCSSFull must include history sidebar row title selector for dark palette")
	assert.Contains(t, fullCSS, ".history-sidebar-search {",
		"GenerateCSSFull must include history sidebar search selector for dark palette")
	assert.Contains(t, fullCSS, ".history-sidebar-empty {",
		"GenerateCSSFull must include history sidebar empty selector for dark palette")
}

func TestGenerateHistorySidebarCSS_InGenerateCSSFull_LightPalette(t *testing.T) {
	fullCSS := GenerateCSSFull(DefaultLightPalette(), 1.0, DefaultFontConfig(), DefaultModeColors())

	assert.Contains(t, fullCSS, ".history-sidebar-outer {",
		"GenerateCSSFull must include history sidebar outer selector for light palette")
	assert.Contains(t, fullCSS, ".history-sidebar-row-title {",
		"GenerateCSSFull must include history sidebar row title selector for light palette")
	assert.Contains(t, fullCSS, ".history-sidebar-search {",
		"GenerateCSSFull must include history sidebar search selector for light palette")
	assert.Contains(t, fullCSS, ".history-sidebar-empty {",
		"GenerateCSSFull must include history sidebar empty selector for light palette")
}

func TestGenerateHistorySidebarCSS_LiveReloadSeamIncludesUpdatedCSS(t *testing.T) {
	// GenerateCSSFull is the live theme reload entry point called when
	// the palette changes at runtime. This test proves that switching
	// palettes produces different history sidebar CSS, confirming the
	// full generation path includes and re-generates the sidebar styles.
	darkCSS := GenerateCSSFull(DefaultDarkPalette(), 1.0, DefaultFontConfig(), DefaultModeColors())
	lightCSS := GenerateCSSFull(DefaultLightPalette(), 1.0, DefaultFontConfig(), DefaultModeColors())

	// Both must contain history sidebar selectors.
	assert.Contains(t, darkCSS, ".history-sidebar-outer {", "dark CSS must have sidebar styles")
	assert.Contains(t, lightCSS, ".history-sidebar-outer {", "light CSS must have sidebar styles")

	// The CSS must differ between palettes (different color values).
	if darkCSS == lightCSS {
		t.Fatal("dark and light palette should produce different CSS when history sidebar is included")
	}

	// The dark and light outputs must both be parseable: each '{' has '}'.
	assert.Equal(t, strings.Count(darkCSS, "{"), strings.Count(darkCSS, "}"),
		"dark CSS braces must be balanced")
	assert.Equal(t, strings.Count(lightCSS, "{"), strings.Count(lightCSS, "}"),
		"light CSS braces must be balanced")
}

func TestGenerateHistorySidebarCSS_InGenerateCSSFull_WithCustomScaleAndFonts(t *testing.T) {
	customFonts := FontConfig{
		SansFont:      "Noto Sans",
		MonospaceFont: "JetBrains Mono",
		GtkFont:       "Noto Sans",
	}
	customModeColors := ModeColors{
		PaneMode:    "#ff0000",
		TabMode:     "#00ff00",
		SessionMode: "#0000ff",
		ResizeMode:  "#ffff00",
	}

	fullCSS := GenerateCSSFull(DefaultDarkPalette(), 2.0, customFonts, customModeColors)

	assert.Contains(t, fullCSS, ".history-sidebar-outer {",
		"GenerateCSSFull with custom scale/fonts must include history sidebar selectors")
	assert.Contains(t, fullCSS, ".history-sidebar-row:hover {",
		"GenerateCSSFull must include hover state for history rows")
	assert.Contains(t, fullCSS, ".history-sidebar-row:selected {",
		"GenerateCSSFull must include selected state for history rows")
	assert.Contains(t, fullCSS, ".history-sidebar-row:focus {",
		"GenerateCSSFull must include focus state for history rows")
}

// TestGenerateHistorySidebarCSS_LiveReloadFullPath verifies that the
// GenerateCSSFull entry point (used by live theme reload) includes the
// history sidebar CSS with correct palette values. This is the path that
// would be called when the user changes themes at runtime.
func TestGenerateHistorySidebarCSS_LiveReloadFullPath(t *testing.T) {
	darkPalette := DefaultDarkPalette()
	lightPalette := DefaultLightPalette()
	customPalette := Palette{
		Background:     "#111111",
		Surface:        "#222222",
		SurfaceVariant: "#333333",
		Text:           "#eeeeee",
		Muted:          "#aaaaaa",
		Accent:         "#ff6600",
		Border:         "#444444",
		Success:        "#00cc44",
		Warning:        "#ffaa00",
		Destructive:    "#cc2222",
	}

	// GenerateCSSFull is the exact function called by the live theme reload
	// path. It generates ALL CSS including the history sidebar selectors.
	darkCSS := GenerateCSSFull(darkPalette, 1.0, DefaultFontConfig(), DefaultModeColors())
	lightCSS := GenerateCSSFull(lightPalette, 1.0, DefaultFontConfig(), DefaultModeColors())
	customCSS := GenerateCSSFull(customPalette, 1.0, DefaultFontConfig(), DefaultModeColors())

	// All three must contain the full set of history sidebar selectors.
	sidebarSelectors := []string{
		".history-sidebar-outer",
		".history-sidebar-search-box",
		".history-sidebar-search",
		".history-sidebar-groups",
		".history-sidebar-group-header",
		".history-sidebar-row",
		".history-sidebar-row:hover",
		".history-sidebar-row:selected",
		".history-sidebar-row:focus",
		".history-sidebar-row-title",
		".history-sidebar-row-subtitle",
		".history-sidebar-row-time",
		".history-sidebar-empty",
		".history-sidebar-loading",
	}

	for _, sel := range sidebarSelectors {
		assert.Contains(t, darkCSS, sel+" {", "dark CSS must have selector %s", sel)
		assert.Contains(t, lightCSS, sel+" {", "light CSS must have selector %s", sel)
		assert.Contains(t, customCSS, sel+" {", "custom CSS must have selector %s", sel)
	}

	// Palette-specific values must differ.
	assert.NotEqual(t, darkCSS, lightCSS, "dark and light GenerateCSSFull output must differ")
	assert.NotEqual(t, darkCSS, customCSS, "dark and custom GenerateCSSFull output must differ")
	assert.NotEqual(t, lightCSS, customCSS, "light and custom GenerateCSSFull output must differ")

	// The accent alpha interpolation must use each palette's accent.
	assert.Contains(t, darkCSS, fmt.Sprintf("alpha(%s, 0.18)", darkPalette.Accent),
		"dark CSS must contain dark palette accent alpha")
	assert.Contains(t, lightCSS, fmt.Sprintf("alpha(%s, 0.18)", lightPalette.Accent),
		"light CSS must contain light palette accent alpha")
	assert.Contains(t, customCSS, fmt.Sprintf("alpha(%s, 0.18)", customPalette.Accent),
		"custom CSS must contain custom palette accent alpha")

	// All three outputs must be syntactically valid: every '{' has a matching '}'.
	for name, css := range map[string]string{
		"dark":   darkCSS,
		"light":  lightCSS,
		"custom": customCSS,
	} {
		assert.Equal(t, strings.Count(css, "{"), strings.Count(css, "}"),
			"%s CSS braces must be balanced", name)
	}
}

// TestGenerateHistorySidebarCSS_LiveReloadPaletteSwitch verifies that
// switching palettes at GenerateCSSFull level updates the history sidebar
// accent values. This simulates what happens when the theme system applies
// a new palette at runtime: the full CSS (including sidebar) is regenerated.
func TestGenerateHistorySidebarCSS_LiveReloadPaletteSwitch(t *testing.T) {
	// Start with dark palette.
	current := DefaultDarkPalette()
	cssBefore := GenerateCSSFull(current, 1.0, DefaultFontConfig(), DefaultModeColors())

	// Verify the initial dark accent is in the sidebar sections.
	assert.Contains(t, cssBefore, fmt.Sprintf("alpha(%s, 0.18)", current.Accent),
		"initial dark palette accent must appear in generated CSS")

	// Switch to light palette (simulating runtime theme change).
	current = DefaultLightPalette()
	cssAfter := GenerateCSSFull(current, 1.0, DefaultFontConfig(), DefaultModeColors())

	// Verify the light accent replaces the dark one.
	assert.Contains(t, cssAfter, fmt.Sprintf("alpha(%s, 0.18)", current.Accent),
		"light palette accent must appear in CSS after switch")

	// The dark accent value must NOT appear in the light CSS.
	darkAccent := DefaultDarkPalette().Accent
	if current.Accent != darkAccent {
		assert.NotContains(t, cssAfter, fmt.Sprintf("alpha(%s, 0.18)", darkAccent),
			"dark accent must not remain in CSS after switching to light palette")
	}

	// History sidebar selectors must be present in both.
	assert.Contains(t, cssBefore, ".history-sidebar-row:selected {")
	assert.Contains(t, cssAfter, ".history-sidebar-row:selected {")

	// The before and after CSS must be different (palette changed).
	assert.NotEqual(t, cssBefore, cssAfter, "CSS must change when palette switches")
}
