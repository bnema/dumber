package homepage

import (
	"context"
	"encoding/json"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// FavoritesHandlers handles favorite-related messages from the homepage.
type FavoritesHandlers struct {
	favoritesUC *usecase.ManageFavoritesUseCase
}

// NewFavoritesHandlers creates a new FavoritesHandlers instance.
func NewFavoritesHandlers(favoritesUC *usecase.ManageFavoritesUseCase) *FavoritesHandlers {
	return &FavoritesHandlers{favoritesUC: favoritesUC}
}

// HandleList handles favorite_list messages.
func (h *FavoritesHandlers) HandleList() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		requestID := ParseRequestID(payload)

		log.Debug().
			Str("request_id", requestID).
			Msg("handling favorite_list")

		favorites, err := h.favoritesUC.GetAll(ctx)
		if err != nil {
			return NewErrorResponse(requestID, err), nil
		}

		return NewSuccessResponse(requestID, favorites), nil
	})
}

// setShortcutRequest is the payload for favorite_set_shortcut messages.
type setShortcutRequest struct {
	RequestID   string `json:"requestId"`
	FavoriteID  int64  `json:"favorite_id"`
	ShortcutKey *int   `json:"shortcut_key"` // 1-9 or null to remove
}

// HandleSetShortcut handles favorite_set_shortcut messages.
func (h *FavoritesHandlers) HandleSetShortcut() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req setShortcutRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Int64("favorite_id", req.FavoriteID).
			Msg("handling favorite_set_shortcut")

		if err := h.favoritesUC.SetShortcut(ctx, entity.FavoriteID(req.FavoriteID), req.ShortcutKey); err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, nil), nil
	})
}

// getByShortcutRequest is the payload for favorite_get_by_shortcut messages.
type getByShortcutRequest struct {
	RequestID   string `json:"requestId"`
	ShortcutKey int    `json:"shortcut_key"`
}

// HandleGetByShortcut handles favorite_get_by_shortcut messages.
func (h *FavoritesHandlers) HandleGetByShortcut() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req getByShortcutRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Int("shortcut_key", req.ShortcutKey).
			Msg("handling favorite_get_by_shortcut")

		favorite, err := h.favoritesUC.GetByShortcut(ctx, req.ShortcutKey)
		if err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, favorite), nil
	})
}

// setFolderRequest is the payload for favorite_set_folder messages.
type setFolderRequest struct {
	RequestID  string `json:"requestId"`
	FavoriteID int64  `json:"favorite_id"`
	FolderID   *int64 `json:"folder_id"` // null to move to root
}

// HandleSetFolder handles favorite_set_folder messages.
func (h *FavoritesHandlers) HandleSetFolder() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req setFolderRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Int64("favorite_id", req.FavoriteID).
			Msg("handling favorite_set_folder")

		var folderID *entity.FolderID
		if req.FolderID != nil {
			id := entity.FolderID(*req.FolderID)
			folderID = &id
		}

		if err := h.favoritesUC.Move(ctx, entity.FavoriteID(req.FavoriteID), folderID); err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, nil), nil
	})
}
