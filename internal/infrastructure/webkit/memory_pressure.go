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

	if cfg.ConservativeThreshold > 0 && cfg.StrictThreshold <= 0 {
		log.Warn().Msg("conservative threshold set without strict threshold; skipping conservative to avoid WebKit assertion")
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

	if cfg.ConservativeThreshold > 0 && cfg.StrictThreshold <= 0 {
		log.Warn().Msg("conservative threshold set without strict threshold; skipping conservative to avoid WebKit assertion")
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
	// IMPORTANT: WebKit asserts that conservative < strict, so set strict first.
	strict := cfg.StrictThreshold
	conservative := cfg.ConservativeThreshold

	if strict > 0 && strict < 1 {
		settings.SetStrictThreshold(strict)
	}

	// Only set conservative when we also have a strict threshold; otherwise
	// WebKit's internal default strict value may not be initialized yet.
	if conservative > 0 && conservative < 1 && strict > 0 {
		if conservative < strict {
			settings.SetConservativeThreshold(conservative)
		}
	}

	// Kill threshold: -1 = unset, 0 = never kill, >0 = threshold
	if cfg.KillThreshold >= 0 {
		settings.SetKillThreshold(cfg.KillThreshold)
	}

	return settings
}

// Ensure MemoryPressureApplier implements the interface.
var _ port.MemoryPressureApplier = (*MemoryPressureApplier)(nil)
