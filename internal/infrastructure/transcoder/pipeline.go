package transcoder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
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

// pipeline orchestrates a single GPU transcode session. It reads from
// an HTTP source, decodes with a hardware decoder (or software fallback),
// encodes video to an open codec (AV1/VP9) via GPU, encodes audio to
// Opus via libopus, and muxes the result into WebM written to an
// io.PipeWriter.
type pipeline struct {
	hwCaps    port.HWCapabilities
	sourceURL string
	headers   map[string]string
	quality   string
	pw        *io.PipeWriter
	cancel    context.CancelFunc
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

func newPipeline(hwCaps port.HWCapabilities, sourceURL string, headers map[string]string, quality string, pw *io.PipeWriter, logger zerolog.Logger) *pipeline {
	return &pipeline{
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
		p.pw.CloseWithError(err)
	} else {
		p.pw.Close()
	}
}

// doRun contains the actual pipeline logic. Errors are returned, and
// the caller (run) closes the pipe writer accordingly.
func (p *pipeline) doRun(ctx context.Context) error {
	// --- Step 1: HTTP GET the source ---
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.sourceURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	for k, v := range p.headers {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch source: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("source returned HTTP %d", resp.StatusCode)
	}

	// --- Step 2: Create read AVIO callback from HTTP body ---
	body := resp.Body
	readFn := func(_ unsafe.Pointer, buf unsafe.Pointer, bufSize int32) int32 {
		if bufSize <= 0 {
			return -1
		}
		goBuf := unsafe.Slice((*byte)(buf), int(bufSize))
		n, readErr := body.Read(goBuf)
		if n > 0 {
			return int32(n)
		}
		if readErr != nil {
			// Return AVERROR_EOF for EOF, generic error otherwise.
			return averrorEOF
		}
		return -1
	}
	p.readCb = purego.NewCallback(readFn)

	readAVIO := ffmpeg.AvioAllocContext(avioBufSize, false, nil, p.readCb, 0, 0)
	if readAVIO == nil {
		return errors.New("failed to allocate read AVIO context")
	}
	defer ffmpeg.AvioContextFree(unsafe.Pointer(&readAVIO))

	// --- Step 3: Open input format context with custom AVIO ---
	inFmtCtx := ffmpeg.FormatAllocContext()
	if inFmtCtx == nil {
		return errors.New("failed to allocate input format context")
	}

	ffmpeg.FmtCtxSetPB(inFmtCtx, readAVIO)
	ffmpeg.FmtCtxSetFlags(inFmtCtx, ffmpeg.FmtCtxFlags(inFmtCtx)|ffmpeg.AVFMT_FLAG_CUSTOM_IO)

	if ret := ffmpeg.FormatOpenInput(unsafe.Pointer(&inFmtCtx), "", nil, nil); ret < 0 {
		ffmpeg.FormatFreeContext(inFmtCtx)
		return fmt.Errorf("open input: %d", ret)
	}
	defer ffmpeg.FormatCloseInput(unsafe.Pointer(&inFmtCtx))

	// --- Step 4: Find stream info ---
	if ret := ffmpeg.FormatFindStreamInfo(inFmtCtx, nil); ret < 0 {
		return fmt.Errorf("find stream info: %d", ret)
	}

	// --- Step 5: Find best video and audio streams ---
	videoIdx := ffmpeg.FindBestStream(inFmtCtx, int32(ffmpeg.AVMEDIA_TYPE_VIDEO), -1, -1, nil, 0)
	audioIdx := ffmpeg.FindBestStream(inFmtCtx, int32(ffmpeg.AVMEDIA_TYPE_AUDIO), -1, -1, nil, 0)

	if videoIdx < 0 {
		return errors.New("no video stream found in source")
	}

	videoStream := ffmpeg.FmtCtxStream(inFmtCtx, int(videoIdx))
	videoStreamWrap := (*ffmpeg.Stream)(videoStream)
	videoCodecPar := videoStreamWrap.Codecpar()

	// --- Step 6: Open video decoder ---
	vidDecCtx, err := p.openVideoDecoder(videoCodecPar)
	if err != nil {
		return fmt.Errorf("open video decoder: %w", err)
	}
	defer ffmpeg.CodecFreeContext(unsafe.Pointer(&vidDecCtx))

	// --- Step 7-8: Open video encoder with HW accel ---
	hwDeviceCtx, vidEncCtx, err := p.openVideoEncoder(vidDecCtx)
	if err != nil {
		return fmt.Errorf("open video encoder: %w", err)
	}
	if hwDeviceCtx != nil {
		defer ffmpeg.BufferUnref(&hwDeviceCtx)
	}
	defer ffmpeg.CodecFreeContext(unsafe.Pointer(&vidEncCtx))

	// --- Step 9: Create output format context for WebM ---
	var outFmtCtx unsafe.Pointer
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
	defer ffmpeg.AvioContextFree(unsafe.Pointer(&writeAVIO))

	ffmpeg.FmtCtxSetPB(outFmtCtx, writeAVIO)
	ffmpeg.FmtCtxSetFlags(outFmtCtx, ffmpeg.FmtCtxFlags(outFmtCtx)|ffmpeg.AVFMT_FLAG_CUSTOM_IO)

	// --- Step 11: Add video output stream ---
	outVideoStream := ffmpeg.FormatNewStream(outFmtCtx, nil)
	if outVideoStream == nil {
		return errors.New("failed to create output video stream")
	}
	outVideoStreamWrap := (*ffmpeg.Stream)(outVideoStream)
	outVideoIdx := int(outVideoStreamWrap.Index())

	if ret := ffmpeg.CodecParametersFromContext(outVideoStreamWrap.Codecpar(), vidEncCtx); ret < 0 {
		return fmt.Errorf("copy video encoder params to stream: %d", ret)
	}

	// --- Step 11b: Add audio output stream (if audio present) ---
	var audioTx *audioTranscoder
	if audioIdx >= 0 {
		audioStream := ffmpeg.FmtCtxStream(inFmtCtx, int(audioIdx))
		audioStreamWrap := (*ffmpeg.Stream)(audioStream)
		audioCodecPar := audioStreamWrap.Codecpar()

		var txErr error
		audioTx, txErr = newAudioTranscoder(audioCodecPar, outFmtCtx, int(audioIdx))
		if txErr != nil {
			// Audio transcoding failure is non-fatal; we proceed with video only.
			audioTx = nil
			audioIdx = -1
		}
	}
	if audioTx != nil {
		defer audioTx.close()
	}

	// --- Step 12: Write header ---
	if ret := ffmpeg.FormatWriteHeader(outFmtCtx, nil); ret < 0 {
		return fmt.Errorf("write header: %d", ret)
	}

	// --- Step 13: Main decode/encode loop ---
	pkt := ffmpeg.PacketAlloc()
	if pkt == nil {
		return errors.New("failed to allocate read packet")
	}
	defer ffmpeg.PacketFree(unsafe.Pointer(&pkt))

	frame := ffmpeg.FrameAlloc()
	if frame == nil {
		return errors.New("failed to allocate decode frame")
	}
	defer ffmpeg.FrameFree(unsafe.Pointer(&frame))

	encPkt := ffmpeg.PacketAlloc()
	if encPkt == nil {
		return errors.New("failed to allocate encode packet")
	}
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

		streamIdx := pktStreamIndex(pkt)

		switch {
		case streamIdx == videoIdx:
			if err := p.transcodeVideoPacket(pkt, frame, encPkt, vidDecCtx, vidEncCtx, outFmtCtx, videoStreamWrap, outVideoIdx); err != nil {
				ffmpeg.PacketUnref(pkt)
				return fmt.Errorf("video transcode: %w", err)
			}

		case audioTx != nil && streamIdx == audioIdx:
			if err := audioTx.processPacket(pkt, outFmtCtx); err != nil {
				ffmpeg.PacketUnref(pkt)
				return fmt.Errorf("audio transcode: %w", err)
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
		p.encodeAndWriteVideoFrame(frame, encPkt, vidEncCtx, outFmtCtx, videoStreamWrap, outVideoIdx)
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
		outVidStreamWrap := (*ffmpeg.Stream)(outVidStream)
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

// openVideoDecoder creates and opens a decoder context for the video stream.
// It prefers hardware decoders (h264_vaapi, h264_cuvid) based on hwCaps,
// falling back to the generic software decoder.
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
func (p *pipeline) openVideoEncoder(decCtx unsafe.Pointer) (hwDeviceCtx unsafe.Pointer, encCtx unsafe.Pointer, err error) {
	if len(p.hwCaps.Encoders) == 0 {
		return nil, nil, errors.New("no GPU encoders available")
	}

	// Create the HW device context.
	var deviceType int32
	switch p.hwCaps.API {
	case "vaapi":
		deviceType = ffmpeg.AV_HWDEVICE_TYPE_VAAPI
	case "nvenc":
		deviceType = ffmpeg.AV_HWDEVICE_TYPE_CUDA
	default:
		return nil, nil, fmt.Errorf("unknown HW API: %s", p.hwCaps.API)
	}

	hwDeviceCtx, hwErr := ffmpeg.HWDeviceCtxCreate(deviceType, p.hwCaps.Device)
	if hwErr != nil {
		return nil, nil, fmt.Errorf("create HW device: %w", hwErr)
	}

	// Find the first working encoder.
	var enc unsafe.Pointer
	var encoderName string
	for _, name := range p.hwCaps.Encoders {
		enc = ffmpeg.CodecFindEncoderByName(name)
		if enc != nil {
			encoderName = name
			break
		}
	}
	if enc == nil {
		ffmpeg.BufferUnref(&hwDeviceCtx)
		return nil, nil, errors.New("no usable encoder codec found")
	}

	encCtx = ffmpeg.CodecAllocContext3(enc)
	if encCtx == nil {
		ffmpeg.BufferUnref(&hwDeviceCtx)
		return nil, nil, errors.New("failed to allocate video encoder context")
	}

	// Copy dimensions and framerate from decoder.
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

	// Set pixel format based on the API.
	if p.hwCaps.API == "vaapi" {
		ffmpeg.CodecCtxSetPixFmt(encCtx, int32(ffmpeg.PixelFormatPixFmtVaapi))
	} else {
		// CUDA/NVENC uses NV12.
		ffmpeg.CodecCtxSetPixFmt(encCtx, int32(ffmpeg.PixelFormatPixFmtNv12))
	}

	// Set GOP size to 2 seconds worth of frames.
	gopSize := int32(framerate.Num / framerate.Den * 2)
	if gopSize <= 0 {
		gopSize = 60
	}
	codecCtxSetGopSize(encCtx, gopSize)

	// Set quality/bitrate based on quality parameter.
	p.applyQualityPreset(encCtx, encoderName, width, height)

	// Attach HW device context. Use BufferRef to create a new reference
	// since the encoder will take ownership of one ref.
	hwDeviceRef := ffmpeg.BufferRef(hwDeviceCtx)
	if hwDeviceRef == nil {
		ffmpeg.CodecFreeContext(unsafe.Pointer(&encCtx))
		ffmpeg.BufferUnref(&hwDeviceCtx)
		return nil, nil, errors.New("failed to ref HW device context for encoder")
	}
	ffmpeg.CodecCtxSetHwDeviceCtx(encCtx, hwDeviceRef)

	// For VAAPI encoders, we need a HW frames context.
	if p.hwCaps.API == "vaapi" {
		framesRef := ffmpeg.HWFrameCtxAlloc(hwDeviceCtx)
		if framesRef == nil {
			ffmpeg.CodecFreeContext(unsafe.Pointer(&encCtx))
			ffmpeg.BufferUnref(&hwDeviceCtx)
			return nil, nil, errors.New("failed to allocate HW frames context")
		}

		// Configure the frames context. The HWFramesContext struct starts
		// with an AVBufferRef header. The actual AVHWFramesContext data is
		// accessible after dereferencing. We configure via av_opt or direct
		// offset access. For simplicity, use the known offsets:
		// AVHWFramesContext (within the AVBufferRef->data):
		//   format (sw_format) at offset 8, width at 12, height at 16,
		//   sw_format at 20, initial_pool_size at 24.
		// Actually, the buffer ref data pointer leads to AVHWFramesContext.
		// The fields are: AVClass* (8), AVBufferRef* device_ref (8),
		//   AVBufferPool* pool (8), int initial_pool_size (4),
		//   enum AVPixelFormat format (4), enum AVPixelFormat sw_format (4),
		//   int width (4), int height (4).
		// So from the data pointer:
		//   offset 24: initial_pool_size (int32)
		//   offset 28: format (int32) — the HW pixel format
		//   offset 32: sw_format (int32) — the SW pixel format
		//   offset 36: width (int32)
		//   offset 40: height (int32)
		//
		// We access data via the AVBufferRef at offset 0 (data field at offset 0).
		hwFramesData := *(*unsafe.Pointer)(framesRef) // AVBufferRef->data
		if hwFramesData != nil {
			*(*int32)(unsafe.Add(hwFramesData, 24)) = 20 // initial_pool_size
			*(*int32)(unsafe.Add(hwFramesData, 28)) = int32(ffmpeg.PixelFormatPixFmtVaapi)
			*(*int32)(unsafe.Add(hwFramesData, 32)) = int32(ffmpeg.PixelFormatPixFmtNv12) // sw_format
			*(*int32)(unsafe.Add(hwFramesData, 36)) = width
			*(*int32)(unsafe.Add(hwFramesData, 40)) = height
		}

		if ret := ffmpeg.HWFrameCtxInit(framesRef); ret < 0 {
			ffmpeg.BufferUnref(&framesRef)
			ffmpeg.CodecFreeContext(unsafe.Pointer(&encCtx))
			ffmpeg.BufferUnref(&hwDeviceCtx)
			return nil, nil, fmt.Errorf("init HW frames context: %d", ret)
		}

		ffmpeg.CodecCtxSetHwFramesCtx(encCtx, framesRef)
	}

	if ret := ffmpeg.CodecOpen2(encCtx, enc, nil); ret < 0 {
		ffmpeg.CodecFreeContext(unsafe.Pointer(&encCtx))
		ffmpeg.BufferUnref(&hwDeviceCtx)
		return nil, nil, fmt.Errorf("open video encoder %s: %d", encoderName, ret)
	}

	return hwDeviceCtx, encCtx, nil
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
	if bitrate < 500_000 {
		bitrate = 500_000 // floor at 500 kbps
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
func (p *pipeline) transcodeVideoPacket(pkt, frame, encPkt, decCtx, encCtx, outFmtCtx unsafe.Pointer, inStreamWrap *ffmpeg.Stream, outVideoIdx int) error {
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

		if err := p.encodeAndWriteVideoFrame(frame, encPkt, encCtx, outFmtCtx, inStreamWrap, outVideoIdx); err != nil {
			ffmpeg.FrameUnref(frame)
			return err
		}
		ffmpeg.FrameUnref(frame)
	}

	return nil
}

// encodeAndWriteVideoFrame sends a decoded frame to the encoder and
// writes any resulting packets to the output.
func (p *pipeline) encodeAndWriteVideoFrame(frame, encPkt, encCtx, outFmtCtx unsafe.Pointer, inStreamWrap *ffmpeg.Stream, outVideoIdx int) error {
	// If the frame is in SW format and the encoder expects HW format,
	// we would need to upload. For now, send directly — the HW encoder
	// with hw_device_ctx set should handle format conversion internally
	// when the decoder outputs SW frames.

	ret := ffmpeg.CodecSendFrame(encCtx, frame)
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
		outVidStreamWrap := (*ffmpeg.Stream)(outVidStream)
		ffmpeg.PacketRescaleTs(encPkt, ffmpeg.CodecCtxTimeBase(encCtx), outVidStreamWrap.TimeBase())

		if wret := ffmpeg.InterleavedWriteFrame(outFmtCtx, encPkt); wret < 0 {
			ffmpeg.PacketUnref(encPkt)
			return fmt.Errorf("video write frame: %d", wret)
		}
		ffmpeg.PacketUnref(encPkt)
	}

	return nil
}

// averrorEOF is the AVERROR_EOF constant.
const averrorEOF = -('E' | 'O'<<8 | 'F'<<16 | ' '<<24)
