package webkit

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
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
	decideDestCb := func(d webkit.Download, suggestedFilename string) bool {
		destPath := filepath.Join(downloadPath, suggestedFilename)
		// WebKit expects a file:// URI for the destination.
		d.SetDestination("file://" + destPath)

		// Notify: download started.
		if eventHandler != nil {
			eventHandler.OnDownloadEvent(ctx, port.DownloadEvent{
				Type:        port.DownloadEventStarted,
				Filename:    suggestedFilename,
				Destination: destPath,
			})
		}

		log.Info().
			Str("filename", suggestedFilename).
			Str("destination", destPath).
			Msg("download started")

		return false // false = we handled it synchronously
	}
	download.ConnectDecideDestination(&decideDestCb)

	// Handle failed signal.
	failedCb := func(d webkit.Download, errPtr uintptr) {
		failedMu.Lock()
		downloadFailed = true
		failedMu.Unlock()

		dest := d.GetDestination()
		filename := extractFilename(dest)

		if eventHandler != nil {
			eventHandler.OnDownloadEvent(ctx, port.DownloadEvent{
				Type:        port.DownloadEventFailed,
				Filename:    filename,
				Destination: dest,
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
	return filepath.Base(path)
}
