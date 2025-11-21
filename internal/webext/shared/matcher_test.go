package shared

import "testing"

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		url     string
		want    bool
	}{
		{"all_urls matches https", "<all_urls>", "https://example.com/page", true},
		{"all_urls matches http", "<all_urls>", "http://example.com/page", true},
		{"wildcard scheme https", "*://*.example.com/*", "https://www.example.com/page", true},
		{"wildcard scheme http", "*://*.example.com/*", "http://api.example.com/path", true},
		{"specific scheme match", "https://example.com/*", "https://example.com/page", true},
		{"specific scheme mismatch", "https://example.com/*", "http://example.com/page", false},
		{"wildcard host subdomain", "https://*.google.com/*", "https://mail.google.com/", true},
		{"wildcard host wrong domain", "https://*.google.com/*", "https://google.co.uk/", false},
		{"path wildcard subpath", "https://example.com/api/*", "https://example.com/api/v1/users", true},
		{"path wildcard different", "https://example.com/api/*", "https://example.com/docs/", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp, err := NewMatchPattern(tt.pattern)
			if err != nil {
				t.Fatalf("NewMatchPattern() error = %v", err)
			}
			if mp == nil {
				t.Fatal("NewMatchPattern() returned nil")
			}

			got := mp.Match(tt.url)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v (pattern=%q, url=%q)", got, tt.want, tt.pattern, tt.url)
			}
		})
	}
}

func TestMatchURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		patterns []string
		want     bool
	}{
		{"matches first pattern", "https://example.com/page", []string{"*://example.com/*", "https://other.com/*"}, true},
		{"matches second pattern", "https://other.com/page", []string{"https://example.com/*", "*://other.com/*"}, true},
		{"no patterns match", "https://example.com/page", []string{"https://other.com/*"}, false},
		{"empty patterns", "https://example.com/page", []string{}, false},
		{"all_urls in list", "https://example.com/page", []string{"<all_urls>"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchURL(tt.url, tt.patterns)
			if got != tt.want {
				t.Errorf("MatchURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExcludesURL(t *testing.T) {
	tests := []struct {
		name            string
		url             string
		excludePatterns []string
		want            bool
	}{
		{"excluded by pattern", "https://example.com/admin", []string{"*://example.com/admin*"}, true},
		{"not excluded", "https://example.com/public", []string{"*://example.com/admin*"}, false},
		{"no excludes", "https://example.com/page", []string{}, false},
		{"multiple excludes match", "https://example.com/secret", []string{"*://example.com/admin*", "*://example.com/secret*"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExcludesURL(tt.url, tt.excludePatterns)
			if got != tt.want {
				t.Errorf("ExcludesURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
