package filtering

import "errors"

var (
	// ErrFiltersNotReady indicates that filters are not yet compiled/loaded
	ErrFiltersNotReady = errors.New("filters not ready")

	// ErrInvalidFilterFormat indicates malformed filter data
	ErrInvalidFilterFormat = errors.New("invalid filter format")

	// ErrCacheCorrupted indicates corrupted cache data
	ErrCacheCorrupted = errors.New("cache corrupted")

	// ErrNetworkError indicates network-related errors during filter download
	ErrNetworkError = errors.New("network error")
)
