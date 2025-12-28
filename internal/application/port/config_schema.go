package port

import "github.com/bnema/dumber/internal/domain/entity"

// ConfigSchemaProvider provides configuration schema information.
type ConfigSchemaProvider interface {
	// GetSchema returns all configuration keys with their metadata.
	GetSchema() []entity.ConfigKeyInfo
}
