package theme

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateCSS_StandaloneOmniboxWindowUsesTransparentBackground(t *testing.T) {
	css := GenerateCSS(DefaultDarkPalette())

	if !strings.Contains(css, "window.standalone-omnibox-window") {
		t.Fatalf("expected standalone omnibox window selector in CSS, got %q", css)
	}
	re := regexp.MustCompile(`window\.standalone-omnibox-window\s*\{\s*background-color:\s*transparent\s*;?\s*\}`)
	if !re.MatchString(css) {
		t.Fatalf("expected standalone omnibox window background to be transparent")
	}
}

func TestGenerateCSS_OmniboxHeaderIsOpaque(t *testing.T) {
	css := GenerateCSS(DefaultDarkPalette())

	if !strings.Contains(css, ".omnibox-header {") {
		t.Fatalf("expected omnibox header selector in CSS")
	}
	if !strings.Contains(css, "background-color: var(--surface);") {
		t.Fatalf("expected omnibox header to use an opaque surface background")
	}
	if !strings.Contains(css, "background-image: none;") {
		t.Fatalf("expected omnibox header gradient to be disabled")
	}
}

func TestGenerateCSS_OmniboxHeaderBadgeSelectorsExist(t *testing.T) {
	css := GenerateCSS(DefaultDarkPalette())

	if !strings.Contains(css, ".omnibox-header-badge {") {
		t.Fatalf("expected omnibox header badge selector in CSS")
	}
	if !strings.Contains(css, ".omnibox-header-badge:hover {") {
		t.Fatalf("expected omnibox header badge hover selector in CSS")
	}
}

func TestGenerateCSS_OmniboxHeaderBadgeUsesPassiveNeutralChipStyling(t *testing.T) {
	css := GenerateCSS(DefaultDarkPalette())

	badgeRe := regexp.MustCompile(`(?s)\.omnibox-header-badge\s*\{[^}]*background-color:\s*alpha\(var\(--surface-variant\),\s*0\.65\);[^}]*color:\s*var\(--muted\);[^}]*border:\s*0\.0625em solid alpha\(var\(--border\),\s*0\.85\);[^}]*border-radius:\s*0\.1875em;[^}]*padding:\s*0\.0625em 0\.375em;[^}]*font-size:\s*0\.6875em;`)
	if !badgeRe.MatchString(css) {
		t.Fatalf("expected omnibox header badge to use passive neutral chip styling")
	}

	hoverRe := regexp.MustCompile(`(?s)\.omnibox-header-badge:hover\s*\{[^}]*background-color:\s*shade\(var\(--surface-variant\),\s*1\.08\);[^}]*border-color:\s*var\(--border\);[^}]*color:\s*var\(--text\);`)
	if !hoverRe.MatchString(css) {
		t.Fatalf("expected omnibox header badge hover styling to stay neutral while hinting interactivity")
	}

	badgeBlock := regexp.MustCompile(`(?s)\.omnibox-header-badge\s*\{[^}]*\}`).FindString(css)
	if strings.Contains(badgeBlock, "background-color: alpha(var(--accent)") {
		t.Fatalf("expected omnibox header badge resting state to avoid accent-based styling")
	}
	if strings.Contains(badgeBlock, "color: var(--accent)") {
		t.Fatalf("expected omnibox header badge resting text to avoid accent color")
	}
}

func TestGenerateCSS_FavoriteRowsOnlyTintTrailingAffordance(t *testing.T) {
	css := GenerateCSS(DefaultDarkPalette())

	if !strings.Contains(css, ".omnibox-row.omnibox-row-favorite {") {
		t.Fatalf("expected favorite row selector in CSS")
	}
	if !strings.Contains(css, "border-left: 0.1875em solid var(--warning);") {
		t.Fatalf("expected favorite rows to mark the left edge with the warning color")
	}
	if strings.Contains(css, "border-right: 0.125em solid alpha(var(--warning), 0.55);") {
		t.Fatalf("expected favorite indicator to move off the right edge")
	}
	if !strings.Contains(css, "color: mix(var(--warning), var(--muted), 0.45);") {
		t.Fatalf("expected favorite star color to be softened")
	}
	if strings.Contains(css, ".omnibox-row.omnibox-row-favorite .omnibox-favorite-star-slot") {
		t.Fatalf("expected favorite highlight to move off the star slot")
	}
}

func TestGenerateCSS_OmniboxSearchAreaMatchesHeaderSurface(t *testing.T) {
	css := GenerateCSS(DefaultDarkPalette())

	containerRe := regexp.MustCompile(`(?s)\.omnibox-container\s*\{[^}]*background-color:\s*var\(--surface\);`)
	if !containerRe.MatchString(css) {
		t.Fatalf("expected omnibox container to use the header surface color")
	}
	entryRe := regexp.MustCompile(`(?s)entry\.omnibox-entry\s*,\s*entry\.omnibox-entry\s*>\s*text\s*\{[^}]*background-color:\s*alpha\(var\(--bg\),\s*0\.88\);[^}]*background-image:\s*none;`)
	if !entryRe.MatchString(css) {
		t.Fatalf("expected omnibox entry and its text node to use the darker background fill")
	}
	focusedEntryRe := regexp.MustCompile(`(?s)entry\.omnibox-entry:focus[^\{]*,\s*entry\.omnibox-entry:focus-within[^\{]*,\s*entry\.omnibox-entry:focus-visible[^\{]*,\s*entry\.omnibox-entry:focus\s*>\s*text[^\{]*,\s*entry\.omnibox-entry:focus-within\s*>\s*text[^\{]*,\s*entry\.omnibox-entry:focus-visible\s*>\s*text\s*\{[^}]*background-color:\s*shade\(var\(--bg\),\s*1\.05\);`)
	if !focusedEntryRe.MatchString(css) {
		t.Fatalf("expected focused omnibox entry text node to keep the darker bg-based fill")
	}
	scrolledRe := regexp.MustCompile(`(?s)\.omnibox-scrolled\s*\{[^}]*background-color:\s*var\(--surface\);`)
	if !scrolledRe.MatchString(css) {
		t.Fatalf("expected omnibox results area to use the header surface color")
	}
}

func TestGenerateCSSWithScale_DoesNotEmitRootTextScaling(t *testing.T) {
	css := GenerateCSSWithScale(DefaultDarkPalette(), 1.3)

	assert.NotContains(t, css, "/* UI Scaling (130%) */")
	assert.NotContains(t, css, "window {\n\tfont-size:")
}

func TestGenerateCSSWithScale_DoesNotEmitLineHeightOverrides(t *testing.T) {
	css := GenerateCSSWithScale(DefaultDarkPalette(), 1.3)

	assert.NotContains(t, css, "line-height:")
	assert.NotContains(t, css, "label,\nentry,\nentry > text {\n\tline-height:")
}

func TestGenerateCSSWithScale_UsesOpaqueOmniboxRowBaseBackground(t *testing.T) {
	css := GenerateCSSWithScale(DefaultDarkPalette(), 1.3)

	assert.Contains(t, css, ".omnibox-row {")
	assert.Contains(t, css, "background-color: var(--surface);")
	assert.NotContains(t, css, ".omnibox-row {\n\tpadding: 0.5em 0.75em;\n\tmargin: 0;\n\tborder-radius: 0;\n\tborder-left: 0.1875em solid transparent;\n\tborder-bottom: 0.0625em solid alpha(var(--border), 0.35);\n\tbackground-color: transparent;")
}

func TestGenerateCSSWithScale_UsesMediumWeightForOmniboxSuggestionTitle(t *testing.T) {
	css := GenerateCSSWithScale(DefaultDarkPalette(), 1.3)

	assert.Contains(t, css, ".omnibox-suggestion-title {")
	assert.Contains(t, css, "font-weight: 500;")
	assert.NotContains(t, css, ".omnibox-suggestion-title {\n\tfont-size: 0.875em;\n\tcolor: var(--text);\n\tfont-weight: 400;")
}

func TestGenerateCSS_DoesNotEmitUnsupportedGTKProperties(t *testing.T) {
	css := GenerateCSS(DefaultDarkPalette())

	assert.NotContains(t, css, "pointer-events:")
	assert.NotContains(t, css, "text-align:")
}
