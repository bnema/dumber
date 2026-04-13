package port

import (
	"context"
	"io"
)

// TranscodeSession represents an active GPU transcoding session.
// Read returns transcoded chunks. Close aborts and frees GPU resources.
type TranscodeSession interface {
	io.ReadCloser
	// ContentType returns the MIME type of the transcoded output (e.g., "video/webm").
	ContentType() string
}

// HWCapabilities describes the detected GPU hardware encoding capabilities.
type HWCapabilities struct {
	API      string   // "vaapi", "nvenc", or "" (none available)
	Device   string   // auto-detected device path
	Encoders []string // e.g., ["av1_vaapi"]
	Decoders []string // e.g., ["h264_vaapi"]
}

// MediaTranscoder handles GPU-accelerated video transcoding.
type MediaTranscoder interface {
	// Available returns true if a compatible GPU encoder was found.
	Available() bool
	// Capabilities returns the detected GPU hardware info.
	Capabilities() HWCapabilities
	// Start begins a GPU transcode session, fetching from sourceURL.
	// Headers are forwarded to the source HTTP request (cookies, auth).
	Start(ctx context.Context, sourceURL string, headers map[string]string) (TranscodeSession, error)
	// Close shuts down all active transcode sessions and releases resources.
	Close()
}
