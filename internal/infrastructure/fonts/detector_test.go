package fonts

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testContext() context.Context {
	logger := zerolog.Nop()
	return logger.WithContext(context.Background())
}

func TestDetector_IsAvailable(t *testing.T) {
	ctx := testContext()
	detector := NewDetector()

	// fc-list should be available on most Linux systems
	// This test documents the behavior rather than asserting a specific value
	available := detector.IsAvailable(ctx)
	t.Logf("fc-list available: %v", available)
}

func TestDetector_GetAvailableFonts(t *testing.T) {
	ctx := testContext()
	detector := NewDetector()

	if !detector.IsAvailable(ctx) {
		t.Skip("fc-list not available on this system")
	}

	fonts, err := detector.GetAvailableFonts(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, fonts, "expected at least some fonts to be installed")

	t.Logf("Found %d font families", len(fonts))
}

func TestDetector_GetAvailableFonts_Caching(t *testing.T) {
	ctx := testContext()
	detector := NewDetector()

	if !detector.IsAvailable(ctx) {
		t.Skip("fc-list not available on this system")
	}

	// First call populates cache
	fonts1, err := detector.GetAvailableFonts(ctx)
	require.NoError(t, err)

	// Second call should return cached result
	fonts2, err := detector.GetAvailableFonts(ctx)
	require.NoError(t, err)

	assert.Len(t, fonts2, len(fonts1), "cached result should match original")
}

func TestDetector_SelectBestFont_SansSerif(t *testing.T) {
	ctx := testContext()
	detector := NewDetector()

	if !detector.IsAvailable(ctx) {
		t.Skip("fc-list not available on this system")
	}

	font := detector.SelectBestFont(ctx, port.FontCategorySansSerif, SansSerifFallbackChain())
	assert.NotEmpty(t, font)

	// Should either be a font from the chain or the generic fallback
	isFromChain := false
	for _, f := range SansSerifFallbackChain() {
		if font == f {
			isFromChain = true
			break
		}
	}
	if !isFromChain {
		assert.Equal(t, "sans-serif", font, "should fall back to generic if no chain fonts available")
	}

	t.Logf("Selected sans-serif font: %s", font)
}

func TestDetector_SelectBestFont_Serif(t *testing.T) {
	ctx := testContext()
	detector := NewDetector()

	if !detector.IsAvailable(ctx) {
		t.Skip("fc-list not available on this system")
	}

	font := detector.SelectBestFont(ctx, port.FontCategorySerif, SerifFallbackChain())
	assert.NotEmpty(t, font)

	isFromChain := false
	for _, f := range SerifFallbackChain() {
		if font == f {
			isFromChain = true
			break
		}
	}
	if !isFromChain {
		assert.Equal(t, "serif", font, "should fall back to generic if no chain fonts available")
	}

	t.Logf("Selected serif font: %s", font)
}

func TestDetector_SelectBestFont_Monospace(t *testing.T) {
	ctx := testContext()
	detector := NewDetector()

	if !detector.IsAvailable(ctx) {
		t.Skip("fc-list not available on this system")
	}

	font := detector.SelectBestFont(ctx, port.FontCategoryMonospace, MonospaceFallbackChain())
	assert.NotEmpty(t, font)

	isFromChain := false
	for _, f := range MonospaceFallbackChain() {
		if font == f {
			isFromChain = true
			break
		}
	}
	if !isFromChain {
		assert.Equal(t, "monospace", font, "should fall back to generic if no chain fonts available")
	}

	t.Logf("Selected monospace font: %s", font)
}

func TestDetector_SelectBestFont_EmptyChain(t *testing.T) {
	ctx := testContext()
	detector := NewDetector()

	if !detector.IsAvailable(ctx) {
		t.Skip("fc-list not available on this system")
	}

	// Empty chain should return generic fallback
	font := detector.SelectBestFont(ctx, port.FontCategorySansSerif, []string{})
	assert.Equal(t, "sans-serif", font)

	font = detector.SelectBestFont(ctx, port.FontCategorySerif, []string{})
	assert.Equal(t, "serif", font)

	font = detector.SelectBestFont(ctx, port.FontCategoryMonospace, []string{})
	assert.Equal(t, "monospace", font)
}

func TestDetector_SelectBestFont_NoMatchingFonts(t *testing.T) {
	ctx := testContext()
	detector := NewDetector()

	if !detector.IsAvailable(ctx) {
		t.Skip("fc-list not available on this system")
	}

	// Chain with fonts that definitely don't exist
	chain := []string{
		"NonExistentFont1234",
		"AnotherFakeFont5678",
	}

	font := detector.SelectBestFont(ctx, port.FontCategorySansSerif, chain)
	assert.Equal(t, "sans-serif", font, "should fall back to generic when no fonts match")
}

func TestDetector_genericFallback(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		category port.FontCategory
		expected string
	}{
		{port.FontCategorySansSerif, "sans-serif"},
		{port.FontCategorySerif, "serif"},
		{port.FontCategoryMonospace, "monospace"},
		{port.FontCategory("unknown"), "sans-serif"}, // default case
	}

	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			result := detector.genericFallback(tt.category)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFallbackChains_NotEmpty(t *testing.T) {
	assert.NotEmpty(t, SansSerifFallbackChain(), "sans-serif fallback chain should not be empty")
	assert.NotEmpty(t, SerifFallbackChain(), "serif fallback chain should not be empty")
	assert.NotEmpty(t, MonospaceFallbackChain(), "monospace fallback chain should not be empty")
}

func TestFallbackChains_PreferredFontsFirst(t *testing.T) {
	// Verify preferred fonts are first in chain
	assert.Equal(t, "Fira Sans", SansSerifFallbackChain()[0], "Fira Sans should be first in sans-serif chain")
	assert.Equal(t, "Noto Serif", SerifFallbackChain()[0], "Noto Serif should be first in serif chain")
	assert.Equal(t, "Fira Code", MonospaceFallbackChain()[0], "Fira Code should be first in monospace chain")
}
