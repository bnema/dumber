package component

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/cache"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

const (
	omniboxWidthPct   = 0.8 // 80% of parent window width
	omniboxMaxWidth   = 800 // Maximum width in pixels
	omniboxHeightPct  = 0.6 // 60% of parent window height
	omniboxMaxResults = 10
	debounceDelayMs   = 150
)

// ViewMode distinguishes history search from favorites display.
type ViewMode string

const (
	ViewModeHistory   ViewMode = "history"
	ViewModeFavorites ViewMode = "favorites"
)

// Suggestion represents a search result from history.
type Suggestion struct {
	URL        string
	Title      string
	FaviconURL string
}

// Favorite represents a bookmarked URL.
type Favorite struct {
	ID         int64
	URL        string
	Title      string
	FaviconURL string
	Position   int
}

// Omnibox is the native GTK4 address bar / command palette.
type Omnibox struct {
	// GTK widgets
	outerBox     *gtk.Box // Outer container for positioning
	mainBox      *gtk.Box // Main content box
	headerBox    *gtk.Box
	historyBtn   *gtk.Button
	favoritesBtn *gtk.Button
	zoomLabel    *gtk.Label
	entry        *gtk.SearchEntry
	scrolledWin  *gtk.ScrolledWindow
	listBox      *gtk.ListBox

	// Parent overlay reference for sizing (set via SetParentOverlay)
	parentOverlay layout.OverlayWidget

	// State
	mu            sync.RWMutex
	visible       bool
	viewMode      ViewMode
	selectedIndex int
	suggestions   []Suggestion
	favorites     []Favorite
	hasNavigated  bool // true if user navigated with arrow keys (enables space to toggle favorite)

	// Dependencies
	historyUC       *usecase.SearchHistoryUseCase
	favoritesUC     *usecase.ManageFavoritesUseCase
	faviconCache    *cache.FaviconCache
	clipboard       port.Clipboard
	shortcuts       map[string]config.SearchShortcut
	defaultSearch   string
	initialBehavior string
	ctx             context.Context

	// Callbacks
	onNavigate func(url string)
	onClose    func()
	onToast    func(message string)

	// Debouncing
	debounceTimer *time.Timer
	debounceMu    sync.Mutex
	lastQuery     string // Prevent duplicate searches

	// Scaling
	uiScale float64

	// Cached measurements (populated after first layout)
	measuredHeights struct {
		header    int  // headerBox natural height
		entry     int  // entry natural height
		singleRow int  // single ListBoxRow natural height
		valid     bool // whether cache is valid
	}
}

// OmniboxConfig holds configuration for creating an Omnibox.
type OmniboxConfig struct {
	HistoryUC       *usecase.SearchHistoryUseCase
	FavoritesUC     *usecase.ManageFavoritesUseCase
	FaviconCache    *cache.FaviconCache
	Clipboard       port.Clipboard
	Shortcuts       map[string]config.SearchShortcut
	DefaultSearch   string
	InitialBehavior string
	UIScale         float64              // UI scale for favicon sizing
	OnNavigate      func(url string)     // Callback when user navigates via omnibox
	OnToast         func(message string) // Callback to show toast notification
}

// NewOmnibox creates a new native GTK4 omnibox widget.
// Call SetParentOverlay() before Show() to set the parent for sizing.
func NewOmnibox(ctx context.Context, cfg OmniboxConfig) *Omnibox {
	log := logging.FromContext(ctx)

	uiScale := cfg.UIScale
	if uiScale <= 0 {
		uiScale = 1.0
	}

	o := &Omnibox{
		viewMode:        ViewModeHistory,
		selectedIndex:   -1,
		historyUC:       cfg.HistoryUC,
		favoritesUC:     cfg.FavoritesUC,
		faviconCache:    cfg.FaviconCache,
		clipboard:       cfg.Clipboard,
		shortcuts:       cfg.Shortcuts,
		defaultSearch:   cfg.DefaultSearch,
		initialBehavior: cfg.InitialBehavior,
		onToast:         cfg.OnToast,
		ctx:             ctx,
		uiScale:         uiScale,
	}

	if err := o.createWidgets(); err != nil {
		log.Error().Err(err).Msg("failed to create omnibox widgets")
		return nil
	}

	o.setupKeyboardHandling()
	o.setupEntryChanged()

	// Set navigation callback if provided
	if cfg.OnNavigate != nil {
		o.onNavigate = cfg.OnNavigate
	}

	log.Debug().Msg("omnibox created")
	return o
}

// SetParentOverlay sets the overlay widget used for sizing calculations.
// Must be called before Show().
func (o *Omnibox) SetParentOverlay(overlay layout.OverlayWidget) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.parentOverlay = overlay
}

// WidgetAsLayout returns the omnibox's outer widget as a layout.Widget.
// This is useful for adding the omnibox to a PaneView overlay.
func (o *Omnibox) WidgetAsLayout(factory layout.WidgetFactory) layout.Widget {
	if o.outerBox == nil {
		return nil
	}
	return factory.WrapWidget(&o.outerBox.Widget)
}

// resizeAndCenter adjusts the omnibox size based on content and centers it.
// rowCount is the number of result rows to display (0 = no content, max 10).
func (o *Omnibox) resizeAndCenter(rowCount int) {
	if o.parentOverlay == nil || o.outerBox == nil || o.mainBox == nil {
		return
	}

	// Use GetAllocatedWidth/Height for actual rendered size
	parentWidth := o.parentOverlay.GetAllocatedWidth()
	parentHeight := o.parentOverlay.GetAllocatedHeight()

	if parentWidth <= 0 || parentHeight <= 0 {
		log := logging.FromContext(o.ctx)
		log.Error().
			Int("parent_width", parentWidth).
			Int("parent_height", parentHeight).
			Msg("omnibox: invalid parent overlay dimensions")
		return
	}

	width := int(float64(parentWidth) * omniboxWidthPct)
	if width > omniboxMaxWidth {
		width = omniboxMaxWidth
	}

	// Cap at max results
	if rowCount > omniboxMaxResults {
		rowCount = omniboxMaxResults
	}

	// Schedule measurement after GTK has laid out widgets
	// This ensures we get accurate heights from actual rendered content
	var cb glib.SourceFunc = func(uintptr) bool {
		o.measureAndResize(width, rowCount)
		return false // One-shot
	}
	glib.IdleAdd(&cb, 0)
}

// measureAndResize calculates and sets the omnibox height based on row count.
// Uses measured widget heights when available, falls back to estimates.
func (o *Omnibox) measureAndResize(width, rowCount int) {
	if o.outerBox == nil || o.mainBox == nil {
		return
	}

	log := logging.FromContext(o.ctx)

	var rowHeight int

	if o.measuredHeights.valid && o.measuredHeights.singleRow > 0 {
		// Use measured values from GTK4 Measure API
		rowHeight = o.measuredHeights.singleRow
	} else {
		// Fallback to hardcoded estimate (before first measurement)
		rowHeight = int(60 * o.uiScale)
	}

	// Calculate content height for the scrolled window
	// This is the key to dynamic sizing - limit the ScrolledWindow's max height
	contentHeight := 0
	if rowCount > 0 {
		contentHeight = rowCount * rowHeight
	}

	// Cap at max results
	maxContentHeight := omniboxMaxResults * rowHeight
	if contentHeight > maxContentHeight {
		contentHeight = maxContentHeight
	}

	// Set size constraints on ScrolledWindow to control dynamic sizing
	if o.scrolledWin != nil {
		// Reset min first to avoid assertion failure when shrinking
		// GTK requires min <= max, so we reset min before setting new values
		o.scrolledWin.SetMinContentHeight(-1)
		o.scrolledWin.SetMaxContentHeight(contentHeight)
		o.scrolledWin.SetMinContentHeight(contentHeight)
	}

	// Force layout recalculation
	o.outerBox.QueueResize()

	log.Debug().
		Int("width", width).
		Int("contentHeight", contentHeight).
		Int("rowHeight", rowHeight).
		Int("rows", rowCount).
		Bool("measured", o.measuredHeights.valid).
		Msg("omnibox resized")
}

// createWidgets builds the GTK widget hierarchy.
func (o *Omnibox) createWidgets() error {
	// Create outer container for positioning (will be added to an overlay)
	o.outerBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if o.outerBox == nil {
		return errNilWidget("outerBox")
	}
	o.outerBox.AddCssClass("omnibox-outer")
	o.outerBox.SetHalign(gtk.AlignCenterValue) // Center horizontally
	o.outerBox.SetValign(gtk.AlignStartValue)  // Align to top
	o.outerBox.SetVisible(false)               // Hidden by default

	// Main content box
	o.mainBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if o.mainBox == nil {
		return errNilWidget("mainBox")
	}
	o.mainBox.AddCssClass("omnibox-container")

	// Header with History/Favorites toggle
	o.headerBox = gtk.NewBox(gtk.OrientationHorizontalValue, 0)
	if o.headerBox == nil {
		return errNilWidget("headerBox")
	}
	o.headerBox.AddCssClass("omnibox-header")
	o.headerBox.SetHexpand(true)

	o.historyBtn = gtk.NewButtonWithLabel("History")
	if o.historyBtn == nil {
		return errNilWidget("historyBtn")
	}
	o.historyBtn.AddCssClass("omnibox-header-btn")
	o.historyBtn.AddCssClass("omnibox-header-active")
	o.historyBtn.SetCanFocus(false)

	o.favoritesBtn = gtk.NewButtonWithLabel("Favorites")
	if o.favoritesBtn == nil {
		return errNilWidget("favoritesBtn")
	}
	o.favoritesBtn.AddCssClass("omnibox-header-btn")
	o.favoritesBtn.SetCanFocus(false)

	// Connect header button clicks
	historyClickCb := func(btn gtk.Button) {
		o.setViewMode(ViewModeHistory)
	}
	o.historyBtn.ConnectClicked(&historyClickCb)

	favoritesClickCb := func(btn gtk.Button) {
		o.setViewMode(ViewModeFavorites)
	}
	o.favoritesBtn.ConnectClicked(&favoritesClickCb)

	// Zoom indicator label (shown when zoom != 100%)
	o.zoomLabel = gtk.NewLabel(nil)
	if o.zoomLabel != nil {
		o.zoomLabel.AddCssClass("omnibox-zoom-indicator")
		o.zoomLabel.SetVisible(false)
		o.zoomLabel.SetHalign(gtk.AlignEndValue)
		o.zoomLabel.SetHexpand(true)
	}

	o.headerBox.Append(&o.historyBtn.Widget)
	o.headerBox.Append(&o.favoritesBtn.Widget)
	if o.zoomLabel != nil {
		o.headerBox.Append(&o.zoomLabel.Widget)
	}

	// Search entry field
	o.entry = gtk.NewSearchEntry()
	if o.entry == nil {
		return errNilWidget("entry")
	}
	o.entry.AddCssClass("omnibox-entry")
	o.entry.SetHexpand(true)

	placeholder := "Search history or enter URL..."
	o.entry.SetPlaceholderText(&placeholder)

	// Scrolled window for list
	o.scrolledWin = gtk.NewScrolledWindow()
	if o.scrolledWin == nil {
		return errNilWidget("scrolledWin")
	}
	o.scrolledWin.AddCssClass("omnibox-scrolled")
	o.scrolledWin.SetVexpand(true)
	o.scrolledWin.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	// Let GTK handle natural sizing - propagate child height up to max
	o.scrolledWin.SetPropagateNaturalHeight(true)

	// List box for suggestions
	o.listBox = gtk.NewListBox()
	if o.listBox == nil {
		return errNilWidget("listBox")
	}
	o.listBox.AddCssClass("omnibox-listbox")
	o.listBox.SetSelectionMode(gtk.SelectionSingleValue)

	// Connect row selection
	rowSelectedCb := func(lb gtk.ListBox, rowPtr uintptr) {
		if rowPtr == 0 {
			o.mu.Lock()
			o.selectedIndex = -1
			o.mu.Unlock()
			return
		}
		row := gtk.ListBoxRowNewFromInternalPtr(rowPtr)
		if row != nil {
			o.mu.Lock()
			o.selectedIndex = row.GetIndex()
			o.mu.Unlock()
		}
	}
	o.listBox.ConnectRowSelected(&rowSelectedCb)

	// Assemble hierarchy
	o.scrolledWin.SetChild(&o.listBox.Widget)
	o.mainBox.Append(&o.headerBox.Widget)
	o.mainBox.Append(&o.entry.Widget)
	o.mainBox.Append(&o.scrolledWin.Widget)
	o.outerBox.Append(&o.mainBox.Widget)

	return nil
}

// setupKeyboardHandling adds keyboard event handling.
func (o *Omnibox) setupKeyboardHandling() {
	log := logging.FromContext(o.ctx)
	controller := gtk.NewEventControllerKey()
	if controller == nil {
		log.Error().Msg("failed to create event controller key")
		return
	}

	// Set capture phase to intercept before entry
	controller.SetPropagationPhase(gtk.PhaseCaptureValue)

	keyPressedCb := func(ctrl gtk.EventControllerKey, keyval uint, keycode uint, state gdk.ModifierType) bool {
		return o.handleKeyPress(keyval, state)
	}
	controller.ConnectKeyPressed(&keyPressedCb)

	o.outerBox.AddController(&controller.EventController)
}

// setupEntryChanged wires entry text changes to debounced search.
func (o *Omnibox) setupEntryChanged() {
	// SearchEntry has built-in debouncing via search-changed signal
	changedCb := func(_ gtk.SearchEntry) {
		o.onEntryChanged()
	}
	o.entry.ConnectSearchChanged(&changedCb)
}

// handleKeyPress processes keyboard events.
// Returns true if the event was handled.
func (o *Omnibox) handleKeyPress(keyval uint, state gdk.ModifierType) bool {
	ctrl := state&gdk.ControlMaskValue != 0

	switch keyval {
	case uint(gdk.KEY_Escape):
		text := o.entry.GetText()
		if text != "" {
			o.entry.SetText("")
			return true
		}
		o.Hide(o.ctx)
		return true

	case uint(gdk.KEY_Return), uint(gdk.KEY_KP_Enter):
		o.navigateToSelected()
		return true

	case uint(gdk.KEY_Up):
		o.selectPrevious()
		return true

	case uint(gdk.KEY_Down):
		o.selectNext()
		return true

	case uint(gdk.KEY_Tab):
		o.toggleViewMode()
		return true

	case uint(gdk.KEY_space):
		// Space toggles favorite only if user has navigated with arrow keys
		// Otherwise, let space pass through to entry for typing
		o.mu.RLock()
		navigated := o.hasNavigated
		o.mu.RUnlock()
		if navigated {
			o.toggleFavorite()
			return true
		}
		return false // Let entry handle space for typing

	case uint(gdk.KEY_y):
		// 'y' yanks (copies) the selected URL to clipboard when navigating
		o.mu.RLock()
		navigated := o.hasNavigated
		o.mu.RUnlock()
		if navigated {
			o.yankSelectedURL()
			return true
		}
		return false // Let entry handle 'y' for typing

	case uint(gdk.KEY_1), uint(gdk.KEY_2), uint(gdk.KEY_3), uint(gdk.KEY_4), uint(gdk.KEY_5),
		uint(gdk.KEY_6), uint(gdk.KEY_7), uint(gdk.KEY_8), uint(gdk.KEY_9):
		if ctrl {
			index := int(keyval - uint(gdk.KEY_1)) // 0-8
			o.selectAndNavigate(index)
			return true
		}

	case uint(gdk.KEY_0):
		if ctrl {
			o.selectAndNavigate(9) // 10th item
			return true
		}
	}

	return false // Let entry handle the key
}

// onEntryChanged handles text input changes with debouncing.
func (o *Omnibox) onEntryChanged() {
	// Reset navigation state when user types - space should type, not toggle favorite
	o.mu.Lock()
	o.hasNavigated = false
	o.mu.Unlock()

	o.debounceMu.Lock()
	if o.debounceTimer != nil {
		o.debounceTimer.Stop()
	}
	o.debounceTimer = time.AfterFunc(debounceDelayMs*time.Millisecond, func() {
		o.performSearch()
	})
	o.debounceMu.Unlock()
}

// performSearch executes the search based on current view mode and query.
func (o *Omnibox) performSearch() {
	o.mu.RLock()
	visible := o.visible
	mode := o.viewMode
	o.mu.RUnlock()

	// Skip search if omnibox is hidden
	if !visible {
		return
	}

	query := o.entry.GetText()

	// Skip duplicate queries
	o.debounceMu.Lock()
	if query == o.lastQuery && query != "" {
		o.debounceMu.Unlock()
		return
	}
	o.lastQuery = query
	o.debounceMu.Unlock()

	if mode == ViewModeFavorites {
		o.loadFavorites(query)
		return
	}

	// History search
	if query == "" {
		o.loadInitialHistory()
		return
	}

	// Perform fuzzy search
	go func() {
		ctx := o.ctx
		log := logging.FromContext(ctx)
		if o.historyUC == nil {
			return
		}

		input := usecase.SearchInput{
			Query: query,
			Limit: omniboxMaxResults,
		}
		output, err := o.historyUC.Search(ctx, input)
		if err != nil {
			log.Error().Err(err).Msg("history search failed")
			return
		}

		suggestions := make([]Suggestion, 0, len(output.Matches))
		for _, r := range output.Matches {
			suggestions = append(suggestions, Suggestion{
				URL:   r.Entry.URL,
				Title: r.Entry.Title,
			})
		}

		// Marshal back to GTK main thread
		o.idleAddUpdateSuggestions(suggestions)
	}()
}

// loadInitialHistory loads history based on InitialBehavior config.
func (o *Omnibox) loadInitialHistory() {
	if o.historyUC == nil {
		return
	}

	go func() {
		ctx := o.ctx
		log := logging.FromContext(ctx)
		var suggestions []Suggestion

		switch o.initialBehavior {
		case "none":
			o.idleAddUpdateSuggestions(nil)
			return

		case "most_visited":
			// TODO: Implement GetMostVisited in use case if needed
			fallthrough

		case "recent", "":
			results, err := o.historyUC.GetRecent(ctx, omniboxMaxResults, 0)
			if err != nil {
				log.Error().Err(err).Msg("failed to load recent history")
				return
			}
			suggestions = make([]Suggestion, 0, len(results))
			for _, r := range results {
				suggestions = append(suggestions, Suggestion{
					URL:   r.URL,
					Title: r.Title,
				})
			}
		}

		o.idleAddUpdateSuggestions(suggestions)
	}()
}

// loadFavorites loads favorites, optionally filtered by query.
func (o *Omnibox) loadFavorites(query string) {
	if o.favoritesUC == nil {
		return
	}

	go func() {
		ctx := o.ctx
		log := logging.FromContext(ctx)
		results, err := o.favoritesUC.GetAll(ctx)
		if err != nil {
			log.Error().Err(err).Msg("failed to load favorites")
			return
		}

		favorites := make([]Favorite, 0, len(results))
		queryLower := strings.ToLower(query)
		for i, r := range results {
			// Filter by query if provided
			if query != "" {
				titleMatch := strings.Contains(strings.ToLower(r.Title), queryLower)
				urlMatch := strings.Contains(strings.ToLower(r.URL), queryLower)
				if !titleMatch && !urlMatch {
					continue
				}
			}
			favorites = append(favorites, Favorite{
				ID:       int64(r.ID),
				URL:      r.URL,
				Title:    r.Title,
				Position: i,
			})
		}

		o.idleAddUpdateFavorites(favorites)
	}()
}

// updateSuggestions updates the list with history suggestions.
func (o *Omnibox) updateSuggestions(suggestions []Suggestion) {
	o.mu.Lock()
	o.suggestions = suggestions
	o.selectedIndex = -1
	o.mu.Unlock()

	o.rebuildList()

	// Hide scrolled window when there are no suggestions
	rowCount := len(suggestions)
	if o.scrolledWin != nil {
		o.scrolledWin.SetVisible(rowCount > 0)
	}
	o.resizeAndCenter(rowCount)

	// Select first item if available
	if rowCount > 0 {
		o.selectIndex(0)
	}
}

// updateFavorites updates the list with favorites.
func (o *Omnibox) updateFavorites(favorites []Favorite) {
	o.mu.Lock()
	o.favorites = favorites
	o.selectedIndex = -1
	o.mu.Unlock()

	o.rebuildList()

	// Hide scrolled window when there are no favorites
	rowCount := len(favorites)
	if o.scrolledWin != nil {
		o.scrolledWin.SetVisible(rowCount > 0)
	}
	o.resizeAndCenter(rowCount)

	// Select first item if available
	if rowCount > 0 {
		o.selectIndex(0)
	}
}

// rebuildList rebuilds the ListBox contents.
func (o *Omnibox) rebuildList() {
	// Clear existing rows
	o.listBox.RemoveAll()

	o.mu.RLock()
	mode := o.viewMode
	suggestions := o.suggestions
	favorites := o.favorites
	o.mu.RUnlock()

	if mode == ViewModeHistory {
		for i, s := range suggestions {
			row := o.createSuggestionRow(s, i)
			if row != nil {
				o.listBox.Append(&row.Widget)
			}
		}
	} else {
		for i, f := range favorites {
			row := o.createFavoriteRow(f, i)
			if row != nil {
				o.listBox.Append(&row.Widget)
			}
		}
	}

	// Schedule measurement if not yet cached and we have rows
	if !o.measuredHeights.valid && o.listBox.GetRowAtIndex(0) != nil {
		var cb glib.SourceFunc = func(uintptr) bool {
			if o.parentOverlay == nil {
				return false
			}
			width := o.parentOverlay.GetAllocatedWidth()
			if width <= 0 {
				return false // Overlay not allocated yet, skip
			}
			forWidth := int(float64(width) * omniboxWidthPct)
			if o.measureComponentHeights(forWidth) {
				// Re-trigger resize with accurate measurements
				o.mu.RLock()
				var count int
				if o.viewMode == ViewModeHistory {
					count = len(o.suggestions)
				} else {
					count = len(o.favorites)
				}
				o.mu.RUnlock()
				o.measureAndResize(forWidth, count)
			}
			return false
		}
		glib.IdleAdd(&cb, 0)
	}
}

// createFaviconImage creates a favicon image with async loading from cache.
func (o *Omnibox) createFaviconImage(url, fallbackIcon string) *gtk.Image {
	favicon := gtk.NewImage()
	if favicon == nil {
		return nil
	}
	favicon.SetFromIconName(&fallbackIcon)
	favicon.SetPixelSize(int(16 * o.uiScale))
	favicon.AddCssClass("omnibox-favicon")

	// Async load favicon from cache
	if o.faviconCache != nil && url != "" {
		o.faviconCache.GetOrFetch(o.ctx, url, func(texture *gdk.Texture) {
			if texture != nil {
				var cb glib.SourceFunc = func(data uintptr) bool {
					favicon.SetFromPaintable(texture)
					return false
				}
				glib.IdleAdd(&cb, 0)
			}
		})
	}

	return favicon
}

// createRowWithFavicon creates a ListBoxRow with favicon, title, URL, and shortcut badge.
func (o *Omnibox) createRowWithFavicon(url, title, fallbackIcon string, index int) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()
	if row == nil {
		return nil
	}
	row.AddCssClass("omnibox-row")

	hbox := gtk.NewBox(gtk.OrientationHorizontalValue, 8)
	if hbox == nil {
		return nil
	}
	hbox.SetHexpand(true)

	// Favicon image (vertically centered)
	if favicon := o.createFaviconImage(url, fallbackIcon); favicon != nil {
		favicon.SetValign(gtk.AlignCenterValue)
		hbox.Append(&favicon.Widget)
	}

	// Vertical box for title + URL
	textBox := gtk.NewBox(gtk.OrientationVerticalValue, 2)
	if textBox == nil {
		return nil
	}
	textBox.SetHexpand(true)
	textBox.SetValign(gtk.AlignCenterValue)

	// Title label (or URL if no title)
	displayTitle := title
	if displayTitle == "" {
		displayTitle = url
	}
	titleLabel := gtk.NewLabel(nil)
	if titleLabel != nil {
		titleLabel.SetText(displayTitle)
		titleLabel.AddCssClass("omnibox-suggestion-title")
		titleLabel.SetHalign(gtk.AlignStartValue)
		titleLabel.SetEllipsize(2) // PANGO_ELLIPSIZE_END
		textBox.Append(&titleLabel.Widget)
	}

	// URL label (only if title exists and differs from URL)
	if title != "" && title != url {
		urlLabel := gtk.NewLabel(nil)
		if urlLabel != nil {
			urlLabel.SetText(url)
			urlLabel.AddCssClass("omnibox-suggestion-url")
			urlLabel.SetHalign(gtk.AlignStartValue)
			urlLabel.SetEllipsize(2) // PANGO_ELLIPSIZE_END
			textBox.Append(&urlLabel.Widget)
		}
	}

	hbox.Append(&textBox.Widget)

	// Shortcut badge (Ctrl+1-9, Ctrl+0 for 10th)
	if index <= 9 {
		shortcutLabel := gtk.NewLabel(nil)
		if shortcutLabel != nil {
			if index < 9 {
				shortcutLabel.SetText(formatShortcut(index + 1))
			} else {
				shortcutLabel.SetText("Ctrl+0")
			}
			shortcutLabel.AddCssClass("omnibox-shortcut-badge")
			shortcutLabel.SetValign(gtk.AlignCenterValue)
			hbox.Append(&shortcutLabel.Widget)
		}
	}

	row.SetChild(&hbox.Widget)
	return row
}

// createSuggestionRow creates a ListBoxRow for a suggestion.
func (o *Omnibox) createSuggestionRow(s Suggestion, index int) *gtk.ListBoxRow {
	return o.createRowWithFavicon(s.URL, s.Title, "web-browser-symbolic", index)
}

// createFavoriteRow creates a ListBoxRow for a favorite.
func (o *Omnibox) createFavoriteRow(f Favorite, index int) *gtk.ListBoxRow {
	return o.createRowWithFavicon(f.URL, f.Title, "starred-symbolic", index)
}

// selectIndex selects a row by index.
func (o *Omnibox) selectIndex(index int) {
	// Note: Don't hold mutex when calling SelectRow - it triggers rowSelectedCb
	// which also acquires the mutex, causing deadlock
	row := o.listBox.GetRowAtIndex(index)
	if row != nil {
		o.listBox.SelectRow(row)
		// selectedIndex is updated by rowSelectedCb
	}
}

// selectNext moves selection down.
func (o *Omnibox) selectNext() {
	o.mu.Lock()
	current := o.selectedIndex
	mode := o.viewMode
	var maxIndex int
	if mode == ViewModeHistory {
		maxIndex = len(o.suggestions) - 1
	} else {
		maxIndex = len(o.favorites) - 1
	}
	o.hasNavigated = true // User is navigating with arrow keys
	o.mu.Unlock()

	if maxIndex < 0 {
		return
	}

	newIndex := current + 1
	if newIndex > maxIndex {
		newIndex = 0 // Wrap around
	}
	o.selectIndex(newIndex)
}

// selectPrevious moves selection up.
func (o *Omnibox) selectPrevious() {
	o.mu.Lock()
	current := o.selectedIndex
	mode := o.viewMode
	var maxIndex int
	if mode == ViewModeHistory {
		maxIndex = len(o.suggestions) - 1
	} else {
		maxIndex = len(o.favorites) - 1
	}
	o.hasNavigated = true // User is navigating with arrow keys
	o.mu.Unlock()

	if maxIndex < 0 {
		return
	}

	newIndex := current - 1
	if newIndex < 0 {
		newIndex = maxIndex // Wrap around
	}
	o.selectIndex(newIndex)
}

// selectAndNavigate selects an index and navigates to it.
func (o *Omnibox) selectAndNavigate(index int) {
	o.selectIndex(index)
	o.navigateToSelected()
}

// navigateToSelected navigates to the currently selected item or typed URL.
// If the user typed a URL-like string, prioritize navigating to that directly.
func (o *Omnibox) navigateToSelected() {
	o.mu.RLock()
	mode := o.viewMode
	idx := o.selectedIndex
	suggestions := o.suggestions
	favorites := o.favorites
	o.mu.RUnlock()

	entryText := o.entry.GetText()
	var url string

	// Check if user typed a URL-like string (contains . and no spaces)
	// If so, navigate to that URL directly instead of the selection
	if o.looksLikeURL(entryText) {
		url = o.buildURL(entryText)
	} else if idx < 0 {
		// No selection - use entry text as URL/search
		url = o.buildURL(entryText)
	} else if mode == ViewModeHistory {
		if idx < len(suggestions) {
			url = suggestions[idx].URL
		}
	} else {
		if idx < len(favorites) {
			url = favorites[idx].URL
		}
	}

	if url == "" {
		return
	}

	o.Hide(o.ctx)

	if o.onNavigate != nil {
		o.onNavigate(url)
	}
}

// looksLikeURL checks if the input appears to be a URL (not a search query).
// Returns true for strings like "github.com", "google.com/search", etc.
func (o *Omnibox) looksLikeURL(input string) bool {
	return url.LooksLikeURL(input)
}

// toggleFavorite adds or removes the selected item from favorites.
// In History mode: adds the selected item to favorites
// In Favorites mode: removes the selected item from favorites
func (o *Omnibox) toggleFavorite() {
	log := logging.FromContext(o.ctx)

	o.mu.RLock()
	mode := o.viewMode
	idx := o.selectedIndex
	suggestions := o.suggestions
	favorites := o.favorites
	o.mu.RUnlock()

	if o.favoritesUC == nil {
		log.Warn().Msg("toggle favorite: favoritesUC is nil")
		return
	}

	if mode == ViewModeHistory {
		// Add to favorites
		if idx < 0 || idx >= len(suggestions) {
			log.Debug().Int("index", idx).Msg("add favorite: invalid selection")
			return
		}

		s := suggestions[idx]
		if s.URL == "" {
			log.Debug().Msg("add favorite: empty URL")
			return
		}

		go func() {
			ctx := o.ctx
			log := logging.FromContext(ctx)

			log.Debug().Str("url", s.URL).Str("title", s.Title).Msg("adding to favorites")

			input := usecase.AddFavoriteInput{
				URL:   s.URL,
				Title: s.Title,
			}

			fav, err := o.favoritesUC.Add(ctx, input)
			if err != nil {
				log.Error().Err(err).Str("url", s.URL).Msg("failed to add favorite")
				return
			}

			log.Info().Str("url", s.URL).Int64("id", int64(fav.ID)).Msg("favorite added from omnibox")
		}()
	} else {
		// Remove from favorites
		if idx < 0 || idx >= len(favorites) {
			log.Debug().Int("index", idx).Msg("remove favorite: invalid selection")
			return
		}

		f := favorites[idx]
		if f.ID == 0 {
			log.Debug().Msg("remove favorite: invalid ID")
			return
		}

		go func() {
			ctx := o.ctx
			log := logging.FromContext(ctx)

			log.Debug().Int64("id", f.ID).Str("url", f.URL).Msg("removing from favorites")

			err := o.favoritesUC.Remove(ctx, entity.FavoriteID(f.ID))
			if err != nil {
				log.Error().Err(err).Int64("id", f.ID).Msg("failed to remove favorite")
				return
			}

			log.Info().Int64("id", f.ID).Str("url", f.URL).Msg("favorite removed from omnibox")

			// Refresh favorites list
			o.loadFavorites(o.entry.GetText())
		}()
	}
}

// yankSelectedURL copies the URL of the selected item to clipboard.
func (o *Omnibox) yankSelectedURL() {
	log := logging.FromContext(o.ctx)

	if o.clipboard == nil {
		log.Warn().Msg("yank URL: clipboard is nil")
		return
	}

	o.mu.RLock()
	mode := o.viewMode
	idx := o.selectedIndex
	suggestions := o.suggestions
	favorites := o.favorites
	o.mu.RUnlock()

	var url string
	if mode == ViewModeHistory {
		if idx < 0 || idx >= len(suggestions) {
			log.Debug().Int("index", idx).Msg("yank URL: invalid selection")
			return
		}
		url = suggestions[idx].URL
	} else {
		if idx < 0 || idx >= len(favorites) {
			log.Debug().Int("index", idx).Msg("yank URL: invalid selection")
			return
		}
		url = favorites[idx].URL
	}

	if url == "" {
		log.Debug().Msg("yank URL: empty URL")
		return
	}

	go func() {
		ctx := o.ctx
		if err := o.clipboard.WriteText(ctx, url); err != nil {
			log.Error().Err(err).Str("url", url).Msg("failed to copy URL to clipboard")
			return
		}
		log.Debug().Str("url", url).Msg("URL copied to clipboard")

		// Show toast notification on success (must run on GTK main thread)
		if o.onToast != nil {
			var cb glib.SourceFunc
			cb = func(_ uintptr) bool {
				o.onToast("URL copied")
				return false // Don't repeat
			}
			glib.IdleAdd(&cb, 0)
		}
	}()
}

// buildURL constructs a URL from input, handling search shortcuts.
func (o *Omnibox) buildURL(input string) string {
	if input == "" {
		return ""
	}

	// Check for search shortcut (e.g., "g:query")
	if colonIdx := strings.Index(input, ":"); colonIdx > 0 && colonIdx < 10 {
		prefix := input[:colonIdx]
		if shortcut, ok := o.shortcuts[prefix]; ok {
			query := strings.TrimSpace(input[colonIdx+1:])
			return strings.Replace(shortcut.URL, "%s", query, 1)
		}
	}

	// Check if it looks like a URL - use shared URL normalization
	if url.LooksLikeURL(input) {
		return url.Normalize(input)
	}

	// Use default search
	if o.defaultSearch != "" {
		return strings.Replace(o.defaultSearch, "%s", input, 1)
	}

	return input
}

// toggleViewMode switches between history and favorites.
func (o *Omnibox) toggleViewMode() {
	o.mu.RLock()
	current := o.viewMode
	o.mu.RUnlock()

	if current == ViewModeHistory {
		o.setViewMode(ViewModeFavorites)
	} else {
		o.setViewMode(ViewModeHistory)
	}
}

// setViewMode changes the view mode and updates UI.
func (o *Omnibox) setViewMode(mode ViewMode) {
	o.mu.Lock()
	o.viewMode = mode
	o.mu.Unlock()

	// Update header button styling
	if mode == ViewModeHistory {
		o.historyBtn.AddCssClass("omnibox-header-active")
		o.favoritesBtn.RemoveCssClass("omnibox-header-active")
	} else {
		o.historyBtn.RemoveCssClass("omnibox-header-active")
		o.favoritesBtn.AddCssClass("omnibox-header-active")
	}

	// Reload data
	o.performSearch()
}

// Show opens the omnibox with optional initial query.
func (o *Omnibox) Show(ctx context.Context, query string) {
	log := logging.FromContext(ctx)
	log.Debug().Str("query", query).Msg("showing omnibox")

	o.mu.Lock()
	if o.visible {
		o.mu.Unlock()
		return
	}
	o.visible = true
	o.mu.Unlock()

	// Set initial query
	o.entry.SetText(query)

	// Determine if we expect content initially
	// No content expected if: no query AND initialBehavior is "none"
	expectContent := query != "" || o.initialBehavior != "none"

	// Hide scrolled window if no content expected
	if o.scrolledWin != nil {
		o.scrolledWin.SetVisible(expectContent)
		// Reset content height constraints - will be updated when results arrive
		o.scrolledWin.SetMinContentHeight(-1)
		o.scrolledWin.SetMaxContentHeight(0)
	}

	// Set width and vertical positioning (20% from top)
	parentWidth := o.parentOverlay.GetAllocatedWidth()
	parentHeight := o.parentOverlay.GetAllocatedHeight()
	width := int(float64(parentWidth) * omniboxWidthPct)
	if width > omniboxMaxWidth {
		width = omniboxMaxWidth
	}
	marginTop := int(float64(parentHeight) * 0.20) // 20% from top

	o.mainBox.SetSizeRequest(width, -1)
	o.outerBox.SetMarginTop(marginTop)

	// Show the omnibox
	o.outerBox.SetVisible(true)

	// Trigger initial resize (will be updated when results arrive)
	o.resizeAndCenter(0)

	// Focus the entry
	o.entry.GrabFocus()

	// Load initial data (may update size later if results found)
	o.performSearch()
}

// Hide closes the omnibox.
func (o *Omnibox) Hide(ctx context.Context) {
	log := logging.FromContext(ctx)
	log.Debug().Msg("hiding omnibox")

	o.mu.Lock()
	if !o.visible {
		o.mu.Unlock()
		return
	}
	o.visible = false
	o.mu.Unlock()

	// Cancel any pending search
	o.debounceMu.Lock()
	if o.debounceTimer != nil {
		o.debounceTimer.Stop()
		o.debounceTimer = nil
	}
	o.debounceMu.Unlock()

	// Hide omnibox
	o.outerBox.SetVisible(false)

	// Clear state
	o.entry.SetText("")
	o.listBox.RemoveAll()

	if o.onClose != nil {
		o.onClose()
	}
}

// Toggle shows if hidden, hides if visible.
func (o *Omnibox) Toggle(ctx context.Context) {
	o.mu.RLock()
	visible := o.visible
	o.mu.RUnlock()

	if visible {
		o.Hide(ctx)
	} else {
		o.Show(ctx, "")
	}
}

// IsVisible returns whether the omnibox is currently shown.
func (o *Omnibox) IsVisible() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.visible
}

// SetOnNavigate sets the callback for URL navigation.
func (o *Omnibox) SetOnNavigate(fn func(url string)) {
	o.onNavigate = fn
}

// SetOnClose sets the callback for omnibox close events.
func (o *Omnibox) SetOnClose(fn func()) {
	o.onClose = fn
}

// UpdateZoomIndicator updates the zoom percentage display.
// Shows the indicator when zoom != 100%, hides it when at 100%.
func (o *Omnibox) UpdateZoomIndicator(factor float64) {
	var cb glib.SourceFunc = func(data uintptr) bool {
		if o.zoomLabel == nil {
			return false
		}
		if factor == 1.0 {
			o.zoomLabel.SetVisible(false)
		} else {
			percentage := int(factor * 100)
			o.zoomLabel.SetText(fmt.Sprintf("%d%%", percentage))
			o.zoomLabel.SetVisible(true)
		}
		return false
	}
	glib.IdleAdd(&cb, 0)
}

// idleAddUpdateSuggestions schedules updateSuggestions on the GTK main thread.
func (o *Omnibox) idleAddUpdateSuggestions(suggestions []Suggestion) {
	var cb glib.SourceFunc = func(data uintptr) bool {
		o.updateSuggestions(suggestions)
		return false // One-shot callback
	}
	glib.IdleAdd(&cb, 0)
}

// idleAddUpdateFavorites schedules updateFavorites on the GTK main thread.
func (o *Omnibox) idleAddUpdateFavorites(favorites []Favorite) {
	var cb glib.SourceFunc = func(data uintptr) bool {
		o.updateFavorites(favorites)
		return false // One-shot callback, return false to remove source
	}
	glib.IdleAdd(&cb, 0)
}

// Widget returns the omnibox widget for embedding in an overlay.
func (o *Omnibox) Widget() *gtk.Widget {
	if o.outerBox == nil {
		return nil
	}
	return &o.outerBox.Widget
}

// Destroy cleans up omnibox resources.
func (o *Omnibox) Destroy() {
	o.debounceMu.Lock()
	if o.debounceTimer != nil {
		o.debounceTimer.Stop()
	}
	o.debounceMu.Unlock()

	if o.outerBox != nil {
		o.outerBox.Unparent()
		o.outerBox = nil
	}
}

// formatShortcut formats a shortcut number.
func formatShortcut(n int) string {
	return "Ctrl+" + string(rune('0'+n))
}

// measureWidgetHeight returns the natural height of a widget for the given width.
// Returns 0 if widget is nil or measurement fails.
func measureWidgetHeight(widget *gtk.Widget, forWidth int) int {
	if widget == nil {
		return 0
	}
	var minHeight, naturalHeight, minBaseline, naturalBaseline int
	widget.Measure(
		gtk.OrientationVerticalValue,
		forWidth,
		&minHeight,
		&naturalHeight,
		&minBaseline,
		&naturalBaseline,
	)
	return naturalHeight
}

// measureComponentHeights measures and caches all component heights.
// Must be called on GTK main thread after widgets have been laid out.
func (o *Omnibox) measureComponentHeights(forWidth int) bool {
	if o.headerBox == nil || o.entry == nil || o.listBox == nil {
		return false
	}

	headerH := measureWidgetHeight(&o.headerBox.Widget, forWidth)
	entryH := measureWidgetHeight(&o.entry.Widget, forWidth)

	// Measure first row if it exists
	var rowH int
	if row := o.listBox.GetRowAtIndex(0); row != nil {
		rowH = measureWidgetHeight(&row.Widget, forWidth)
	}

	// Validate measurements (header and entry must be positive)
	if headerH <= 0 || entryH <= 0 {
		return false
	}

	o.measuredHeights.header = headerH
	o.measuredHeights.entry = entryH
	if rowH > 0 {
		o.measuredHeights.singleRow = rowH
		o.measuredHeights.valid = true
	}
	return true
}

// errNilWidget creates an error for nil widget creation.
func errNilWidget(name string) error {
	return &widgetError{name: name}
}

type widgetError struct {
	name string
}

func (e *widgetError) Error() string {
	return "failed to create widget: " + e.name
}
