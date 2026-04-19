package downloadruntime

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

const dirPerm = 0o755

type codedDownloadError interface {
	DownloadErrorCode() int
	DownloadErrorReason() string
}

// Runtime shares common download preparation and event emission logic across engines.
type Runtime struct {
	mu           sync.RWMutex
	downloadPath string
	eventHandler port.DownloadEventHandler
	preparer     port.DownloadPreparer
}

// New creates a Runtime. The caller must ensure preparer is non-nil;
// ConfigureDownloads validates this before reaching here.
func New(downloadPath string, eventHandler port.DownloadEventHandler, preparer port.DownloadPreparer) *Runtime {
	return &Runtime{
		downloadPath: downloadPath,
		eventHandler: eventHandler,
		preparer:     preparer,
	}
}

func (r *Runtime) SetDownloadPath(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.downloadPath = path
}

func (r *Runtime) ResolveDestination(
	ctx context.Context,
	suggestedFilename string,
	response port.DownloadResponse,
) (*port.DownloadPrepareOutput, error) {
	r.mu.RLock()
	downloadPath := r.downloadPath
	preparer := r.preparer
	r.mu.RUnlock()

	if err := os.MkdirAll(downloadPath, dirPerm); err != nil {
		return nil, fmt.Errorf("failed to create download directory %q: %w", downloadPath, err)
	}

	output := preparer.Execute(ctx, port.DownloadPrepareInput{
		SuggestedFilename: suggestedFilename,
		Response:          response,
		DownloadDir:       downloadPath,
	})
	if output == nil {
		return nil, fmt.Errorf("failed to prepare download destination for %q", suggestedFilename)
	}

	return output, nil
}

func (r *Runtime) EmitStarted(ctx context.Context, output *port.DownloadPrepareOutput) {
	if output == nil {
		return
	}

	r.mu.RLock()
	eventHandler := r.eventHandler
	r.mu.RUnlock()

	if eventHandler != nil {
		eventHandler.OnDownloadEvent(ctx, port.DownloadEvent{
			Type:        port.DownloadEventStarted,
			Filename:    output.Filename,
			Destination: output.DestinationPath,
		})
	}

	logging.FromContext(ctx).Info().
		Str("filename", output.Filename).
		Str("destination", output.DestinationPath).
		Msg("download started")
}

func (r *Runtime) EmitFinished(ctx context.Context, filename, destination string) {
	r.mu.RLock()
	eventHandler := r.eventHandler
	r.mu.RUnlock()

	if eventHandler != nil {
		eventHandler.OnDownloadEvent(ctx, port.DownloadEvent{
			Type:        port.DownloadEventFinished,
			Filename:    filename,
			Destination: destination,
		})
	}

	logging.FromContext(ctx).Info().
		Str("filename", filename).
		Str("destination", destination).
		Msg("download finished")
}

func (r *Runtime) EmitFailed(ctx context.Context, filename, destination string, err error) {
	r.mu.RLock()
	eventHandler := r.eventHandler
	r.mu.RUnlock()

	if eventHandler != nil {
		eventHandler.OnDownloadEvent(ctx, port.DownloadEvent{
			Type:        port.DownloadEventFailed,
			Filename:    filename,
			Destination: destination,
			Error:       err,
		})
	}

	log := logging.FromContext(ctx).Warn().
		Err(err).
		Str("filename", filename).
		Str("destination", destination)
	if codedErr, ok := err.(codedDownloadError); ok {
		log = log.
			Int("error_code", codedErr.DownloadErrorCode()).
			Str("error_reason", codedErr.DownloadErrorReason())
	}
	log.Msg("download failed")
}
