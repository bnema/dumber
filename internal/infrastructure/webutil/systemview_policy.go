package webutil

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/andybalholm/brotli"
)

// Shared system-view (dumb://) request policy used by every browser engine.
// Engines keep only their native request/response translation; routing
// safety, trust, and asset decoding rules live here once.

const (
	// SystemviewsAssetDir is the embedded directory holding system-view assets.
	SystemviewsAssetDir = "systemviews"
	// MaxSystemviewsWASMBytes bounds the decompressed system-view WASM size.
	MaxSystemviewsWASMBytes = 64 * 1024 * 1024
	// SystemviewFaviconSize is the only favicon size served to system views.
	SystemviewFaviconSize = 32
)

// SafeSystemviewsAssetPath validates relPath inside assetDir and returns the
// cleaned full path. It rejects traversal, absolute paths, NUL bytes, and any
// directory other than SystemviewsAssetDir.
func SafeSystemviewsAssetPath(assetDir, relPath string) (fullPath, cleanRelPath string, ok bool) {
	assetDir = strings.Trim(assetDir, "/")
	if assetDir != SystemviewsAssetDir {
		return "", "", false
	}

	relPath = strings.TrimLeft(relPath, "/")
	if relPath == "" || strings.ContainsRune(relPath, '\x00') {
		return "", "", false
	}

	cleanRelPath = path.Clean(relPath)
	if cleanRelPath == "." || cleanRelPath == ".." || strings.HasPrefix(cleanRelPath, "../") || path.IsAbs(cleanRelPath) {
		return "", "", false
	}

	fullPath = path.Join(assetDir, cleanRelPath)
	if fullPath != assetDir && !strings.HasPrefix(fullPath, assetDir+"/") {
		return "", "", false
	}
	return fullPath, cleanRelPath, true
}

// ReadSystemviewAsset reads an embedded asset. For .wasm files a sibling .br
// artifact is authoritative when present: it is decompressed with a
// MaxSystemviewsWASMBytes bound, and a corrupt or oversized artifact is an
// error rather than a silent fallback to the raw file.
func ReadSystemviewAsset(assets fs.FS, fullPath, relPath string) ([]byte, error) {
	if strings.HasSuffix(relPath, ".wasm") {
		if compressed, err := fs.ReadFile(assets, fullPath+".br"); err == nil {
			data, err := io.ReadAll(io.LimitReader(brotli.NewReader(bytes.NewReader(compressed)), MaxSystemviewsWASMBytes+1))
			if err != nil {
				return nil, err
			}
			if len(data) > MaxSystemviewsWASMBytes {
				return nil, fmt.Errorf("decompressed asset %s exceeds %d bytes", fullPath, MaxSystemviewsWASMBytes)
			}
			return data, nil
		}
	}
	return fs.ReadFile(assets, fullPath)
}

// SystemviewTrust describes how an engine identifies system-view requesters.
type SystemviewTrust struct {
	// IsTrustedURL reports whether an origin or referrer URL is a system view.
	IsTrustedURL func(string) bool
	// OpaqueOriginUsesReferrer treats an Origin of "null" like a missing
	// Origin. WebKit needs this because custom-scheme pages have opaque
	// origins; engines that serve system views from a real origin must not.
	OpaqueOriginUsesReferrer bool
}

// Allows decides whether a request may call private system-view APIs. A
// present Origin is authoritative; otherwise the referrer is checked.
func (t SystemviewTrust) Allows(origin, referrer string) bool {
	if t.IsTrustedURL == nil {
		return false
	}
	origin = strings.TrimSpace(origin)
	opaque := strings.EqualFold(origin, "null")
	if origin != "" && (!opaque || !t.OpaqueOriginUsesReferrer) {
		return t.IsTrustedURL(origin)
	}
	return t.IsTrustedURL(strings.TrimSpace(referrer))
}

// DumbURLHost returns the page host of a dumb:// URL (dumb://history or
// dumb:history) and whether raw is a dumb:// URL at all.
func DumbURLHost(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(parsed.Scheme, "dumb") {
		return "", false
	}
	host := parsed.Host
	if host == "" {
		host = parsed.Opaque
	}
	if idx := strings.IndexAny(host, "/?#"); idx >= 0 {
		host = host[:idx]
	}
	return host, true
}

// Client-facing favicon API request errors.
var (
	ErrFaviconMissingDomain     = errors.New("missing domain")
	ErrFaviconUnsupportedSize   = errors.New("unsupported favicon size")
	ErrFaviconInvalidRequestURL = errors.New("invalid request URL")
)

// ParseSystemviewFaviconRequest extracts the domain and size from a favicon
// API URL. Only SystemviewFaviconSize is accepted.
func ParseSystemviewFaviconRequest(rawURL string) (domain string, size int, err error) {
	parsed, parseErr := url.Parse(rawURL)
	if parseErr != nil {
		return "", 0, ErrFaviconInvalidRequestURL
	}
	domain = strings.TrimSpace(parsed.Query().Get("domain"))
	if domain == "" {
		return "", 0, ErrFaviconMissingDomain
	}
	if rawSize := strings.TrimSpace(parsed.Query().Get("size")); rawSize != "" {
		parsedSize, convErr := strconv.Atoi(rawSize)
		if convErr != nil || parsedSize != SystemviewFaviconSize {
			return "", 0, ErrFaviconUnsupportedSize
		}
	}
	return domain, SystemviewFaviconSize, nil
}
