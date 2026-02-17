package url

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "http scheme unchanged",
			input: "http://example.com",
			want:  "http://example.com",
		},
		{
			name:  "https scheme unchanged",
			input: "https://example.com",
			want:  "https://example.com",
		},
		{
			name:  "file scheme unchanged",
			input: "file:///path/to/file.html",
			want:  "file:///path/to/file.html",
		},
		{
			name:  "dumb scheme unchanged",
			input: "dumb://home",
			want:  "dumb://home",
		},
		{
			name:  "about scheme unchanged",
			input: "about:blank",
			want:  "about:blank",
		},
		{
			name:  "domain gets https",
			input: "example.com",
			want:  "https://example.com",
		},
		{
			name:  "domain with path gets https",
			input: "example.com/path",
			want:  "https://example.com/path",
		},
		{
			name:  "search query unchanged",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "single word unchanged",
			input: "hello",
			want:  "hello",
		},
		{
			name:  "localhost",
			input: "localhost",
			want:  "http://localhost",
		},
		{
			name:  "localhost with port",
			input: "localhost:5173",
			want:  "http://localhost:5173",
		},
		{
			name:  "localhost with alt port",
			input: "localhost:8080",
			want:  "http://localhost:8080",
		},
		{
			name:  "localhost with path",
			input: "localhost/api/v1",
			want:  "http://localhost/api/v1",
		},
		{
			name:  "localhost.com is a domain not localhost",
			input: "localhost.com",
			want:  "https://localhost.com",
		},
		{
			name:  "localhostevil.com is a domain not localhost",
			input: "localhostevil.com",
			want:  "https://localhostevil.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Normalize(tt.input)
			if got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalize_LocalFiles(t *testing.T) {
	// Create a temporary file for testing
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.html")
	if err := os.WriteFile(tmpFile, []byte("<html></html>"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	// Create a subdirectory with a file
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}
	subFile := filepath.Join(subDir, "page.html")
	if err := os.WriteFile(subFile, []byte("<html></html>"), 0644); err != nil {
		t.Fatalf("failed to create subdir file: %v", err)
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "absolute path to existing file",
			input: tmpFile,
			want:  "file://" + tmpFile,
		},
		{
			name:  "absolute path to existing directory",
			input: tmpDir,
			want:  "file://" + tmpDir,
		},
		{
			name:  "non-existent absolute path returned unchanged",
			input: "/nonexistent/file.html",
			want:  "/nonexistent/file.html",
		},
		{
			name:  "non-existent relative path with dot prefix returned unchanged",
			input: "./nonexistent/file.html",
			want:  "./nonexistent/file.html",
		},
		{
			name:  "non-existent home path returned unchanged",
			input: "~/nonexistent/file.html",
			want:  "~/nonexistent/file.html",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Normalize(tt.input)
			if got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalize_RelativePaths(t *testing.T) {
	// Create a temp file in current directory
	tmpFile, err := os.CreateTemp(".", "test-*.html")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Get absolute path for comparison
	absPath, err := filepath.Abs(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to get abs path: %v", err)
	}

	t.Run("relative path to existing file", func(t *testing.T) {
		got := Normalize(tmpFile.Name())
		want := "file://" + absPath
		if got != want {
			t.Errorf("Normalize(%q) = %q, want %q", tmpFile.Name(), got, want)
		}
	})
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home directory")
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "tilde expansion",
			input: "~/Documents",
			want:  filepath.Join(home, "Documents"),
		},
		{
			name:  "no tilde unchanged",
			input: "/absolute/path",
			want:  "/absolute/path",
		},
		{
			name:  "relative path unchanged",
			input: "./relative/path",
			want:  "./relative/path",
		},
		{
			name:  "tilde in middle unchanged",
			input: "/path/~/file",
			want:  "/path/~/file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandHome(tt.input)
			if got != tt.want {
				t.Errorf("expandHome(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTrimLeadingSpacesIfURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single space before https URL",
			input: " https://example.com",
			want:  "https://example.com",
		},
		{
			name:  "multiple spaces before https URL",
			input: "   https://example.com",
			want:  "https://example.com",
		},
		{
			name:  "tab before URL",
			input: "\thttps://example.com",
			want:  "https://example.com",
		},
		{
			name:  "mixed whitespace before URL",
			input: " \t https://example.com",
			want:  "https://example.com",
		},
		{
			name:  "space before domain",
			input: " example.com",
			want:  "example.com",
		},
		{
			name:  "spaces before localhost",
			input: "  localhost:8080",
			want:  "localhost:8080",
		},
		{
			name:  "no leading space URL unchanged",
			input: "https://example.com",
			want:  "https://example.com",
		},
		{
			name:  "search query with leading space unchanged",
			input: " hello world",
			want:  " hello world",
		},
		{
			name:  "single word with leading space unchanged",
			input: " hello",
			want:  " hello",
		},
		{
			name:  "empty string unchanged",
			input: "",
			want:  "",
		},
		{
			name:  "only spaces unchanged",
			input: "   ",
			want:  "   ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TrimLeadingSpacesIfURL(tt.input)
			if got != tt.want {
				t.Errorf("TrimLeadingSpacesIfURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLooksLikeURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
		{
			name:  "http URL",
			input: "http://example.com",
			want:  true,
		},
		{
			name:  "https URL",
			input: "https://example.com",
			want:  true,
		},
		{
			name:  "file URL",
			input: "file:///path/to/file",
			want:  true,
		},
		{
			name:  "dumb URL",
			input: "dumb://home",
			want:  true,
		},
		{
			name:  "about URL",
			input: "about:blank",
			want:  true,
		},
		{
			name:  "domain",
			input: "example.com",
			want:  true,
		},
		{
			name:  "domain with path",
			input: "example.com/path",
			want:  true,
		},
		{
			name:  "search query",
			input: "hello world",
			want:  false,
		},
		{
			name:  "single word",
			input: "hello",
			want:  false,
		},
		{
			name:  "localhost",
			input: "localhost",
			want:  true,
		},
		{
			name:  "localhost with port",
			input: "localhost:5173",
			want:  true,
		},
		{
			name:  "localhost with alt port",
			input: "localhost:8080",
			want:  true,
		},
		{
			name:  "localhost with path",
			input: "localhost/api/v1",
			want:  true,
		},
		{
			name:  "localhost.com is a domain not localhost",
			input: "localhost.com",
			want:  true,
		},
		{
			name:  "localhostevil.com is a domain not localhost",
			input: "localhostevil.com",
			want:  true,
		},
		{
			name:  "ipv4 with port",
			input: "100.64.0.10:3000",
			want:  true,
		},
		{
			name:  "ipv4",
			input: "127.0.0.1",
			want:  true,
		},
		{
			name:  "ipv6 bracketed",
			input: "[::1]",
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LooksLikeURL(tt.input)
			if got != tt.want {
				t.Errorf("LooksLikeURL(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalize_IPAddressGetsHTTPByDefault(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "tailnet ipv4", input: "100.64.0.10", want: "http://100.64.0.10"},
		{name: "tailnet ipv4 with port", input: "100.64.0.10:8080", want: "http://100.64.0.10:8080"},
		{name: "ipv4 with path", input: "100.64.0.10/api", want: "http://100.64.0.10/api"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Normalize(tt.input)
			if got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsExternalScheme(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
		{
			name:  "http scheme",
			input: "http://example.com",
			want:  false,
		},
		{
			name:  "https scheme",
			input: "https://example.com",
			want:  false,
		},
		{
			name:  "file scheme",
			input: "file:///path/to/file.html",
			want:  false,
		},
		{
			name:  "dumb scheme",
			input: "dumb://home",
			want:  false,
		},
		{
			name:  "about scheme",
			input: "about:blank",
			want:  false,
		},
		{
			name:  "data scheme",
			input: "data:text/plain;base64,SGVsbG8gV29ybGQ==",
			want:  false,
		},
		{
			name:  "blob scheme",
			input: "blob:https://example.com/blob-id",
			want:  false,
		},
		{
			name:  "javascript scheme",
			input: "javascript:alert('hello')",
			want:  false,
		},
		{
			name:  "vscode scheme",
			input: "vscode://open",
			want:  true,
		},
		{
			name:  "spotify scheme",
			input: "spotify:track:123",
			want:  true,
		},
		{
			name:  "steam scheme",
			input: "steam://run/123",
			want:  true,
		},
		{
			name:  "uppercase HTTP scheme",
			input: "HTTP://example.com",
			want:  false,
		},
		{
			name:  "uppercase ABOUT scheme",
			input: "ABOUT:blank",
			want:  false,
		},
		{
			name:  "uppercase VSCODE scheme",
			input: "VSCODE://open",
			want:  true,
		},
		{
			name:  "malformed URL",
			input: "://invalid",
			want:  false,
		},
		{
			name:  "no scheme",
			input: "example.com",
			want:  false,
		},
		{
			name:  "scheme only",
			input: "custom://",
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsExternalScheme(tt.input)
			if got != tt.want {
				t.Errorf("IsExternalScheme(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractOrigin_ValidURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{
			name:     "https with port",
			uri:      "https://example.com:8443/path?query=1",
			expected: "https://example.com:8443",
		},
		{
			name:     "https without port",
			uri:      "https://example.com/path",
			expected: "https://example.com",
		},
		{
			name:     "http",
			uri:      "http://localhost:8080/app",
			expected: "http://localhost:8080",
		},
		{
			name:     "uppercase host",
			uri:      "https://EXAMPLE.COM/path",
			expected: "https://example.com",
		},
		{
			name:     "uppercase scheme",
			uri:      "HTTPS://example.com/path",
			expected: "https://example.com",
		},
		{
			name:     "https default port omitted",
			uri:      "https://example.com:443/path",
			expected: "https://example.com",
		},
		{
			name:     "http default port omitted",
			uri:      "http://example.com:80/path",
			expected: "http://example.com",
		},
		{
			name:     "https explicit non-default port kept",
			uri:      "https://example.com:8080/path",
			expected: "https://example.com:8080",
		},
		{
			name:     "mixed case with default port",
			uri:      "HTTPS://EXAMPLE.COM:443/path",
			expected: "https://example.com",
		},
		{
			name:     "ipv6 with explicit port",
			uri:      "https://[::1]:8443/path",
			expected: "https://[::1]:8443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origin, err := ExtractOrigin(tt.uri)
			if err != nil {
				t.Errorf("ExtractOrigin(%q) returned error: %v", tt.uri, err)
				return
			}
			if origin != tt.expected {
				t.Errorf("ExtractOrigin(%q) = %q, want %q", tt.uri, origin, tt.expected)
			}
		})
	}
}

func TestExtractOrigin_InvalidURI(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{name: "empty", uri: ""},
		{name: "no scheme", uri: "example.com"},
		{name: "no host", uri: "https://"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origin, err := ExtractOrigin(tt.uri)
			if err == nil {
				t.Errorf("ExtractOrigin(%q) should return error, got origin: %q", tt.uri, origin)
			}
		})
	}
}
