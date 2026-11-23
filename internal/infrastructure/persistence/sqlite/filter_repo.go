package sqlite

import (
	"context"
	"database/sql"

	"bnema.dev/dumber/internal/domain/repository"
	"bnema.dev/dumber/internal/infrastructure/persistence/sqlite/sqlc"
	"bnema.dev/dumber/internal/logging"
)

type contentWhitelistRepo struct {
	queries *sqlc.Queries
}

// NewContentWhitelistRepository creates a new SQLite-backed content whitelist repository.
func NewContentWhitelistRepository(db *sql.DB) repository.ContentWhitelistRepository {
	return &contentWhitelistRepo{queries: sqlc.New(db)}
}

func (r *contentWhitelistRepo) Add(ctx context.Context, domain string) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("domain", domain).Msg("adding domain to whitelist")

	return r.queries.AddToWhitelist(ctx, domain)
}

func (r *contentWhitelistRepo) Remove(ctx context.Context, domain string) error {
	return r.queries.RemoveFromWhitelist(ctx, domain)
}

func (r *contentWhitelistRepo) Contains(ctx context.Context, domain string) (bool, error) {
	count, err := r.queries.IsWhitelisted(ctx, domain)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *contentWhitelistRepo) GetAll(ctx context.Context) ([]string, error) {
	return r.queries.GetAllWhitelistedDomains(ctx)
}
