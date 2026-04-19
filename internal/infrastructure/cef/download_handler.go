package cef

import (
	"context"
	"fmt"
	"sync"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
	downloadutil "github.com/bnema/dumber/internal/domain/download"
	"github.com/bnema/dumber/internal/infrastructure/downloadruntime"
	"github.com/bnema/dumber/internal/logging"
)

// maxFinishedEntries caps the finished-ID set. When the cap is reached the set
// is cleared, accepting a tiny risk of a duplicate terminal event for very old
// downloads. In practice download IDs are monotonically increasing uint32s and
// no user session will approach this limit.
const maxFinishedEntries = 1024

type downloadHandler struct {
	runtime  *downloadruntime.Runtime
	mu       sync.Mutex
	active   map[uint32]cefDownloadState
	finished map[uint32]struct{} // IDs that already emitted a terminal event
}

type cefDownloadState struct {
	filename            string
	destination         string
	lastProgressPercent int32
}

type cefDownloadInterruptError struct {
	filename string
	reason   purecef.DownloadInterruptReason
}

func (e cefDownloadInterruptError) Error() string {
	return fmt.Sprintf(
		"download interrupted: %s (code=%d, reason=%s)",
		e.filename,
		e.reason,
		downloadInterruptReasonLabel(e.reason),
	)
}

func (e cefDownloadInterruptError) DownloadErrorCode() int {
	return int(e.reason)
}

func (e cefDownloadInterruptError) DownloadErrorReason() string {
	return downloadInterruptReasonLabel(e.reason)
}

type cefDownloadResponseAdapter struct {
	item purecef.DownloadItem
}

var downloadInterruptReasonLabels = map[purecef.DownloadInterruptReason]string{
	purecef.DownloadInterruptReasonNone:                        "none",
	purecef.DownloadInterruptReasonFileFailed:                  "file_failed",
	purecef.DownloadInterruptReasonFileAccessDenied:            "file_access_denied",
	purecef.DownloadInterruptReasonFileNoSpace:                 "file_no_space",
	purecef.DownloadInterruptReasonFileNameTooLong:             "file_name_too_long",
	purecef.DownloadInterruptReasonFileTooLarge:                "file_too_large",
	purecef.DownloadInterruptReasonFileVirusInfected:           "file_virus_infected",
	purecef.DownloadInterruptReasonFileTransientError:          "file_transient_error",
	purecef.DownloadInterruptReasonFileBlocked:                 "file_blocked",
	purecef.DownloadInterruptReasonFileSecurityCheckFailed:     "file_security_check_failed",
	purecef.DownloadInterruptReasonFileTooShort:                "file_too_short",
	purecef.DownloadInterruptReasonFileHashMismatch:            "file_hash_mismatch",
	purecef.DownloadInterruptReasonFileSameAsSource:            "file_same_as_source",
	purecef.DownloadInterruptReasonNetworkFailed:               "network_failed",
	purecef.DownloadInterruptReasonNetworkTimeout:              "network_timeout",
	purecef.DownloadInterruptReasonNetworkDisconnected:         "network_disconnected",
	purecef.DownloadInterruptReasonNetworkServerDown:           "network_server_down",
	purecef.DownloadInterruptReasonNetworkInvalidRequest:       "network_invalid_request",
	purecef.DownloadInterruptReasonServerFailed:                "server_failed",
	purecef.DownloadInterruptReasonServerNoRange:               "server_no_range",
	purecef.DownloadInterruptReasonServerBadContent:            "server_bad_content",
	purecef.DownloadInterruptReasonServerUnauthorized:          "server_unauthorized",
	purecef.DownloadInterruptReasonServerCertProblem:           "server_cert_problem",
	purecef.DownloadInterruptReasonServerForbidden:             "server_forbidden",
	purecef.DownloadInterruptReasonServerUnreachable:           "server_unreachable",
	purecef.DownloadInterruptReasonServerContentLengthMismatch: "server_content_length_mismatch",
	purecef.DownloadInterruptReasonServerCrossOriginRedirect:   "server_cross_origin_redirect",
	purecef.DownloadInterruptReasonUserCanceled:                "user_canceled",
	purecef.DownloadInterruptReasonUserShutdown:                "user_shutdown",
	purecef.DownloadInterruptReasonCrash:                       "crash",
}

func downloadInterruptReasonLabel(reason purecef.DownloadInterruptReason) string {
	if label, ok := downloadInterruptReasonLabels[reason]; ok {
		return label
	}
	return "unknown"
}

func newDownloadHandler(
	downloadPath string,
	eventHandler port.DownloadEventHandler,
	preparer port.DownloadPreparer,
) *downloadHandler {
	return &downloadHandler{
		runtime:  downloadruntime.New(downloadPath, eventHandler, preparer),
		active:   make(map[uint32]cefDownloadState),
		finished: make(map[uint32]struct{}),
	}
}

func (h *downloadHandler) canDownload(_ purecef.Browser, _, _ string) bool {
	return true
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
		filename:            output.Filename,
		destination:         output.DestinationPath,
		lastProgressPercent: -1,
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

	// Fast path: skip already-finished downloads to avoid re-populating active.
	h.mu.Lock()
	_, done := h.finished[id]
	h.mu.Unlock()
	if done {
		return
	}

	state := h.currentState(id, downloadItem)
	h.emitProgressIfNeeded(ctx, id, downloadItem, state)

	switch {
	case downloadItem.IsComplete():
		if !h.markFinished(id) {
			return
		}
		h.runtime.EmitFinished(ctx, state.filename, state.destination)
	case downloadItem.IsCanceled() || downloadItem.IsInterrupted():
		if !h.markFinished(id) {
			return
		}
		var err error
		if downloadItem.IsInterrupted() {
			err = cefDownloadInterruptError{
				filename: state.filename,
				reason:   downloadItem.GetInterruptReason(),
			}
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

func (h *downloadHandler) emitProgressIfNeeded(
	ctx context.Context,
	id uint32,
	item purecef.DownloadItem,
	state cefDownloadState,
) {
	percent, ok := downloadProgressPercentFromItem(item)
	if !ok {
		return
	}

	h.mu.Lock()
	current, exists := h.active[id]
	if !exists {
		current = state
	}
	if current.lastProgressPercent == percent {
		h.mu.Unlock()
		return
	}
	current.lastProgressPercent = percent
	h.active[id] = current
	h.mu.Unlock()

	h.runtime.EmitProgress(
		ctx,
		current.filename,
		current.destination,
		float64(percent)/100,
		item.GetReceivedBytes(),
		item.GetTotalBytes(),
	)
}

func downloadProgressPercentFromItem(item purecef.DownloadItem) (int32, bool) {
	if item == nil || item.IsComplete() || item.IsCanceled() || item.IsInterrupted() {
		return 0, false
	}

	percent := item.GetPercentComplete()
	if percent < 0 {
		total := item.GetTotalBytes()
		received := item.GetReceivedBytes()
		if total <= 0 || received <= 0 {
			return 0, false
		}
		percent = int32((received * 100) / total)
	}
	if percent <= 0 || percent >= 100 {
		return 0, false
	}
	return percent, true
}

func (h *downloadHandler) markFinished(id uint32) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, seen := h.finished[id]; seen {
		return false
	}
	delete(h.active, id)
	if len(h.finished) >= maxFinishedEntries {
		clear(h.finished)
	}
	h.finished[id] = struct{}{}
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
