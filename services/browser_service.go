package services

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/db"
)

// BrowserService handles browser-related operations for Wails integration.
type BrowserService struct {
	config    *config.Config
	dbQueries *db.Queries
}

// NavigationResult represents the result of a navigation operation.
type NavigationResult struct {
	URL           string        `json:"url"`
	Title         string        `json:"title"`
	Success       bool          `json:"success"`
	Error         string        `json:"error,omitempty"`
	LoadTime      time.Duration `json:"load_time"`
	RedirectChain []string      `json:"redirect_chain,omitempty"`
}

// HistoryEntry represents a simplified history entry for frontend.
type HistoryEntry struct {
	ID          int64     `json:"id"`
	URL         string    `json:"url"`
	Title       string    `json:"title"`
	VisitCount  int32     `json:"visit_count"`
	LastVisited time.Time `json:"last_visited"`
	CreatedAt   time.Time `json:"created_at"`
}

// NewBrowserService creates a new BrowserService instance.
func NewBrowserService(cfg *config.Config, queries *db.Queries) *BrowserService {
	return &BrowserService{
		config:    cfg,
		dbQueries: queries,
	}
}

// Navigate handles navigation to a URL and records it in history.
func (s *BrowserService) Navigate(ctx context.Context, url string) (*NavigationResult, error) {
	startTime := time.Now()
	
	if url == "" {
		return &NavigationResult{
			Success: false,
			Error:   "URL cannot be empty",
		}, nil
	}

	// Record the navigation in history
	err := s.recordHistory(ctx, url, "")
	if err != nil {
		// Don't fail navigation if history recording fails
		fmt.Printf("Failed to record history: %v\n", err)
	}

	return &NavigationResult{
		URL:      url,
		Success:  true,
		LoadTime: time.Since(startTime),
	}, nil
}

// UpdatePageTitle updates the title of the current page in history.
func (s *BrowserService) UpdatePageTitle(ctx context.Context, url, title string) error {
	if url == "" || title == "" {
		return fmt.Errorf("URL and title cannot be empty")
	}

	// Use AddOrUpdateHistory to update the title
	titleNull := sql.NullString{String: title, Valid: true}
	return s.dbQueries.AddOrUpdateHistory(ctx, url, titleNull)
}

// GetRecentHistory returns recent browser history entries.
func (s *BrowserService) GetRecentHistory(ctx context.Context, limit int) ([]HistoryEntry, error) {
	if limit <= 0 {
		limit = 50 // Default limit
	}

	entries, err := s.dbQueries.GetHistory(ctx, int64(limit))
	if err != nil {
		return nil, err
	}

	result := make([]HistoryEntry, len(entries))
	for i, entry := range entries {
		result[i] = HistoryEntry{
			ID:          entry.ID,
			URL:         entry.Url,
			Title:       entry.Title.String,
			VisitCount:  int32(entry.VisitCount.Int64),
			LastVisited: entry.LastVisited.Time,
			CreatedAt:   entry.CreatedAt.Time,
		}
	}

	return result, nil
}

// SearchHistory searches browser history.
func (s *BrowserService) SearchHistory(ctx context.Context, query string, limit int) ([]HistoryEntry, error) {
	if query == "" {
		return s.GetRecentHistory(ctx, limit)
	}

	if limit <= 0 {
		limit = 50
	}

	urlPattern := sql.NullString{String: "%" + query + "%", Valid: true}
	titlePattern := sql.NullString{String: "%" + query + "%", Valid: true}
	
	entries, err := s.dbQueries.SearchHistory(ctx, urlPattern, titlePattern, int64(limit))
	if err != nil {
		return nil, err
	}

	result := make([]HistoryEntry, len(entries))
	for i, entry := range entries {
		result[i] = HistoryEntry{
			ID:          entry.ID,
			URL:         entry.Url,
			Title:       entry.Title.String,
			VisitCount:  int32(entry.VisitCount.Int64),
			LastVisited: entry.LastVisited.Time,
			CreatedAt:   entry.CreatedAt.Time,
		}
	}

	return result, nil
}

// DeleteHistoryEntry removes a specific history entry.
func (s *BrowserService) DeleteHistoryEntry(ctx context.Context, id int64) error {
	// Note: DeleteHistory method doesn't exist in current schema
	// This would need to be implemented in the database layer
	return fmt.Errorf("delete history not implemented yet")
}

// ClearHistory removes all history entries.
func (s *BrowserService) ClearHistory(ctx context.Context) error {
	// Note: ClearAllHistory method doesn't exist in current schema
	// This would need to be implemented in the database layer
	return fmt.Errorf("clear all history not implemented yet")
}

// GetHistoryStats returns statistics about browser history.
func (s *BrowserService) GetHistoryStats(ctx context.Context) (map[string]interface{}, error) {
	// Get recent entries for basic stats
	recentEntries, err := s.dbQueries.GetHistory(ctx, 1000)
	if err != nil {
		return nil, err
	}

	// Calculate basic statistics
	stats := map[string]interface{}{
		"total_entries": len(recentEntries),
		"recent_count":  len(recentEntries),
	}

	// Add more detailed stats if we have entries
	if len(recentEntries) > 0 {
		stats["oldest_entry"] = recentEntries[len(recentEntries)-1].CreatedAt.Time
		stats["newest_entry"] = recentEntries[0].CreatedAt.Time
	}

	return stats, nil
}

// GetConfig returns the current browser configuration.
func (s *BrowserService) GetConfig(ctx context.Context) (*config.Config, error) {
	return s.config, nil
}

// UpdateConfig updates the browser configuration.
func (s *BrowserService) UpdateConfig(ctx context.Context, newConfig *config.Config) error {
	// In a real implementation, you'd want to validate and persist the config
	s.config = newConfig
	return nil
}

// GetSearchShortcuts returns available search shortcuts.
func (s *BrowserService) GetSearchShortcuts(ctx context.Context) (map[string]config.SearchShortcut, error) {
	return s.config.SearchShortcuts, nil
}

// recordHistory adds or updates a history entry.
func (s *BrowserService) recordHistory(ctx context.Context, url, title string) error {
	// Use AddOrUpdateHistory which handles both cases
	titleNull := sql.NullString{Valid: false}
	if title != "" {
		titleNull = sql.NullString{String: title, Valid: true}
	}
	
	return s.dbQueries.AddOrUpdateHistory(ctx, url, titleNull)
}