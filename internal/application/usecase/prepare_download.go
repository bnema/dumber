package usecase

import (
	"context"
	"path/filepath"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/download"
	"github.com/bnema/dumber/internal/logging"
)

// PrepareDownloadInput contains the inputs for preparing a download destination.
type PrepareDownloadInput struct {
	// SuggestedFilename is the filename suggested by WebKit (from Content-Disposition or other sources).
	SuggestedFilename string
	// Response provides additional metadata (MIME type, URI) for filename resolution.
	// May be nil if not available.
	Response port.DownloadResponse
	// DownloadDir is the directory where downloads should be saved.
	DownloadDir string
}

// PrepareDownloadOutput contains the resolved download destination.
type PrepareDownloadOutput struct {
	// Filename is the sanitized, safe filename to use.
	Filename string
	// DestinationPath is the full path where the download should be saved.
	DestinationPath string
}

// PrepareDownloadUseCase orchestrates filename resolution for downloads.
// It combines multiple sources (suggested name, response headers, URI) to
// determine the best filename, then sanitizes it to prevent path traversal.
type PrepareDownloadUseCase struct {
	fs port.FileSystem
}

// NewPrepareDownloadUseCase creates a new PrepareDownloadUseCase.
// If fs is nil, filename deduplication is disabled.
func NewPrepareDownloadUseCase(fs port.FileSystem) *PrepareDownloadUseCase {
	return &PrepareDownloadUseCase{fs: fs}
}

// Execute resolves the download filename and destination path.
func (u *PrepareDownloadUseCase) Execute(ctx context.Context, input PrepareDownloadInput) *PrepareDownloadOutput {
	log := logging.FromContext(ctx)

	// Resolve filename from multiple sources (priority: suggested > response suggested > URI)
	resolvedName := u.resolveSuggestedFilename(input.SuggestedFilename, input.Response)

	// Get MIME type for extension inference
	mimeType := ""
	if input.Response != nil {
		mimeType = input.Response.GetMimeType()
	}

	// Sanitize filename and add extension if needed
	safeName := download.SanitizeFilenameWithExtension(resolvedName, mimeType)

	// Make filename unique if file already exists
	if u.fs != nil {
		safeName = download.MakeUniqueFilename(input.DownloadDir, safeName, func(path string) bool {
			exists, err := u.fs.Exists(ctx, path)
			return err == nil && exists
		})
	}

	destPath := filepath.Join(input.DownloadDir, safeName)

	log.Debug().
		Str("suggested", input.SuggestedFilename).
		Str("resolved", resolvedName).
		Str("sanitized", safeName).
		Str("destPath", destPath).
		Msg("prepared download destination")

	return &PrepareDownloadOutput{
		Filename:        safeName,
		DestinationPath: destPath,
	}
}

// resolveSuggestedFilename determines the best filename from available sources.
func (*PrepareDownloadUseCase) resolveSuggestedFilename(name string, response port.DownloadResponse) string {
	// Priority 1: Use provided suggested filename
	if name != "" {
		return name
	}

	if response == nil {
		return name
	}

	// Priority 2: Use response's suggested filename (from Content-Disposition)
	if suggested := response.GetSuggestedFilename(); suggested != "" {
		return suggested
	}

	// Priority 3: Extract from URI path
	uri := response.GetUri()
	if uri == "" {
		return name
	}

	return download.ExtractFilenameFromURI(uri)
}
