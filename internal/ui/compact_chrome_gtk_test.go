package ui

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/theme"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func measuredHeight(widget *gtk.Widget) int {
	var minimum, natural int
	widget.Measure(gtk.OrientationVerticalValue, -1, &minimum, &natural, nil, nil)
	return minimum
}

// TestGTKCompactChromeHeightsMatchMeasuredTabBar checks, with real GTK
// measurement, that the content inset equals the tab bar footprint and that
// tab buttons and stacked titlebars share one height at every font size.
func TestGTKCompactChromeHeightsMatchMeasuredTabBar(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	display := gdk.DisplayGetDefault()
	provider := gtk.NewCssProvider()
	require.NotNil(t, provider)
	defer provider.Unref()
	provider.LoadFromString(theme.GenerateCSS(theme.DefaultDarkPalette()))
	gtk.StyleContextAddProviderForDisplay(display, provider, uint(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION))
	defer gtk.StyleContextRemoveProviderForDisplay(display, provider)

	settings := gtk.SettingsGetForDisplay(display)
	require.NotNil(t, settings)
	defer settings.Unref()
	originalFont := settings.GetPropertyGtkFontName()
	defer settings.SetPropertyGtkFontName(originalFont)

	barHeights := map[string]map[string]int{"top": {}, "bottom": {}}
	for _, font := range []string{"Sans 9", "Sans 11", "Sans 20", "Sans 32"} {
		for _, position := range []string{"top", "bottom"} {
			// Widgets resolve their style when created, so build them per font.
			settings.SetPropertyGtkFontName(font)
			mw, err := window.New(context.Background(), gtkApp, position)
			require.NoError(t, err)

			barWidget := mw.TabBar().Widget()
			assert.True(t, barWidget.HasCssClass("tab-bar-"+position), "%s %s position class", font, position)
			mw.SetTabBarContentInsetVisible(true)
			emptyBar := measuredHeight(barWidget)
			assert.Equal(t, emptyBar, measuredHeight(mw.TabBarContentInset()), "%s %s empty bar inset", font, position)

			mw.TabBar().AddTab(entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(entity.PaneID("pane-1"))))
			bar := measuredHeight(barWidget)
			button := measuredHeight(barWidget.GetFirstChild())
			assert.Equal(t, bar, measuredHeight(mw.TabBarContentInset()), "%s %s inset", font, position)
			assert.Equal(t, emptyBar, bar, "%s %s empty and filled bar", font, position)
			assert.Equal(t, button, bar, "%s %s bar wraps its tab buttons", font, position)

			// Auto-hide keeps the inset height stable.
			mw.TabBar().SetAutoHidden(true)
			assert.Equal(t, bar, measuredHeight(mw.TabBarContentInset()), "%s %s auto-hidden inset", font, position)

			stack := layout.NewStackedView(layout.NewGtkWidgetFactory())
			stack.AddPane(context.Background(), "pane-1", "Title", "", nil)
			stack.AddPane(context.Background(), "pane-2", "Title", "", nil)
			mw.SetContent(stack.Widget().GtkWidget())
			titlebar := stack.Box().GtkWidget().GetFirstChild()
			require.NotNil(t, titlebar)
			assert.Equal(t, bar, measuredHeight(titlebar), "%s %s stacked titlebar matches tab bar", font, position)
			barHeights[position][font] = bar

			// Detach the stack and drop the box reference it owns.
			mw.SetContent(nil)
			stack.Box().GtkWidget().Unref()
			mw.Destroy()
		}
	}
	// Both positions share one row height; small fonts keep the compact
	// floor and large fonts grow the shared row.
	assert.Equal(t, barHeights["top"], barHeights["bottom"])
	heights := barHeights["top"]
	assert.Equal(t, heights["Sans 9"], heights["Sans 11"])
	assert.Greater(t, heights["Sans 20"], heights["Sans 11"])
	assert.Greater(t, heights["Sans 32"], heights["Sans 20"])
	t.Logf("measured tab bar heights: %v", barHeights)
}
