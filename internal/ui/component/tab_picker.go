package component

import (
	"context"
	"sync"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

type TabPickerItem struct {
	TabID entity.TabID
	Title string
	IsNew bool
	Index int // 0-based index in tab list, -1 for + New
}

// TabPicker is a modal overlay for selecting a tab destination.
//
// It is intentionally similar to SessionManager/Omnibox patterns.
type TabPicker struct {
	// GTK widgets
	outerBox       *gtk.Box
	mainBox        *gtk.Box
	headerBox      *gtk.Box
	titleLabel     *gtk.Label
	scrolledWindow *gtk.ScrolledWindow
	listBox        *gtk.ListBox
	footerLabel    *gtk.Label

	parentOverlay layout.OverlayWidget

	mu            sync.RWMutex
	visible       bool
	items         []TabPickerItem
	selectedIndex int
	uiScale       float64

	onClose  func()
	onSelect func(item TabPickerItem)

	retainedCallbacks []interface{}
	ctx               context.Context
}

type TabPickerConfig struct {
	UIScale  float64
	OnClose  func()
	OnSelect func(item TabPickerItem)
}

func NewTabPicker(ctx context.Context, cfg TabPickerConfig) *TabPicker {
	uiScale := cfg.UIScale
	if uiScale <= 0 {
		uiScale = 1.0
	}

	tp := &TabPicker{
		ctx:           ctx,
		onClose:       cfg.OnClose,
		onSelect:      cfg.OnSelect,
		selectedIndex: 0,
		uiScale:       uiScale,
	}

	if err := tp.createWidgets(); err != nil {
		return nil
	}
	tp.attachKeyController()
	return tp
}

func (tp *TabPicker) SetParentOverlay(overlay layout.OverlayWidget) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	tp.parentOverlay = overlay
}

func (tp *TabPicker) Widget() *gtk.Widget {
	if tp.outerBox == nil {
		return nil
	}
	return &tp.outerBox.Widget
}

func (tp *TabPicker) WidgetAsLayout(factory layout.WidgetFactory) layout.Widget {
	if tp.outerBox == nil {
		return nil
	}
	return factory.WrapWidget(&tp.outerBox.Widget)
}

func (tp *TabPicker) IsVisible() bool {
	tp.mu.RLock()
	defer tp.mu.RUnlock()
	return tp.visible
}

func (tp *TabPicker) Show(ctx context.Context, items []TabPickerItem) {
	tp.mu.Lock()
	if tp.visible {
		tp.mu.Unlock()
		return
	}
	tp.visible = true
	tp.items = items
	tp.selectedIndex = 0
	tp.mu.Unlock()

	tp.populateList()
	tp.resizeAndCenter()
	if tp.outerBox != nil {
		tp.outerBox.SetVisible(true)
	}
	if tp.listBox != nil {
		tp.listBox.GrabFocus()
	}
}

func (tp *TabPicker) Hide(ctx context.Context) {
	tp.mu.Lock()
	if !tp.visible {
		tp.mu.Unlock()
		return
	}
	tp.visible = false
	tp.mu.Unlock()

	if tp.outerBox != nil {
		tp.outerBox.SetVisible(false)
	}
	if tp.listBox != nil {
		tp.listBox.RemoveAll()
	}

	if tp.onClose != nil {
		tp.onClose()
	}
}

func (tp *TabPicker) Toggle(ctx context.Context, items []TabPickerItem) {
	if tp.IsVisible() {
		tp.Hide(ctx)
	} else {
		tp.Show(ctx, items)
	}
}

func (tp *TabPicker) createWidgets() error {
	if err := tp.createOuter(); err != nil {
		return err
	}
	if err := tp.createMain(); err != nil {
		return err
	}
	if err := tp.createHeader(); err != nil {
		return err
	}
	if err := tp.createList(); err != nil {
		return err
	}
	if err := tp.createFooter(); err != nil {
		return err
	}
	tp.assemble()
	return nil
}

func (tp *TabPicker) createOuter() error {
	tp.outerBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if tp.outerBox == nil {
		return errNilWidget("tabPickerOuterBox")
	}
	tp.outerBox.AddCssClass("tab-picker-outer")
	tp.outerBox.SetHalign(gtk.AlignCenterValue)
	tp.outerBox.SetValign(gtk.AlignStartValue)
	tp.outerBox.SetVisible(false)
	return nil
}

func (tp *TabPicker) createMain() error {
	tp.mainBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if tp.mainBox == nil {
		return errNilWidget("tabPickerMainBox")
	}
	tp.mainBox.AddCssClass("tab-picker-container")
	return nil
}

func (tp *TabPicker) createHeader() error {
	tp.headerBox = gtk.NewBox(gtk.OrientationHorizontalValue, 0)
	if tp.headerBox == nil {
		return errNilWidget("tabPickerHeaderBox")
	}
	tp.headerBox.AddCssClass("tab-picker-header")

	title := "Move Pane To Tab"
	tp.titleLabel = gtk.NewLabel(&title)
	if tp.titleLabel == nil {
		return errNilWidget("tabPickerTitleLabel")
	}
	tp.titleLabel.AddCssClass("tab-picker-title")
	tp.titleLabel.SetHalign(gtk.AlignStartValue)
	tp.titleLabel.SetHexpand(true)
	tp.headerBox.Append(&tp.titleLabel.Widget)

	shortcutText := "Ctrl+P m"
	shortcutLabel := gtk.NewLabel(&shortcutText)
	if shortcutLabel != nil {
		shortcutLabel.AddCssClass("omnibox-shortcut-badge")
		tp.headerBox.Append(&shortcutLabel.Widget)
	}
	return nil
}

func (tp *TabPicker) createList() error {
	tp.scrolledWindow = gtk.NewScrolledWindow()
	if tp.scrolledWindow == nil {
		return errNilWidget("tabPickerScrolledWindow")
	}
	tp.scrolledWindow.AddCssClass("tab-picker-scrolled")
	tp.scrolledWindow.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	tp.scrolledWindow.SetPropagateNaturalHeight(false)

	tp.listBox = gtk.NewListBox()
	if tp.listBox == nil {
		return errNilWidget("tabPickerListBox")
	}
	tp.listBox.AddCssClass("tab-picker-list")
	tp.listBox.SetSelectionMode(gtk.SelectionSingleValue)

	rowSelectedCb := func(_ gtk.ListBox, rowPtr uintptr) {
		if rowPtr == 0 {
			return
		}
		row := gtk.ListBoxRowNewFromInternalPtr(rowPtr)
		if row == nil {
			return
		}
		idx := row.GetIndex()
		tp.mu.Lock()
		tp.selectedIndex = idx
		tp.mu.Unlock()
	}
	tp.retainedCallbacks = append(tp.retainedCallbacks, rowSelectedCb)
	tp.listBox.ConnectRowSelected(&rowSelectedCb)

	tp.scrolledWindow.SetChild(&tp.listBox.Widget)
	return nil
}

func (tp *TabPicker) createFooter() error {
	footerText := "↑↓/jk navigate  Enter confirm  1-9 pick tab  n new tab  Esc close"
	tp.footerLabel = gtk.NewLabel(&footerText)
	if tp.footerLabel == nil {
		return errNilWidget("tabPickerFooterLabel")
	}
	tp.footerLabel.AddCssClass("tab-picker-footer")
	tp.footerLabel.SetHalign(gtk.AlignCenterValue)
	return nil
}

func (tp *TabPicker) assemble() {
	if tp.outerBox == nil || tp.mainBox == nil {
		return
	}
	if tp.headerBox != nil {
		tp.mainBox.Append(&tp.headerBox.Widget)
	}
	if tp.scrolledWindow != nil {
		tp.mainBox.Append(&tp.scrolledWindow.Widget)
	}
	if tp.footerLabel != nil {
		tp.mainBox.Append(&tp.footerLabel.Widget)
	}
	tp.outerBox.Append(&tp.mainBox.Widget)
}

func (tp *TabPicker) resizeAndCenter() {
	if tp.outerBox == nil || tp.mainBox == nil {
		return
	}

	width, marginTop := CalculateModalDimensions(tp.parentOverlay, TabPickerSizeDefaults)
	tp.mainBox.SetSizeRequest(width, -1)
	tp.outerBox.SetMarginTop(marginTop)

	// Height based on rows
	tp.mu.RLock()
	count := len(tp.items)
	tp.mu.RUnlock()

	if tp.scrolledWindow != nil {
		rowH := ScaleValue(DefaultRowHeights.Standard, tp.uiScale)
		maxH := TabPickerListDefaults.MaxVisibleRows * rowH
		h := count * rowH
		if h > maxH {
			h = maxH
		}
		if h < rowH {
			h = rowH
		}
		cb := glib.SourceFunc(func(_ uintptr) bool {
			SetScrolledWindowHeight(tp.scrolledWindow, h)
			tp.outerBox.QueueResize()
			return false
		})
		glib.IdleAdd(&cb, 0)
	}
}

func (tp *TabPicker) populateList() {
	if tp.listBox == nil {
		return
	}
	p := tp.listBox
	p.RemoveAll()

	tp.mu.RLock()
	items := append([]TabPickerItem(nil), tp.items...)
	tp.mu.RUnlock()

	for _, it := range items {
		row := gtk.NewListBoxRow()
		if row == nil {
			continue
		}
		row.AddCssClass("tab-picker-row")

		const rowSpacing = 8
		hbox := gtk.NewBox(gtk.OrientationHorizontalValue, rowSpacing)
		if hbox == nil {
			continue
		}
		hbox.SetHexpand(true)

		labelText := it.Title
		if it.IsNew {
			labelText = "+ New Tab"
		}

		label := gtk.NewLabel(&labelText)
		if label != nil {
			label.AddCssClass("tab-picker-row-title")
			label.SetHalign(gtk.AlignStartValue)
			label.SetHexpand(true)
			hbox.Append(&label.Widget)
		}

		row.SetChild(&hbox.Widget)
		p.Append(&row.Widget)
	}

	// select first row
	if row := p.GetRowAtIndex(0); row != nil {
		p.SelectRow(row)
	}
}

func (tp *TabPicker) attachKeyController() {
	controller := gtk.NewEventControllerKey()
	if controller == nil {
		return
	}
	controller.SetPropagationPhase(gtk.PhaseCaptureValue)

	keyPressedCb := func(_ gtk.EventControllerKey, keyval uint, _ uint, _ gdk.ModifierType) bool {
		return tp.handleKeyPress(keyval)
	}
	tp.retainedCallbacks = append(tp.retainedCallbacks, keyPressedCb)
	controller.ConnectKeyPressed(&keyPressedCb)
	tp.outerBox.AddController(&controller.EventController)
}

func (tp *TabPicker) handleKeyPress(keyval uint) bool {
	switch keyval {
	case uint(gdk.KEY_Escape):
		tp.Hide(tp.ctx)
		return true
	case uint(gdk.KEY_Up), uint(gdk.KEY_k):
		tp.selectRelative(-1)
		return true
	case uint(gdk.KEY_Down), uint(gdk.KEY_j):
		tp.selectRelative(1)
		return true
	case uint(gdk.KEY_n):
		tp.chooseNewTab()
		return true
	case uint(gdk.KEY_Return), uint(gdk.KEY_KP_Enter):
		tp.confirmSelection()
		return true
	case uint(gdk.KEY_1), uint(gdk.KEY_2), uint(gdk.KEY_3), uint(gdk.KEY_4), uint(gdk.KEY_5),
		uint(gdk.KEY_6), uint(gdk.KEY_7), uint(gdk.KEY_8), uint(gdk.KEY_9):
		idx := int(keyval - uint(gdk.KEY_1))
		tp.selectIndex(idx)
		tp.confirmSelection()
		return true
	default:
		return false
	}
}

func (tp *TabPicker) selectRelative(delta int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	count := len(tp.items)
	if count == 0 {
		return
	}

	i := tp.selectedIndex + delta
	if i < 0 {
		i = count - 1
	} else if i >= count {
		i = 0
	}
	tp.selectedIndex = i
	if tp.listBox != nil {
		if row := tp.listBox.GetRowAtIndex(i); row != nil {
			tp.listBox.SelectRow(row)
			row.GrabFocus()
		}
	}
}

func (tp *TabPicker) selectIndex(index int) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	count := len(tp.items)
	if count == 0 {
		return
	}
	if index < 0 {
		index = 0
	}
	if index >= count {
		index = count - 1
	}
	tp.selectedIndex = index
	if tp.listBox != nil {
		if row := tp.listBox.GetRowAtIndex(index); row != nil {
			tp.listBox.SelectRow(row)
			row.GrabFocus()
		}
	}
}

func (tp *TabPicker) chooseNewTab() {
	tp.mu.RLock()
	items := append([]TabPickerItem(nil), tp.items...)
	tp.mu.RUnlock()

	for i := range items {
		if items[i].IsNew {
			tp.selectIndex(i)
			tp.confirmSelection()
			return
		}
	}
}

func (tp *TabPicker) confirmSelection() {
	tp.mu.RLock()
	idx := tp.selectedIndex
	items := append([]TabPickerItem(nil), tp.items...)
	tp.mu.RUnlock()

	if idx < 0 || idx >= len(items) {
		return
	}
	item := items[idx]

	tp.Hide(tp.ctx)
	if tp.onSelect != nil {
		tp.onSelect(item)
	}
}
