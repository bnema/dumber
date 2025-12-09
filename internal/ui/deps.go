// Package ui provides the GTK4 presentation layer for the dumber browser.
package ui

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/rs/zerolog"
)

// Dependencies holds all injected dependencies for the UI layer.
// This struct is created once at startup and passed to UI components.
type Dependencies struct {
	// Core context and configuration
	Ctx    context.Context
	Config *config.Config
	Logger *zerolog.Logger

	// WebKit infrastructure
	WebContext *webkit.WebKitContext
	Pool       port.WebViewPool
	Factory    port.WebViewFactory
	Settings   *webkit.SettingsManager

	// Repositories
	HistoryRepo  repository.HistoryRepository
	FavoriteRepo repository.FavoriteRepository
	ZoomRepo     repository.ZoomRepository
	FilterRepo   repository.ContentWhitelistRepository

	// Use Cases
	TabsUC      *usecase.ManageTabsUseCase
	PanesUC     *usecase.ManagePanesUseCase
	NavigateUC  *usecase.NavigateUseCase
	ZoomUC      *usecase.ManageZoomUseCase
	FavoritesUC *usecase.ManageFavoritesUseCase
	HistoryUC   *usecase.SearchHistoryUseCase
}

// Validate checks that all required dependencies are set.
func (d *Dependencies) Validate() error {
	if d.Ctx == nil {
		return ErrMissingDependency("Ctx")
	}
	if d.Config == nil {
		return ErrMissingDependency("Config")
	}
	if d.Logger == nil {
		return ErrMissingDependency("Logger")
	}
	// WebKit dependencies are optional for testing without WebViews
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
