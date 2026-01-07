package port

// XDGPaths provides XDG Base Directory paths.
type XDGPaths interface {
	ConfigDir() (string, error)
	DataDir() (string, error)
	StateDir() (string, error)
	CacheDir() (string, error)

	FilterJSONDir() (string, error)
	FilterStoreDir() (string, error)
	FilterCacheDir() (string, error)

	// ManDir returns the user man page directory (man1 section).
	// Typically $XDG_DATA_HOME/man/man1 or ~/.local/share/man/man1.
	ManDir() (string, error)

	// DownloadDir returns the user's download directory.
	// Typically $XDG_DOWNLOAD_DIR or ~/Downloads.
	DownloadDir() (string, error)
}
