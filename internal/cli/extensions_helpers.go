package cli

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/webext"
)

// buildExtensionManager constructs a web extension manager for CLI operations.
func buildExtensionManager(cli *CLI) (*webext.Manager, error) {
	dataDir, err := config.GetDataDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get data dir: %w", err)
	}

	extensionsDir := filepath.Join(dataDir, "extensions")
	extDataDir := filepath.Join(dataDir, "extension-data")

	if err := os.MkdirAll(extensionsDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create extensions directory: %w", err)
	}
	if err := os.MkdirAll(extDataDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create extension data directory: %w", err)
	}

	manager := webext.NewManager(extensionsDir, extDataDir, cli.DB, cli.Queries)

	if err := manager.LoadExtensionsFromDB(); err != nil {
		log.Printf("[cli] Warning: failed to load extensions from database: %v", err)
	}
	if err := manager.LoadExtensions(extensionsDir); err != nil {
		log.Printf("[cli] Warning: failed to load extensions from disk: %v", err)
	}

	return manager, nil
}
