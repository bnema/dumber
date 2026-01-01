package fonts

import (
	"bufio"
	"context"
	"os/exec"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// Fallback chains for each font category (unexported to prevent modification).
var (
	sansSerifFallbackChain = []string{
		"Fira Sans",
		"Noto Sans",
		"DejaVu Sans",
		"Liberation Sans",
		"FreeSans",
	}

	serifFallbackChain = []string{
		"Noto Serif",
		"DejaVu Serif",
		"Liberation Serif",
		"FreeSerif",
	}

	monospaceFallbackChain = []string{
		"Fira Code",
		"JetBrains Mono",
		"Noto Sans Mono",
		"DejaVu Sans Mono",
		"Liberation Mono",
		"FreeMono",
	}
)

// SansSerifFallbackChain returns the fallback chain for sans-serif fonts.
func SansSerifFallbackChain() []string {
	result := make([]string, len(sansSerifFallbackChain))
	copy(result, sansSerifFallbackChain)
	return result
}

// SerifFallbackChain returns the fallback chain for serif fonts.
func SerifFallbackChain() []string {
	result := make([]string, len(serifFallbackChain))
	copy(result, serifFallbackChain)
	return result
}

// MonospaceFallbackChain returns the fallback chain for monospace fonts.
func MonospaceFallbackChain() []string {
	result := make([]string, len(monospaceFallbackChain))
	copy(result, monospaceFallbackChain)
	return result
}

// Generic CSS fallbacks when no fonts from the chain are found.
const (
	genericSansSerif = "sans-serif"
	genericSerif     = "serif"
	genericMonospace = "monospace"
)

// Detector implements port.FontDetector using fontconfig's fc-list command.
type Detector struct {
	mu             sync.RWMutex
	cachedFonts    []string
	cachePopulated bool
}

// NewDetector creates a new font detector.
func NewDetector() *Detector {
	return &Detector{}
}

// IsAvailable implements port.FontDetector.
// Returns true if fc-list command is available on the system.
func (*Detector) IsAvailable(_ context.Context) bool {
	_, err := exec.LookPath("fc-list")
	return err == nil
}

// GetAvailableFonts implements port.FontDetector.
// Returns a list of font family names installed on the system.
func (d *Detector) GetAvailableFonts(ctx context.Context) ([]string, error) {
	log := logging.FromContext(ctx)

	d.mu.RLock()
	if d.cachePopulated {
		fonts := d.cachedFonts
		d.mu.RUnlock()
		return fonts, nil
	}
	d.mu.RUnlock()

	d.mu.Lock()
	defer d.mu.Unlock()

	// Double-check after acquiring write lock.
	if d.cachePopulated {
		return d.cachedFonts, nil
	}

	fonts, err := d.queryFonts(ctx)
	if err != nil {
		log.Debug().Err(err).Msg("failed to query system fonts")
		return nil, err
	}

	d.cachedFonts = fonts
	d.cachePopulated = true
	log.Debug().Int("count", len(fonts)).Msg("cached system fonts")

	return fonts, nil
}

// SelectBestFont implements port.FontDetector.
// Returns the first available font from the fallback chain for the given category.
func (d *Detector) SelectBestFont(
	ctx context.Context,
	category port.FontCategory,
	fallbackChain []string,
) string {
	log := logging.FromContext(ctx)

	availableFonts, err := d.GetAvailableFonts(ctx)
	if err != nil {
		log.Debug().
			Str("category", string(category)).
			Err(err).
			Msg("font detection unavailable, using generic fallback")
		return d.genericFallback(category)
	}

	// Build a set for O(1) lookup.
	fontSet := make(map[string]struct{}, len(availableFonts))
	for _, f := range availableFonts {
		fontSet[f] = struct{}{}
	}

	// Find the first available font in the chain.
	for _, font := range fallbackChain {
		if _, exists := fontSet[font]; exists {
			log.Debug().
				Str("category", string(category)).
				Str("font", font).
				Msg("selected font from fallback chain")
			return font
		}
	}

	// No fonts from chain found, use generic fallback.
	generic := d.genericFallback(category)
	log.Debug().
		Str("category", string(category)).
		Str("fallback", generic).
		Msg("no fonts from fallback chain available, using generic")
	return generic
}

// queryFonts executes fc-list and parses the output.
func (*Detector) queryFonts(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "fc-list", ":", "family")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	fontSet := make(map[string]struct{})
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// fc-list may return comma-separated families for fonts with aliases.
		// e.g., "DejaVu Sans,DejaVu Sans Light"
		families := strings.Split(line, ",")
		for _, family := range families {
			family = strings.TrimSpace(family)
			if family != "" {
				fontSet[family] = struct{}{}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Convert set to slice.
	fonts := make([]string, 0, len(fontSet))
	for font := range fontSet {
		fonts = append(fonts, font)
	}

	return fonts, nil
}

// genericFallback returns the CSS generic fallback for the given category.
func (*Detector) genericFallback(category port.FontCategory) string {
	switch category {
	case port.FontCategorySansSerif:
		return genericSansSerif
	case port.FontCategorySerif:
		return genericSerif
	case port.FontCategoryMonospace:
		return genericMonospace
	default:
		return genericSansSerif
	}
}
