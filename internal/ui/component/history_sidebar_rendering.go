package component

import (
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/bnema/puregotk/v4/pango"

	"github.com/bnema/dumber/internal/domain/entity"
)

// =============================================================================
// List rendering
// =============================================================================

// rebuildList clears and repopulates the list box from current groups.
// Must be called on the GTK main thread. Preserves scroll and selection.
func (hs *HistorySidebar) rebuildList() {
	hs.mu.Lock()
	if hs.destroyed || hs.listBox == nil {
		hs.mu.Unlock()
		return
	}
	listBox := hs.listBox
	rows := append([]historyDisplayRow(nil), hs.displayRows...)
	query := hs.currentQuery
	hasSearchResults := hs.searchResults != nil
	searchDone := hs.searchDone
	totalLoaded := hs.totalLoaded
	isLoading := hs.isLoading
	hs.clearRelativeTimeBindingsLocked()
	hs.mu.Unlock()

	// Remove all rows
	listBox.RemoveAll()

	if len(rows) == 0 {
		if !hasSearchResults && totalLoaded == 0 {
			// Browse has not loaded yet AND no search has completed.
			hs.showLoadingOrEmpty(listBox, query, searchDone, isLoading)
			return
		}
		// Search completed with 0 results, or browse loaded but empty (no history).
		hs.showEmptyState(listBox, query)
		hs.restoreScrollAndSelectionInListBox(listBox)
		return
	}

	for _, row := range rows {
		switch row.Kind {
		case historyDisplayRowHeader:
			hs.appendGroupHeader(listBox, row.Label)
		case historyDisplayRowEntry:
			hs.appendEntryRow(listBox, row.Entry)
		}
	}

	listBox.Show()

	// Restore previous scroll position and selection
	hs.restoreScrollAndSelectionInListBox(listBox)

	// If no selection was restored and this is the first load, select first entry
	hs.ensureAtLeastOneSelectionInListBox(listBox)
}

func (hs *HistorySidebar) showLoadingOrEmpty(listBox *gtk.ListBox, query string, searchDone, isLoading bool) {
	label := gtk.NewLabel(nil)
	if label == nil {
		return
	}
	label.AddCssClass("history-sidebar-loading")

	switch {
	case isLoading && query == "":
		label.SetText("Loading history...")
	case query != "" && !searchDone:
		label.SetText("Searching...")
	case query != "":
		label.SetText(noResultsText(query))
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
	listBox.Append(&row.Widget)
}

func noResultsText(query string) string {
	return "No results for \"" + query + "\""
}

func (hs *HistorySidebar) showEmptyState(listBox *gtk.ListBox, query string) {
	label := gtk.NewLabel(nil)
	if label == nil {
		return
	}
	label.AddCssClass("history-sidebar-empty")

	if query != "" {
		label.SetText(noResultsText(query))
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
	listBox.Append(&row.Widget)
}

// appendGroupHeader adds a non-selectable group header label to the list.
func (hs *HistorySidebar) appendGroupHeader(listBox *gtk.ListBox, labelText string) {
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
	listBox.Append(&row.Widget)
}

// appendEntryRow adds a selectable two-line entry row to the list.
func (hs *HistorySidebar) appendEntryRow(listBox *gtk.ListBox, entry *entity.HistoryEntry) {
	if entry == nil {
		return
	}
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
	timeLabel.SetText(relativeTimeAt(entry.LastVisited, hs.currentTime()))
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

	listBox.Append(&row.Widget)
	hs.bindRelativeTimeLabel(timeLabel, entry.LastVisited)
}

// ensureAtLeastOneSelection selects the first selectable row if nothing is selected.
func (hs *HistorySidebar) ensureAtLeastOneSelectionInListBox(listBox *gtk.ListBox) {
	if listBox == nil || listBox.GetSelectedRow() != nil {
		return
	}
	for i := 0; ; i++ {
		row := listBox.GetRowAtIndex(i)
		if row == nil {
			break
		}
		if row.GetSelectable() {
			listBox.SelectRow(row)
			return
		}
	}
}
