package color

import "testing"

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
			if got := ReadableTextColor(tt.background, tt.preferred); got != tt.want {
				t.Fatalf("ReadableTextColor(%q, %q) = %q, want %q", tt.background, tt.preferred, got, tt.want)
			}
		})
	}
}

func TestHexToRGBA(t *testing.T) {
	tests := []struct {
		name       string
		hex        string
		r, g, b, a float32
	}{
		{name: "short", hex: "#f80", r: 1, g: 136.0 / 255, b: 0, a: 1},
		{name: "standard", hex: "#FF8000", r: 1, g: 128.0 / 255, b: 0, a: 1},
		{name: "with alpha", hex: "#ff800080", r: 1, g: 128.0 / 255, b: 0, a: 128.0 / 255},
		{name: "invalid digit zeroes its channel", hex: "#1z8000", r: 0, g: 128.0 / 255, b: 0, a: 1},
		{name: "missing hash", hex: "ff8000", r: 0, g: 0, b: 0, a: 1},
		{name: "bad length", hex: "#ff80", r: 0, g: 0, b: 0, a: 1},
		{name: "empty", hex: "", r: 0, g: 0, b: 0, a: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := HexToRGBA(tt.hex)
			if r != tt.r || g != tt.g || b != tt.b || a != tt.a {
				t.Fatalf("HexToRGBA(%q) = (%v, %v, %v, %v), want (%v, %v, %v, %v)", tt.hex, r, g, b, a, tt.r, tt.g, tt.b, tt.a)
			}
		})
	}
}
