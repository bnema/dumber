package webkit

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
)

// MemoryPressureApplier implements port.MemoryPressureApplier for WebKitGTK.
type MemoryPressureApplier struct{}

// NewMemoryPressureApplier creates a new MemoryPressureApplier.
func NewMemoryPressureApplier() *MemoryPressureApplier {
	return &MemoryPressureApplier{}
}

// ApplyNetworkProcessSettings applies memory pressure settings to the network process.
// Must be called BEFORE creating any NetworkSession.
func (*MemoryPressureApplier) ApplyNetworkProcessSettings(ctx context.Context, cfg *port.MemoryPressureConfig) error {
	log := logging.FromContext(ctx)

	if cfg == nil || !cfg.IsConfigured() {
		log.Debug().Msg("no network process memory pressure settings configured")
		return nil
	}

	settings := buildMemoryPressureSettings(cfg)
	if settings == nil {
		return nil
	}

	webkit.NetworkSessionSetMemoryPressureSettings(settings)
	log.Info().
		Int("limit_mb", cfg.MemoryLimitMB).
		Float64("poll_sec", cfg.PollIntervalSec).
		Float64("conservative", cfg.ConservativeThreshold).
		Float64("strict", cfg.StrictThreshold).
		Float64("kill", cfg.KillThreshold).
		Msg("applied network process memory pressure settings")

	return nil
}

// ApplyWebProcessSettings applies memory pressure settings to web processes.
// Returns an opaque settings object that should be passed to WebContext creation.
// Returns nil if no settings are configured.
func (*MemoryPressureApplier) ApplyWebProcessSettings(ctx context.Context, cfg *port.MemoryPressureConfig) (any, error) {
	log := logging.FromContext(ctx)

	if cfg == nil || !cfg.IsConfigured() {
		log.Debug().Msg("no web process memory pressure settings configured")
		return nil, nil
	}

	settings := buildMemoryPressureSettings(cfg)
	if settings == nil {
		return nil, nil
	}

	log.Info().
		Int("limit_mb", cfg.MemoryLimitMB).
		Float64("poll_sec", cfg.PollIntervalSec).
		Float64("conservative", cfg.ConservativeThreshold).
		Float64("strict", cfg.StrictThreshold).
		Float64("kill", cfg.KillThreshold).
		Msg("prepared web process memory pressure settings")

	return settings, nil
}

// buildMemoryPressureSettings creates WebKitMemoryPressureSettings from config.
// Returns nil if no settings are configured.
func buildMemoryPressureSettings(cfg *port.MemoryPressureConfig) *webkit.MemoryPressureSettings {
	if cfg == nil || !cfg.IsConfigured() {
		return nil
	}

	settings := webkit.NewMemoryPressureSettings()

	if cfg.MemoryLimitMB > 0 {
		settings.SetMemoryLimit(uint(cfg.MemoryLimitMB))
	}

	if cfg.PollIntervalSec > 0 {
		settings.SetPollInterval(cfg.PollIntervalSec)
	}

	// Thresholds must be in (0, 1)
	if cfg.ConservativeThreshold > 0 && cfg.ConservativeThreshold < 1 {
		settings.SetConservativeThreshold(cfg.ConservativeThreshold)
	}

	if cfg.StrictThreshold > 0 && cfg.StrictThreshold < 1 {
		settings.SetStrictThreshold(cfg.StrictThreshold)
	}

	// Kill threshold: -1 = unset, 0 = never kill, >0 = threshold
	if cfg.KillThreshold >= 0 {
		settings.SetKillThreshold(cfg.KillThreshold)
	}

	return settings
}

// Ensure MemoryPressureApplier implements the interface.
var _ port.MemoryPressureApplier = (*MemoryPressureApplier)(nil)
