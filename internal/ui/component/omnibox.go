package component

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

const (
	omniboxWidth      = 600
	omniboxMaxResults = 10
	debounceDelayMs   = 250 // Increased for stability
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
	window       *gtk.Window
	mainBox      *gtk.Box
	headerBox    *gtk.Box
	historyBtn   *gtk.Button
	favoritesBtn *gtk.Button
	entry        *gtk.SearchEntry
	scrolledWin  *gtk.ScrolledWindow
	listBox      *gtk.ListBox

	// Parent window reference for positioning
	parentWindow *gtk.ApplicationWindow

	// State
	mu            sync.RWMutex
	visible       bool
	viewMode      ViewMode
	selectedIndex int
	suggestions   []Suggestion
	favorites     []Favorite

	// Dependencies
	historyUC       *usecase.SearchHistoryUseCase
	favoritesUC     *usecase.ManageFavoritesUseCase
	shortcuts       map[string]config.SearchShortcut
	defaultSearch   string
	initialBehavior string
	ctx             context.Context

	// Callbacks
	onNavigate func(url string)
	onClose    func()

	// Debouncing
	debounceTimer *time.Timer
	debounceMu    sync.Mutex
	lastQuery     string // Prevent duplicate searches
}

// OmniboxConfig holds configuration for creating an Omnibox.
type OmniboxConfig struct {
	HistoryUC       *usecase.SearchHistoryUseCase
	FavoritesUC     *usecase.ManageFavoritesUseCase
	Shortcuts       map[string]config.SearchShortcut
	DefaultSearch   string
	InitialBehavior string
}

// NewOmnibox creates a new native GTK4 omnibox widget.
func NewOmnibox(ctx context.Context, parent *gtk.ApplicationWindow, cfg OmniboxConfig) *Omnibox {
	log := logging.FromContext(ctx)

	o := &Omnibox{
		parentWindow:    parent,
		viewMode:        ViewModeHistory,
		selectedIndex:   -1,
		historyUC:       cfg.HistoryUC,
		favoritesUC:     cfg.FavoritesUC,
		shortcuts:       cfg.Shortcuts,
		defaultSearch:   cfg.DefaultSearch,
		initialBehavior: cfg.InitialBehavior,
		ctx:             ctx,
	}

	if err := o.createWidgets(); err != nil {
		log.Error().Err(err).Msg("failed to create omnibox widgets")
		return nil
	}

	o.setupKeyboardHandling()
	o.setupEntryChanged()

	log.Debug().Msg("omnibox created")
	return o
}

// createWidgets builds the GTK widget hierarchy.
func (o *Omnibox) createWidgets() error {
	// Create the popup window
	o.window = gtk.NewWindow()
	if o.window == nil {
		return errNilWidget("window")
	}

	o.window.SetModal(true)
	o.window.SetDecorated(false)
	o.window.SetDefaultSize(omniboxWidth, 400)
	o.window.AddCssClass("omnibox-window")

	// Set transient for parent window (positioning)
	if o.parentWindow != nil {
		o.window.SetTransientFor(&o.parentWindow.Window)
	}

	// Main vertical box
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

	o.headerBox.Append(&o.historyBtn.Widget)
	o.headerBox.Append(&o.favoritesBtn.Widget)

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
	o.window.SetChild(&o.mainBox.Widget)

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

	o.window.AddController(&controller.EventController)
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
		// Space toggles favorite: add in history mode, remove in favorites mode
		o.toggleFavorite()
		return true

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

	// Select first item if available
	if len(suggestions) > 0 {
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

	// Select first item if available
	if len(favorites) > 0 {
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
}

// createSuggestionRow creates a ListBoxRow for a suggestion.
func (o *Omnibox) createSuggestionRow(s Suggestion, index int) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()
	if row == nil {
		return nil
	}
	row.AddCssClass("omnibox-row")

	box := gtk.NewBox(gtk.OrientationHorizontalValue, 8)
	if box == nil {
		return nil
	}
	box.SetHexpand(true)

	// URL/Title label
	displayText := s.Title
	if displayText == "" {
		displayText = s.URL
	}
	label := gtk.NewLabel(nil)
	if label != nil {
		label.SetText(displayText) // Explicitly set text
		label.AddCssClass("omnibox-suggestion-title")
		label.SetHalign(gtk.AlignStartValue)
		label.SetHexpand(true)
		label.SetEllipsize(2) // PANGO_ELLIPSIZE_END
		box.Append(&label.Widget)
	}

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
			box.Append(&shortcutLabel.Widget)
		}
	}

	row.SetChild(&box.Widget)
	return row
}

// createFavoriteRow creates a ListBoxRow for a favorite.
func (o *Omnibox) createFavoriteRow(f Favorite, index int) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()
	if row == nil {
		return nil
	}
	row.AddCssClass("omnibox-row")

	box := gtk.NewBox(gtk.OrientationHorizontalValue, 8)
	if box == nil {
		return nil
	}
	box.SetHexpand(true)

	// Title label
	displayText := f.Title
	if displayText == "" {
		displayText = f.URL
	}
	label := gtk.NewLabel(nil)
	if label != nil {
		label.SetText(displayText)
		label.AddCssClass("omnibox-suggestion-title")
		label.SetHalign(gtk.AlignStartValue)
		label.SetHexpand(true)
		label.SetEllipsize(2)
		box.Append(&label.Widget)
	}

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
			box.Append(&shortcutLabel.Widget)
		}
	}

	row.SetChild(&box.Widget)
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
	o.mu.RLock()
	current := o.selectedIndex
	mode := o.viewMode
	var maxIndex int
	if mode == ViewModeHistory {
		maxIndex = len(o.suggestions) - 1
	} else {
		maxIndex = len(o.favorites) - 1
	}
	o.mu.RUnlock()

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
	o.mu.RLock()
	current := o.selectedIndex
	mode := o.viewMode
	var maxIndex int
	if mode == ViewModeHistory {
		maxIndex = len(o.suggestions) - 1
	} else {
		maxIndex = len(o.favorites) - 1
	}
	o.mu.RUnlock()

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

// navigateToSelected navigates to the currently selected item.
func (o *Omnibox) navigateToSelected() {
	o.mu.RLock()
	mode := o.viewMode
	idx := o.selectedIndex
	suggestions := o.suggestions
	favorites := o.favorites
	o.mu.RUnlock()

	var url string

	if idx < 0 {
		// No selection - use entry text as URL
		url = o.buildURL(o.entry.GetText())
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

	// Check if it looks like a URL
	if strings.Contains(input, ".") && !strings.Contains(input, " ") {
		if !strings.HasPrefix(input, "http://") && !strings.HasPrefix(input, "https://") {
			return "https://" + input
		}
		return input
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

	// Load initial data
	o.performSearch()

	// Present the window
	o.window.Present()

	// Focus the entry
	o.entry.GrabFocus()
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

	// Hide window
	o.window.SetVisible(false)

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

// Destroy cleans up omnibox resources.
func (o *Omnibox) Destroy() {
	o.debounceMu.Lock()
	if o.debounceTimer != nil {
		o.debounceTimer.Stop()
	}
	o.debounceMu.Unlock()

	if o.window != nil {
		o.window.Destroy()
		o.window = nil
	}
}

// formatShortcut formats a shortcut number.
func formatShortcut(n int) string {
	return "Ctrl+" + string(rune('0'+n))
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
