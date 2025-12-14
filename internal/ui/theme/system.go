package theme

import (
	"os"
	"strings"

	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// DetectSystemDarkMode checks if the system prefers dark mode.
// It checks multiple sources in order of preference:
// 1. GTK_THEME environment variable (contains "dark")
// 2. GTK Settings gtk-application-prefer-dark-theme property
// Returns true if dark mode is preferred.
func DetectSystemDarkMode() bool {
	// Check GTK_THEME environment variable first
	gtkTheme := os.Getenv("GTK_THEME")
	if gtkTheme != "" {
		if strings.Contains(strings.ToLower(gtkTheme), "dark") {
			return true
		}
		// If GTK_THEME is set but doesn't contain "dark", it's likely light
		return false
	}

	// Try GTK Settings
	settings := gtk.SettingsGetDefault()
	if settings != nil {
		// gtk-application-prefer-dark-theme is a boolean property
		preferDark := settings.GetPropertyGtkApplicationPreferDarkTheme()
		return preferDark
	}

	// Default to dark mode if detection fails (better for eyes)
	return true
}

// ResolveColorScheme determines the effective dark mode preference.
// It takes the config value ("prefer-dark", "prefer-light", "default")
// and resolves "default" to the system preference.
func ResolveColorScheme(configScheme string) bool {
	switch strings.ToLower(configScheme) {
	case "prefer-dark", "dark":
		return true
	case "prefer-light", "light":
		return false
	default:
		// "default" or empty - follow system
		return DetectSystemDarkMode()
	}
}
