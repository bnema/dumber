package webkit

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

// Paned wraps gtk.Paned for split pane layouts
type Paned struct {
	paned *gtk.Paned
}

// NewPaned creates a new Paned widget with the given orientation
func NewPaned(orientation gtk.Orientation) *Paned {
	return &Paned{
		paned: gtk.NewPaned(orientation),
	}
}

// SetStartChild sets the first child of the paned
func (p *Paned) SetStartChild(child gtk.Widgetter) {
	if p == nil || p.paned == nil {
		return
	}
	p.paned.SetStartChild(child)
}

// SetEndChild sets the second child of the paned
func (p *Paned) SetEndChild(child gtk.Widgetter) {
	if p == nil || p.paned == nil {
		return
	}
	p.paned.SetEndChild(child)
}

// SetPosition sets the divider position
func (p *Paned) SetPosition(pos int) {
	if p == nil || p.paned == nil {
		return
	}
	p.paned.SetPosition(pos)
}

// GetPosition returns the divider position
func (p *Paned) GetPosition() int {
	if p == nil || p.paned == nil {
		return 0
	}
	return p.paned.Position()
}

// SetResizeStartChild sets whether the start child should resize
func (p *Paned) SetResizeStartChild(resize bool) {
	if p == nil || p.paned == nil {
		return
	}
	p.paned.SetResizeStartChild(resize)
}

// SetResizeEndChild sets whether the end child should resize
func (p *Paned) SetResizeEndChild(resize bool) {
	if p == nil || p.paned == nil {
		return
	}
	p.paned.SetResizeEndChild(resize)
}

// AsWidget returns the paned as a Widgetter
func (p *Paned) AsWidget() gtk.Widgetter {
	if p == nil || p.paned == nil {
		return nil
	}
	return p.paned
}

// StartChild returns the start child
func (p *Paned) StartChild() gtk.Widgetter {
	if p == nil || p.paned == nil {
		return nil
	}
	return p.paned.StartChild()
}

// EndChild returns the end child
func (p *Paned) EndChild() gtk.Widgetter {
	if p == nil || p.paned == nil {
		return nil
	}
	return p.paned.EndChild()
}

// Box wraps gtk.Box for layout management
type Box struct {
	box *gtk.Box
}

// NewBox creates a new Box with the given orientation and spacing
func NewBox(orientation gtk.Orientation, spacing int) *Box {
	return &Box{
		box: gtk.NewBox(orientation, spacing),
	}
}

// Append adds a child to the end of the box
func (b *Box) Append(child gtk.Widgetter) {
	if b == nil || b.box == nil {
		return
	}
	b.box.Append(child)
}

// Prepend adds a child to the beginning of the box
func (b *Box) Prepend(child gtk.Widgetter) {
	if b == nil || b.box == nil {
		return
	}
	b.box.Prepend(child)
}

// Remove removes a child from the box
func (b *Box) Remove(child gtk.Widgetter) {
	if b == nil || b.box == nil {
		return
	}
	b.box.Remove(child)
}

// InsertChildAfter inserts a child after the specified sibling
func (b *Box) InsertChildAfter(child, sibling gtk.Widgetter) {
	if b == nil || b.box == nil {
		return
	}
	b.box.InsertChildAfter(child, sibling)
}

// AsWidget returns the box as a Widgetter
func (b *Box) AsWidget() gtk.Widgetter {
	if b == nil || b.box == nil {
		return nil
	}
	return b.box
}

// Label wraps gtk.Label for text display
type Label struct {
	label *gtk.Label
}

// NewLabel creates a new Label with the given text
func NewLabel(text string) *Label {
	return &Label{
		label: gtk.NewLabel(text),
	}
}

// SetText updates the label text
func (l *Label) SetText(text string) {
	if l == nil || l.label == nil {
		return
	}
	l.label.SetText(text)
}

// GetText returns the current label text
func (l *Label) GetText() string {
	if l == nil || l.label == nil {
		return ""
	}
	return l.label.Text()
}

// SetEllipsize sets the ellipsization mode
func (l *Label) SetEllipsize(mode pango.EllipsizeMode) {
	if l == nil || l.label == nil {
		return
	}
	l.label.SetEllipsize(mode)
}

// SetMaxWidthChars sets the maximum width in characters
func (l *Label) SetMaxWidthChars(nChars int) {
	if l == nil || l.label == nil {
		return
	}
	l.label.SetMaxWidthChars(nChars)
}

// AsWidget returns the label as a Widgetter
func (l *Label) AsWidget() gtk.Widgetter {
	if l == nil || l.label == nil {
		return nil
	}
	return l.label
}

// Image wraps gtk.Image for displaying images (e.g., favicons)
type Image struct {
	image *gtk.Image
}

// NewImage creates a new empty Image
func NewImage() *Image {
	return &Image{
		image: gtk.NewImage(),
	}
}

// NewImageFromFile creates a new Image from a file
func NewImageFromFile(filename string) *Image {
	return &Image{
		image: gtk.NewImageFromFile(filename),
	}
}

// SetFromFile loads an image from a file
func (i *Image) SetFromFile(filename string) {
	if i == nil || i.image == nil {
		return
	}
	i.image.SetFromFile(filename)
}

// SetPixelSize sets the pixel size for the image
func (i *Image) SetPixelSize(size int) {
	if i == nil || i.image == nil {
		return
	}
	i.image.SetPixelSize(size)
}

// AsWidget returns the image as a Widgetter
func (i *Image) AsWidget() gtk.Widgetter {
	if i == nil || i.image == nil {
		return nil
	}
	return i.image
}

// Widget helper functions for common operations

// getWidget extracts the underlying *gtk.Widget from a Widgetter
// This is needed because some methods are only available on *gtk.Widget
func getWidget(w gtk.Widgetter) *gtk.Widget {
	if w == nil {
		return nil
	}
	// The Widgetter interface provides a way to get the base widget
	// In gotk4, we can cast to get access to the underlying widget
	if widget, ok := w.(interface{ Widget() *gtk.Widget }); ok {
		return widget.Widget()
	}
	// Fallback: try to type assert directly
	if widget, ok := w.(*gtk.Widget); ok {
		return widget
	}
	return nil
}

// WidgetShow makes a widget visible
func WidgetShow(w gtk.Widgetter) {
	if widget := getWidget(w); widget != nil {
		widget.SetVisible(true)
	}
}

// WidgetHide makes a widget invisible
func WidgetHide(w gtk.Widgetter) {
	if widget := getWidget(w); widget != nil {
		widget.SetVisible(false)
	}
}

// WidgetSetVisible sets the visibility of a widget
func WidgetSetVisible(w gtk.Widgetter, visible bool) {
	if widget := getWidget(w); widget != nil {
		widget.SetVisible(visible)
	}
}

// WidgetSetSizeRequest sets the minimum size request
func WidgetSetSizeRequest(w gtk.Widgetter, width, height int) {
	if widget := getWidget(w); widget != nil {
		widget.SetSizeRequest(width, height)
	}
}

// WidgetQueueResize queues a resize operation
func WidgetQueueResize(w gtk.Widgetter) {
	if widget := getWidget(w); widget != nil {
		widget.QueueResize()
	}
}

// WidgetGrabFocus gives focus to a widget
func WidgetGrabFocus(w gtk.Widgetter) {
	if widget := getWidget(w); widget != nil {
		widget.GrabFocus()
	}
}
