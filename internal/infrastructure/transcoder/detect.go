package transcoder

import (
	"mime"
	"strings"
)

// proprietaryVideoMIMEs lists MIME types for containers that typically carry
// proprietary codecs (H.264, HEVC, AAC). These require GPU transcoding to
// open formats (VP8/VP9/AV1 + Opus) for playback in browsers without
// proprietary codec support.
var proprietaryVideoMIMEs = map[string]bool{
	"video/mp4":        true, // H.264/HEVC + AAC
	"video/x-flv":      true, // Flash Video (H.264 or Sorenson)
	"video/3gpp":       true, // 3GPP (H.264 + AAC)
	"video/3gpp2":      true, // 3GPP2 (H.264 + AAC)
	"video/mp2t":       true, // MPEG-TS (usually H.264)
	"video/mpeg":       true, // MPEG-1/2
	"video/x-msvideo":  true, // AVI (often H.264)
	"video/x-matroska": true, // MKV (often H.264/HEVC)
	"video/quicktime":  true, // MOV (H.264 + AAC)
	"video/x-m4v":      true, // M4V (H.264 + AAC)
}

// openVideoMIMEs lists MIME types for containers carrying open codecs
// (VP8/VP9/AV1 + Opus/Vorbis). These do not require transcoding.
var openVideoMIMEs = map[string]bool{
	"video/webm": true, // VP8/VP9/AV1 + Opus/Vorbis
	"video/ogg":  true, // Theora + Vorbis
}

// IsProprietaryVideoMIME returns true if the MIME type indicates a video
// format that typically carries proprietary codecs requiring transcoding.
func IsProprietaryVideoMIME(contentType string) bool {
	mediaType := parseMediaType(contentType)
	return proprietaryVideoMIMEs[mediaType]
}

// IsOpenVideoMIME returns true if the MIME type indicates a video format
// with open codecs that do not require transcoding.
func IsOpenVideoMIME(contentType string) bool {
	mediaType := parseMediaType(contentType)
	return openVideoMIMEs[mediaType]
}

// parseMediaType extracts the MIME type from a Content-Type header value,
// stripping parameters like charset or codecs.
func parseMediaType(contentType string) string {
	if contentType == "" {
		return ""
	}
	// mime.ParseMediaType handles the full RFC 2045 parsing, but is
	// overkill and can fail on non-conforming values. A simple split
	// on ";" is more resilient for our needs.
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType != "" {
		return strings.ToLower(mediaType)
	}
	// Fallback: trim parameters manually
	if idx := strings.IndexByte(contentType, ';'); idx >= 0 {
		contentType = contentType[:idx]
	}
	return strings.ToLower(strings.TrimSpace(contentType))
}
