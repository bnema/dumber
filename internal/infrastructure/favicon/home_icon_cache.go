package favicon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	appport "bnema.dev/dumber/internal/application/port"
)

// HomeIconCache stores 128px home service icons on disk in a directory
// separate from browsing favicons. The file mtime is the store time.
type HomeIconCache struct {
	dir string
}

var _ appport.HomeIconCache = (*HomeIconCache)(nil)

// NewHomeIconCache creates a cache rooted at dir (e.g. <favicons>/home).
func NewHomeIconCache(dir string) *HomeIconCache {
	return &HomeIconCache{dir: dir}
}

// homeIconCacheKey hashes scheme + host (with port) + path.
func homeIconCacheKey(serviceURL string) string {
	key := strings.TrimSpace(serviceURL)
	if u, err := url.Parse(key); err == nil && u.Host != "" {
		key = strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + u.EscapedPath()
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func (c *HomeIconCache) path(serviceURL string) string {
	return filepath.Join(c.dir, homeIconCacheKey(serviceURL)+".png")
}

// Get returns the cached PNG and its store time.
func (c *HomeIconCache) Get(_ context.Context, serviceURL string) ([]byte, time.Time, bool) {
	if c == nil || c.dir == "" {
		return nil, time.Time{}, false
	}
	p := c.path(serviceURL)
	info, err := os.Stat(p)
	if err != nil {
		return nil, time.Time{}, false
	}
	data, err := os.ReadFile(p)
	if err != nil || len(data) == 0 {
		return nil, time.Time{}, false
	}
	return data, info.ModTime(), true
}

// Put writes the PNG atomically.
func (c *HomeIconCache) Put(_ context.Context, serviceURL string, pngData []byte) error {
	if c == nil || c.dir == "" {
		return nil
	}
	if err := os.MkdirAll(c.dir, diskCacheDirPerm); err != nil {
		return fmt.Errorf("home icon cache: create dir: %w", err)
	}
	tmp, err := os.CreateTemp(c.dir, ".home-icon-*")
	if err != nil {
		return fmt.Errorf("home icon cache: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(pngData); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("home icon cache: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("home icon cache: close: %w", err)
	}
	if err := os.Chmod(tmpName, diskCacheFilePerm); err != nil {
		return fmt.Errorf("home icon cache: chmod: %w", err)
	}
	if err := os.Rename(tmpName, c.path(serviceURL)); err != nil {
		return fmt.Errorf("home icon cache: rename: %w", err)
	}
	return nil
}
