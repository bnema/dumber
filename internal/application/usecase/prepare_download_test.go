package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockDownloadResponse implements port.DownloadResponse for testing.
type mockDownloadResponse struct {
	mimeType          string
	suggestedFilename string
	uri               string
}

func (m *mockDownloadResponse) GetMimeType() string          { return m.mimeType }
func (m *mockDownloadResponse) GetSuggestedFilename() string { return m.suggestedFilename }
func (m *mockDownloadResponse) GetUri() string               { return m.uri }

// mockFileSystem implements port.FileSystem for testing.
type mockFileSystem struct {
	existingFiles map[string]bool
	existsErr     error
}

func (m *mockFileSystem) Exists(_ context.Context, path string) (bool, error) {
	if m.existsErr != nil {
		return false, m.existsErr
	}
	return m.existingFiles[path], nil
}

func (*mockFileSystem) IsDirectory(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (*mockFileSystem) GetSize(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (*mockFileSystem) RemoveAll(_ context.Context, _ string) error {
	return nil
}

func TestPrepareDownloadUseCase_Execute(t *testing.T) {
	ctx := context.Background()
	uc := NewPrepareDownloadUseCase(nil) // nil FileSystem disables deduplication

	tests := []struct {
		name             string
		input            PrepareDownloadInput
		expectedFilename string
		expectedPath     string
	}{
		{
			name: "uses suggested filename directly",
			input: PrepareDownloadInput{
				SuggestedFilename: "document.pdf",
				DownloadDir:       "/tmp/downloads",
			},
			expectedFilename: "document.pdf",
			expectedPath:     "/tmp/downloads/document.pdf",
		},
		{
			name: "sanitizes path traversal in suggested filename",
			input: PrepareDownloadInput{
				SuggestedFilename: "../../../etc/passwd",
				DownloadDir:       "/tmp/downloads",
			},
			expectedFilename: "passwd",
			expectedPath:     "/tmp/downloads/passwd",
		},
		{
			name: "falls back to response suggested filename",
			input: PrepareDownloadInput{
				SuggestedFilename: "",
				Response: &mockDownloadResponse{
					suggestedFilename: "from-header.pdf",
				},
				DownloadDir: "/tmp/downloads",
			},
			expectedFilename: "from-header.pdf",
			expectedPath:     "/tmp/downloads/from-header.pdf",
		},
		{
			name: "falls back to URI path",
			input: PrepareDownloadInput{
				SuggestedFilename: "",
				Response: &mockDownloadResponse{
					suggestedFilename: "",
					uri:               "https://example.com/files/report.pdf",
				},
				DownloadDir: "/tmp/downloads",
			},
			expectedFilename: "report.pdf",
			expectedPath:     "/tmp/downloads/report.pdf",
		},
		{
			name: "adds extension from MIME type when missing",
			input: PrepareDownloadInput{
				SuggestedFilename: "report",
				Response: &mockDownloadResponse{
					mimeType: "application/pdf",
				},
				DownloadDir: "/tmp/downloads",
			},
			expectedFilename: "report.pdf",
			expectedPath:     "/tmp/downloads/report.pdf",
		},
		{
			name: "preserves existing extension even with MIME type",
			input: PrepareDownloadInput{
				SuggestedFilename: "report.doc",
				Response: &mockDownloadResponse{
					mimeType: "application/pdf",
				},
				DownloadDir: "/tmp/downloads",
			},
			expectedFilename: "report.doc",
			expectedPath:     "/tmp/downloads/report.doc",
		},
		{
			name: "handles nil response",
			input: PrepareDownloadInput{
				SuggestedFilename: "file.txt",
				Response:          nil,
				DownloadDir:       "/tmp/downloads",
			},
			expectedFilename: "file.txt",
			expectedPath:     "/tmp/downloads/file.txt",
		},
		{
			name: "uses default filename for empty inputs",
			input: PrepareDownloadInput{
				SuggestedFilename: "",
				Response:          nil,
				DownloadDir:       "/tmp/downloads",
			},
			expectedFilename: "download",
			expectedPath:     "/tmp/downloads/download",
		},
		{
			name: "sanitizes windows path traversal",
			input: PrepareDownloadInput{
				SuggestedFilename: "..\\..\\Windows\\System32\\config",
				DownloadDir:       "/tmp/downloads",
			},
			expectedFilename: "config",
			expectedPath:     "/tmp/downloads/config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := uc.Execute(ctx, tt.input)

			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedFilename, result.Filename)
			assert.Equal(t, tt.expectedPath, result.DestinationPath)
		})
	}
}

func TestPrepareDownloadUseCase_ResolvePriority(t *testing.T) {
	ctx := context.Background()
	uc := NewPrepareDownloadUseCase(nil)

	t.Run("suggested filename takes priority over response", func(t *testing.T) {
		input := PrepareDownloadInput{
			SuggestedFilename: "priority.pdf",
			Response: &mockDownloadResponse{
				suggestedFilename: "response.pdf",
				uri:               "https://example.com/uri.pdf",
			},
			DownloadDir: "/tmp",
		}

		result := uc.Execute(ctx, input)
		assert.Equal(t, "priority.pdf", result.Filename)
	})

	t.Run("response suggested takes priority over URI", func(t *testing.T) {
		input := PrepareDownloadInput{
			SuggestedFilename: "",
			Response: &mockDownloadResponse{
				suggestedFilename: "response.pdf",
				uri:               "https://example.com/uri.pdf",
			},
			DownloadDir: "/tmp",
		}

		result := uc.Execute(ctx, input)
		assert.Equal(t, "response.pdf", result.Filename)
	})
}

func TestPrepareDownloadUseCase_Deduplication(t *testing.T) {
	ctx := context.Background()

	t.Run("existing file triggers _(1) suffix", func(t *testing.T) {
		fs := &mockFileSystem{
			existingFiles: map[string]bool{
				"/tmp/downloads/document.pdf": true,
			},
		}
		uc := NewPrepareDownloadUseCase(fs)

		input := PrepareDownloadInput{
			SuggestedFilename: "document.pdf",
			DownloadDir:       "/tmp/downloads",
		}

		result := uc.Execute(ctx, input)
		assert.Equal(t, "document_(1).pdf", result.Filename)
		assert.Equal(t, "/tmp/downloads/document_(1).pdf", result.DestinationPath)
	})

	t.Run("multiple existing files increment suffix", func(t *testing.T) {
		fs := &mockFileSystem{
			existingFiles: map[string]bool{
				"/tmp/downloads/document.pdf":     true,
				"/tmp/downloads/document_(1).pdf": true,
			},
		}
		uc := NewPrepareDownloadUseCase(fs)

		input := PrepareDownloadInput{
			SuggestedFilename: "document.pdf",
			DownloadDir:       "/tmp/downloads",
		}

		result := uc.Execute(ctx, input)
		assert.Equal(t, "document_(2).pdf", result.Filename)
		assert.Equal(t, "/tmp/downloads/document_(2).pdf", result.DestinationPath)
	})

	t.Run("fs.Exists error assumes file exists (conservative)", func(t *testing.T) {
		fs := &mockFileSystem{
			existingFiles: map[string]bool{},
			existsErr:     errors.New("permission denied"),
		}
		uc := NewPrepareDownloadUseCase(fs)

		input := PrepareDownloadInput{
			SuggestedFilename: "document.pdf",
			DownloadDir:       "/tmp/downloads",
		}

		result := uc.Execute(ctx, input)
		// When fs.Exists returns an error, we conservatively assume file exists
		// This triggers deduplication to use timestamp fallback (after 1000 attempts)
		assert.NotEqual(t, "document.pdf", result.Filename)
		assert.Contains(t, result.Filename, "document_")
	})

	t.Run("non-existing file uses original name", func(t *testing.T) {
		fs := &mockFileSystem{
			existingFiles: map[string]bool{},
		}
		uc := NewPrepareDownloadUseCase(fs)

		input := PrepareDownloadInput{
			SuggestedFilename: "document.pdf",
			DownloadDir:       "/tmp/downloads",
		}

		result := uc.Execute(ctx, input)
		assert.Equal(t, "document.pdf", result.Filename)
		assert.Equal(t, "/tmp/downloads/document.pdf", result.DestinationPath)
	})
}
