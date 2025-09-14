package filtering

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/logging"
)

// SetupFilterSystem creates and initializes the complete filtering system
func SetupFilterSystem() (*FilterManager, error) {
	// Create filter store
	store, err := NewFileFilterStore()
	if err != nil {
		return nil, fmt.Errorf("failed to create filter store: %w", err)
	}

	// Create filter compiler
	compiler := NewDefaultFilterCompiler()

	// Create filter manager with all components
	manager := NewFilterManager(store, compiler)

	logging.Info("Filter system initialized successfully")
	return manager, nil
}

// InitializeFiltersAsync initializes the filtering system asynchronously
func InitializeFiltersAsync(manager *FilterManager) error {
	ctx := context.Background()
	return manager.InitAsync(ctx)
}
