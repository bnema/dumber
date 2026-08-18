package component

import (
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/domain/entity"
)

//nolint:gocyclo,funlen // Centralized local key routing keeps sidebar keyboard behavior easy to audit.
func (fs *FavoritesSidebar) setupKeyboardNavigation() {
	if fs == nil || fs.outerBox == nil {
		return
	}
	keyController := gtk.NewEventControllerKey()
	if keyController == nil {
		return
	}
	// Directional controls must run before GtkSearchEntry and GtkListBox consume
	// arrows. Explicit text-input focus tracking below still defers printable
	// keys to their focused entry.
	keyController.SetPropagationPhase(gtk.PhaseCaptureValue)
	keyPressedCb := func(_ gtk.EventControllerKey, keyval uint, _ uint, state gdk.ModifierType) bool {
		fs.mu.RLock()
		if fs.destroyed {
			fs.mu.RUnlock()
			return false
		}
		searchEntry := fs.searchEntry
		onClose := fs.onClose
		fs.mu.RUnlock()

		textEditing := fs.inTextEditContext()
		if fs.isTextInputFocused(fs.tagNameEntry) &&
			(keyval == uint(gdk.KEY_Return) || keyval == uint(gdk.KEY_KP_Enter)) {
			return fs.createTagFromFocusedEntry()
		}
		if keyval == uint(gdk.KEY_Up) || keyval == uint(gdk.KEY_Down) {
			if fs.routeDirectionalFocus(keyval) {
				return true
			}
		}
		if shouldDeferToTextInput(textEditing, keyval) {
			return false
		}
		switch keyval {
		case uint(gdk.KEY_Escape):
			if fs.cancelManagement() {
				return true
			}
			if searchEntry != nil && searchEntry.GetText() != "" {
				searchEntry.SetText("")
				return true
			}
			if onClose != nil {
				onClose()
			}
			return true
		case uint(gdk.KEY_Return), uint(gdk.KEY_KP_Enter):
			if fs.createTagFromFocusedEntry() {
				return true
			}
			if fs.focusedTagControlIndex() >= 0 {
				return false
			}
			return fs.handleReturnKey(state)
		case uint(gdk.KEY_Tab), uint(gdk.KEY_ISO_Left_Tab):
			fs.cycleFocusZone(state&gdk.ShiftMaskValue != 0)
			return true
		case uint(gdk.KEY_Page_Up):
			if textEditing {
				return false
			}
			fs.selectAdjacentRow(-5)
			return true
		case uint(gdk.KEY_Page_Down):
			if textEditing {
				return false
			}
			fs.selectAdjacentRow(5)
			return true
		case uint(gdk.KEY_Home):
			if textEditing {
				return false
			}
			fs.mu.RLock()
			index := firstSelectableIndex(fs.displayRows)
			fs.mu.RUnlock()
			fs.selectIndex(index)
			return true
		case uint(gdk.KEY_End):
			if textEditing {
				return false
			}
			fs.mu.RLock()
			index := lastSelectableIndex(fs.displayRows)
			fs.mu.RUnlock()
			fs.selectIndex(index)
			return true
		case uint(gdk.KEY_Up):
			if textEditing {
				return false
			}
			fs.selectAdjacentRow(-1)
			return true
		case uint(gdk.KEY_Down):
			if textEditing {
				return false
			}
			fs.selectAdjacentRow(1)
			return true
		case uint(gdk.KEY_plus), uint(gdk.KEY_KP_Add):
			if !textEditing && fs.listHasFocus() {
				return fs.showTagBindingPicker()
			}
			return false
		case uint(gdk.KEY_slash):
			if textEditing {
				return false
			}
			searchFocused := searchEntry != nil && searchEntry.HasFocus()
			if !shouldFocusSearchForSlash(searchFocused) {
				return false
			}
			fs.focusSearch()
			return true
		}
		return fs.handleSingleKeyCommand(keyval, state)
	}
	fs.retainedCallbacks = append(fs.retainedCallbacks, keyPressedCb)
	keyController.ConnectKeyPressed(&keyPressedCb)
	fs.outerBox.AddController(&keyController.EventController)
}

func (fs *FavoritesSidebar) handleReturnKey(state gdk.ModifierType) bool {
	if fs == nil {
		return false
	}
	if fs.confirmDeleteActive() {
		return fs.confirmDeleteFavorite()
	}
	formActive := fs.formModeActive()
	if state&gdk.ControlMaskValue != 0 && fs.submitForm() {
		return true
	}
	if formActive {
		return false
	}
	return fs.handleEnterKey(state)
}

func (fs *FavoritesSidebar) handleEnterKey(state gdk.ModifierType) bool {
	if fs == nil {
		return false
	}
	url := fs.selectedEnterURL()
	if url == "" {
		return false
	}
	return fs.dispatchEnterURL(state, url)
}

func (fs *FavoritesSidebar) selectedEnterURL() string {
	if fs == nil {
		return ""
	}
	if fs.listBox == nil {
		fs.mu.RLock()
		defer fs.mu.RUnlock()
		if fs.destroyed || len(fs.displayRows) != 1 {
			return ""
		}
		return fs.rowURLAt(0)
	}
	row := fs.listBox.GetSelectedRow()
	if row == nil || !row.GetSelectable() {
		return ""
	}
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	if fs.destroyed {
		return ""
	}
	return fs.rowURLAt(row.GetIndex())
}

func (fs *FavoritesSidebar) onRowActivated(row *gtk.ListBoxRow) {
	if row == nil || !row.GetSelectable() {
		return
	}
	fs.mu.RLock()
	if fs.destroyed {
		fs.mu.RUnlock()
		return
	}
	url := fs.rowURLAt(row.GetIndex())
	fs.mu.RUnlock()
	fs.navigateToURL(url)
}

func (fs *FavoritesSidebar) selectAdjacentRow(delta int) {
	if fs == nil || fs.listBox == nil {
		return
	}
	current := -1
	if row := fs.listBox.GetSelectedRow(); row != nil {
		current = row.GetIndex()
	}
	fs.mu.RLock()
	if fs.destroyed {
		fs.mu.RUnlock()
		return
	}
	target := current
	step := 1
	if delta < 0 {
		step = -1
	}
	for i := 0; i < abs(delta); i++ {
		next := nextSelectableIndex(fs.displayRows, target, step)
		if next == -1 {
			break
		}
		target = next
	}
	if current < 0 {
		if step > 0 {
			target = firstSelectableIndex(fs.displayRows)
		} else {
			target = lastSelectableIndex(fs.displayRows)
		}
	}
	fs.mu.RUnlock()
	fs.selectIndex(target)
}

func (fs *FavoritesSidebar) selectIndex(index int) {
	if fs == nil || index < 0 {
		return
	}
	fs.mu.RLock()
	destroyed := fs.destroyed
	listBox := fs.listBox
	fs.mu.RUnlock()
	if destroyed || listBox == nil {
		return
	}
	if row := listBox.GetRowAtIndex(index); row != nil && row.GetSelectable() {
		listBox.SelectRow(row)
	}
}

func (fs *FavoritesSidebar) navigateToURL(url string) {
	if fs == nil || fs.onNavigate == nil || url == "" {
		return
	}
	fs.scheduleIdle(glib.SourceFunc(func(uintptr) bool {
		fs.mu.RLock()
		destroyed := fs.destroyed
		fs.mu.RUnlock()
		if !destroyed {
			fs.handleNavigationError(fs.onNavigate(fs.ctx, url))
		}
		return false
	}))
}

func (fs *FavoritesSidebar) navigateWithoutClosing(url string) {
	if fs == nil || url == "" {
		return
	}
	cb := fs.onNavigateKeepOpen
	if cb == nil {
		cb = fs.onNavigate
	}
	if cb == nil {
		return
	}
	fs.scheduleIdle(glib.SourceFunc(func(uintptr) bool {
		fs.mu.RLock()
		destroyed := fs.destroyed
		fs.mu.RUnlock()
		if !destroyed {
			fs.handleNavigationError(cb(fs.ctx, url))
		}
		return false
	}))
}

func (fs *FavoritesSidebar) navigateToNewPane(url string) {
	if fs == nil || fs.onOpenInNewPane == nil || url == "" {
		return
	}
	fs.scheduleIdle(glib.SourceFunc(func(uintptr) bool {
		fs.mu.RLock()
		destroyed := fs.destroyed
		fs.mu.RUnlock()
		if !destroyed {
			fs.handleNavigationError(fs.onOpenInNewPane(fs.ctx, url))
		}
		return false
	}))
}

func shouldFocusSearchForSlash(searchFocused bool) bool { return !searchFocused }

// shouldDeferToTextInput keeps the sidebar controller from treating text typed
// into a descendant entry as a global sidebar command.
// Tab traverses focus and Escape cancels sidebar management while text is focused.
func shouldDeferToTextInput(textEditing bool, keyval uint) bool {
	return textEditing && keyval != uint(gdk.KEY_Tab) &&
		keyval != uint(gdk.KEY_ISO_Left_Tab) && keyval != uint(gdk.KEY_Escape)
}

func (fs *FavoritesSidebar) routeDirectionalFocus(keyval uint) bool {
	if fs == nil {
		return false
	}
	fs.mu.RLock()
	zone := fs.focusZone
	fs.mu.RUnlock()
	switch {
	case zone == favoritesSidebarFocusTags:
		// Tag filters and creation are deliberately Tab-only controls.
		return true
	case keyval == uint(gdk.KEY_Down) && zone == favoritesSidebarFocusSearch:
		fs.focusListAndSelectFirst()
		return true
	case keyval == uint(gdk.KEY_Up) && zone == favoritesSidebarFocusList && fs.selectedRowIsFirst():
		fs.setFocusZone(favoritesSidebarFocusSearch)
		if fs.searchEntry != nil {
			fs.searchEntry.GrabFocus()
		}
		return true
	}
	return false
}

func (fs *FavoritesSidebar) focusListAndSelectFirst() {
	if fs == nil {
		return
	}
	fs.setFocusZone(favoritesSidebarFocusList)
	if fs.listBox == nil {
		return
	}
	fs.listBox.GrabFocus()
	fs.mu.RLock()
	index := firstSelectableIndex(fs.displayRows)
	fs.mu.RUnlock()
	fs.selectIndex(index)
}

func (fs *FavoritesSidebar) selectedRowIsFirst() bool {
	if fs == nil {
		return false
	}
	if fs.listBox == nil {
		return true
	}
	row := fs.listBox.GetSelectedRow()
	if row == nil {
		return true
	}
	fs.mu.RLock()
	first := firstSelectableIndex(fs.displayRows)
	fs.mu.RUnlock()
	return row.GetIndex() == first
}

func (fs *FavoritesSidebar) listHasFocus() bool {
	if fs == nil {
		return false
	}
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return fs.focusZone == favoritesSidebarFocusList
}

func (fs *FavoritesSidebar) inTextEditContext() bool {
	if fs == nil {
		return false
	}
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return fs.textInputWidget != nil
}

func (fs *FavoritesSidebar) isTextInputFocused(widget *gtk.Entry) bool {
	if fs == nil || widget == nil {
		return false
	}
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return fs.textInputWidget == &widget.Widget
}

func (fs *FavoritesSidebar) handleSingleKeyCommand(keyval uint, state gdk.ModifierType) bool {
	shortcutModifiers := gdk.ControlMaskValue | gdk.ShiftMaskValue | gdk.AltMaskValue
	if fs == nil || fs.inTextEditContext() || state&shortcutModifiers != 0 {
		return false
	}
	fs.mu.RLock()
	mode := fs.mode
	confirm := fs.confirmDelete
	fs.mu.RUnlock()
	if mode == favoritesSidebarModeShortcut {
		if keyval >= uint(gdk.KEY_1) && keyval <= uint(gdk.KEY_9) {
			v := int(keyval - uint(gdk.KEY_0))
			return fs.setShortcutKey(&v)
		}
		if keyval == uint(gdk.KEY_BackSpace) || keyval == uint(gdk.KEY_Delete) {
			return fs.setShortcutKey(nil)
		}
	}
	if keyval == uint(gdk.KEY_Delete) {
		if confirm {
			return fs.confirmDeleteFavorite()
		}
		return fs.requestDeleteConfirmation()
	}
	switch keyval {
	case uint(gdk.KEY_a):
		fs.beginAddForm()
		return true
	case uint(gdk.KEY_e):
		fs.beginEditForm()
		return true
	case uint(gdk.KEY_s):
		fs.enterShortcutMode()
		return true
	case uint(gdk.KEY_r):
		fs.startLoad()
		return true
	case uint(gdk.KEY_c):
		fs.clearQueryAndTags()
		return true
	}
	return false
}

func (fs *FavoritesSidebar) clearQueryAndTags() {
	if fs == nil {
		return
	}
	if fs.searchEntry != nil {
		fs.searchEntry.SetText("")
	}
	fs.mu.Lock()
	if !fs.destroyed {
		fs.currentQuery = ""
		fs.selectedTagIDs = make(map[entity.TagID]struct{})
		fs.rebuildDisplayRowsLocked()
	}
	fs.mu.Unlock()
	fs.renderTags()
	fs.rebuildList()
}

func (fs *FavoritesSidebar) cycleFocusZone(reverse bool) {
	if fs == nil {
		return
	}
	if fs.cycleTagControlFocus(reverse) {
		return
	}
	fs.mu.RLock()
	current := fs.focusZone
	tagCount := len(fs.tagControls)
	fs.mu.RUnlock()
	if reverse && current == favoritesSidebarFocusList && tagCount > 0 {
		fs.focusTagControl(tagCount - 1)
		return
	}
	zones := fs.availableFocusZones()
	if len(zones) == 0 {
		return
	}
	idx := -1
	for i, zone := range zones {
		if zone == current {
			idx = i
			break
		}
	}
	if reverse {
		idx--
		if idx < 0 {
			idx = len(zones) - 1
		}
	} else {
		idx++
		if idx >= len(zones) {
			idx = 0
		}
	}
	fs.focusZoneWidget(zones[idx])
}

func (fs *FavoritesSidebar) cycleTagControlFocus(reverse bool) bool {
	fs.mu.RLock()
	inTags := fs.focusZone == favoritesSidebarFocusTags
	controls := append([]*gtk.Button(nil), fs.tagControls...)
	fs.mu.RUnlock()
	if !inTags || len(controls) == 0 {
		return false
	}
	index := fs.focusedTagControlIndex()
	if index < 0 {
		if reverse {
			fs.focusTagControl(len(controls) - 1)
		} else {
			fs.focusTagControl(0)
		}
		return true
	}
	if reverse {
		if index > 0 {
			fs.focusTagControl(index - 1)
			return true
		}
		return false
	}
	if index < len(controls)-1 {
		fs.focusTagControl(index + 1)
		return true
	}
	return false
}

func (fs *FavoritesSidebar) availableFocusZones() []favoritesSidebarFocusZone {
	zones := []favoritesSidebarFocusZone{}
	if fs.searchEntry != nil {
		zones = append(zones, favoritesSidebarFocusSearch)
	}
	if fs.tagBox != nil {
		zones = append(zones, favoritesSidebarFocusTags)
	}
	if fs.listBox != nil {
		zones = append(zones, favoritesSidebarFocusList)
	}
	fs.mu.RLock()
	formActive := fs.mode == favoritesSidebarModeAdd || fs.mode == favoritesSidebarModeEdit
	confirmActive := fs.confirmDelete
	fs.mu.RUnlock()
	if formActive && fs.formBox != nil {
		zones = append(zones, favoritesSidebarFocusForm)
	}
	if confirmActive {
		zones = append(zones, favoritesSidebarFocusConfirm)
	}
	return zones
}

func (fs *FavoritesSidebar) focusZoneWidget(zone favoritesSidebarFocusZone) {
	fs.mu.Lock()
	if !fs.destroyed {
		fs.focusZone = zone
	}
	fs.mu.Unlock()
	switch zone {
	case favoritesSidebarFocusSearch:
		fs.focusSearch()
	case favoritesSidebarFocusTags:
		fs.focusTagControl(0)
	case favoritesSidebarFocusList, favoritesSidebarFocusConfirm:
		if fs.listBox != nil {
			fs.listBox.GrabFocus()
		}
	case favoritesSidebarFocusForm:
		fs.focusForm()
	}
}

func (fs *FavoritesSidebar) dispatchEnterURL(state gdk.ModifierType, url string) bool {
	if fs == nil || url == "" {
		return false
	}
	if state&gdk.ShiftMaskValue != 0 {
		fs.navigateToNewPane(url)
	} else if state&gdk.ControlMaskValue != 0 {
		fs.navigateWithoutClosing(url)
	} else {
		fs.navigateToURL(url)
	}
	return true
}

func (fs *FavoritesSidebar) handleNavigationError(err error) {
	if fs == nil || err == nil {
		return
	}
	fs.mu.Lock()
	if fs.destroyed {
		fs.mu.Unlock()
		return
	}
	fs.setNoticeLocked(err.Error())
	fs.mu.Unlock()
	fs.rebuildList()
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
