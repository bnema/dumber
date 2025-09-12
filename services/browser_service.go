// Package services contains application services that orchestrate business logic.
package services

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"math"
	neturl "net/url"
	"time"

	"github.com/bnema/dumber/internal/cache"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/db"
	"github.com/bnema/dumber/pkg/webkit"
)

func clampToInt32(i int64) int32 {
	if i > math.MaxInt32 {
		i = math.MaxInt32
	}
	if i < math.MinInt32 {
		i = math.MinInt32
	}
	return int32(i) //nolint:gosec // value is clamped to int32 bounds
}

// WindowTitleUpdater interface allows the service to update the window title
// Decoupled from any specific GUI framework.
type WindowTitleUpdater interface {
	SetTitle(title string)
}

// BrowserService handles browser-related operations for the built-in browser.
type BrowserService struct {
	config             *config.Config
	dbQueries          db.DatabaseQuerier
	windowTitleUpdater WindowTitleUpdater
	webView            *webkit.WebView
	initialURL         string
	injectableScript   string
}

// ServiceName returns the service name for frontend binding
func (s *BrowserService) ServiceName() string {
	return "BrowserService"
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
func NewBrowserService(cfg *config.Config, queries db.DatabaseQuerier) *BrowserService {
	return &BrowserService{
		config:             cfg,
		dbQueries:          queries,
		windowTitleUpdater: nil,
		webView:            nil,
		initialURL:         "",
		injectableScript:   "",
	}
}

// SetWindowTitleUpdater sets the window title updater interface
func (s *BrowserService) SetWindowTitleUpdater(updater WindowTitleUpdater) {
	s.windowTitleUpdater = updater
}

// AttachWebView connects a native WebKit WebView to this service for integration.
func (s *BrowserService) AttachWebView(view *webkit.WebView) {
	s.webView = view
}

// LoadInjectableScript loads the minified injectable controls script from assets
func (s *BrowserService) LoadInjectableScript(assets embed.FS) error {
	scriptBytes, err := assets.ReadFile("frontend/dist/injected-controls.min.js")
	if err != nil {
		return fmt.Errorf("failed to load injectable script: %w", err)
	}
	s.injectableScript = string(scriptBytes)
	return nil
}

// InjectControlScript injects the global controls script into the current page
func (s *BrowserService) InjectControlScript(ctx context.Context) error {
	_ = ctx
	if s.webView == nil {
		return fmt.Errorf("webview not attached")
	}
	if s.injectableScript == "" {
		return fmt.Errorf("injectable script not loaded")
	}
	wrappedScript := "(function(){try{" +
		"if(document.readyState==='loading'){document.addEventListener('DOMContentLoaded',function(){try{" +
		s.injectableScript +
		"}catch(e){console.warn('Dumber Browser controls failed to load on DOMContentLoaded:', e);}});}else{" +
		s.injectableScript +
		"}}catch(e){console.warn('Dumber Browser controls injection failed:', e);}})();"
	return s.webView.InjectScript(wrappedScript)
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

	// Store the initial URL for frontend synchronization
	if s.initialURL == "" {
		s.initialURL = url
	}

	// Record the navigation in history
	err := s.recordHistory(ctx, url, "")
	if err != nil {
		// Don't fail navigation if history recording fails
		fmt.Printf("Failed to record history: %v\n", err)
	}

	// Update favicon asynchronously after navigation
	go s.updateFavicon(ctx, url)

	return &NavigationResult{
		URL:      url,
		Success:  true,
		LoadTime: time.Since(startTime),
	}, nil
}

// UpdatePageTitle updates the title of the current page in history and the window title.
func (s *BrowserService) UpdatePageTitle(ctx context.Context, url, title string) error {
	if url == "" || title == "" {
		return fmt.Errorf("URL and title cannot be empty")
	}

	// Use AddOrUpdateHistory to update the title
	titleNull := sql.NullString{String: title, Valid: true}
	err := s.dbQueries.AddOrUpdateHistory(ctx, url, titleNull)
	if err != nil {
		return err
	}

	// Update the window title if we have a title updater
	if s.windowTitleUpdater != nil && title != "" {
		windowTitle := fmt.Sprintf("Dumber - %s", title)
		s.windowTitleUpdater.SetTitle(windowTitle)
	}

	return nil
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
		// Defensive cast to int32 to prevent overflow
		vc := clampToInt32(entry.VisitCount.Int64)
		result[i] = HistoryEntry{
			ID:          entry.ID,
			URL:         entry.Url,
			Title:       entry.Title.String,
			VisitCount:  vc,
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
		vc := clampToInt32(entry.VisitCount.Int64)
		result[i] = HistoryEntry{
			ID:          entry.ID,
			URL:         entry.Url,
			Title:       entry.Title.String,
			VisitCount:  vc,
			LastVisited: entry.LastVisited.Time,
			CreatedAt:   entry.CreatedAt.Time,
		}
	}

	return result, nil
}

// DeleteHistoryEntry removes a specific history entry.
func (s *BrowserService) DeleteHistoryEntry(ctx context.Context, id int64) error {
	_ = ctx
	_ = id
	// Note: DeleteHistory method doesn't exist in current schema
	// This would need to be implemented in the database layer
	return fmt.Errorf("delete history not implemented yet")
}

// ClearHistory removes all history entries.
func (s *BrowserService) ClearHistory(ctx context.Context) error {
	_ = ctx
	// Note: ClearAllHistory method doesn't exist in current schema
	// This would need to be implemented in the database layer
	return fmt.Errorf("clear all history not implemented yet")
}

// GetHistoryStats returns statistics about browser history.
func (s *BrowserService) GetHistoryStats(ctx context.Context) (map[string]interface{}, error) {
	// Get recent entries for basic stats
	recentEntries, err := s.dbQueries.GetHistory(ctx, recentHistoryLimit)
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
	_ = ctx
	return s.config, nil
}

// UpdateConfig updates the browser configuration.
func (s *BrowserService) UpdateConfig(ctx context.Context, newConfig *config.Config) error {
	_ = ctx
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

// Firefox zoom levels: 30%, 50%, 67%, 80%, 90%, 100%, 110%, 120%, 133%, 150%, 170%, 200%, 240%, 300%, 400%, 500%
var firefoxZoomLevels = []float64{0.3, 0.5, 0.67, 0.8, 0.9, 1.0, 1.1, 1.2, 1.33, 1.5, 1.7, 2.0, 2.4, 3.0, 4.0, 5.0}

// ZoomIn increases the zoom level to the next Firefox zoom level.
func (s *BrowserService) ZoomIn(ctx context.Context, url string) (float64, error) {
	currentZoom, err := s.GetZoomLevel(ctx, url)
	if err != nil {
		return 1.0, err
	}

	newZoom := s.getNextZoomLevel(currentZoom, true)
	err = s.SetZoomLevel(ctx, url, newZoom)
	if err != nil {
		return currentZoom, err
	}

	return newZoom, nil
}

// ZoomOut decreases the zoom level to the previous Firefox zoom level.
func (s *BrowserService) ZoomOut(ctx context.Context, url string) (float64, error) {
	currentZoom, err := s.GetZoomLevel(ctx, url)
	if err != nil {
		return 1.0, err
	}

	newZoom := s.getNextZoomLevel(currentZoom, false)
	err = s.SetZoomLevel(ctx, url, newZoom)
	if err != nil {
		return currentZoom, err
	}

	return newZoom, nil
}

// ResetZoom resets the zoom level to 1.0 for the current URL.
func (s *BrowserService) ResetZoom(ctx context.Context, url string) (float64, error) {
	return s.setZoom(ctx, url, 1.0)
}

// GetZoomLevel retrieves the saved zoom level for a URL.
func (s *BrowserService) GetZoomLevel(ctx context.Context, url string) (float64, error) {
	if url == "" {
		return 1.0, nil
	}

	key := zoomKeyFromURL(url)
	zoomLevel, err := s.dbQueries.GetZoomLevel(ctx, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Fallback to exact URL record if present (backward compatibility)
			zl, err2 := s.dbQueries.GetZoomLevel(ctx, url)
			if err2 == nil {
				return zl, nil
			}
			// No zoom setting found, return default
			return 1.0, nil
		}
		return 1.0, err
	}

	return zoomLevel, nil
}

// SetZoomLevel sets the zoom level for a URL.
func (s *BrowserService) SetZoomLevel(ctx context.Context, url string, zoomLevel float64) error {
	if url == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	// Clamp zoom level to reasonable bounds
	if zoomLevel < zoomMin {
		zoomLevel = zoomMin
	} else if zoomLevel > zoomMax {
		zoomLevel = zoomMax
	}

	key := zoomKeyFromURL(url)
	return s.dbQueries.SetZoomLevel(ctx, key, zoomLevel)
}

// GetCurrentURL returns the current URL (this would be implemented by the frontend)
func (s *BrowserService) GetCurrentURL(ctx context.Context) (string, error) {
	if s.webView == nil {
		return "", nil
	}
	return s.webView.GetCurrentURL(), nil
}

// GetInitialURL returns the initially navigated URL for frontend synchronization
func (s *BrowserService) GetInitialURL(ctx context.Context) (string, error) {
	return s.initialURL, nil
}

// CopyCurrentURL copies the current URL to clipboard (frontend-initiated)
func (s *BrowserService) CopyCurrentURL(ctx context.Context, url string) error {
	// This is primarily for logging/recording the copy action
	// The actual clipboard operation is handled by the frontend
	if url != "" {
		// Could record this action in analytics if needed
		fmt.Printf("URL copied to clipboard: %s\n", url)
	}
	return nil
}

// GoBack provides navigation back functionality
func (s *BrowserService) GoBack(ctx context.Context) error {
	if s.webView != nil {
		return s.webView.GoBack()
	}
	fmt.Println("Navigation: Go back")
	return nil
}

// GoForward provides navigation forward functionality
func (s *BrowserService) GoForward(ctx context.Context) error {
	if s.webView != nil {
		return s.webView.GoForward()
	}
	fmt.Println("Navigation: Go forward")
	return nil
}

// getNextZoomLevel returns the next Firefox zoom level in the specified direction
const (
	zoomMin            = 0.25
	zoomMax            = 5.0
	recentHistoryLimit = 1000
)

func (s *BrowserService) getNextZoomLevel(currentZoom float64, zoomIn bool) float64 {
	// Find the closest current zoom level
	closestIndex := 0
	minDiff := 10.0 // Large initial value

	for i, level := range firefoxZoomLevels {
		diff := currentZoom - level
		if diff < 0 {
			diff = -diff // abs value
		}
		if diff < minDiff {
			minDiff = diff
			closestIndex = i
		}
	}

	if zoomIn {
		// Move to next higher zoom level
		if closestIndex < len(firefoxZoomLevels)-1 {
			return firefoxZoomLevels[closestIndex+1]
		}
		return firefoxZoomLevels[closestIndex] // Already at max
	}
	// Move to next lower zoom level
	if closestIndex > 0 {
		return firefoxZoomLevels[closestIndex-1]
	}
	return firefoxZoomLevels[closestIndex] // Already at min
}

// setZoom sets the zoom level to a specific value
func (s *BrowserService) setZoom(ctx context.Context, url string, zoomLevel float64) (float64, error) {
	err := s.SetZoomLevel(ctx, url, zoomLevel)
	if err != nil {
		return 1.0, err
	}
	return zoomLevel, nil
}

// zoomKeyFromURL extracts a stable per-domain key for zoom persistence.
// Uses hostname if present; falls back to the full input URL string.
func zoomKeyFromURL(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Host
}

// extractDomain extracts the domain from a URL for zoom level storage
// Domain-based zoom allows settings to persist across different pages of the same site
// extractDomain is no longer used; zoom is stored per URL for contract parity.

// ZoomKeyForLog exposes the derived zoom key (host or raw URL) for logging from other packages.
func ZoomKeyForLog(raw string) string { return zoomKeyFromURL(raw) }

// updateFavicon fetches and stores favicon URL for a given page URL
func (s *BrowserService) updateFavicon(ctx context.Context, pageURL string) {
	parsedURL, err := neturl.Parse(pageURL)
	if err != nil {
		return // Silently fail for invalid URLs
	}

	// Skip favicon update for localhost, file://, or special schemes
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return
	}
	if parsedURL.Host == "localhost" || parsedURL.Host == "127.0.0.1" {
		return
	}

	// Standard favicon location
	faviconURL := fmt.Sprintf("%s://%s/favicon.ico", parsedURL.Scheme, parsedURL.Host)

	// Update in database
	faviconNullString := sql.NullString{String: faviconURL, Valid: true}
	if err := s.dbQueries.UpdateHistoryFavicon(ctx, faviconNullString, pageURL); err != nil {
		// Silently fail - favicon is not critical
		return
	}

	// Also cache the favicon for dmenu use
	if faviconCache, err := cache.NewFaviconCache(); err == nil {
		faviconCache.CacheAsync(faviconURL)
	}
}
