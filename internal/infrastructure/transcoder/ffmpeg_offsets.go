package transcoder

// Offset-based accessors for FFmpeg struct fields not yet exposed
// by purego-ffmpeg.
//
// Validated against:
//   FFmpeg 8.1 (n8.1)
//   libavcodec   62.28.100
//   libavformat  62.12.100
//   libavutil    60.26.100
//
// All offsets verified via offsetof() on the installed headers.

import "unsafe"

// ---------------------------------------------------------------------------
// AVPacket field offsets (FFmpeg 8.x, libavcodec 62)
// Layout: AVBufferRef* buf (0), int64 pts (8), int64 dts (16),
//         uint8* data (24), int size (32), int stream_index (36), int flags (40)
// ---------------------------------------------------------------------------

const (
	offsetPktStreamIndex = 36
)

func pktStreamIndex(pkt unsafe.Pointer) int32 {
	return *(*int32)(unsafe.Add(pkt, offsetPktStreamIndex))
}

func pktSetStreamIndex(pkt unsafe.Pointer, idx int32) {
	*(*int32)(unsafe.Add(pkt, offsetPktStreamIndex)) = idx
}

// ---------------------------------------------------------------------------
// AVFrame field offsets (FFmpeg 8.x, libavutil 60)
// Layout (relevant fields):
//   data[8]       at offset 0    (8 pointers = 64 bytes)
//   linesize[8]   at offset 64   (8 x int32 = 32 bytes)
//   ...
//   width         at offset 104
//   height        at offset 108
//   nb_samples    at offset 112
//   format        at offset 116
//   pts           at offset 136
//   sample_rate   at offset 180
//   hw_frames_ctx at offset 328 (AVBufferRef*)
//   ch_layout     at offset 384 (AVChannelLayout, 24 bytes)
// ---------------------------------------------------------------------------

const (
	offsetFrameWidth       = 104
	offsetFrameHeight      = 108
	offsetFrameNbSamples   = 112
	offsetFrameFormat      = 116
	offsetFrameHWFramesCtx = 328
	offsetFramePts         = 136
	offsetFrameSampleRate  = 180
)

func frameWidth(f unsafe.Pointer) int32 {
	return *(*int32)(unsafe.Add(f, offsetFrameWidth))
}

func frameSetWidth(f unsafe.Pointer, v int32) {
	*(*int32)(unsafe.Add(f, offsetFrameWidth)) = v
}

func frameHeight(f unsafe.Pointer) int32 {
	return *(*int32)(unsafe.Add(f, offsetFrameHeight))
}

func frameSetHeight(f unsafe.Pointer, v int32) {
	*(*int32)(unsafe.Add(f, offsetFrameHeight)) = v
}

func frameNbSamples(f unsafe.Pointer) int32 {
	return *(*int32)(unsafe.Add(f, offsetFrameNbSamples))
}

func frameSetNbSamples(f unsafe.Pointer, v int32) {
	*(*int32)(unsafe.Add(f, offsetFrameNbSamples)) = v
}

func frameFormat(f unsafe.Pointer) int32 {
	return *(*int32)(unsafe.Add(f, offsetFrameFormat))
}

func frameSetFormat(f unsafe.Pointer, v int32) {
	*(*int32)(unsafe.Add(f, offsetFrameFormat)) = v
}

func frameSetHWFramesCtx(f, v unsafe.Pointer) {
	*(*unsafe.Pointer)(unsafe.Add(f, offsetFrameHWFramesCtx)) = v
}

func framePts(f unsafe.Pointer) int64 {
	return *(*int64)(unsafe.Add(f, offsetFramePts))
}

func frameSetPts(f unsafe.Pointer, v int64) {
	*(*int64)(unsafe.Add(f, offsetFramePts)) = v
}

func frameSetSampleRate(f unsafe.Pointer, v int32) {
	*(*int32)(unsafe.Add(f, offsetFrameSampleRate)) = v
}

// ---------------------------------------------------------------------------
// AVCodecContext additional offsets (FFmpeg 8.x, libavcodec 62)
// ---------------------------------------------------------------------------

const (
	offsetCodecCtxGopSize  = 332
	offsetCodecCtxBitRate  = 56
	offsetCodecCtxChLayout = 352 // AVChannelLayout struct (24 bytes)
)

func codecCtxSetGopSize(ctx unsafe.Pointer, v int32) {
	*(*int32)(unsafe.Add(ctx, offsetCodecCtxGopSize)) = v
}

func codecCtxSetBitRate(ctx unsafe.Pointer, v int64) {
	*(*int64)(unsafe.Add(ctx, offsetCodecCtxBitRate)) = v
}

// chLayoutCopy copies the 24-byte AVChannelLayout from src to dst.
// Both must point to the ch_layout field within their respective AVCodecContext.
//
//nolint:gosec // These offsets target fixed-size FFmpeg structs validated against headers.
func codecCtxCopyChLayout(dst, src unsafe.Pointer) {
	dstLayout := unsafe.Add(dst, offsetCodecCtxChLayout)
	srcLayout := unsafe.Add(src, offsetCodecCtxChLayout)
	// AVChannelLayout is 24 bytes in FFmpeg 8.x.
	copy(
		unsafe.Slice((*byte)(dstLayout), channelLayoutSize),
		unsafe.Slice((*byte)(srcLayout), channelLayoutSize),
	)
}

// ---------------------------------------------------------------------------
// SwrContext channel layout offsets (FFmpeg 8.x, libswresample)
// SwrContext is opaque; offsets verified by writing known AVChannelLayout
// values via av_opt_set_chlayout and scanning the resulting memory.
// ---------------------------------------------------------------------------

const (
	channelLayoutSize = 24

	offsetSwrInChLayout  = 192 // AVChannelLayout (24 bytes)
	offsetSwrOutChLayout = 216 // AVChannelLayout (24 bytes)
)

// swrCopyChLayoutFromCodecCtx copies an AVChannelLayout (24 bytes) from
// an AVCodecContext's ch_layout field into the SwrContext's in/out ch_layout.
//
//nolint:gosec // These offsets target fixed-size FFmpeg structs validated against headers.
func swrSetChLayoutFromCodecCtx(swr, codecCtx unsafe.Pointer, swrOffset int) {
	dst := unsafe.Add(swr, swrOffset)
	src := unsafe.Add(codecCtx, offsetCodecCtxChLayout)
	copy(
		unsafe.Slice((*byte)(dst), channelLayoutSize),
		unsafe.Slice((*byte)(src), channelLayoutSize),
	)
}
