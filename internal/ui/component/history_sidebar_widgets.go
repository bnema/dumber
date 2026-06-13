package component

import (
	"fmt"

	"github.com/bnema/puregotk/v4/gtk"
)

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
