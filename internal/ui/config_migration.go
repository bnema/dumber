package ui

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
)

// checkConfigMigration checks if user config needs migration and shows a toast.
func (a *App) checkConfigMigration(ctx context.Context) {
	log := logging.FromContext(ctx)
	cfg := config.Get()

	migrator := config.NewMigrator()
	result, err := migrator.CheckMigration()
	if err != nil {
		log.Debug().Err(err).Msg("config migration check failed")
		return
	}

	if result == nil || len(result.MissingKeys) == 0 {
		return // No migration needed
	}

	// Log is always shown for debugging purposes
	log.Info().
		Int("missing_keys", len(result.MissingKeys)).
		Msg("config migration available")

	// Show toast notification if enabled
	if cfg.Update.NotifyOnNewSettings && a.appToaster != nil {
		msg := fmt.Sprintf("Config has %d new settings. Run 'dumber config migrate'", len(result.MissingKeys))
		a.appToaster.Show(ctx, msg, component.ToastInfo)
	}
}
