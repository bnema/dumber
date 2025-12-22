package filesystem

import (
	"context"
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/application/port"
)

// Adapter implements port.FileSystem using the OS filesystem.
type Adapter struct{}

// New creates a new filesystem adapter.
func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Exists(_ context.Context, path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (a *Adapter) IsDirectory(_ context.Context, path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

func (a *Adapter) GetSize(_ context.Context, path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		return info.Size(), nil
	}

	var size int64
	err = filepath.Walk(path, func(_ string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !fi.IsDir() {
			size += fi.Size()
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return size, nil
}

func (a *Adapter) RemoveAll(_ context.Context, path string) error {
	return os.RemoveAll(path)
}

var _ port.FileSystem = (*Adapter)(nil)
