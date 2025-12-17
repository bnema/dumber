package theme

import (
	"os"
	"os/exec"
	"strings"

	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// DetectSystemDarkMode checks if the system prefers dark mode.
// It checks multiple sources in order of preference:
// 1. GTK_THEME environment variable (contains "dark")
// 2. GNOME gsettings color-scheme (prefer-dark/prefer-light)
// 3. GTK Settings gtk-application-prefer-dark-theme property
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

	// Check GNOME gsettings color-scheme (modern GNOME/GTK4 method)
	if colorScheme := getGsettingsColorScheme(); colorScheme != "" {
		switch colorScheme {
		case "prefer-dark":
			return true
		case "prefer-light":
			return false
			// "default" falls through to next check
		}
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

// getGsettingsColorScheme queries GNOME gsettings for the color-scheme preference.
// Returns "prefer-dark", "prefer-light", "default", or empty string on error.
func getGsettingsColorScheme() string {
	cmd := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	// Output is like "'prefer-dark'\n", strip quotes and whitespace
	result := strings.TrimSpace(string(output))
	result = strings.Trim(result, "'\"")
	return result
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
