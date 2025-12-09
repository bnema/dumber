package repository

import "context"

// ContentWhitelistRepository defines operations for content filter whitelist persistence.
// Whitelisted domains bypass content filtering.
type ContentWhitelistRepository interface {
	// Add adds a domain to the whitelist.
	Add(ctx context.Context, domain string) error

	// Remove removes a domain from the whitelist.
	Remove(ctx context.Context, domain string) error

	// Contains checks if a domain is whitelisted.
	Contains(ctx context.Context, domain string) (bool, error)

	// GetAll retrieves all whitelisted domains.
	GetAll(ctx context.Context) ([]string, error)
}
