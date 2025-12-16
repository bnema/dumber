package homepage

import (
	"context"
	"encoding/json"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// TagHandlers handles tag-related messages from the homepage.
type TagHandlers struct {
	favoritesUC *usecase.ManageFavoritesUseCase
}

// NewTagHandlers creates a new TagHandlers instance.
func NewTagHandlers(favoritesUC *usecase.ManageFavoritesUseCase) *TagHandlers {
	return &TagHandlers{favoritesUC: favoritesUC}
}

// HandleList handles tag_list messages.
func (h *TagHandlers) HandleList() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, webviewID webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		requestID := ParseRequestID(payload)

		log.Debug().
			Str("request_id", requestID).
			Msg("handling tag_list")

		tags, err := h.favoritesUC.GetAllTags(ctx)
		if err != nil {
			return NewErrorResponse(requestID, err), nil
		}

		return NewSuccessResponse(requestID, tags), nil
	})
}

// createTagRequest is the payload for tag_create messages.
type createTagRequest struct {
	RequestID string  `json:"requestId"`
	Name      string  `json:"name"`
	Color     *string `json:"color"`
}

// HandleCreate handles tag_create messages.
func (h *TagHandlers) HandleCreate() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, webviewID webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req createTagRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Str("name", req.Name).
			Msg("handling tag_create")

		color := ""
		if req.Color != nil {
			color = *req.Color
		}

		tag, err := h.favoritesUC.AddTag(ctx, req.Name, color)
		if err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, tag), nil
	})
}

// deleteTagRequest is the payload for tag_delete messages.
type deleteTagRequest struct {
	RequestID string `json:"requestId"`
	ID        int64  `json:"id"`
}

// HandleDelete handles tag_delete messages.
func (h *TagHandlers) HandleDelete() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, webviewID webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req deleteTagRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Int64("id", req.ID).
			Msg("handling tag_delete")

		if err := h.favoritesUC.DeleteTag(ctx, entity.TagID(req.ID)); err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, nil), nil
	})
}

// updateTagRequest is the payload for tag_update messages.
type updateTagRequest struct {
	RequestID string  `json:"requestId"`
	ID        int64   `json:"id"`
	Name      *string `json:"name"`
	Color     *string `json:"color"`
}

// HandleUpdate handles tag_update messages.
// NOTE: This requires UpdateTag() method to be added to ManageFavoritesUseCase.
func (h *TagHandlers) HandleUpdate() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, webviewID webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req updateTagRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Int64("id", req.ID).
			Msg("handling tag_update")

		name := ""
		if req.Name != nil {
			name = *req.Name
		}
		color := ""
		if req.Color != nil {
			color = *req.Color
		}

		if err := h.favoritesUC.UpdateTag(ctx, entity.TagID(req.ID), name, color); err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, nil), nil
	})
}

// tagAssignRequest is the payload for tag_assign messages.
type tagAssignRequest struct {
	RequestID  string `json:"requestId"`
	FavoriteID int64  `json:"favorite_id"`
	TagID      int64  `json:"tag_id"`
}

// HandleAssign handles tag_assign messages.
func (h *TagHandlers) HandleAssign() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, webviewID webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req tagAssignRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Int64("favorite_id", req.FavoriteID).
			Int64("tag_id", req.TagID).
			Msg("handling tag_assign")

		if err := h.favoritesUC.TagFavorite(ctx, entity.FavoriteID(req.FavoriteID), entity.TagID(req.TagID)); err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, nil), nil
	})
}

// HandleRemove handles tag_remove messages.
func (h *TagHandlers) HandleRemove() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, webviewID webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req tagAssignRequest // same structure as assign
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Int64("favorite_id", req.FavoriteID).
			Int64("tag_id", req.TagID).
			Msg("handling tag_remove")

		if err := h.favoritesUC.UntagFavorite(ctx, entity.FavoriteID(req.FavoriteID), entity.TagID(req.TagID)); err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, nil), nil
	})
}
