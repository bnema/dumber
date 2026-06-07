package port

import (
	"context"

	"github.com/bnema/dumber/internal/domain/favicon"
)

type FaviconBlobStore interface {
	ReadOriginal(ctx context.Context, key favicon.Key) ([]byte, string, error)
	WriteOriginal(ctx context.Context, key favicon.Key, data []byte, contentType string) error
	ReadPNG(ctx context.Context, key favicon.Key) ([]byte, string, error)
	WritePNG(ctx context.Context, key favicon.Key, data []byte) error
	ReadSizedPNG(ctx context.Context, key favicon.Key, size int) ([]byte, string, error)
	WriteSizedPNG(ctx context.Context, key favicon.Key, size int, data []byte) error
	RemoveDerived(ctx context.Context, key favicon.Key) error
}
