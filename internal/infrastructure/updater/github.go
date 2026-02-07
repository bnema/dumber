// Package updater provides update checking and downloading functionality.
package updater

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/env"
	"github.com/bnema/dumber/internal/logging"
)

const (
	// GitHub API endpoint for latest release.
	githubAPIURL = "https://api.github.com/repos/bnema/dumber/releases/latest"

	// Download URL pattern for the release archive.
	// Uses version-less filename for stable "latest" download URL.
	downloadURLTemplate = "https://github.com/bnema/dumber/releases/latest/download/dumber_%s_%s.tar.gz"

	// Checksums file URL pattern.
	checksumsURLTemplate = "https://github.com/bnema/dumber/releases/latest/download/checksums.txt"

	// HTTP client timeout for API requests.
	apiTimeout = 10 * time.Second

	// HTTP client timeout for downloads.
	downloadTimeout = 5 * time.Minute

	// File permission for directories and executables.
	execPerm = 0o755

	// Maximum size for extracted binary (100MB) - prevents decompression bombs.
	maxBinarySize = 100 * 1024 * 1024

	// Maximum archive download size (150MB) - prevents unbounded downloads.
	maxArchiveSize = 150 * 1024 * 1024

	// Minimum binary size (1MB) - prevents truncated/corrupt binaries.
	minBinarySize = 1 * 1024 * 1024

	// Allowed GitHub download host.
	allowedDownloadHost = "github.com"

	// Maximum number of attempts for retryable network requests.
	maxRetryAttempts = 3

	// Base delay used for exponential backoff between retries.
	retryBaseDelay = 250 * time.Millisecond

	// Maximum delay cap for exponential backoff between retries.
	retryMaxDelay = 2 * time.Second

	// Max random jitter added to each retry backoff.
	retryJitterMax = 200 * time.Millisecond
)

// githubRelease represents the GitHub API response for a release.
type githubRelease struct {
	TagName     string    `json:"tag_name"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Body        string    `json:"body"`
}

// GitHubChecker implements UpdateChecker using the GitHub API.
type GitHubChecker struct {
	client    *http.Client
	randInt63 func(n int64) int64
	sleep     func(ctx context.Context, d time.Duration) error
}

// NewGitHubChecker creates a new GitHub-based update checker.
func NewGitHubChecker() *GitHubChecker {
	return &GitHubChecker{
		client:    &http.Client{Timeout: apiTimeout},
		randInt63: rand.Int63n,
		sleep:     waitForBackoff,
	}
}

// CheckForUpdate checks if a newer version is available on GitHub.
func (g *GitHubChecker) CheckForUpdate(ctx context.Context, currentVersion string) (*entity.UpdateInfo, error) {
	log := logging.FromContext(ctx)

	// Skip check for dev builds.
	if currentVersion == "" || currentVersion == "dev" {
		log.Debug().Str("version", currentVersion).Msg("skipping update check for dev build")
		return &entity.UpdateInfo{
			CurrentVersion: currentVersion,
			LatestVersion:  currentVersion,
			IsNewer:        false,
		}, nil
	}

	// Skip check for package manager installs (Flatpak, AUR).
	// These should use their respective package managers for updates.
	if env.IsFlatpak() || env.IsPacman() {
		log.Debug().
			Bool("flatpak", env.IsFlatpak()).
			Bool("pacman", env.IsPacman()).
			Msg("skipping update check for package manager install")
		return &entity.UpdateInfo{
			CurrentVersion: currentVersion,
			LatestVersion:  currentVersion,
			IsNewer:        false,
		}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubAPIURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "dumber-browser/"+currentVersion)

	resp, err := g.doRequestWithRetry(ctx, req)
	if err != nil {
		if isRetryableRequestError(err) {
			log.Debug().Err(err).Msg("transient update check failure; skipping check")
			return nil, fmt.Errorf("%w: %w", port.ErrUpdateCheckTransient, err)
		}
		return nil, fmt.Errorf("failed to fetch latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		if isRetryableStatus(resp.StatusCode) {
			log.Debug().Int("status", resp.StatusCode).Msg("transient update check API status; skipping check")
			return nil, fmt.Errorf("%w: github API returned status %d", port.ErrUpdateCheckTransient, resp.StatusCode)
		}
		return nil, fmt.Errorf("github API returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode release: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentClean := strings.TrimPrefix(currentVersion, "v")

	isNewer := compareVersions(currentClean, latestVersion) < 0

	// Build download URL based on current architecture.
	archName := getArchName()
	downloadURL := fmt.Sprintf(downloadURLTemplate, runtime.GOOS, archName)

	log.Debug().
		Str("current", currentVersion).
		Str("latest", latestVersion).
		Bool("is_newer", isNewer).
		Msg("update check completed")

	return &entity.UpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  latestVersion,
		IsNewer:        isNewer,
		ReleaseURL:     release.HTMLURL,
		DownloadURL:    downloadURL,
		PublishedAt:    release.PublishedAt,
		ReleaseNotes:   release.Body,
	}, nil
}

// getArchName returns the architecture name used in release assets.
func getArchName() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "386":
		return "i386"
	default:
		return runtime.GOARCH
	}
}

// compareVersions compares two semantic versions.
// Returns -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2.
func compareVersions(v1, v2 string) int {
	// Parse version components.
	parse := func(v string) (major, minor, patch int) {
		// Remove any pre-release suffix for comparison.
		v = regexp.MustCompile(`-.*$`).ReplaceAllString(v, "")
		parts := strings.Split(v, ".")
		if len(parts) >= 1 {
			major, _ = strconv.Atoi(parts[0])
		}
		if len(parts) >= 2 {
			minor, _ = strconv.Atoi(parts[1])
		}
		if len(parts) >= 3 {
			patch, _ = strconv.Atoi(parts[2])
		}
		return
	}

	maj1, min1, pat1 := parse(v1)
	maj2, min2, pat2 := parse(v2)

	if maj1 != maj2 {
		if maj1 < maj2 {
			return -1
		}
		return 1
	}
	if min1 != min2 {
		if min1 < min2 {
			return -1
		}
		return 1
	}
	if pat1 != pat2 {
		if pat1 < pat2 {
			return -1
		}
		return 1
	}
	return 0
}

// GitHubDownloader implements UpdateDownloader for GitHub releases.
type GitHubDownloader struct {
	client    *http.Client
	randInt63 func(n int64) int64
	sleep     func(ctx context.Context, d time.Duration) error
}

// NewGitHubDownloader creates a new GitHub release downloader.
func NewGitHubDownloader() *GitHubDownloader {
	return &GitHubDownloader{
		client:    &http.Client{Timeout: downloadTimeout},
		randInt63: rand.Int63n,
		sleep:     waitForBackoff,
	}
}

func isRetryableStatus(status int) bool {
	if status == http.StatusForbidden || status == http.StatusTooManyRequests || status == http.StatusRequestTimeout {
		return true
	}
	return status >= http.StatusInternalServerError
}

func isRetryableRequestError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return true
		}
		//nolint:staticcheck // net.Error.Temporary is deprecated but still useful for transient detection
		if netErr.Temporary() {
			return true
		}
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if isTransientSyscallError(opErr.Err) {
			return true
		}
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Temporary() {
		return true
	}

	return false
}

func isTransientSyscallError(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	switch errno {
	case syscall.ECONNRESET, syscall.ECONNREFUSED,
		syscall.EADDRNOTAVAIL, syscall.ENETUNREACH,
		syscall.EHOSTUNREACH:
		return true
	}
	return false
}

func waitForBackoff(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryDelayForAttempt(attempt int, randInt63 func(n int64) int64) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := retryBaseDelay
	for i := 1; i < attempt; i++ {
		if delay >= retryMaxDelay {
			delay = retryMaxDelay
			break
		}
		delay *= 2
		if delay > retryMaxDelay {
			delay = retryMaxDelay
		}
	}

	if randInt63 != nil && retryJitterMax > 0 {
		delay += time.Duration(randInt63(int64(retryJitterMax)))
	}

	if delay > retryMaxDelay {
		delay = retryMaxDelay
	}

	return delay
}

// doRequestWithRetryHelper retries the same request instance across attempts.
// It assumes req.Body is non-consumable (for example http.NoBody); it does not clone
// the request or rewind req.Body. Callers needing retries with a real body must provide
// a fresh cloned request per attempt or use a rewindable body implementation.
func doRequestWithRetryHelper(
	ctx context.Context,
	client *http.Client,
	req *http.Request,
	sleep func(ctx context.Context, d time.Duration) error,
	randInt63 func(n int64) int64,
) (*http.Response, error) {
	for attempt := 1; ; attempt++ {
		if attempt > maxRetryAttempts {
			return nil, fmt.Errorf("request failed after retries")
		}
		resp, err := client.Do(req)
		if err != nil {
			if !isRetryableRequestError(err) || attempt == maxRetryAttempts {
				return nil, err
			}
			if waitErr := sleep(ctx, retryDelayForAttempt(attempt, randInt63)); waitErr != nil {
				return nil, waitErr
			}
			continue
		}

		if !isRetryableStatus(resp.StatusCode) || attempt == maxRetryAttempts {
			return resp, nil
		}

		_ = resp.Body.Close()
		if waitErr := sleep(ctx, retryDelayForAttempt(attempt, randInt63)); waitErr != nil {
			return nil, waitErr
		}
	}
}

func (g *GitHubChecker) doRequestWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	return doRequestWithRetryHelper(ctx, g.client, req, g.sleep, g.randInt63)
}

// validateDownloadURL ensures the URL is a valid GitHub releases URL.
func validateDownloadURL(downloadURL string) error {
	parsed, err := url.Parse(downloadURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Scheme != "https" {
		return fmt.Errorf("URL must use HTTPS, got %s", parsed.Scheme)
	}

	if parsed.Host != allowedDownloadHost {
		return fmt.Errorf("URL must be from %s, got %s", allowedDownloadHost, parsed.Host)
	}

	// Verify it's a releases download path: /owner/repo/releases/...
	if !strings.Contains(parsed.Path, "/releases/") {
		return fmt.Errorf("URL must be a GitHub releases URL")
	}

	return nil
}

// fetchChecksums downloads the checksums.txt file and returns a map of filename -> sha256.
func (g *GitHubDownloader) fetchChecksums(ctx context.Context) (map[string]string, error) {
	log := logging.FromContext(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumsURLTemplate, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create checksums request: %w", err)
	}
	req.Header.Set("User-Agent", "dumber-browser")

	resp, err := g.doRequestWithRetry(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch checksums: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("checksums fetch failed with status %d", resp.StatusCode)
	}

	checksums := make(map[string]string)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Format: "sha256  filename" (two spaces between)
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			hash := parts[0]
			filename := parts[len(parts)-1] // Last field is filename
			checksums[filename] = hash
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse checksums: %w", err)
	}

	log.Debug().Int("count", len(checksums)).Msg("fetched checksums")
	return checksums, nil
}

// verifyChecksum verifies the SHA256 checksum of a file.
func verifyChecksum(filePath, expectedHash string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file for checksum: %w", err)
	}
	defer func() { _ = file.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("failed to compute checksum: %w", err)
	}

	actualHash := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actualHash, expectedHash) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	return nil
}

// getExpectedChecksum fetches checksums and returns the hash for the given URL.
func (g *GitHubDownloader) getExpectedChecksum(ctx context.Context, downloadURL string) (string, error) {
	checksums, err := g.fetchChecksums(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to fetch checksums: %w", err)
	}

	parsedURL, err := url.Parse(downloadURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse download URL: %w", err)
	}

	archiveFilename := filepath.Base(parsedURL.Path)
	expectedHash, ok := checksums[archiveFilename]
	if !ok {
		return "", fmt.Errorf("no checksum found for %s", archiveFilename)
	}

	return expectedHash, nil
}

// Download fetches the update archive from the given URL.
func (g *GitHubDownloader) Download(ctx context.Context, downloadURL, destDir string) (string, error) {
	log := logging.FromContext(ctx)

	// Validate the download URL is from GitHub releases.
	if err := validateDownloadURL(downloadURL); err != nil {
		return "", fmt.Errorf("invalid download URL: %w", err)
	}

	// Ensure destination directory exists.
	if err := os.MkdirAll(destDir, execPerm); err != nil {
		return "", fmt.Errorf("failed to create download directory: %w", err)
	}

	// Fetch and look up expected checksum for this archive.
	expectedHash, err := g.getExpectedChecksum(ctx, downloadURL)
	if err != nil {
		return "", err
	}

	// Create the request.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("failed to create download request: %w", err)
	}
	req.Header.Set("User-Agent", "dumber-browser")

	log.Debug().Str("url", downloadURL).Msg("downloading update")

	resp, err := g.doRequestWithRetry(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Check Content-Length if available to enforce size limit.
	if resp.ContentLength > maxArchiveSize {
		return "", fmt.Errorf("archive too large: %d bytes (max %d)", resp.ContentLength, maxArchiveSize)
	}

	archivePath := filepath.Join(destDir, "dumber_update.tar.gz")

	// Create the file.
	file, err := os.Create(archivePath)
	if err != nil {
		return "", fmt.Errorf("failed to create archive file: %w", err)
	}

	// Use LimitReader to prevent unbounded downloads even if Content-Length is missing/spoofed.
	limitedReader := io.LimitReader(resp.Body, maxArchiveSize+1)
	written, err := io.Copy(file, limitedReader)
	if closeErr := file.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(archivePath)
		return "", fmt.Errorf("failed to write archive: %w", err)
	}

	// Check if we hit the limit (means file was too large).
	if written > maxArchiveSize {
		_ = os.Remove(archivePath)
		return "", fmt.Errorf("archive exceeds maximum size of %d bytes", maxArchiveSize)
	}

	// Verify checksum before returning.
	if err := verifyChecksum(archivePath, expectedHash); err != nil {
		_ = os.Remove(archivePath)
		return "", fmt.Errorf("checksum verification failed: %w", err)
	}

	// Safely truncate hash for logging (SHA256 is 64 chars, but be defensive).
	displayHash := expectedHash
	if len(displayHash) > 16 {
		displayHash = displayHash[:16] + "..."
	}

	log.Debug().
		Int64("bytes", written).
		Str("path", archivePath).
		Str("checksum", displayHash).
		Msg("download completed and verified")

	return archivePath, nil
}

func (g *GitHubDownloader) doRequestWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	return doRequestWithRetryHelper(ctx, g.client, req, g.sleep, g.randInt63)
}

// sanitizeTarPath validates and sanitizes a tar header name to prevent path traversal.
func sanitizeTarPath(name, destDir string) (string, error) {
	// Clean the path to remove any . or .. components.
	cleaned := filepath.Clean(name)

	// Reject absolute paths.
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("absolute path not allowed: %s", name)
	}

	// Reject paths that contain ".." as a path component (not as part of filename).
	// Split by separator and check each component.
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if part == ".." {
			return "", fmt.Errorf("path traversal detected: %s", name)
		}
	}

	// Join with destination and verify result is within destDir.
	fullPath := filepath.Join(destDir, cleaned)
	absDestDir, err := filepath.Abs(destDir)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute dest path: %w", err)
	}
	absFullPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute full path: %w", err)
	}

	// Ensure the resolved path is within destDir.
	if !strings.HasPrefix(absFullPath, absDestDir+string(filepath.Separator)) && absFullPath != absDestDir {
		return "", fmt.Errorf("path escapes destination directory: %s", name)
	}

	return fullPath, nil
}

// Extract extracts the dumber binary from the tar.gz archive.
func (*GitHubDownloader) Extract(ctx context.Context, archivePath, destDir string) (string, error) {
	log := logging.FromContext(ctx)

	file, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("failed to open archive: %w", err)
	}
	defer func() { _ = file.Close() }()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)

	var binaryPath string

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read tar: %w", err)
		}

		// Look for the dumber binary (inside dumber_VERSION/ directory).
		if header.Typeflag == tar.TypeReg && strings.HasSuffix(header.Name, "/dumber") {
			// Sanitize the path to prevent path traversal attacks.
			_, err := sanitizeTarPath(header.Name, destDir)
			if err != nil {
				return "", fmt.Errorf("invalid tar entry: %w", err)
			}

			// We always extract to a fixed name regardless of archive path.
			binaryPath = filepath.Join(destDir, "dumber")

			outFile, err := os.OpenFile(binaryPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, execPerm)
			if err != nil {
				return "", fmt.Errorf("failed to create binary file: %w", err)
			}

			// Limit copy to prevent decompression bombs (G110).
			written, err := io.CopyN(outFile, tr, maxBinarySize)
			if closeErr := outFile.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
			if err != nil && !errors.Is(err, io.EOF) {
				_ = os.Remove(binaryPath)
				return "", fmt.Errorf("failed to extract binary: %w", err)
			}

			// Verify minimum binary size to detect truncated/corrupt files.
			if written < minBinarySize {
				_ = os.Remove(binaryPath)
				return "", fmt.Errorf("binary too small (%d bytes), expected at least %d bytes", written, minBinarySize)
			}

			log.Debug().Int64("bytes", written).Str("path", binaryPath).Msg("extracted binary")
			break
		}
	}

	if binaryPath == "" {
		return "", fmt.Errorf("dumber binary not found in archive")
	}

	return binaryPath, nil
}
