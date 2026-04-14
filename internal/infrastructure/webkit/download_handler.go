package webkit

import (
	"context"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	downloadutil "github.com/bnema/dumber/internal/domain/download"
	"github.com/bnema/dumber/internal/infrastructure/downloadruntime"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/webkit"
)

// DownloadHandler manages WebKit downloads and notifies the UI layer.
type DownloadHandler struct {
	runtime *downloadruntime.Runtime
	mu      sync.RWMutex
}

// NewDownloadHandler creates a new download handler.
// Panics if preparer is nil (fail fast on misconfiguration).
func NewDownloadHandler(
	downloadPath string,
	handler port.DownloadEventHandler,
	preparer port.DownloadPreparer,
) *DownloadHandler {
	if preparer == nil {
		panic("preparer is required")
	}
	return &DownloadHandler{
		runtime: downloadruntime.New(downloadPath, handler, preparer),
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
	runtime := h.runtime
	h.mu.RUnlock()

	// Track download state in a dedicated struct to ensure proper isolation.
	// Each download gets its own state to prevent interference between concurrent downloads.
	state := &downloadState{}

	// Handle decide-destination signal to set download path.
	//nolint:unparam // bool return is required by WebKit's signal signature (false = handled synchronously)
	decideDestCb := func(d webkit.Download, suggestedFilename string) bool {
		var response port.DownloadResponse
		if resp := d.GetResponse(); resp != nil {
			response = &uriResponseAdapter{resp: resp}
		}

		output, err := runtime.ResolveDestination(ctx, suggestedFilename, response)
		if err != nil {
			log.Error().Err(err).Msg("failed to prepare download destination")
			d.Cancel()
			return false
		}

		log.Debug().
			Str("suggested", suggestedFilename).
			Str("sanitized", output.Filename).
			Str("destPath", output.DestinationPath).
			Msg("setting download destination")

		// WebKit expects an absolute file path for the destination.
		d.SetDestination(output.DestinationPath)

		runtime.EmitStarted(ctx, output)

		return false // false = we handled it synchronously
	}
	download.ConnectDecideDestination(&decideDestCb)

	// Handle failed signal.
	failedCb := func(d webkit.Download, gerr *glib.Error) {
		state.mu.Lock()
		state.failed = true
		state.mu.Unlock()

		dest := d.GetDestination()
		filename := downloadutil.ExtractFilenameFromDestination(dest)

		// Create error for the failed download.
		downloadErr := fmt.Errorf("download failed: %s: %s", filename, gerr.MessageGo())

		runtime.EmitFailed(ctx, filename, dest, downloadErr)
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

		runtime.EmitFinished(ctx, filename, dest)
	}
	download.ConnectFinished(&finishedCb)
}

// SetDownloadPath updates the download directory.
func (h *DownloadHandler) SetDownloadPath(path string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.runtime.SetDownloadPath(path)
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
