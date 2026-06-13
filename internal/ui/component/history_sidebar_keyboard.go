package component

import (
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/domain/entity"
)

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
			hs.applyDeletedEntryLocked(url, entryID, nextSelectedURL)
			hs.mu.Unlock()

			hs.rebuildList()
			return false
		})
		hs.scheduleIdle(cb)
	}()

	return true
}

// applyDeletedEntryLocked updates local sidebar state after a successful
// history delete. Must be called with hs.mu write lock held.
func (hs *HistorySidebar) applyDeletedEntryLocked(url string, entryID int64, nextSelectedURL string) {
	hs.preserveScrollAndSelection()
	hs.prevSelectedURL = nextSelectedURL
	hs.searchGen++
	hs.loadGen++
	hs.isLoading = false
	hs.loadStarted = false
	hs.removeFromAllEntries(url, entryID)
	hs.totalLoaded = len(hs.allEntries)
	hs.removeFromSearchResults(entryID)
	hs.rebuildLocalGroups()
}

// findEntryIDByIndex returns the entry ID for the linear ListBox index.
// Must be called with hs.mu read lock held.
func (hs *HistorySidebar) findEntryIDByIndex(index int) int64 {
	entry := newKeyboardNavModelFromRows(hs.displayRows).entryAt(index)
	if entry == nil {
		return 0
	}
	return entry.ID
}

// rebuildLocalGroups rebuilds hs.groups from the current allEntries and query.
// Must be called with hs.mu write lock held.
func (hs *HistorySidebar) rebuildLocalGroups() {
	if hs.currentQuery == "" {
		hs.setDisplayGroupsLocked(groupHistoryByDay(hs.allEntries))
	} else if hs.searchResults != nil {
		hs.setDisplayGroupsLocked(groupHistoryByDay(hs.searchResults))
	} else {
		// For search mode, the search results are handled by doFTSearch.
		// Removing an entry while in search mode would need a re-search.
		// Fall back to grouping searchResults if they exist.
		hs.setDisplayGroupsLocked(nil)
	}
}

// removeFromAllEntries removes all history entries matching the given URL or ID
// from hs.allEntries. Must be called with hs.mu write lock held.
func (hs *HistorySidebar) removeFromAllEntries(_ string, id int64) {
	filtered := make([]*entity.HistoryEntry, 0, len(hs.allEntries))
	for _, e := range hs.allEntries {
		if e != nil && e.ID == id {
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
	model := newKeyboardNavModelFromRows(hs.displayRows)
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
	targetIdx := newKeyboardNavModelFromRows(hs.displayRows).previousDayBoundary(currentIdx)
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
	targetIdx := newKeyboardNavModelFromRows(hs.displayRows).nextDayBoundary(currentIdx)
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
	// GetRowAtIndex returns nil once the index is past the end of the list,
	// so this scan terminates naturally at the first missing row.
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
	model := newKeyboardNavModelFromRows(hs.displayRows)
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
	entry := newKeyboardNavModelFromRows(hs.displayRows).entryAt(row.GetIndex())
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
	hs.scheduleIdle(navigateCb)
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
	hs.scheduleIdle(navigateCb)
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
	hs.scheduleIdle(navigateCb)
}

// closeSidebar calls the configured OnClose callback to tell the host to
// hide the sidebar and restore focus to the active content pane/webview.
func (hs *HistorySidebar) closeSidebar() {
	if hs.onClose != nil {
		hs.onClose()
	}
}
