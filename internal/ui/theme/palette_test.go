package theme

import (
	"strings"
	"testing"
)

func TestReadableTextColor(t *testing.T) {
	tests := []struct {
		name       string
		background string
		preferred  string
		want       string
	}{
		{
			name:       "keeps readable palette text",
			background: "#282828",
			preferred:  "#ffffff",
			want:       "#ffffff",
		},
		{
			name:       "uses light text on dark control",
			background: "#775f79",
			preferred:  "#3d303f",
			want:       "#ffffff",
		},
		{
			name:       "uses dark text on light control",
			background: "#f0f0f0",
			preferred:  "#dddddd",
			want:       "#000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := readableTextColor(tt.background, tt.preferred); got != tt.want {
				t.Fatalf("readableTextColor(%q, %q) = %q, want %q", tt.background, tt.preferred, got, tt.want)
			}
		})
	}
}

func TestPaletteToCSSVarsAddsReadableControlText(t *testing.T) {
	palette := DefaultLightPalette()
	palette.SurfaceVariant = "#775f79"
	palette.Text = "#3d303f"

	css := palette.ToCSSVars()
	if !strings.Contains(css, "  --control-text: #ffffff;\n") {
		t.Fatalf("ToCSSVars() did not add light control text for dark input background:\n%s", css)
	}
}
