package parser

import (
	"testing"
)

func TestURLValidator_IsValidURL(t *testing.T) {
	validator := NewURLValidator()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid URLs with scheme
		{"HTTPS URL", "https://example.com", true},
		{"HTTP URL", "http://example.com", true},
		{"URL with path", "https://example.com/path", true},
		{"URL with query", "https://example.com?q=test", true},

		// Valid domains
		{"Simple domain", "example.com", true},
		{"Subdomain", "www.example.com", true},
		{"Multiple subdomains", "api.v1.example.com", true},
		{"Domain with path", "example.com/path", true},

		// Valid IP addresses
		{"IPv4 address", "192.168.1.1", true},
		{"Localhost IP", "127.0.0.1", true},

		// Invalid inputs
		{"Empty string", "", false},
		{"Just text", "hello world", false},
		{"Single word", "test", false},
		{"Space in domain", "exam ple.com", false},
		{"Invalid TLD", "example.toolongtobevalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.IsValidURL(tt.input)
			if result != tt.expected {
				t.Errorf("IsValidURL(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestURLValidator_IsDirectURL(t *testing.T) {
	validator := NewURLValidator()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Should be treated as direct URLs
		{"Complete HTTPS URL", "https://github.com", true},
		{"Complete HTTP URL", "http://example.com", true},
		{"Domain only", "github.com", true},
		{"Subdomain", "api.github.com", true},
		{"Domain with path", "github.com/user/repo", true},
		{"IP address", "192.168.1.1", true},
		{"Localhost", "127.0.0.1", true},

		// Should NOT be treated as direct URLs
		{"Single word", "github", false},
		{"Search query", "golang tutorial", false},
		{"Shortcut format", "g: search query", false},
		{"Empty string", "", false},
		{"Just spaces", "   ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.IsDirectURL(tt.input)
			if result != tt.expected {
				t.Errorf("IsDirectURL(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestURLValidator_IsSearchShortcut(t *testing.T) {
	validator := NewURLValidator()

	tests := []struct {
		name           string
		input          string
		expectShortcut bool
		expectKey      string
		expectQuery    string
	}{
		{
			name:           "Google shortcut",
			input:          "g: golang tutorial",
			expectShortcut: true,
			expectKey:      "g",
			expectQuery:    "golang tutorial",
		},
		{
			name:           "GitHub shortcut",
			input:          "gh: cobra cli",
			expectShortcut: true,
			expectKey:      "gh",
			expectQuery:    "cobra cli",
		},
		{
			name:           "Stack Overflow shortcut",
			input:          "so: fuzzy search algorithm",
			expectShortcut: true,
			expectKey:      "so",
			expectQuery:    "fuzzy search algorithm",
		},
		{
			name:           "Empty query",
			input:          "g: ",
			expectShortcut: true,
			expectKey:      "g",
			expectQuery:    "",
		},
		{
			name:           "Numeric shortcut",
			input:          "123: test",
			expectShortcut: true,
			expectKey:      "123",
			expectQuery:    "test",
		},
		// Not shortcuts
		{
			name:           "Just colon",
			input:          ":",
			expectShortcut: false,
		},
		{
			name:           "No colon",
			input:          "github search",
			expectShortcut: false,
		},
		{
			name:           "URL with colon",
			input:          "https://example.com",
			expectShortcut: false,
		},
		{
			name:           "Empty string",
			input:          "",
			expectShortcut: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isShortcut, key, query := validator.IsSearchShortcut(tt.input)

			if isShortcut != tt.expectShortcut {
				t.Errorf("IsSearchShortcut(%q) isShortcut = %v, want %v", tt.input, isShortcut, tt.expectShortcut)
			}

			if tt.expectShortcut {
				if key != tt.expectKey {
					t.Errorf("IsSearchShortcut(%q) key = %q, want %q", tt.input, key, tt.expectKey)
				}
				if query != tt.expectQuery {
					t.Errorf("IsSearchShortcut(%q) query = %q, want %q", tt.input, query, tt.expectQuery)
				}
			}
		})
	}
}

func TestURLValidator_NormalizeURL(t *testing.T) {
	validator := NewURLValidator()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Already have scheme
		{"HTTPS URL", "https://example.com", "https://example.com"},
		{"HTTP URL", "http://example.com", "http://example.com"},

		// Need scheme added
		{"Domain only", "example.com", "https://example.com"},
		{"Subdomain", "www.example.com", "https://www.example.com"},
		{"Domain with path", "example.com/path", "https://example.com/path"},
		{"IP address", "192.168.1.1", "https://192.168.1.1"},

		// Special cases
		{"Empty string", "", ""},
		{"Just spaces", "   ", ""},
		{"Non-URL text", "hello world", "hello world"}, // Should remain unchanged
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.NormalizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestURLValidator_ExtractDomain(t *testing.T) {
	validator := NewURLValidator()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// URLs with scheme
		{"HTTPS URL", "https://github.com", "github.com"},
		{"HTTP URL", "http://example.com", "example.com"},
		{"URL with path", "https://github.com/user/repo", "github.com"},
		{"URL with query", "https://google.com/search?q=test", "google.com"},
		{"URL with port", "https://localhost:8080", "localhost"},

		// Domains without scheme
		{"Domain only", "github.com", "github.com"},
		{"Subdomain", "api.github.com", "api.github.com"},
		{"Domain with path", "github.com/user/repo", "github.com"},

		// IP addresses
		{"IPv4", "192.168.1.1", "192.168.1.1"},
		{"IPv4 with port", "192.168.1.1:8080", "192.168.1.1"},

		// Edge cases
		{"Empty string", "", ""},
		{"Just path", "/path/to/resource", ""}, // Invalid input returns empty
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.ExtractDomain(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractDomain(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestURLValidator_HasFileExtension(t *testing.T) {
	validator := NewURLValidator()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// URLs with file extensions
		{"HTML file", "https://example.com/index.html", true},
		{"PDF file", "https://example.com/document.pdf", true},
		{"Image file", "https://example.com/image.jpg", true},
		{"File in subdirectory", "https://example.com/docs/readme.txt", true},

		// URLs without file extensions
		{"Root URL", "https://example.com", false},
		{"Root with slash", "https://example.com/", false},
		{"Directory path", "https://example.com/docs/", false},
		{"Path without extension", "https://example.com/about", false},

		// Edge cases
		{"Dot at end", "https://example.com/file.", false},
		{"Multiple dots", "https://example.com/file.name.ext", true},
		{"Query params", "https://example.com/file.html?q=test", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.HasFileExtension(tt.input)
			if result != tt.expected {
				t.Errorf("HasFileExtension(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestURLValidator_IsLocalhost(t *testing.T) {
	validator := NewURLValidator()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Localhost variants
		{"localhost", "http://localhost", true},
		{"localhost with port", "http://localhost:3000", true},
		{"localhost HTTPS", "https://localhost:8080", true},
		{"127.0.0.1", "http://127.0.0.1", true},
		{"127.0.0.1 with port", "http://127.0.0.1:8080", true},
		{"IPv6 localhost", "http://[::1]", true},
		{"Local domain", "http://app.local", true},

		// Non-localhost
		{"example.com", "https://example.com", false},
		{"github.com", "https://github.com", false},
		{"127.0.0.2", "http://127.0.0.2", false},     // Different local IP
		{"192.168.1.1", "http://192.168.1.1", false}, // Private network but not localhost

		// Edge cases
		{"Empty string", "", false},
		{"Just localhost", "localhost", true},
		{"Just IP", "127.0.0.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.IsLocalhost(tt.input)
			if result != tt.expected {
				t.Errorf("IsLocalhost(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestURLValidator_SanitizeInput(t *testing.T) {
	validator := NewURLValidator()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Normal text", "github.com", "github.com"},
		{"Text with spaces", "  github.com  ", "github.com"},
		{"Text with tabs", "\tgithub.com\t", "github.com"},
		{"Text with newlines", "\ngithub.com\n", "github.com"},
		{"Unicode text", "github.com/ñoño", "github.com/ñoño"},
		{"Empty string", "", ""},
		{"Only whitespace", "   \t\n   ", ""},
		{"Mixed whitespace and text", " \t github.com \n ", "github.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.SanitizeInput(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeInput(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestURLValidator_GetURLType(t *testing.T) {
	validator := NewURLValidator()

	tests := []struct {
		name     string
		input    string
		expected InputType
	}{
		// Direct URLs
		{"HTTPS URL", "https://github.com", InputTypeDirectURL},
		{"Domain", "github.com", InputTypeDirectURL},
		{"IP address", "192.168.1.1", InputTypeDirectURL},

		// Search shortcuts
		{"Google shortcut", "g: golang tutorial", InputTypeSearchShortcut},
		{"GitHub shortcut", "gh: cobra cli", InputTypeSearchShortcut},

		// History search (everything else initially)
		{"Single word", "github", InputTypeHistorySearch},
		{"Search phrase", "golang tutorial", InputTypeHistorySearch},
		{"Question", "how to use fuzzy search", InputTypeHistorySearch},
		{"Empty string", "", InputTypeHistorySearch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.GetURLType(tt.input)
			if result != tt.expected {
				t.Errorf("GetURLType(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestURLValidator_IsDomain(t *testing.T) {
	validator := NewURLValidator()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid domains
		{"Simple domain", "example.com", true},
		{"Subdomain", "www.example.com", true},
		{"Multiple subdomains", "api.v1.example.com", true},
		{"Country TLD", "example.co.uk", true},
		{"New TLD", "example.dev", true},

		// Invalid domains
		{"Single word", "example", false},
		{"No TLD", "www.example", false},
		{"Starts with dot", ".example.com", false},
		{"Ends with dot", "example.com.", false},
		{"Contains spaces", "exam ple.com", false},
		{"Too long segment", "verylongsubdomainthatexceedssixtyThreecharacterslimitfordomainsegments.com", false},
		{"Empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.isDomain(tt.input)
			if result != tt.expected {
				t.Errorf("isDomain(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestURLValidator_IsIPAddress(t *testing.T) {
	validator := NewURLValidator()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid IPv4
		{"Standard IPv4", "192.168.1.1", true},
		{"Localhost IPv4", "127.0.0.1", true},
		{"Zero IPv4", "0.0.0.0", true},
		{"Max IPv4", "255.255.255.255", true},

		// Valid IPv6
		{"Standard IPv6", "2001:db8::1", true},
		{"Localhost IPv6", "::1", true},

		// Invalid IPs
		{"Invalid IPv4 (out of range)", "256.256.256.256", false},
		{"Invalid IPv4 (too many octets)", "192.168.1.1.1", false},
		{"Invalid IPv4 (too few octets)", "192.168.1", false},
		{"Not an IP", "example.com", false},
		{"Empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.isIPAddress(tt.input)
			if result != tt.expected {
				t.Errorf("isIPAddress(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestURLValidator_LooksLikeDomain(t *testing.T) {
	validator := NewURLValidator()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Should look like domains
		{"Standard domain", "example.com", true},
		{"Subdomain", "www.example.com", true},
		{"Uncommon TLD", "example.xyz", true},
		{"Multiple levels", "a.b.c.example.com", true},

		// Should not look like domains
		{"No dot", "example", false},
		{"Has space", "exam ple.com", false},
		{"Too long", string(make([]rune, 300)), false}, // Very long string
		{"Empty segment", "example..com", false},
		{"Starts with non-alphanumeric", "-example.com", false},
		{"Ends with non-alphanumeric", "example-.com", false},
		{"Empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.looksLikeDomain(tt.input)
			if result != tt.expected {
				t.Errorf("looksLikeDomain(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func BenchmarkURLValidator_IsValidURL(b *testing.B) {
	validator := NewURLValidator()
	testInputs := []string{
		"https://github.com",
		"github.com",
		"192.168.1.1",
		"g: search query",
		"just some text",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.IsValidURL(testInputs[i%len(testInputs)])
	}
}

func BenchmarkURLValidator_IsDirectURL(b *testing.B) {
	validator := NewURLValidator()
	testInputs := []string{
		"https://github.com",
		"github.com",
		"search query",
		"g: shortcut",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.IsDirectURL(testInputs[i%len(testInputs)])
	}
}

func BenchmarkURLValidator_ExtractDomain(b *testing.B) {
	validator := NewURLValidator()
	testURL := "https://api.github.com/users/octocat/repos"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ExtractDomain(testURL)
	}
}
