package port

import (
	"context"

	"github.com/bnema/dumber/internal/domain/favicon"
)

// FaviconFetchRequest specifies the page and optional icon URL for a favicon fetch.
// PageURL is the absolute source page URL; IconURL may be empty when the fetcher should discover or fall back.
type FaviconFetchRequest struct {
	PageURL string
	IconURL string
}

// FaviconFetchedIcon contains fetched favicon bytes and their source metadata.
// PageURL is the source page, IconURL is the resource URL, ResolvedKey optionally
// overrides the storage key when a provider is host/domain scoped rather than exact-page scoped,
// Source identifies discovery origin, and ContentType is the MIME type.
type FaviconFetchedIcon struct {
	PageURL     string
	IconURL     string
	ResolvedKey favicon.Key
	Bytes       []byte
	Source      favicon.Source
	ContentType string
}

// FaviconFetcher safely fetches favicon resources for a FaviconFetchRequest.
// Implementations validate URLs/content and return ErrFaviconMiss for not-found or disallowed resources.
type FaviconFetcher interface {
	Fetch(ctx context.Context, request FaviconFetchRequest) (*FaviconFetchedIcon, error)
}
