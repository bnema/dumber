package cef

import (
	"context"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/infrastructure/filtering/ceffilter"
	"github.com/bnema/dumber/internal/logging"
)

type cefFilterBackend interface {
	HasActive() bool
	ShouldBlock(req ceffilter.Request) bool
}

type filterResourceRequestHandler struct {
	ctx              context.Context
	backend          cefFilterBackend
	requestInitiator string
	isNavigation     bool
}

var _ purecef.ResourceRequestHandler = (*filterResourceRequestHandler)(nil)

func (h *filterResourceRequestHandler) GetCookieAccessFilter(
	purecef.Browser,
	purecef.Frame,
	purecef.Request,
) purecef.CookieAccessFilter {
	return nil
}

func (h *filterResourceRequestHandler) OnBeforeResourceLoad(
	_ purecef.Browser,
	frame purecef.Frame,
	request purecef.Request,
	_ purecef.Callback,
) purecef.ReturnValue {
	if h == nil || h.backend == nil || request == nil || !h.backend.HasActive() {
		return purecef.ReturnValueRvContinue
	}

	filterReq := ceffilter.Request{
		URL:              request.GetURL(),
		ResourceType:     mapCEFResourceType(request.GetResourceType()),
		RequestInitiator: h.requestInitiator,
		FrameURL:         frameURL(frame),
		IsNavigation:     h.isNavigation,
	}
	if !h.backend.ShouldBlock(filterReq) {
		return purecef.ReturnValueRvContinue
	}

	ctx := h.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	logging.FromContext(ctx).
		Debug().
		Str("url", logging.TruncateURL(filterReq.URL, maxSchemeTruncatedURLLength)).
		Str("resource_type", string(filterReq.ResourceType)).
		Msg("cef: blocked request by content filter")
	return purecef.ReturnValueRvCancel
}

func (*filterResourceRequestHandler) GetResourceHandler(
	purecef.Browser,
	purecef.Frame,
	purecef.Request,
) purecef.ResourceHandler {
	return nil
}

func (*filterResourceRequestHandler) OnResourceRedirect(
	purecef.Browser,
	purecef.Frame,
	purecef.Request,
	purecef.Response,
	uintptr,
) {
}

func (*filterResourceRequestHandler) OnResourceResponse(
	purecef.Browser,
	purecef.Frame,
	purecef.Request,
	purecef.Response,
) int32 {
	return 0
}

func (*filterResourceRequestHandler) GetResourceResponseFilter(
	purecef.Browser,
	purecef.Frame,
	purecef.Request,
	purecef.Response,
) purecef.ResponseFilter {
	return nil
}

func (*filterResourceRequestHandler) OnResourceLoadComplete(
	purecef.Browser,
	purecef.Frame,
	purecef.Request,
	purecef.Response,
	purecef.UrlrequestStatus,
	int64,
) {
}

func (*filterResourceRequestHandler) OnProtocolExecution(
	purecef.Browser,
	purecef.Frame,
	purecef.Request,
	*int32,
) {
}

func mapCEFResourceType(resourceType purecef.ResourceType) ceffilter.ResourceType {
	switch resourceType {
	case purecef.ResourceTypeRtMainFrame, purecef.ResourceTypeRtSubFrame,
		purecef.ResourceTypeRtNavigationPreloadMainFrame, purecef.ResourceTypeRtNavigationPreloadSubFrame:
		return ceffilter.ResourceTypeDocument
	case purecef.ResourceTypeRtStylesheet:
		return ceffilter.ResourceTypeStyleSheet
	case purecef.ResourceTypeRtScript, purecef.ResourceTypeRtWorker,
		purecef.ResourceTypeRtSharedWorker, purecef.ResourceTypeRtServiceWorker:
		return ceffilter.ResourceTypeScript
	case purecef.ResourceTypeRtImage, purecef.ResourceTypeRtFavicon:
		return ceffilter.ResourceTypeImage
	case purecef.ResourceTypeRtFontResource:
		return ceffilter.ResourceTypeFont
	case purecef.ResourceTypeRtMedia:
		return ceffilter.ResourceTypeMedia
	case purecef.ResourceTypeRtXhr, purecef.ResourceTypeRtPing,
		purecef.ResourceTypeRtCspReport, purecef.ResourceTypeRtPrefetch:
		return ceffilter.ResourceTypeXHR
	default:
		return ceffilter.ResourceTypeRaw
	}
}

func frameURL(frame purecef.Frame) string {
	if frame == nil {
		return ""
	}
	return frame.GetURL()
}
