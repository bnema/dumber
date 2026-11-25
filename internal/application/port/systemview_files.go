package port

import "context"

// SystemViewUserStyle reads the user stylesheet applied to system view pages.
type SystemViewUserStyle interface {
	// Path returns the user stylesheet path.
	Path() string
	// Read returns the stylesheet content. A missing file yields "".
	Read() (string, error)
}

// SystemViewFileWatcher watches the user files that customize system views
// and calls onUserCSSChange from a background goroutine when user.css changes.
type SystemViewFileWatcher interface {
	Start(ctx context.Context, onUserCSSChange func()) error
	Stop() error
}
