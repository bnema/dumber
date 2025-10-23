// Package services contains application services that orchestrate business logic.
package services

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"math"
	neturl "net/url"
	"sync"
	"time"

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

// historyUpdate represents a pending history write operation
type historyUpdate struct {
	url   string
	title sql.NullString
}

// BrowserService handles browser-related operations for the built-in browser.
type BrowserService struct {
	config             *config.Config
	dbQueries          db.DatabaseQuerier
	windowTitleUpdater WindowTitleUpdater
	webView            *webkit.WebView
	initialURL         string
	guiBundle          string
	zoomCache          sync.Map           // In-memory cache: key = domain/URL, value = float64 zoom level
	historyQueue       chan historyUpdate // Queue for batched history writes
	historyFlushDone   chan struct{}      // Signal when history flush is complete
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
	FaviconURL  string    `json:"favicon_url"`
	VisitCount  int32     `json:"visit_count"`
	LastVisited time.Time `json:"last_visited"`
	CreatedAt   time.Time `json:"created_at"`
}

// NewBrowserService creates a new BrowserService instance.
func NewBrowserService(cfg *config.Config, queries db.DatabaseQuerier) *BrowserService {
	s := &BrowserService{
		config:             cfg,
		dbQueries:          queries,
		windowTitleUpdater: nil,
		webView:            nil,
		initialURL:         "",
		guiBundle:          "",
		historyQueue:       make(chan historyUpdate, 100), // Buffer 100 history updates
		historyFlushDone:   make(chan struct{}),
	}

	// Start background batch processor for history writes
	go s.processHistoryQueue()

	return s
}

// SetWindowTitleUpdater sets the window title updater interface
func (s *BrowserService) SetWindowTitleUpdater(updater WindowTitleUpdater) {
	s.windowTitleUpdater = updater
}

// AttachWebView connects a native WebKit WebView to this service for integration.
func (s *BrowserService) AttachWebView(view *webkit.WebView) {
	s.webView = view
	// Favicon handling is now managed by FaviconService
}

// LoadGUIBundle loads the unified GUI bundle from assets
func (s *BrowserService) LoadGUIBundle(assets embed.FS) error {
	log.Printf("[browser] Attempting to load GUI bundle from assets/gui/gui.min.js")
	bundleBytes, err := assets.ReadFile("assets/gui/gui.min.js")
	if err != nil {
		log.Printf("[browser] ERROR: Failed to load GUI bundle: %v", err)
		return fmt.Errorf("failed to load GUI bundle: %w", err)
	}
	s.guiBundle = string(bundleBytes)
	log.Printf("[browser] GUI bundle loaded successfully into browser service, size: %d bytes", len(bundleBytes))
	return nil
}

// GetGUIBundle returns the loaded GUI bundle string
func (s *BrowserService) GetGUIBundle() string {
	return s.guiBundle
}

// InjectToastSystem injects the GUI bundle and initializes the toast system
func (s *BrowserService) InjectToastSystem(ctx context.Context, theme string) error {
	_ = ctx
	if s.webView == nil {
		return fmt.Errorf("webview not attached")
	}
	if s.guiBundle == "" {
		return fmt.Errorf("GUI bundle not loaded")
	}

	// Set theme if provided
	themeScript := ""
	if theme != "" {
		themeScript = fmt.Sprintf("window.__dumber_initial_theme = '%s';", theme)
	}

	// Inject the unified GUI bundle and initialize toast system after DOM ready
	toastScript := s.guiBundle + ";" + themeScript +
		"(function(){" +
		"function initToast(){" +
		"if(window.__dumber_gui && window.__dumber_gui.initializeToast){" +
		"window.__dumber_gui.initializeToast().then(function(){" +
		"console.log('✅ Toast system initialized via GUI bundle');" +
		"}).catch(function(e){console.error('❌ Toast initialization failed:', e);});" +
		"}else{console.error('❌ GUI bundle not properly loaded');}" +
		"}" +
		"if(document.readyState==='loading'){" +
		"document.addEventListener('DOMContentLoaded',initToast);" +
		"}else{initToast();}" +
		"})();"

	return s.webView.InjectScript(toastScript)
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

	// Favicon detection is now handled automatically by WebKit's native favicon database

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
		updater := s.windowTitleUpdater
		if s.webView != nil {
			s.webView.RunOnMainThread(func() {
				updater.SetTitle(windowTitle)
			})
		} else {
			updater.SetTitle(windowTitle)
		}
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
			FaviconURL:  entry.FaviconUrl.String,
			VisitCount:  vc,
			LastVisited: entry.LastVisited.Time,
			CreatedAt:   entry.CreatedAt.Time,
		}
	}

	return result, nil
}

// GetRecentHistoryWithOffset returns recent browser history entries with pagination support.
func (s *BrowserService) GetRecentHistoryWithOffset(ctx context.Context, limit, offset int) ([]HistoryEntry, error) {
	if limit <= 0 {
		limit = 20 // Default limit for pagination
	}
	if offset < 0 {
		offset = 0
	}

	entries, err := s.dbQueries.GetHistoryWithOffset(ctx, int64(limit), int64(offset))
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
			FaviconURL:  entry.FaviconUrl.String,
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
		// For omnibox display, convert favicon URI to file path when available
		faviconURL := entry.FaviconUrl.String
		result[i] = HistoryEntry{
			ID:          entry.ID,
			URL:         entry.Url,
			Title:       entry.Title.String,
			FaviconURL:  faviconURL,
			VisitCount:  vc,
			LastVisited: entry.LastVisited.Time,
			CreatedAt:   entry.CreatedAt.Time,
		}
	}

	return result, nil
}

// DeleteHistoryEntry removes a specific history entry.
func (s *BrowserService) DeleteHistoryEntry(ctx context.Context, id int64) error {
	return s.dbQueries.DeleteHistory(ctx, id)
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
// Now queues the update for batched processing instead of immediate DB write.
func (s *BrowserService) recordHistory(ctx context.Context, url, title string) error {
	titleNull := sql.NullString{Valid: false}
	if title != "" {
		titleNull = sql.NullString{String: title, Valid: true}
	}

	// Queue the history update (non-blocking with buffer)
	select {
	case s.historyQueue <- historyUpdate{url: url, title: titleNull}:
		// Successfully queued
		return nil
	default:
		// Queue full, write directly (fallback)
		log.Printf("Warning: history queue full, writing directly")
		return s.dbQueries.AddOrUpdateHistory(ctx, url, titleNull)
	}
}

// processHistoryQueue processes batched history writes in the background.
// Flushes every 5 seconds or when a batch reaches 50 items.
func (s *BrowserService) processHistoryQueue() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	batch := make([]historyUpdate, 0, 50)
	const maxBatchSize = 50

	flush := func() {
		if len(batch) == 0 {
			return
		}

		ctx := context.Background()
		// Process all updates in the batch
		// Note: We can't use a transaction with sqlc-generated code easily,
		// but we can still batch the operations to reduce overhead
		for _, update := range batch {
			if err := s.dbQueries.AddOrUpdateHistory(ctx, update.url, update.title); err != nil {
				log.Printf("Warning: failed to write history for %s: %v", update.url, err)
			}
		}

		log.Printf("Flushed %d history updates to database", len(batch))
		batch = batch[:0] // Clear batch
	}

	for {
		select {
		case update, ok := <-s.historyQueue:
			if !ok {
				// Channel closed, flush remaining and exit
				flush()
				close(s.historyFlushDone)
				return
			}
			batch = append(batch, update)
			if len(batch) >= maxBatchSize {
				flush()
			}

		case <-ticker.C:
			flush()
		}
	}
}

// FlushHistoryQueue flushes any pending history writes and stops the processor.
// Call this during graceful shutdown.
func (s *BrowserService) FlushHistoryQueue() {
	close(s.historyQueue)
	<-s.historyFlushDone // Wait for flush to complete
	log.Printf("History queue flushed and closed")
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

// LoadZoomCacheFromDB loads all zoom levels from database into memory cache on startup.
// This eliminates database reads during page transitions.
func (s *BrowserService) LoadZoomCacheFromDB(ctx context.Context) error {
	zoomLevels, err := s.dbQueries.ListZoomLevels(ctx)
	if err != nil {
		return fmt.Errorf("failed to load zoom levels: %w", err)
	}

	loadedCount := 0
	for _, zl := range zoomLevels {
		s.zoomCache.Store(zl.Domain, zl.ZoomFactor)
		loadedCount++
	}

	log.Printf("Loaded %d zoom levels into cache", loadedCount)
	return nil
}

// GetZoomLevel retrieves the saved zoom level for a URL.
// Now reads from in-memory cache instead of database for instant access.
func (s *BrowserService) GetZoomLevel(ctx context.Context, url string) (float64, error) {
	if url == "" {
		return s.config.DefaultZoom, nil
	}

	key := zoomKeyFromURL(url)

	// Check cache first (fast RAM lookup)
	if cachedZoom, ok := s.zoomCache.Load(key); ok {
		if zoom, ok := cachedZoom.(float64); ok {
			return zoom, nil
		}
	}

	// Fallback to exact URL record for backward compatibility (cache miss)
	if key != url {
		if cachedZoom, ok := s.zoomCache.Load(url); ok {
			if zoom, ok := cachedZoom.(float64); ok {
				return zoom, nil
			}
		}
	}

	// No zoom setting found, return configured default
	return s.config.DefaultZoom, nil
}

// SetZoomLevel sets the zoom level for a URL.
// Updates cache immediately (instant) and persists to DB asynchronously.
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

	// Update cache immediately (fast, synchronous)
	s.zoomCache.Store(key, zoomLevel)

	// Persist to database asynchronously (non-blocking)
	go func() {
		bgCtx := context.Background()
		if err := s.dbQueries.SetZoomLevel(bgCtx, key, zoomLevel); err != nil {
			log.Printf("Warning: failed to persist zoom level to DB for %s: %v", key, err)
		}
	}()

	return nil
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

// ColorPalettesResponse holds light and dark palettes for JSON marshaling
type ColorPalettesResponse struct {
	Light config.ColorPalette `json:"light"`
	Dark  config.ColorPalette `json:"dark"`
}

// GetColorPalettesForMessaging returns the color palettes from config
func (s *BrowserService) GetColorPalettesForMessaging() ColorPalettesResponse {
	if s.config == nil {
		return ColorPalettesResponse{}
	}
	return ColorPalettesResponse{
		Light: s.config.Appearance.LightPalette,
		Dark:  s.config.Appearance.DarkPalette,
	}
}
