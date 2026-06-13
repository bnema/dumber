package component

import (
	"context"
	"fmt"
	"sync"

	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/bnema/puregotk/v4/pango"

	"github.com/bnema/dumber/internal/application/dto"
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
	allEntries []*entity.HistoryEntry // Flat list, most-recent-first
	groups     []historyGroup         // Current display groups (filtered)

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

	// Search debounce timer
	debounceTimer uint

	// Retained callbacks
	retainedCallbacks []interface{}

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
	hs.mu.Unlock()

	if hs.cancel != nil {
		hs.cancel()
	}

	if timerID != 0 {
		glib.SourceRemove(timerID)
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
	defer hs.mu.Unlock()

	if hs.outerBox == nil || hs.destroyed {
		return
	}

	hs.outerBox.SetVisible(true)
	hs.visible = true

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
	glib.IdleAdd(&reloadCb, 0)

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
	glib.IdleAdd(&cb, 0)
}

// Hide hides the sidebar.
func (hs *HistorySidebar) Hide() {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	if hs.outerBox == nil || hs.destroyed {
		return
	}

	hs.outerBox.SetVisible(false)
	hs.visible = false
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
	hs.groups = nil
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
// Widget creation
// =============================================================================

func (hs *HistorySidebar) createWidgets() error {
	if err := hs.initOuterBox(); err != nil {
		return err
	}
	if err := hs.initSearchBox(); err != nil {
		return err
	}
	if err := hs.initListArea(); err != nil {
		return err
	}
	return nil
}

func (hs *HistorySidebar) initOuterBox() error {
	hs.outerBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if hs.outerBox == nil {
		return fmt.Errorf("history sidebar: outer box creation failed")
	}
	hs.outerBox.AddCssClass("history-sidebar-outer")
	hs.outerBox.SetSizeRequest(sidebarMinWidth, -1)
	hs.outerBox.SetHexpand(false)
	hs.outerBox.SetVexpand(true)
	hs.outerBox.SetVisible(false)
	return nil
}

func (hs *HistorySidebar) initSearchBox() error {
	hs.searchBox = gtk.NewBox(gtk.OrientationHorizontalValue, 4)
	if hs.searchBox == nil {
		return fmt.Errorf("history sidebar: search box creation failed")
	}
	hs.searchBox.AddCssClass("history-sidebar-search-box")
	hs.searchBox.SetHexpand(true)

	hs.searchEntry = gtk.NewSearchEntry()
	if hs.searchEntry == nil {
		return fmt.Errorf("history sidebar: search entry creation failed")
	}
	hs.searchEntry.AddCssClass("history-sidebar-search")
	hs.searchEntry.SetHexpand(true)
	placeholder := "Search history..."
	hs.searchEntry.SetPlaceholderText(&placeholder)

	hs.searchBox.Append(&hs.searchEntry.Widget)
	hs.outerBox.Append(&hs.searchBox.Widget)
	return nil
}

func (hs *HistorySidebar) initListArea() error {
	hs.scrolledWin = gtk.NewScrolledWindow()
	if hs.scrolledWin == nil {
		return fmt.Errorf("history sidebar: scrolled window creation failed")
	}
	hs.scrolledWin.SetVexpand(true)
	hs.scrolledWin.SetHexpand(true)
	hs.scrolledWin.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	hs.scrolledWin.AddCssClass("history-sidebar-groups")

	hs.listBox = gtk.NewListBox()
	if hs.listBox == nil {
		return fmt.Errorf("history sidebar: list box creation failed")
	}
	hs.listBox.AddCssClass("history-sidebar-groups")
	hs.listBox.SetActivateOnSingleClick(true)
	hs.listBox.SetSelectionMode(gtk.SelectionSingleValue)

	// Connect row activation (Enter or double-click)
	rowActivatedCb := func(_ gtk.ListBox, rowPtr uintptr) {
		row := gtk.ListBoxRowNewFromInternalPtr(rowPtr)
		if row == nil {
			return
		}
		hs.onRowActivated(row)
	}
	hs.retainedCallbacks = append(hs.retainedCallbacks, rowActivatedCb)
	hs.listBox.ConnectRowActivated(&rowActivatedCb)

	hs.scrolledWin.SetChild(&hs.listBox.Widget)
	hs.outerBox.Append(&hs.scrolledWin.Widget)
	return nil
}

// =============================================================================
// Data loading — background goroutine with paging
// =============================================================================

func (hs *HistorySidebar) startLoadHistory() {
	hs.mu.Lock()
	hs.loadGen++
	gen := hs.loadGen
	hs.loadStarted = true
	hs.isLoading = true
	hs.mu.Unlock()

	// Fetch first page in a background goroutine
	go hs.fetchPage(0, gen)
}

// fetchPage fetches a page of history entries in a background goroutine
// and schedules the UI update on the GTK main thread.
func (hs *HistorySidebar) fetchPage(offset int, gen uint64) {
	hs.mu.RLock()
	uc := hs.historyUC
	ctx := hs.ctx
	hs.mu.RUnlock()

	if uc == nil || ctx == nil {
		// No provider; show empty state
		cb := glib.SourceFunc(func(uintptr) bool {
			hs.mu.Lock()
			if hs.destroyed {
				hs.mu.Unlock()
				return false
			}
			hs.loadStarted = false
			hs.isLoading = false
			hs.loadDone = true
			hs.hasMore = false
			hs.mu.Unlock()
			hs.scheduleRebuild()
			return false
		})
		glib.IdleAdd(&cb, 0)
		return
	}

	entries, err := uc.GetRecent(ctx, sidebarPageSize, offset)
	if err != nil {
		hs.logger.Error().Err(err).Int("offset", offset).Msg("failed to load history page")
	}

	if entries == nil {
		entries = []*entity.HistoryEntry{}
	}

	hasMore := len(entries) >= sidebarPageSize

	hs.mu.Lock()

	// If a newer load was started since this fetch began, drop stale results.
	// Must NOT mutate isLoading/loadStarted — they belong to the current
	// generation set by startLoadHistory or LoadMore.
	if gen != hs.loadGen {
		hs.mu.Unlock()
		return
	}

	// If search is active, don't update browse state with stale page data
	// and don't overwrite search results.
	if hs.currentQuery != "" {
		hs.isLoading = false
		hs.loadStarted = false
		hs.mu.Unlock()
		return
	}

	hs.totalLoaded = offset + len(entries)
	hs.hasMore = hasMore
	hs.isLoading = false
	hs.loadStarted = false
	hs.loadDone = true

	if offset == 0 {
		// First page: replace all entries
		hs.allEntries = entries
	} else {
		// Subsequent page: append
		hs.allEntries = append(hs.allEntries, entries...)
	}

	// Group for display
	hs.groups = groupHistoryByDay(hs.allEntries)
	hs.mu.Unlock()

	// Schedule UI rebuild on GTK main thread
	cb := glib.SourceFunc(func(uintptr) bool {
		hs.rebuildList()
		return false
	})
	glib.IdleAdd(&cb, 0)
}

// LoadMore fetches the next page and appends it to the existing entries.
func (hs *HistorySidebar) LoadMore() {
	hs.mu.Lock()
	if hs.isLoading || !hs.hasMore || hs.destroyed || hs.currentQuery != "" {
		hs.mu.Unlock()
		return
	}
	hs.isLoading = true
	offset := hs.totalLoaded
	gen := hs.loadGen
	hs.mu.Unlock()

	hs.logger.Debug().Int("offset", offset).Msg("loading more history entries")
	go hs.fetchPage(offset, gen)
}

// =============================================================================
// Scroll-aware load-more: detects when the user reaches the bottom
// =============================================================================

func (hs *HistorySidebar) setupScrollLoadMore() {
	if hs.scrolledWin == nil {
		return
	}

	vadj := hs.scrolledWin.GetVadjustment()
	if vadj == nil {
		return
	}

	changedCb := func(_ gtk.Adjustment) {
		hs.mu.RLock()
		if hs.destroyed || !hs.hasMore || hs.isLoading {
			hs.mu.RUnlock()
			return
		}
		value := vadj.GetValue()
		upper := vadj.GetUpper()
		pageSize := vadj.GetPageSize()
		hs.mu.RUnlock()

		// Trigger load-more when within 200px of the bottom
		if pageSize > 0 && value+pageSize >= upper-200.0 {
			hs.LoadMore()
		}
	}
	hs.retainedCallbacks = append(hs.retainedCallbacks, changedCb)
	vadj.ConnectValueChanged(&changedCb)
}

// =============================================================================
// Scroll/selection preservation
// =============================================================================

// preserveScrollAndSelection saves the current scroll position and selected row
// URL before a rebuild. Must be called with hs.mu write lock held.
func (hs *HistorySidebar) preserveScrollAndSelection() {
	hs.prevScrollValue = 0
	hs.prevSelectedURL = ""

	if hs.scrolledWin != nil {
		if vadj := hs.scrolledWin.GetVadjustment(); vadj != nil {
			hs.prevScrollValue = vadj.GetValue()
		}
	}
	if hs.listBox != nil {
		if selected := hs.listBox.GetSelectedRow(); selected != nil {
			if url := hs.entryURLAtIndex(selected.GetIndex()); url != "" {
				hs.prevSelectedURL = url
			}
		}
	}
}

// restoreScrollAndSelection restores the previously saved scroll position and
// selection after a rebuild. Called on the GTK main thread.
func (hs *HistorySidebar) restoreScrollAndSelection() {
	// Restore selection first (changes scroll position)
	if hs.prevSelectedURL != "" {
		hs.selectRowByURL(hs.prevSelectedURL)
	}

	// Then restore scroll position if we have one
	if hs.prevScrollValue > 0 && hs.scrolledWin != nil {
		if vadj := hs.scrolledWin.GetVadjustment(); vadj != nil {
			maxVal := vadj.GetUpper() - vadj.GetPageSize()
			if hs.prevScrollValue > maxVal {
				hs.prevScrollValue = maxVal
			}
			if hs.prevScrollValue >= 0 {
				vadj.SetValue(hs.prevScrollValue)
			}
		}
	}

	hs.prevScrollValue = 0
	hs.prevSelectedURL = ""
}

// getRowURL extracts the URL stored in a list box row.
func (hs *HistorySidebar) getRowURL(row *gtk.ListBoxRow) string {
	if row == nil || !row.GetSelectable() {
		return ""
	}
	child := row.GetChild()
	if child == nil {
		return ""
	}

	// The child is the vertical box. Walk children to find our stored URL.
	// We store the URL directly on the row as data.
	// Actually, let's use a simpler approach: walk the list box to find the entry.
	idx := row.GetIndex()
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	return hs.entryURLAtIndex(idx)
}

// entryURLAtIndex returns the URL of the history entry at the given
// linear list index (including group headers which return "").
func (hs *HistorySidebar) entryURLAtIndex(index int) string {
	return newKeyboardNavModel(hs.groups).entryURLAt(index)
}

// selectRowByURL finds and selects a row whose URL matches.
func (hs *HistorySidebar) selectRowByURL(url string) {
	if url == "" || hs.listBox == nil {
		return
	}
	for i := 0; ; i++ {
		row := hs.listBox.GetRowAtIndex(i)
		if row == nil {
			break
		}
		if !row.GetSelectable() {
			continue
		}
		if hs.getRowURL(row) == url {
			hs.listBox.SelectRow(row)
			return
		}
	}
}

// =============================================================================
// Search / filtering
// =============================================================================

func (hs *HistorySidebar) setupSearchHandler() {
	if hs.searchEntry == nil {
		return
	}

	changedCb := func(_ gtk.SearchEntry) {
		hs.onSearchChanged()
	}
	hs.retainedCallbacks = append(hs.retainedCallbacks, changedCb)
	hs.searchEntry.ConnectSearchChanged(&changedCb)
}

func (hs *HistorySidebar) onSearchChanged() {
	hs.mu.Lock()
	if hs.destroyed {
		hs.mu.Unlock()
		return
	}
	hs.currentQuery = hs.searchEntry.GetText()
	hs.preserveScrollAndSelection()
	oldTimer := hs.debounceTimer
	hs.debounceTimer = 0
	hs.mu.Unlock()

	if oldTimer != 0 {
		glib.SourceRemove(oldTimer)
	}

	filterCb := glib.SourceFunc(func(uintptr) bool {
		hs.applyFilter()
		return false
	})
	timerID := glib.TimeoutAdd(uint(sidebarSearchDebounceMs), &filterCb, 0)

	hs.mu.Lock()
	if hs.destroyed {
		hs.mu.Unlock()
		if timerID != 0 {
			glib.SourceRemove(timerID)
		}
		return
	}
	hs.debounceTimer = timerID
	hs.mu.Unlock()
}

func (hs *HistorySidebar) applyFilter() {
	hs.mu.Lock()
	hs.debounceTimer = 0
	query := hs.currentQuery

	if query == "" {
		// Empty query: use in-memory browse entries (paged getRecent).
		// Clear search state and invalidate any in-flight search so a late
		// search result doesn't overwrite browse state.
		hs.searchResults = nil
		hs.searchDone = false
		hs.searchGen++
		hs.groups = nil
		if !hs.loadDone {
			// Browse was never fully loaded (e.g., a search superseded the
			// initial page fetch). Clear the list, show a loading indicator,
			// and restart loading history in the background.
			hs.mu.Unlock()
			hs.scheduleRebuild() // Shows "Loading history…" while fetch runs
			hs.startLoadHistory()
			return
		}
		hs.groups = groupHistoryByDay(hs.allEntries)
		hs.mu.Unlock()
		hs.scheduleRebuild()
		return
	}

	// Non-empty query: use real FTS search via the provider.
	// Cancel any stale in-flight search via generation counter.
	hs.searchGen++
	gen := hs.searchGen
	hs.searchDone = false
	hs.searchResults = nil
	hs.groups = nil
	hs.mu.Unlock()

	// Clear the list immediately to avoid showing stale browse results
	// while the search is in flight.
	hs.scheduleClearList()

	hs.doFTSearch(query, gen)
}

// doFTSearch runs a history FTS search in a background goroutine and
// updates the display when results arrive. Stale results (from a superseded
// search generation) are silently dropped.
func (hs *HistorySidebar) doFTSearch(query string, gen uint64) {
	hs.mu.RLock()
	uc := hs.historyUC
	hs.mu.RUnlock()

	if uc == nil {
		return
	}

	go func() {
		out, err := uc.Search(hs.ctx, dto.HistorySearchInput{Query: query, Limit: sidebarSearchLimit})
		var entries []*entity.HistoryEntry
		if out != nil {
			entries = make([]*entity.HistoryEntry, len(out.Matches))
			for i, m := range out.Matches {
				entries[i] = m.Entry
			}
		}
		if err != nil {
			hs.logger.Error().Err(err).Str("query", query).Msg("history FTS search failed")
		}
		if entries == nil {
			entries = []*entity.HistoryEntry{}
		}

		// Apply results on the GTK main thread with stale-result protection
		cb := glib.SourceFunc(func(uintptr) bool {
			if hs.applySearchResults(entries, gen, err) {
				hs.scheduleRebuild()
			}
			return false
		})
		glib.IdleAdd(&cb, 0)
	}()
}

// applySearchResults applies search results under the generation guard.
// Returns true if results were applied (non-stale), false if the generation
// had moved on and the results were dropped.
func (hs *HistorySidebar) applySearchResults(entries []*entity.HistoryEntry, gen uint64, err error) bool {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	if hs.destroyed || gen != hs.searchGen {
		return false
	}
	hs.searchResults = entries
	hs.searchDone = true
	if err != nil {
		hs.searchErr = err
	}
	hs.groups = groupHistoryByDay(entries)
	return true
}

// scheduleClearList clears the list box on the GTK main thread.
func (hs *HistorySidebar) scheduleClearList() {
	cb := glib.SourceFunc(func(uintptr) bool {
		hs.mu.RLock()
		destroyed := hs.destroyed
		listBox := hs.listBox
		hs.mu.RUnlock()
		if destroyed || listBox == nil {
			return false
		}
		listBox.RemoveAll()
		return false
	})
	glib.IdleAdd(&cb, 0)
}

// scheduleRebuild schedules a list rebuild on the GTK main thread.
func (hs *HistorySidebar) scheduleRebuild() {
	cb := glib.SourceFunc(func(uintptr) bool {
		hs.rebuildList()
		return false
	})
	glib.IdleAdd(&cb, 0)
}

// =============================================================================
// List rendering
// =============================================================================

// rebuildList clears and repopulates the list box from current groups.
// Must be called on the GTK main thread. Preserves scroll and selection.
func (hs *HistorySidebar) rebuildList() {
	hs.mu.RLock()
	if hs.destroyed || hs.listBox == nil {
		hs.mu.RUnlock()
		return
	}
	groups := hs.groups
	query := hs.currentQuery
	hasSearchResults := hs.searchResults != nil
	totalLoaded := hs.totalLoaded
	hs.mu.RUnlock()

	// Remove all rows
	hs.listBox.RemoveAll()

	if len(groups) == 0 {
		if !hasSearchResults && totalLoaded == 0 {
			// Browse has not loaded yet AND no search has completed.
			hs.showLoadingOrEmpty()
			return
		}
		// Search completed with 0 results, or browse loaded but empty (no history).
		hs.showEmptyState(query)
		hs.restoreScrollAndSelection()
		return
	}

	for _, group := range groups {
		// Group header row
		hs.appendGroupHeader(group.Label)

		// Entry rows
		for _, entry := range group.Entries {
			hs.appendEntryRow(entry)
		}
	}

	hs.listBox.Show()

	// Restore previous scroll position and selection
	hs.restoreScrollAndSelection()

	// If no selection was restored and this is the first load, select first entry
	hs.ensureAtLeastOneSelection()
}

func (hs *HistorySidebar) showLoadingOrEmpty() {
	label := gtk.NewLabel(nil)
	if label == nil {
		return
	}
	label.AddCssClass("history-sidebar-loading")

	hs.mu.RLock()
	isLoading := hs.isLoading
	query := hs.currentQuery
	hs.mu.RUnlock()

	switch {
	case isLoading && query == "":
		label.SetText("Loading history...")
	case query != "":
		label.SetText(fmt.Sprintf("No results for \"%s\"", query))
	default:
		label.SetText("No browsing history")
	}

	label.SetWrap(false)
	label.SetXalign(0.0)

	row := gtk.NewListBoxRow()
	if row == nil {
		return
	}
	row.SetSelectable(false)
	row.SetCanFocus(false)
	row.SetActivatable(false)
	row.SetChild(&label.Widget)
	hs.listBox.Append(&row.Widget)
}

func (hs *HistorySidebar) showEmptyState(query string) {
	label := gtk.NewLabel(nil)
	if label == nil {
		return
	}
	label.AddCssClass("history-sidebar-empty")

	if query != "" {
		label.SetText(fmt.Sprintf("No results for \"%s\"", query))
	} else {
		label.SetText("No browsing history")
	}

	label.SetWrap(false)
	label.SetXalign(0.0)

	row := gtk.NewListBoxRow()
	if row == nil {
		return
	}
	row.SetSelectable(false)
	row.SetCanFocus(false)
	row.SetActivatable(false)
	row.SetChild(&label.Widget)
	hs.listBox.Append(&row.Widget)
}

// appendGroupHeader adds a non-selectable group header label to the list.
func (hs *HistorySidebar) appendGroupHeader(labelText string) {
	label := gtk.NewLabel(&labelText)
	if label == nil {
		return
	}
	label.AddCssClass("history-sidebar-group-header")
	label.SetXalign(0.0)
	label.SetHexpand(true)

	row := gtk.NewListBoxRow()
	if row == nil {
		return
	}
	row.SetSelectable(false)
	row.SetCanFocus(false)
	row.SetActivatable(false)
	row.SetChild(&label.Widget)
	hs.listBox.Append(&row.Widget)
}

// appendEntryRow adds a selectable two-line entry row to the list.
func (hs *HistorySidebar) appendEntryRow(entry *entity.HistoryEntry) {
	// Outer vertical box for two-line layout
	rowBox := gtk.NewBox(gtk.OrientationVerticalValue, 1)
	if rowBox == nil {
		return
	}
	rowBox.SetHexpand(true)

	// Title line (first line)
	titleLabel := gtk.NewLabel(nil)
	if titleLabel == nil {
		return
	}
	titleLabel.AddCssClass("history-sidebar-row-title")
	titleLabel.SetText(safeSidebarString(entry.Title, entry.URL))
	titleLabel.SetXalign(0.0)
	titleLabel.SetHexpand(true)
	titleLabel.SetEllipsize(pango.EllipsizeEndValue)

	// Subtitle line with URL and time
	subBox := gtk.NewBox(gtk.OrientationHorizontalValue, 0)
	if subBox == nil {
		return
	}
	subBox.SetHexpand(true)

	urlLabel := gtk.NewLabel(nil)
	if urlLabel == nil {
		return
	}
	urlLabel.AddCssClass("history-sidebar-row-subtitle")
	urlLabel.SetText(readableURL(entry.URL))
	urlLabel.SetXalign(0.0)
	urlLabel.SetHexpand(true)
	urlLabel.SetEllipsize(pango.EllipsizeEndValue)

	timeLabel := gtk.NewLabel(nil)
	if timeLabel == nil {
		return
	}
	timeLabel.AddCssClass("history-sidebar-row-time")
	timeLabel.SetText(relativeTime(entry.LastVisited))
	timeLabel.SetXalign(1.0)

	subBox.Append(&urlLabel.Widget)
	subBox.Append(&timeLabel.Widget)

	rowBox.Append(&titleLabel.Widget)
	rowBox.Append(&subBox.Widget)

	// Create the list box row
	row := gtk.NewListBoxRow()
	if row == nil {
		return
	}
	row.AddCssClass("history-sidebar-row")
	row.SetSelectable(true)
	row.SetActivatable(true)
	row.SetCanFocus(true)
	row.SetFocusOnClick(true)
	row.SetChild(&rowBox.Widget)

	hs.listBox.Append(&row.Widget)
}

// ensureAtLeastOneSelection selects the first selectable row if nothing is selected.
func (hs *HistorySidebar) ensureAtLeastOneSelection() {
	if hs.listBox.GetSelectedRow() != nil {
		return
	}
	for i := 0; ; i++ {
		row := hs.listBox.GetRowAtIndex(i)
		if row == nil {
			break
		}
		if row.GetSelectable() {
			hs.listBox.SelectRow(row)
			return
		}
	}
}

// =============================================================================
// Keyboard navigation
// =============================================================================

func (hs *HistorySidebar) setupKeyboardNavigation() {
	if hs.outerBox == nil {
		return
	}

	// The ListBox already supports Up/Down arrow navigation natively.
	// We add a PhaseCapture key controller on the outerBox to intercept
	// keys before the ListBox processes them.
	keyController := gtk.NewEventControllerKey()
	if keyController == nil {
		return
	}
	keyController.SetPropagationPhase(gtk.PhaseCaptureValue)

	keyPressedCb := func(_ gtk.EventControllerKey, keyval uint, _ uint, state gdk.ModifierType) bool {
		switch keyval {
		// --- Escape: clear search or close sidebar ---
		case uint(gdk.KEY_Escape):
			if hs.searchEntry != nil && hs.searchEntry.GetText() != "" {
				hs.searchEntry.SetText("")
				return true
			}
			// Close sidebar explicitly and restore focus
			hs.closeSidebar()
			return true

		// --- Enter variants ---
		case uint(gdk.KEY_Return), uint(gdk.KEY_KP_Enter):
			return hs.handleEnterKey(keyval, state)

		// --- Delete: remove selected entry ---
		case uint(gdk.KEY_Delete), uint(gdk.KEY_KP_Delete):
			if hs.searchEntry != nil && hs.searchEntry.GetText() != "" {
				return false
			}
			return hs.handleDeleteKey()

		// --- PageUp / PageDown: scroll by page ---
		case uint(gdk.KEY_Page_Up):
			hs.scrollByPage(-1)
			return true
		case uint(gdk.KEY_Page_Down):
			hs.scrollByPage(1)
			return true

		// --- Home / End: jump to first/last selectable row ---
		case uint(gdk.KEY_Home):
			hs.jumpToFirstSelectable()
			return true
		case uint(gdk.KEY_End):
			hs.jumpToLastSelectable()
			return true

		// --- Up / Down: previous / next selectable row ---
		// Ctrl+Up / Ctrl+Down: previous / next day group jump ---
		case uint(gdk.KEY_Up):
			if state&gdk.ControlMaskValue != 0 {
				hs.jumpToPreviousDay()
				return true
			}
			hs.selectPreviousRow()
			return true
		case uint(gdk.KEY_Down):
			if state&gdk.ControlMaskValue != 0 {
				hs.jumpToNextDay()
				return true
			}
			hs.selectNextRow()
			return true
		}

		return false
	}

	hs.retainedCallbacks = append(hs.retainedCallbacks, keyPressedCb)
	keyController.ConnectKeyPressed(&keyPressedCb)

	hs.outerBox.AddController(&keyController.EventController)
}

// handleEnterKey processes Enter, Ctrl+Enter, and Shift+Enter on a selected row.
// Returns true if the key was consumed.
func (hs *HistorySidebar) handleEnterKey(keyval uint, state gdk.ModifierType) bool {
	// Determine activation mode from modifiers
	var action HistorySidebarKeyboardAction

	switch {
	case state&gdk.ControlMaskValue != 0:
		// Ctrl+Enter: navigate but keep sidebar open
		action = SidebarActionKeepOpenOnActivate
	case state&gdk.ShiftMaskValue != 0:
		// Shift+Enter: navigate in new pane
		action = SidebarActionNewPaneOnActivate
	default:
		// Plain Enter: navigate using the default activation behavior.
		action = SidebarActionCloseOnActivate
	}

	// Find the selected row and its URL
	row := hs.listBox.GetSelectedRow()
	if row == nil || !row.GetSelectable() {
		return false
	}

	hs.mu.RLock()
	url := hs.entryURLAtIndex(row.GetIndex())
	hs.mu.RUnlock()
	if url == "" {
		return false
	}

	// Schedule activation on the GTK main thread
	switch action {
	case SidebarActionKeepOpenOnActivate:
		hs.navigateWithoutClosing(url)
	case SidebarActionNewPaneOnActivate:
		hs.navigateToNewPane(url)
	default:
		hs.navigateToURL(url)
	}

	// Consume the key event
	return true
}

// handleDeleteKey removes the selected history entry and updates the selection.
// Returns true if the key was consumed.
func (hs *HistorySidebar) handleDeleteKey() bool {
	row := hs.listBox.GetSelectedRow()
	if row == nil || !row.GetSelectable() {
		return false
	}
	if hs.historyUC == nil {
		return false
	}

	idx := row.GetIndex()

	hs.mu.RLock()
	url := hs.entryURLAtIndex(idx)
	entryID := hs.findEntryIDByIndex(idx)
	nextSelectedURL := ""
	if nextRow := hs.findNextSelectableAfter(idx); nextRow != -1 {
		nextSelectedURL = hs.entryURLAtIndex(nextRow)
	}
	hs.mu.RUnlock()

	if url == "" || entryID <= 0 {
		return false
	}

	go func() {
		if err := hs.historyUC.Delete(hs.ctx, entryID); err != nil {
			hs.logger.Error().Err(err).Int64("entry_id", entryID).Msg("failed to delete history entry")
			return
		}

		cb := glib.SourceFunc(func(uintptr) bool {
			hs.mu.Lock()
			if hs.destroyed {
				hs.mu.Unlock()
				return false
			}
			hs.preserveScrollAndSelection()
			hs.prevSelectedURL = nextSelectedURL
			hs.searchGen++
			hs.removeFromAllEntries(url, entryID)
			hs.removeFromSearchResults(entryID)
			hs.rebuildLocalGroups()
			hs.mu.Unlock()

			hs.rebuildList()
			return false
		})
		glib.IdleAdd(&cb, 0)
	}()

	return true
}

// findEntryIDByIndex returns the entry ID for the linear ListBox index.
// Must be called with hs.mu read lock held.
func (hs *HistorySidebar) findEntryIDByIndex(index int) int64 {
	entry := newKeyboardNavModel(hs.groups).entryAt(index)
	if entry == nil {
		return 0
	}
	return entry.ID
}

// rebuildLocalGroups rebuilds hs.groups from the current allEntries and query.
// Must be called with hs.mu write lock held.
func (hs *HistorySidebar) rebuildLocalGroups() {
	if hs.currentQuery == "" {
		hs.groups = groupHistoryByDay(hs.allEntries)
	} else if hs.searchResults != nil {
		hs.groups = groupHistoryByDay(hs.searchResults)
	} else {
		// For search mode, the search results are handled by doFTSearch.
		// Removing an entry while in search mode would need a re-search.
		// Fall back to grouping searchResults if they exist.
		hs.groups = nil
	}
}

// removeFromAllEntries removes all history entries matching the given URL or ID
// from hs.allEntries. Must be called with hs.mu write lock held.
func (hs *HistorySidebar) removeFromAllEntries(url string, id int64) {
	filtered := make([]*entity.HistoryEntry, 0, len(hs.allEntries))
	for _, e := range hs.allEntries {
		if e != nil && (e.URL == url || e.ID == id) {
			continue
		}
		filtered = append(filtered, e)
	}
	hs.allEntries = filtered
}

// removeFromSearchResults removes all history entries matching the given ID
// from hs.searchResults. Must be called with hs.mu write lock held.
func (hs *HistorySidebar) removeFromSearchResults(id int64) {
	if hs.searchResults == nil {
		return
	}
	filtered := make([]*entity.HistoryEntry, 0, len(hs.searchResults))
	for _, e := range hs.searchResults {
		if e != nil && e.ID == id {
			continue
		}
		filtered = append(filtered, e)
	}
	hs.searchResults = filtered
}

// findNextSelectableAfter returns the ListBox index of the next selectable
// row after the given index, falling back to the previous selectable row.
// Must be called with hs.mu read lock held.
func (hs *HistorySidebar) findNextSelectableAfter(idx int) int {
	model := newKeyboardNavModel(hs.groups)
	if next := model.nextSelectableIndex(idx, +1); next != -1 {
		return next
	}
	if prev := model.nextSelectableIndex(idx, -1); prev != -1 {
		return prev
	}
	return -1
}

// scrollByPage scrolls the list by one page up or down,
// keeping the selection visible.
func (hs *HistorySidebar) scrollByPage(direction int) {
	if hs.scrolledWin == nil {
		return
	}
	vadj := hs.scrolledWin.GetVadjustment()
	if vadj == nil {
		return
	}
	pageSize := vadj.GetPageSize()
	current := vadj.GetValue()
	newVal := current + float64(direction)*(pageSize*0.9) // 90% page for overlap
	if newVal < 0 {
		newVal = 0
	}
	upper := vadj.GetUpper() - pageSize
	if newVal > upper {
		newVal = upper
	}
	if newVal >= 0 {
		vadj.SetValue(newVal)
	}
}

// jumpToPreviousDay selects the first entry in the previous day group
// relative to the currently selected row.
func (hs *HistorySidebar) jumpToPreviousDay() {
	currentIdx := -1
	if row := hs.listBox.GetSelectedRow(); row != nil {
		currentIdx = row.GetIndex()
	}

	hs.mu.RLock()
	targetIdx := newKeyboardNavModel(hs.groups).previousDayBoundary(currentIdx)
	hs.mu.RUnlock()
	if targetIdx == -1 {
		hs.jumpToFirstSelectable()
		return
	}
	if row := hs.listBox.GetRowAtIndex(targetIdx); row != nil && row.GetSelectable() {
		hs.listBox.SelectRow(row)
		hs.scrollRowIntoView(targetIdx)
		return
	}
	hs.jumpToFirstSelectable()
}

// jumpToNextDay selects the first entry in the next day group
// relative to the currently selected row.
func (hs *HistorySidebar) jumpToNextDay() {
	currentIdx := -1
	if row := hs.listBox.GetSelectedRow(); row != nil {
		currentIdx = row.GetIndex()
	}

	hs.mu.RLock()
	targetIdx := newKeyboardNavModel(hs.groups).nextDayBoundary(currentIdx)
	hs.mu.RUnlock()
	if targetIdx == -1 {
		hs.jumpToLastSelectable()
		return
	}
	if row := hs.listBox.GetRowAtIndex(targetIdx); row != nil && row.GetSelectable() {
		hs.listBox.SelectRow(row)
		hs.scrollRowIntoView(targetIdx)
		return
	}
	hs.jumpToLastSelectable()
}

// scrollRowIntoView scrolls the scrolled window to ensure the row at
// the given ListBox index is visible.
func (hs *HistorySidebar) scrollRowIntoView(index int) {
	hs.ensureRowVisible(index)
}

// jumpToFirstSelectable selects the first selectable row in the list.
func (hs *HistorySidebar) jumpToFirstSelectable() {
	for i := 0; ; i++ {
		row := hs.listBox.GetRowAtIndex(i)
		if row == nil {
			break
		}
		if row.GetSelectable() {
			hs.listBox.SelectRow(row)
			hs.ensureRowVisible(i)
			return
		}
	}
}

// jumpToLastSelectable selects the last selectable row in the list.
func (hs *HistorySidebar) jumpToLastSelectable() {
	// Walk backwards through the rows
	maxIdx := 0
	var lastRow *gtk.ListBoxRow
	for i := 0; ; i++ {
		row := hs.listBox.GetRowAtIndex(i)
		if row == nil {
			break
		}
		maxIdx = i
		if row.GetSelectable() {
			lastRow = row
		}
	}
	// If we found a selectable row, try it. Otherwise fall back to last row.
	if lastRow != nil {
		hs.listBox.SelectRow(lastRow)
		hs.ensureRowVisible(lastRow.GetIndex())
		return
	}
	// Fallback: last row regardless of selectability
	if maxIdx > 0 {
		if row := hs.listBox.GetRowAtIndex(maxIdx); row != nil {
			hs.listBox.SelectRow(row)
			hs.ensureRowVisible(maxIdx)
		}
	}
}

// =============================================================================
// Up/Down row selection (with search entry focus preserved)
// =============================================================================

// selectPreviousRow selects the previous selectable row (skipping headers).
// Focus remains in the search entry; the ListBox selection is updated
// programmatically and scrolled into view.
func (hs *HistorySidebar) selectPreviousRow() {
	hs.selectAdjacentRow(-1)
}

// selectNextRow selects the next selectable row (skipping headers).
// Focus remains in the search entry; the ListBox selection is updated
// programmatically and scrolled into view.
func (hs *HistorySidebar) selectNextRow() {
	hs.selectAdjacentRow(1)
}

// selectAdjacentRow moves selection by direction (-1 or +1) to the next
// selectable row, skipping non-selectable (header) rows. If nothing is
// currently selected, it selects the first (down) or last (up) selectable
// row. Focus remains in the search entry.
func (hs *HistorySidebar) selectAdjacentRow(direction int) {
	if hs.listBox == nil {
		return
	}

	current := -1
	if row := hs.listBox.GetSelectedRow(); row != nil {
		current = row.GetIndex()
	}

	hs.mu.RLock()
	model := newKeyboardNavModel(hs.groups)
	target := -1
	if current < 0 {
		if direction > 0 {
			target = model.firstSelectableIndex()
		} else {
			target = model.lastSelectableIndex()
		}
	} else {
		target = model.nextSelectableIndex(current, direction)
	}
	hs.mu.RUnlock()

	if target == -1 {
		return
	}
	if row := hs.listBox.GetRowAtIndex(target); row != nil && row.GetSelectable() {
		hs.listBox.SelectRow(row)
		hs.ensureRowVisible(target)
	}
}

// ensureRowVisible adjusts the scrolled window so the row at index is
// visible, WITHOUT calling GrabFocus (preserving search entry focus).
// The Y position is computed by summing the allocated heights of all
// preceding rows.
func (hs *HistorySidebar) ensureRowVisible(index int) {
	if hs.scrolledWin == nil || hs.listBox == nil {
		return
	}
	vadj := hs.scrolledWin.GetVadjustment()
	if vadj == nil {
		return
	}
	row := hs.listBox.GetRowAtIndex(index)
	if row == nil {
		return
	}

	// Sum allocated heights of all preceding rows to estimate Y position.
	var yPos int
	for i := 0; i < index; i++ {
		r := hs.listBox.GetRowAtIndex(i)
		if r == nil {
			continue
		}
		yPos += r.GetAllocatedHeight()
	}

	rowHeight := row.GetAllocatedHeight()
	if rowHeight <= 0 {
		return
	}

	pageSize := vadj.GetPageSize()
	current := vadj.GetValue()
	rowTop := float64(yPos)
	rowBottom := rowTop + float64(rowHeight)

	if rowTop < current {
		// Row is above the visible area — scroll up.
		vadj.SetValue(rowTop)
	} else if rowBottom > current+pageSize {
		// Row is below the visible area — scroll down.
		vadj.SetValue(rowBottom - pageSize)
	}
}

// =============================================================================
// Row activation (Enter / click)
// =============================================================================

func (hs *HistorySidebar) onRowActivated(row *gtk.ListBoxRow) {
	if row == nil || !row.GetSelectable() {
		return
	}

	hs.mu.RLock()
	// Allow activation when browse is loaded or when search results are available.
	// This prevents a race where the user searches before the initial browse page
	// finishes loading — browse may be left unloaded, but search results should
	// still be activatable.
	hasSearchResults := hs.searchDone && hs.searchResults != nil
	if (!hs.loadDone && !hasSearchResults) || len(hs.groups) == 0 {
		hs.mu.RUnlock()
		return
	}
	entry := newKeyboardNavModel(hs.groups).entryAt(row.GetIndex())
	hs.mu.RUnlock()
	if entry == nil || entry.URL == "" {
		return
	}

	hs.navigateToURL(entry.URL)
}

func (hs *HistorySidebar) navigateToURL(url string) {
	if hs.onURL == nil || url == "" {
		return
	}

	navigateCb := glib.SourceFunc(func(uintptr) bool {
		hs.mu.RLock()
		destroyed := hs.destroyed
		hs.mu.RUnlock()
		if destroyed {
			return false
		}
		hs.onURL(hs.ctx, url)
		return false
	})
	glib.IdleAdd(&navigateCb, 0)
}

// navigateWithoutClosing navigates to the URL but does NOT close the sidebar.
// Used by Ctrl+Enter activation.
func (hs *HistorySidebar) navigateWithoutClosing(url string) {
	if hs.onNavigateKeepOpen == nil || url == "" {
		return
	}
	hs.doNavigateWithoutClose(url)
}

// doNavigateWithoutClose schedules navigation without closing the sidebar.
// Uses the dedicated OnNavigateKeepOpen path so hosts can override the
// default activation behavior when they need a distinct keep-open action.
func (hs *HistorySidebar) doNavigateWithoutClose(url string) {
	navigateCb := glib.SourceFunc(func(uintptr) bool {
		hs.mu.RLock()
		destroyed := hs.destroyed
		hs.mu.RUnlock()
		if destroyed {
			return false
		}
		hs.onNavigateKeepOpen(hs.ctx, url)
		return false
	})
	glib.IdleAdd(&navigateCb, 0)
}

// navigateToNewPane navigates to the URL by opening it in a new pane.
// The sidebar stays open. Used by Shift+Enter activation.
func (hs *HistorySidebar) navigateToNewPane(url string) {
	if hs.onOpenInNewPane == nil || url == "" {
		return
	}

	navigateCb := glib.SourceFunc(func(uintptr) bool {
		hs.mu.RLock()
		destroyed := hs.destroyed
		hs.mu.RUnlock()
		if destroyed {
			return false
		}
		if err := hs.onOpenInNewPane(hs.ctx, url); err != nil {
			hs.logger.Error().Err(err).Str("url", url).Msg("history sidebar new-pane navigation failed")
		}
		return false
	})
	glib.IdleAdd(&navigateCb, 0)
}

// closeSidebar calls the configured OnClose callback to tell the host to
// hide the sidebar and restore focus to the active content pane/webview.
func (hs *HistorySidebar) closeSidebar() {
	if hs.onClose != nil {
		hs.onClose()
	}
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
