package cef

import (
	"context"
	"sync"
	"unsafe"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/transcoder"
)

// ---------------------------------------------------------------------------
// Compile-time interface checks
// ---------------------------------------------------------------------------

var _ purecef.ResourceRequestHandler = (*transcodingRequestHandler)(nil)

// transcodingResourceHandler is checked indirectly: purecef.NewResourceHandler
// accepts a purecef.ResourceHandler, and the struct implements all methods.

// ---------------------------------------------------------------------------
// transcodingResourceHandler — serves a transcoded WebM stream to CEF
// ---------------------------------------------------------------------------

type transcodingResourceHandler struct {
	transcoder port.MediaTranscoder
	sourceURL  string
	headers    map[string]string
	session    port.TranscodeSession
	ctx        context.Context
	cancel     context.CancelFunc
}

// Open starts the transcode session asynchronously. CEF will call
// GetResponseHeaders and Read once the callback fires.
func (rh *transcodingResourceHandler) Open(_ purecef.Request, handleRequest unsafe.Pointer, callback purecef.Callback) int32 {
	// Signal async handling: set handleRequest = 0.
	*(*int32)(handleRequest) = 0

	go func() {
		session, err := rh.transcoder.Start(rh.ctx, rh.sourceURL, rh.headers)
		if err != nil {
			// Let CEF know we failed — Cancel will be called.
			rh.cancel()
			return
		}
		rh.session = session
		callback.Cont()
	}()

	return 1
}

// GetResponseHeaders sets the streaming response metadata.
func (rh *transcodingResourceHandler) GetResponseHeaders(response purecef.Response, responseLength unsafe.Pointer, _ uintptr) {
	response.SetStatus(200)
	response.SetStatusText("OK")
	response.SetMimeType("video/webm")
	response.SetHeaderByName("Accept-Ranges", "none", 1)
	// Streaming — unknown length.
	*(*int64)(responseLength) = -1
}

// Read copies transcoded data from the session into the CEF output buffer.
func (rh *transcodingResourceHandler) Read(
	dataOut unsafe.Pointer, bytesToRead int32,
	bytesRead unsafe.Pointer, _ purecef.ResourceReadCallback,
) int32 {
	if rh.session == nil {
		return 0
	}

	buf := make([]byte, bytesToRead)
	n, err := rh.session.Read(buf)
	if n > 0 {
		dst := unsafe.Slice((*byte)(dataOut), n)
		copy(dst, buf[:n])
		*(*int32)(bytesRead) = int32(n)
		return 1
	}
	if err != nil {
		return 0 // EOF or error
	}
	return 0
}

// Cancel aborts the transcode session and releases resources.
func (rh *transcodingResourceHandler) Cancel() {
	if rh.session != nil {
		rh.session.Close()
	}
	if rh.cancel != nil {
		rh.cancel()
	}
}

// ProcessRequest is deprecated; Open is used instead.
func (rh *transcodingResourceHandler) ProcessRequest(_ purecef.Request, _ purecef.Callback) int32 {
	return 0
}

// ReadResponse is deprecated; Read is used instead.
func (rh *transcodingResourceHandler) ReadResponse(_ unsafe.Pointer, _ int32, _ unsafe.Pointer, _ purecef.Callback) int32 {
	return 0
}

// Skip is not used for streaming content.
func (rh *transcodingResourceHandler) Skip(_ int64, _ unsafe.Pointer, _ purecef.ResourceSkipCallback) int32 {
	return 0
}

// ---------------------------------------------------------------------------
// transcodingRequestHandler — ResourceRequestHandler that intercepts
// proprietary video responses and restarts them through the transcoder
// ---------------------------------------------------------------------------

type transcodingRequestHandler struct {
	transcoder       port.MediaTranscoder
	transcodableURLs sync.Map // url string -> requestInfo
}

type requestInfo struct {
	headers map[string]string
}

// newTranscodingRequestHandler creates a ResourceRequestHandler that detects
// proprietary video MIME types and restarts those requests through a GPU
// transcoding ResourceHandler.
func newTranscodingRequestHandler(tc port.MediaTranscoder) purecef.ResourceRequestHandler {
	return purecef.NewResourceRequestHandler(&transcodingRequestHandler{
		transcoder: tc,
	})
}

// OnResourceResponse inspects the MIME type of every response. If it matches
// a proprietary video format (H.264, HEVC, etc.), it stores the request info
// and returns 1 to restart the request — at which point GetResourceHandler
// will provide the transcoding handler.
func (h *transcodingRequestHandler) OnResourceResponse(_ purecef.Browser, _ purecef.Frame, request purecef.Request, response purecef.Response) int32 {
	if response == nil {
		return 0
	}
	mimeType := response.GetMimeType()
	if !transcoder.IsProprietaryVideoMIME(mimeType) {
		return 0
	}

	// Extract relevant headers from the original request so the transcoder
	// can authenticate with the source server.
	headers := make(map[string]string)
	if cookie := request.GetHeaderByName("Cookie"); cookie != "" {
		headers["Cookie"] = cookie
	}
	if auth := request.GetHeaderByName("Authorization"); auth != "" {
		headers["Authorization"] = auth
	}
	if referer := request.GetHeaderByName("Referer"); referer != "" {
		headers["Referer"] = referer
	}

	url := request.GetURL()
	h.transcodableURLs.Store(url, requestInfo{headers: headers})

	return 1 // restart request
}

// GetResourceHandler returns a transcoding ResourceHandler for URLs that were
// previously identified as proprietary video by OnResourceResponse.
func (h *transcodingRequestHandler) GetResourceHandler(_ purecef.Browser, _ purecef.Frame, request purecef.Request) purecef.ResourceHandler {
	if request == nil {
		return nil
	}
	url := request.GetURL()
	val, ok := h.transcodableURLs.LoadAndDelete(url)
	if !ok {
		return nil
	}
	info := val.(requestInfo)

	ctx, cancel := context.WithCancel(context.Background())
	return purecef.NewResourceHandler(&transcodingResourceHandler{
		transcoder: h.transcoder,
		sourceURL:  url,
		headers:    info.headers,
		ctx:        ctx,
		cancel:     cancel,
	})
}

// --- No-op methods required by purecef.ResourceRequestHandler ---

func (h *transcodingRequestHandler) GetCookieAccessFilter(_ purecef.Browser, _ purecef.Frame, _ purecef.Request) purecef.CookieAccessFilter {
	return nil
}

func (h *transcodingRequestHandler) OnBeforeResourceLoad(_ purecef.Browser, _ purecef.Frame, _ purecef.Request, _ purecef.Callback) purecef.ReturnValue {
	return purecef.ReturnValueRvContinue
}

func (h *transcodingRequestHandler) OnResourceRedirect(_ purecef.Browser, _ purecef.Frame, _ purecef.Request, _ purecef.Response, _ uintptr) {
}

func (h *transcodingRequestHandler) GetResourceResponseFilter(_ purecef.Browser, _ purecef.Frame, _ purecef.Request, _ purecef.Response) purecef.ResponseFilter {
	return nil
}

func (h *transcodingRequestHandler) OnResourceLoadComplete(_ purecef.Browser, _ purecef.Frame, _ purecef.Request, _ purecef.Response, _ purecef.UrlrequestStatus, _ int64) {
}

func (h *transcodingRequestHandler) OnProtocolExecution(_ purecef.Browser, _ purecef.Frame, _ purecef.Request, _ unsafe.Pointer) {
}
