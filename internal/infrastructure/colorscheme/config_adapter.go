package colorscheme

import (
	"github.com/bnema/dumber/internal/infrastructure/config"
)

// ConfigAdapter adapts config.Config to the ConfigProvider interface.
type ConfigAdapter struct {
	cfg *config.Config
}

// NewConfigAdapter creates a new config adapter.
func NewConfigAdapter(cfg *config.Config) *ConfigAdapter {
	return &ConfigAdapter{cfg: cfg}
}

// GetColorScheme implements ConfigProvider.
func (a *ConfigAdapter) GetColorScheme() string {
	if a.cfg == nil {
		return ""
	}
	return a.cfg.Appearance.ColorScheme
}
