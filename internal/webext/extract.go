package webext

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/config"
)

// EnsureWebExtSO extracts the embedded WebProcess extension .so to user's libexec directory
// Returns the directory path where the .so was extracted
func EnsureWebExtSO(assets embed.FS) (string, error) {
	// Read the embedded .so file from assets
	soData, err := assets.ReadFile("assets/webext/dumber-webext.so")
	if err != nil {
		return "", fmt.Errorf("failed to read embedded .so: %w", err)
	}

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

	// Check if .so already exists and is valid
	if info, err := os.Stat(soPath); err == nil && info.Size() == int64(len(soData)) {
		// File exists with correct size, assume it's valid
		log.Printf("[webext] Using existing WebProcess extension: %s", soPath)
		return libexecDir, nil
	}

	// Extract embedded .so
	log.Printf("[webext] Extracting WebProcess extension to %s (%d bytes)", soPath, len(soData))
	if err := os.WriteFile(soPath, soData, 0755); err != nil {
		return "", fmt.Errorf("failed to write .so file: %w", err)
	}

	log.Printf("[webext] WebProcess extension extracted successfully")
	return libexecDir, nil
}
