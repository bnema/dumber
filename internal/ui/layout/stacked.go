package layout

import (
	"errors"
	"sync"
)

// ErrStackEmpty is returned when operating on an empty stack.
var ErrStackEmpty = errors.New("stack is empty")

// ErrIndexOutOfBounds is returned when an index is out of range.
var ErrIndexOutOfBounds = errors.New("index out of bounds")

// ErrCannotRemoveLastPane is returned when trying to remove the last pane from a stack.
var ErrCannotRemoveLastPane = errors.New("cannot remove last pane from stack")

// stackedPane represents a single pane within a stacked container.
type stackedPane struct {
	titleBar  BoxWidget // horizontal box with favicon + title label
	container Widget    // the pane view widget
	title     string
	favicon   ImageWidget
	label     LabelWidget
	isActive  bool
}

// StackedView manages a stack of panes where only one is visible at a time.
// Inactive panes show title bars that can be clicked to activate them.
type StackedView struct {
	factory     WidgetFactory
	box         BoxWidget // vertical container for all content
	panes       []*stackedPane
	activeIndex int

	onActivate func(index int) // called when a pane is activated via title bar click

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

// AddPane adds a new pane to the stack.
// The new pane becomes active (visible).
func (sv *StackedView) AddPane(title, faviconIconName string, container Widget) int {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	// Create title bar
	titleBar := sv.factory.NewBox(OrientationHorizontal, 4)
	titleBar.AddCssClass("stacked-pane-titlebar")

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
	label.SetMaxWidthChars(30)
	label.SetHexpand(true)
	label.SetXalign(0.0)
	titleBar.Append(label)

	// Make title bar clickable
	titleButton := sv.factory.NewButton()
	titleButton.SetChild(titleBar)
	titleButton.AddCssClass("stacked-pane-title-button")
	titleButton.SetFocusOnClick(false)

	pane := &stackedPane{
		titleBar:  titleBar,
		container: container,
		title:     title,
		favicon:   favicon,
		label:     label,
		isActive:  false,
	}

	index := len(sv.panes)
	sv.panes = append(sv.panes, pane)

	// Connect click handler
	titleButton.ConnectClicked(func() {
		sv.mu.RLock()
		callback := sv.onActivate
		sv.mu.RUnlock()

		if callback != nil {
			callback(index)
		}
	})

	// Add to container
	sv.box.Append(titleButton)
	if container != nil {
		sv.box.Append(container)
	}

	// Set this pane as active
	sv.setActiveInternal(index)

	return index
}

// RemovePane removes a pane from the stack by index.
// Returns an error if trying to remove the last pane.
func (sv *StackedView) RemovePane(index int) error {
	sv.mu.Lock()
	defer sv.mu.Unlock()

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
		sv.updateVisibilityInternal()
	}

	return nil
}

// SetActive activates the pane at the given index.
// The active pane's container is shown; inactive panes show only title bars.
func (sv *StackedView) SetActive(index int) error {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if len(sv.panes) == 0 {
		return ErrStackEmpty
	}
	if index < 0 || index >= len(sv.panes) {
		return ErrIndexOutOfBounds
	}

	sv.setActiveInternal(index)
	return nil
}

// setActiveInternal sets the active pane without locking.
func (sv *StackedView) setActiveInternal(index int) {
	sv.activeIndex = index
	sv.updateVisibilityInternal()
}

// updateVisibilityInternal updates visibility of all panes based on activeIndex.
func (sv *StackedView) updateVisibilityInternal() {
	for i, pane := range sv.panes {
		isActive := i == sv.activeIndex
		pane.isActive = isActive

		// Title bar is always visible for inactive panes, hidden for active
		if pane.titleBar != nil {
			parent := pane.titleBar.GetParent()
			if parent != nil {
				parent.SetVisible(!isActive)
			}
		}

		// Container is visible only for active pane
		if pane.container != nil {
			pane.container.SetVisible(isActive)
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

// SetOnActivate sets the callback for when a pane is activated via title bar click.
func (sv *StackedView) SetOnActivate(fn func(index int)) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	sv.onActivate = fn
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

// UpdateFavicon updates the favicon of a pane at the given index.
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
func (sv *StackedView) NavigateNext() error {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if len(sv.panes) == 0 {
		return ErrStackEmpty
	}

	newIndex := (sv.activeIndex + 1) % len(sv.panes)
	sv.setActiveInternal(newIndex)
	return nil
}

// NavigatePrevious moves to the previous pane in the stack (wraps around).
func (sv *StackedView) NavigatePrevious() error {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if len(sv.panes) == 0 {
		return ErrStackEmpty
	}

	newIndex := (sv.activeIndex - 1 + len(sv.panes)) % len(sv.panes)
	sv.setActiveInternal(newIndex)
	return nil
}
