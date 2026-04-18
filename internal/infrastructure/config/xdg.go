// Package config provides configuration management for dumber with Viper integration.
package config

import (
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
)

// File permission constants
const (
	dirPerm  = 0755 // Standard directory permissions (rwxr-xr-x)
	filePerm = 0644 // Standard file permissions (rw-r--r--)
)

const (
	appName      = "dumber"
	databaseName = "dumber.db"
)

// XDGDirs holds the XDG Base Directory paths for the application.
type XDGDirs struct {
	ConfigHome string
	DataHome   string
	StateHome  string
	CacheHome  string
}

// GetXDGDirs returns the dumber-specific XDG directories.
// In prod they match the current XDG layout exactly.
// In dev they resolve into the shared sandbox under .dev/dumber/{config,data,state,cache}.
func GetXDGDirs() (*XDGDirs, error) {
	profile, err := resolveSharedProfile()
	if err != nil {
		return nil, err
	}
	return &XDGDirs{
		ConfigHome: profile.Shared.ConfigDir,
		DataHome:   profile.Shared.DataDir,
		StateHome:  profile.Shared.StateDir,
		CacheHome:  profile.Shared.CacheDir,
	}, nil
}

func resolveSharedProfile() (runtimeprofile.Profile, error) {
	base, err := getBaseXDGDirs()
	if err != nil {
		return runtimeprofile.Profile{}, err
	}
	return runtimeprofile.Resolve(runtimeprofile.ResolveInput{
		Env: os.Getenv,
		CWD: os.Getwd,
		Base: runtimeprofile.BasePaths{
			ConfigHome: base.ConfigHome,
			DataHome:   base.DataHome,
			StateHome:  base.StateHome,
			CacheHome:  base.CacheHome,
		},
	})
}

func getBaseXDGDirs() (*XDGDirs, error) {
	if os.Getenv("ENV") == "dev" {
		return &XDGDirs{}, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(homeDir, ".config")
	}
	configHome = filepath.Join(configHome, appName)

	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = filepath.Join(homeDir, ".local", "share")
	}
	dataHome = filepath.Join(dataHome, appName)

	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(homeDir, ".local", "state")
	}
	stateHome = filepath.Join(stateHome, appName)

	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		cacheHome = filepath.Join(homeDir, ".cache")
	}
	cacheHome = filepath.Join(cacheHome, appName)

	return &XDGDirs{
		ConfigHome: configHome,
		DataHome:   dataHome,
		StateHome:  stateHome,
		CacheHome:  cacheHome,
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

// GetLogDir returns the shared application log directory for dumber.
func GetLogDir() (string, error) {
	profile, err := resolveSharedProfile()
	if err != nil {
		return "", err
	}
	return profile.Shared.LogDir, nil
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
// The database contains user data (history, favorites, preferences) that should
// be backed up, so it belongs in XDG_DATA_HOME per the XDG specification.
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

// GetFaviconCacheDir returns the XDG-compliant favicon cache directory for dumber.
// Favicon cache is stored in XDG_CACHE_HOME as it's transient data that can be refetched.
func GetFaviconCacheDir() (string, error) {
	dirs, err := GetXDGDirs()
	if err != nil {
		return "", err
	}
	return filepath.Join(dirs.CacheHome, "favicons"), nil
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
		dirs.CacheHome,
	}

	for _, dir := range directories {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return err
		}
	}

	return nil
}
