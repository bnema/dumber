package port

import "context"

// ImageData contains resolved image bytes and MIME type metadata.
type ImageData struct {
	Bytes    []byte
	MimeType string
}

// ImageDataResolver resolves image bytes from a URI.
type ImageDataResolver interface {
	ResolveImageData(ctx context.Context, uri string) (ImageData, error)
}

// ResolvedImageSaver persists already resolved image bytes.
type ResolvedImageSaver interface {
	SaveResolvedImage(ctx context.Context, image ImageData, menuContext MenuContext) error
}
