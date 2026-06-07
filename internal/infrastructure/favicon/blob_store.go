package favicon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	appport "github.com/bnema/dumber/internal/application/port"
	domainfavicon "github.com/bnema/dumber/internal/domain/favicon"
)

const contentTypeSuffix = ".content-type"

// BlobStore stores favicon originals and derived PNG files on disk.
type BlobStore struct {
	cache *Cache
}

func NewBlobStore(diskDir string) *BlobStore        { return &BlobStore{cache: NewCache(diskDir)} }
func NewBlobStoreFromCache(cache *Cache) *BlobStore { return &BlobStore{cache: cache} }

func (s *BlobStore) ReadOriginal(ctx context.Context, key domainfavicon.Key) ([]byte, string, error) {
	_ = ctx
	path := s.cache.DiskPath(string(key))
	data, err := readNonEmpty(path)
	if err != nil {
		return nil, "", err
	}
	ct, err := os.ReadFile(path + contentTypeSuffix)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, "", err
	}
	return data, strings.TrimSpace(string(ct)), nil
}

func (s *BlobStore) WriteOriginal(ctx context.Context, key domainfavicon.Key, data []byte, contentType string) error {
	_ = ctx
	path := s.cache.DiskPath(string(key))
	if err := atomicWrite(path, data); err != nil {
		return err
	}
	if contentType != "" {
		return atomicWrite(path+contentTypeSuffix, []byte(contentType))
	}
	_ = os.Remove(path + contentTypeSuffix)
	return nil
}

func (s *BlobStore) ReadPNG(ctx context.Context, key domainfavicon.Key) ([]byte, string, error) {
	_ = ctx
	data, err := readNonEmpty(s.cache.DiskPathPNG(string(key)))
	if err != nil {
		return nil, "", err
	}
	return data, "image/png", nil
}
func (s *BlobStore) WritePNG(ctx context.Context, key domainfavicon.Key, data []byte) error {
	_ = ctx
	return atomicWrite(s.cache.DiskPathPNG(string(key)), data)
}
func (s *BlobStore) ReadSizedPNG(ctx context.Context, key domainfavicon.Key, size int) ([]byte, string, error) {
	_ = ctx
	data, err := readNonEmpty(s.cache.DiskPathPNGSized(string(key), size))
	if err != nil {
		return nil, "", err
	}
	return data, "image/png", nil
}
func (s *BlobStore) WriteSizedPNG(ctx context.Context, key domainfavicon.Key, size int, data []byte) error {
	_ = ctx
	return atomicWrite(s.cache.DiskPathPNGSized(string(key), size), data)
}

func (s *BlobStore) RemoveDerived(ctx context.Context, key domainfavicon.Key) error {
	_ = ctx
	if s.cache == nil || s.cache.diskDir == "" {
		return nil
	}
	pngPath := s.cache.DiskPathPNG(string(key))
	paths := []string{pngPath}
	var errs []error
	basePrefix := strings.TrimSuffix(filepath.Base(pngPath), ".png") + "."
	pattern := filepath.Join(s.cache.diskDir, basePrefix+"*.png")
	matches, globErr := filepath.Glob(pattern)
	if globErr != nil {
		errs = append(errs, globErr)
	}
	for _, match := range matches {
		if isSizedPNGForKey(filepath.Base(match), basePrefix) {
			paths = append(paths, match)
		}
	}
	for _, p := range paths {
		if p != "" {
			if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func isSizedPNGForKey(baseName, basePrefix string) bool {
	if !strings.HasPrefix(baseName, basePrefix) || !strings.HasSuffix(baseName, ".png") {
		return false
	}
	sizePart := strings.TrimSuffix(strings.TrimPrefix(baseName, basePrefix), ".png")
	if sizePart == "" {
		return false
	}
	for _, r := range sizePart {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func readNonEmpty(path string) ([]byte, error) {
	if path == "" {
		return nil, appport.ErrFaviconMiss
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, appport.ErrFaviconMiss
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, appport.ErrFaviconMiss
	}
	return data, nil
}

func atomicWrite(path string, data []byte) error {
	if path == "" || len(data) == 0 {
		return appport.ErrFaviconInvalidInput
	}
	if err := os.MkdirAll(filepath.Dir(path), diskCacheDirPerm); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(diskCacheFilePerm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("atomic write %s: %w", path, err)
	}
	return nil
}
