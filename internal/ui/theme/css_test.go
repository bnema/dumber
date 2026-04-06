package theme

import (
	"strings"
	"testing"
)

func TestGenerateCSS_StandaloneOmniboxWindowUsesTransparentBackground(t *testing.T) {
	css := GenerateCSS(DefaultDarkPalette())

	if !strings.Contains(css, "window.standalone-omnibox-window") {
		t.Fatalf("expected standalone omnibox window selector in CSS, got %q", css)
	}
	if !strings.Contains(css, "window.standalone-omnibox-window {\n\tbackground-color: transparent;") {
		t.Fatalf("expected standalone omnibox window background to be transparent")
	}
}
