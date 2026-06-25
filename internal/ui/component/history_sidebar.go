package component

import (
	"context"
	"sync"
	"time"

	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/rs/zerolog"
)

// sidebar constants
const (
	sidebarMinWidth         = 280
	sidebarDefaultWidth     = 320
	sidebarSearchDebounceMs = 150
	sidebarReloadDebounceMs = 200
	sidebarRelativeTickMs   = 60 * 1000
	sidebarPageSize         = 100 // entries fetched per page
	sidebarSearchLimit      = 100 // max FTS search results
)

// HistorySidebarKeyboardAction enumerates the possible activation actions
// triggered by keyboard Enter variants.
type HistorySidebarKeyboardAction int

const (
	// SidebarActionCloseOnActivate is the default activation path.
	// The host currently keeps the sidebar visible for default activation.
	SidebarActionCloseOnActivate HistorySidebarKeyboardAction = iota
	// SidebarActionKeepOpenOnActivate navigates but leaves the sidebar visible.
	SidebarActionKeepOpenOnActivate
	// SidebarActionNewPaneOnActivate navigates by opening the URL in a new pane/split.
	SidebarActionNewPaneOnActivate
)

// HistorySidebar is a GTK sidebar component that displays browsing history
// grouped by day, with search/filter support and keyboard navigation.
// History is loaded page by page with background goroutines to avoid
// blocking the GTK main thread.
type HistorySidebar struct {
	// GTK widgets
	outerBox    *gtk.Box
	searchBox   *gtk.Box
	searchEntry *gtk.SearchEntry
	scrolledWin *gtk.ScrolledWindow
	listBox     *gtk.ListBox

	// Dependencies
	historyUC          port.HistorySidebarHistory
	onURL              func(ctx context.Context, url string)
	onOpenInNewPane    func(ctx context.Context, url string) error
	onNavigateKeepOpen func(ctx context.Context, url string)
	onClose            func()

	// Data
	allEntries  []*entity.HistoryEntry // Flat list, most-recent-first
	groups      []historyGroup         // Current display groups (filtered)
	displayRows []historyDisplayRow    // Explicit rows currently rendered

	// Paging state (browse mode only)
	totalLoaded int // how many entries have been fetched so far
	hasMore     bool
	isLoading   bool
	loadGen     uint64 // incremented each new browse load; used for stale-result protection

	// State
	visible      bool
	currentQuery string
	loadStarted  bool
	loadDone     bool
	destroyed    bool
	mu           sync.RWMutex
	logger       zerolog.Logger

	// Search state
	searchResults []*entity.HistoryEntry // non-nil when a search has completed
	searchGen     uint64                 // incremented each search; used for stale-result protection
	searchDone    bool                   // true when the last search completed
	searchErr     error                  // last search error (if any)

	// Scroll/selection preservation
	prevScrollValue float64
	prevSelectedURL string

	// Search/reload/relative-time timers. GLib source IDs are owned and
	// created/removed on the GTK main context.
	debounceTimer          uint
	reloadDebounceTimer    uint
	relativeTimeTicker     uint
	relativeTimeLabelBinds []relativeTimeLabelBinding
	relativeTimeDayKey     dayKey
	relativeTimeDayKeySet  bool

	// Retained callbacks
	retainedCallbacks []interface{}

	// idleScheduler dispatches work onto the GTK main thread.
	// Tests may override it to exercise scheduled callbacks deterministically.
	idleScheduler func(glib.SourceFunc)
	timeoutAdd    func(uint, glib.SourceFunc) uint
	sourceRemove  func(uint)
	now           func() time.Time

	// Context
	ctx    context.Context
	cancel context.CancelFunc
}

// HistorySidebarConfig holds configuration for creating a HistorySidebar.
type HistorySidebarConfig struct {
	// HistoryUC provides history query and delete operations.
	HistoryUC port.HistorySidebarHistory

	// OnNavigate is called when the user activates a history entry.
	// The default Enter / click behavior keeps the sidebar open after navigating.
	OnNavigate func(ctx context.Context, url string) error

	// OnOpenInNewPane is called when Shift+Enter activates a URL.
	// Should open the URL in a new pane/split. If nil, Shift+Enter
	// falls back to the default plain-Enter behavior.
	OnOpenInNewPane func(ctx context.Context, url string) error

	// OnNavigateKeepOpen is called when Ctrl+Enter activates a URL.
	// Unlike OnNavigate, the sidebar must NOT be closed after this
	// callback returns. If nil, Ctrl+Enter falls back to OnNavigate.
	OnNavigateKeepOpen func(ctx context.Context, url string) error

	// OnClose is called when the sidebar should close itself (e.g. Escape
	// with empty search). The host should hide the sidebar and restore
	// focus to the active content pane/webview.
	OnClose func()
}

// NewHistorySidebar creates a new HistorySidebar component.
func NewHistorySidebar(ctx context.Context, cfg HistorySidebarConfig) *HistorySidebar {
	ctx, cancel := context.WithCancel(ctx)
	log := logging.FromContext(ctx).With().Str("component", "history-sidebar").Logger()

	hs := &HistorySidebar{
		historyUC: cfg.HistoryUC,
		onURL: func(callCtx context.Context, url string) {
			if cfg.OnNavigate != nil {
				if err := cfg.OnNavigate(callCtx, url); err != nil {
					log.Error().Err(err).Str("url", url).Msg("history sidebar navigate failed")
				}
			}
		},
		onOpenInNewPane: func(callCtx context.Context, url string) error {
			if cfg.OnOpenInNewPane == nil {
				// Fall back to regular navigation when no new-pane handler set
				if cfg.OnNavigate != nil {
					return cfg.OnNavigate(callCtx, url)
				}
				return nil
			}
			return cfg.OnOpenInNewPane(callCtx, url)
		},
		onNavigateKeepOpen: func(callCtx context.Context, url string) {
			if cfg.OnNavigateKeepOpen != nil {
				if err := cfg.OnNavigateKeepOpen(callCtx, url); err != nil {
					log.Error().Err(err).Str("url", url).Msg("history sidebar keep-open navigate failed")
				}
			} else if cfg.OnNavigate != nil {
				// Fall back to the default navigate path when no dedicated
				// keep-open callback is configured.
				if err := cfg.OnNavigate(callCtx, url); err != nil {
					log.Error().Err(err).Str("url", url).Msg("history sidebar keep-open fallback failed")
				}
			}
		},
		onClose: func() {
			if cfg.OnClose != nil {
				cfg.OnClose()
			}
		},
		logger:  log,
		ctx:     ctx,
		cancel:  cancel,
		hasMore: cfg.HistoryUC != nil,
	}

	if err := hs.createWidgets(); err != nil {
		log.Error().Err(err).Msg("failed to create history sidebar widgets")
		cancel()
		return nil
	}

	hs.setupSearchHandler()
	hs.setupScrollLoadMore()
	hs.setupKeyboardNavigation()

	log.Debug().Msg("history sidebar created")

	// Start loading history asynchronously (background goroutine)
	hs.startLoadHistory()

	return hs
}

// Destroy cleans up the sidebar and releases resources.
func (hs *HistorySidebar) Destroy() {
	hs.mu.Lock()
	if hs.destroyed {
		hs.mu.Unlock()
		return
	}
	hs.destroyed = true
	timerID := hs.debounceTimer
	hs.debounceTimer = 0
	reloadTimerID := hs.cancelReloadDebounceLocked()
	tickerID := hs.relativeTimeTicker
	hs.relativeTimeTicker = 0
	hs.clearRelativeTimeBindingsLocked()
	hs.mu.Unlock()

	if hs.cancel != nil {
		hs.cancel()
	}

	if timerID != 0 {
		hs.removeSource(timerID)
	}
	if reloadTimerID != 0 {
		hs.removeSource(reloadTimerID)
	}
	if tickerID != 0 {
		hs.removeSource(tickerID)
	}
}

// WidgetAsLayout returns the sidebar's outer widget for embedding.
func (hs *HistorySidebar) WidgetAsLayout(factory layout.WidgetFactory) layout.Widget {
	if hs.outerBox == nil {
		return nil
	}
	return factory.WrapWidget(&hs.outerBox.Widget)
}

// Widget returns the raw GTK widget.
func (hs *HistorySidebar) Widget() *gtk.Widget {
	if hs.outerBox == nil {
		return nil
	}
	return &hs.outerBox.Widget
}

// Show displays the sidebar and focuses the search entry.
// History data is refreshed asynchronously so Ctrl+H always shows
// current recent visits, not stale data from initialization.
func (hs *HistorySidebar) Show() {
	hs.mu.Lock()
	if hs.outerBox == nil || hs.destroyed {
		hs.mu.Unlock()
		return
	}

	reloadTimerID := hs.reloadDebounceTimer
	hs.reloadDebounceTimer = 0
	hs.outerBox.SetVisible(true)
	hs.visible = true
	hs.mu.Unlock()

	if reloadTimerID != 0 {
		hs.removeSource(reloadTimerID)
	}
	hs.startRelativeTimeTicker()

	// Schedule a background reload so the sidebar shows fresh data
	// when it becomes visible, not stale data captured at init time.
	reloadCb := glib.SourceFunc(func(uintptr) bool {
		hs.mu.RLock()
		destroyed := hs.destroyed
		hs.mu.RUnlock()
		if destroyed {
			return false
		}
		hs.Reload()
		return false
	})
	hs.scheduleIdle(reloadCb)

	// Focus search entry via idle callback to ensure layout is stable.
	cb := glib.SourceFunc(func(uintptr) bool {
		hs.mu.RLock()
		destroyed := hs.destroyed
		entry := hs.searchEntry
		hs.mu.RUnlock()
		if destroyed || entry == nil {
			return false
		}
		entry.GrabFocus()
		return false
	})
	hs.scheduleIdle(cb)
}

// Hide hides the sidebar.
func (hs *HistorySidebar) Hide() {
	hs.mu.Lock()
	if hs.destroyed {
		hs.mu.Unlock()
		return
	}

	reloadTimerID := hs.cancelReloadDebounceLocked()
	tickerID := hs.relativeTimeTicker
	hs.relativeTimeTicker = 0
	hs.clearRelativeTimeBindingsLocked()
	if hs.outerBox != nil {
		hs.outerBox.SetVisible(false)
	}
	hs.visible = false
	hs.mu.Unlock()

	if reloadTimerID != 0 {
		hs.removeSource(reloadTimerID)
	}
	if tickerID != 0 {
		hs.removeSource(tickerID)
	}
}

// IsVisible returns whether the sidebar is visible.
func (hs *HistorySidebar) IsVisible() bool {
	hs.mu.RLock()
	defer hs.mu.RUnlock()
	return hs.visible
}

// Reload triggers a fresh load of history data. Preserves the current search
// query, scroll position, and selection.
func (hs *HistorySidebar) Reload() {
	hs.mu.Lock()
	if hs.destroyed {
		hs.mu.Unlock()
		return
	}
	hs.preserveScrollAndSelection()
	savedQuery := hs.currentQuery

	// Reset all data state but keep the query preserved so the search entry
	// text and internal state remain consistent.
	hs.loadDone = false
	hs.loadStarted = false
	hs.totalLoaded = 0
	hs.hasMore = hs.historyUC != nil
	hs.isLoading = false
	hs.allEntries = nil
	hs.setDisplayGroupsLocked(nil)
	hs.searchResults = nil
	hs.searchDone = false
	hs.searchErr = nil

	if savedQuery == "" {
		hs.currentQuery = ""
		hs.mu.Unlock()
		hs.startLoadHistory()
	} else {
		hs.currentQuery = savedQuery
		hs.searchGen++ // Invalidate any in-flight search
		gen := hs.searchGen
		hs.mu.Unlock()
		hs.scheduleClearList()
		hs.doFTSearch(savedQuery, gen)
	}
}

// SetSearchQuery externally sets the search text and triggers filtering.
func (hs *HistorySidebar) SetSearchQuery(query string) {
	hs.mu.RLock()
	entry := hs.searchEntry
	hs.mu.RUnlock()
	if entry != nil {
		entry.SetText(query)
	}
}

// ClearSearch clears the search entry text.
func (hs *HistorySidebar) ClearSearch() {
	hs.SetSearchQuery("")
}

// =============================================================================
// Helpers
// =============================================================================

func safeSidebarString(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
