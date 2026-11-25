package url

import "testing"

func TestResolveBrowserStartupURL(t *testing.T) {
	tests := []struct {
		name, explicit, configured, want string
	}{
		{"explicit wins", "https://example.com", "dumb://history", "https://example.com"},
		{"configured when no explicit", "", "dumb://history", "dumb://history"},
		{"whitespace explicit uses configured", "   ", " dumb://favorites ", "dumb://favorites"},
		{"empty falls back to home", "", "", "dumb://home"},
		{"whitespace falls back to home", " ", "  ", "dumb://home"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveBrowserStartupURL(tt.explicit, tt.configured); got != tt.want {
				t.Fatalf("ResolveBrowserStartupURL(%q, %q) = %q, want %q", tt.explicit, tt.configured, got, tt.want)
			}
		})
	}
}

func TestDefaultBrowserStartupURL(t *testing.T) {
	if got := DefaultBrowserStartupURL(); got != "dumb://home" {
		t.Fatalf("DefaultBrowserStartupURL() = %q, want dumb://home", got)
	}
}
