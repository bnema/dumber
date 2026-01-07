package webkit

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
			result := sanitizeFilename(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractFilename(t *testing.T) {
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
			result := extractFilename(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewDownloadHandler(t *testing.T) {
	t.Run("creates handler with path and nil event handler", func(t *testing.T) {
		handler := NewDownloadHandler("/tmp/downloads", nil)

		assert.NotNil(t, handler)
		assert.Equal(t, "/tmp/downloads", handler.downloadPath)
		assert.Nil(t, handler.eventHandler)
	})

	t.Run("creates handler with custom path", func(t *testing.T) {
		handler := NewDownloadHandler("/custom/path", nil)

		assert.Equal(t, "/custom/path", handler.downloadPath)
	})
}

func TestDownloadHandler_SetDownloadPath(t *testing.T) {
	handler := NewDownloadHandler("/initial/path", nil)

	handler.SetDownloadPath("/new/path")

	assert.Equal(t, "/new/path", handler.downloadPath)
}
