package component

import (
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"
)

func (fs *FavoritesSidebar) setupKeyboardNavigation() {
	if fs == nil || fs.outerBox == nil {
		return
	}
	keyController := gtk.NewEventControllerKey()
	if keyController == nil {
		return
	}
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

		switch keyval {
		case uint(gdk.KEY_Escape):
			if searchEntry != nil && searchEntry.GetText() != "" {
				searchEntry.SetText("")
				return true
			}
			if onClose != nil {
				onClose()
			}
			return true
		case uint(gdk.KEY_Return), uint(gdk.KEY_KP_Enter):
			return fs.handleEnterKey(state)
		case uint(gdk.KEY_Page_Up):
			fs.selectAdjacentRow(-5)
			return true
		case uint(gdk.KEY_Page_Down):
			fs.selectAdjacentRow(5)
			return true
		case uint(gdk.KEY_Home):
			fs.mu.RLock()
			index := firstSelectableIndex(fs.displayRows)
			fs.mu.RUnlock()
			fs.selectIndex(index)
			return true
		case uint(gdk.KEY_End):
			fs.mu.RLock()
			index := lastSelectableIndex(fs.displayRows)
			fs.mu.RUnlock()
			fs.selectIndex(index)
			return true
		case uint(gdk.KEY_Up):
			fs.selectAdjacentRow(-1)
			return true
		case uint(gdk.KEY_Down):
			fs.selectAdjacentRow(1)
			return true
		case uint(gdk.KEY_slash):
			searchFocused := searchEntry != nil && searchEntry.Widget.HasFocus()
			if !shouldFocusSearchForSlash(searchFocused) {
				return false
			}
			fs.focusSearch()
			return true
		}
		return false
	}
	fs.retainedCallbacks = append(fs.retainedCallbacks, keyPressedCb)
	keyController.ConnectKeyPressed(&keyPressedCb)
	fs.outerBox.AddController(&keyController.EventController)
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
	fs.notice = err.Error()
	fs.mu.Unlock()
	fs.rebuildList()
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
