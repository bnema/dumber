package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/bnema/dumber/internal/logging"
)

// HistoryMessage contains fields for history operations.
type HistoryMessage struct {
	Type      string `json:"type"`
	Q         string `json:"q"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
	HistoryID string `json:"historyId"`
	RequestID string `json:"requestId"`
	Range     string `json:"range"`  // hour, day, week, month
	Domain    string `json:"domain"` // for domain-based deletion
}

// HandleHistoryRecent processes recent history requests.
func HandleHistoryRecent(c *Context, msg HistoryMessage) {
	if !c.IsReady() {
		return
	}

	limit := msg.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := msg.Offset

	var entries []interface{}
	var err error

	if offset > 0 {
		histEntries, e := c.BrowserService.GetRecentHistoryWithOffset(c.Ctx(), limit, offset)
		if e != nil {
			err = e
		} else {
			entries = make([]interface{}, len(histEntries))
			for i, entry := range histEntries {
				entries[i] = entry
			}
		}
	} else {
		histEntries, e := c.BrowserService.GetRecentHistory(c.Ctx(), limit)
		if e != nil {
			err = e
		} else {
			entries = make([]interface{}, len(histEntries))
			for i, entry := range histEntries {
				entries[i] = entry
			}
		}
	}

	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to get recent history: %v", err))
		requestIdParam := ""
		if msg.RequestID != "" {
			requestIdParam = ", '" + msg.RequestID + "'"
		}
		_ = c.WebView.InjectScript("window.__dumber_history_error && window.__dumber_history_error('Failed to load recent history'" + requestIdParam + ")")
		return
	}

	b, _ := json.Marshal(entries)
	requestIdParam := ""
	if msg.RequestID != "" {
		requestIdParam = ", '" + msg.RequestID + "'"
	}
	_ = c.WebView.InjectScript("window.__dumber_history_recent && window.__dumber_history_recent(" + string(b) + requestIdParam + ")")
}

// HandleHistoryStats processes history stats requests.
func HandleHistoryStats(c *Context) {
	if !c.IsReady() {
		return
	}

	stats, err := c.BrowserService.GetHistoryStats(c.Ctx())
	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to get history stats: %v", err))
		_ = c.InjectError("__dumber_history_error", "Failed to load history stats")
		return
	}

	_ = c.InjectJSON("__dumber_history_stats", stats)
}

// HandleHistorySearch processes history search requests.
func HandleHistorySearch(c *Context, msg HistoryMessage) {
	if !c.IsReady() {
		return
	}

	limit := msg.Limit
	if limit <= 0 {
		limit = 5
	}

	entries, err := c.BrowserService.SearchHistory(c.Ctx(), msg.Q, limit)
	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to search history: %v", err))
		_ = c.InjectError("__dumber_history_error", "Failed to search history")
		return
	}

	_ = c.InjectJSON("__dumber_history_search", entries)
}

// HandleHistoryDelete processes single history entry deletion.
func HandleHistoryDelete(c *Context, msg HistoryMessage) {
	if !c.IsReady() {
		return
	}

	if msg.HistoryID == "" {
		logging.Warn("[handlers] History delete: missing historyId")
		_ = c.InjectError("__dumber_history_error", "Missing history ID")
		return
	}

	id, err := strconv.ParseInt(msg.HistoryID, 10, 64)
	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] History delete: invalid ID format: %v", err))
		_ = c.InjectError("__dumber_history_error", "Invalid history ID format")
		return
	}

	if err := c.BrowserService.DeleteHistoryEntry(c.Ctx(), id); err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to delete history entry: %v", err))
		_ = c.InjectError("__dumber_history_error", "Failed to delete history entry")
		return
	}

	_ = c.InjectJSON("__dumber_history_deleted", map[string]string{"deletedId": msg.HistoryID})
}

// HandleHistoryTimeline returns history grouped by date for timeline display.
func HandleHistoryTimeline(c *Context, msg HistoryMessage) {
	if !c.IsReady() {
		return
	}

	limit := msg.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := msg.Offset

	entries, err := c.BrowserService.GetHistoryTimeline(c.Ctx(), limit, offset)
	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to get history timeline: %v", err))
		_ = c.InjectError("__dumber_history_error", "Failed to load history timeline")
		return
	}

	_ = c.InjectJSONWithRequestID("__dumber_history_timeline", entries, msg.RequestID)
}

// HandleHistorySearchFTS performs full-text search on history.
func HandleHistorySearchFTS(c *Context, msg HistoryMessage) {
	if !c.IsReady() {
		return
	}

	if msg.Q == "" {
		_ = c.InjectJSONWithRequestID("__dumber_history_search_results", []interface{}{}, msg.RequestID)
		return
	}

	limit := msg.Limit
	if limit <= 0 {
		limit = 100
	}

	entries, err := c.BrowserService.SearchHistoryFTS(c.Ctx(), msg.Q, limit)
	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to FTS search history: %v", err))
		_ = c.InjectError("__dumber_history_error", "Failed to search history")
		return
	}

	_ = c.InjectJSONWithRequestID("__dumber_history_search_results", entries, msg.RequestID)
}

// HandleHistoryDeleteRange deletes history by time range.
func HandleHistoryDeleteRange(c *Context, msg HistoryMessage) {
	if !c.IsReady() {
		return
	}

	var err error
	switch msg.Range {
	case "hour":
		err = c.BrowserService.DeleteHistoryLastHour(c.Ctx())
	case "day":
		err = c.BrowserService.DeleteHistoryLastDay(c.Ctx())
	case "week":
		err = c.BrowserService.DeleteHistoryLastWeek(c.Ctx())
	case "month":
		err = c.BrowserService.DeleteHistoryLastMonth(c.Ctx())
	default:
		_ = c.InjectError("__dumber_history_error", "Invalid time range")
		return
	}

	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to delete history range %s: %v", msg.Range, err))
		_ = c.InjectError("__dumber_history_error", "Failed to clear history")
		return
	}

	_ = c.InjectJSON("__dumber_history_range_deleted", map[string]string{"range": msg.Range})
}

// HandleHistoryClearAll deletes all history.
func HandleHistoryClearAll(c *Context) {
	if !c.IsReady() {
		return
	}

	if err := c.BrowserService.ClearAllHistory(c.Ctx()); err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to clear all history: %v", err))
		_ = c.InjectError("__dumber_history_error", "Failed to clear history")
		return
	}

	_ = c.InjectJSON("__dumber_history_cleared", map[string]bool{"success": true})
}

// HandleHistoryDeleteDomain deletes all history for a domain.
func HandleHistoryDeleteDomain(c *Context, msg HistoryMessage) {
	if !c.IsReady() {
		return
	}

	if msg.Domain == "" {
		_ = c.InjectError("__dumber_history_error", "Missing domain")
		return
	}

	if err := c.BrowserService.DeleteHistoryByDomain(c.Ctx(), msg.Domain); err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to delete history for domain %s: %v", msg.Domain, err))
		_ = c.InjectError("__dumber_history_error", "Failed to delete domain history")
		return
	}

	_ = c.InjectJSON("__dumber_history_domain_deleted", map[string]string{"domain": msg.Domain})
}

// HandleHistoryAnalytics returns history analytics data.
func HandleHistoryAnalytics(c *Context, msg HistoryMessage) {
	if !c.IsReady() {
		return
	}

	analytics, err := c.BrowserService.GetHistoryAnalytics(c.Ctx())
	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to get history analytics: %v", err))
		_ = c.InjectError("__dumber_history_error", "Failed to load analytics")
		return
	}

	_ = c.InjectJSONWithRequestID("__dumber_analytics", analytics, msg.RequestID)
}

// HandleDomainStats returns domain statistics.
func HandleDomainStats(c *Context, msg HistoryMessage) {
	if !c.IsReady() {
		return
	}

	limit := msg.Limit
	if limit <= 0 {
		limit = 20
	}

	stats, err := c.BrowserService.GetDomainStats(c.Ctx(), limit)
	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to get domain stats: %v", err))
		_ = c.InjectError("__dumber_history_error", "Failed to load domain stats")
		return
	}

	_ = c.InjectJSONWithRequestID("__dumber_domain_stats", stats, msg.RequestID)
}

// parseNullString converts a string to sql.NullString for domain deletion.
func parseNullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
