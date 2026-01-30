package repository

import (
	"context"

	"github.com/bnema/dumber/internal/domain/entity"
)

// PermissionRepository defines operations for permission persistence.
// Only microphone and camera permissions are persisted per W3C spec.
type PermissionRepository interface {
	// Get retrieves the permission record for a specific origin and permission type.
	// Returns nil if no record exists (treat as "prompt" state).
	Get(ctx context.Context, origin string, permType entity.PermissionType) (*entity.PermissionRecord, error)

	// Set saves or updates a permission record.
	Set(ctx context.Context, record *entity.PermissionRecord) error

	// Delete removes a permission record for a specific origin and type.
	Delete(ctx context.Context, origin string, permType entity.PermissionType) error

	// GetAll retrieves all permission records for an origin.
	GetAll(ctx context.Context, origin string) ([]*entity.PermissionRecord, error)
}
