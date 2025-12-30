package component

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/adapter"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

const (
	debounceDelayMs = 150
	endBoxSpacing   = 6
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
	IsFavorite bool // Indicates if this URL is also bookmarked as a favorite
}

// BangSuggestion represents a configured bang shortcut.
// A bang is invoked by typing "!<key>".
type BangSuggestion struct {
	Key         string
	Description string
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
	bangBadge    *gtk.Label
	entry        *gtk.SearchEntry
	scrolledWin  *gtk.ScrolledWindow
	listBox      *gtk.ListBox

	// Parent overlay reference for sizing (set via SetParentOverlay)
	parentOverlay layout.OverlayWidget

	// State
	mu              sync.RWMutex
	visible         bool
	viewMode        ViewMode
	selectedIndex   int
	suggestions     []Suggestion
	favorites       []Favorite
	bangSuggestions []BangSuggestion
	bangMode        bool
	detectedBang    string
	hasNavigated    bool // true if user navigated with arrow keys (enables space to toggle favorite)

	// Dependencies
	historyUC       *usecase.SearchHistoryUseCase
	favoritesUC     *usecase.ManageFavoritesUseCase
	faviconAdapter  *adapter.FaviconAdapter
	copyURLUC       *usecase.CopyURLUseCase
	shortcutsUC     *usecase.SearchShortcutsUseCase
	defaultSearch   string
	initialBehavior string
	ctx             context.Context

	// Callbacks
	onNavigate func(url string)
	onClose    func()
	onToast    func(ctx context.Context, message string, level ToastLevel)

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

	// Callback retention: keep GTK signal callbacks reachable by Go GC.
	retainedCallbacks []interface{}
}

// OmniboxConfig holds configuration for creating an Omnibox.
type OmniboxConfig struct {
	HistoryUC       *usecase.SearchHistoryUseCase
	FavoritesUC     *usecase.ManageFavoritesUseCase
	FaviconAdapter  *adapter.FaviconAdapter
	CopyURLUC       *usecase.CopyURLUseCase
	ShortcutsUC     *usecase.SearchShortcutsUseCase
	DefaultSearch   string
	InitialBehavior string
	UIScale         float64                                                     // UI scale for favicon sizing
	OnNavigate      func(url string)                                            // Callback when user navigates via omnibox
	OnToast         func(ctx context.Context, message string, level ToastLevel) // Callback to show toast notification
	OnFocusIn       func(entry *gtk.SearchEntry)                                // Callback when entry gains focus (for accent picker)
	OnFocusOut      func()                                                      // Callback when entry loses focus
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
		faviconAdapter:  cfg.FaviconAdapter,
		copyURLUC:       cfg.CopyURLUC,
		shortcutsUC:     cfg.ShortcutsUC,
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
	o.setupFocusCallbacks(cfg.OnFocusIn, cfg.OnFocusOut)

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

	// Calculate width using shared helper
	width, _ := CalculateModalDimensions(o.parentOverlay, OmniboxSizeDefaults)

	// Cap at max results
	if rowCount > OmniboxListDefaults.MaxVisibleRows {
		rowCount = OmniboxListDefaults.MaxVisibleRows
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
		// Fallback to scaled estimate (before first measurement)
		rowHeight = ScaleValue(DefaultRowHeights.Standard, o.uiScale)
	}

	// Calculate content height for the scrolled window
	// This is the key to dynamic sizing - limit the ScrolledWindow's max height
	contentHeight := 0
	if rowCount > 0 {
		contentHeight = rowCount * rowHeight
	}

	// Cap at max results
	maxContentHeight := OmniboxListDefaults.MaxVisibleRows * rowHeight
	if contentHeight > maxContentHeight {
		contentHeight = maxContentHeight
	}

	// Set size constraints on ScrolledWindow using shared helper
	SetScrolledWindowHeight(o.scrolledWin, contentHeight)

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
	if err := o.initOuterBox(); err != nil {
		return err
	}
	if err := o.initMainBox(); err != nil {
		return err
	}
	if err := o.initHeader(); err != nil {
		return err
	}
	if err := o.initEntry(); err != nil {
		return err
	}
	if err := o.initList(); err != nil {
		return err
	}
	o.assembleWidgets()
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

	keyPressedCb := func(_ gtk.EventControllerKey, keyval, keycode uint, state gdk.ModifierType) bool {
		return o.handleKeyPress(keyval, keycode, state)
	}
	o.retainedCallbacks = append(o.retainedCallbacks, keyPressedCb)
	controller.ConnectKeyPressed(&keyPressedCb)

	o.outerBox.AddController(&controller.EventController)
}

func (o *Omnibox) initOuterBox() error {
	o.outerBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if o.outerBox == nil {
		return errNilWidget("outerBox")
	}
	o.outerBox.AddCssClass("omnibox-outer")
	o.outerBox.SetHalign(gtk.AlignCenterValue) // Center horizontally
	o.outerBox.SetValign(gtk.AlignStartValue)  // Align to top
	o.outerBox.SetVisible(false)               // Hidden by default
	return nil
}

func (o *Omnibox) initMainBox() error {
	o.mainBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if o.mainBox == nil {
		return errNilWidget("mainBox")
	}
	o.mainBox.AddCssClass("omnibox-container")
	return nil
}

func (o *Omnibox) initHeader() error {
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

	historyClickCb := func(_ gtk.Button) {
		o.setViewMode(ViewModeHistory)
	}
	o.retainedCallbacks = append(o.retainedCallbacks, historyClickCb)
	o.historyBtn.ConnectClicked(&historyClickCb)

	favoritesClickCb := func(_ gtk.Button) {
		o.setViewMode(ViewModeFavorites)
	}
	o.retainedCallbacks = append(o.retainedCallbacks, favoritesClickCb)
	o.favoritesBtn.ConnectClicked(&favoritesClickCb)

	o.bangBadge = gtk.NewLabel(nil)
	if o.bangBadge != nil {
		o.bangBadge.AddCssClass("omnibox-bang-badge")
		o.bangBadge.SetVisible(false)
	}

	o.zoomLabel = gtk.NewLabel(nil)
	if o.zoomLabel != nil {
		o.zoomLabel.AddCssClass("omnibox-zoom-indicator")
		o.zoomLabel.SetVisible(false)
	}
	return nil
}

func (o *Omnibox) initEntry() error {
	o.entry = gtk.NewSearchEntry()
	if o.entry == nil {
		return errNilWidget("entry")
	}
	o.entry.AddCssClass("omnibox-entry")
	o.entry.SetHexpand(true)

	placeholder := "Search history or enter URL… (! lists bangs)"
	o.entry.SetPlaceholderText(&placeholder)
	return nil
}

func (o *Omnibox) initList() error {
	o.scrolledWin = gtk.NewScrolledWindow()
	if o.scrolledWin == nil {
		return errNilWidget("scrolledWin")
	}
	o.scrolledWin.AddCssClass("omnibox-scrolled")
	o.scrolledWin.SetVexpand(true)
	o.scrolledWin.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	o.scrolledWin.SetPropagateNaturalHeight(true)

	o.listBox = gtk.NewListBox()
	if o.listBox == nil {
		return errNilWidget("listBox")
	}
	o.listBox.AddCssClass("omnibox-listbox")
	o.listBox.SetSelectionMode(gtk.SelectionSingleValue)

	rowSelectedCb := func(_ gtk.ListBox, rowPtr uintptr) {
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
	o.retainedCallbacks = append(o.retainedCallbacks, rowSelectedCb)
	o.listBox.ConnectRowSelected(&rowSelectedCb)
	return nil
}

func (o *Omnibox) assembleWidgets() {
	o.headerBox.Append(&o.historyBtn.Widget)
	o.headerBox.Append(&o.favoritesBtn.Widget)

	endBox := gtk.NewBox(gtk.OrientationHorizontalValue, endBoxSpacing)
	if endBox != nil {
		endBox.SetHexpand(true)
		endBox.SetHalign(gtk.AlignEndValue)
		if o.bangBadge != nil {
			endBox.Append(&o.bangBadge.Widget)
		}
		if o.zoomLabel != nil {
			endBox.Append(&o.zoomLabel.Widget)
		}
		o.headerBox.Append(&endBox.Widget)
	}

	o.scrolledWin.SetChild(&o.listBox.Widget)
	o.mainBox.Append(&o.headerBox.Widget)
	o.mainBox.Append(&o.entry.Widget)
	o.mainBox.Append(&o.scrolledWin.Widget)
	o.outerBox.Append(&o.mainBox.Widget)
}

// setupEntryChanged wires entry text changes to debounced search.
func (o *Omnibox) setupEntryChanged() {
	// SearchEntry has built-in debouncing via search-changed signal
	changedCb := func(_ gtk.SearchEntry) {
		o.onEntryChanged()
	}
	o.retainedCallbacks = append(o.retainedCallbacks, changedCb)
	o.entry.ConnectSearchChanged(&changedCb)
}

// setupFocusCallbacks wires focus in/out callbacks for accent picker integration.
func (o *Omnibox) setupFocusCallbacks(onFocusIn func(*gtk.SearchEntry), onFocusOut func()) {
	if onFocusIn == nil && onFocusOut == nil {
		return
	}

	focusController := gtk.NewEventControllerFocus()
	if focusController == nil {
		return
	}

	if onFocusIn != nil {
		entry := o.entry // Capture for closure
		focusInCb := func(_ gtk.EventControllerFocus) {
			onFocusIn(entry)
		}
		o.retainedCallbacks = append(o.retainedCallbacks, focusInCb)
		focusController.ConnectEnter(&focusInCb)
	}

	if onFocusOut != nil {
		focusOutCb := func(_ gtk.EventControllerFocus) {
			onFocusOut()
		}
		o.retainedCallbacks = append(o.retainedCallbacks, focusOutCb)
		focusController.ConnectLeave(&focusOutCb)
	}

	o.entry.AddController(&focusController.EventController)
}

// handleKeyPress processes keyboard events.
// Returns true if the event was handled.
func (o *Omnibox) handleKeyPress(keyval, keycode uint, state gdk.ModifierType) bool {
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

	case uint(gdk.KEY_l), uint(gdk.KEY_L):
		// Ctrl+L toggles omnibox (hides when visible)
		if ctrl {
			o.Hide(o.ctx)
			return true
		}

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

	default:
		return o.handleCtrlNumberShortcut(keyval, keycode, ctrl)
	}

	return false // Let entry handle the key
}

// handleCtrlNumberShortcut handles Ctrl+1-9 and Ctrl+0 for quick navigation.
// Uses hardware keycode as fallback for non-QWERTY keyboards (AZERTY, QWERTZ, etc.).
func (o *Omnibox) handleCtrlNumberShortcut(keyval, keycode uint, ctrl bool) bool {
	if !ctrl {
		return false
	}

	const tenthItemIndex = 9

	// First try keyval (works for QWERTY layouts)
	switch keyval {
	case uint(gdk.KEY_1), uint(gdk.KEY_2), uint(gdk.KEY_3), uint(gdk.KEY_4), uint(gdk.KEY_5),
		uint(gdk.KEY_6), uint(gdk.KEY_7), uint(gdk.KEY_8), uint(gdk.KEY_9):
		idx := int(keyval - uint(gdk.KEY_1))
		o.selectAndNavigate(idx)
		return true
	case uint(gdk.KEY_0):
		o.selectAndNavigate(tenthItemIndex)
		return true
	}

	// Fallback: use hardware keycode for non-QWERTY layouts
	// This enables Ctrl+1-9/0 to work on AZERTY, QWERTZ, etc.
	if index, ok := input.KeycodeToDigitIndex(keycode); ok {
		log := logging.FromContext(o.ctx)
		log.Debug().
			Uint("keycode", keycode).
			Int("index", index).
			Msg("omnibox shortcut via hardware keycode fallback (non-QWERTY layout)")
		o.selectAndNavigate(index)
		return true
	}

	return false
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

	if strings.HasPrefix(query, "!") {
		o.updateBangDetection(query)
		o.loadBangSuggestions(query)
		return
	}

	o.clearBangState()

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

		// Run history search and favorite URL fetch in parallel
		type searchResult struct {
			output *usecase.SearchOutput
			err    error
		}
		searchCh := make(chan searchResult, 1)
		favCh := make(chan map[string]struct{}, 1)

		go func() {
			searchInput := usecase.SearchInput{
				Query: query,
				Limit: OmniboxListDefaults.MaxResults,
			}
			output, err := o.historyUC.Search(ctx, searchInput)
			searchCh <- searchResult{output, err}
		}()

		go func() {
			favCh <- o.getFavoriteURLs(ctx)
		}()

		// Wait for both results
		sr := <-searchCh
		favoriteURLs := <-favCh

		if sr.err != nil {
			log.Error().Err(sr.err).Msg("history search failed")
			return
		}
		if sr.output == nil {
			return
		}

		suggestions := make([]Suggestion, 0, len(sr.output.Matches))
		for _, r := range sr.output.Matches {
			_, isFav := favoriteURLs[r.Entry.URL]
			suggestions = append(suggestions, Suggestion{
				URL:        r.Entry.URL,
				Title:      r.Entry.Title,
				IsFavorite: isFav,
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

		case "most_visited", "recent", "":
			// Run history fetch and favorite URL fetch in parallel
			type historyResult struct {
				results []*entity.HistoryEntry
				err     error
			}
			historyCh := make(chan historyResult, 1)
			favCh := make(chan map[string]struct{}, 1)

			go func() {
				// TODO: Implement GetMostVisited in use case if needed
				results, err := o.historyUC.GetRecent(ctx, OmniboxListDefaults.MaxResults, 0)
				historyCh <- historyResult{results, err}
			}()

			go func() {
				favCh <- o.getFavoriteURLs(ctx)
			}()

			// Wait for both results
			hr := <-historyCh
			favoriteURLs := <-favCh

			if hr.err != nil {
				log.Error().Err(hr.err).Msg("failed to load recent history")
				return
			}
			if hr.results == nil {
				return
			}

			suggestions = make([]Suggestion, 0, len(hr.results))
			for _, r := range hr.results {
				_, isFav := favoriteURLs[r.URL]
				suggestions = append(suggestions, Suggestion{
					URL:        r.URL,
					Title:      r.Title,
					IsFavorite: isFav,
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

func (o *Omnibox) loadBangSuggestions(query string) {
	if o.shortcutsUC == nil {
		return
	}
	output := o.shortcutsUC.FilterBangs(o.ctx, usecase.FilterBangsInput{Query: query})
	suggestions := make([]BangSuggestion, len(output.Suggestions))
	for i, s := range output.Suggestions {
		suggestions[i] = BangSuggestion{Key: s.Key, Description: s.Description}
	}
	o.idleAddUpdateBangSuggestions(suggestions)
}

func (o *Omnibox) updateBangDetection(query string) {
	if o.shortcutsUC == nil {
		o.clearDetectedBang()
		return
	}
	output := o.shortcutsUC.DetectBangKey(o.ctx, usecase.DetectBangKeyInput{Query: query})
	if output.Key == "" {
		o.clearDetectedBang()
		return
	}

	o.setDetectedBang(output.Key, output.Description)
}

func (o *Omnibox) clearBangState() {
	o.mu.Lock()
	o.bangMode = false
	o.bangSuggestions = nil
	o.mu.Unlock()

	o.clearDetectedBang()
}

func (o *Omnibox) setDetectedBang(key, description string) {
	o.mu.Lock()
	o.detectedBang = key
	o.mu.Unlock()

	if o.bangBadge == nil {
		return
	}
	label := description
	var cb glib.SourceFunc = func(uintptr) bool {
		o.entry.AddCssClass("omnibox-entry-bang-active")
		o.bangBadge.SetText(label)
		o.bangBadge.SetVisible(true)
		return false
	}
	glib.IdleAdd(&cb, 0)
}

func (o *Omnibox) clearDetectedBang() {
	o.mu.Lock()
	o.detectedBang = ""
	o.mu.Unlock()

	if o.bangBadge == nil {
		return
	}
	var cb glib.SourceFunc = func(uintptr) bool {
		o.entry.RemoveCssClass("omnibox-entry-bang-active")
		o.bangBadge.SetVisible(false)
		return false
	}
	glib.IdleAdd(&cb, 0)
}

func (o *Omnibox) idleAddUpdateBangSuggestions(suggestions []BangSuggestion) {
	var cb glib.SourceFunc = func(uintptr) bool {
		o.updateBangSuggestions(suggestions)
		return false
	}
	glib.IdleAdd(&cb, 0)
}

func (o *Omnibox) updateBangSuggestions(suggestions []BangSuggestion) {
	o.mu.Lock()
	o.bangMode = true
	o.bangSuggestions = suggestions
	o.selectedIndex = -1
	o.mu.Unlock()

	o.rebuildList()

	rowCount := len(suggestions)
	if o.scrolledWin != nil {
		o.scrolledWin.SetVisible(rowCount > 0)
	}
	o.resizeAndCenter(rowCount)

	if rowCount > 0 {
		o.selectIndex(0)
	}
}

// updateSuggestions updates the list with history suggestions.
func (o *Omnibox) updateSuggestions(suggestions []Suggestion) {
	o.mu.Lock()
	o.bangMode = false
	o.bangSuggestions = nil
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
	o.bangMode = false
	o.bangSuggestions = nil
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
	bangMode := o.bangMode
	bangSuggestions := o.bangSuggestions
	o.mu.RUnlock()

	if bangMode {
		for i, b := range bangSuggestions {
			row := o.createBangRow(b, i)
			if row != nil {
				o.listBox.Append(&row.Widget)
			}
		}
	} else if mode == ViewModeHistory {
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
			forWidth := int(float64(width) * OmniboxSizeDefaults.WidthPct)
			if o.measureComponentHeights(forWidth) {
				// Re-trigger resize with accurate measurements
				o.mu.RLock()
				var count int
				if o.bangMode {
					count = len(o.bangSuggestions)
				} else if o.viewMode == ViewModeHistory {
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
func (o *Omnibox) createFaviconImage(rawURL, fallbackIcon string) *gtk.Image {
	favicon := gtk.NewImage()
	if favicon == nil {
		return nil
	}
	favicon.SetFromIconName(&fallbackIcon)
	favicon.SetPixelSize(int(16 * o.uiScale))
	favicon.AddCssClass("omnibox-favicon")

	// Async load favicon from cache
	if o.faviconAdapter != nil && rawURL != "" {
		o.faviconAdapter.GetOrFetch(o.ctx, rawURL, func(texture *gdk.Texture) {
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
// Uses rawURL for both favicon fetching and display.
func (o *Omnibox) createRowWithFavicon(rawURL, title, fallbackIcon string, index int) *gtk.ListBoxRow {
	return o.createRowWithFaviconURL(rawURL, title, rawURL, fallbackIcon, index)
}

// createRowWithFaviconURL creates a ListBoxRow with favicon, title, URL label, and shortcut badge.
// faviconURL is used for async favicon fetching (can be empty to skip), displayURL is shown as secondary label.
func (o *Omnibox) createRowWithFaviconURL(displayURL, title, faviconURL, fallbackIcon string, index int) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()
	if row == nil {
		return nil
	}
	row.AddCssClass("omnibox-row")

	const rowSpacing = 8
	hbox := gtk.NewBox(gtk.OrientationHorizontalValue, rowSpacing)
	if hbox == nil {
		return nil
	}
	hbox.SetHexpand(true)

	// Favicon image (vertically centered)
	if favicon := o.createFaviconImage(faviconURL, fallbackIcon); favicon != nil {
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
		displayTitle = displayURL
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
	if title != "" && title != displayURL {
		urlLabel := gtk.NewLabel(nil)
		if urlLabel != nil {
			urlLabel.SetText(displayURL)
			urlLabel.AddCssClass("omnibox-suggestion-url")
			urlLabel.SetHalign(gtk.AlignStartValue)
			urlLabel.SetEllipsize(2) // PANGO_ELLIPSIZE_END
			textBox.Append(&urlLabel.Widget)
		}
	}

	hbox.Append(&textBox.Widget)

	// Shortcut badge (Ctrl+1-9, Ctrl+0 for 10th)
	const maxShortcutIndex = 9
	if index <= maxShortcutIndex {
		shortcutLabel := gtk.NewLabel(nil)
		if shortcutLabel != nil {
			if index < maxShortcutIndex {
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
	row := o.createRowWithFavicon(s.URL, s.Title, "web-browser-symbolic", index)
	if row != nil && s.IsFavorite {
		row.AddCssClass("omnibox-row-favorite")
	}
	return row
}

// createFavoriteRow creates a ListBoxRow for a favorite.
func (o *Omnibox) createFavoriteRow(f Favorite, index int) *gtk.ListBoxRow {
	return o.createRowWithFavicon(f.URL, f.Title, "starred-symbolic", index)
}

func (o *Omnibox) createBangRow(b BangSuggestion, index int) *gtk.ListBoxRow {
	// Pass description as URL param (displayed as secondary label) and empty
	// faviconURL to skip async favicon fetching - bang rows use static icon only
	row := o.createRowWithFaviconURL(b.Description, "!"+b.Key, "", "system-search-symbolic", index)
	if row != nil {
		row.AddCssClass("omnibox-row-bang")
	}
	return row
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
	bangMode := o.bangMode
	var maxIndex int
	if bangMode {
		maxIndex = len(o.bangSuggestions) - 1
	} else if mode == ViewModeHistory {
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
	bangMode := o.bangMode
	var maxIndex int
	if bangMode {
		maxIndex = len(o.bangSuggestions) - 1
	} else if mode == ViewModeHistory {
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
	bangMode := o.bangMode
	bangSuggestions := o.bangSuggestions
	o.mu.RUnlock()

	entryText := o.entry.GetText()

	if bangMode {
		// If user typed a full bang query, navigate using the bang URL.
		if o.shortcutsUC != nil {
			navOutput := o.shortcutsUC.BuildNavigationText(o.ctx, usecase.BuildNavigationTextInput{EntryText: entryText})
			if navOutput.Valid {
				targetURL := o.buildURL(navOutput.Text)
				if targetURL == "" {
					return
				}
				o.Hide(o.ctx)
				if o.onNavigate != nil {
					o.onNavigate(targetURL)
				}
				return
			}
		}

		// Otherwise, Enter autocompletes the selected bang.
		if idx >= 0 && idx < len(bangSuggestions) {
			o.entry.SetText("!" + bangSuggestions[idx].Key + " ")
			o.entry.SetPosition(-1)
			return
		}
	}

	var targetURL string

	// Check if user typed a URL-like string (contains . and no spaces)
	// If so, navigate to that URL directly instead of the selection
	if o.looksLikeURL(entryText) {
		targetURL = o.buildURL(entryText)
	} else if idx < 0 {
		// No selection - use entry text as URL/search
		targetURL = o.buildURL(entryText)
	} else if mode == ViewModeHistory {
		if idx < len(suggestions) {
			targetURL = suggestions[idx].URL
		}
	} else {
		if idx < len(favorites) {
			targetURL = favorites[idx].URL
		}
	}

	if targetURL == "" {
		return
	}

	o.Hide(o.ctx)

	if o.onNavigate != nil {
		o.onNavigate(targetURL)
	}
}

// looksLikeURL checks if the text appears to be a URL (not a search query).
// Returns true for strings like "github.com", "google.com/search", etc.
func (o *Omnibox) looksLikeURL(text string) bool {
	return url.LooksLikeURL(text)
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
		// Toggle favorite in history mode
		if idx < 0 || idx >= len(suggestions) {
			log.Debug().Int("index", idx).Msg("toggle favorite: invalid selection")
			return
		}

		s := suggestions[idx]
		if s.URL == "" {
			log.Debug().Msg("toggle favorite: empty URL")
			return
		}

		go func() {
			ctx := o.ctx
			goLog := logging.FromContext(ctx)

			result, err := o.favoritesUC.Toggle(ctx, s.URL, s.Title)
			if err != nil {
				goLog.Error().Err(err).Str("url", s.URL).Msg("failed to toggle favorite")
				if o.onToast != nil {
					msg := "Failed to add favorite"
					if s.IsFavorite {
						msg = "Failed to remove favorite"
					}
					cb := glib.SourceFunc(func(_ uintptr) bool {
						o.onToast(ctx, msg, ToastError)
						return false
					})
					glib.IdleAdd(&cb, 0)
				}
				return
			}

			// Update suggestion state
			o.mu.Lock()
			if idx < len(o.suggestions) && o.suggestions[idx].URL == s.URL {
				o.suggestions[idx].IsFavorite = result.Added
			}
			o.mu.Unlock()

			// Update row CSS and show toast on GTK main thread
			cb := glib.SourceFunc(func(_ uintptr) bool {
				o.updateRowFavoriteIndicator(idx, result.Added)
				if o.onToast != nil {
					o.onToast(ctx, result.Message, ToastSuccess)
				}
				return false
			})
			glib.IdleAdd(&cb, 0)
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
				// Show error toast
				if o.onToast != nil {
					cb := glib.SourceFunc(func(_ uintptr) bool {
						o.onToast(ctx, "Failed to remove favorite", ToastError)
						return false
					})
					glib.IdleAdd(&cb, 0)
				}
				return
			}

			log.Info().Int64("id", f.ID).Str("url", f.URL).Msg("favorite removed from omnibox")

			// Refresh favorites list and show toast
			o.loadFavorites(o.entry.GetText())
			if o.onToast != nil {
				cb := glib.SourceFunc(func(_ uintptr) bool {
					o.onToast(ctx, "Favorite removed", ToastSuccess)
					return false
				})
				glib.IdleAdd(&cb, 0)
			}
		}()
	}
}

// updateRowFavoriteIndicator updates a single row's favorite indicator CSS class.
// Must be called from GTK main thread (via glib.IdleAdd).
func (o *Omnibox) updateRowFavoriteIndicator(index int, isFavorite bool) {
	row := o.listBox.GetRowAtIndex(index)
	if row == nil {
		return
	}
	if isFavorite {
		row.AddCssClass("omnibox-row-favorite")
	} else {
		row.RemoveCssClass("omnibox-row-favorite")
	}
}

// yankSelectedURL copies the URL of the selected item to clipboard.
func (o *Omnibox) yankSelectedURL() {
	log := logging.FromContext(o.ctx)

	if o.copyURLUC == nil {
		log.Warn().Msg("yank URL: copy URL use case is nil")
		return
	}

	o.mu.RLock()
	mode := o.viewMode
	idx := o.selectedIndex
	suggestions := o.suggestions
	favorites := o.favorites
	o.mu.RUnlock()

	var selectedURL string
	if mode == ViewModeHistory {
		if idx < 0 || idx >= len(suggestions) {
			log.Debug().Int("index", idx).Msg("yank URL: invalid selection")
			return
		}
		selectedURL = suggestions[idx].URL
	} else {
		if idx < 0 || idx >= len(favorites) {
			log.Debug().Int("index", idx).Msg("yank URL: invalid selection")
			return
		}
		selectedURL = favorites[idx].URL
	}

	if selectedURL == "" {
		log.Debug().Msg("yank URL: empty URL")
		return
	}

	go func() {
		ctx := o.ctx
		if err := o.copyURLUC.Copy(ctx, selectedURL); err != nil {
			return // Use case already logs the error
		}

		// Show toast notification on success (must run on GTK main thread)
		if o.onToast != nil {
			cb := glib.SourceFunc(func(_ uintptr) bool {
				o.onToast(ctx, "URL copied", ToastSuccess)
				return false // Don't repeat
			})
			glib.IdleAdd(&cb, 0)
		}
	}()
}

// buildURL constructs a URL from text, handling search shortcuts.
func (o *Omnibox) buildURL(text string) string {
	var shortcutURLs map[string]string
	if o.shortcutsUC != nil {
		shortcutURLs = o.shortcutsUC.ShortcutURLs()
	}
	return url.BuildSearchURL(text, shortcutURLs, o.defaultSearch)
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

	// Calculate dimensions using shared helper
	width, marginTop := CalculateModalDimensions(o.parentOverlay, OmniboxSizeDefaults)

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
	o.clearBangState()
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

// getFavoriteURLs returns a set of all favorited URLs for batch lookup.
// Returns empty map on error to avoid blocking history display.
func (o *Omnibox) getFavoriteURLs(ctx context.Context) map[string]struct{} {
	if o.favoritesUC == nil {
		return make(map[string]struct{})
	}
	urls, err := o.favoritesUC.GetAllURLs(ctx)
	if err != nil {
		log := logging.FromContext(ctx)
		log.Warn().Err(err).Msg("failed to load favorite URLs for indicator")
		return make(map[string]struct{})
	}
	return urls
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

	o.mu.Lock()
	o.visible = false
	o.mu.Unlock()

	o.parentOverlay = nil
	o.retainedCallbacks = nil

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
