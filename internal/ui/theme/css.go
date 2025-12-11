// Package theme provides GTK CSS styling for UI components.
package theme

import (
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// tabBarCSS contains GTK4 CSS styling for the tab bar and tab buttons.
// These styles match the old gotk4 implementation.
const tabBarCSS = `
/* Tab bar styling */
.tab-bar {
	background-color: #2b2b2b;
	border-top: 2px solid #353535;
	padding: 0;
	min-height: 32px;
}

/* Tab button styling - matches stacked pane titles */
button.tab-button {
	background-color: #404040;
	background-image: none;
	border: none;
	border-right: 1px solid #353535;
	border-radius: 0;
	padding: 4px 8px;
	transition: background-color 200ms ease-in-out;
}

button.tab-button:hover {
	background-color: #505050;
}

button.tab-button.tab-button-active {
	background-color: #707070;
	font-weight: 600;
}

/* Tab title text */
.tab-title {
	font-size: 12px;
	color: #ffffff;
	font-weight: 500;
}
`

// ApplyCSS loads tab bar styling into the display.
func ApplyCSS(display *gdk.Display) {
	if display == nil {
		return
	}

	provider := gtk.NewCssProvider()
	if provider == nil {
		return
	}

	provider.LoadFromString(tabBarCSS)
	gtk.StyleContextAddProviderForDisplay(display, provider, uint(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION))
}
