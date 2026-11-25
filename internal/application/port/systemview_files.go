package port

import "context"

// SystemViewUserStyle reads the user stylesheet applied to system view pages.
type SystemViewUserStyle interface {
	// Path returns the user stylesheet path.
	Path() string
	// Read returns the stylesheet content. A missing file yields "".
	Read() (string, error)
}

// SystemViewFileHandlers are the callbacks for system view user files. They
// run on the watcher goroutine. A nil handler ignores that file.
type SystemViewFileHandlers struct {
	OnUserCSS    func()
	OnHomeConfig func()
}

// SystemViewFileWatcher watches the user files that customize system views.
type SystemViewFileWatcher interface {
	Start(ctx context.Context, handlers SystemViewFileHandlers) error
	Stop() error
}
