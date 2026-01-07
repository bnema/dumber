package webkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
)

const (
	// dirPerm is the permission mode for creating download directories.
	dirPerm = 0755
)

// DownloadHandler manages WebKit downloads and notifies the UI layer.
type DownloadHandler struct {
	downloadPath string
	eventHandler port.DownloadEventHandler
	mu           sync.RWMutex
}

// NewDownloadHandler creates a new download handler.
func NewDownloadHandler(downloadPath string, handler port.DownloadEventHandler) *DownloadHandler {
	return &DownloadHandler{
		downloadPath: downloadPath,
		eventHandler: handler,
	}
}

// HandleDownload sets up signal handlers for a new download.
func (h *DownloadHandler) HandleDownload(ctx context.Context, download *webkit.Download) {
	log := logging.FromContext(ctx)

	h.mu.RLock()
	downloadPath := h.downloadPath
	eventHandler := h.eventHandler
	h.mu.RUnlock()

	// Track if download failed (to avoid duplicate finished notifications).
	var downloadFailed bool
	var failedMu sync.Mutex

	// Handle decide-destination signal to set download path.
	//nolint:unparam // bool return is required by WebKit's signal signature (false = handled synchronously)
	decideDestCb := func(d webkit.Download, suggestedFilename string) bool {
		// Ensure download directory exists.
		if err := os.MkdirAll(downloadPath, dirPerm); err != nil {
			log.Error().Err(err).Str("path", downloadPath).Msg("failed to create download directory")
			d.Cancel()
			return false
		}

		// Sanitize filename to prevent path traversal attacks.
		safeName := sanitizeFilename(suggestedFilename)
		destPath := filepath.Join(downloadPath, safeName)

		log.Debug().
			Str("suggested", suggestedFilename).
			Str("sanitized", safeName).
			Str("destPath", destPath).
			Msg("setting download destination")

		// WebKit expects an absolute file path for the destination.
		d.SetDestination(destPath)

		// Notify: download started.
		if eventHandler != nil {
			eventHandler.OnDownloadEvent(ctx, port.DownloadEvent{
				Type:        port.DownloadEventStarted,
				Filename:    safeName,
				Destination: destPath,
			})
		}

		log.Info().
			Str("filename", safeName).
			Str("destination", destPath).
			Msg("download started")

		return false // false = we handled it synchronously
	}
	download.ConnectDecideDestination(&decideDestCb)

	// Handle failed signal.
	failedCb := func(d webkit.Download, _ uintptr) {
		failedMu.Lock()
		downloadFailed = true
		failedMu.Unlock()

		dest := d.GetDestination()
		filename := extractFilename(dest)

		// Create error for the failed download.
		downloadErr := fmt.Errorf("download failed: %s", filename)

		if eventHandler != nil {
			eventHandler.OnDownloadEvent(ctx, port.DownloadEvent{
				Type:        port.DownloadEventFailed,
				Filename:    filename,
				Destination: dest,
				Error:       downloadErr,
			})
		}

		log.Warn().Str("filename", filename).Msg("download failed")
	}
	download.ConnectFailed(&failedCb)

	// Handle finished signal.
	finishedCb := func(d webkit.Download) {
		failedMu.Lock()
		failed := downloadFailed
		failedMu.Unlock()

		// Don't send finished event if we already sent a failed event.
		if failed {
			return
		}

		dest := d.GetDestination()
		filename := extractFilename(dest)

		if eventHandler != nil {
			eventHandler.OnDownloadEvent(ctx, port.DownloadEvent{
				Type:        port.DownloadEventFinished,
				Filename:    filename,
				Destination: dest,
			})
		}

		log.Info().Str("filename", filename).Msg("download finished")
	}
	download.ConnectFinished(&finishedCb)
}

// SetDownloadPath updates the download directory.
func (h *DownloadHandler) SetDownloadPath(path string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.downloadPath = path
}

// extractFilename extracts the filename from a file:// URI or path.
func extractFilename(dest string) string {
	// Remove file:// prefix if present.
	path := strings.TrimPrefix(dest, "file://")
	base := filepath.Base(path)

	// Handle edge cases consistently with sanitizeFilename.
	if base == "." || base == "" {
		return "download"
	}
	return base
}

// sanitizeFilename sanitizes a suggested filename to prevent path traversal attacks.
// It extracts only the base name and handles edge cases like "." or "..".
func sanitizeFilename(name string) string {
	// Normalize Windows-style separators to forward slashes.
	// filepath.Base only handles the OS-native separator, so on Linux
	// backslashes would not be treated as path separators.
	name = strings.ReplaceAll(name, "\\", "/")

	// Get only the base name (removes any directory components).
	clean := filepath.Base(name)

	// If Base returns "." or ".." (edge cases), use fallback.
	if clean == "." || clean == ".." || clean == "" {
		return "download"
	}

	return clean
}
