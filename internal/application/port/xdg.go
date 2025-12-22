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
}
