package transcoder

import (
	"mime"
	"net/url"
	"path"
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

// streamingManifestMIMEs lists manifest/container MIME types that usually
// resolve to proprietary segment streams via HLS or DASH.
var streamingManifestMIMEs = map[string]bool{
	"application/vnd.apple.mpegurl": true, // RFC 8216
	"application/x-mpegurl":         true, // common non-standard alias
	"audio/x-mpegurl":               true, // Go stdlib / legacy servers
	"audio/mpegurl":                 true, // rare legacy alias
	"application/dash+xml":          true,
}

var proprietaryVideoExtensions = map[string]bool{
	".mp4":  true,
	".m4v":  true,
	".mov":  true,
	".mkv":  true,
	".avi":  true,
	".flv":  true,
	".3gp":  true,
	".3g2":  true,
	".mpeg": true,
	".mpg":  true,
}

const syntheticTranscodePath = "/__dumber__/transcode.webm"

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

// IsStreamingManifestMIME returns true for HLS/DASH manifest MIME types.
func IsStreamingManifestMIME(contentType string) bool {
	mediaType := parseMediaType(contentType)
	return streamingManifestMIMEs[mediaType]
}

// IsStreamingManifestURL returns true for URLs whose path indicates an HLS or
// DASH manifest entrypoint.
func IsStreamingManifestURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return hasManifestExtension(rawURL)
	}

	return hasManifestExtension(parsed.Path)
}

// IsEagerTranscodeURL returns true for URLs that can be identified as
// transcodable before any response MIME is received.
func IsEagerTranscodeURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}

	if IsSyntheticTranscodeURL(rawURL) {
		return true
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return hasEagerTranscodeExtension(rawURL)
	}

	return hasEagerTranscodeExtension(parsed.Path)
}

// IsSyntheticTranscodeURL returns true for same-origin synthetic media URLs
// that should be intercepted and resolved to an underlying source URL.
func IsSyntheticTranscodeURL(rawURL string) bool {
	_, _, _, ok := ParseSyntheticTranscodeURL(rawURL)
	return ok
}

// ParseSyntheticTranscodeURL extracts the underlying source URL and optional
// forwarded headers from a synthetic transcode URL.
func ParseSyntheticTranscodeURL(rawURL string) (sourceURL, referer, origin string, ok bool) {
	if rawURL == "" {
		return "", "", "", false
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", "", "", false
	}
	if parsed.Path != syntheticTranscodePath {
		return "", "", "", false
	}

	sourceURL = strings.TrimSpace(parsed.Query().Get("src"))
	if sourceURL == "" {
		return "", "", "", false
	}
	sourceParsed, err := url.Parse(sourceURL)
	if err != nil || sourceParsed.Host == "" || (sourceParsed.Scheme != "http" && sourceParsed.Scheme != "https") {
		return "", "", "", false
	}

	referer = strings.TrimSpace(parsed.Query().Get("referer"))
	origin = strings.TrimSpace(parsed.Query().Get("origin"))
	return sourceURL, referer, origin, true
}

func hasManifestExtension(value string) bool {
	ext := strings.ToLower(path.Ext(value))
	return ext == ".m3u8" || ext == ".mpd"
}

func hasEagerTranscodeExtension(value string) bool {
	ext := strings.ToLower(path.Ext(value))
	return hasManifestExtension(value) || proprietaryVideoExtensions[ext]
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
