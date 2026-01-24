package download

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal filename",
			input:    "document.pdf",
			expected: "document.pdf",
		},
		{
			name:     "filename with spaces",
			input:    "my document.pdf",
			expected: "my document.pdf",
		},
		{
			name:     "path traversal with parent dirs",
			input:    "../../../etc/passwd",
			expected: "passwd",
		},
		{
			name:     "path traversal hidden file",
			input:    "../.ssh/id_rsa",
			expected: "id_rsa",
		},
		{
			name:     "nested path",
			input:    "foo/bar/baz.txt",
			expected: "baz.txt",
		},
		{
			name:     "absolute path",
			input:    "/etc/passwd",
			expected: "passwd",
		},
		{
			name:     "dot only",
			input:    ".",
			expected: "download",
		},
		{
			name:     "double dot only",
			input:    "..",
			expected: "download",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "download",
		},
		{
			name:     "hidden file",
			input:    ".bashrc",
			expected: ".bashrc",
		},
		{
			name:     "windows style path",
			input:    "..\\..\\Windows\\System32\\config",
			expected: "config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeFilename(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeFilenameWithExtension(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		mimeType string
		expected string
	}{
		{
			name:     "no extension adds inferred",
			input:    "download",
			mimeType: "application/pdf",
			expected: "download.pdf",
		},
		{
			name:     "existing extension preserved",
			input:    "report.pdf",
			mimeType: "application/pdf",
			expected: "report.pdf",
		},
		{
			name:     "path traversal with inferred extension",
			input:    "../report",
			mimeType: "application/pdf",
			expected: "report.pdf",
		},
		{
			name:     "no mime type returns sanitized",
			input:    "report",
			mimeType: "",
			expected: "report",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeFilenameWithExtension(tt.input, tt.mimeType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetExtensionFromMimeType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty mime type",
			input:    "",
			expected: "",
		},
		{
			name:     "unknown mime type",
			input:    "application/unknown",
			expected: "",
		},
		{
			name:     "pdf mime type",
			input:    "application/pdf",
			expected: ".pdf",
		},
		{
			name:     "pdf mime type with charset param",
			input:    "application/pdf; charset=binary",
			expected: ".pdf",
		},
		{
			name:     "text html with charset",
			input:    "text/html; charset=utf-8",
			expected: ".htm",
		},
		{
			name:     "mime type with multiple params",
			input:    "application/json; charset=utf-8; boundary=something",
			expected: ".json",
		},
		{
			name:     "invalid mime type",
			input:    "not-a-valid-mime",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetExtensionFromMimeType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractFilenameFromDestination(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "file URI with path",
			input:    "file:///home/user/Downloads/document.pdf",
			expected: "document.pdf",
		},
		{
			name:     "file URI root",
			input:    "file:///document.pdf",
			expected: "document.pdf",
		},
		{
			name:     "plain path",
			input:    "/home/user/Downloads/image.png",
			expected: "image.png",
		},
		{
			name:     "filename only",
			input:    "video.mp4",
			expected: "video.mp4",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "download",
		},
		{
			name:     "file URI with spaces",
			input:    "file:///home/user/My Documents/file.txt",
			expected: "file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractFilenameFromDestination(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractFilenameFromURI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "http URL with filename",
			input:    "https://example.com/files/document.pdf",
			expected: "document.pdf",
		},
		{
			name:     "http URL with query params",
			input:    "https://example.com/files/document.pdf?token=abc",
			expected: "document.pdf",
		},
		{
			name:     "http URL without filename",
			input:    "https://example.com/",
			expected: "download",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "download",
		},
		{
			name:     "plain path",
			input:    "/path/to/file.txt",
			expected: "file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractFilenameFromURI(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMakeUniqueFilename(t *testing.T) {
	tests := []struct {
		name          string
		dir           string
		filename      string
		existingFiles map[string]bool
		expected      string
	}{
		{
			name:          "file does not exist",
			dir:           "/tmp",
			filename:      "document.pdf",
			existingFiles: map[string]bool{},
			expected:      "document.pdf",
		},
		{
			name:     "file exists, adds _(1)",
			dir:      "/tmp",
			filename: "document.pdf",
			existingFiles: map[string]bool{
				"/tmp/document.pdf": true,
			},
			expected: "document_(1).pdf",
		},
		{
			name:     "file and _(1) exist, adds _(2)",
			dir:      "/tmp",
			filename: "document.pdf",
			existingFiles: map[string]bool{
				"/tmp/document.pdf":     true,
				"/tmp/document_(1).pdf": true,
			},
			expected: "document_(2).pdf",
		},
		{
			name:     "no extension",
			dir:      "/tmp",
			filename: "download",
			existingFiles: map[string]bool{
				"/tmp/download": true,
			},
			expected: "download_(1)",
		},
		{
			name:     "hidden file with extension",
			dir:      "/tmp",
			filename: ".config.bak",
			existingFiles: map[string]bool{
				"/tmp/.config.bak": true,
			},
			expected: ".config_(1).bak",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists := func(path string) bool {
				return tt.existingFiles[path]
			}
			result := MakeUniqueFilename(tt.dir, tt.filename, exists)
			assert.Equal(t, tt.expected, result)
		})
	}
}
