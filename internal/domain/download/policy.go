package download

import (
	"mime"
	"net/url"
	"path"
	"strings"
)

var forcedDownloadExtensions = map[string]struct{}{
	".7z":       {},
	".apk":      {},
	".appimage": {},
	".bz2":      {},
	".deb":      {},
	".dmg":      {},
	".exe":      {},
	".gz":       {},
	".img":      {},
	".iso":      {},
	".msi":      {},
	".pdf":      {},
	".pkg":      {},
	".rar":      {},
	".rpm":      {},
	".tar":      {},
	".tgz":      {},
	".xz":       {},
	".zip":      {},
}

var forcedDownloadMIMETypes = map[string]struct{}{
	"application/pdf":               {},
	"application/x-apple-diskimage": {},
	"application/x-cd-image":        {},
	"application/x-iso9660-image":   {},
}

// ShouldForceDownload returns true when the resource should be downloaded
// instead of navigated/rendered inline.
func ShouldForceDownload(uri, mimeType string) bool {
	return ShouldForceDownloadForMIMEType(mimeType) || ShouldForceDownloadForURI(uri)
}

// ShouldForceDownloadForURI returns true when the URI path has a known
// download-only extension.
func ShouldForceDownloadForURI(uri string) bool {
	ext := strings.ToLower(path.Ext(uriPath(uri)))
	if ext == "" {
		return false
	}
	_, ok := forcedDownloadExtensions[ext]
	return ok
}

// ShouldForceDownloadForMIMEType returns true when the MIME type maps to a
// known download-only artifact.
func ShouldForceDownloadForMIMEType(mimeType string) bool {
	if mimeType == "" {
		return false
	}

	mediaType := strings.ToLower(strings.TrimSpace(mimeType))
	if parsed, _, err := mime.ParseMediaType(mediaType); err == nil {
		mediaType = parsed
	}

	_, ok := forcedDownloadMIMETypes[mediaType]
	return ok
}

func uriPath(raw string) string {
	if raw == "" {
		return ""
	}

	parsed, err := url.Parse(raw)
	if err == nil && parsed.Path != "" {
		return parsed.Path
	}

	if cut := strings.IndexAny(raw, "?#"); cut >= 0 {
		return raw[:cut]
	}

	return raw
}
