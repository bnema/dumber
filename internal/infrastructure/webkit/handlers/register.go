package handlers

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/infrastructure/webkit/handlers/homepage"
)

// Config holds all dependencies for message handlers.
type Config struct {
	HistoryUC         *usecase.SearchHistoryUseCase
	FavoritesUC       *usecase.ManageFavoritesUseCase
	Clipboard         port.Clipboard
	ConfigGetter      func() *config.Config
	OnClipboardCopied func(textLen int) // Called when auto-copy completes (for toast notification)
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

	// Keybindings handlers (always available)
	if err := RegisterKeybindingsHandlers(ctx, router); err != nil {
		return err
	}

	// Clipboard handlers (for auto-copy on selection feature)
	if cfg.Clipboard != nil && cfg.ConfigGetter != nil {
		if err := RegisterClipboardHandlers(ctx, router, cfg.Clipboard, cfg.ConfigGetter, cfg.OnClipboardCopied); err != nil {
			return err
		}
	}

	// Future handler groups go here:
	// - settings handlers
	// - downloads handlers
	// - etc.

	return nil
}
