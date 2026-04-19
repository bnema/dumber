package cef

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
	downloadutil "github.com/bnema/dumber/internal/domain/download"
	"github.com/bnema/dumber/internal/infrastructure/downloadruntime"
	"github.com/bnema/dumber/internal/logging"
)

type downloadHandler struct {
	runtime *downloadruntime.Runtime
	mu      sync.Mutex
	active  map[uint32]cefDownloadState
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
	return &downloadHandler{
		runtime: downloadruntime.New(downloadPath, eventHandler, preparer),
		active:  make(map[uint32]cefDownloadState),
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
	output, err := h.runtime.ResolveDestination(ctx, suggestedName, &cefDownloadResponseAdapter{item: downloadItem})
	if err != nil {
		log.Error().Err(err).Msg("cef: failed to prepare download destination")
		callback.Cont("", 1)
		return true
	}

	h.mu.Lock()
	h.active[downloadItem.GetID()] = cefDownloadState{
		filename:    output.Filename,
		destination: output.DestinationPath,
	}
	h.mu.Unlock()

	callback.Cont(output.DestinationPath, 0)
	h.runtime.EmitStarted(ctx, output)

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

	id := downloadItem.GetID()
	state := h.currentState(id, downloadItem)

	switch {
	case downloadItem.IsComplete():
		if !h.markFinished(id, state) {
			return
		}
		h.runtime.EmitFinished(ctx, state.filename, state.destination)
	case downloadItem.IsCanceled() || downloadItem.IsInterrupted():
		if !h.markFinished(id, state) {
			return
		}
		var err error
		if downloadItem.IsInterrupted() {
			err = fmt.Errorf("download interrupted: %s (%v)", state.filename, downloadItem.GetInterruptReason())
		} else {
			err = fmt.Errorf("download canceled: %s", state.filename)
		}
		h.runtime.EmitFailed(ctx, state.filename, state.destination, err)
	}
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
