package cef

import (
	"sync/atomic"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
)

// filterRequestHandler adapts a port.ContentFilter to CEF resource loading.
// This is the test-only feasibility seam for the filtering gate: it is
// deliberately NOT wired into client/handler creation, so it has no
// production effect. Decisions are synchronous; asynchronous rule loading,
// atomic updates, and corruption recovery belong to the separately approved
// production design.
type filterRequestHandler struct {
	filter port.ContentFilter
	closed atomic.Bool
}

// newFilterRequestHandler builds the seam adapter around matcher. A nil
// matcher allows everything.
func newFilterRequestHandler(matcher port.ContentFilter) *filterRequestHandler {
	return &filterRequestHandler{filter: matcher}
}

// Close retires the seam: later requests are allowed without consulting the
// matcher (fail-open is correct for a test seam; production UX for a
// missing/errored policy belongs to the follow-up design).
func (h *filterRequestHandler) Close() {
	if h == nil {
		return
	}
	h.closed.Store(true)
}

// OnBeforeResourceLoad maps one filter decision onto CEF's synchronous
// return value. Unclassifiable requests (nil request/empty URL) and a
// retired/nil matcher allow the request. The CEF continuation callback is
// untouched: this seam never defers decisions asynchronously.
func (h *filterRequestHandler) OnBeforeResourceLoad(
	browser purecef.Browser, frame purecef.Frame, request purecef.Request, _ purecef.Callback,
) purecef.ReturnValue {
	if h == nil || h.closed.Load() || h.filter == nil || request == nil {
		return purecef.ReturnValueRvContinue
	}
	var isMain bool
	if frame != nil {
		isMain = frame.IsMain()
	}
	url := request.GetURL()
	if url == "" {
		return purecef.ReturnValueRvContinue
	}
	// Document loads are main/sub-frame resource types; everything else
	// is a subresource of the current document.
	resourceType := request.GetResourceType()
	isNavigation := resourceType == purecef.ResourceTypeRtMainFrame || resourceType == purecef.ResourceTypeRtSubFrame
	_ = browser
	if h.filter.Decide(port.FilterRequest{URL: url, IsMainFrame: isMain, IsNavigation: isNavigation}) == port.FilterBlock {
		return purecef.ReturnValueRvCancel
	}
	return purecef.ReturnValueRvContinue
}
