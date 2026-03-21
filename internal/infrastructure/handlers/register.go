package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/handlers/homepage"
)

// AccentKeyHandler is implemented by the InsertAccentUseCase to receive
// key press/release events forwarded from WebView JS via the message bridge.
//
// Deprecated: use port.AccentKeyHandler instead. This alias is kept for compatibility.
type AccentKeyHandler = port.AccentKeyHandler

// RegisterAll registers all message handlers with the router.
// The router can be any engine's message router that implements port.WebUIHandlerRouter.
func RegisterAll(ctx context.Context, router port.WebUIHandlerRouter, deps port.HandlerDependencies) error {
	// Homepage handlers (history, favorites, folders, tags)
	if deps.HistoryUC != nil && deps.FavoritesUC != nil {
		if err := homepage.RegisterHandlers(ctx, router, homepage.Config{
			HistoryUC:   deps.HistoryUC,
			FavoritesUC: deps.FavoritesUC,
		}); err != nil {
			return err
		}
	}

	// Configuration handlers
	if deps.SaveConfig != nil {
		if err := RegisterConfigHandlers(ctx, router, deps.SaveConfig); err != nil {
			return err
		}
	}

	// Keybindings handlers (always available)
	if deps.KeybindingsGetter == nil {
		return fmt.Errorf("KeybindingsGetter is required")
	}
	kbHandler := NewKeybindingsHandler(
		deps.KeybindingsGetter,
		deps.KeybindingSetter,
		deps.KeybindingResetter,
		deps.AllKeybindingsResetter,
	)
	if err := RegisterKeybindingsHandlers(ctx, router, kbHandler); err != nil {
		return err
	}

	// Clipboard handlers (for auto-copy on selection feature)
	if deps.Clipboard != nil && deps.AutoCopyConfig != nil {
		if err := RegisterClipboardHandlers(ctx, router, deps.Clipboard, deps.AutoCopyConfig, deps.OnClipboardCopied); err != nil {
			return err
		}
	}

	return nil
}

// RegisterAccentHandlers registers accent key press/release handlers with the router.
// Must be called after the AccentKeyHandler is initialized (i.e., after initAccentPicker).
func RegisterAccentHandlers(_ context.Context, router port.WebUIHandlerRouter, handler AccentKeyHandler) error {
	if router == nil {
		return fmt.Errorf("RegisterAccentHandlers: router must not be nil")
	}
	if handler == nil {
		return fmt.Errorf("RegisterAccentHandlers: handler must not be nil")
	}

	if err := router.RegisterHandler("accent_key_press", port.WebUIMessageHandlerFunc(
		func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
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

	if err := router.RegisterHandler("accent_key_release", port.WebUIMessageHandlerFunc(
		func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
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
