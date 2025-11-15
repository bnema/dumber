// Package config provides XDG Base Directory specification compliance utilities.
package config

import (
	"os"
	"path/filepath"
)

const (
	appName      = "dumber"
	databaseName = "dumber.sqlite"
)

// XDGDirs holds the XDG Base Directory paths for the application.
type XDGDirs struct {
	ConfigHome string
	DataHome   string
	StateHome  string
}

// GetXDGDirs returns the XDG Base Directory paths for dumber.
// It follows the XDG Base Directory specification:
// - $XDG_CONFIG_HOME/dumber (default: ~/.config/dumber)
// - $XDG_DATA_HOME/dumber (default: ~/.local/share/dumber)
// - $XDG_STATE_HOME/dumber (default: ~/.local/state/dumber)
func GetXDGDirs() (*XDGDirs, error) {
	// Development mode: use .dev directory in current working directory
	if os.Getenv("ENV") == "dev" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		devDir := filepath.Join(cwd, ".dev", appName)
		return &XDGDirs{
			ConfigHome: devDir,
			DataHome:   devDir,
			StateHome:  devDir,
		}, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	// XDG_CONFIG_HOME
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(homeDir, ".config")
	}
	configHome = filepath.Join(configHome, appName)

	// XDG_DATA_HOME
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = filepath.Join(homeDir, ".local", "share")
	}
	dataHome = filepath.Join(dataHome, appName)

	// XDG_STATE_HOME
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(homeDir, ".local", "state")
	}
	stateHome = filepath.Join(stateHome, appName)

	return &XDGDirs{
		ConfigHome: configHome,
		DataHome:   dataHome,
		StateHome:  stateHome,
	}, nil
}

// GetConfigDir returns the XDG config directory for dumber.
func GetConfigDir() (string, error) {
	dirs, err := GetXDGDirs()
	if err != nil {
		return "", err
	}
	return dirs.ConfigHome, nil
}

// GetDataDir returns the XDG data directory for dumber.
func GetDataDir() (string, error) {
	dirs, err := GetXDGDirs()
	if err != nil {
		return "", err
	}
	return dirs.DataHome, nil
}

// GetStateDir returns the XDG state directory for dumber.
func GetStateDir() (string, error) {
	dirs, err := GetXDGDirs()
	if err != nil {
		return "", err
	}
	return dirs.StateHome, nil
}

// GetLogDir returns the XDG-compliant log directory for dumber.
// Logs are stored in XDG_STATE_HOME as per specification.
func GetLogDir() (string, error) {
	stateDir, err := GetStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(stateDir, "logs"), nil
}

// GetConfigFile returns the path to the main configuration file.
func GetConfigFile() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "config.toml"), nil
}

// GetDatabaseFile returns the path to the database file in the data directory.
// The database contains important user data (history, cookies, preferences) and
// therefore belongs in XDG_DATA_HOME, not XDG_STATE_HOME.
func GetDatabaseFile() (string, error) {
	dataDir, err := GetDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, databaseName), nil
}

// GetFilterCacheDir returns the XDG-compliant filter cache directory for dumber.
// Filter cache is stored in XDG_STATE_HOME as it's transient data that can be regenerated.
func GetFilterCacheDir() (string, error) {
	stateDir, err := GetStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(stateDir, "filter-cache"), nil
}

// GetFilterCacheFile returns the path to the main filter cache file.
func GetFilterCacheFile() (string, error) {
	cacheDir, err := GetFilterCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "filters.cache"), nil
}

// EnsureDirectories creates the XDG directories if they don't exist.
func EnsureDirectories() error {
	dirs, err := GetXDGDirs()
	if err != nil {
		return err
	}

	// Create all directories with proper permissions
	directories := []string{
		dirs.ConfigHome,
		dirs.DataHome,
		dirs.StateHome,
	}

	for _, dir := range directories {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return err
		}
	}

	return nil
}
