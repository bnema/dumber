package port

import "context"

// DesktopIntegrationStatus represents the current state of desktop integration.
type DesktopIntegrationStatus struct {
	DesktopFileInstalled bool
	DesktopFilePath      string
	IconInstalled        bool
	IconFilePath         string
	IsDefaultBrowser     bool
	ExecutablePath       string
}

// DesktopIntegration provides desktop environment integration operations.
type DesktopIntegration interface {
	// GetStatus checks the current desktop integration state.
	GetStatus(ctx context.Context) (*DesktopIntegrationStatus, error)

	// InstallDesktopFile writes the desktop file to XDG applications directory.
	// Returns the path where the file was installed.
	// Idempotent: safe to call multiple times.
	InstallDesktopFile(ctx context.Context) (string, error)

	// InstallIcon writes the icon file to XDG icons directory.
	// Returns the path where the icon was installed.
	// Idempotent: safe to call multiple times.
	InstallIcon(ctx context.Context, svgData []byte) (string, error)

	// RemoveDesktopFile removes the desktop file from XDG applications directory.
	// Idempotent: returns nil if file doesn't exist.
	RemoveDesktopFile(ctx context.Context) error

	// RemoveIcon removes the icon file from XDG icons directory.
	// Idempotent: returns nil if file doesn't exist.
	RemoveIcon(ctx context.Context) error

	// SetAsDefaultBrowser sets dumber as the default web browser using xdg-settings.
	// Returns error if desktop file is not installed.
	SetAsDefaultBrowser(ctx context.Context) error

	// UnsetAsDefaultBrowser resets default browser if dumber is currently default.
	// Idempotent: returns nil if not currently default.
	UnsetAsDefaultBrowser(ctx context.Context) error
}
