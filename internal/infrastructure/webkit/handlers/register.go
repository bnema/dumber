package handlers

import (
	"context"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/infrastructure/webkit/handlers/homepage"
)

// Config holds all dependencies for message handlers.
type Config struct {
	HistoryUC   *usecase.SearchHistoryUseCase
	FavoritesUC *usecase.ManageFavoritesUseCase
}

// RegisterAll registers all message handlers with the router.
func RegisterAll(ctx context.Context, router *webkit.MessageRouter, cfg Config) error {
	// Homepage handlers (history, favorites, folders, tags)
	if cfg.HistoryUC != nil && cfg.FavoritesUC != nil {
		if err := homepage.RegisterHandlers(ctx, router, homepage.Config{
			HistoryUC:   cfg.HistoryUC,
			FavoritesUC: cfg.FavoritesUC,
		}); err != nil {
			return err
		}
	}

	// Configuration handlers (always available)
	if err := RegisterConfigHandlers(ctx, router); err != nil {
		return err
	}

	// Future handler groups go here:
	// - settings handlers
	// - downloads handlers
	// - etc.

	return nil
}
