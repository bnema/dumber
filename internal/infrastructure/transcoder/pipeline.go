package transcoder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"unsafe"

	"net"
	"time"

	"github.com/ebitengine/purego"
	"github.com/rs/zerolog"

	"github.com/bnema/purego-ffmpeg/ffmpeg"

	"github.com/bnema/dumber/internal/application/port"
)

// avioBufSize is the buffer size for custom AVIO contexts (32 KiB).
const avioBufSize = 32 * 1024

const (
	vaapiInitialPoolSize = 20
	minVideoBitrate      = 500_000
)

// pipeline orchestrates a single GPU transcode session. It reads from
// an HTTP source, decodes with a hardware decoder (or software fallback),
// encodes video to an open codec (AV1/VP9) via GPU, encodes audio to
// Opus via libopus, and muxes the result into WebM written to an
// io.PipeWriter.
type pipeline struct {
	sessionID string
	hwCaps    port.HWCapabilities
	sourceURL string
	headers   map[string]string
	quality   string
	pw        *io.PipeWriter
	logger    zerolog.Logger

	// Prevent GC of callback closures and uintptrs.
	readCb  uintptr
	writeCb uintptr
}

// httpClient is used for fetching source media. It has connect/TLS/header
// timeouts but no overall Timeout so that long-lived streaming responses
// are not killed. The session context handles cancellation.
var httpClient = &http.Client{
	Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	},
}

func newPipeline(
	sessionID string,
	hwCaps port.HWCapabilities,
	sourceURL string,
	headers map[string]string,
	quality string,
	pw *io.PipeWriter,
	logger zerolog.Logger,
) *pipeline {
	return &pipeline{
		sessionID: sessionID,
		hwCaps:    hwCaps,
		sourceURL: sourceURL,
		headers:   headers,
		quality:   quality,
		pw:        pw,
		logger:    logger,
	}
}

// run executes the full transcode pipeline in the calling goroutine.
// It closes pw when done (with error or nil).
func (p *pipeline) run(ctx context.Context) {
	err := p.doRun(ctx)
	if err != nil {
		log := p.logger.With().
			Str("session_id", p.sessionID).
			Str("source_url", p.sourceURL).
			Logger()
		if errors.Is(err, context.Canceled) {
			log.Debug().Err(err).Msg("transcode pipeline canceled")
		} else {
			log.Error().Err(err).Msg("transcode pipeline failed")
		}
		_ = p.pw.CloseWithError(err)
	} else {
		p.logger.Debug().
			Str("session_id", p.sessionID).
			Str("source_url", p.sourceURL).
			Msg("transcode pipeline completed")
		_ = p.pw.Close()
	}
}

// doRun contains the actual pipeline logic. Errors are returned, and
// the caller (run) closes the pipe writer accordingly.
func (p *pipeline) doRun(ctx context.Context) error {
	return p.doRunImpl(ctx)
}

//nolint:gocyclo,funlen,gosec // FFmpeg pipeline setup/teardown uses necessary branching and unsafe C-API calls.
func (p *pipeline) doRunImpl(ctx context.Context) error {
	// --- Step 1-3: Open the input using the appropriate strategy ---
	inFmtCtx, closeInput, err := p.openInputFormatContext(ctx)
	if err != nil {
		return err
	}
	defer closeInput()

	// --- Step 4: Find stream info ---
	if ret := ffmpeg.FormatFindStreamInfo(inFmtCtx, nil); ret < 0 {
		return fmt.Errorf("find stream info: %d", ret)
	}

	// --- Step 5: Select input video/audio streams ---
	videoIdx, audioIdx, err := p.selectInputStreams(inFmtCtx)
	if err != nil {
		return err
	}

	videoStream := ffmpeg.FmtCtxStream(inFmtCtx, videoIdx)
	videoStreamWrap := ffmpeg.WrapStream(videoStream)
	videoCodecPar := videoStreamWrap.Codecpar()

	// --- Step 6: Open video decoder ---
	vidDecCtx, err := p.openVideoDecoder(videoCodecPar)
	if err != nil {
		return fmt.Errorf("open video decoder: %w", err)
	}
	//nolint:gosec // FFmpeg requires pointer-to-pointer cleanup for codec contexts.
	defer ffmpeg.CodecFreeContext(unsafe.Pointer(&vidDecCtx))

	// --- Step 7-8: Open video encoder with HW accel ---
	hwDeviceCtx, vidEncCtx, err := p.openVideoEncoder(vidDecCtx)
	if err != nil {
		return fmt.Errorf("open video encoder: %w", err)
	}
	if hwDeviceCtx != nil {
		defer ffmpeg.BufferUnref(&hwDeviceCtx)
	}
	//nolint:gosec // FFmpeg requires pointer-to-pointer cleanup for codec contexts.
	defer ffmpeg.CodecFreeContext(unsafe.Pointer(&vidEncCtx))

	// --- Step 9: Create output format context for WebM ---
	var outFmtCtx unsafe.Pointer
	//nolint:gosec // FFmpeg requires pointer-to-pointer allocation for output contexts.
	if ret := ffmpeg.FormatAllocOutputContext2(unsafe.Pointer(&outFmtCtx), nil, "webm", ""); ret < 0 {
		return fmt.Errorf("alloc output context: %d", ret)
	}
	defer ffmpeg.FormatFreeContext(outFmtCtx)

	// --- Step 10: Create write AVIO callback ---
	pw := p.pw
	writeFn := func(_ unsafe.Pointer, buf unsafe.Pointer, bufSize int32) int32 {
		if bufSize <= 0 {
			return 0
		}
		//nolint:gosec // FFmpeg supplies a valid callback buffer and explicit byte count here.
		goBuf := unsafe.Slice((*byte)(buf), int(bufSize))
		n, writeErr := pw.Write(goBuf)
		if writeErr != nil {
			return -1
		}
		return int32(n)
	}
	p.writeCb = purego.NewCallback(writeFn)

	writeAVIO := ffmpeg.AvioAllocContext(avioBufSize, true, nil, 0, p.writeCb, 0)
	if writeAVIO == nil {
		return errors.New("failed to allocate write AVIO context")
	}
	//nolint:gosec // FFmpeg requires pointer-to-pointer cleanup for AVIO contexts.
	defer ffmpeg.AvioContextFree(unsafe.Pointer(&writeAVIO))

	ffmpeg.FmtCtxSetPB(outFmtCtx, writeAVIO)
	ffmpeg.FmtCtxSetFlags(outFmtCtx, ffmpeg.FmtCtxFlags(outFmtCtx)|ffmpeg.AVFMT_FLAG_CUSTOM_IO)

	// --- Step 11: Add video output stream ---
	outVideoStream := ffmpeg.FormatNewStream(outFmtCtx, nil)
	if outVideoStream == nil {
		return errors.New("failed to create output video stream")
	}
	outVideoStreamWrap := ffmpeg.WrapStream(outVideoStream)
	outVideoIdx := int(outVideoStreamWrap.Index())

	if ret := ffmpeg.CodecParametersFromContext(outVideoStreamWrap.Codecpar(), vidEncCtx); ret < 0 {
		return fmt.Errorf("copy video encoder params to stream: %d", ret)
	}

	// --- Step 11b: Add audio output stream (if audio present) ---
	var audioTx *audioTranscoder
	if audioIdx >= 0 {
		audioStream := ffmpeg.FmtCtxStream(inFmtCtx, audioIdx)
		audioStreamWrap := ffmpeg.WrapStream(audioStream)
		audioCodecPar := audioStreamWrap.Codecpar()

		var txErr error
		audioTx, txErr = newAudioTranscoder(audioCodecPar, outFmtCtx, audioIdx)
		if txErr != nil {
			// Audio transcoding failure is non-fatal; we proceed with video only.
			audioTx = nil
			audioIdx = -1
		}
	}
	// Use a closure so the defer sees the current value of audioTx at
	// cleanup time. This allows us to close and nil-out audioTx mid-loop
	// (e.g., on audio processing error) without double-closing.
	defer func() {
		if audioTx != nil {
			audioTx.close()
		}
	}()

	// --- Step 12: Write header ---
	if ret := ffmpeg.FormatWriteHeader(outFmtCtx, nil); ret < 0 {
		return fmt.Errorf("write header: %d", ret)
	}

	// --- Step 13: Main decode/encode loop ---
	pkt := ffmpeg.PacketAlloc()
	if pkt == nil {
		return errors.New("failed to allocate read packet")
	}
	//nolint:gosec // FFmpeg requires pointer-to-pointer cleanup for packet allocations.
	defer ffmpeg.PacketFree(unsafe.Pointer(&pkt))

	frame := ffmpeg.FrameAlloc()
	if frame == nil {
		return errors.New("failed to allocate decode frame")
	}
	//nolint:gosec // FFmpeg requires pointer-to-pointer cleanup for frame allocations.
	defer ffmpeg.FrameFree(unsafe.Pointer(&frame))

	encPkt := ffmpeg.PacketAlloc()
	if encPkt == nil {
		return errors.New("failed to allocate encode packet")
	}
	//nolint:gosec // FFmpeg requires pointer-to-pointer cleanup for packet allocations.
	defer ffmpeg.PacketFree(unsafe.Pointer(&encPkt))

	for {
		// Check context cancellation.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ret := ffmpeg.ReadFrame(inFmtCtx, pkt)
		if ret < 0 {
			// EOF or error — break to flush.
			break
		}

		streamIdx := int(pktStreamIndex(pkt))

		switch {
		case streamIdx == videoIdx:
			if err := p.transcodeVideoPacket(pkt, frame, encPkt, vidDecCtx, vidEncCtx, outFmtCtx, outVideoIdx); err != nil {
				ffmpeg.PacketUnref(pkt)
				return fmt.Errorf("video transcode: %w", err)
			}

		case audioTx != nil && streamIdx == audioIdx:
			if err := audioTx.processPacket(pkt, outFmtCtx); err != nil {
				// Audio errors are non-fatal: log warning, disable audio,
				// and continue with video-only output. This matches the
				// non-fatal handling of audio init failure above.
				p.logger.Warn().
					Str("session_id", p.sessionID).
					Err(err).
					Msg("audio processing failed, continuing with video only")
				audioTx.close()
				audioTx = nil
				audioIdx = -1
			}
		}

		ffmpeg.PacketUnref(pkt)
	}

	// --- Step 14: Flush decoders and encoders ---
	// Flush video decoder.
	ffmpeg.CodecSendPacket(vidDecCtx, nil)
	for {
		ret := ffmpeg.CodecReceiveFrame(vidDecCtx, frame)
		if ret < 0 {
			break
		}
		if err := p.encodeAndWriteVideoFrame(frame, encPkt, vidEncCtx, outFmtCtx, outVideoIdx); err != nil {
			ffmpeg.FrameUnref(frame)
			return fmt.Errorf("video flush: %w", err)
		}
		ffmpeg.FrameUnref(frame)
	}

	// Flush video encoder.
	ffmpeg.CodecSendFrame(vidEncCtx, nil)
	for {
		ret := ffmpeg.CodecReceivePacket(vidEncCtx, encPkt)
		if ret < 0 {
			break
		}
		pktSetStreamIndex(encPkt, int32(outVideoIdx))
		outVidStream := ffmpeg.FmtCtxStream(outFmtCtx, outVideoIdx)
		outVidStreamWrap := ffmpeg.WrapStream(outVidStream)
		ffmpeg.PacketRescaleTs(encPkt, ffmpeg.CodecCtxTimeBase(vidEncCtx), outVidStreamWrap.TimeBase())
		ffmpeg.InterleavedWriteFrame(outFmtCtx, encPkt)
		ffmpeg.PacketUnref(encPkt)
	}

	// Flush audio.
	if audioTx != nil {
		audioTx.flush(outFmtCtx, p.logger)
	}

	// --- Step 15: Write trailer ---
	if ret := ffmpeg.WriteTrailer(outFmtCtx); ret < 0 {
		return fmt.Errorf("write trailer: %d", ret)
	}

	return nil
}

func (p *pipeline) selectInputStreams(inFmtCtx unsafe.Pointer) (videoIdx, audioIdx int, err error) {
	videoIdx = -1
	audioIdx = -1
	bestVideoPixels := int64(-1)

	streamCount := int(ffmpeg.FmtCtxNbStreams(inFmtCtx))
	for i := 0; i < streamCount; i++ {
		stream := ffmpeg.FmtCtxStream(inFmtCtx, i)
		if stream == nil {
			continue
		}
		streamWrap := ffmpeg.WrapStream(stream)
		codecPar := streamWrap.Codecpar()
		if codecPar == nil {
			continue
		}

		switch ffmpeg.CodecParCodecType(codecPar) {
		case ffmpeg.AVMEDIA_TYPE_VIDEO:
			width := int64(ffmpeg.CodecParWidth(codecPar))
			height := int64(ffmpeg.CodecParHeight(codecPar))
			pixels := width * height
			if videoIdx == -1 || pixels > bestVideoPixels {
				videoIdx = i
				bestVideoPixels = pixels
			}
		case ffmpeg.AVMEDIA_TYPE_AUDIO:
			if audioIdx == -1 {
				audioIdx = i
			}
		}
	}

	if videoIdx < 0 {
		return -1, audioIdx, errors.New("no video stream found in source")
	}

	videoCodecPar := ffmpeg.WrapStream(ffmpeg.FmtCtxStream(inFmtCtx, videoIdx)).Codecpar()
	log := p.logger.With().
		Str("session_id", p.sessionID).
		Str("source_url", p.sourceURL).
		Int("video_stream_index", videoIdx).
		Int32("video_codec_id", ffmpeg.CodecParCodecID(videoCodecPar)).
		Int32("video_width", ffmpeg.CodecParWidth(videoCodecPar)).
		Int32("video_height", ffmpeg.CodecParHeight(videoCodecPar)).
		Logger()
	if audioIdx >= 0 {
		audioCodecPar := ffmpeg.WrapStream(ffmpeg.FmtCtxStream(inFmtCtx, audioIdx)).Codecpar()
		log.Info().
			Int("audio_stream_index", audioIdx).
			Int32("audio_codec_id", ffmpeg.CodecParCodecID(audioCodecPar)).
			Msg("transcode pipeline selected input streams")
	} else {
		log.Info().Msg("transcode pipeline selected input streams")
	}

	return videoIdx, audioIdx, nil
}

func (p *pipeline) openInputFormatContext(ctx context.Context) (unsafe.Pointer, func(), error) {
	if IsStreamingManifestURL(p.sourceURL) {
		return p.openManifestInputContext()
	}
	return p.openCustomIOInputContext(ctx)
}

func (p *pipeline) openCustomIOInputContext(ctx context.Context) (unsafe.Pointer, func(), error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.sourceURL, http.NoBody)
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	for k, v := range p.headers {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req) //nolint:bodyclose // closed by returned cleanup after FFmpeg finishes reading.
	if err != nil {
		return nil, nil, fmt.Errorf("fetch source: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		_ = resp.Body.Close()
		return nil, nil, fmt.Errorf("source returned HTTP %d", resp.StatusCode)
	}

	body := resp.Body
	readFn := func(_ unsafe.Pointer, buf unsafe.Pointer, bufSize int32) int32 {
		if bufSize <= 0 {
			return -1
		}
		//nolint:gosec // FFmpeg supplies a valid callback buffer and explicit byte count here.
		goBuf := unsafe.Slice((*byte)(buf), int(bufSize))
		// Retry up to 3 times for the edge case where body.Read returns
		// n=0, err=nil (allowed by the io.Reader contract but unusual).
		for tries := 0; tries < 3; tries++ {
			n, readErr := body.Read(goBuf)
			if n > 0 {
				return int32(n)
			}
			if readErr != nil {
				// Return AVERROR_EOF for EOF, generic error otherwise.
				return averrorEOF
			}
			// n == 0 && err == nil: retry
		}
		return averrorEOF
	}
	p.readCb = purego.NewCallback(readFn)

	readAVIO := ffmpeg.AvioAllocContext(avioBufSize, false, nil, p.readCb, 0, 0)
	if readAVIO == nil {
		_ = resp.Body.Close()
		return nil, nil, errors.New("failed to allocate read AVIO context")
	}

	inFmtCtx := ffmpeg.FormatAllocContext()
	if inFmtCtx == nil {
		//nolint:gosec // FFmpeg requires pointer-to-pointer cleanup for AVIO contexts.
		ffmpeg.AvioContextFree(unsafe.Pointer(&readAVIO))
		_ = resp.Body.Close()
		return nil, nil, errors.New("failed to allocate input format context")
	}

	ffmpeg.FmtCtxSetPB(inFmtCtx, readAVIO)
	ffmpeg.FmtCtxSetFlags(inFmtCtx, ffmpeg.FmtCtxFlags(inFmtCtx)|ffmpeg.AVFMT_FLAG_CUSTOM_IO)

	//nolint:gosec // FFmpeg requires pointer-to-pointer input opening for format contexts.
	if ret := ffmpeg.FormatOpenInput(unsafe.Pointer(&inFmtCtx), p.sourceURL, nil, nil); ret < 0 {
		//nolint:gosec // FFmpeg requires pointer-to-pointer cleanup for AVIO contexts.
		ffmpeg.AvioContextFree(unsafe.Pointer(&readAVIO))
		_ = resp.Body.Close()
		ffmpeg.FormatFreeContext(inFmtCtx)
		return nil, nil, fmt.Errorf("open input: %d", ret)
	}

	cleanup := func() {
		//nolint:gosec // FFmpeg requires pointer-to-pointer cleanup for format contexts.
		ffmpeg.FormatCloseInput(unsafe.Pointer(&inFmtCtx))
		//nolint:gosec // FFmpeg requires pointer-to-pointer cleanup for AVIO contexts.
		ffmpeg.AvioContextFree(unsafe.Pointer(&readAVIO))
		_ = resp.Body.Close()
	}
	return inFmtCtx, cleanup, nil
}

func (p *pipeline) openManifestInputContext() (unsafe.Pointer, func(), error) {
	inFmtCtx := ffmpeg.FormatAllocContext()
	if inFmtCtx == nil {
		return nil, nil, errors.New("failed to allocate input format context")
	}

	var opts unsafe.Pointer
	defer freeInputOptions(&opts)

	if err := setInputOption(&opts, "protocol_whitelist", "file,http,https,tcp,tls,crypto"); err != nil {
		ffmpeg.FormatFreeContext(inFmtCtx)
		return nil, nil, err
	}
	if userAgent := p.headers["User-Agent"]; userAgent != "" {
		if err := setInputOption(&opts, "user_agent", userAgent); err != nil {
			ffmpeg.FormatFreeContext(inFmtCtx)
			return nil, nil, err
		}
	}
	if referer := p.headers["Referer"]; referer != "" {
		if err := setInputOption(&opts, "referer", referer); err != nil {
			ffmpeg.FormatFreeContext(inFmtCtx)
			return nil, nil, err
		}
	}

	headerLines := formatHTTPHeaderOptions(p.headers)
	if headerLines != "" {
		if err := setInputOption(&opts, "headers", headerLines); err != nil {
			ffmpeg.FormatFreeContext(inFmtCtx)
			return nil, nil, err
		}
	}

	//nolint:gosec // FFmpeg requires pointer-to-pointer input opening for format contexts and option dictionaries.
	if ret := ffmpeg.FormatOpenInput(unsafe.Pointer(&inFmtCtx), p.sourceURL, nil, unsafe.Pointer(&opts)); ret < 0 {
		ffmpeg.FormatFreeContext(inFmtCtx)
		return nil, nil, fmt.Errorf("open manifest input: %d", ret)
	}

	return inFmtCtx, func() {
		//nolint:gosec // FFmpeg requires pointer-to-pointer cleanup for format contexts.
		ffmpeg.FormatCloseInput(unsafe.Pointer(&inFmtCtx))
	}, nil
}

func freeInputOptions(opts *unsafe.Pointer) {
	if opts == nil || *opts == nil {
		return
	}
	// av_dict_free expects an AVDictionary**. Passing the dictionary pointer
	// itself corrupts memory and can crash inside libavutil during cleanup.
	//nolint:gosec // FFmpeg requires passing the dictionary pointer by address for cleanup.
	ffmpeg.DictFree(unsafe.Pointer(opts))
	*opts = nil
}

func setInputOption(opts *unsafe.Pointer, key, value string) error {
	if value == "" {
		return nil
	}
	//nolint:gosec // FFmpeg requires passing the dictionary pointer by address for option updates.
	if ret := ffmpeg.DictSet(unsafe.Pointer(opts), key, value, 0); ret < 0 {
		return fmt.Errorf("set input option %q: %d", key, ret)
	}
	return nil
}

func formatHTTPHeaderOptions(headers map[string]string) string {
	if len(headers) == 0 {
		return ""
	}

	keys := make([]string, 0, len(headers))
	for key, value := range headers {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if strings.EqualFold(key, "User-Agent") || strings.EqualFold(key, "Referer") {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return ""
	}

	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteString(": ")
		builder.WriteString(headers[key])
		builder.WriteString("\r\n")
	}
	return builder.String()
}

// openVideoDecoder creates and opens a decoder context for the video stream.
// It prefers hardware decoders (h264_vaapi, h264_cuvid) based on hwCaps,
// falling back to the generic software decoder.
//
//nolint:gosec // FFmpeg decoder setup requires unsafe pointer-based contexts and cleanup.
func (p *pipeline) openVideoDecoder(codecPar unsafe.Pointer) (unsafe.Pointer, error) {
	codecID := ffmpeg.CodecParCodecID(codecPar)

	// Try HW decoder first.
	var dec unsafe.Pointer
	for _, name := range p.hwCaps.Decoders {
		dec = ffmpeg.CodecFindDecoderByName(name)
		if dec != nil {
			break
		}
	}

	// Fallback to software decoder.
	if dec == nil {
		dec = ffmpeg.CodecFindDecoder(codecID)
	}
	if dec == nil {
		return nil, fmt.Errorf("no decoder found for codec ID %d", codecID)
	}

	decCtx := ffmpeg.CodecAllocContext3(dec)
	if decCtx == nil {
		return nil, errors.New("failed to allocate video decoder context")
	}

	if ret := ffmpeg.CodecParametersToContext(decCtx, codecPar); ret < 0 {
		ffmpeg.CodecFreeContext(unsafe.Pointer(&decCtx))
		return nil, fmt.Errorf("copy video codec params: %d", ret)
	}

	if ret := ffmpeg.CodecOpen2(decCtx, dec, nil); ret < 0 {
		ffmpeg.CodecFreeContext(unsafe.Pointer(&decCtx))
		return nil, fmt.Errorf("open video decoder: %d", ret)
	}

	return decCtx, nil
}

// openVideoEncoder creates a hardware device context and encoder for
// the video stream. It uses the first available encoder from hwCaps.Encoders.
func (p *pipeline) openVideoEncoder(decCtx unsafe.Pointer) (hwDeviceCtx, encCtx unsafe.Pointer, err error) {
	if len(p.hwCaps.Encoders) == 0 {
		return nil, nil, errors.New("no GPU encoders available")
	}

	// Create the HW device context.
	var deviceType int32
	switch p.hwCaps.API {
	case hwAPINameVAAPI:
		deviceType = int32(ffmpeg.HwdeviceTypeVaapi)
	case "nvenc":
		deviceType = int32(ffmpeg.HwdeviceTypeCuda)
	default:
		return nil, nil, fmt.Errorf("unknown HW API: %s", p.hwCaps.API)
	}

	hwDeviceCtx, hwErr := ffmpeg.HWDeviceCtxCreate(deviceType, p.hwCaps.Device)
	if hwErr != nil {
		return nil, nil, fmt.Errorf("create HW device: %w", hwErr)
	}

	width := ffmpeg.CodecCtxWidth(decCtx)
	height := ffmpeg.CodecCtxHeight(decCtx)

	var lastErr error
	for _, name := range p.hwCaps.Encoders {
		enc := ffmpeg.CodecFindEncoderByName(name)
		if enc == nil {
			continue
		}

		encCtx, err = p.tryOpenVideoEncoder(decCtx, hwDeviceCtx, name, enc)
		if err == nil {
			p.logger.Info().
				Str("session_id", p.sessionID).
				Str("source_url", p.sourceURL).
				Str("encoder", name).
				Str("api", p.hwCaps.API).
				Int32("width", width).
				Int32("height", height).
				Msg("transcode pipeline selected video encoder")
			return hwDeviceCtx, encCtx, nil
		}
		lastErr = err
		p.logger.Warn().
			Str("session_id", p.sessionID).
			Str("source_url", p.sourceURL).
			Str("encoder", name).
			Err(err).
			Msg("transcode pipeline encoder candidate failed")
	}

	ffmpeg.BufferUnref(&hwDeviceCtx)
	if lastErr != nil {
		return nil, nil, fmt.Errorf("no usable encoder codec found: %w", lastErr)
	}
	return nil, nil, errors.New("no usable encoder codec found")
}

func (p *pipeline) tryOpenVideoEncoder(decCtx, hwDeviceCtx unsafe.Pointer, encoderName string, enc unsafe.Pointer) (unsafe.Pointer, error) {
	encCtx := ffmpeg.CodecAllocContext3(enc)
	if encCtx == nil {
		return nil, errors.New("failed to allocate video encoder context")
	}

	width := ffmpeg.CodecCtxWidth(decCtx)
	height := ffmpeg.CodecCtxHeight(decCtx)
	ffmpeg.CodecCtxSetWidth(encCtx, width)
	ffmpeg.CodecCtxSetHeight(encCtx, height)

	framerate := ffmpeg.CodecCtxFramerate(decCtx)
	if framerate.Num == 0 {
		framerate = ffmpeg.AVRational{Num: 30, Den: 1}
	}
	ffmpeg.CodecCtxSetFramerate(encCtx, framerate)
	ffmpeg.CodecCtxSetTimeBase(encCtx, ffmpeg.AVRational{Num: framerate.Den, Den: framerate.Num})

	if p.hwCaps.API == hwAPINameVAAPI {
		ffmpeg.CodecCtxSetPixFmt(encCtx, int32(ffmpeg.PixFmtVaapi))
	} else {
		ffmpeg.CodecCtxSetPixFmt(encCtx, int32(ffmpeg.PixFmtNv12))
	}

	gopSize := framerate.Num / framerate.Den * 2
	if gopSize <= 0 {
		gopSize = 60
	}
	codecCtxSetGopSize(encCtx, gopSize)
	p.applyQualityPreset(encCtx, encoderName, width, height)

	hwDeviceRef := ffmpeg.BufferRef(hwDeviceCtx)
	if hwDeviceRef == nil {
		//nolint:gosec // FFmpeg requires pointer-to-pointer cleanup for codec contexts.
		ffmpeg.CodecFreeContext(unsafe.Pointer(&encCtx))
		return nil, errors.New("failed to ref HW device context for encoder")
	}
	ffmpeg.CodecCtxSetHwDeviceCtx(encCtx, hwDeviceRef)

	if p.hwCaps.API == hwAPINameVAAPI {
		framesRef := ffmpeg.HWFrameCtxAlloc(hwDeviceCtx)
		if framesRef == nil {
			//nolint:gosec // FFmpeg requires pointer-to-pointer cleanup for codec contexts.
			ffmpeg.CodecFreeContext(unsafe.Pointer(&encCtx))
			return nil, errors.New("failed to allocate HW frames context")
		}

		hwFramesData := ffmpeg.BufferRefData(framesRef)
		if hwFramesData == nil {
			ffmpeg.BufferUnref(&framesRef)
			//nolint:gosec // FFmpeg requires pointer-to-pointer cleanup for codec contexts.
			ffmpeg.CodecFreeContext(unsafe.Pointer(&encCtx))
			return nil, errors.New("failed to access HW frames context payload")
		}

		ffmpeg.HWFramesCtxSetInitialPoolSize(hwFramesData, vaapiInitialPoolSize)
		ffmpeg.HWFramesCtxSetFormat(hwFramesData, int32(ffmpeg.PixFmtVaapi))
		ffmpeg.HWFramesCtxSetSWFormat(hwFramesData, int32(ffmpeg.PixFmtNv12))
		ffmpeg.HWFramesCtxSetWidth(hwFramesData, width)
		ffmpeg.HWFramesCtxSetHeight(hwFramesData, height)

		if ret := ffmpeg.HWFrameCtxInit(framesRef); ret < 0 {
			ffmpeg.BufferUnref(&framesRef)
			//nolint:gosec // FFmpeg requires pointer-to-pointer cleanup for codec contexts.
			ffmpeg.CodecFreeContext(unsafe.Pointer(&encCtx))
			return nil, fmt.Errorf("init HW frames context: %d", ret)
		}

		ffmpeg.CodecCtxSetHwFramesCtx(encCtx, framesRef)
	}

	if ret := ffmpeg.CodecOpen2(encCtx, enc, nil); ret < 0 {
		//nolint:gosec // FFmpeg requires pointer-to-pointer cleanup for codec contexts.
		ffmpeg.CodecFreeContext(unsafe.Pointer(&encCtx))
		return nil, fmt.Errorf("open video encoder %s: %d", encoderName, ret)
	}

	return encCtx, nil
}

// applyQualityPreset sets bitrate and encoder options based on the
// quality string ("low", "medium", "high").
func (p *pipeline) applyQualityPreset(encCtx unsafe.Pointer, encoderName string, width, height int32) {
	pixels := int64(width) * int64(height)

	// Base bitrate scaled to resolution (targeting 1080p baseline).
	// low: ~1.5 Mbps at 1080p, medium: ~3 Mbps, high: ~6 Mbps.
	var bitsPerPixel float64
	switch p.quality {
	case "low":
		bitsPerPixel = 0.07
	case "high":
		bitsPerPixel = 0.30
	default: // "medium"
		bitsPerPixel = 0.15
	}

	bitrate := int64(float64(pixels) * bitsPerPixel)
	if bitrate < minVideoBitrate {
		bitrate = minVideoBitrate // floor at 500 kbps
	}
	codecCtxSetBitRate(encCtx, bitrate)

	// Set encoder-specific options via av_opt_set.
	if strings.HasSuffix(encoderName, "_vaapi") {
		// VAAPI encoders use rc_mode for rate control.
		ffmpeg.AVOptSet(encCtx, "rc_mode", "VBR", 0)
	} else if strings.HasSuffix(encoderName, "_nvenc") {
		// NVENC uses preset and rc.
		switch p.quality {
		case "low":
			ffmpeg.AVOptSet(encCtx, "preset", "p4", 0)
		case "high":
			ffmpeg.AVOptSet(encCtx, "preset", "p7", 0)
		default:
			ffmpeg.AVOptSet(encCtx, "preset", "p5", 0)
		}
		ffmpeg.AVOptSet(encCtx, "rc", "vbr", 0)
	}
}

// transcodeVideoPacket decodes a video packet, encodes the resulting
// frames, and writes them to the output.
func (p *pipeline) transcodeVideoPacket(pkt, frame, encPkt, decCtx, encCtx, outFmtCtx unsafe.Pointer, outVideoIdx int) error {
	ret := ffmpeg.CodecSendPacket(decCtx, pkt)
	if ret < 0 {
		return fmt.Errorf("video decode send packet: %d", ret)
	}

	for {
		ret = ffmpeg.CodecReceiveFrame(decCtx, frame)
		if ret < 0 {
			// EAGAIN or EOF.
			break
		}

		if err := p.encodeAndWriteVideoFrame(frame, encPkt, encCtx, outFmtCtx, outVideoIdx); err != nil {
			ffmpeg.FrameUnref(frame)
			return err
		}
		ffmpeg.FrameUnref(frame)
	}

	return nil
}

// encodeAndWriteVideoFrame sends a decoded frame to the encoder and
// writes any resulting packets to the output.
func (p *pipeline) encodeAndWriteVideoFrame(frame, encPkt, encCtx, outFmtCtx unsafe.Pointer, outVideoIdx int) error {
	encFrame := frame
	var cleanup func()
	if p.hwCaps.API == hwAPINameVAAPI {
		var err error
		encFrame, cleanup, err = uploadVideoFrameToVAAPI(frame, encCtx)
		if err != nil {
			return err
		}
		defer cleanup()
	}

	ret := ffmpeg.CodecSendFrame(encCtx, encFrame)
	if ret < 0 {
		return fmt.Errorf("video encode send frame: %d", ret)
	}

	for {
		ret = ffmpeg.CodecReceivePacket(encCtx, encPkt)
		if ret < 0 {
			break
		}

		pktSetStreamIndex(encPkt, int32(outVideoIdx))

		outVidStream := ffmpeg.FmtCtxStream(outFmtCtx, outVideoIdx)
		outVidStreamWrap := ffmpeg.WrapStream(outVidStream)
		ffmpeg.PacketRescaleTs(encPkt, ffmpeg.CodecCtxTimeBase(encCtx), outVidStreamWrap.TimeBase())

		if wret := ffmpeg.InterleavedWriteFrame(outFmtCtx, encPkt); wret < 0 {
			ffmpeg.PacketUnref(encPkt)
			return fmt.Errorf("video write frame: %d", wret)
		}
		ffmpeg.PacketUnref(encPkt)
	}

	return nil
}

func uploadVideoFrameToVAAPI(frame, encCtx unsafe.Pointer) (unsafe.Pointer, func(), error) {
	if frameFormat(frame) == int32(ffmpeg.PixFmtVaapi) {
		return frame, func() {}, nil
	}

	hwFramesCtx := ffmpeg.CodecCtxHwFramesCtx(encCtx)
	if hwFramesCtx == nil {
		return nil, nil, errors.New("video encoder missing hw_frames_ctx")
	}

	hwFrame := ffmpeg.FrameAlloc()
	if hwFrame == nil {
		return nil, nil, errors.New("failed to allocate VAAPI frame")
	}

	cleanup := func() {
		if hwFrame != nil {
			//nolint:gosec // FFmpeg requires pointer-to-pointer cleanup for frame allocations.
			ffmpeg.FrameFree(unsafe.Pointer(&hwFrame))
		}
	}

	frameSetFormat(hwFrame, int32(ffmpeg.PixFmtVaapi))
	frameSetWidth(hwFrame, frameWidth(frame))
	frameSetHeight(hwFrame, frameHeight(frame))
	frameSetPts(hwFrame, framePts(frame))

	hwFramesRef := ffmpeg.BufferRef(hwFramesCtx)
	if hwFramesRef == nil {
		cleanup()
		return nil, nil, errors.New("failed to ref encoder hw frames context")
	}
	frameSetHWFramesCtx(hwFrame, hwFramesRef)

	if ret := ffmpeg.HWFrameGetBuffer(hwFramesCtx, hwFrame, 0); ret < 0 {
		cleanup()
		return nil, nil, fmt.Errorf("allocate VAAPI frame buffer: %d", ret)
	}
	if ret := ffmpeg.HWFrameTransferData(hwFrame, frame, 0); ret < 0 {
		cleanup()
		return nil, nil, fmt.Errorf("upload frame to VAAPI: %d", ret)
	}

	return hwFrame, cleanup, nil
}

// averrorEOF is the AVERROR_EOF constant.
const averrorEOF = -('E' | 'O'<<8 | 'F'<<16 | ' '<<24)
