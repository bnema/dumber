package webext

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	uBlockExtensionID = "ublock-origin"
	uBlockRepo        = "gorhill/uBlock"
)

// GitHubRelease represents a GitHub release
type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// EnsureUBlockOrigin checks if uBlock Origin is installed, and downloads it if not
func (m *Manager) EnsureUBlockOrigin() error {
	// Check if uBlock is already installed in bundled extensions
	m.mu.RLock()
	if _, exists := m.bundled[uBlockExtensionID]; exists {
		m.mu.RUnlock()
		log.Printf("[webext] uBlock Origin already installed")
		return nil
	}
	m.mu.RUnlock()

	log.Printf("[webext] uBlock Origin not found, downloading latest version...")

	// Get latest release info
	release, err := getLatestUBlockRelease()
	if err != nil {
		return fmt.Errorf("failed to get latest uBlock release: %w", err)
	}

	log.Printf("[webext] Latest uBlock Origin version: %s", release.TagName)

	// Find the chromium zip asset
	var downloadURL string
	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, "chromium.zip") {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("chromium build not found in release %s", release.TagName)
	}

	// Download and extract
	installPath := filepath.Join(m.dataDir, "..", "bundled", uBlockExtensionID)
	if err := downloadAndExtractUBlock(downloadURL, installPath); err != nil {
		return fmt.Errorf("failed to download uBlock: %w", err)
	}

	log.Printf("[webext] uBlock Origin %s installed to %s", release.TagName, installPath)

	// Load the extension
	ext, err := m.loadExtension(installPath, true)
	if err != nil {
		return fmt.Errorf("failed to load uBlock extension: %w", err)
	}

	// Add to bundled extensions and enable
	m.mu.Lock()
	m.bundled[uBlockExtensionID] = ext
	m.enabled[uBlockExtensionID] = true
	m.mu.Unlock()

	log.Printf("[webext] uBlock Origin enabled")

	return nil
}

// getLatestUBlockRelease fetches the latest release info from GitHub
func getLatestUBlockRelease() (*GitHubRelease, error) {
	// Try using gh CLI first (faster and respects rate limits better)
	if ghRelease, err := getLatestReleaseViaGH(); err == nil {
		return ghRelease, nil
	}

	// Fallback to direct API call
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", uBlockRepo)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

// getLatestReleaseViaGH uses gh CLI to fetch release info
func getLatestReleaseViaGH() (*GitHubRelease, error) {
	cmd := exec.Command("gh", "api", fmt.Sprintf("/repos/%s/releases/latest", uBlockRepo))

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var release GitHubRelease
	if err := json.Unmarshal(output, &release); err != nil {
		return nil, err
	}

	return &release, nil
}

// downloadAndExtractUBlock downloads and extracts the uBlock zip file
func downloadAndExtractUBlock(url, installPath string) error {
	// Create temporary file for download
	tmpFile, err := os.CreateTemp("", "ublock-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Download the zip file
	log.Printf("[webext] Downloading from %s...", url)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Write to temp file
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return err
	}
	tmpFile.Close()

	// Create install directory
	if err := os.MkdirAll(installPath, 0755); err != nil {
		return err
	}

	// Extract to temporary directory first
	tmpExtractDir, err := os.MkdirTemp("", "ublock-extract-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpExtractDir)

	// Extract using unzip command
	cmd := exec.Command("unzip", "-q", "-o", tmpFile.Name(), "-d", tmpExtractDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract zip: %w", err)
	}

	// Find the uBlock0.chromium subdirectory
	uBlockDir := filepath.Join(tmpExtractDir, "uBlock0.chromium")
	if _, err := os.Stat(uBlockDir); os.IsNotExist(err) {
		return fmt.Errorf("uBlock0.chromium directory not found in zip")
	}

	// Move the uBlock0.chromium contents to the install path
	if err := os.RemoveAll(installPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(uBlockDir, installPath); err != nil {
		// Fall back to copy when rename crosses filesystems
		if !errors.Is(err, syscall.EXDEV) {
			return fmt.Errorf("failed to move extension: %w", err)
		}

		if err := copyDir(uBlockDir, installPath); err != nil {
			return fmt.Errorf("failed to move extension (copy fallback): %w", err)
		}

		if err := os.RemoveAll(uBlockDir); err != nil {
			log.Printf("[webext] Warning: failed to cleanup temp uBlock directory: %v", err)
		}
	}

	log.Printf("[webext] Extracted to %s", installPath)
	return nil
}

// copyDir copies a directory tree from src to dst.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}

		if d.Type()&os.ModeSymlink != 0 {
			// Preserve symlinks by copying the link target.
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, target)
		}

		return copyFile(path, target)
	})
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Sync()
}
