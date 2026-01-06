package port

import "context"

// FontCategory represents a category of fonts for fallback selection.
type FontCategory string

const (
	// FontCategorySansSerif is for sans-serif fonts (UI text).
	FontCategorySansSerif FontCategory = "sans-serif"
	// FontCategorySerif is for serif fonts (reading content).
	FontCategorySerif FontCategory = "serif"
	// FontCategoryMonospace is for monospace fonts (code, fixed-width).
	FontCategoryMonospace FontCategory = "monospace"
)

// FontDetector detects available system fonts and selects the best
// font from a fallback chain. This is used during first-run config
// creation to ensure configured fonts are actually installed.
type FontDetector interface {
	// GetAvailableFonts returns a list of font family names installed on the system.
	// Returns an error if font detection is not available (e.g., fc-list missing).
	GetAvailableFonts(ctx context.Context) ([]string, error)

	// SelectBestFont returns the first available font from the fallback chain,
	// or the generic CSS fallback (e.g., "sans-serif") if none are installed.
	// The category is used to determine the generic fallback.
	SelectBestFont(ctx context.Context, category FontCategory, fallbackChain []string) string

	// IsAvailable returns true if font detection is available on this system.
	IsAvailable(ctx context.Context) bool
}
