package transcoder

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/bnema/purego-ffmpeg/ffmpeg"
	"github.com/rs/zerolog"
)

// opusSampleRate is the standard output sample rate for Opus encoding.
// Opus supports 8, 12, 16, 24, and 48 kHz. 48 kHz is the canonical rate.
const opusSampleRate = 48000

// opusBitrate is the default Opus encoding bitrate (128 kbps stereo).
const opusBitrate = 128000

// opusFrameSize is the number of samples per Opus frame at 48 kHz.
// 960 samples = 20 ms at 48 kHz, which is the standard Opus frame duration.
const opusFrameSize = 960

// audioTranscoder decodes an input audio stream (typically AAC) and
// re-encodes it to Opus, performing sample rate conversion if needed.
type audioTranscoder struct {
	decoder   unsafe.Pointer // AVCodecContext* for decoding
	encoder   unsafe.Pointer // AVCodecContext* for Opus encoding
	resampler unsafe.Pointer // SwrContext* (nil if no resampling needed)
	frame     unsafe.Pointer // reusable AVFrame for resampled output
	decFrame  unsafe.Pointer // reusable AVFrame for decoded output
	encPkt    unsafe.Pointer // reusable AVPacket for encoded output
	inStream  int            // input stream index in the source container
	outStream int            // output stream index in the muxer
}

// newAudioTranscoder sets up an AAC (or other) decoder and a libopus encoder.
// inCodecPar is the AVCodecParameters* from the input audio stream.
// outFmtCtx is the AVFormatContext* for the output container (used to
// add the output stream). inStreamIdx is the input stream index.
func newAudioTranscoder(inCodecPar, outFmtCtx unsafe.Pointer, inStreamIdx int) (*audioTranscoder, error) {
	// --- Decoder setup ---
	codecID := ffmpeg.CodecParCodecID(inCodecPar)
	dec := ffmpeg.CodecFindDecoder(codecID)
	if dec == nil {
		return nil, fmt.Errorf("audio decoder not found for codec ID %d", codecID)
	}

	decCtx := ffmpeg.CodecAllocContext3(dec)
	if decCtx == nil {
		return nil, errors.New("failed to allocate audio decoder context")
	}

	if ret := ffmpeg.CodecParametersToContext(decCtx, inCodecPar); ret < 0 {
		ffmpeg.CodecFreeContext(unsafe.Pointer(&decCtx))
		return nil, fmt.Errorf("failed to copy audio codec parameters to decoder: %d", ret)
	}

	if ret := ffmpeg.CodecOpen2(decCtx, dec, nil); ret < 0 {
		ffmpeg.CodecFreeContext(unsafe.Pointer(&decCtx))
		return nil, fmt.Errorf("failed to open audio decoder: %d", ret)
	}

	// --- Encoder setup (libopus) ---
	enc := ffmpeg.CodecFindEncoderByName("libopus")
	if enc == nil {
		ffmpeg.CodecFreeContext(unsafe.Pointer(&decCtx))
		return nil, errors.New("libopus encoder not found")
	}

	encCtx := ffmpeg.CodecAllocContext3(enc)
	if encCtx == nil {
		ffmpeg.CodecFreeContext(unsafe.Pointer(&decCtx))
		return nil, errors.New("failed to allocate audio encoder context")
	}

	// libopus accepts s16 (interleaved) and flt (float interleaved).
	// Planar formats (fltp) are NOT supported and cause encoder open to fail.
	ffmpeg.CodecCtxSetSampleFmt(encCtx, int32(ffmpeg.SampleFmtFlt))
	ffmpeg.CodecCtxSetSampleRate(encCtx, opusSampleRate)
	codecCtxSetBitRate(encCtx, opusBitrate)

	// Copy channel layout from decoder to encoder.
	codecCtxCopyChLayout(encCtx, decCtx)

	// Set time base to 1/sample_rate for audio.
	ffmpeg.CodecCtxSetTimeBase(encCtx, ffmpeg.AVRational{Num: 1, Den: opusSampleRate})

	// Note: WebM container does not require AV_CODEC_FLAG_GLOBAL_HEADER for Opus.

	if ret := ffmpeg.CodecOpen2(encCtx, enc, nil); ret < 0 {
		ffmpeg.CodecFreeContext(unsafe.Pointer(&encCtx))
		ffmpeg.CodecFreeContext(unsafe.Pointer(&decCtx))
		return nil, fmt.Errorf("failed to open audio encoder: %d", ret)
	}

	// --- Add output stream ---
	outStreamPtr := ffmpeg.FormatNewStream(outFmtCtx, nil)
	if outStreamPtr == nil {
		ffmpeg.CodecFreeContext(unsafe.Pointer(&encCtx))
		ffmpeg.CodecFreeContext(unsafe.Pointer(&decCtx))
		return nil, errors.New("failed to create output audio stream")
	}

	outStreamWrap := ffmpeg.WrapStream(outStreamPtr)
	outStreamIdx := int(outStreamWrap.Index())

	// Copy encoder parameters to the output stream's codecpar.
	if ret := ffmpeg.CodecParametersFromContext(outStreamWrap.Codecpar(), encCtx); ret < 0 {
		ffmpeg.CodecFreeContext(unsafe.Pointer(&encCtx))
		ffmpeg.CodecFreeContext(unsafe.Pointer(&decCtx))
		return nil, fmt.Errorf("failed to copy audio encoder params to stream: %d", ret)
	}

	// --- Resampler setup (if sample rates differ) ---
	var swr unsafe.Pointer
	var resampledFrame unsafe.Pointer

	inSampleRate := ffmpeg.CodecCtxSampleRate(decCtx)
	inSampleFmt := ffmpeg.CodecCtxSampleFmt(decCtx)
	needResample := inSampleRate != opusSampleRate || inSampleFmt != int32(ffmpeg.SampleFmtFlt)

	if needResample {
		swr = ffmpeg.SwrAlloc()
		if swr == nil {
			ffmpeg.CodecFreeContext(unsafe.Pointer(&encCtx))
			ffmpeg.CodecFreeContext(unsafe.Pointer(&decCtx))
			return nil, errors.New("failed to allocate resampler")
		}

		// Configure resampler via av_opt_set on the SwrContext.
		ffmpeg.AVOptSetInt(swr, "in_sample_rate", int64(inSampleRate), 0)
		ffmpeg.AVOptSetInt(swr, "out_sample_rate", int64(opusSampleRate), 0)
		ffmpeg.AVOptSet(swr, "in_sample_fmt", ffmpeg.GetSampleFmtName(inSampleFmt), 0)
		ffmpeg.AVOptSet(swr, "out_sample_fmt", ffmpeg.GetSampleFmtName(int32(ffmpeg.SampleFmtFlt)), 0)

		// Modern FFmpeg (8.x) requires explicit channel layout configuration
		// on the SwrContext. Without it, swr_init may succeed but the resampler
		// operates with 0 channels and produces silence. Copy the decoder's
		// channel layout to both in and out (we keep them identical — same
		// number of channels, only sample rate/format changes).
		swrSetChLayoutFromCodecCtx(swr, decCtx, offsetSwrInChLayout)
		swrSetChLayoutFromCodecCtx(swr, decCtx, offsetSwrOutChLayout)

		if ret := ffmpeg.SwrInit(swr); ret < 0 {
			ffmpeg.SwrFree(unsafe.Pointer(&swr))
			ffmpeg.CodecFreeContext(unsafe.Pointer(&encCtx))
			ffmpeg.CodecFreeContext(unsafe.Pointer(&decCtx))
			return nil, fmt.Errorf("failed to init resampler: %d", ret)
		}

		// Allocate a reusable output frame for resampled data.
		resampledFrame = ffmpeg.FrameAlloc()
		if resampledFrame == nil {
			ffmpeg.SwrFree(unsafe.Pointer(&swr))
			ffmpeg.CodecFreeContext(unsafe.Pointer(&encCtx))
			ffmpeg.CodecFreeContext(unsafe.Pointer(&decCtx))
			return nil, errors.New("failed to allocate resampled frame")
		}

		frameSetFormat(resampledFrame, int32(ffmpeg.SampleFmtFlt))
		frameSetSampleRate(resampledFrame, opusSampleRate)
		frameSetNbSamples(resampledFrame, opusFrameSize)
		// Copy channel layout from decoder to the resampled frame.
		codecCtxCopyChLayout(resampledFrame, decCtx)
	}

	decFrame := ffmpeg.FrameAlloc()
	if decFrame == nil {
		if resampledFrame != nil {
			ffmpeg.FrameFree(unsafe.Pointer(&resampledFrame))
		}
		if swr != nil {
			ffmpeg.SwrFree(unsafe.Pointer(&swr))
		}
		ffmpeg.CodecFreeContext(unsafe.Pointer(&encCtx))
		ffmpeg.CodecFreeContext(unsafe.Pointer(&decCtx))
		return nil, errors.New("failed to allocate audio decode frame")
	}

	encPkt := ffmpeg.PacketAlloc()
	if encPkt == nil {
		ffmpeg.FrameFree(unsafe.Pointer(&decFrame))
		if resampledFrame != nil {
			ffmpeg.FrameFree(unsafe.Pointer(&resampledFrame))
		}
		if swr != nil {
			ffmpeg.SwrFree(unsafe.Pointer(&swr))
		}
		ffmpeg.CodecFreeContext(unsafe.Pointer(&encCtx))
		ffmpeg.CodecFreeContext(unsafe.Pointer(&decCtx))
		return nil, errors.New("failed to allocate audio encode packet")
	}

	return &audioTranscoder{
		decoder:   decCtx,
		encoder:   encCtx,
		resampler: swr,
		frame:     resampledFrame,
		decFrame:  decFrame,
		encPkt:    encPkt,
		inStream:  inStreamIdx,
		outStream: outStreamIdx,
	}, nil
}

// processPacket decodes an audio packet, optionally resamples, encodes
// to Opus, and writes the resulting packets to outFmtCtx.
func (a *audioTranscoder) processPacket(pkt, outFmtCtx unsafe.Pointer) error {
	// Send packet to decoder.
	ret := ffmpeg.CodecSendPacket(a.decoder, pkt)
	if ret < 0 {
		return fmt.Errorf("audio decode send packet: %d", ret)
	}

	frame := a.decFrame
	encPkt := a.encPkt

	for {
		ret = ffmpeg.CodecReceiveFrame(a.decoder, frame)
		if ret < 0 {
			// EAGAIN or EOF — no more frames from this packet.
			break
		}

		// Determine which frame to send to the encoder.
		encFrame := frame
		if a.resampler != nil {
			encFrame = a.frame

			// AVFrame.data[0] is the first field at offset 0, so casting the
			// frame pointer to *unsafe.Pointer yields &data[0]. This is the
			// standard FFmpeg pattern for passing data pointers to swr_convert.
			// purego-ffmpeg does not expose typed accessors for the data array,
			// so raw pointer arithmetic is the only option.
			inData := (*unsafe.Pointer)(frame)
			outData := (*unsafe.Pointer)(a.frame)
			inSamples := frameNbSamples(frame)

			// Allocate buffer for output frame if needed.
			frameSetNbSamples(a.frame, opusFrameSize)
			if ret := ffmpeg.FrameGetBuffer(a.frame, 0); ret < 0 {
				ffmpeg.FrameUnref(frame)
				return fmt.Errorf("failed to allocate resampled frame buffer: %d", ret)
			}

			converted := ffmpeg.SwrConvert(
				a.resampler,
				unsafe.Pointer(outData), opusFrameSize,
				unsafe.Pointer(inData), inSamples,
			)
			if converted < 0 {
				ffmpeg.FrameUnref(frame)
				return fmt.Errorf("audio resample failed: %d", converted)
			}
			frameSetNbSamples(a.frame, converted)

			// Copy pts from decoded frame, rescaling for the new sample rate.
			pts := framePts(frame)
			if pts >= 0 {
				decTB := ffmpeg.CodecCtxTimeBase(a.decoder)
				encTB := ffmpeg.CodecCtxTimeBase(a.encoder)
				frameSetPts(a.frame, ffmpeg.RescaleQ(pts, decTB, encTB))
			}
		}

		// Send frame to encoder.
		if ret := ffmpeg.CodecSendFrame(a.encoder, encFrame); ret < 0 {
			ffmpeg.FrameUnref(frame)
			return fmt.Errorf("audio encode send frame: %d", ret)
		}

		// Receive encoded packets.
		for {
			ret = ffmpeg.CodecReceivePacket(a.encoder, encPkt)
			if ret < 0 {
				break
			}

			pktSetStreamIndex(encPkt, int32(a.outStream))

			// Rescale timestamps from encoder time base to output stream time base.
			outStream := ffmpeg.FmtCtxStream(outFmtCtx, a.outStream)
			outStreamWrap := ffmpeg.WrapStream(outStream)
			ffmpeg.PacketRescaleTs(encPkt, ffmpeg.CodecCtxTimeBase(a.encoder), outStreamWrap.TimeBase())

			if wret := ffmpeg.InterleavedWriteFrame(outFmtCtx, encPkt); wret < 0 {
				ffmpeg.PacketUnref(encPkt)
				ffmpeg.FrameUnref(frame)
				return fmt.Errorf("audio write frame: %d", wret)
			}
			ffmpeg.PacketUnref(encPkt)
		}

		ffmpeg.FrameUnref(frame)
	}

	return nil
}

// flush drains remaining frames from the decoder and encoder.
// The logger parameter is used to report non-fatal resampling errors
// that occur during flush (we continue flushing rather than aborting).
func (a *audioTranscoder) flush(outFmtCtx unsafe.Pointer, logger zerolog.Logger) {
	// Flush decoder by sending NULL packet.
	ffmpeg.CodecSendPacket(a.decoder, nil)

	frame := a.decFrame
	encPkt := a.encPkt

	// Drain decoded frames.
	for {
		ret := ffmpeg.CodecReceiveFrame(a.decoder, frame)
		if ret < 0 {
			break
		}

		encFrame := frame
		if a.resampler != nil {
			encFrame = a.frame
			// See processPacket for why these casts are safe: AVFrame.data[0]
			// is at offset 0, so the frame pointer itself is &data[0].
			inData := (*unsafe.Pointer)(frame)
			outData := (*unsafe.Pointer)(a.frame)
			inSamples := frameNbSamples(frame)

			frameSetNbSamples(a.frame, opusFrameSize)
			if ret := ffmpeg.FrameGetBuffer(a.frame, 0); ret < 0 {
				logger.Warn().Int32("ret", ret).Msg("failed to allocate resampled frame buffer during flush")
				ffmpeg.FrameUnref(frame)
				continue
			}

			converted := ffmpeg.SwrConvert(
				a.resampler,
				unsafe.Pointer(outData), opusFrameSize,
				unsafe.Pointer(inData), inSamples,
			)
			if converted < 0 {
				logger.Warn().Int32("ret", converted).Msg("audio resample failed during flush")
				ffmpeg.FrameUnref(frame)
				continue
			}
			frameSetNbSamples(a.frame, converted)
		}

		ffmpeg.CodecSendFrame(a.encoder, encFrame)

		for {
			ret := ffmpeg.CodecReceivePacket(a.encoder, encPkt)
			if ret < 0 {
				break
			}
			pktSetStreamIndex(encPkt, int32(a.outStream))
			outStream := ffmpeg.FmtCtxStream(outFmtCtx, a.outStream)
			outStreamWrap := ffmpeg.WrapStream(outStream)
			ffmpeg.PacketRescaleTs(encPkt, ffmpeg.CodecCtxTimeBase(a.encoder), outStreamWrap.TimeBase())
			ffmpeg.InterleavedWriteFrame(outFmtCtx, encPkt)
			ffmpeg.PacketUnref(encPkt)
		}

		ffmpeg.FrameUnref(frame)
	}

	// Flush encoder by sending NULL frame.
	ffmpeg.CodecSendFrame(a.encoder, nil)
	for {
		ret := ffmpeg.CodecReceivePacket(a.encoder, encPkt)
		if ret < 0 {
			break
		}
		pktSetStreamIndex(encPkt, int32(a.outStream))
		outStream := ffmpeg.FmtCtxStream(outFmtCtx, a.outStream)
		outStreamWrap := ffmpeg.WrapStream(outStream)
		ffmpeg.PacketRescaleTs(encPkt, ffmpeg.CodecCtxTimeBase(a.encoder), outStreamWrap.TimeBase())
		ffmpeg.InterleavedWriteFrame(outFmtCtx, encPkt)
		ffmpeg.PacketUnref(encPkt)
	}
}

// close frees all FFmpeg resources owned by the audio transcoder.
func (a *audioTranscoder) close() {
	if a.encPkt != nil {
		ffmpeg.PacketFree(unsafe.Pointer(&a.encPkt))
	}
	if a.decFrame != nil {
		ffmpeg.FrameFree(unsafe.Pointer(&a.decFrame))
	}
	if a.frame != nil {
		ffmpeg.FrameFree(unsafe.Pointer(&a.frame))
	}
	if a.resampler != nil {
		ffmpeg.SwrFree(unsafe.Pointer(&a.resampler))
	}
	if a.encoder != nil {
		ffmpeg.CodecFreeContext(unsafe.Pointer(&a.encoder))
	}
	if a.decoder != nil {
		ffmpeg.CodecFreeContext(unsafe.Pointer(&a.decoder))
	}
}
