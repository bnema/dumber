package theme

import (
	"regexp"
	"strings"
	"testing"
)

func TestGenerateCSS_StandaloneOmniboxWindowUsesTransparentBackground(t *testing.T) {
	css := GenerateCSS(DefaultDarkPalette())

	if !strings.Contains(css, "window.standalone-omnibox-window") {
		t.Fatalf("expected standalone omnibox window selector in CSS, got %q", css)
	}
	re := regexp.MustCompile(`window\.standalone-omnibox-window\s*\{\s*background-color:\s*transparent\s*;?\s*\}`)
	if !re.MatchString(css) {
		t.Fatalf("expected standalone omnibox window background to be transparent")
	}
}

func TestGenerateCSS_OmniboxHeaderIsOpaque(t *testing.T) {
	css := GenerateCSS(DefaultDarkPalette())

	if !strings.Contains(css, ".omnibox-header {") {
		t.Fatalf("expected omnibox header selector in CSS")
	}
	if !strings.Contains(css, "background-color: var(--surface);") {
		t.Fatalf("expected omnibox header to use an opaque surface background")
	}
	if !strings.Contains(css, "background-image: none;") {
		t.Fatalf("expected omnibox header gradient to be disabled")
	}
}

func TestGenerateCSS_FavoriteRowsOnlyTintTrailingAffordance(t *testing.T) {
	css := GenerateCSS(DefaultDarkPalette())

	if !strings.Contains(css, ".omnibox-row.omnibox-row-favorite {") {
		t.Fatalf("expected favorite row selector in CSS")
	}
	if !strings.Contains(css, "border-left: 0.1875em solid var(--warning);") {
		t.Fatalf("expected favorite rows to mark the left edge with the warning color")
	}
	if strings.Contains(css, "border-right: 0.125em solid alpha(var(--warning), 0.55);") {
		t.Fatalf("expected favorite indicator to move off the right edge")
	}
	if !strings.Contains(css, "color: mix(var(--warning), var(--muted), 0.45);") {
		t.Fatalf("expected favorite star color to be softened")
	}
	if strings.Contains(css, ".omnibox-row.omnibox-row-favorite .omnibox-favorite-star-slot") {
		t.Fatalf("expected favorite highlight to move off the star slot")
	}
}
