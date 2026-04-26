package homepage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
)

func folderIDFromInt64(id *int64) (*entity.FolderID, error) {
	if id == nil {
		return nil, nil
	}
	if *id <= 0 {
		return nil, fmt.Errorf("folder id must be positive")
	}
	folderID := entity.FolderID(*id)
	return &folderID, nil
}

func tagIDsFromInt64s(ids []int64) ([]entity.TagID, error) {
	out := make([]entity.TagID, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("tag id must be positive")
		}
		out = append(out, entity.TagID(id))
	}
	return out, nil
}

// FavoritesHandlers handles favorite-related messages from the homepage.
type FavoritesHandlers struct {
	favoritesUC port.HomepageFavorites
}

// NewFavoritesHandlers creates a new FavoritesHandlers instance.
func NewFavoritesHandlers(favoritesUC port.HomepageFavorites) *FavoritesHandlers {
	return &FavoritesHandlers{favoritesUC: favoritesUC}
}

// HandleList handles favorite_list messages.
func (h *FavoritesHandlers) HandleList() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
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

type favoriteCreateRequest struct {
	RequestID  string  `json:"requestId"`
	URL        string  `json:"url"`
	Title      string  `json:"title"`
	FaviconURL string  `json:"favicon_url"`
	FolderID   *int64  `json:"folder_id"`
	Tags       []int64 `json:"tags"`
}

// HandleCreate handles favorite_create messages.
func (h *FavoritesHandlers) HandleCreate() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req favoriteCreateRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Msg("handling favorite_create")

		trimmedURL := strings.TrimSpace(req.URL)
		if trimmedURL == "" {
			return NewErrorResponse(req.RequestID, fmt.Errorf("URL is required")), nil
		}
		folderID, err := folderIDFromInt64(req.FolderID)
		if err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}
		tags, err := tagIDsFromInt64s(req.Tags)
		if err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		input := dto.FavoriteCreateInput{
			URL:        trimmedURL,
			Title:      req.Title,
			FaviconURL: req.FaviconURL,
			FolderID:   folderID,
			Tags:       tags,
		}
		favorite, err := h.favoritesUC.AddFavorite(ctx, input)
		if err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, favorite), nil
	})
}

type favoriteUpdateRequest struct {
	RequestID   string `json:"requestId"`
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	FaviconURL  string `json:"favicon_url"`
	FolderID    *int64 `json:"folder_id"`
	ShortcutKey *int   `json:"shortcut_key"`
}

// HandleUpdate handles favorite_update messages.
func (h *FavoritesHandlers) HandleUpdate() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req favoriteUpdateRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Int64("favorite_id", req.ID).
			Msg("handling favorite_update")

		if req.ID <= 0 {
			return NewErrorResponse(req.RequestID, fmt.Errorf("favorite id must be positive")), nil
		}

		folderID, err := folderIDFromInt64(req.FolderID)
		if err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		favorite, err := h.favoritesUC.UpdateFavorite(ctx, dto.FavoriteUpdateInput{
			ID:          entity.FavoriteID(req.ID),
			Title:       req.Title,
			FaviconURL:  req.FaviconURL,
			FolderID:    folderID,
			ShortcutKey: req.ShortcutKey,
		})
		if err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, favorite), nil
	})
}

type favoriteDeleteRequest struct {
	RequestID string `json:"requestId"`
	ID        int64  `json:"id"`
}

// HandleDelete handles favorite_delete messages.
func (h *FavoritesHandlers) HandleDelete() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req favoriteDeleteRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Int64("favorite_id", req.ID).
			Msg("handling favorite_delete")

		if req.ID <= 0 {
			return NewErrorResponse(req.RequestID, fmt.Errorf("favorite id must be positive")), nil
		}

		if err := h.favoritesUC.DeleteFavorite(ctx, entity.FavoriteID(req.ID)); err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, nil), nil
	})
}

// setShortcutRequest is the payload for favorite_set_shortcut messages.
type setShortcutRequest struct {
	RequestID   string `json:"requestId"`
	FavoriteID  int64  `json:"favorite_id"`
	ShortcutKey *int   `json:"shortcut_key"` // 1-9 or null to remove
}

// HandleSetShortcut handles favorite_set_shortcut messages.
func (h *FavoritesHandlers) HandleSetShortcut() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req setShortcutRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Int64("favorite_id", req.FavoriteID).
			Msg("handling favorite_set_shortcut")

		if req.FavoriteID <= 0 {
			return NewErrorResponse(req.RequestID, fmt.Errorf("invalid favorite id")), nil
		}

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
func (h *FavoritesHandlers) HandleGetByShortcut() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
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
func (h *FavoritesHandlers) HandleSetFolder() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req setFolderRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Int64("favorite_id", req.FavoriteID).
			Msg("handling favorite_set_folder")

		if req.FavoriteID <= 0 {
			return NewErrorResponse(req.RequestID, fmt.Errorf("invalid favorite_id")), nil
		}

		folderID, err := folderIDFromInt64(req.FolderID)
		if err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		if err := h.favoritesUC.Move(ctx, entity.FavoriteID(req.FavoriteID), folderID); err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, nil), nil
	})
}
