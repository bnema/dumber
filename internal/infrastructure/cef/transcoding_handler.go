package cef

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
	"unsafe"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
	"github.com/rs/zerolog"
)

// ---------------------------------------------------------------------------
// Compile-time interface checks
// ---------------------------------------------------------------------------

var _ purecef.ResourceRequestHandler = (*transcodingRequestHandler)(nil)

const (
	maxTranscodingURLLength = 240
	httpStatusOK            = 200
)

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
	logf       func() zerolog.Logger
	totalBytes int64
}

// Open starts the transcode session asynchronously. CEF will call
// GetResponseHeaders and Read once the callback fires.
func (rh *transcodingResourceHandler) Open(
	_ purecef.Request, handleRequest unsafe.Pointer, callback purecef.Callback,
) int32 {
	// Signal async handling: set handleRequest = 0.
	*(*int32)(handleRequest) = 0

	log := rh.logger()
	sourceURL := logging.TruncateURL(rh.sourceURL, maxTranscodingURLLength)
	log.Info().
		Str("source_url", sourceURL).
		Int("forwarded_header_count", len(rh.headers)).
		Msg("cef: starting transcoding resource stream")

	go func() {
		session, err := rh.transcoder.Start(rh.ctx, rh.sourceURL, rh.headers)
		if err != nil {
			log.Error().
				Err(err).
				Str("source_url", sourceURL).
				Msg("cef: failed to start transcoding resource stream")
			// Let CEF know we failed — Cancel will be called.
			rh.cancel()
			callback.Cont()
			return
		}
		rh.session = session
		log.Info().
			Str("source_url", sourceURL).
			Str("content_type", session.ContentType()).
			Msg("cef: transcoding resource stream ready")
		callback.Cont()
	}()

	return 1
}

// GetResponseHeaders sets the streaming response metadata.
func (rh *transcodingResourceHandler) GetResponseHeaders(
	response purecef.Response, responseLength unsafe.Pointer, _ uintptr,
) {
	response.SetStatus(httpStatusOK)
	response.SetStatusText("OK")
	response.SetMimeType("video/webm")
	response.SetHeaderByName("Accept-Ranges", "none", 1)
	// CORS wildcard is required because transcoded streams are loaded by <video>
	// elements on arbitrary origins (e.g. reddit.com). The streams contain no
	// sensitive data — they are re-encoded from publicly-accessible media URLs.
	response.SetHeaderByName("Access-Control-Allow-Origin", "*", 1)
	response.SetHeaderByName("Cache-Control", "no-store", 1)
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

	dst := unsafe.Slice((*byte)(dataOut), int(bytesToRead))
	n, err := rh.session.Read(dst)
	if n > 0 {
		rh.totalBytes += int64(n)
		if rh.totalBytes == int64(n) {
			rh.logger().Info().
				Str("source_url", logging.TruncateURL(rh.sourceURL, maxTranscodingURLLength)).
				Int("first_chunk_bytes", n).
				Msg("cef: transcoding resource stream produced first bytes")
		}
		*(*int32)(bytesRead) = int32(n)
		return 1
	}
	if err != nil {
		sourceURL := logging.TruncateURL(rh.sourceURL, maxTranscodingURLLength)
		log := rh.logger().With().
			Str("source_url", sourceURL).
			Int64("total_bytes", rh.totalBytes).
			Logger()
		switch {
		case errors.Is(err, io.EOF) && rh.totalBytes == 0:
			log.Warn().Msg("cef: transcoding resource stream ended before producing data")
		case errors.Is(err, io.EOF):
			log.Debug().Msg("cef: transcoding resource stream reached EOF")
		default:
			log.Error().Err(err).Msg("cef: transcoding resource stream read failed")
		}
		return 0 // EOF or error
	}
	return 0
}

// Cancel aborts the transcode session and releases resources.
func (rh *transcodingResourceHandler) Cancel() {
	sourceURL := logging.TruncateURL(rh.sourceURL, maxTranscodingURLLength)
	rh.logger().Debug().
		Str("source_url", sourceURL).
		Int64("total_bytes", rh.totalBytes).
		Msg("cef: transcoding resource stream canceled")
	if rh.session != nil {
		if err := rh.session.Close(); err != nil {
			rh.logger().Warn().
				Str("source_url", sourceURL).
				Err(err).
				Msg("cef: transcoding resource stream close failed")
		}
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
// proprietary video responses and streaming manifests, then restarts them
// through the transcoder
// ---------------------------------------------------------------------------

type transcodingRequestHandler struct {
	transcoder       port.MediaTranscoder
	classifier       MediaClassifier
	transcodableURLs sync.Map // url string -> requestInfo
	ctxf             func() context.Context
}

type requestInfo struct {
	headers map[string]string
}

// newTranscodingRequestHandler creates a ResourceRequestHandler that detects
// proprietary video containers and HLS/DASH manifest entrypoints, then
// restarts those requests through a GPU transcoding ResourceHandler.
func newTranscodingRequestHandler(
	tc port.MediaTranscoder, classifier MediaClassifier, ctxf func() context.Context,
) purecef.ResourceRequestHandler {
	return purecef.NewResourceRequestHandler(&transcodingRequestHandler{
		transcoder: tc,
		classifier: classifier.normalize(),
		ctxf:       ctxf,
	})
}

// OnResourceResponse inspects the MIME type and URL of every response. If it
// matches a proprietary video format or a streaming manifest entrypoint, it
// stores the request info and returns 1 to restart the request — at which
// point GetResourceHandler will provide the transcoding handler.
func (h *transcodingRequestHandler) OnResourceResponse(
	_ purecef.Browser, _ purecef.Frame, request purecef.Request, response purecef.Response,
) int32 {
	if response == nil || request == nil {
		return 0
	}
	mimeType := response.GetMimeType()
	url := request.GetURL()

	log := h.logger()
	isProprietary, isAlreadyOpen, isManifest := classifyMediaResponse(h.classifier, url, mimeType)
	if isProprietary || isAlreadyOpen || isManifest {
		log.Debug().
			Str("url", logging.TruncateURL(url, maxTranscodingURLLength)).
			Str("mime_type", mimeType).
			Bool("proprietary_match", isProprietary).
			Bool("already_open", isAlreadyOpen).
			Bool("manifest_match", isManifest).
			Msg("cef: OnResourceResponse observed media response")
	}

	if isAlreadyOpen {
		return 0
	}

	if !isProprietary && !isManifest {
		return 0
	}

	info := buildRequestInfo(request)
	h.transcodableURLs.Store(url, info)
	reason := "proprietary-container"
	if isManifest {
		reason = "streaming-manifest"
	}
	log.Info().
		Str("url", logging.TruncateURL(url, maxTranscodingURLLength)).
		Str("mime_type", mimeType).
		Str("transcode_reason", reason).
		Int("forwarded_header_count", len(info.headers)).
		Msg("cef: media response marked for transcoding")

	return 1 // restart request
}

// GetResourceHandler returns a transcoding ResourceHandler for URLs that were
// either identified eagerly by URL heuristics or marked by OnResourceResponse.
func (h *transcodingRequestHandler) GetResourceHandler(
	_ purecef.Browser, _ purecef.Frame, request purecef.Request,
) purecef.ResourceHandler {
	if request == nil {
		return nil
	}
	url := request.GetURL()
	val, ok := h.transcodableURLs.LoadAndDelete(url)
	info := requestInfo{}
	eager := false
	sourceURL := url
	if ok {
		cached, typeOK := val.(requestInfo)
		if !typeOK {
			return nil
		}
		info = cached
	} else if syntheticSourceURL, referer, origin, synthetic := resolveTranscodeSource(h.classifier, url); synthetic {
		eager = true
		sourceURL = syntheticSourceURL
		info = buildRequestInfo(request)
		if referer != "" && info.headers["Referer"] == "" {
			info.headers["Referer"] = referer
		}
		if origin != "" && info.headers["Origin"] == "" {
			info.headers["Origin"] = origin
		}
	} else {
		return nil
	}

	parentCtx := context.Background()
	if h.ctxf != nil {
		if provided := h.ctxf(); provided != nil {
			parentCtx = provided
		}
	}
	ctx, cancel := context.WithTimeout(parentCtx, 5*time.Minute)
	shortURL := logging.TruncateURL(url, maxTranscodingURLLength)
	shortSourceURL := logging.TruncateURL(sourceURL, maxTranscodingURLLength)
	h.logger().Info().
		Str("url", shortURL).
		Str("source_url", shortSourceURL).
		Bool("eager_match", eager).
		Int("forwarded_header_count", len(info.headers)).
		Msg("cef: GetResourceHandler returning transcoding resource handler")
	return purecef.NewResourceHandler(&transcodingResourceHandler{
		transcoder: h.transcoder,
		sourceURL:  sourceURL,
		headers:    info.headers,
		ctx:        ctx,
		cancel:     cancel,
		logf:       h.logf,
	})
}

func classifyMediaResponse(classifier MediaClassifier, rawURL, mimeType string) (bool, bool, bool) {
	proprietary := classifier.IsProprietaryVideoMIME(mimeType)
	alreadyOpen := classifier.IsOpenVideoMIME(mimeType)
	manifest := !alreadyOpen && (classifier.IsStreamingManifestMIME(mimeType) || classifier.IsStreamingManifestURL(rawURL))
	return proprietary, alreadyOpen, manifest
}

func resolveTranscodeSource(classifier MediaClassifier, rawURL string) (string, string, string, bool) {
	if sourceURL, referer, origin, ok := classifier.ParseSyntheticTranscodeURL(rawURL); ok {
		return sourceURL, referer, origin, true
	}
	if classifier.IsEagerTranscodeURL(rawURL) {
		return rawURL, "", "", true
	}
	return "", "", "", false
}

// --- No-op methods required by purecef.ResourceRequestHandler ---

func (h *transcodingRequestHandler) GetCookieAccessFilter(
	_ purecef.Browser, _ purecef.Frame, _ purecef.Request,
) purecef.CookieAccessFilter {
	return nil
}

func (h *transcodingRequestHandler) OnBeforeResourceLoad(
	_ purecef.Browser, _ purecef.Frame, _ purecef.Request, _ purecef.Callback,
) purecef.ReturnValue {
	return purecef.ReturnValueRvContinue
}

func (h *transcodingRequestHandler) OnResourceRedirect(
	_ purecef.Browser, _ purecef.Frame, _ purecef.Request, _ purecef.Response, _ uintptr,
) {
}

func (h *transcodingRequestHandler) GetResourceResponseFilter(
	_ purecef.Browser, _ purecef.Frame, _ purecef.Request, _ purecef.Response,
) purecef.ResponseFilter {
	return nil
}

func (h *transcodingRequestHandler) OnResourceLoadComplete(
	_ purecef.Browser, _ purecef.Frame, _ purecef.Request, _ purecef.Response,
	_ purecef.UrlrequestStatus, _ int64,
) {
}

func (h *transcodingRequestHandler) OnProtocolExecution(_ purecef.Browser, _ purecef.Frame, _ purecef.Request, _ unsafe.Pointer) {
}

func buildRequestInfo(request purecef.Request) requestInfo {
	headers := make(map[string]string)
	if request == nil {
		return requestInfo{headers: headers}
	}
	if cookie := request.GetHeaderByName("Cookie"); cookie != "" {
		headers["Cookie"] = cookie
	}
	if auth := request.GetHeaderByName("Authorization"); auth != "" {
		headers["Authorization"] = auth
	}
	if referer := request.GetHeaderByName("Referer"); referer != "" {
		headers["Referer"] = referer
	}
	if userAgent := request.GetHeaderByName("User-Agent"); userAgent != "" {
		headers["User-Agent"] = userAgent
	}
	if origin := request.GetHeaderByName("Origin"); origin != "" {
		headers["Origin"] = origin
	}
	return requestInfo{headers: headers}
}

func (h *transcodingRequestHandler) logger() *zerolog.Logger {
	logger := h.logf()
	return &logger
}

func (h *transcodingRequestHandler) logf() zerolog.Logger {
	ctx := context.Background()
	if h != nil && h.ctxf != nil {
		if provided := h.ctxf(); provided != nil {
			ctx = provided
		}
	}
	return logging.FromContext(ctx).With().Str("component", "cef-transcoding").Logger()
}

func (rh *transcodingResourceHandler) logger() *zerolog.Logger {
	if rh != nil && rh.logf != nil {
		sourceURL := logging.TruncateURL(rh.sourceURL, maxTranscodingURLLength)
		logger := rh.logf().With().Str("source_url", sourceURL).Logger()
		return &logger
	}
	logger := logging.FromContext(context.Background()).With().Str("component", "cef-transcoding").Logger()
	return &logger
}
