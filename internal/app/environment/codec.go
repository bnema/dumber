package environment

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/logging"
)

// ApplyCodecConfiguration applies codec preferences to environment variables and GStreamer settings
func ApplyCodecConfiguration(codecPrefs config.CodecConfig) {
	logging.Info(fmt.Sprintf("[codec] Applying codec preferences: preferred=%s, force_av1=%t, block_vp9=%t",
		codecPrefs.PreferredCodecs, codecPrefs.ForceAV1, codecPrefs.BlockVP9))

	// Build comprehensive GStreamer plugin rankings
	var rankSettings []string

	// Get existing GST_PLUGIN_FEATURE_RANK or start fresh
	existingRank := os.Getenv("GST_PLUGIN_FEATURE_RANK")
	if existingRank != "" {
		rankSettings = strings.Split(existingRank, ",")
	}

	// AV1 decoder promotions (high priority)
	if codecPrefs.ForceAV1 || strings.Contains(codecPrefs.PreferredCodecs, "av1") {
		av1Ranks := []string{
			"vaav1dec:768",  // VA-API AV1 decoder (highest priority)
			"nvav1dec:512",  // NVDEC AV1 decoder
			"av1dec:384",    // Software AV1 decoder (fallback)
			"avdec_av1:256", // libav AV1 decoder
		}
		rankSettings = append(rankSettings, av1Ranks...)
		logging.Info(fmt.Sprintf("[codec] Promoted AV1 decoders for maximum priority"))
	}

	// H.264 decoder management
	if strings.Contains(codecPrefs.PreferredCodecs, "h264") {
		h264Ranks := []string{
			"vah264dec:512",  // VA-API H.264 decoder
			"nvh264dec:384",  // NVDEC H.264 decoder
			"avdec_h264:256", // Software H.264 decoder
		}
		rankSettings = append(rankSettings, h264Ranks...)
	}

	// VP9 decoder demotion/blocking
	if codecPrefs.DisableVP9Hardware || codecPrefs.BlockVP9 {
		vp9Demotions := []string{
			"vaapivp9dec:0", // Block VA-API VP9
			"nvvp9dec:0",    // Block NVDEC VP9
		}
		if codecPrefs.BlockVP9 {
			vp9Demotions = append(vp9Demotions, "vp9dec:0", "avdec_vp9:0") // Block all VP9
		} else {
			// Allow software VP9 with low priority
			vp9Demotions = append(vp9Demotions, "vp9dec:64", "avdec_vp9:64")
		}
		rankSettings = append(rankSettings, vp9Demotions...)
		logging.Info(fmt.Sprintf("[codec] Demoted/blocked VP9 decoders"))
	}

	// VP8 decoder blocking
	if codecPrefs.BlockVP8 {
		vp8Demotions := []string{
			"vaapivp8dec:0",
			"vp8dec:0",
			"avdec_vp8:0",
		}
		rankSettings = append(rankSettings, vp8Demotions...)
		logging.Info(fmt.Sprintf("[codec] Blocked VP8 decoders"))
	}

	// Apply the complete ranking system
	if len(rankSettings) > 0 {
		finalRank := strings.Join(rankSettings, ",")
		if err := os.Setenv("GST_PLUGIN_FEATURE_RANK", finalRank); err != nil {
			logging.Warn(fmt.Sprintf("[codec] Warning: failed to set GST_PLUGIN_FEATURE_RANK: %v", err))
		} else {
			logging.Info(fmt.Sprintf("[codec] Set comprehensive GST_PLUGIN_FEATURE_RANK: %s", finalRank))
		}
	}

	// Set video buffer sizes if specified
	if codecPrefs.VideoBufferSizeMB > 0 {
		bufferSize := strconv.Itoa(codecPrefs.VideoBufferSizeMB * 1024 * 1024)

		if err := os.Setenv("GST_BUFFER_SIZE", bufferSize); err != nil {
			logging.Warn(fmt.Sprintf("[codec] Warning: failed to set GST_BUFFER_SIZE: %v", err))
		}

		if err := os.Setenv("GST_QUEUE2_MAX_SIZE_BYTES", bufferSize); err != nil {
			logging.Warn(fmt.Sprintf("[codec] Warning: failed to set GST_QUEUE2_MAX_SIZE_BYTES: %v", err))
		}

		logging.Info(fmt.Sprintf("[codec] Set video buffer size to %dMB", codecPrefs.VideoBufferSizeMB))
	}

	// Set queue buffer time if specified
	if codecPrefs.QueueBufferTimeSec > 0 {
		// Convert seconds to nanoseconds for GStreamer
		bufferTime := strconv.Itoa(codecPrefs.QueueBufferTimeSec * 1000000000)

		if err := os.Setenv("GST_QUEUE2_MAX_SIZE_TIME", bufferTime); err != nil {
			logging.Warn(fmt.Sprintf("[codec] Warning: failed to set GST_QUEUE2_MAX_SIZE_TIME: %v", err))
		} else {
			logging.Info(fmt.Sprintf("[codec] Set queue buffer time to %ds", codecPrefs.QueueBufferTimeSec))
		}
	}

	// Set additional codec-specific environment variables
	if codecPrefs.ForceAV1 {
		// Enable AV1 hardware decoding if available
		if err := os.Setenv("GST_AV1_DECODER_ENABLE_HW", "1"); err != nil {
			logging.Warn(fmt.Sprintf("[codec] Warning: failed to set GST_AV1_DECODER_ENABLE_HW: %v", err))
		}
	}

	// Disable video post-processing to avoid VA-API issues
	if err := os.Setenv("GST_VAAPI_DISABLE_VPP", "1"); err != nil {
		logging.Warn(fmt.Sprintf("[codec] Warning: failed to set GST_VAAPI_DISABLE_VPP: %v", err))
	}

	logging.Info(fmt.Sprintf("[codec] Codec configuration applied successfully"))
}

// BuildBlockedCodecsList converts config codec blocking preferences to a string slice
func BuildBlockedCodecsList(codecPrefs config.CodecConfig) []string {
	var blocked []string

	if codecPrefs.BlockVP9 {
		blocked = append(blocked, "vp9")
	}
	if codecPrefs.BlockVP8 {
		blocked = append(blocked, "vp8")
	}

	return blocked
}
