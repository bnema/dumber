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
