package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite/sqlc"
	"github.com/bnema/dumber/internal/logging"
)

type permissionRepo struct {
	queries *sqlc.Queries
}

// NewPermissionRepository creates a new SQLite-backed permission repository.
func NewPermissionRepository(db *sql.DB) repository.PermissionRepository {
	return &permissionRepo{queries: sqlc.New(db)}
}

func (r *permissionRepo) Get(ctx context.Context, origin string, permType entity.PermissionType) (*entity.PermissionRecord, error) {
	log := logging.FromContext(ctx)
	log.Debug().Str("origin", origin).Str("type", string(permType)).Msg("getting permission")

	row, err := r.queries.GetPermission(ctx, sqlc.GetPermissionParams{
		Origin:         origin,
		PermissionType: string(permType),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return permissionFromRow(row), nil
}

func (r *permissionRepo) Set(ctx context.Context, record *entity.PermissionRecord) error {
	log := logging.FromContext(ctx)

	if record == nil {
		log.Error().Msg("cannot set nil permission record")
		return errors.New("cannot set nil permission record")
	}

	log.Debug().
		Str("origin", record.Origin).
		Str("type", string(record.Type)).
		Str("decision", string(record.Decision)).
		Msg("setting permission")

	return r.queries.SetPermission(ctx, sqlc.SetPermissionParams{
		Origin:         record.Origin,
		PermissionType: string(record.Type),
		Decision:       string(record.Decision),
	})
}

func (r *permissionRepo) Delete(ctx context.Context, origin string, permType entity.PermissionType) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("origin", origin).Str("type", string(permType)).Msg("deleting permission")

	return r.queries.DeletePermission(ctx, sqlc.DeletePermissionParams{
		Origin:         origin,
		PermissionType: string(permType),
	})
}

func (r *permissionRepo) GetAll(ctx context.Context, origin string) ([]*entity.PermissionRecord, error) {
	log := logging.FromContext(ctx)
	log.Debug().Str("origin", origin).Msg("getting all permissions for origin")

	rows, err := r.queries.ListPermissionsByOrigin(ctx, origin)
	if err != nil {
		return nil, err
	}

	records := make([]*entity.PermissionRecord, len(rows))
	for i, row := range rows {
		records[i] = permissionFromRow(row)
	}
	return records, nil
}

func permissionFromRow(row sqlc.Permission) *entity.PermissionRecord {
	record := &entity.PermissionRecord{
		Origin:   row.Origin,
		Type:     entity.PermissionType(row.PermissionType),
		Decision: entity.PermissionDecision(row.Decision),
	}
	if row.UpdatedAt.Valid {
		record.UpdatedAt = row.UpdatedAt.Time.Unix()
	}
	return record
}
