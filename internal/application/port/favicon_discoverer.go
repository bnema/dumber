package port

import "context"

type FaviconDiscovery struct {
	PageURL  string
	IconURLs []string
}

type FaviconDiscoverer interface {
	Discover(ctx context.Context, pageURL string) (*FaviconDiscovery, error)
}
