// Package media provides video playback diagnostics and hardware acceleration detection.
package media

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// Adapter implements port.MediaDiagnostics using system tools.
type Adapter struct{}

// New creates a new media diagnostics adapter.
func New() port.MediaDiagnostics {
	return &Adapter{}
}

// RunDiagnostics checks GStreamer plugins and VA-API availability.
func (a *Adapter) RunDiagnostics(ctx context.Context) *port.MediaDiagnosticsResult {
	log := logging.FromContext(ctx)
	result := &port.MediaDiagnosticsResult{}

	// Check GStreamer plugins via gst-inspect-1.0
	a.checkGStreamerPlugins(ctx, result)

	// Check VA-API via vainfo
	a.checkVAAPI(ctx, result)

	// Determine overall hardware accel availability
	result.HWAccelAvailable = result.HasVAPlugin || result.HasVAAPIPlugin || result.HasNVCodecPlugin
	result.AV1HWAvailable = len(result.AV1Decoders) > 0

	// Generate user-friendly warnings
	a.generateWarnings(result)

	log.Info().
		Bool("hw_accel", result.HWAccelAvailable).
		Bool("av1_hw", result.AV1HWAvailable).
		Bool("va_plugin", result.HasVAPlugin).
		Bool("vaapi_plugin", result.HasVAAPIPlugin).
		Bool("nvcodec_plugin", result.HasNVCodecPlugin).
		Str("vaapi_driver", result.VAAPIDriver).
		Msg("media diagnostics complete")

	return result
}

// checkGStreamerPlugins detects available GStreamer decoder plugins.
func (a *Adapter) checkGStreamerPlugins(ctx context.Context, r *port.MediaDiagnosticsResult) {
	log := logging.FromContext(ctx)

	// Check if gst-inspect-1.0 is available
	gstInspect, err := exec.LookPath("gst-inspect-1.0")
	if err != nil {
		log.Warn().Msg("gst-inspect-1.0 not found - GStreamer not installed, video playback will fail")
		r.GStreamerAvailable = false
		return
	}
	r.GStreamerAvailable = true

	// Check 'va' plugin (modern stateless decoders)
	// Package: gst-plugin-va (Arch) or gstreamer1.0-plugins-bad (Debian/Ubuntu)
	if out, err := exec.CommandContext(ctx, gstInspect, "va").Output(); err == nil {
		r.HasVAPlugin = true
		a.parseVADecoders(string(out), r)
	}

	// Check 'vaapi' plugin (legacy gstreamer-vaapi)
	if out, err := exec.CommandContext(ctx, gstInspect, "vaapi").Output(); err == nil {
		r.HasVAAPIPlugin = true
		a.parseVAAPIDecoders(string(out), r)
	}

	// Check 'nvcodec' plugin (NVIDIA)
	if out, err := exec.CommandContext(ctx, gstInspect, "nvcodec").Output(); err == nil {
		r.HasNVCodecPlugin = true
		a.parseNVCodecDecoders(string(out), r)
	}

	log.Debug().
		Bool("va", r.HasVAPlugin).
		Bool("vaapi", r.HasVAAPIPlugin).
		Bool("nvcodec", r.HasNVCodecPlugin).
		Msg("gstreamer plugins checked")
}

// parseVADecoders extracts decoder names from gst-inspect-1.0 va output.
func (a *Adapter) parseVADecoders(output string, r *port.MediaDiagnosticsResult) {
	if a == nil {
		return
	}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Format: "  vah264dec: VA-API H.264 Decoder"
		if strings.HasPrefix(line, "vaav1dec") {
			r.AV1Decoders = append(r.AV1Decoders, "vaav1dec")
		}
		if strings.HasPrefix(line, "vah264dec") {
			r.H264Decoders = append(r.H264Decoders, "vah264dec")
		}
		if strings.HasPrefix(line, "vah265dec") {
			r.H265Decoders = append(r.H265Decoders, "vah265dec")
		}
		if strings.HasPrefix(line, "vavp9dec") {
			r.VP9Decoders = append(r.VP9Decoders, "vavp9dec")
		}
	}
}

// parseVAAPIDecoders extracts decoder names from gst-inspect-1.0 vaapi output.
func (a *Adapter) parseVAAPIDecoders(output string, r *port.MediaDiagnosticsResult) {
	if a == nil {
		return
	}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Format: "  vaapih264dec: VA-API H264 decoder"
		if strings.HasPrefix(line, "vaapiav1dec") {
			r.AV1Decoders = append(r.AV1Decoders, "vaapiav1dec")
		}
		if strings.HasPrefix(line, "vaapih264dec") {
			r.H264Decoders = append(r.H264Decoders, "vaapih264dec")
		}
		if strings.HasPrefix(line, "vaapih265dec") {
			r.H265Decoders = append(r.H265Decoders, "vaapih265dec")
		}
		if strings.HasPrefix(line, "vaapivp9dec") {
			r.VP9Decoders = append(r.VP9Decoders, "vaapivp9dec")
		}
	}
}

// parseNVCodecDecoders extracts decoder names from gst-inspect-1.0 nvcodec output.
func (a *Adapter) parseNVCodecDecoders(output string, r *port.MediaDiagnosticsResult) {
	if a == nil {
		return
	}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Format: "  nvav1dec: NVDEC AV1 Decoder"
		if strings.HasPrefix(line, "nvav1dec") {
			r.AV1Decoders = append(r.AV1Decoders, "nvav1dec")
		}
		if strings.HasPrefix(line, "nvh264dec") {
			r.H264Decoders = append(r.H264Decoders, "nvh264dec")
		}
		if strings.HasPrefix(line, "nvh265dec") {
			r.H265Decoders = append(r.H265Decoders, "nvh265dec")
		}
		if strings.HasPrefix(line, "nvvp9dec") {
			r.VP9Decoders = append(r.VP9Decoders, "nvvp9dec")
		}
	}
}

// checkVAAPI detects VA-API driver and version.
func (a *Adapter) checkVAAPI(ctx context.Context, r *port.MediaDiagnosticsResult) {
	log := logging.FromContext(ctx)
	if a == nil {
		return
	}

	// Check LIBVA_DRIVER_NAME environment first
	if driver := os.Getenv("LIBVA_DRIVER_NAME"); driver != "" {
		r.VAAPIDriver = driver
	}

	// Run vainfo for detailed information
	vainfo, err := exec.LookPath("vainfo")
	if err != nil {
		log.Debug().Msg("vainfo not found, skipping VA-API driver detection")
		return
	}

	out, err := exec.CommandContext(ctx, vainfo).CombinedOutput()
	if err != nil {
		log.Debug().Err(err).Msg("vainfo failed")
		return
	}

	output := string(out)
	r.VAAPIAvailable = true

	// Parse driver name and version from vainfo output
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		// Extract driver from "vainfo: Driver version: ..."
		if strings.Contains(line, "Driver version:") {
			lowerLine := strings.ToLower(line)
			if strings.Contains(lowerLine, "radeonsi") || strings.Contains(lowerLine, "radeon") {
				r.VAAPIDriver = "radeonsi"
			} else if strings.Contains(lowerLine, "i965") {
				r.VAAPIDriver = "i965"
			} else if strings.Contains(lowerLine, "ihd") || strings.Contains(lowerLine, "intel") {
				r.VAAPIDriver = "iHD"
			} else if strings.Contains(lowerLine, "nvidia") {
				r.VAAPIDriver = "nvidia"
			}
		}

		// Extract VA-API version from "vainfo: VA-API version: X.XX"
		if strings.Contains(line, "VA-API version:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				r.VAAPIVersion = strings.TrimSpace(parts[len(parts)-1])
			}
		}
	}

	log.Debug().
		Bool("available", r.VAAPIAvailable).
		Str("driver", r.VAAPIDriver).
		Str("version", r.VAAPIVersion).
		Msg("vaapi status")
}

// generateWarnings creates user-friendly warning messages.
//nolint:revive // receiver required for interface consistency
func (a *Adapter) generateWarnings(r *port.MediaDiagnosticsResult) {
	// Critical: GStreamer not installed
	if !r.GStreamerAvailable {
		r.Warnings = append(r.Warnings,
			"CRITICAL: GStreamer not installed. Video playback will not work!",
			"Install: gst-plugins-base gst-plugins-good gst-plugins-bad gst-plugins-ugly gst-libav",
		)
		return // No point checking other things
	}

	if !r.HWAccelAvailable {
		r.Warnings = append(r.Warnings,
			"No hardware video acceleration detected. Video will use software decoding (higher CPU).",
			"Install VA plugin: gst-plugin-va (Arch) or gstreamer1.0-plugins-bad (Debian/Ubuntu)",
			"Install VA-API driver: libva-mesa-driver (AMD) | intel-media-driver (Intel) | nvidia-vaapi-driver (NVIDIA)",
		)
	}

	// AV1 is the preferred codec - warn if not available
	if !r.AV1HWAvailable && r.HWAccelAvailable {
		r.Warnings = append(r.Warnings,
			"AV1 hardware decoder not found. AV1 streams will use software decoding.")
	}

	// Check for essential codecs for Twitch
	if len(r.H264Decoders) == 0 && len(r.VP9Decoders) == 0 {
		r.Warnings = append(r.Warnings,
			"No H.264 or VP9 hardware decoders found. Twitch/YouTube may have degraded performance.")
	}

	// Prefer modern VA plugin over legacy VAAPI
	if r.HasVAAPIPlugin && !r.HasVAPlugin {
		r.Warnings = append(r.Warnings,
			"Using legacy gstreamer-vaapi. Install gst-plugin-va (Arch) or "+
				"gstreamer1.0-plugins-bad (Debian/Ubuntu) for modern VA stateless decoders.")
	}
}
