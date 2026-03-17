package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/infrastructure/webkit/handlers/homepage"
)

// AccentKeyHandler is implemented by the InsertAccentUseCase to receive
// key press/release events forwarded from WebView JS via the message bridge.
// Deprecated: use port.AccentKeyHandler instead. This alias is kept for compatibility.
type AccentKeyHandler = port.AccentKeyHandler

// Config holds all dependencies for message handlers.
type Config struct {
	HistoryUC      port.HomepageHistory
	FavoritesUC    port.HomepageFavorites
	Clipboard      port.Clipboard
	AutoCopyConfig port.AutoCopyConfig
	SaveConfig     func(context.Context, port.WebUIConfig) error // Pre-built by bootstrap (usecase.SaveWebUIConfigUseCase.Execute)
	// KeybindingsHandler is required (not optional like other handlers) because the WebUI
	// always needs keybinding read/write support regardless of configuration.
	KeybindingsHandler *KeybindingsHandler // Pre-built by bootstrap
	OnClipboardCopied  func(textLen int)   // Called when auto-copy completes (for toast notification)
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

	// Configuration handlers
	if err := RegisterConfigHandlers(ctx, router, cfg.SaveConfig); err != nil {
		return err
	}

	// Keybindings handlers (always available)
	if cfg.KeybindingsHandler == nil {
		return fmt.Errorf("KeybindingsHandler is required")
	}
	if err := RegisterKeybindingsHandlers(ctx, router, cfg.KeybindingsHandler); err != nil {
		return err
	}

	// Clipboard handlers (for auto-copy on selection feature)
	if cfg.Clipboard != nil && cfg.AutoCopyConfig != nil {
		if err := RegisterClipboardHandlers(ctx, router, cfg.Clipboard, cfg.AutoCopyConfig, cfg.OnClipboardCopied); err != nil {
			return err
		}
	}

	return nil
}

// RegisterAccentHandlers registers accent key press/release handlers with the router.
// Must be called after the AccentKeyHandler is initialized (i.e., after initAccentPicker).
func RegisterAccentHandlers(_ context.Context, router *webkit.MessageRouter, handler AccentKeyHandler) error {
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

	return nil
}
