package colorscheme

import (
	"os/exec"
	"strings"
)

const (
	detectorNameGsettings = "gsettings"
	priorityGsettings     = 10
)

// GsettingsDetector detects color scheme from GNOME gsettings.
// This is the most reliable method for GNOME-based desktops.
type GsettingsDetector struct{}

// NewGsettingsDetector creates a new gsettings-based detector.
func NewGsettingsDetector() *GsettingsDetector {
	return &GsettingsDetector{}
}

// Name implements port.ColorSchemeDetector.
func (*GsettingsDetector) Name() string {
	return detectorNameGsettings
}

// Priority implements port.ColorSchemeDetector.
func (*GsettingsDetector) Priority() int {
	return priorityGsettings
}

// Available implements port.ColorSchemeDetector.
// Returns true if gsettings command is available.
func (*GsettingsDetector) Available() bool {
	_, err := exec.LookPath("gsettings")
	return err == nil
}

// Detect implements port.ColorSchemeDetector.
// Queries org.gnome.desktop.interface color-scheme.
func (*GsettingsDetector) Detect() (prefersDark, ok bool) {
	cmd := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme")
	output, err := cmd.Output()
	if err != nil {
		return false, false
	}

	// Output is like "'prefer-dark'\n", strip quotes and whitespace
	result := strings.TrimSpace(string(output))
	result = strings.Trim(result, "'\"")

	switch result {
	case "prefer-dark":
		return true, true
	case "prefer-light":
		return false, true
	case "default":
		// "default" means follow system, which we can't determine here
		return false, false
	default:
		return false, false
	}
}
