package homepage

import (
	"context"
	"encoding/json"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// FolderHandlers handles folder-related messages from the homepage.
type FolderHandlers struct {
	favoritesUC *usecase.ManageFavoritesUseCase
}

// NewFolderHandlers creates a new FolderHandlers instance.
func NewFolderHandlers(favoritesUC *usecase.ManageFavoritesUseCase) *FolderHandlers {
	return &FolderHandlers{favoritesUC: favoritesUC}
}

// HandleList handles folder_list messages.
func (h *FolderHandlers) HandleList() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, webviewID webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		requestID := ParseRequestID(payload)

		log.Debug().
			Str("request_id", requestID).
			Msg("handling folder_list")

		folders, err := h.favoritesUC.GetAllFolders(ctx)
		if err != nil {
			return NewErrorResponse(requestID, err), nil
		}

		return NewSuccessResponse(requestID, folders), nil
	})
}

// createFolderRequest is the payload for folder_create messages.
type createFolderRequest struct {
	RequestID string  `json:"requestId"`
	Name      string  `json:"name"`
	Icon      *string `json:"icon"`
}

// HandleCreate handles folder_create messages.
func (h *FolderHandlers) HandleCreate() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, webviewID webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req createFolderRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Str("name", req.Name).
			Msg("handling folder_create")

		folder, err := h.favoritesUC.CreateFolder(ctx, req.Name, nil)
		if err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		// Set icon if provided
		if req.Icon != nil && *req.Icon != "" {
			folder.Icon = *req.Icon
		}

		return NewSuccessResponse(req.RequestID, folder), nil
	})
}

// deleteFolderRequest is the payload for folder_delete messages.
type deleteFolderRequest struct {
	RequestID string `json:"requestId"`
	ID        int64  `json:"id"`
}

// HandleDelete handles folder_delete messages.
func (h *FolderHandlers) HandleDelete() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, webviewID webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req deleteFolderRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Int64("id", req.ID).
			Msg("handling folder_delete")

		if err := h.favoritesUC.DeleteFolder(ctx, entity.FolderID(req.ID)); err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, nil), nil
	})
}

// updateFolderRequest is the payload for folder_update messages.
type updateFolderRequest struct {
	RequestID string  `json:"requestId"`
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	Icon      *string `json:"icon"`
}

// HandleUpdate handles folder_update messages.
// NOTE: This requires UpdateFolder() method to be added to ManageFavoritesUseCase.
func (h *FolderHandlers) HandleUpdate() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, webviewID webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req updateFolderRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Int64("id", req.ID).
			Str("name", req.Name).
			Msg("handling folder_update")

		icon := ""
		if req.Icon != nil {
			icon = *req.Icon
		}

		if err := h.favoritesUC.UpdateFolder(ctx, entity.FolderID(req.ID), req.Name, icon); err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, nil), nil
	})
}
