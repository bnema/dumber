package homepage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
)

// HistoryHandlers handles history-related messages from the homepage.
type HistoryHandlers struct {
	historyUC port.HomepageHistory
}

// NewHistoryHandlers creates a new HistoryHandlers instance.
func NewHistoryHandlers(historyUC port.HomepageHistory) *HistoryHandlers {
	return &HistoryHandlers{historyUC: historyUC}
}

// timelineRequest is the payload for history_timeline messages.
type timelineRequest struct {
	RequestID string `json:"requestId"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
}

type timelineByDomainRequest struct {
	RequestID string `json:"requestId"`
	Domain    string `json:"domain"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
}

type timelineWindowRequest struct {
	RequestID string `json:"requestId"`
	Before    string `json:"before"`
	Domain    string `json:"domain"`
}

// HandleTimeline handles history_timeline messages.
func (h *HistoryHandlers) HandleTimeline() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
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
		if req.Limit <= 0 {
			return NewErrorResponse(req.RequestID, fmt.Errorf("history_timeline requires a positive limit; use history_timeline_window for lazy history loading")), nil
		}

		entries, err := h.historyUC.GetRecent(ctx, req.Limit, req.Offset)
		if err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, entries), nil
	})
}

// HandleTimelineByDomain handles history_timeline_by_domain messages.
func (h *HistoryHandlers) HandleTimelineByDomain() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req timelineByDomainRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		req.Domain = strings.TrimSpace(req.Domain)
		if req.Domain == "" {
			return NewErrorResponse(req.RequestID, fmt.Errorf("domain is required")), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Str("domain", req.Domain).
			Int("limit", req.Limit).
			Int("offset", req.Offset).
			Msg("handling history_timeline_by_domain")
		if req.Limit <= 0 {
			return NewErrorResponse(req.RequestID, fmt.Errorf("history_timeline_by_domain requires a positive limit; use history_timeline_window for lazy history loading")), nil
		}

		entries, err := h.historyUC.GetRecentByDomain(ctx, req.Domain, req.Limit, req.Offset)
		if err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, entries), nil
	})
}

// HandleTimelineWindow handles history_timeline_window messages.
func (h *HistoryHandlers) HandleTimelineWindow() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req timelineWindowRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		domain := strings.TrimSpace(req.Domain)

		var before time.Time
		if strings.TrimSpace(req.Before) != "" {
			parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(req.Before))
			if err != nil {
				return NewErrorResponse(req.RequestID, fmt.Errorf("invalid history window cursor")), nil
			}
			before = parsed
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Str("domain", domain).
			Time("before", before).
			Msg("handling history_timeline_window")

		window, err := h.historyUC.GetRecentWindow(ctx, before, domain)
		if err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, window), nil
	})
}

// searchRequest is the payload for history_search_fts messages.
type searchRequest struct {
	RequestID string `json:"requestId"`
	Query     string `json:"query"`
	Limit     int    `json:"limit"`
}

// HandleSearchFTS handles history_search_fts messages.
func (h *HistoryHandlers) HandleSearchFTS() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req searchRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().
			Str("request_id", req.RequestID).
			Str("query", req.Query).
			Msg("handling history_search_fts")

		output, err := h.historyUC.Search(ctx, port.HistorySearchInput{
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
func (h *HistoryHandlers) HandleDeleteEntry() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
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
func (h *HistoryHandlers) HandleDeleteRange() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req deleteRangeRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		req.Range = strings.TrimSpace(req.Range)
		log.Debug().
			Str("request_id", req.RequestID).
			Str("range", req.Range).
			Msg("handling history_delete_range")

		if err := h.historyUC.ClearRange(ctx, req.Range); err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, nil), nil
	})
}

// HandleClearAll handles history_clear_all messages.
func (h *HistoryHandlers) HandleClearAll() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
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

// HandleStats handles history_stats messages.
func (h *HistoryHandlers) HandleStats() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req struct {
			RequestID string `json:"requestId"`
		}
		if err := json.Unmarshal(payload, &req); err != nil {
			return NewErrorResponse("", err), nil
		}

		log.Debug().Str("request_id", req.RequestID).Msg("handling history_stats")

		stats, err := h.historyUC.GetStats(ctx)
		if err != nil {
			return NewErrorResponse(req.RequestID, err), nil
		}

		return NewSuccessResponse(req.RequestID, stats), nil
	})
}

// HandleAnalytics handles history_analytics messages.
func (h *HistoryHandlers) HandleAnalytics() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
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
func (h *HistoryHandlers) HandleDomainStats() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
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
func (h *HistoryHandlers) HandleDeleteDomain() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
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
