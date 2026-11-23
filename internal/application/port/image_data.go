package port

import (
	"context"

	"bnema.dev/dumber/internal/domain/entity"
)

// ImageDataResolver resolves image bytes from a URI.
type ImageDataResolver interface {
	ResolveImageData(ctx context.Context, uri string) (entity.ImageData, error)
}

// ResolvedImageSaver persists already resolved image bytes.
type ResolvedImageSaver interface {
	SaveResolvedImage(ctx context.Context, image entity.ImageData, menuContext MenuContext) error
}
