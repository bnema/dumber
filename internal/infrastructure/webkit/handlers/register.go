package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/infrastructure/webkit/handlers/homepage"
	"github.com/bnema/dumber/internal/logging"
)

// AccentKeyHandler is implemented by the InsertAccentUseCase to receive
// key press/release events forwarded from WebView JS via the message bridge.
type AccentKeyHandler interface {
	OnKeyPressed(ctx context.Context, char rune, shiftHeld bool) bool
	OnKeyReleased(ctx context.Context, char rune)
}

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
	keybindingsHandler, err := createKeybindingsHandler()
	if err != nil {
		return fmt.Errorf("failed to create keybindings handler: %w", err)
	}
	if err := RegisterKeybindingsHandlers(ctx, router, keybindingsHandler); err != nil {
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

// RegisterAccentHandlers registers accent key press/release handlers with the router.
// Must be called after the AccentKeyHandler is initialized (i.e., after initAccentPicker).
func RegisterAccentHandlers(ctx context.Context, router *webkit.MessageRouter, handler AccentKeyHandler) error {
	if router == nil {
		return fmt.Errorf("RegisterAccentHandlers: router must not be nil")
	}
	if handler == nil {
		return fmt.Errorf("RegisterAccentHandlers: handler must not be nil")
	}

	if err := router.RegisterHandler("accent_key_press", webkit.MessageHandlerFunc(
		func(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
			var p struct {
				Char  string `json:"char"`
				Shift bool   `json:"shift"`
			}
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			if r, _ := utf8.DecodeRuneInString(p.Char); r != utf8.RuneError && utf8.RuneCountInString(p.Char) == 1 {
				handler.OnKeyPressed(ctx, r, p.Shift)
			}
			return nil, nil
		},
	)); err != nil {
		return err
	}

	if err := router.RegisterHandler("accent_key_release", webkit.MessageHandlerFunc(
		func(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
			var p struct {
				Char string `json:"char"`
			}
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			if r, _ := utf8.DecodeRuneInString(p.Char); r != utf8.RuneError && utf8.RuneCountInString(p.Char) == 1 {
				handler.OnKeyReleased(ctx, r)
			}
			return nil, nil
		},
	)); err != nil {
		return err
	}

	log := logging.FromContext(ctx)
	log.Info().Msg("registered accent key handlers")
	return nil
}

// createKeybindingsHandler wires the gateway and use cases for keybindings.
func createKeybindingsHandler() (*KeybindingsHandler, error) {
	mgr := config.GetManager()
	if mgr == nil {
		return nil, fmt.Errorf("config manager not initialized")
	}

	gateway := config.NewKeybindingsGateway(mgr)

	return NewKeybindingsHandler(
		usecase.NewGetKeybindingsUseCase(gateway),
		usecase.NewSetKeybindingUseCase(gateway, gateway),
		usecase.NewResetKeybindingUseCase(gateway),
		usecase.NewResetAllKeybindingsUseCase(gateway),
	), nil
}
