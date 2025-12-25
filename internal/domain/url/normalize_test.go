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
