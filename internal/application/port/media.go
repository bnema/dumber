// Package port defines interfaces for external dependencies.
package port

import "context"

// MediaDiagnosticsResult contains hardware acceleration detection results.
type MediaDiagnosticsResult struct {
	// GStreamer availability
	GStreamerAvailable bool

	// GStreamer plugins
	HasVAPlugin      bool // gst-plugins-bad VA (modern stateless decoders)
	HasVAAPIPlugin   bool // gstreamer-vaapi (legacy)
	HasNVCodecPlugin bool // nvcodec for NVIDIA

	// Detected hardware decoders
	AV1Decoders  []string
	H264Decoders []string
	H265Decoders []string
	VP9Decoders  []string

	// VA-API info
	VAAPIAvailable bool
	VAAPIDriver    string
	VAAPIVersion   string

	// Summary
	HWAccelAvailable bool
	AV1HWAvailable   bool
	Warnings         []string
}

// MediaDiagnostics provides video playback capability detection.
type MediaDiagnostics interface {
	// RunDiagnostics checks GStreamer plugins and VA-API availability.
	RunDiagnostics(ctx context.Context) *MediaDiagnosticsResult
}
