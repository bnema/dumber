package webext

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/config"
)

// EnsureWebExtSO extracts the embedded WebProcess extension .so to user's libexec directory
// Returns the directory path where the .so was extracted
func EnsureWebExtSO(assets fs.FS) (string, error) {
	// Read the embedded .so file from assets
	soData, err := fs.ReadFile(assets, "assets/webext/dumber-webext.so")
	if err != nil {
		return "", fmt.Errorf("failed to read embedded .so: %w", err)
	}
	embeddedHash := sha256.Sum256(soData)

	// Get user's data directory (~/.local/share/dumber)
	dataDir, err := config.GetDataDir()
	if err != nil {
		return "", fmt.Errorf("failed to get data directory: %w", err)
	}

	// Create libexec directory in ~/.local/libexec/dumber
	libexecDir := filepath.Join(filepath.Dir(dataDir), "libexec", "dumber")
	if err := os.MkdirAll(libexecDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create libexec directory: %w", err)
	}

	soPath := filepath.Join(libexecDir, "dumber-webext.so")

	// Check if .so already exists and hash matches embedded copy
	if fileHashMatches(soPath, embeddedHash) {
		log.Printf("[webext] Using existing WebProcess extension: %s", soPath)
		return libexecDir, nil
	}

	// Extract embedded .so
	log.Printf("[webext] Extracting WebProcess extension to %s (%d bytes)", soPath, len(soData))
	if err := writeAtomically(soPath, soData, 0o755); err != nil {
		return "", fmt.Errorf("failed to write .so file: %w", err)
	}

	log.Printf("[webext] WebProcess extension extracted successfully")
	return libexecDir, nil
}

// fileHashMatches returns true if a file exists and its SHA-256 matches the expected.
func fileHashMatches(path string, expected [32]byte) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}

	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum == expected
}

// writeAtomically writes data to a temp file and renames it into place to avoid partial writes.
func writeAtomically(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "tmp-webext-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}
