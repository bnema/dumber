package theme

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateFavoritesSidebarCSS_ContainsCompactControlStyles(t *testing.T) {
	css := generateFavoritesSidebarCSS(DefaultDarkPalette())

	for _, selector := range []string{
		".favorites-sidebar-tags",
		".favorites-sidebar-tag-filter",
		".favorites-sidebar-tag-filter.suggested-action",
		".favorites-sidebar-tag-add",
		".favorites-sidebar-tag-prompt",
		".favorites-sidebar-tag-binding",
		".favorites-sidebar-shortcut-badge",
		".favorites-sidebar-tag-chip",
		".favorites-sidebar-form",
		".favorites-sidebar-form-tag-matches",
		".favorites-sidebar-form-tag-match",
	} {
		assert.Contains(t, css, selector+" {")
	}
	assert.Contains(t, css, "font-size: 0.66em;", "tags and shortcut badges stay compact")
	assert.Contains(t, css, "border-bottom: 0.0625em solid var(--border);", "filters are separated from the list")
	assert.Contains(t, css, "border-top: 0.0625em solid var(--border);", "the form is separated from the list")
}

func TestGenerateFavoritesSidebarCSS_UsesSharedThemeVariables(t *testing.T) {
	css := generateFavoritesSidebarCSS(DefaultDarkPalette())

	assert.Contains(t, css, "var(--surface)")
	assert.Contains(t, css, "var(--surface-variant)")
	assert.Contains(t, css, "var(--accent)")
	assert.NotContains(t, css, DefaultDarkPalette().Accent)
	assert.Equal(t, css, generateFavoritesSidebarCSS(DefaultLightPalette()))
}

func TestGenerateFavoritesSidebarCSS_IsRegisteredWithTheFullTheme(t *testing.T) {
	css := GenerateCSSFull(DefaultDarkPalette(), 1, DefaultFontConfig(), DefaultModeColors())

	assert.Contains(t, css, "Native Sidebar Foundation")
	assert.Contains(t, css, "Favorites Sidebar Styling")
	assert.Contains(t, css, ".favorites-sidebar-tag-filter {")
	assert.Contains(t, css, ".favorites-sidebar-form {")
	assert.Equal(t, strings.Count(css, "{"), strings.Count(css, "}"))
}
