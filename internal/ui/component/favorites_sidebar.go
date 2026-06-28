package component

import (
	"context"
	"fmt"
	"sync"

	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
)

// FavoritesSidebarConfig holds configuration for creating a FavoritesSidebar.
type FavoritesSidebarConfig struct {
	FavoritesUC        port.FavoritesSidebarFavorites
	OnNavigate         func(ctx context.Context, url string) error
	OnNavigateKeepOpen func(ctx context.Context, url string) error
	OnOpenInNewPane    func(ctx context.Context, url string) error
	OnClose            func()
}

// FavoritesSidebar is a read-only GTK sidebar component for browsing favorites.
type FavoritesSidebar struct {
	outerBox    *gtk.Box
	searchEntry *gtk.SearchEntry
	tagBox      *gtk.Box
	scrolledWin *gtk.ScrolledWindow
	listBox     *gtk.ListBox
	formBox     *gtk.Box

	formURLEntry      *gtk.SearchEntry
	formTitleEntry    *gtk.SearchEntry
	formTagsEntry     *gtk.SearchEntry
	formShortcutEntry *gtk.SearchEntry
	formSaveButton    *gtk.Button
	formURL           string
	formTitle         string
	formTags          string
	formShortcut      string

	favoritesUC        port.FavoritesSidebarFavorites
	onNavigate         func(ctx context.Context, url string) error
	onNavigateKeepOpen func(ctx context.Context, url string) error
	onOpenInNewPane    func(ctx context.Context, url string) error
	onClose            func()

	allFavorites   []*entity.Favorite
	allTags        []*entity.Tag
	selectedTagIDs map[entity.TagID]struct{}
	displayRows    []favoriteSidebarDisplayRow
	currentQuery   string
	notice         string
	loadGen        uint64
	destroyed      bool
	visible        bool
	focusZone      favoritesSidebarFocusZone
	mode           favoritesSidebarMode
	editingID      entity.FavoriteID
	confirmDelete  bool

	retainedCallbacks []interface{}
	tagCallbacks      []interface{}
	ctx               context.Context
	cancel            context.CancelFunc
	mu                sync.RWMutex

	idleScheduler func(glib.SourceFunc)
}

// NewFavoritesSidebar creates a native favorites sidebar and starts loading data.
func NewFavoritesSidebar(ctx context.Context, cfg FavoritesSidebarConfig) *FavoritesSidebar {
	ctx, cancel := context.WithCancel(ctx)
	fs := &FavoritesSidebar{
		favoritesUC:        cfg.FavoritesUC,
		onNavigate:         cfg.OnNavigate,
		onNavigateKeepOpen: cfg.OnNavigateKeepOpen,
		onOpenInNewPane:    cfg.OnOpenInNewPane,
		onClose:            cfg.OnClose,
		selectedTagIDs:     make(map[entity.TagID]struct{}),
		ctx:                ctx,
		cancel:             cancel,
	}
	if err := fs.createWidgets(); err != nil {
		cancel()
		return nil
	}
	fs.setupSearchHandler()
	fs.setupKeyboardNavigation()
	fs.startLoad()
	return fs
}

// Widget returns the raw GTK widget.
func (fs *FavoritesSidebar) Widget() *gtk.Widget {
	if fs == nil || fs.outerBox == nil {
		return nil
	}
	return &fs.outerBox.Widget
}

// Show displays the sidebar and focuses search. Nil-safe.
func (fs *FavoritesSidebar) Show() {
	if fs == nil {
		return
	}
	fs.mu.Lock()
	if fs.destroyed || fs.outerBox == nil {
		fs.mu.Unlock()
		return
	}
	fs.outerBox.SetVisible(true)
	fs.visible = true
	fs.mu.Unlock()
	fs.focusSearch()
}

// Hide hides the sidebar. Nil-safe.
func (fs *FavoritesSidebar) Hide() {
	if fs == nil {
		return
	}
	fs.mu.Lock()
	if fs.destroyed {
		fs.mu.Unlock()
		return
	}
	if fs.outerBox != nil {
		fs.outerBox.SetVisible(false)
	}
	fs.visible = false
	fs.mu.Unlock()
}

// Destroy cancels background work and marks the sidebar destroyed. Nil-safe.
func (fs *FavoritesSidebar) Destroy() {
	if fs == nil {
		return
	}
	fs.mu.Lock()
	if fs.destroyed {
		fs.mu.Unlock()
		return
	}
	fs.destroyed = true
	fs.mu.Unlock()
	if fs.cancel != nil {
		fs.cancel()
	}
}

func (fs *FavoritesSidebar) createWidgets() error {
	fs.outerBox = gtk.NewBox(gtk.OrientationVerticalValue, 6)
	if fs.outerBox == nil {
		return fmt.Errorf("favorites sidebar: outer box creation failed")
	}
	fs.outerBox.AddCssClass("favorites-sidebar-outer")
	fs.outerBox.SetSizeRequest(sidebarMinWidth, -1)
	fs.outerBox.SetHexpand(false)
	fs.outerBox.SetVexpand(true)
	fs.outerBox.SetVisible(false)

	fs.searchEntry = gtk.NewSearchEntry()
	if fs.searchEntry == nil {
		return fmt.Errorf("favorites sidebar: search entry creation failed")
	}
	fs.searchEntry.AddCssClass("favorites-sidebar-search")
	fs.searchEntry.SetHexpand(true)
	placeholder := "Search favorites"
	fs.searchEntry.SetPlaceholderText(&placeholder)
	fs.outerBox.Append(&fs.searchEntry.Widget)

	fs.tagBox = gtk.NewBox(gtk.OrientationHorizontalValue, 4)
	if fs.tagBox == nil {
		return fmt.Errorf("favorites sidebar: tag box creation failed")
	}
	fs.tagBox.AddCssClass("favorites-sidebar-tags")
	fs.tagBox.SetHexpand(true)
	fs.outerBox.Append(&fs.tagBox.Widget)

	fs.scrolledWin = gtk.NewScrolledWindow()
	if fs.scrolledWin == nil {
		return fmt.Errorf("favorites sidebar: scrolled window creation failed")
	}
	fs.scrolledWin.SetVexpand(true)
	fs.scrolledWin.SetHexpand(true)
	fs.scrolledWin.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)

	fs.listBox = gtk.NewListBox()
	if fs.listBox == nil {
		return fmt.Errorf("favorites sidebar: list box creation failed")
	}
	fs.listBox.AddCssClass("favorites-sidebar-list")
	fs.listBox.SetActivateOnSingleClick(true)
	fs.listBox.SetSelectionMode(gtk.SelectionSingleValue)
	rowActivatedCb := func(_ gtk.ListBox, rowPtr uintptr) {
		row := gtk.ListBoxRowNewFromInternalPtr(rowPtr)
		fs.onRowActivated(row)
	}
	fs.retainedCallbacks = append(fs.retainedCallbacks, rowActivatedCb)
	fs.listBox.ConnectRowActivated(&rowActivatedCb)

	fs.scrolledWin.SetChild(&fs.listBox.Widget)
	fs.outerBox.Append(&fs.scrolledWin.Widget)

	fs.formBox = gtk.NewBox(gtk.OrientationVerticalValue, 4)
	if fs.formBox == nil {
		return fmt.Errorf("favorites sidebar: form box creation failed")
	}
	fs.formBox.AddCssClass("favorites-sidebar-form")
	fs.formBox.SetVisible(false)
	fs.outerBox.Append(&fs.formBox.Widget)
	return nil
}

func (fs *FavoritesSidebar) focusSearch() {
	if fs == nil {
		return
	}
	fs.scheduleIdle(glib.SourceFunc(func(uintptr) bool {
		fs.mu.RLock()
		destroyed := fs.destroyed
		entry := fs.searchEntry
		fs.mu.RUnlock()
		if !destroyed && entry != nil {
			entry.GrabFocus()
		}
		return false
	}))
}

func (fs *FavoritesSidebar) startLoad() {
	if fs == nil {
		return
	}
	fs.mu.Lock()
	if fs.destroyed {
		fs.mu.Unlock()
		return
	}
	fs.loadGen++
	gen := fs.loadGen
	uc := fs.favoritesUC
	ctx := fs.ctx
	fs.notice = "Loading favorites..."
	fs.mu.Unlock()

	if uc == nil || ctx == nil {
		fs.applyLoadedData(nil, nil, gen, nil)
		return
	}
	go func() {
		favorites, favErr := uc.GetAll(ctx)
		tags, tagErr := uc.GetAllTags(ctx)
		err := favErr
		if err == nil {
			err = tagErr
		}
		fs.scheduleIdle(glib.SourceFunc(func(uintptr) bool {
			fs.applyLoadedData(favorites, tags, gen, err)
			return false
		}))
	}()
}

func (fs *FavoritesSidebar) applyLoadedData(favorites []*entity.Favorite, tags []*entity.Tag, gen uint64, err error) bool {
	if fs == nil {
		return false
	}
	fs.mu.Lock()
	if fs.destroyed || gen != fs.loadGen {
		fs.mu.Unlock()
		return false
	}
	if favorites == nil {
		favorites = []*entity.Favorite{}
	}
	if tags == nil {
		tags = []*entity.Tag{}
	}
	fs.allFavorites = favorites
	fs.allTags = tags
	if err != nil {
		fs.notice = err.Error()
	} else {
		fs.notice = ""
	}
	fs.rebuildDisplayRowsLocked()
	fs.mu.Unlock()
	fs.renderTags()
	fs.rebuildList()
	return true
}

func (fs *FavoritesSidebar) setupSearchHandler() {
	if fs == nil || fs.searchEntry == nil {
		return
	}
	changedCb := func(_ gtk.SearchEntry) {
		fs.mu.RLock()
		destroyed := fs.destroyed
		fs.mu.RUnlock()
		if destroyed {
			return
		}
		fs.onSearchChanged()
	}
	fs.retainedCallbacks = append(fs.retainedCallbacks, changedCb)
	fs.searchEntry.ConnectSearchChanged(&changedCb)
}

func (fs *FavoritesSidebar) onSearchChanged() {
	fs.mu.Lock()
	if fs.destroyed {
		fs.mu.Unlock()
		return
	}
	if fs.searchEntry != nil {
		fs.currentQuery = fs.searchEntry.GetText()
	}
	fs.rebuildDisplayRowsLocked()
	fs.mu.Unlock()
	fs.rebuildList()
}

func (fs *FavoritesSidebar) rebuildDisplayRowsLocked() {
	query := usecase.FavoriteSidebarQuery{Text: fs.currentQuery}
	for id := range fs.selectedTagIDs {
		query.TagIDs = append(query.TagIDs, id)
	}
	filtered := usecase.FilterFavoritesForSidebar(fs.allFavorites, query)
	fs.displayRows = buildFavoriteSidebarDisplayRows(filtered)
}

func (fs *FavoritesSidebar) scheduleIdle(cb glib.SourceFunc) {
	if fs != nil && fs.idleScheduler != nil {
		fs.idleScheduler(cb)
		return
	}
	glib.IdleAdd(&cb, 0)
}
