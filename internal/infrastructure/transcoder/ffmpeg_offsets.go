package transcoder

// Offset-based accessors for FFmpeg struct fields not yet exposed
// by purego-ffmpeg. These offsets are for FFmpeg 7.x (libavcodec 62,
// libavformat 62). Verified against the same headers used by purego-ffmpeg.

import "unsafe"

// ---------------------------------------------------------------------------
// AVPacket field offsets (FFmpeg 7.x)
// Layout: AVBufferRef* buf (0), int64 pts (8), int64 dts (16),
//         uint8* data (24), int size (32), int stream_index (36), int flags (40)
// ---------------------------------------------------------------------------

const (
	offsetPktPts         = 8
	offsetPktDts         = 16
	offsetPktStreamIndex = 36
)

func pktStreamIndex(pkt unsafe.Pointer) int32 {
	return *(*int32)(unsafe.Add(pkt, offsetPktStreamIndex))
}

func pktSetStreamIndex(pkt unsafe.Pointer, idx int32) {
	*(*int32)(unsafe.Add(pkt, offsetPktStreamIndex)) = idx
}

// ---------------------------------------------------------------------------
// AVFrame field offsets (FFmpeg 7.x, libavutil 60)
// Layout (relevant fields):
//   data[8]      at offset 0    (8 pointers = 64 bytes)
//   linesize[8]  at offset 64   (8 x int32 = 32 bytes)
//   ...
//   width        at offset 268
//   height       at offset 272
//   nb_samples   at offset 276
//   format       at offset 280
//   pts          at offset 328
//   ...
//   sample_rate  at offset 432
//   ch_layout    at offset 440  (AVChannelLayout, 24 bytes)
// ---------------------------------------------------------------------------

const (
	offsetFrameWidth      = 268
	offsetFrameHeight     = 272
	offsetFrameNbSamples  = 276
	offsetFrameFormat     = 280
	offsetFramePts        = 328
	offsetFrameSampleRate = 432
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

func framePts(f unsafe.Pointer) int64 {
	return *(*int64)(unsafe.Add(f, offsetFramePts))
}

func frameSetPts(f unsafe.Pointer, v int64) {
	*(*int64)(unsafe.Add(f, offsetFramePts)) = v
}

func frameSampleRate(f unsafe.Pointer) int32 {
	return *(*int32)(unsafe.Add(f, offsetFrameSampleRate))
}

func frameSetSampleRate(f unsafe.Pointer, v int32) {
	*(*int32)(unsafe.Add(f, offsetFrameSampleRate)) = v
}

// ---------------------------------------------------------------------------
// AVCodecContext additional offsets
// flags at offset 72 (int32, but stored as uint32 in practice)
// ---------------------------------------------------------------------------

const (
	offsetCodecCtxFlags    = 72
	offsetCodecCtxGopSize  = 120
	offsetCodecCtxBitRate  = 28
	offsetCodecCtxChLayout = 368 // AVChannelLayout struct (24 bytes)
)

// AV_CODEC_FLAG_GLOBAL_HEADER indicates the codec uses global headers
// that should be stored in extradata instead of every keyframe.
const avCodecFlagGlobalHeader = 1 << 22

func codecCtxFlags(ctx unsafe.Pointer) int32 {
	return *(*int32)(unsafe.Add(ctx, offsetCodecCtxFlags))
}

func codecCtxSetFlags(ctx unsafe.Pointer, v int32) {
	*(*int32)(unsafe.Add(ctx, offsetCodecCtxFlags)) = v
}

func codecCtxSetGopSize(ctx unsafe.Pointer, v int32) {
	*(*int32)(unsafe.Add(ctx, offsetCodecCtxGopSize)) = v
}

func codecCtxSetBitRate(ctx unsafe.Pointer, v int64) {
	*(*int64)(unsafe.Add(ctx, offsetCodecCtxBitRate)) = v
}

// chLayoutCopy copies the 24-byte AVChannelLayout from src to dst.
// Both must point to the ch_layout field within their respective AVCodecContext.
func codecCtxCopyChLayout(dst, src unsafe.Pointer) {
	dstLayout := unsafe.Add(dst, offsetCodecCtxChLayout)
	srcLayout := unsafe.Add(src, offsetCodecCtxChLayout)
	// AVChannelLayout is 24 bytes in FFmpeg 7.x.
	copy(
		unsafe.Slice((*byte)(dstLayout), 24),
		unsafe.Slice((*byte)(srcLayout), 24),
	)
}

// ---------------------------------------------------------------------------
// AVCodecParameters additional offsets
// ---------------------------------------------------------------------------

const (
	offsetCodecParSampleRate = 152
	offsetCodecParChLayout   = 56 // AVChannelLayout (24 bytes) at offset 56
)

func codecParSampleRate(par unsafe.Pointer) int32 {
	return *(*int32)(unsafe.Add(par, offsetCodecParSampleRate))
}

// codecParCopyChLayoutTo copies the ch_layout from AVCodecParameters to
// an AVCodecContext.
func codecParCopyChLayoutTo(codecCtx, par unsafe.Pointer) {
	dstLayout := unsafe.Add(codecCtx, offsetCodecCtxChLayout)
	srcLayout := unsafe.Add(par, offsetCodecParChLayout)
	copy(
		unsafe.Slice((*byte)(dstLayout), 24),
		unsafe.Slice((*byte)(srcLayout), 24),
	)
}
