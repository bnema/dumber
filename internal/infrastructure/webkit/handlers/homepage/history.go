package homepage

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// HistoryHandlers handles history-related messages from the homepage.
type HistoryHandlers struct {
	historyUC *usecase.SearchHistoryUseCase
}

// NewHistoryHandlers creates a new HistoryHandlers instance.
func NewHistoryHandlers(historyUC *usecase.SearchHistoryUseCase) *HistoryHandlers {
	return &HistoryHandlers{historyUC: historyUC}
}

// timelineRequest is the payload for history_timeline messages.
type timelineRequest struct {
	RequestID string `json:"requestId"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
}

// HandleTimeline handles history_timeline messages.
func (h *HistoryHandlers) HandleTimeline() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req timelineRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Int("limit", req.Limit).
			Int("offset", req.Offset).
			Msg("handling history_timeline")

		entries, err := h.historyUC.GetRecent(ctx, req.Limit, req.Offset)
		if err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, entries), nil
	})
}

// searchRequest is the payload for history_search_fts messages.
type searchRequest struct {
	RequestID string `json:"requestId"`
	Query     string `json:"query"`
	Limit     int    `json:"limit"`
}

// HandleSearchFTS handles history_search_fts messages.
func (h *HistoryHandlers) HandleSearchFTS() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req searchRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Str("query", req.Query).
			Msg("handling history_search_fts")

		output, err := h.historyUC.Search(ctx, usecase.SearchInput{
			Query: req.Query,
			Limit: req.Limit,
		})
		if err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		// Convert matches to entries for frontend compatibility
		entries := make([]*entity.HistoryEntry, 0, len(output.Matches))
		for _, m := range output.Matches {
			entries = append(entries, m.Entry)
		}

		return NewSuccessResponse(req.RequestID, entries), nil
	})
}

// deleteEntryRequest is the payload for history_delete_entry messages.
type deleteEntryRequest struct {
	RequestID string `json:"requestId"`
	ID        int64  `json:"id"`
}

// HandleDeleteEntry handles history_delete_entry messages.
func (h *HistoryHandlers) HandleDeleteEntry() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req deleteEntryRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Int64("id", req.ID).
			Msg("handling history_delete_entry")

		if err := h.historyUC.Delete(ctx, req.ID); err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, nil), nil
	})
}

// deleteRangeRequest is the payload for history_delete_range messages.
type deleteRangeRequest struct {
	RequestID string `json:"requestId"`
	Range     string `json:"range"` // "hour", "day", "week", "month", "all"
}

// HandleDeleteRange handles history_delete_range messages.
func (h *HistoryHandlers) HandleDeleteRange() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req deleteRangeRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Str("range", req.Range).
			Msg("handling history_delete_range")

		before := rangeToTime(req.Range)
		if before.IsZero() {
			// "all" case - clear all
			if err := h.historyUC.ClearAll(ctx); err != nil {
				return NewErrorResponse(req.RequestID, err), nil
			}
		} else {
			if err := h.historyUC.ClearOlderThan(ctx, before); err != nil {
				return NewErrorResponse(req.RequestID, err), nil
			}
		}

		return NewSuccessResponse(req.RequestID, nil), nil
	})
}

// HandleClearAll handles history_clear_all messages.
func (h *HistoryHandlers) HandleClearAll() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		requestID := ParseRequestID(payload)

		log.Debug().
			Str("request_id", requestID).
			Msg("handling history_clear_all")

		if err := h.historyUC.ClearAll(ctx); err != nil {
			return NewErrorResponse(requestID, err), nil
		}

		return NewSuccessResponse(requestID, nil), nil
	})
}

// rangeToTime converts a range string to a time.Time cutoff.
// Returns zero time for "all" which means delete everything.
func rangeToTime(r string) time.Time {
	now := time.Now()
	switch r {
	case "hour":
		return now.Add(-time.Hour)
	case "day":
		return now.AddDate(0, 0, -1)
	case "week":
		return now.AddDate(0, 0, -7)
	case "month":
		return now.AddDate(0, -1, 0)
	case "all":
		return time.Time{} // Zero time signals "delete all"
	default:
		return now
	}
}

// HandleAnalytics handles history_analytics messages.
func (h *HistoryHandlers) HandleAnalytics() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		requestID := ParseRequestID(payload)

		log.Debug().
			Str("request_id", requestID).
			Msg("handling history_analytics")

		analytics, err := h.historyUC.GetAnalytics(ctx)
		if err != nil {
			return NewErrorResponse(requestID, err), nil
		}

		return NewSuccessResponse(requestID, analytics), nil
	})
}

// domainStatsRequest is the payload for history_domain_stats messages.
type domainStatsRequest struct {
	RequestID string `json:"requestId"`
	Limit     int    `json:"limit"`
}

// HandleDomainStats handles history_domain_stats messages.
func (h *HistoryHandlers) HandleDomainStats() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req domainStatsRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Int("limit", req.Limit).
			Msg("handling history_domain_stats")

		stats, err := h.historyUC.GetDomainStats(ctx, req.Limit)
		if err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, stats), nil
	})
}

// deleteDomainRequest is the payload for history_delete_domain messages.
type deleteDomainRequest struct {
	RequestID string `json:"requestId"`
	Domain    string `json:"domain"`
}

// HandleDeleteDomain handles history_delete_domain messages.
func (h *HistoryHandlers) HandleDeleteDomain() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req deleteDomainRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Str("domain", req.Domain).
			Msg("handling history_delete_domain")

		if err := h.historyUC.DeleteByDomain(ctx, req.Domain); err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, nil), nil
	})
}
