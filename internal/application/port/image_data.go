package port

import "context"

// ImageData contains resolved image bytes.
type ImageData struct {
	Bytes []byte
}

// ImageDataResolver resolves image bytes from a URI.
type ImageDataResolver interface {
	ResolveImageData(ctx context.Context, uri string) (ImageData, error)
}

// ResolvedImageSaver persists already resolved image bytes.
type ResolvedImageSaver interface {
	SaveResolvedImage(ctx context.Context, image ImageData) error
}
