package systemviews

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderAppFrame_ReturnsFragment(t *testing.T) {
	t.Parallel()

	page := renderedPage{
		route:    RouteHistory,
		subtitle: "History",
		body:     `<ul><li>hello</li></ul>`,
	}
	theme := shellTheme{RootClass: "sv-dark"}

	out := renderAppFrame(page, theme)

	// Must be a fragment — no full document tags.
	assert.NotContains(t, out, "<html")
	assert.NotContains(t, out, "<head")
	assert.NotContains(t, out, "<body")

	// Root div carries theme class.
	assert.Contains(t, out, `class="sv-app sv-dark"`)

	// Route is on the root element.
	assert.Contains(t, out, `data-route="history"`)

	// Shell wrapper present.
	assert.Contains(t, out, `class="sv-shell"`)

	// Body content passed through.
	assert.Contains(t, out, `<ul><li>hello</li></ul>`)

	// Subtitle rendered.
	assert.Contains(t, out, "History")
}

func TestRenderAppFrame_DefaultThemeClass(t *testing.T) {
	t.Parallel()

	page := renderedPage{
		route:    RouteFavorites,
		subtitle: "Favorites",
		body:     `<p>content</p>`,
	}
	// Empty theme — should still get sv-app with sv-dark default.
	out := renderAppFrame(page, shellTheme{})

	assert.Contains(t, out, `class="sv-app sv-dark"`)
	assert.Contains(t, out, `data-route="favorites"`)
}

func TestRenderAppFrame_InlineVarsApplied(t *testing.T) {
	t.Parallel()

	page := renderedPage{
		route:    RouteConfig,
		subtitle: "Config",
		body:     `<p>settings</p>`,
	}
	theme := shellTheme{
		RootClass:  "sv-light",
		InlineVars: "--sv-background: #fff;",
	}

	out := renderAppFrame(page, theme)

	assert.Contains(t, out, `style="--sv-background: #fff;"`)
	assert.Contains(t, out, `class="sv-app sv-light"`)
}

func TestListHTMLWrapsRows(t *testing.T) {
	t.Parallel()

	out := listHTML(`<li class="sv-list-row">entry</li>`, "Nothing here")

	assert.Contains(t, out, `class="sv-list"`)
	assert.Contains(t, out, `class="sv-list-row"`)
	assert.Contains(t, out, "entry")
}

func TestListHTMLEmptyState(t *testing.T) {
	t.Parallel()

	out := listHTML("", "Nothing here")

	assert.Contains(t, out, `class="sv-empty"`)
	assert.Contains(t, out, "Nothing here")
}

func TestLinkHTMLSanitizesHref(t *testing.T) {
	t.Parallel()

	out := linkHTML("javascript:alert(1)", "Bad")

	assert.Contains(t, out, `class="sv-link"`)
	assert.Contains(t, out, `href="#"`)
	assert.Contains(t, out, "Bad")
}

func TestKVHTMLRendersPairs(t *testing.T) {
	t.Parallel()

	out := kvHTML([]kvPair{{Label: "Engine", Value: "webkit"}})

	assert.Contains(t, out, `class="sv-kv"`)
	assert.Contains(t, out, "Engine")
	assert.Contains(t, out, "webkit")
}

func TestSectionHTML(t *testing.T) {
	t.Parallel()

	out := sectionHTML("test-section", "Test Title", "<p>inner</p>")
	assert.Contains(t, out, `class="sv-section test-section"`)
	assert.Contains(t, out, "Test Title")
	assert.Contains(t, out, "<p>inner</p>")
}

func TestEmptyStateHTML(t *testing.T) {
	t.Parallel()

	out := emptyStateHTML("Nothing here")
	assert.Contains(t, out, `class="sv-empty"`)
	assert.Contains(t, out, "Nothing here")
}
