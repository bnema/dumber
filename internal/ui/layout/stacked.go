package layout

import (
	"context"
	"errors"
	"sync"

	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/gobject"
)

// ErrStackEmpty is returned when operating on an empty stack.
var ErrStackEmpty = errors.New("stack is empty")

// ErrIndexOutOfBounds is returned when an index is out of range.
var ErrIndexOutOfBounds = errors.New("index out of bounds")

// ErrCannotRemoveLastPane is returned when trying to remove the last pane from a stack.
var ErrCannotRemoveLastPane = errors.New("cannot remove last pane from stack")

const (
	stackedTitleMaxWidthChars = 30
	stackedPaneCloseIcon      = "window-close-symbolic"
)

// stackedPane represents a single pane within a stacked container.
type stackedPane struct {
	paneID    string    // unique identifier for this pane
	titleBar  BoxWidget // horizontal box with favicon + title label
	container Widget    // the pane view widget
	title     string
	favicon   ImageWidget
	label     LabelWidget
	isActive  bool

	// Signal handler IDs for cleanup on removal
	titleClickSignalID uint32
	closeClickSignalID uint32
	titleButton        ButtonWidget // stored for signal disconnection
	closeButton        ButtonWidget // stored for signal disconnection
}

// StackedView manages a stack of panes where only one is visible at a time.
// Inactive panes show title bars that can be clicked to activate them.
type StackedView struct {
	factory     WidgetFactory
	box         BoxWidget // vertical container for all content
	panes       []*stackedPane
	activeIndex int

	onActivate  func(index int)     // called when a pane is activated via title bar click
	onClosePane func(paneID string) // called when a pane's close button is clicked

	mu sync.RWMutex
}

// NewStackedView creates a new stacked pane container.
func NewStackedView(factory WidgetFactory) *StackedView {
	box := factory.NewBox(OrientationVertical, 0)
	box.SetHexpand(true)
	box.SetVexpand(true)

	return &StackedView{
		factory:     factory,
		box:         box,
		panes:       make([]*stackedPane, 0),
		activeIndex: -1,
	}
}

// titleBarComponents holds the widgets created for a pane's title bar.
type titleBarComponents struct {
	titleBar    BoxWidget
	titleButton ButtonWidget
	closeBtn    ButtonWidget
	favicon     ImageWidget
	label       LabelWidget
}

// createTitleBar creates the title bar widgets for a pane.
func (sv *StackedView) createTitleBar(title, faviconIconName string) titleBarComponents {
	// Create title bar container - must not expand vertically
	titleBar := sv.factory.NewBox(OrientationHorizontal, 4)
	titleBar.AddCssClass("stacked-pane-titlebar")
	titleBar.SetVexpand(false)

	// Create favicon image
	favicon := sv.factory.NewImage()
	if faviconIconName != "" {
		favicon.SetFromIconName(faviconIconName)
	} else {
		favicon.SetFromIconName("web-browser-symbolic")
	}
	favicon.SetPixelSize(16)
	titleBar.Append(favicon)

	// Create title label
	label := sv.factory.NewLabel(title)
	label.SetEllipsize(EllipsizeEnd)
	label.SetMaxWidthChars(stackedTitleMaxWidthChars)
	label.SetHexpand(true)
	label.SetXalign(0.0)
	titleBar.Append(label)

	// Create close button using GTK's native icon button support
	closeBtn := sv.factory.NewButton()
	closeBtn.SetIconName(stackedPaneCloseIcon)
	closeBtn.AddCssClass("stacked-pane-close-button")
	closeBtn.SetFocusOnClick(false)
	closeBtn.SetVexpand(false)
	closeBtn.SetHexpand(false)
	titleBar.Append(closeBtn)

	// Make title bar clickable - ensure it doesn't expand vertically
	titleButton := sv.factory.NewButton()
	titleButton.SetChild(titleBar)
	titleButton.AddCssClass("stacked-pane-title-button")
	titleButton.SetFocusOnClick(false)
	titleButton.SetVexpand(false) // Critical: don't let title bar expand vertically
	titleButton.SetHexpand(true)  // But fill horizontal space

	return titleBarComponents{
		titleBar:    titleBar,
		titleButton: titleButton,
		closeBtn:    closeBtn,
		favicon:     favicon,
		label:       label,
	}
}

// AddPane adds a new pane to the stack.
// The new pane becomes active (visible).
func (sv *StackedView) AddPane(ctx context.Context, paneID, title, faviconIconName string, container Widget) int {
	log := logging.FromContext(ctx)
	sv.mu.Lock()
	defer sv.mu.Unlock()

	log.Debug().
		Str("title", title).
		Bool("container_nil", container == nil).
		Int("current_count", len(sv.panes)).
		Msg("StackedView.AddPane called")

	// Create title bar components
	tb := sv.createTitleBar(title, faviconIconName)

	// Connect click handlers using paneID (not index, to handle removals)
	titleSignalID, closeSignalID := sv.connectTitleBarHandlers(tb, paneID)

	pane := &stackedPane{
		paneID:             paneID,
		titleBar:           tb.titleBar,
		container:          container,
		title:              title,
		favicon:            tb.favicon,
		label:              tb.label,
		isActive:           false,
		titleClickSignalID: titleSignalID,
		closeClickSignalID: closeSignalID,
		titleButton:        tb.titleButton,
		closeButton:        tb.closeBtn,
	}

	index := len(sv.panes)
	sv.panes = append(sv.panes, pane)

	// Add to container
	log.Debug().
		Int("index", index).
		Msg("StackedView: appending titleButton and container to box")
	sv.box.Append(tb.titleButton)
	if container != nil {
		sv.box.Append(container)
	}

	// Set this pane as active
	sv.setActiveInternal(ctx, index)

	log.Debug().
		Int("new_index", index).
		Int("new_count", len(sv.panes)).
		Msg("StackedView.AddPane completed")

	return index
}

// connectTitleBarHandlers connects the click handlers for a title bar.
// Uses paneID lookup instead of captured index to handle pane removals correctly.
// Returns the signal IDs for later disconnection.
func (sv *StackedView) connectTitleBarHandlers(tb titleBarComponents, paneID string) (titleSignalID, closeSignalID uint32) {
	// Connect title bar click handler for activation
	titleSignalID = tb.titleButton.ConnectClicked(func() {
		sv.mu.RLock()
		callback := sv.onActivate
		currentIndex := sv.findPaneIndexInternal(paneID)
		sv.mu.RUnlock()

		if callback != nil && currentIndex >= 0 {
			callback(currentIndex)
		}
	})

	// Connect close button click handler
	closeSignalID = tb.closeBtn.ConnectClicked(func() {
		sv.mu.RLock()
		onClose := sv.onClosePane
		sv.mu.RUnlock()

		if onClose != nil {
			onClose(paneID)
		}
	})

	return titleSignalID, closeSignalID
}

// disconnectPaneSignals disconnects signal handlers from a pane's buttons.
// This prevents memory leaks when panes are removed from the stack.
func (sv *StackedView) disconnectPaneSignals(pane *stackedPane) {
	if pane == nil {
		return
	}

	// Disconnect title button click signal
	if pane.titleButton != nil && pane.titleClickSignalID != 0 {
		gtkWidget := pane.titleButton.GtkWidget()
		if gtkWidget != nil {
			obj := gobject.ObjectNewFromInternalPtr(gtkWidget.GoPointer())
			gobject.SignalHandlerDisconnect(obj, pane.titleClickSignalID)
		}
	}

	// Disconnect close button click signal
	if pane.closeButton != nil && pane.closeClickSignalID != 0 {
		gtkWidget := pane.closeButton.GtkWidget()
		if gtkWidget != nil {
			obj := gobject.ObjectNewFromInternalPtr(gtkWidget.GoPointer())
			gobject.SignalHandlerDisconnect(obj, pane.closeClickSignalID)
		}
	}
}

// InsertPaneAfter inserts a new pane after the specified index position.
// Use afterIndex=-1 to insert at the beginning.
// The new pane becomes active (visible).
// Returns the index where the pane was inserted.
func (sv *StackedView) InsertPaneAfter(
	ctx context.Context, afterIndex int, paneID, title, faviconIconName string, container Widget,
) int {
	log := logging.FromContext(ctx)
	sv.mu.Lock()
	defer sv.mu.Unlock()

	// Validate afterIndex - clamp to valid range
	if afterIndex < -1 {
		afterIndex = -1
	}
	if afterIndex >= len(sv.panes) {
		afterIndex = len(sv.panes) - 1
	}
	insertIndex := afterIndex + 1

	log.Debug().
		Str("title", title).
		Int("after_index", afterIndex).
		Int("insert_index", insertIndex).
		Int("current_count", len(sv.panes)).
		Msg("StackedView.InsertPaneAfter called")

	// Create title bar components
	tb := sv.createTitleBar(title, faviconIconName)

	// Connect click handlers using paneID (not index, to handle removals)
	titleSignalID, closeSignalID := sv.connectTitleBarHandlers(tb, paneID)

	pane := &stackedPane{
		paneID:             paneID,
		titleBar:           tb.titleBar,
		container:          container,
		title:              title,
		favicon:            tb.favicon,
		label:              tb.label,
		isActive:           false,
		titleClickSignalID: titleSignalID,
		closeClickSignalID: closeSignalID,
		titleButton:        tb.titleButton,
		closeButton:        tb.closeBtn,
	}

	// Insert into slice at correct position
	sv.panes = append(sv.panes, nil)
	copy(sv.panes[insertIndex+1:], sv.panes[insertIndex:])
	sv.panes[insertIndex] = pane

	// Insert widgets at correct position in GTK box
	sv.insertTitleBarWidgets(tb.titleButton, container, insertIndex)

	// Set this pane as active
	sv.setActiveInternal(ctx, insertIndex)

	log.Debug().
		Int("insert_index", insertIndex).
		Int("new_count", len(sv.panes)).
		Msg("StackedView.InsertPaneAfter completed")

	return insertIndex
}

// insertTitleBarWidgets inserts the title bar button and container at the correct position.
func (sv *StackedView) insertTitleBarWidgets(titleButton ButtonWidget, container Widget, insertIndex int) {
	if insertIndex > 0 && sv.panes[insertIndex-1] != nil {
		// Insert after the previous pane's container
		prevPane := sv.panes[insertIndex-1]
		if prevPane.container != nil {
			sv.box.InsertChildAfter(titleButton, prevPane.container)
			if container != nil {
				sv.box.InsertChildAfter(container, titleButton)
			}
		} else {
			// No container, insert after title bar's parent (the button)
			prevTitleParent := prevPane.titleBar.GetParent()
			if prevTitleParent != nil {
				sv.box.InsertChildAfter(titleButton, prevTitleParent)
				if container != nil {
					sv.box.InsertChildAfter(container, titleButton)
				}
			} else {
				// Fallback to append
				sv.box.Append(titleButton)
				if container != nil {
					sv.box.Append(container)
				}
			}
		}
	} else {
		// Insert at beginning
		if container != nil {
			sv.box.Prepend(container)
		}
		sv.box.Prepend(titleButton)
	}
}

// RemovePane removes a pane from the stack by index.
// Returns an error if trying to remove the last pane.
func (sv *StackedView) RemovePane(ctx context.Context, index int) error {
	log := logging.FromContext(ctx)
	sv.mu.Lock()
	defer sv.mu.Unlock()

	log.Debug().
		Int("index", index).
		Int("pane_count", len(sv.panes)).
		Msg("StackedView.RemovePane called")

	if len(sv.panes) == 0 {
		return ErrStackEmpty
	}
	if index < 0 || index >= len(sv.panes) {
		return ErrIndexOutOfBounds
	}
	if len(sv.panes) == 1 {
		return ErrCannotRemoveLastPane
	}

	pane := sv.panes[index]

	// Disconnect signal handlers before removing widgets to prevent memory leaks
	sv.disconnectPaneSignals(pane)

	// Remove from container
	if pane.titleBar != nil {
		// The title bar is wrapped in a button, need to get parent
		parent := pane.titleBar.GetParent()
		if parent != nil {
			sv.box.Remove(parent)
		}
	}
	if pane.container != nil {
		sv.box.Remove(pane.container)
	}

	// Remove from slice
	sv.panes = append(sv.panes[:index], sv.panes[index+1:]...)

	// Adjust active index
	if sv.activeIndex >= len(sv.panes) {
		sv.activeIndex = len(sv.panes) - 1
	} else if sv.activeIndex > index {
		sv.activeIndex--
	}

	// Update visibility
	if sv.activeIndex >= 0 {
		sv.updateVisibilityInternal(ctx)
	}

	return nil
}

// SetActive activates the pane at the given index.
// The active pane's container is shown; inactive panes show only title bars.
func (sv *StackedView) SetActive(ctx context.Context, index int) error {
	log := logging.FromContext(ctx)
	sv.mu.Lock()
	defer sv.mu.Unlock()

	log.Debug().
		Int("index", index).
		Int("pane_count", len(sv.panes)).
		Msg("StackedView.SetActive called")

	if len(sv.panes) == 0 {
		return ErrStackEmpty
	}
	if index < 0 || index >= len(sv.panes) {
		return ErrIndexOutOfBounds
	}

	sv.setActiveInternal(ctx, index)
	return nil
}

// setActiveInternal sets the active pane without locking.
func (sv *StackedView) setActiveInternal(ctx context.Context, index int) {
	sv.activeIndex = index
	sv.updateVisibilityInternal(ctx)
}

// updateVisibilityInternal updates visibility of all panes based on activeIndex.
func (sv *StackedView) updateVisibilityInternal(ctx context.Context) {
	log := logging.FromContext(ctx)

	log.Debug().
		Int("active_index", sv.activeIndex).
		Int("pane_count", len(sv.panes)).
		Msg("StackedView.updateVisibilityInternal called")

	for i, pane := range sv.panes {
		isActive := i == sv.activeIndex
		pane.isActive = isActive

		// Title bar is always visible for inactive panes, hidden for active
		if pane.titleBar != nil {
			parent := pane.titleBar.GetParent()
			if parent != nil {
				parent.SetVisible(!isActive)
				log.Debug().
					Int("pane_index", i).
					Str("title", pane.title).
					Bool("is_active", isActive).
					Bool("titlebar_visible", !isActive).
					Msg("StackedView: set titlebar visibility")
			}
		}

		// Container is visible only for active pane
		if pane.container != nil {
			pane.container.SetVisible(isActive)
			log.Debug().
				Int("pane_index", i).
				Str("title", pane.title).
				Bool("is_active", isActive).
				Bool("container_visible", isActive).
				Msg("StackedView: set container visibility")
		}

		// Update CSS classes
		if pane.titleBar != nil {
			if isActive {
				pane.titleBar.AddCssClass("active")
			} else {
				pane.titleBar.RemoveCssClass("active")
			}
		}
	}
}

// ActiveIndex returns the index of the currently active pane.
// Returns -1 if the stack is empty.
func (sv *StackedView) ActiveIndex() int {
	sv.mu.RLock()
	defer sv.mu.RUnlock()

	return sv.activeIndex
}

// Count returns the number of panes in the stack.
func (sv *StackedView) Count() int {
	sv.mu.RLock()
	defer sv.mu.RUnlock()

	return len(sv.panes)
}

// FindPaneIndex returns the index of the pane with the given ID.
// Returns -1 if not found.
func (sv *StackedView) FindPaneIndex(paneID string) int {
	sv.mu.RLock()
	defer sv.mu.RUnlock()

	return sv.findPaneIndexInternal(paneID)
}

// findPaneIndexInternal returns the index of a pane by ID.
// Caller must hold at least a read lock on sv.mu.
func (sv *StackedView) findPaneIndexInternal(paneID string) int {
	for i, pane := range sv.panes {
		if pane.paneID == paneID {
			return i
		}
	}
	return -1
}

// SetOnActivate sets the callback for when a pane is activated via title bar click.
func (sv *StackedView) SetOnActivate(fn func(index int)) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	sv.onActivate = fn
}

// SetOnClosePane sets the callback for when a pane's close button is clicked.
func (sv *StackedView) SetOnClosePane(fn func(paneID string)) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	sv.onClosePane = fn
}

// UpdateTitle updates the title of a pane at the given index.
func (sv *StackedView) UpdateTitle(index int, title string) error {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if index < 0 || index >= len(sv.panes) {
		return ErrIndexOutOfBounds
	}

	sv.panes[index].title = title
	if sv.panes[index].label != nil {
		sv.panes[index].label.SetText(title)
	}

	return nil
}

// UpdateFavicon updates the favicon of a pane at the given index using an icon name.
func (sv *StackedView) UpdateFavicon(index int, iconName string) error {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if index < 0 || index >= len(sv.panes) {
		return ErrIndexOutOfBounds
	}

	if sv.panes[index].favicon != nil {
		if iconName != "" {
			sv.panes[index].favicon.SetFromIconName(iconName)
		} else {
			sv.panes[index].favicon.SetFromIconName("web-browser-symbolic")
		}
	}

	return nil
}

// UpdateFaviconTexture updates the favicon of a pane at the given index using a texture.
// If texture is nil, falls back to the default web-browser-symbolic icon.
func (sv *StackedView) UpdateFaviconTexture(index int, texture Paintable) error {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if index < 0 || index >= len(sv.panes) {
		return ErrIndexOutOfBounds
	}

	if sv.panes[index].favicon != nil {
		if texture != nil {
			sv.panes[index].favicon.SetFromPaintable(texture)
		} else {
			sv.panes[index].favicon.SetFromIconName("web-browser-symbolic")
		}
	}

	return nil
}

// GetContainer returns the container widget for the pane at the given index.
func (sv *StackedView) GetContainer(index int) (Widget, error) {
	sv.mu.RLock()
	defer sv.mu.RUnlock()

	if index < 0 || index >= len(sv.panes) {
		return nil, ErrIndexOutOfBounds
	}

	return sv.panes[index].container, nil
}

// Widget returns the underlying BoxWidget for embedding in containers.
func (sv *StackedView) Widget() Widget {
	return sv.box
}

// Box returns the underlying BoxWidget for direct access.
func (sv *StackedView) Box() BoxWidget {
	return sv.box
}

// NavigateNext moves to the next pane in the stack (wraps around).
func (sv *StackedView) NavigateNext(ctx context.Context) error {
	log := logging.FromContext(ctx)
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if len(sv.panes) == 0 {
		return ErrStackEmpty
	}

	newIndex := (sv.activeIndex + 1) % len(sv.panes)
	log.Debug().
		Int("old_index", sv.activeIndex).
		Int("new_index", newIndex).
		Msg("StackedView.NavigateNext")
	sv.setActiveInternal(ctx, newIndex)
	return nil
}

// NavigatePrevious moves to the previous pane in the stack (wraps around).
func (sv *StackedView) NavigatePrevious(ctx context.Context) error {
	log := logging.FromContext(ctx)
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if len(sv.panes) == 0 {
		return ErrStackEmpty
	}

	newIndex := (sv.activeIndex - 1 + len(sv.panes)) % len(sv.panes)
	log.Debug().
		Int("old_index", sv.activeIndex).
		Int("new_index", newIndex).
		Msg("StackedView.NavigatePrevious")
	sv.setActiveInternal(ctx, newIndex)
	return nil
}
