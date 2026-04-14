package cef

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
	downloadutil "github.com/bnema/dumber/internal/domain/download"
	"github.com/bnema/dumber/internal/logging"
)

const cefDownloadDirPerm = 0o755

type downloadHandler struct {
	downloadPath string
	eventHandler port.DownloadEventHandler
	preparer     port.DownloadPreparer

	mu     sync.Mutex
	active map[uint32]cefDownloadState
}

type cefDownloadState struct {
	filename    string
	destination string
	finished    bool
}

type cefDownloadResponseAdapter struct {
	item purecef.DownloadItem
}

func newDownloadHandler(
	downloadPath string,
	eventHandler port.DownloadEventHandler,
	preparer port.DownloadPreparer,
) *downloadHandler {
	if preparer == nil {
		panic("preparer is required")
	}

	return &downloadHandler{
		downloadPath: downloadPath,
		eventHandler: eventHandler,
		preparer:     preparer,
		active:       make(map[uint32]cefDownloadState),
	}
}

func (h *downloadHandler) canDownload(_ purecef.Browser, _ string, requestMethod string) bool {
	return requestMethod == "" || strings.EqualFold(requestMethod, http.MethodGet)
}

func (h *downloadHandler) onBeforeDownload(
	ctx context.Context,
	_ purecef.Browser,
	downloadItem purecef.DownloadItem,
	suggestedName string,
	callback purecef.BeforeDownloadCallback,
) bool {
	if downloadItem == nil || callback == nil {
		return false
	}

	log := logging.FromContext(ctx)
	downloadPath, eventHandler, preparer := h.snapshot()
	if err := os.MkdirAll(downloadPath, cefDownloadDirPerm); err != nil {
		log.Error().Err(err).Str("path", downloadPath).Msg("cef: failed to create download directory")
		callback.Cont("", 1)
		return true
	}

	output := preparer.Execute(ctx, port.DownloadPrepareInput{
		SuggestedFilename: suggestedName,
		Response:          &cefDownloadResponseAdapter{item: downloadItem},
		DownloadDir:       downloadPath,
	})

	h.mu.Lock()
	h.active[downloadItem.GetID()] = cefDownloadState{
		filename:    output.Filename,
		destination: output.DestinationPath,
	}
	h.mu.Unlock()

	callback.Cont(output.DestinationPath, 0)

	if eventHandler != nil {
		eventHandler.OnDownloadEvent(ctx, port.DownloadEvent{
			Type:        port.DownloadEventStarted,
			Filename:    output.Filename,
			Destination: output.DestinationPath,
		})
	}

	log.Info().
		Uint32("download_id", downloadItem.GetID()).
		Str("filename", output.Filename).
		Str("destination", output.DestinationPath).
		Msg("cef: download started")

	return true
}

func (h *downloadHandler) onDownloadUpdated(
	ctx context.Context,
	downloadItem purecef.DownloadItem,
	callback purecef.DownloadItemCallback,
) {
	_ = callback
	if downloadItem == nil {
		return
	}

	log := logging.FromContext(ctx)
	id := downloadItem.GetID()
	state := h.currentState(id, downloadItem)

	switch {
	case downloadItem.IsComplete():
		if !h.markFinished(id, state) {
			return
		}
		if h.eventHandler != nil {
			h.eventHandler.OnDownloadEvent(ctx, port.DownloadEvent{
				Type:        port.DownloadEventFinished,
				Filename:    state.filename,
				Destination: state.destination,
			})
		}
		log.Info().
			Uint32("download_id", id).
			Str("filename", state.filename).
			Msg("cef: download finished")
	case downloadItem.IsCanceled() || downloadItem.IsInterrupted():
		if !h.markFinished(id, state) {
			return
		}
		err := fmt.Errorf("download failed: %s", state.filename)
		if downloadItem.IsInterrupted() {
			err = fmt.Errorf("download interrupted: %s (%s)", state.filename, downloadItem.GetInterruptReason())
		} else if downloadItem.IsCanceled() {
			err = fmt.Errorf("download canceled: %s", state.filename)
		}
		if h.eventHandler != nil {
			h.eventHandler.OnDownloadEvent(ctx, port.DownloadEvent{
				Type:        port.DownloadEventFailed,
				Filename:    state.filename,
				Destination: state.destination,
				Error:       err,
			})
		}
		log.Warn().
			Uint32("download_id", id).
			Err(err).
			Msg("cef: download failed")
	}
}

func (h *downloadHandler) snapshot() (string, port.DownloadEventHandler, port.DownloadPreparer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.downloadPath, h.eventHandler, h.preparer
}

func (h *downloadHandler) currentState(id uint32, item purecef.DownloadItem) cefDownloadState {
	h.mu.Lock()
	defer h.mu.Unlock()

	state, ok := h.active[id]
	if !ok {
		state = cefDownloadState{}
	}
	if state.destination == "" {
		state.destination = item.GetFullPath()
	}
	if state.filename == "" {
		state.filename = downloadFilenameFromItem(item, state.destination)
	}
	h.active[id] = state
	return state
}

func (h *downloadHandler) markFinished(id uint32, state cefDownloadState) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	current, ok := h.active[id]
	if !ok {
		state.finished = true
		h.active[id] = state
		return true
	}
	if current.finished {
		return false
	}
	state.finished = true
	h.active[id] = state
	return true
}

func downloadFilenameFromItem(item purecef.DownloadItem, destination string) string {
	if destination != "" {
		return downloadutil.ExtractFilenameFromDestination(destination)
	}
	if item == nil {
		return downloadutil.DefaultFilename
	}
	if suggested := item.GetSuggestedFileName(); suggested != "" {
		return downloadutil.SanitizeFilenameWithExtension(suggested, item.GetMimeType())
	}
	if uri := item.GetURL(); uri != "" {
		return downloadutil.ExtractFilenameFromURI(uri)
	}
	return downloadutil.DefaultFilename
}

func (a *cefDownloadResponseAdapter) GetMimeType() string {
	if a == nil || a.item == nil {
		return ""
	}
	return a.item.GetMimeType()
}

func (a *cefDownloadResponseAdapter) GetSuggestedFilename() string {
	if a == nil || a.item == nil {
		return ""
	}
	return a.item.GetSuggestedFileName()
}

func (a *cefDownloadResponseAdapter) GetUri() string {
	if a == nil || a.item == nil {
		return ""
	}
	return a.item.GetURL()
}
