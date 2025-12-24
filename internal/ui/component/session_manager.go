package component

import (
	"context"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

const (
	sessionManagerWidthPct  = 0.6 // 60% of parent window width
	sessionManagerMaxWidth  = 600 // Maximum width in pixels
	sessionManagerTopMargin = 0.15
	sessionListMaxHeight    = 400
	sessionDefaultListLimit = 50
	sessionIDMaxDisplay     = 20
	sessionRowBoxSpacing    = 8
)

// SessionManager is a modal overlay for managing sessions.
type SessionManager struct {
	// GTK widgets
	outerBox       *gtk.Box
	mainBox        *gtk.Box
	headerBox      *gtk.Box
	titleLabel     *gtk.Label
	scrolledWindow *gtk.ScrolledWindow
	listBox        *gtk.ListBox
	footerLabel    *gtk.Label

	// Parent overlay reference for sizing
	parentOverlay layout.OverlayWidget

	// State
	mu            sync.RWMutex
	visible       bool
	sessions      []entity.SessionInfo
	selectedIndex int

	// Dependencies
	listSessionsUC *usecase.ListSessionsUseCase
	spawner        port.SessionSpawner
	currentSession entity.SessionID

	// Callbacks
	onClose func()
	onOpen  func(sessionID entity.SessionID)

	// GTK callback retention
	retainedCallbacks []interface{}

	ctx context.Context
}

// SessionManagerConfig holds configuration for creating a SessionManager.
type SessionManagerConfig struct {
	ListSessionsUC *usecase.ListSessionsUseCase
	Spawner        port.SessionSpawner
	CurrentSession entity.SessionID
	OnClose        func()
	OnOpen         func(sessionID entity.SessionID)
}

// NewSessionManager creates a new SessionManager component.
func NewSessionManager(ctx context.Context, cfg SessionManagerConfig) *SessionManager {
	log := logging.FromContext(ctx)

	sm := &SessionManager{
		ctx:            ctx,
		listSessionsUC: cfg.ListSessionsUC,
		spawner:        cfg.Spawner,
		currentSession: cfg.CurrentSession,
		onClose:        cfg.OnClose,
		onOpen:         cfg.OnOpen,
		selectedIndex:  -1,
	}

	if err := sm.createWidgets(); err != nil {
		log.Error().Err(err).Msg("failed to create session manager widgets")
		return nil
	}

	sm.attachKeyController()

	log.Debug().Msg("session manager created")
	return sm
}

// SetParentOverlay sets the overlay widget used for sizing calculations.
// Must be called before Show().
func (sm *SessionManager) SetParentOverlay(overlay layout.OverlayWidget) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.parentOverlay = overlay
}

// WidgetAsLayout returns the session manager's outer widget as a layout.Widget.
func (sm *SessionManager) WidgetAsLayout(factory layout.WidgetFactory) layout.Widget {
	if sm.outerBox == nil {
		return nil
	}
	return factory.WrapWidget(&sm.outerBox.Widget)
}

// Widget returns the session manager widget for embedding in an overlay.
func (sm *SessionManager) Widget() *gtk.Widget {
	if sm.outerBox == nil {
		return nil
	}
	return &sm.outerBox.Widget
}

// Show displays the session manager and loads sessions.
func (sm *SessionManager) Show(ctx context.Context) {
	log := logging.FromContext(ctx)
	log.Debug().Msg("showing session manager")

	sm.mu.Lock()
	if sm.visible {
		sm.mu.Unlock()
		return
	}
	sm.visible = true
	sm.mu.Unlock()

	// Calculate size and position
	sm.resizeAndCenter()

	// Show the widget
	sm.outerBox.SetVisible(true)

	// Load sessions asynchronously
	go sm.loadSessions()

	// Focus the list
	if sm.listBox != nil {
		sm.listBox.GrabFocus()
	}
}

// Hide hides the session manager.
func (sm *SessionManager) Hide(ctx context.Context) {
	log := logging.FromContext(ctx)
	log.Debug().Msg("hiding session manager")

	sm.mu.Lock()
	if !sm.visible {
		sm.mu.Unlock()
		return
	}
	sm.visible = false
	sm.mu.Unlock()

	sm.outerBox.SetVisible(false)

	// Clear state
	sm.listBox.RemoveAll()

	if sm.onClose != nil {
		sm.onClose()
	}
}

// IsVisible returns whether the session manager is visible.
func (sm *SessionManager) IsVisible() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.visible
}

// Toggle shows if hidden, hides if visible.
func (sm *SessionManager) Toggle(ctx context.Context) {
	sm.mu.RLock()
	visible := sm.visible
	sm.mu.RUnlock()

	if visible {
		sm.Hide(ctx)
	} else {
		sm.Show(ctx)
	}
}

func (sm *SessionManager) resizeAndCenter() {
	if sm.outerBox == nil || sm.mainBox == nil {
		return
	}

	var parentWidth, parentHeight int

	if sm.parentOverlay != nil {
		parentWidth = sm.parentOverlay.GetAllocatedWidth()
		parentHeight = sm.parentOverlay.GetAllocatedHeight()
	}

	// Use defaults if parent overlay not set or not yet allocated
	if parentWidth <= 0 {
		parentWidth = sessionManagerMaxWidth
	}
	if parentHeight <= 0 {
		parentHeight = 600
	}

	width := int(float64(parentWidth) * sessionManagerWidthPct)
	if width > sessionManagerMaxWidth {
		width = sessionManagerMaxWidth
	}

	marginTop := int(float64(parentHeight) * sessionManagerTopMargin)

	sm.mainBox.SetSizeRequest(width, -1)
	sm.outerBox.SetMarginTop(marginTop)
}

func (sm *SessionManager) createWidgets() error {
	// Outer container for positioning
	sm.outerBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if sm.outerBox == nil {
		return errNilWidget("sessionOuterBox")
	}
	sm.outerBox.AddCssClass("session-manager-outer")
	sm.outerBox.SetHalign(gtk.AlignCenterValue)
	sm.outerBox.SetValign(gtk.AlignStartValue)
	sm.outerBox.SetVisible(false)

	// Main container with styling
	sm.mainBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if sm.mainBox == nil {
		return errNilWidget("sessionMainBox")
	}
	sm.mainBox.AddCssClass("session-manager-container")

	// Header
	if err := sm.initHeader(); err != nil {
		return err
	}

	// Scrolled window for list
	if err := sm.initList(); err != nil {
		return err
	}

	// Footer with shortcuts
	if err := sm.initFooter(); err != nil {
		return err
	}

	// Assemble
	sm.mainBox.Append(&sm.headerBox.Widget)
	sm.mainBox.Append(&sm.scrolledWindow.Widget)
	sm.mainBox.Append(&sm.footerLabel.Widget)
	sm.outerBox.Append(&sm.mainBox.Widget)

	return nil
}

func (sm *SessionManager) initHeader() error {
	sm.headerBox = gtk.NewBox(gtk.OrientationHorizontalValue, 0)
	if sm.headerBox == nil {
		return errNilWidget("sessionHeaderBox")
	}
	sm.headerBox.AddCssClass("session-manager-header")

	title := "Sessions"
	sm.titleLabel = gtk.NewLabel(&title)
	if sm.titleLabel == nil {
		return errNilWidget("sessionTitleLabel")
	}
	sm.titleLabel.AddCssClass("session-manager-title")
	sm.titleLabel.SetHalign(gtk.AlignStartValue)
	sm.titleLabel.SetHexpand(true)
	sm.headerBox.Append(&sm.titleLabel.Widget)

	// Shortcut badge
	shortcutText := "Ctrl+O"
	shortcutLabel := gtk.NewLabel(&shortcutText)
	if shortcutLabel != nil {
		shortcutLabel.AddCssClass("omnibox-shortcut-badge")
		sm.headerBox.Append(&shortcutLabel.Widget)
	}

	return nil
}

func (sm *SessionManager) initList() error {
	sm.scrolledWindow = gtk.NewScrolledWindow()
	if sm.scrolledWindow == nil {
		return errNilWidget("sessionScrolledWindow")
	}
	sm.scrolledWindow.AddCssClass("session-manager-scrolled")
	sm.scrolledWindow.SetMaxContentHeight(sessionListMaxHeight)
	sm.scrolledWindow.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	sm.scrolledWindow.SetPropagateNaturalHeight(true)

	sm.listBox = gtk.NewListBox()
	if sm.listBox == nil {
		return errNilWidget("sessionListBox")
	}
	sm.listBox.AddCssClass("session-manager-list")
	sm.listBox.SetSelectionMode(gtk.SelectionSingleValue)

	// Connect row selection
	rowSelectedCb := func(_ gtk.ListBox, rowPtr uintptr) {
		if rowPtr == 0 {
			sm.mu.Lock()
			sm.selectedIndex = -1
			sm.mu.Unlock()
			return
		}
		row := gtk.ListBoxRowNewFromInternalPtr(rowPtr)
		if row != nil {
			sm.mu.Lock()
			sm.selectedIndex = row.GetIndex()
			sm.mu.Unlock()
		}
	}
	sm.retainedCallbacks = append(sm.retainedCallbacks, rowSelectedCb)
	sm.listBox.ConnectRowSelected(&rowSelectedCb)

	sm.scrolledWindow.SetChild(&sm.listBox.Widget)
	return nil
}

func (sm *SessionManager) initFooter() error {
	footerText := "↑↓/jk navigate  Enter open  x delete  Esc close"
	sm.footerLabel = gtk.NewLabel(&footerText)
	if sm.footerLabel == nil {
		return errNilWidget("sessionFooterLabel")
	}
	sm.footerLabel.AddCssClass("session-manager-footer")
	sm.footerLabel.SetHalign(gtk.AlignCenterValue)
	return nil
}

func (sm *SessionManager) attachKeyController() {
	controller := gtk.NewEventControllerKey()
	if controller == nil {
		return
	}
	controller.SetPropagationPhase(gtk.PhaseCaptureValue)

	keyPressedCb := func(_ gtk.EventControllerKey, keyval uint, _ uint, state gdk.ModifierType) bool {
		return sm.handleKeyPress(keyval, state)
	}
	sm.retainedCallbacks = append(sm.retainedCallbacks, keyPressedCb)
	controller.ConnectKeyPressed(&keyPressedCb)
	sm.outerBox.AddController(&controller.EventController)
}

func (sm *SessionManager) handleKeyPress(keyval uint, _ gdk.ModifierType) bool {
	switch keyval {
	case uint(gdk.KEY_Escape):
		sm.Hide(sm.ctx)
		return true

	case uint(gdk.KEY_Up), uint(gdk.KEY_k):
		sm.selectPrevious()
		return true

	case uint(gdk.KEY_Down), uint(gdk.KEY_j):
		sm.selectNext()
		return true

	case uint(gdk.KEY_Return), uint(gdk.KEY_KP_Enter):
		sm.openSelected()
		return true

	case uint(gdk.KEY_x), uint(gdk.KEY_Delete):
		sm.deleteSelected()
		return true
	}
	return false
}

func (sm *SessionManager) loadSessions() {
	log := logging.FromContext(sm.ctx)

	if sm.listSessionsUC == nil {
		log.Warn().Msg("listSessionsUC is nil")
		return
	}

	output, err := sm.listSessionsUC.Execute(sm.ctx, sm.currentSession, sessionDefaultListLimit)
	if err != nil {
		log.Error().Err(err).Msg("failed to list sessions")
		return
	}

	sm.mu.Lock()
	sm.sessions = output.Sessions
	sm.selectedIndex = 0
	sm.mu.Unlock()

	// Update UI on main thread
	cb := glib.SourceFunc(func(_ uintptr) bool {
		sm.populateList()
		return false
	})
	glib.IdleAdd(&cb, 0)
}

func (sm *SessionManager) populateList() {
	sm.mu.RLock()
	sessions := sm.sessions
	sm.mu.RUnlock()

	if sm.listBox == nil {
		return
	}

	// Clear existing rows
	sm.listBox.RemoveAll()

	// Separate active and exited sessions
	var activeSessions, exitedSessions []entity.SessionInfo
	for _, info := range sessions {
		if info.IsCurrent || info.IsActive {
			activeSessions = append(activeSessions, info)
		} else {
			exitedSessions = append(exitedSessions, info)
		}
	}

	// Add active session rows
	for _, info := range activeSessions {
		row := sm.createSessionRow(info)
		if row != nil {
			sm.listBox.Append(&row.Widget)
		}
	}

	// Add divider if there are exited sessions
	if len(exitedSessions) > 0 && len(activeSessions) > 0 {
		divider := sm.createDividerRow("EXITED")
		if divider != nil {
			sm.listBox.Append(&divider.Widget)
		}
	}

	// Add exited session rows
	for _, info := range exitedSessions {
		row := sm.createSessionRow(info)
		if row != nil {
			sm.listBox.Append(&row.Widget)
		}
	}

	// Select first row
	if len(sessions) > 0 {
		firstRow := sm.listBox.GetRowAtIndex(0)
		if firstRow != nil {
			sm.listBox.SelectRow(firstRow)
		}
	}

	// Resize based on content
	rowCount := len(activeSessions) + len(exitedSessions)
	if len(exitedSessions) > 0 && len(activeSessions) > 0 {
		rowCount++ // divider row
	}
	sm.resizeForContent(rowCount)
}

func (sm *SessionManager) resizeForContent(rowCount int) {
	if sm.scrolledWindow == nil {
		return
	}

	const rowHeight = 50
	contentHeight := rowCount * rowHeight

	if contentHeight > sessionListMaxHeight {
		contentHeight = sessionListMaxHeight
	}

	// Schedule resize
	cb := glib.SourceFunc(func(_ uintptr) bool {
		sm.scrolledWindow.SetMinContentHeight(-1)
		sm.scrolledWindow.SetMaxContentHeight(contentHeight)
		sm.scrolledWindow.SetMinContentHeight(contentHeight)
		sm.outerBox.QueueResize()
		return false
	})
	glib.IdleAdd(&cb, 0)
}

func (sm *SessionManager) createDividerRow(text string) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()
	if row == nil {
		return nil
	}
	row.SetSelectable(false)
	row.SetActivatable(false)

	label := gtk.NewLabel(&text)
	if label == nil {
		return nil
	}
	label.AddCssClass("session-divider")
	label.SetHalign(gtk.AlignStartValue)
	row.SetChild(&label.Widget)

	return row
}

func (sm *SessionManager) createSessionRow(info entity.SessionInfo) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()
	if row == nil {
		return nil
	}
	row.AddCssClass("session-manager-row-wrapper")

	hbox := gtk.NewBox(gtk.OrientationHorizontalValue, sessionRowBoxSpacing)
	if hbox == nil {
		return nil
	}
	hbox.AddCssClass("session-manager-row")

	sm.addStatusIndicator(hbox, info)
	sm.addSessionInfo(hbox, info)
	sm.addRelativeTime(hbox, info)

	row.SetChild(&hbox.Widget)
	return row
}

func (sm *SessionManager) addStatusIndicator(hbox *gtk.Box, info entity.SessionInfo) {
	var statusText string
	switch {
	case info.IsCurrent:
		statusText = "●"
		hbox.AddCssClass("session-current")
	case info.IsActive:
		statusText = "○"
		hbox.AddCssClass("session-active")
	default:
		statusText = " "
		hbox.AddCssClass("session-exited")
	}
	statusLabel := gtk.NewLabel(&statusText)
	if statusLabel != nil {
		statusLabel.AddCssClass("session-status")
		hbox.Append(&statusLabel.Widget)
	}
}

func (sm *SessionManager) addSessionInfo(hbox *gtk.Box, info entity.SessionInfo) {
	const infoBoxSpacing = 2
	infoBox := gtk.NewBox(gtk.OrientationVerticalValue, infoBoxSpacing)
	if infoBox == nil {
		return
	}
	infoBox.SetHexpand(true)

	// Session ID (short)
	idText := string(info.Session.ID)
	if len(idText) > sessionIDMaxDisplay {
		idText = idText[:sessionIDMaxDisplay]
	}
	if info.IsCurrent {
		idText += " (current)"
	}
	idLabel := gtk.NewLabel(&idText)
	if idLabel != nil {
		idLabel.AddCssClass("session-id")
		idLabel.SetHalign(gtk.AlignStartValue)
		idLabel.SetEllipsize(2) // PANGO_ELLIPSIZE_END
		infoBox.Append(&idLabel.Widget)
	}

	// Tab/pane count
	countText := fmt.Sprintf("%d tabs, %d panes", info.TabCount, info.PaneCount)
	countLabel := gtk.NewLabel(&countText)
	if countLabel != nil {
		countLabel.AddCssClass("session-count")
		countLabel.SetHalign(gtk.AlignStartValue)
		infoBox.Append(&countLabel.Widget)
	}

	hbox.Append(&infoBox.Widget)
}

func (sm *SessionManager) addRelativeTime(hbox *gtk.Box, info entity.SessionInfo) {
	relTime := usecase.GetRelativeTime(info.UpdatedAt)
	timeLabel := gtk.NewLabel(&relTime)
	if timeLabel != nil {
		timeLabel.AddCssClass("session-time")
		timeLabel.SetValign(gtk.AlignCenterValue)
		hbox.Append(&timeLabel.Widget)
	}
}

func (sm *SessionManager) selectIndex(index int) {
	row := sm.listBox.GetRowAtIndex(index)
	if row != nil {
		sm.listBox.SelectRow(row)
	}
}

func (sm *SessionManager) selectPrevious() {
	sm.mu.Lock()
	if sm.selectedIndex > 0 {
		sm.selectedIndex--
	}
	idx := sm.selectedIndex
	sm.mu.Unlock()

	sm.selectIndex(idx)
}

func (sm *SessionManager) selectNext() {
	sm.mu.Lock()
	maxIndex := len(sm.sessions) - 1
	// Account for divider row
	if sm.selectedIndex < maxIndex {
		sm.selectedIndex++
	}
	idx := sm.selectedIndex
	sm.mu.Unlock()

	sm.selectIndex(idx)
}

func (sm *SessionManager) openSelected() {
	sm.mu.RLock()
	if sm.selectedIndex < 0 || sm.selectedIndex >= len(sm.sessions) {
		sm.mu.RUnlock()
		return
	}
	info := sm.sessions[sm.selectedIndex]
	sm.mu.RUnlock()

	log := logging.FromContext(sm.ctx)

	// Don't open current session
	if info.IsCurrent {
		log.Debug().Msg("cannot open current session")
		return
	}

	log.Info().Str("session_id", string(info.Session.ID)).Msg("opening session")

	sm.Hide(sm.ctx)

	if sm.onOpen != nil {
		sm.onOpen(info.Session.ID)
	}
}

func (sm *SessionManager) deleteSelected() {
	sm.mu.RLock()
	if sm.selectedIndex < 0 || sm.selectedIndex >= len(sm.sessions) {
		sm.mu.RUnlock()
		return
	}
	info := sm.sessions[sm.selectedIndex]
	sm.mu.RUnlock()

	log := logging.FromContext(sm.ctx)

	// Don't delete current or active sessions
	if info.IsCurrent || info.IsActive {
		log.Debug().Msg("cannot delete current or active session")
		return
	}

	log.Info().Str("session_id", string(info.Session.ID)).Msg("deleting session")

	// TODO: Implement session deletion via use case
	// For now, just reload the list
	go sm.loadSessions()
}

// Destroy cleans up session manager resources.
func (sm *SessionManager) Destroy() {
	sm.mu.Lock()
	sm.visible = false
	sm.mu.Unlock()

	sm.parentOverlay = nil
	sm.retainedCallbacks = nil

	if sm.outerBox != nil {
		sm.outerBox.Unparent()
		sm.outerBox = nil
	}
}
