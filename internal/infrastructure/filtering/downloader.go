package filtering

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/logging"
)

const (
	cacheDirPerm     = 0o755
	manifestFilePerm = 0o644

	defaultMaxManifestBytes    int64 = 1 << 20 // 1 MiB
	defaultMaxFilterFiles            = 128
	defaultMaxFilterFileBytes  int64 = 64 << 20  // 64 MiB
	defaultMaxTotalFilterBytes int64 = 512 << 20 // 512 MiB
)

// downloadLimits bounds remote filter metadata and payloads.
type downloadLimits struct {
	maxManifestBytes    int64
	maxFilterFiles      int
	maxFilterFileBytes  int64
	maxTotalFilterBytes int64
}

var defaultDownloadLimits = downloadLimits{
	maxManifestBytes:    defaultMaxManifestBytes,
	maxFilterFiles:      defaultMaxFilterFiles,
	maxFilterFileBytes:  defaultMaxFilterFileBytes,
	maxTotalFilterBytes: defaultMaxTotalFilterBytes,
}

// Downloader handles downloading filter files from GitHub releases.
type Downloader struct {
	baseURL    string
	cacheDir   string
	httpClient *http.Client
	limits     downloadLimits
}

// NewDownloader creates a new Downloader.
// cacheDir is where downloaded JSON files will be stored.
func NewDownloader(cacheDir string) *Downloader {
	return &Downloader{
		baseURL:  GitHubReleaseURL,
		cacheDir: cacheDir,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		limits: defaultDownloadLimits,
	}
}

// GetCachedManifest reads the locally cached manifest.json.
// Returns nil if no cached manifest exists.
func (d *Downloader) GetCachedManifest() (*Manifest, error) {
	manifestPath := filepath.Join(d.cacheDir, FilterFiles.Manifest)
	data, err := readLimitedFile(manifestPath, d.limits.maxManifestBytes, "cached manifest")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse cached manifest: %w", err)
	}
	if err := d.validateManifest(&manifest); err != nil {
		return nil, fmt.Errorf("invalid cached manifest: %w", err)
	}
	return &manifest, nil
}

// FetchManifest downloads the latest manifest.json from GitHub.
func (d *Downloader) FetchManifest(ctx context.Context) (*Manifest, error) {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-downloader").
		Logger()

	url := fmt.Sprintf("%s/%s", d.baseURL, FilterFiles.Manifest)
	log.Debug().Str("url", url).Msg("fetching manifest")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Debug().Err(closeErr).Msg("failed to close manifest response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest fetch failed with status %d", resp.StatusCode)
	}

	data, err := readLimitedResponseBody(resp, d.limits.maxManifestBytes, "manifest")
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest body: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}
	if err := d.validateManifest(&manifest); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	log.Debug().Str("version", manifest.Version).Msg("fetched manifest")
	return &manifest, nil
}

// DownloadProgress reports download progress.
type DownloadProgress struct {
	File       string
	Current    int
	Total      int
	BytesTotal int64
}

// DownloadFilters fetches the manifest to discover which filter files exist,
// then downloads them. Returns the paths to the downloaded files.
func (d *Downloader) DownloadFilters(ctx context.Context, onProgress func(DownloadProgress)) ([]string, error) {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-downloader").
		Logger()

	// Ensure cache directory exists
	if err := os.MkdirAll(d.cacheDir, cacheDirPerm); err != nil {
		return nil, fmt.Errorf("failed to create cache dir: %w", err)
	}

	// Fetch manifest to discover which combined files to download
	manifest, err := d.FetchManifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	if len(manifest.Combined.Files) == 0 {
		return nil, fmt.Errorf("manifest contains no combined filter files")
	}

	log.Info().
		Int("files", len(manifest.Combined.Files)).
		Int("total_rules", manifest.Combined.TotalRules).
		Str("version", manifest.Version).
		Msg("manifest lists filter files")

	files := manifest.Combined.Files
	paths := make([]string, 0, len(files))
	var totalBytes int64

	for i, filename := range files {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if onProgress != nil {
			onProgress(DownloadProgress{
				File:       filename,
				Current:    i + 1,
				Total:      len(files),
				BytesTotal: totalBytes,
			})
		}

		remainingBytes := d.limits.maxTotalFilterBytes - totalBytes
		if remainingBytes <= 0 {
			return nil, fmt.Errorf("filter downloads exceed total size limit of %d bytes", d.limits.maxTotalFilterBytes)
		}

		path, bytesDownloaded, err := d.downloadFile(ctx, filename, minInt64(d.limits.maxFilterFileBytes, remainingBytes))
		if err != nil {
			log.Error().Err(err).Str("file", filename).Msg("failed to download filter file")
			return nil, err
		}

		totalBytes += bytesDownloaded
		paths = append(paths, path)
		log.Debug().Str("file", filename).Int64("bytes", bytesDownloaded).Msg("downloaded filter file")
	}

	// Cache the manifest we already fetched
	if err := d.cacheManifest(manifest); err != nil {
		log.Warn().Err(err).Msg("failed to cache manifest")
	}

	log.Info().Int("files", len(paths)).Int64("total_bytes", totalBytes).Msg("filter download complete")
	return paths, nil
}

// downloadFile downloads a single file and returns its local path.
func (d *Downloader) downloadFile(ctx context.Context, filename string, maxBytes int64) (string, int64, error) {
	localPath, safeFilename, err := d.cachePathForFilterFile(filename)
	if err != nil {
		return "", 0, err
	}

	url := fmt.Sprintf("%s/%s", d.baseURL, safeFilename)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("failed to download %s: %w", filename, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logging.FromContext(ctx).Debug().Err(closeErr).Str("file", filename).Msg("failed to close filter response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("download %s failed with status %d", filename, resp.StatusCode)
	}

	if resp.ContentLength > maxBytes {
		return "", 0, fmt.Errorf("filter file %q too large: content length %d exceeds limit %d", safeFilename, resp.ContentLength, maxBytes)
	}

	// Write to temp file in cacheDir, then atomic rename into destination.
	tmpFile, err := os.CreateTemp(d.cacheDir, filepath.Base(safeFilename)+".*.tmp")
	if err != nil {
		return "", 0, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	bytesWritten, err := copyLimited(tmpFile, resp.Body, maxBytes, fmt.Sprintf("filter file %q", safeFilename))
	if err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("failed to write %s: %w", safeFilename, err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := d.renameTempFile(tmpPath, safeFilename); err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, err
	}

	return localPath, bytesWritten, nil
}

func (d *Downloader) cachePathForFilterFile(filename string) (string, string, error) {
	safeFilename, err := validateFilterFilename(filename)
	if err != nil {
		return "", "", err
	}

	cacheDirAbs, err := filepath.Abs(d.cacheDir)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve cache dir: %w", err)
	}
	localPath := filepath.Join(d.cacheDir, filepath.FromSlash(safeFilename))
	targetAbs, err := filepath.Abs(localPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve cache path: %w", err)
	}
	if err := ensurePathInside(cacheDirAbs, targetAbs); err != nil {
		return "", "", fmt.Errorf("invalid filter filename %q: %w", filename, err)
	}

	return localPath, safeFilename, nil
}

func validateFilterFilename(filename string) (string, error) {
	if filename == "" {
		return "", fmt.Errorf("invalid filter filename: empty")
	}
	if strings.TrimSpace(filename) != filename {
		return "", fmt.Errorf("invalid filter filename %q: surrounding whitespace is not allowed", filename)
	}
	if filepath.IsAbs(filename) || strings.HasPrefix(filename, "/") {
		return "", fmt.Errorf("invalid filter filename %q: absolute paths are not allowed", filename)
	}
	if strings.Contains(filename, "\\") {
		return "", fmt.Errorf("invalid filter filename %q: backslashes are not allowed", filename)
	}
	if strings.ContainsAny(filename, "?#") {
		return "", fmt.Errorf("invalid filter filename %q: URL control characters are not allowed", filename)
	}

	parts := strings.Split(filename, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid filter filename %q: path traversal components are not allowed", filename)
		}
	}

	clean := pathpkg.Clean(filename)
	if pathpkg.Ext(clean) != ".json" {
		return "", fmt.Errorf("invalid filter filename %q: expected .json extension", filename)
	}

	return clean, nil
}

func ensurePathInside(baseAbs, targetAbs string) error {
	rel, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil {
		return fmt.Errorf("failed to compare with cache dir: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("resolved path escapes cache dir")
	}
	return nil
}

func (d *Downloader) renameTempFile(tmpPath, safeFilename string) error {
	relTarget := filepath.FromSlash(safeFilename)
	parentDir := filepath.Dir(relTarget)

	root, err := os.OpenRoot(d.cacheDir)
	if err != nil {
		return fmt.Errorf("failed to open cache root: %w", err)
	}
	defer func() { _ = root.Close() }()

	if parentDir != "." {
		if err := root.MkdirAll(parentDir, cacheDirPerm); err != nil {
			return fmt.Errorf("failed to create target dir: %w", err)
		}
	}

	if err := root.Rename(filepath.Base(tmpPath), relTarget); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// cacheManifest writes an already-fetched manifest to disk.
func (d *Downloader) cacheManifest(manifest *Manifest) error {
	if err := d.validateManifest(manifest); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	path := filepath.Join(d.cacheDir, FilterFiles.Manifest)
	if err := os.WriteFile(path, data, manifestFilePerm); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	return nil
}

// GetCachedFilterPaths returns paths to cached filter JSON files.
// Reads the cached manifest to discover which files should exist.
// Returns nil if the manifest is missing or any filter file is missing.
func (d *Downloader) GetCachedFilterPaths() []string {
	manifest, err := d.GetCachedManifest()
	if err != nil || manifest == nil || len(manifest.Combined.Files) == 0 {
		return nil
	}
	paths := make([]string, 0, len(manifest.Combined.Files))
	for _, filename := range manifest.Combined.Files {
		path, safeFilename, err := d.cachePathForFilterFile(filename)
		if err != nil {
			return nil
		}
		if !d.hasCachedFilterFile(safeFilename) {
			return nil
		}
		paths = append(paths, path)
	}
	return paths
}

// HasCachedFilters checks if all filter JSON files are cached.
func (d *Downloader) HasCachedFilters() bool {
	return d.GetCachedFilterPaths() != nil
}

// NeedsUpdate checks if filters need to be updated based on manifest version.
func (d *Downloader) NeedsUpdate(ctx context.Context) (bool, error) {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-downloader").
		Logger()

	cached, err := d.GetCachedManifest()
	if err != nil {
		return true, nil // If we can't read cached, assume we need update
	}
	if cached == nil {
		log.Debug().Msg("no cached manifest, update needed")
		return true, nil
	}

	latest, err := d.FetchManifest(ctx)
	if err != nil {
		return false, err // Can't check, return error
	}

	needsUpdate := cached.Version != latest.Version
	if needsUpdate {
		log.Info().
			Str("cached_version", cached.Version).
			Str("latest_version", latest.Version).
			Msg("filter update available")
	} else {
		log.Debug().Str("version", cached.Version).Msg("filters up to date")
	}

	return needsUpdate, nil
}

// ClearCache removes all cached filter files.
func (d *Downloader) ClearCache() error {
	return os.RemoveAll(d.cacheDir)
}

func (d *Downloader) hasCachedFilterFile(safeFilename string) bool {
	root, err := os.OpenRoot(d.cacheDir)
	if err != nil {
		return false
	}
	defer func() { _ = root.Close() }()

	info, err := root.Stat(filepath.FromSlash(safeFilename))
	return err == nil && !info.IsDir()
}

func (d *Downloader) validateManifest(manifest *Manifest) error {
	if manifest == nil {
		return fmt.Errorf("manifest is nil")
	}
	if len(manifest.Combined.Files) > d.limits.maxFilterFiles {
		return fmt.Errorf("manifest lists %d combined filter files, limit is %d", len(manifest.Combined.Files), d.limits.maxFilterFiles)
	}
	for _, filename := range manifest.Combined.Files {
		if _, _, err := d.cachePathForFilterFile(filename); err != nil {
			return err
		}
	}
	return nil
}

func readLimitedFile(filename string, limit int64, label string) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("%s too large: size %d exceeds limit %d", label, info.Size(), limit)
	}

	return readLimitedReader(file, limit, label)
}

func readLimitedResponseBody(resp *http.Response, limit int64, label string) ([]byte, error) {
	if resp.ContentLength > limit {
		return nil, fmt.Errorf("%s too large: content length %d exceeds limit %d", label, resp.ContentLength, limit)
	}
	return readLimitedReader(resp.Body, limit, label)
}

func readLimitedReader(r io.Reader, limit int64, label string) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("%s size limit must be positive", label)
	}

	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s too large: exceeds limit %d bytes", label, limit)
	}
	return data, nil
}

func copyLimited(dst io.Writer, src io.Reader, limit int64, label string) (int64, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("%s size limit must be positive", label)
	}

	written, err := io.Copy(dst, io.LimitReader(src, limit+1))
	if err != nil {
		return written, err
	}
	if written > limit {
		return written, fmt.Errorf("%s too large: exceeds limit %d bytes", label, limit)
	}
	return written, nil
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// IsCacheStale checks if the cached manifest is older than maxAge.
// Returns true if cache is stale or doesn't exist.
func (d *Downloader) IsCacheStale(maxAge time.Duration) bool {
	path := filepath.Join(d.cacheDir, FilterFiles.Manifest)
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	return time.Since(info.ModTime()) > maxAge
}
