package theme

import (
	"strings"
	"testing"
)

func TestPaletteToCSSVarsAddsReadableControlText(t *testing.T) {
	palette := DefaultLightPalette()
	palette.SurfaceVariant = "#775f79"
	palette.Text = "#3d303f"

	css := palette.ToCSSVars()
	if !strings.Contains(css, "  --control-text: #ffffff;\n") {
		t.Fatalf("ToCSSVars() did not add light control text for dark input background:\n%s", css)
	}
}

func TestModeColorsToCSSVarsAddsReadableModeText(t *testing.T) {
	colors := DefaultModeColors()
	colors.TabMode = "#ffe066"

	css := colors.ToCSSVars()
	if !strings.Contains(css, "  --tab-mode-text: #000000;\n") {
		t.Fatalf("ToCSSVars() did not add dark text for light tab mode color:\n%s", css)
	}
	if !strings.Contains(css, "  --session-mode-text: #ffffff;\n") {
		t.Fatalf("ToCSSVars() did not keep white text for dark session mode color:\n%s", css)
	}
}
