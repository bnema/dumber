package transcoder

import (
	"github.com/bnema/purego-ffmpeg/ffmpeg"
	"github.com/rs/zerolog"

	"github.com/bnema/dumber/internal/application/port"
)

// usableEncoders lists hardware encoders that output open codecs
// CEF can decode natively (VP9, AV1). H.264/HEVC encoders exist
// but their output cannot be played by CEF's minimal build.
var usableEncoders = map[string]bool{
	"av1_vaapi": true,
	"vp9_vaapi": true,
	"av1_nvenc": true,
}

// isUsableEncoder returns true only for hardware encoders whose output
// codec is open and decodable by CEF (AV1, VP9).
func isUsableEncoder(name string) bool {
	return usableEncoders[name]
}

// hwProfile groups the encoder/decoder names to probe for a given API.
type hwProfile struct {
	api        string // "vaapi" or "nvenc"
	deviceType int32
	encoders   []string
	decoders   []string
}

var vaAPIProfile = hwProfile{
	api:        "vaapi",
	deviceType: int32(ffmpeg.HwdeviceTypeVaapi),
	encoders:   []string{"av1_vaapi", "vp9_vaapi", "h264_vaapi"},
	decoders:   []string{"h264_vaapi", "hevc_vaapi"},
}

var nvencProfile = hwProfile{
	api:        "nvenc",
	deviceType: int32(ffmpeg.HwdeviceTypeCuda),
	// hevc_nvenc and h264_nvenc are probed for logging/diagnostic purposes
	// but filtered as non-usable by isUsableEncoder (only open codecs like
	// AV1 and VP9 are usable in CEF's minimal build).
	encoders: []string{"av1_nvenc", "hevc_nvenc", "h264_nvenc"},
	decoders: []string{"h264_cuvid", "hevc_cuvid"},
}

// ProbeGPU detects available GPU hardware encoding capabilities.
// hwaccelPref selects which APIs to try: "auto" (VAAPI then CUDA),
// "vaapi" (VAAPI only), or "nvenc" (CUDA only).
// Returns empty HWCapabilities if no compatible GPU encoder is found.
func ProbeGPU(hwaccelPref string, logger *zerolog.Logger) port.HWCapabilities {
	if err := ffmpeg.Init(); err != nil {
		logger.Warn().Err(err).Msg("ffmpeg init failed, GPU probe skipped")
		return port.HWCapabilities{}
	}

	var profiles []hwProfile
	switch hwaccelPref {
	case "vaapi":
		profiles = []hwProfile{vaAPIProfile}
	case "nvenc":
		profiles = []hwProfile{nvencProfile}
	default: // "auto" or unrecognised
		profiles = []hwProfile{vaAPIProfile, nvencProfile}
	}

	for _, p := range profiles {
		caps := probeProfile(p, logger)
		if len(caps.Encoders) > 0 {
			return caps
		}
	}

	logger.Info().Msg("no usable GPU encoder found, transcoding will be unavailable")
	return port.HWCapabilities{}
}

// probeProfile attempts to open a hardware device and enumerate the
// available encoders/decoders for a single API profile.
func probeProfile(p hwProfile, logger *zerolog.Logger) port.HWCapabilities {
	deviceCtx, err := ffmpeg.HWDeviceCtxCreate(p.deviceType, "")
	if err != nil {
		logger.Debug().
			Str("api", p.api).
			Err(err).
			Msg("hardware device not available")
		return port.HWCapabilities{}
	}
	// Free the probing device context; a long-lived one will be
	// created per transcode session (or shared across sessions).
	defer ffmpeg.BufferUnref(&deviceCtx)

	logger.Info().
		Str("api", p.api).
		Msg("hardware device opened for probing")

	var encoders []string
	for _, name := range p.encoders {
		if ffmpeg.CodecFindEncoderByName(name) != nil {
			if isUsableEncoder(name) {
				encoders = append(encoders, name)
				logger.Info().Str("encoder", name).Msg("usable GPU encoder found")
			} else {
				logger.Debug().Str("encoder", name).Msg("encoder present but not usable (proprietary output)")
			}
		}
	}

	var decoders []string
	for _, name := range p.decoders {
		if ffmpeg.CodecFindDecoderByName(name) != nil {
			decoders = append(decoders, name)
			logger.Debug().Str("decoder", name).Msg("GPU decoder found")
		}
	}

	return port.HWCapabilities{
		API:      p.api,
		Device:   "", // auto-detected by FFmpeg; reserved for future multi-GPU pinning
		Encoders: encoders,
		Decoders: decoders,
	}
}
