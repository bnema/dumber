package webkit

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	downloadutil "github.com/bnema/dumber/internal/domain/download"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
)

const (
	// dirPerm is the permission mode for creating download directories.
	dirPerm = 0755
)

// DownloadHandler manages WebKit downloads and notifies the UI layer.
type DownloadHandler struct {
	downloadPath      string
	eventHandler      port.DownloadEventHandler
	prepareDownloadUC *usecase.PrepareDownloadUseCase
	mu                sync.RWMutex
}

// NewDownloadHandler creates a new download handler.
// Panics if prepareDownloadUC is nil (fail fast on misconfiguration).
func NewDownloadHandler(
	downloadPath string,
	handler port.DownloadEventHandler,
	prepareDownloadUC *usecase.PrepareDownloadUseCase,
) *DownloadHandler {
	if prepareDownloadUC == nil {
		panic("prepareDownloadUC is required")
	}
	return &DownloadHandler{
		downloadPath:      downloadPath,
		eventHandler:      handler,
		prepareDownloadUC: prepareDownloadUC,
	}
}

// downloadState tracks the state of a single download to prevent duplicate notifications.
// This is allocated per-download to ensure proper isolation between concurrent downloads.
type downloadState struct {
	failed bool
	mu     sync.Mutex
}

// HandleDownload sets up signal handlers for a new download.
func (h *DownloadHandler) HandleDownload(ctx context.Context, download *webkit.Download) {
	log := logging.FromContext(ctx)

	h.mu.RLock()
	downloadPath := h.downloadPath
	eventHandler := h.eventHandler
	prepareDownloadUC := h.prepareDownloadUC
	h.mu.RUnlock()

	// Track download state in a dedicated struct to ensure proper isolation.
	// Each download gets its own state to prevent interference between concurrent downloads.
	state := &downloadState{}

	// Handle decide-destination signal to set download path.
	//nolint:unparam // bool return is required by WebKit's signal signature (false = handled synchronously)
	decideDestCb := func(d webkit.Download, suggestedFilename string) bool {
		// Ensure download directory exists.
		if err := os.MkdirAll(downloadPath, dirPerm); err != nil {
			log.Error().Err(err).Str("path", downloadPath).Msg("failed to create download directory")
			d.Cancel()
			return false
		}

		// Wrap WebKit response as port.DownloadResponse
		var response port.DownloadResponse
		if resp := d.GetResponse(); resp != nil {
			response = &uriResponseAdapter{resp: resp}
		}

		// Use PrepareDownloadUseCase to resolve filename
		output := prepareDownloadUC.Execute(ctx, usecase.PrepareDownloadInput{
			SuggestedFilename: suggestedFilename,
			Response:          response,
			DownloadDir:       downloadPath,
		})

		log.Debug().
			Str("suggested", suggestedFilename).
			Str("sanitized", output.Filename).
			Str("destPath", output.DestinationPath).
			Msg("setting download destination")

		// WebKit expects an absolute file path for the destination.
		d.SetDestination(output.DestinationPath)

		// Notify: download started.
		if eventHandler != nil {
			eventHandler.OnDownloadEvent(ctx, port.DownloadEvent{
				Type:        port.DownloadEventStarted,
				Filename:    output.Filename,
				Destination: output.DestinationPath,
			})
		}

		log.Info().
			Str("filename", output.Filename).
			Str("destination", output.DestinationPath).
			Msg("download started")

		return false // false = we handled it synchronously
	}
	download.ConnectDecideDestination(&decideDestCb)

	// Handle failed signal.
	failedCb := func(d webkit.Download, _ uintptr) {
		state.mu.Lock()
		state.failed = true
		state.mu.Unlock()

		dest := d.GetDestination()
		filename := downloadutil.ExtractFilenameFromDestination(dest)

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
		state.mu.Lock()
		failed := state.failed
		state.mu.Unlock()

		// Don't send finished event if we already sent a failed event.
		if failed {
			return
		}

		dest := d.GetDestination()
		filename := downloadutil.ExtractFilenameFromDestination(dest)

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

// uriResponseAdapter wraps webkit.URIResponse to implement port.DownloadResponse.
type uriResponseAdapter struct {
	resp *webkit.URIResponse
}

func (a *uriResponseAdapter) GetMimeType() string {
	if a.resp == nil {
		return ""
	}
	return a.resp.GetMimeType()
}

func (a *uriResponseAdapter) GetSuggestedFilename() string {
	if a.resp == nil {
		return ""
	}
	return a.resp.GetSuggestedFilename()
}

func (a *uriResponseAdapter) GetUri() string {
	if a.resp == nil {
		return ""
	}
	return a.resp.GetUri()
}
