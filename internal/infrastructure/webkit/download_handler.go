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
// The caller (ConfigureDownloads) must validate that preparer is non-nil.
func NewDownloadHandler(
	downloadPath string,
	handler port.DownloadEventHandler,
	preparer port.DownloadPreparer,
) *DownloadHandler {
	return &DownloadHandler{
		runtime: downloadruntime.New(downloadPath, handler, preparer),
	}
}

// downloadState tracks the state of a single download to prevent duplicate notifications.
// This is allocated per-download to ensure proper isolation between concurrent downloads.
type downloadState struct {
	failed              bool
	filename            string
	destination         string
	lastProgressPercent int
	mu                  sync.Mutex
}

// HandleDownload sets up signal handlers for a new download.
func (h *DownloadHandler) HandleDownload(ctx context.Context, download *webkit.Download) {
	if download == nil {
		return
	}

	h.mu.RLock()
	runtime := h.runtime
	h.mu.RUnlock()

	// Track download state in a dedicated struct to ensure proper isolation.
	// Each download gets its own state to prevent interference between concurrent downloads.
	state := &downloadState{lastProgressPercent: -1}

	decideDestCb := makeDecideDestinationCallback(ctx, runtime, state)
	download.ConnectDecideDestination(&decideDestCb)

	progressCb := makeProgressCallback(ctx, runtime, state)
	download.ConnectReceivedData(&progressCb)

	failedCb := makeFailedCallback(ctx, runtime, state)
	download.ConnectFailed(&failedCb)

	finishedCb := makeFinishedCallback(ctx, runtime, state)
	download.ConnectFinished(&finishedCb)
}

func makeDecideDestinationCallback(
	ctx context.Context,
	runtime *downloadruntime.Runtime,
	state *downloadState,
) func(webkit.Download, string) bool {
	log := logging.FromContext(ctx)

	return func(d webkit.Download, suggestedFilename string) bool {
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

		d.SetDestination(output.DestinationPath)

		state.mu.Lock()
		state.filename = output.Filename
		state.destination = output.DestinationPath
		state.lastProgressPercent = -1
		state.mu.Unlock()

		runtime.EmitStarted(ctx, output)
		emitWebKitDownloadProgress(ctx, runtime, state, d)
		return false
	}
}

func makeProgressCallback(
	ctx context.Context,
	runtime *downloadruntime.Runtime,
	state *downloadState,
) func(webkit.Download, uint64) {
	return func(d webkit.Download, _ uint64) {
		emitWebKitDownloadProgress(ctx, runtime, state, d)
	}
}

func makeFailedCallback(
	ctx context.Context,
	runtime *downloadruntime.Runtime,
	state *downloadState,
) func(webkit.Download, *glib.Error) {
	return func(d webkit.Download, gerr *glib.Error) {
		state.mu.Lock()
		state.failed = true
		state.mu.Unlock()

		dest := d.GetDestination()
		filename := downloadutil.ExtractFilenameFromDestination(dest)
		downloadErr := fmt.Errorf("download failed: %s: %s", filename, gerr.MessageGo())
		runtime.EmitFailed(ctx, filename, dest, downloadErr)
	}
}

func makeFinishedCallback(
	ctx context.Context,
	runtime *downloadruntime.Runtime,
	state *downloadState,
) func(webkit.Download) {
	return func(d webkit.Download) {
		state.mu.Lock()
		failed := state.failed
		state.mu.Unlock()
		if failed {
			return
		}

		dest := d.GetDestination()
		filename := downloadutil.ExtractFilenameFromDestination(dest)
		runtime.EmitFinished(ctx, filename, dest)
	}
}

func emitWebKitDownloadProgress(
	ctx context.Context,
	runtime *downloadruntime.Runtime,
	state *downloadState,
	d webkit.Download,
) {
	state.mu.Lock()
	if state.failed {
		state.mu.Unlock()
		return
	}
	filename := state.filename
	destination := state.destination
	lastPercent := state.lastProgressPercent
	state.mu.Unlock()
	if filename == "" || destination == "" {
		return
	}

	progress := d.GetEstimatedProgress()
	if progress <= 0 || progress >= 1 {
		return
	}
	percent := int(progress * 100)
	if percent <= lastPercent {
		return
	}

	state.mu.Lock()
	if state.failed || percent <= state.lastProgressPercent {
		state.mu.Unlock()
		return
	}
	state.lastProgressPercent = percent
	state.mu.Unlock()

	runtime.EmitProgress(ctx, filename, destination, progress, int64(d.GetReceivedDataLength()), 0)
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
