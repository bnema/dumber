package component

import (
	"context"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/ui/layout"
)

const defaultFloatingPaneURL = "about:blank"

// FloatingPaneOptions controls floating pane behavior.
type FloatingPaneOptions struct {
	WidthPct       float64
	HeightPct      float64
	FallbackWidth  int
	FallbackHeight int
	OnNavigate     func(ctx context.Context, url string) error
}

// FloatingPane tracks persistent floating workspace pane state.
type FloatingPane struct {
	parent         layout.OverlayWidget
	widthPct       float64
	heightPct      float64
	fallbackWidth  int
	fallbackHeight int
	onNavigate     func(ctx context.Context, url string) error

	mu               sync.RWMutex
	visible          bool
	omniboxVisible   bool
	sessionStarted   bool
	currentURL       string
	stateVersion     uint64
	calculatedWidth  int
	calculatedHeight int
}

// NewFloatingPane creates a floating pane state container.
func NewFloatingPane(parent layout.OverlayWidget, opts FloatingPaneOptions) *FloatingPane {
	if opts.WidthPct <= 0 || opts.WidthPct > 1 {
		opts.WidthPct = 0.82
	}
	if opts.HeightPct <= 0 || opts.HeightPct > 1 {
		opts.HeightPct = 0.72
	}
	if opts.FallbackWidth <= 0 {
		opts.FallbackWidth = 1200
	}
	if opts.FallbackHeight <= 0 {
		opts.FallbackHeight = 800
	}

	fp := &FloatingPane{
		parent:         parent,
		widthPct:       opts.WidthPct,
		heightPct:      opts.HeightPct,
		fallbackWidth:  opts.FallbackWidth,
		fallbackHeight: opts.FallbackHeight,
		onNavigate:     opts.OnNavigate,
	}
	fp.Resize()
	return fp
}

// ShowToggle toggles pane visibility; first open initializes a blank session.
func (fp *FloatingPane) ShowToggle(ctx context.Context) error {
	fp.mu.Lock()
	previousVisible := fp.visible
	previousOmniboxVisible := fp.omniboxVisible
	if fp.visible {
		fp.visible = false
		fp.omniboxVisible = false
		fp.stateVersion++
		fp.mu.Unlock()
		return nil
	}

	navigateToBlank := !fp.sessionStarted
	showOmnibox := navigateToBlank || fp.currentURL == defaultFloatingPaneURL
	fp.visible = true
	fp.omniboxVisible = showOmnibox
	fp.stateVersion++
	rollbackVersion := fp.stateVersion
	// Release lock before Navigate: callbacks may block and must not run while
	// holding fp.mu. On error, rollback below restores visibility state.
	fp.mu.Unlock()

	if navigateToBlank {
		if err := fp.Navigate(ctx, defaultFloatingPaneURL); err != nil {
			fp.mu.Lock()
			if fp.stateVersion == rollbackVersion {
				fp.visible = previousVisible
				fp.omniboxVisible = previousOmniboxVisible
				fp.stateVersion++
			}
			fp.mu.Unlock()
			return err
		}
	}

	return nil
}

// Show makes the pane visible without navigation.
// Use for profile sessions that already have a loaded URL.
func (fp *FloatingPane) Show() {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	fp.visible = true
	fp.omniboxVisible = false
	fp.stateVersion++
}

// SessionStarted reports whether the floating pane has navigated at least once.
func (fp *FloatingPane) SessionStarted() bool {
	fp.mu.RLock()
	defer fp.mu.RUnlock()
	return fp.sessionStarted
}

// ShowURL shows the pane and navigates to a target URL.
func (fp *FloatingPane) ShowURL(ctx context.Context, url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		fp.mu.Lock()
		fp.visible = true
		fp.omniboxVisible = true
		fp.stateVersion++
		fp.mu.Unlock()
		return nil
	}

	fp.mu.Lock()
	previousVisible := fp.visible
	previousOmniboxVisible := fp.omniboxVisible
	fp.visible = true
	fp.omniboxVisible = false
	shouldNavigate := !fp.sessionStarted || fp.currentURL != url
	fp.stateVersion++
	rollbackVersion := fp.stateVersion
	fp.mu.Unlock()

	if shouldNavigate {
		if err := fp.Navigate(ctx, url); err != nil {
			fp.mu.Lock()
			if fp.stateVersion == rollbackVersion {
				fp.visible = previousVisible
				fp.omniboxVisible = previousOmniboxVisible
				fp.stateVersion++
			}
			fp.mu.Unlock()
			return err
		}
	}

	return nil
}

// Hide hides the pane while preserving web session state.
func (fp *FloatingPane) Hide(_ context.Context) {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	fp.visible = false
	fp.omniboxVisible = false
	fp.stateVersion++
}

// Navigate loads a URL into the floating pane's persistent session.
func (fp *FloatingPane) Navigate(ctx context.Context, url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil
	}

	fp.mu.RLock()
	onNavigate := fp.onNavigate
	fp.mu.RUnlock()

	if onNavigate != nil {
		if err := onNavigate(ctx, url); err != nil {
			return err
		}
	}

	fp.mu.Lock()
	fp.currentURL = url
	fp.sessionStarted = true
	fp.stateVersion++
	fp.mu.Unlock()

	return nil
}

// Resize recalculates pane dimensions from parent overlay allocation.
func (fp *FloatingPane) Resize() {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	fp.calculatedWidth, fp.calculatedHeight = CalculateOverlayDimensions(
		fp.parent,
		fp.widthPct,
		fp.heightPct,
		fp.fallbackWidth,
		fp.fallbackHeight,
	)
}

// SetParentOverlay updates the workspace overlay used for sizing calculations.
func (fp *FloatingPane) SetParentOverlay(parent layout.OverlayWidget) {
	fp.mu.Lock()
	fp.parent = parent
	fp.mu.Unlock()
	fp.Resize()
}

// Dimensions returns the last calculated pane dimensions.
func (fp *FloatingPane) Dimensions() (width, height int) {
	fp.mu.RLock()
	defer fp.mu.RUnlock()
	return fp.calculatedWidth, fp.calculatedHeight
}

// CurrentURL returns the current URL loaded in the floating session.
func (fp *FloatingPane) CurrentURL() string {
	fp.mu.RLock()
	defer fp.mu.RUnlock()
	return fp.currentURL
}

// IsVisible reports whether the floating pane is currently visible.
func (fp *FloatingPane) IsVisible() bool {
	fp.mu.RLock()
	defer fp.mu.RUnlock()
	return fp.visible
}

// IsOmniboxVisible reports whether the floating pane omnibox is visible.
func (fp *FloatingPane) IsOmniboxVisible() bool {
	fp.mu.RLock()
	defer fp.mu.RUnlock()
	return fp.omniboxVisible
}

// SetOmniboxVisible updates omnibox visibility state for the floating pane.
func (fp *FloatingPane) SetOmniboxVisible(visible bool) {
	fp.mu.Lock()
	fp.omniboxVisible = visible
	fp.stateVersion++
	fp.mu.Unlock()
}

// RecordLoadedURL updates floating pane URL state from external navigation events.
func (fp *FloatingPane) RecordLoadedURL(url string) {
	url = strings.TrimSpace(url)
	if url == "" {
		return
	}

	fp.mu.Lock()
	fp.currentURL = url
	fp.sessionStarted = true
	fp.stateVersion++
	fp.mu.Unlock()
}
