package colorscheme

import (
	"sync/atomic"

	"github.com/jwijenbergh/puregotk/v4/adw"
)

const (
	detectorNameAdwaita = "libadwaita"
	priorityAdwaita     = 100
)

// AdwaitaDetector detects color scheme from libadwaita's StyleManager.
// This is the most accurate detector after adw.Init() has been called,
// as it reflects the same state that WebKit uses for prefers-color-scheme.
type AdwaitaDetector struct {
	available atomic.Bool
}

// NewAdwaitaDetector creates a new libadwaita-based detector.
// Initially unavailable until MarkAvailable() is called after adw.Init().
func NewAdwaitaDetector() *AdwaitaDetector {
	return &AdwaitaDetector{}
}

// Name implements port.ColorSchemeDetector.
func (*AdwaitaDetector) Name() string {
	return detectorNameAdwaita
}

// Priority implements port.ColorSchemeDetector.
func (*AdwaitaDetector) Priority() int {
	return priorityAdwaita
}

// Available implements port.ColorSchemeDetector.
// Returns true only after MarkAvailable() has been called.
func (d *AdwaitaDetector) Available() bool {
	return d.available.Load()
}

// MarkAvailable should be called after adw.Init() completes.
// This enables the detector to query StyleManager.
func (d *AdwaitaDetector) MarkAvailable() {
	d.available.Store(true)
}

// Detect implements port.ColorSchemeDetector.
// Queries libadwaita's StyleManager for the dark mode preference.
func (d *AdwaitaDetector) Detect() (prefersDark, ok bool) {
	if !d.Available() {
		return false, false
	}

	styleMgr := adw.StyleManagerGetDefault()
	if styleMgr == nil {
		return false, false
	}

	return styleMgr.GetDark(), true
}
