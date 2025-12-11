package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// Suggestion represents a minimal suggestion payload for the frontend.
type Suggestion struct {
	URL        string `json:"url"`
	Title      string `json:"title,omitempty"`
	FaviconURL string `json:"favicon_url,omitempty"`
	Favicon    string `json:"favicon,omitempty"`
}

// QueryHandler responds to omnibox search queries.
type QueryHandler struct {
	history *usecase.SearchHistoryUseCase
	config  *config.Config
}

// NewQueryHandler creates a new QueryHandler.
func NewQueryHandler(history *usecase.SearchHistoryUseCase, cfg *config.Config) *QueryHandler {
	return &QueryHandler{
		history: history,
		config:  cfg,
	}
}

// Handle executes a fuzzy history search and returns suggestions.
func (h *QueryHandler) Handle(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
	log := logging.FromContext(ctx)

	var req struct {
		Query string `json:"q"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("decode query payload: %w", err)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
		if h.config != nil && h.config.Omnibox.InitialBehavior != "" {
			// Keep limit low by default; no per-behavior tuning yet.
			limit = 10
		}
	}

	if h.history == nil {
		log.Warn().Msg("history use case is nil; returning empty suggestions")
		return []Suggestion{}, nil
	}

	result, err := h.history.Search(ctx, usecase.SearchInput{
		Query: strings.TrimSpace(req.Query),
		Limit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("history search failed: %w", err)
	}

	return toSuggestions(result.Matches), nil
}

// InitialHistoryHandler returns initial history suggestions for an empty omnibox.
type InitialHistoryHandler struct {
	history *usecase.SearchHistoryUseCase
	config  *config.Config
}

// NewInitialHistoryHandler creates a new handler for initial history load.
func NewInitialHistoryHandler(history *usecase.SearchHistoryUseCase, cfg *config.Config) *InitialHistoryHandler {
	return &InitialHistoryHandler{
		history: history,
		config:  cfg,
	}
}

// Handle returns recent history entries as suggestions.
func (h *InitialHistoryHandler) Handle(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
	log := logging.FromContext(ctx)

	var req struct {
		Limit int `json:"limit"`
	}
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &req)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	if h.history == nil {
		log.Warn().Msg("history use case is nil; returning empty initial history")
		return []Suggestion{}, nil
	}

	entries, err := h.history.GetRecent(ctx, limit, 0)
	if err != nil {
		return nil, fmt.Errorf("get recent history: %w", err)
	}

	suggestions := make([]Suggestion, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		suggestions = append(suggestions, Suggestion{
			URL:        entry.URL,
			Title:      entry.Title,
			FaviconURL: entry.FaviconURL,
			Favicon:    entry.FaviconURL,
		})
	}

	return suggestions, nil
}

// PrefixQueryHandler returns an inline suggestion (ghost text) for a prefix query.
type PrefixQueryHandler struct {
	history *usecase.SearchHistoryUseCase
}

// NewPrefixQueryHandler creates a new handler for prefix inline queries.
func NewPrefixQueryHandler(history *usecase.SearchHistoryUseCase) *PrefixQueryHandler {
	return &PrefixQueryHandler{history: history}
}

// Handle returns the best matching URL for the given prefix (or null).
func (h *PrefixQueryHandler) Handle(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
	log := logging.FromContext(ctx)

	var req struct {
		Query string `json:"q"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("decode prefix query payload: %w", err)
	}

	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, nil
	}

	if h.history == nil {
		log.Warn().Msg("history use case is nil; returning no inline suggestion")
		return nil, nil
	}

	result, err := h.history.Search(ctx, usecase.SearchInput{
		Query: query,
		Limit: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("inline history search failed: %w", err)
	}

	if len(result.Matches) == 0 || result.Matches[0].Entry == nil {
		return nil, nil
	}

	return result.Matches[0].Entry.URL, nil
}

// toSuggestions converts history matches into UI suggestions.
func toSuggestions(matches []entity.HistoryMatch) []Suggestion {
	suggestions := make([]Suggestion, 0, len(matches))
	for _, match := range matches {
		if match.Entry == nil {
			continue
		}
		suggestions = append(suggestions, Suggestion{
			URL:        match.Entry.URL,
			Title:      match.Entry.Title,
			FaviconURL: match.Entry.FaviconURL,
			Favicon:    match.Entry.FaviconURL,
		})
	}
	return suggestions
}
