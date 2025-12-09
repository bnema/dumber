package sqlite

import (
	"context"
	"database/sql"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite/sqlc"
	"github.com/bnema/dumber/internal/logging"
)

type zoomRepo struct {
	queries *sqlc.Queries
}

// NewZoomRepository creates a new SQLite-backed zoom repository.
func NewZoomRepository(db *sql.DB) repository.ZoomRepository {
	return &zoomRepo{queries: sqlc.New(db)}
}

func (r *zoomRepo) Get(ctx context.Context, domain string) (*entity.ZoomLevel, error) {
	log := logging.FromContext(ctx)
	log.Debug().Str("domain", domain).Msg("getting zoom level")

	row, err := r.queries.GetZoomLevel(ctx, domain)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return zoomFromRow(row), nil
}

func (r *zoomRepo) Set(ctx context.Context, level *entity.ZoomLevel) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("domain", level.Domain).Float64("factor", level.ZoomFactor).Msg("setting zoom level")

	return r.queries.SetZoomLevel(ctx, sqlc.SetZoomLevelParams{
		Domain:     level.Domain,
		ZoomFactor: level.ZoomFactor,
	})
}

func (r *zoomRepo) Delete(ctx context.Context, domain string) error {
	return r.queries.DeleteZoomLevel(ctx, domain)
}

func (r *zoomRepo) GetAll(ctx context.Context) ([]*entity.ZoomLevel, error) {
	rows, err := r.queries.ListZoomLevels(ctx)
	if err != nil {
		return nil, err
	}

	levels := make([]*entity.ZoomLevel, len(rows))
	for i, row := range rows {
		levels[i] = zoomFromRow(row)
	}
	return levels, nil
}

func zoomFromRow(row sqlc.ZoomLevel) *entity.ZoomLevel {
	return &entity.ZoomLevel{
		Domain:     row.Domain,
		ZoomFactor: row.ZoomFactor,
		UpdatedAt:  row.UpdatedAt.Time,
	}
}
