package port

import "context"

// FilterState represents the current state of the content filter system.
type FilterState string

const (
	// FilterStateUninitialized means the filter manager has not been initialized yet.
	FilterStateUninitialized FilterState = "uninitialized"
	// FilterStateLoading means filters are being downloaded or compiled.
	FilterStateLoading FilterState = "loading"
	// FilterStateActive means filters are loaded and active.
	FilterStateActive FilterState = "active"
	// FilterStateDisabled means filtering is disabled by configuration.
	FilterStateDisabled FilterState = "disabled"
	// FilterStateError means an error occurred during filter loading.
	FilterStateError FilterState = "error"
)

// FilterStatus represents the current status of the content filter system.
// Used for UI toast notifications and status reporting.
type FilterStatus struct {
	State   FilterState
	Message string
	Version string
}

// FilterManager manages content filters.
type FilterManager interface {
	SetStatusCallback(fn func(FilterStatus))
	LoadAsync(ctx context.Context)
}
