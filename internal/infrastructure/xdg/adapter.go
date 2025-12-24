package xdg

import (
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

// Adapter implements port.XDGPaths using config.GetXDGDirs().
type Adapter struct{}

// New creates a new XDG paths adapter.
func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) ConfigDir() (string, error) {
	return config.GetConfigDir()
}

func (a *Adapter) DataDir() (string, error) {
	return config.GetDataDir()
}

func (a *Adapter) StateDir() (string, error) {
	return config.GetStateDir()
}

func (a *Adapter) CacheDir() (string, error) {
	dirs, err := config.GetXDGDirs()
	if err != nil {
		return "", err
	}
	return dirs.CacheHome, nil
}

func (a *Adapter) FilterJSONDir() (string, error) {
	dataDir, err := a.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "filters", "json"), nil
}

func (a *Adapter) FilterStoreDir() (string, error) {
	dataDir, err := a.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "filters", "store"), nil
}

func (a *Adapter) FilterCacheDir() (string, error) {
	return config.GetFilterCacheDir()
}

func (a *Adapter) ManDir() (string, error) {
	// Man pages go to the user's XDG_DATA_HOME/man/man1, not the dumber-specific dir.
	// This allows 'man dumber' to work without custom MANPATH configuration.
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}

	return filepath.Join(dataHome, "man", "man1"), nil
}

var _ port.XDGPaths = (*Adapter)(nil)
