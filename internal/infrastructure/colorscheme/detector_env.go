package colorscheme

import (
	"os"
	"strings"
)

const (
	detectorNameEnv = "GTK_THEME"
	priorityEnv     = 20
)

// EnvDetector detects color scheme from GTK_THEME environment variable.
// This is useful when users explicitly set their theme via environment.
type EnvDetector struct{}

// NewEnvDetector creates a new environment variable-based detector.
func NewEnvDetector() *EnvDetector {
	return &EnvDetector{}
}

// Name implements port.ColorSchemeDetector.
func (*EnvDetector) Name() string {
	return detectorNameEnv
}

// Priority implements port.ColorSchemeDetector.
func (*EnvDetector) Priority() int {
	return priorityEnv
}

// Available implements port.ColorSchemeDetector.
// Returns true if GTK_THEME environment variable is set.
func (*EnvDetector) Available() bool {
	return os.Getenv("GTK_THEME") != ""
}

// Detect implements port.ColorSchemeDetector.
// Checks if GTK_THEME contains "dark" (case-insensitive).
func (*EnvDetector) Detect() (prefersDark, ok bool) {
	gtkTheme := os.Getenv("GTK_THEME")
	if gtkTheme == "" {
		return false, false
	}

	// If GTK_THEME contains "dark", prefer dark mode
	// Otherwise, assume light mode
	prefersDark = strings.Contains(strings.ToLower(gtkTheme), "dark")
	return prefersDark, true
}
