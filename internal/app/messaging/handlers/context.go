// Package handlers contains domain-specific message handlers for WebView communication.
package handlers

import (
	"context"
	"encoding/json"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/parser"
	"github.com/bnema/dumber/internal/services"
)

// NavigationController interface for URL navigation.
type NavigationController interface {
	NavigateToURL(input string) error
}

// WebViewInjector interface for WebView script injection operations.
type WebViewInjector interface {
	InjectScript(script string) error
	DispatchCustomEvent(eventName string, data interface{}) error
	LoadURL(url string) error
	SetZoom(level float64) error
	ID() uint64
	IsDestroyed() bool
}

// Parser interface for input parsing operations.
type Parser interface {
	ParseInput(ctx context.Context, input string) (*parser.ParseResult, error)
}

// Browser interface for browser service operations used by handlers.
type Browser interface {
	// Navigation
	Navigate(ctx context.Context, url string) (*services.NavigationResult, error)
	GetZoomLevel(ctx context.Context, url string) (float64, error)

	// History
	GetRecentHistory(ctx context.Context, limit int) ([]services.HistoryEntry, error)
	GetRecentHistoryWithOffset(ctx context.Context, limit, offset int) ([]services.HistoryEntry, error)
	GetMostVisited(ctx context.Context, limit int) ([]services.HistoryEntry, error)
	SearchHistory(ctx context.Context, query string, limit int) ([]services.HistoryEntry, error)
	DeleteHistoryEntry(ctx context.Context, id int64) error
	GetHistoryStats(ctx context.Context) (map[string]interface{}, error)
	GetHistoryTimeline(ctx context.Context, limit, offset int) ([]services.TimelineEntry, error)
	SearchHistoryFTS(ctx context.Context, query string, limit int) ([]services.HistoryEntry, error)
	DeleteHistoryLastHour(ctx context.Context) error
	DeleteHistoryLastDay(ctx context.Context) error
	DeleteHistoryLastWeek(ctx context.Context) error
	DeleteHistoryLastMonth(ctx context.Context) error
	ClearAllHistory(ctx context.Context) error
	DeleteHistoryByDomain(ctx context.Context, domain string) error
	GetHistoryAnalytics(ctx context.Context) (*services.HistoryAnalytics, error)
	GetDomainStats(ctx context.Context, limit int) ([]services.DomainStat, error)

	// Favorites
	GetFavorites(ctx context.Context) ([]services.FavoriteEntry, error)
	ToggleFavorite(ctx context.Context, url, title, faviconURL string) (bool, error)
	IsFavorite(ctx context.Context, url string) (bool, error)
	SetFavoriteShortcut(ctx context.Context, favoriteID int64, shortcut int) error
	GetFavoriteByShortcut(ctx context.Context, shortcut int) (*services.FavoriteEntry, error)
	SetFavoriteFolder(ctx context.Context, favoriteID int64, folderID *int64) error

	// Folders
	GetFolders(ctx context.Context) ([]services.FolderEntry, error)
	CreateFolder(ctx context.Context, name, icon string) (*services.FolderEntry, error)
	UpdateFolder(ctx context.Context, id int64, name, icon string) error
	DeleteFolder(ctx context.Context, id int64) error

	// Tags
	GetTags(ctx context.Context) ([]services.TagEntry, error)
	CreateTag(ctx context.Context, name, color string) (*services.TagEntry, error)
	UpdateTag(ctx context.Context, id int64, name, color string) error
	DeleteTag(ctx context.Context, id int64) error
	AssignTag(ctx context.Context, favoriteID, tagID int64) error
	RemoveTag(ctx context.Context, favoriteID, tagID int64) error

	// Config
	GetConfig(ctx context.Context) (*config.Config, error)
	GetSearchShortcuts(ctx context.Context) (map[string]config.SearchShortcut, error)
	GetColorPalettesForMessaging() services.ColorPalettesResponse
	GetBestPrefixMatch(ctx context.Context, query string) string
}

// Context provides shared dependencies for all handlers.
type Context struct {
	ParserService  Parser
	BrowserService Browser
	WebView        WebViewInjector
	NavController  NavigationController
}

// Convenience type aliases for handler return types
type (
	HistoryEntry          = services.HistoryEntry
	TimelineEntry         = services.TimelineEntry
	HistoryAnalytics      = services.HistoryAnalytics
	DomainStat            = services.DomainStat
	FavoriteEntry         = services.FavoriteEntry
	FolderEntry           = services.FolderEntry
	TagEntry              = services.TagEntry
	ColorPalettesResponse = services.ColorPalettesResponse
)

// InjectJSON marshals data and injects it via a JavaScript callback function.
func (c *Context) InjectJSON(callbackName string, data any) error {
	if c.WebView == nil {
		return nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	script := "window." + callbackName + " && window." + callbackName + "(" + string(b) + ")"
	return c.WebView.InjectScript(script)
}

// InjectJSONWithRequestID marshals data and injects it via a JavaScript callback with requestId.
func (c *Context) InjectJSONWithRequestID(callbackName string, data any, requestID string) error {
	if c.WebView == nil {
		return nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var script string
	if requestID != "" {
		script = "window." + callbackName + " && window." + callbackName + "(" + string(b) + ", '" + requestID + "')"
	} else {
		script = "window." + callbackName + " && window." + callbackName + "(" + string(b) + ")"
	}
	return c.WebView.InjectScript(script)
}

// InjectError sends an error message to a JavaScript error callback.
func (c *Context) InjectError(callbackName, message string) error {
	if c.WebView == nil {
		return nil
	}
	escaped, _ := json.Marshal(message)
	script := "window." + callbackName + " && window." + callbackName + "(" + string(escaped) + ")"
	return c.WebView.InjectScript(script)
}

// InjectErrorWithRequestID sends an error with requestId for promise rejection.
func (c *Context) InjectErrorWithRequestID(callbackName, message, requestID string) error {
	if c.WebView == nil {
		return nil
	}
	escaped, _ := json.Marshal(message)
	var script string
	if requestID != "" {
		script = "window." + callbackName + " && window." + callbackName + "(" + string(escaped) + ", '" + requestID + "')"
	} else {
		script = "window." + callbackName + " && window." + callbackName + "(" + string(escaped) + ")"
	}
	return c.WebView.InjectScript(script)
}

// Ctx returns a background context for database operations.
func (c *Context) Ctx() context.Context {
	return context.Background()
}

// IsReady checks if WebView and BrowserService are available.
func (c *Context) IsReady() bool {
	return c.WebView != nil && c.BrowserService != nil
}

// IsWebViewReady checks if WebView is available and not destroyed.
func (c *Context) IsWebViewReady() bool {
	return c.WebView != nil && !c.WebView.IsDestroyed()
}
