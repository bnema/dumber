package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// Favorite represents the payload the frontend expects.
type Favorite struct {
	ID         int64  `json:"id"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	FaviconURL string `json:"favicon_url,omitempty"`
	Position   int    `json:"position,omitempty"`
}

// FavoritesHandler returns all favorites.
type FavoritesHandler struct {
	uc *usecase.ManageFavoritesUseCase
}

// NewFavoritesHandler creates a favorites handler.
func NewFavoritesHandler(uc *usecase.ManageFavoritesUseCase) *FavoritesHandler {
	return &FavoritesHandler{uc: uc}
}

// Handle returns the favorites list.
func (h *FavoritesHandler) Handle(ctx context.Context, _ webkit.WebViewID, _ json.RawMessage) (any, error) {
	log := logging.FromContext(ctx)

	if h.uc == nil {
		log.Warn().Msg("favorites use case is nil; returning empty favorites")
		return []Favorite{}, nil
	}

	favs, err := h.uc.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("get favorites: %w", err)
	}

	return mapFavorites(favs), nil
}

// ToggleFavoriteHandler toggles favorite status for a URL.
type ToggleFavoriteHandler struct {
	uc *usecase.ManageFavoritesUseCase
}

// NewToggleFavoriteHandler creates a toggle favorite handler.
func NewToggleFavoriteHandler(uc *usecase.ManageFavoritesUseCase) *ToggleFavoriteHandler {
	return &ToggleFavoriteHandler{uc: uc}
}

// Handle toggles a favorite and returns the updated list.
func (h *ToggleFavoriteHandler) Handle(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
	log := logging.FromContext(ctx)

	var req struct {
		URL        string `json:"url"`
		Title      string `json:"title"`
		FaviconURL string `json:"faviconURL"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("decode toggle favorite payload: %w", err)
	}

	if h.uc == nil {
		log.Warn().Msg("favorites use case is nil; cannot toggle favorite")
		return []Favorite{}, nil
	}

	existing, err := h.uc.GetByURL(ctx, req.URL)
	if err != nil {
		return nil, fmt.Errorf("check favorite: %w", err)
	}

	if existing != nil {
		if err := h.uc.Remove(ctx, existing.ID); err != nil {
			return nil, fmt.Errorf("remove favorite: %w", err)
		}
		log.Info().Str("url", req.URL).Msg("favorite removed via toggle")
	} else {
		_, err := h.uc.Add(ctx, usecase.AddFavoriteInput{
			URL:        req.URL,
			Title:      req.Title,
			FaviconURL: req.FaviconURL,
		})
		if err != nil {
			return nil, fmt.Errorf("add favorite: %w", err)
		}
		log.Info().Str("url", req.URL).Msg("favorite added via toggle")
	}

	// Return the refreshed list so the UI stays in sync.
	favs, err := h.uc.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("get favorites after toggle: %w", err)
	}

	return mapFavorites(favs), nil
}

func mapFavorites(favs []*entity.Favorite) []Favorite {
	result := make([]Favorite, 0, len(favs))
	for _, f := range favs {
		if f == nil {
			continue
		}
		result = append(result, Favorite{
			ID:         int64(f.ID),
			URL:        f.URL,
			Title:      f.Title,
			FaviconURL: f.FaviconURL,
			Position:   f.Position,
		})
	}
	return result
}
