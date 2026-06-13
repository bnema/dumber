package component

import (
	"fmt"

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
	hs.mu.RLock()
	if hs.destroyed || hs.listBox == nil {
		hs.mu.RUnlock()
		return
	}
	rows := hs.displayRows
	query := hs.currentQuery
	hasSearchResults := hs.searchResults != nil
	searchDone := hs.searchDone
	totalLoaded := hs.totalLoaded
	hs.mu.RUnlock()

	// Remove all rows
	hs.listBox.RemoveAll()

	if len(rows) == 0 {
		if !hasSearchResults && totalLoaded == 0 {
			// Browse has not loaded yet AND no search has completed.
			hs.showLoadingOrEmpty(query, searchDone)
			return
		}
		// Search completed with 0 results, or browse loaded but empty (no history).
		hs.showEmptyState(query)
		hs.restoreScrollAndSelection()
		return
	}

	for _, row := range rows {
		switch row.Kind {
		case historyDisplayRowHeader:
			hs.appendGroupHeader(row.Label)
		case historyDisplayRowEntry:
			hs.appendEntryRow(row.Entry)
		}
	}

	hs.listBox.Show()

	// Restore previous scroll position and selection
	hs.restoreScrollAndSelection()

	// If no selection was restored and this is the first load, select first entry
	hs.ensureAtLeastOneSelection()
}

func (hs *HistorySidebar) showLoadingOrEmpty(query string, searchDone bool) {
	label := gtk.NewLabel(nil)
	if label == nil {
		return
	}
	label.AddCssClass("history-sidebar-loading")

	hs.mu.RLock()
	isLoading := hs.isLoading
	hs.mu.RUnlock()

	switch {
	case isLoading && query == "":
		label.SetText("Loading history...")
	case query != "" && !searchDone:
		label.SetText("Searching...")
	case query != "":
		label.SetText(fmt.Sprintf("No results for %q", query))
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
		label.SetText(fmt.Sprintf("No results for %q", query))
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
