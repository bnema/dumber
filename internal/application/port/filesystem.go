package port

import "context"

// FileSystem provides file system operations for the application layer.
type FileSystem interface {
	Exists(ctx context.Context, path string) (bool, error)
	IsDirectory(ctx context.Context, path string) (bool, error)
	GetSize(ctx context.Context, path string) (int64, error)
	RemoveAll(ctx context.Context, path string) error
}
