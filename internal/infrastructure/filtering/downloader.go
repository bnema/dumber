package filtering

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/bnema/dumber/internal/logging"
)

const (
	cacheDirPerm     = 0o755
	manifestFilePerm = 0o644
)

// Downloader handles downloading filter files from GitHub releases.
type Downloader struct {
	baseURL    string
	cacheDir   string
	httpClient *http.Client
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
	}
}

// GetCachedManifest reads the locally cached manifest.json.
// Returns nil if no cached manifest exists.
func (d *Downloader) GetCachedManifest() (*Manifest, error) {
	path := filepath.Join(d.cacheDir, FilterFiles.Manifest)
	data, err := os.ReadFile(path)
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

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest body: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
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

// DownloadFilters downloads filter JSON files from GitHub releases.
// Returns the paths to the downloaded files.
func (d *Downloader) DownloadFilters(ctx context.Context, onProgress func(DownloadProgress)) ([]string, error) {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-downloader").
		Logger()

	// Ensure cache directory exists
	if err := os.MkdirAll(d.cacheDir, cacheDirPerm); err != nil {
		return nil, fmt.Errorf("failed to create cache dir: %w", err)
	}

	files := FilterFiles.Combined
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

		path, bytesDownloaded, err := d.downloadFile(ctx, filename)
		if err != nil {
			log.Error().Err(err).Str("file", filename).Msg("failed to download filter file")
			return nil, err
		}

		totalBytes += bytesDownloaded
		paths = append(paths, path)
		log.Debug().Str("file", filename).Int64("bytes", bytesDownloaded).Msg("downloaded filter file")
	}

	// Also download and cache the manifest
	if err := d.downloadAndCacheManifest(ctx); err != nil {
		log.Warn().Err(err).Msg("failed to cache manifest")
		// Non-fatal, continue
	}

	log.Info().Int("files", len(paths)).Int64("total_bytes", totalBytes).Msg("filter download complete")
	return paths, nil
}

// downloadFile downloads a single file and returns its local path.
func (d *Downloader) downloadFile(ctx context.Context, filename string) (string, int64, error) {
	url := fmt.Sprintf("%s/%s", d.baseURL, filename)
	localPath := filepath.Join(d.cacheDir, filename)

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

	// Write to temp file first, then atomic rename
	tmpPath := localPath + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create temp file: %w", err)
	}

	bytesWritten, err := io.Copy(file, resp.Body)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("failed to write %s: %w", filename, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, localPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("failed to rename temp file: %w", err)
	}

	return localPath, bytesWritten, nil
}

// downloadAndCacheManifest downloads and caches the manifest.
func (d *Downloader) downloadAndCacheManifest(ctx context.Context) error {
	manifest, err := d.FetchManifest(ctx)
	if err != nil {
		return err
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
// Returns nil if any files are missing.
func (d *Downloader) GetCachedFilterPaths() []string {
	paths := make([]string, 0, len(FilterFiles.Combined))
	for _, filename := range FilterFiles.Combined {
		path := filepath.Join(d.cacheDir, filename)
		if _, err := os.Stat(path); err != nil {
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
