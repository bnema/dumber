package parser

import "testing"

func TestNormalizeHistoryURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"addsTrailingSlash", "https://duckduckgo.com", "https://duckduckgo.com/"},
		{"keepsExistingSlash", "https://duckduckgo.com/", "https://duckduckgo.com/"},
		{"keepsPath", "https://example.com/foo", "https://example.com/foo"},
		{"keepsQuery", "https://example.com?q=1", "https://example.com/?q=1"},
		{"httpScheme", "http://example.com", "http://example.com/"},
		{"nonHttpScheme", "ftp://example.com", "ftp://example.com"},
		{"invalidURL", "not a url", "not a url"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeHistoryURL(tt.input); got != tt.expected {
				t.Fatalf("NormalizeHistoryURL(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
