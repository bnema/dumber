package port

import (
	"context"

	"github.com/bnema/dumber/internal/domain/entity"
)

// ExternalThemeSource provides access to an external theme provider (e.g., Noctalia).
// It must not import GTK, WebKit, CEF, lipgloss, fsnotify, Viper, or infrastructure config.
type ExternalThemeSource interface {
	// Get returns the current external theme, or nil if no external theme is available.
	// An error indicates a transient I/O problem (e.g., connection lost).
	// A nil theme with nil error means the source is enabled but has no theme data yet.
	Get(ctx context.Context) (*entity.ExternalTheme, error)

	// IsEnabled returns true if the external theme source is enabled.
	// When disabled, the usecase should fall back to config/default palettes.
	IsEnabled() bool
}

// ExternalThemeIdentityProvider exposes a stable source identity for last-good cache invalidation.
// Implementations should change this value when the configured provider/format/path changes.
type ExternalThemeIdentityProvider interface {
	ExternalThemeIdentity() string
}

// ConfigurableExternalThemeSource is an external source whose settings can be
// updated from a new config snapshot without replacing the ResolveThemeUseCase.
type ConfigurableExternalThemeSource interface {
	ExternalThemeSource
	ExternalThemeIdentityProvider
	Configure(entity.ExternalThemeConfig)
}

// ExternalThemeWatcher watches an external theme source and notifies callers
// when they should refresh the resolved theme through the shared apply path.
type ExternalThemeWatcher interface {
	Start(ctx context.Context, cfg entity.ExternalThemeConfig, onChange func()) error
	Stop() error
}
