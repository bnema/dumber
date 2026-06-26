package port

import "context"

// LocalPathResolver resolves user-provided local filesystem paths.
// Implementations may perform filesystem and environment I/O.
type LocalPathResolver interface {
	ResolveExistingPath(ctx context.Context, input string) (absPath string, ok bool, err error)
}
