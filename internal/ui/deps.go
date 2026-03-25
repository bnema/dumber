// Package ui provides the GTK4 presentation layer for the dumber browser.
package ui

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/ui/adapter"
	"github.com/bnema/dumber/internal/ui/theme"
	"github.com/bnema/puregotk/v4/gtk"
)

// Dependencies holds all injected dependencies for the UI layer.
// This struct is created once at startup and passed to UI components.
type Dependencies struct {
	// Core context and configuration
	Ctx                    context.Context
	Config                 *config.Config // TODO: replace with port interface in a later milestone
	InitialURL             string         // URL to open on startup (optional)
	RestoreSessionID       string         // Session ID to restore on startup (optional)
	StartupCrashReports    []string
	OnFirstWebViewShown    func(context.Context)
	OnSessionPersisted     func() // Called by main after session is persisted to DB
	OnCrashReportsDetected func([]string)

	// Theme and color scheme management
	Theme           *theme.Manager
	ColorResolver   port.ColorSchemeResolver
	AdwaitaDetector port.ToolkitAvailabilityNotifier

	// XDG paths
	XDG port.XDGPaths

	// Engine (replaces individual webkit fields)
	Engine port.Engine

	// Repositories
	HistoryRepo    repository.HistoryRepository
	FavoriteRepo   repository.FavoriteRepository
	ZoomRepo       repository.ZoomRepository
	PermissionRepo port.PermissionRepository
	FilterRepo     repository.ContentWhitelistRepository

	// Use Cases
	TabsUC       *usecase.ManageTabsUseCase
	PanesUC      *usecase.ManagePanesUseCase
	NavigateUC   *usecase.NavigateUseCase
	ZoomUC       *usecase.ManageZoomUseCase
	PermissionUC *usecase.HandlePermissionUseCase
	FavoritesUC  *usecase.ManageFavoritesUseCase
	HistoryUC    *usecase.SearchHistoryUseCase
	CopyURLUC    *usecase.CopyURLUseCase

	// Infrastructure Adapters
	Clipboard            port.Clipboard
	FaviconService       port.FaviconService
	FaviconAdapterConfig adapter.FaviconAdapterConfig
	FilterManager        port.FilterManager
	IdleInhibitor        port.IdleInhibitor

	// Accent picker for dead keys support
	InsertAccentUC      *usecase.InsertAccentUseCase
	AccentFocusProvider port.FocusedInputProvider
	// NewGTKEntryTarget creates a port.TextInputTarget from a GTK SearchEntry.
	// Injected to avoid importing infrastructure/textinput from the UI layer.
	NewGTKEntryTarget func(entry *gtk.SearchEntry) port.TextInputTarget

	// Session management
	SessionStateRepo repository.SessionStateRepository
	SessionRepo      repository.SessionRepository
	CurrentSessionID entity.SessionID
	SnapshotUC       *usecase.SnapshotSessionUseCase
	// SnapshotServiceFactory creates a snapshot service bound to the given tab-list provider.
	// Called after the App is initialized so the App can serve as the provider.
	SnapshotServiceFactory func(provider port.TabListProvider, intervalMs int) port.SnapshotService
	// SessionSpawner spawns a new dumber instance for session restoration.
	SessionSpawner port.SessionSpawner
	// FileSystem provides file operations (e.g., for download deduplication).
	FileSystem port.FileSystem
	// OnConfigChange registers a callback for config hot-reload.
	// The config pointer in deps.Config is updated in-place before the callback fires.
	OnConfigChange func(callback func())
	// WatchConfig starts watching the config file for changes.
	WatchConfig func() error

	// Update management
	CheckUpdateUC *usecase.CheckUpdateUseCase
	ApplyUpdateUC *usecase.ApplyUpdateUseCase

	// Config migration checker (optional; nil disables migration notifications)
	ConfigMigrator port.ConfigMigrator

	// LaunchExternalURL opens a URI with the system's default handler (e.g. xdg-open).
	// Used for external URL schemes (vscode://, spotify://, etc.).
	// Optional: if nil, external URLs are silently dropped.
	LaunchExternalURL func(uri string)
}

// Validate checks that all required dependencies are set.
func (d *Dependencies) Validate() error {
	if d.Ctx == nil {
		return ErrMissingDependency("Ctx")
	}
	if d.Config == nil {
		return ErrMissingDependency("Config")
	}
	if d.Engine == nil {
		return ErrMissingDependency("Engine")
	}
	// Use cases are optional - can be nil if not needed
	return nil
}

// DependencyError indicates a missing required dependency.
type DependencyError struct {
	Name string
}

func (e DependencyError) Error() string {
	return "missing required dependency: " + e.Name
}

// ErrMissingDependency creates a new DependencyError.
func ErrMissingDependency(name string) error {
	return DependencyError{Name: name}
}
