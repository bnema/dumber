package layout

import (
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gobject"
	"github.com/jwijenbergh/puregotk/v4/graphene"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	"github.com/jwijenbergh/puregotk/v4/pango"
)

// Ensure implementations satisfy interfaces at compile time.
var (
	_ Widget            = (*gtkWidget)(nil)
	_ PanedWidget       = (*gtkPaned)(nil)
	_ BoxWidget         = (*gtkBox)(nil)
	_ OverlayWidget     = (*gtkOverlay)(nil)
	_ LabelWidget       = (*gtkLabel)(nil)
	_ ButtonWidget      = (*gtkButton)(nil)
	_ ImageWidget       = (*gtkImage)(nil)
	_ ProgressBarWidget = (*gtkProgressBar)(nil)
	_ SpinnerWidget     = (*gtkSpinner)(nil)
	_ WidgetFactory     = (*GtkWidgetFactory)(nil)
)

// gtkWidget wraps a gtk.Widget to implement the Widget interface.
type gtkWidget struct {
	inner *gtk.Widget
}

func (w *gtkWidget) Show()                         { w.inner.Show() }
func (w *gtkWidget) Hide()                         { w.inner.Hide() }
func (w *gtkWidget) SetVisible(visible bool)       { w.inner.SetVisible(visible) }
func (w *gtkWidget) IsVisible() bool               { return w.inner.GetVisible() }
func (w *gtkWidget) GrabFocus() bool               { return w.inner.GrabFocus() }
func (w *gtkWidget) HasFocus() bool                { return w.inner.HasFocus() }
func (w *gtkWidget) SetCanFocus(canFocus bool)     { w.inner.SetCanFocus(canFocus) }
func (w *gtkWidget) SetFocusable(focusable bool)   { w.inner.SetFocusable(focusable) }
func (w *gtkWidget) SetFocusOnClick(focus bool)    { w.inner.SetFocusOnClick(focus) }
func (w *gtkWidget) SetCanTarget(canTarget bool)   { w.inner.SetCanTarget(canTarget) }
func (w *gtkWidget) SetHexpand(expand bool)        { w.inner.SetHexpand(expand) }
func (w *gtkWidget) SetVexpand(expand bool)        { w.inner.SetVexpand(expand) }
func (w *gtkWidget) GetHexpand() bool              { return w.inner.GetHexpand() }
func (w *gtkWidget) GetVexpand() bool              { return w.inner.GetVexpand() }
func (w *gtkWidget) SetHalign(align gtk.Align)     { w.inner.SetHalign(align) }
func (w *gtkWidget) SetValign(align gtk.Align)     { w.inner.SetValign(align) }
func (w *gtkWidget) SetSizeRequest(w2, h int)      { w.inner.SetSizeRequest(w2, h) }
func (w *gtkWidget) AddCssClass(class string)      { w.inner.AddCssClass(class) }
func (w *gtkWidget) RemoveCssClass(class string)   { w.inner.RemoveCssClass(class) }
func (w *gtkWidget) HasCssClass(class string) bool { return w.inner.HasCssClass(class) }
func (w *gtkWidget) Unparent()                     { w.inner.Unparent() }
func (w *gtkWidget) GtkWidget() *gtk.Widget        { return w.inner }

func (w *gtkWidget) GetParent() Widget {
	parent := w.inner.GetParent()
	if parent == nil {
		return nil
	}
	return &gtkWidget{inner: parent}
}

func (w *gtkWidget) GetAllocatedWidth() int               { return w.inner.GetAllocatedWidth() }
func (w *gtkWidget) GetAllocatedHeight() int              { return w.inner.GetAllocatedHeight() }
func (w *gtkWidget) AddController(c *gtk.EventController) { w.inner.AddController(c) }

func (w *gtkWidget) ComputePoint(target Widget) (x, y float64, ok bool) {
	srcPoint := &graphene.Point{X: 0, Y: 0}
	outPoint := &graphene.Point{}

	var targetGtk *gtk.Widget
	if target != nil {
		targetGtk = target.GtkWidget()
	}

	ok = w.inner.ComputePoint(targetGtk, srcPoint, outPoint)
	if !ok {
		return 0, 0, false
	}
	return float64(outPoint.X), float64(outPoint.Y), true
}

// gtkPaned wraps gtk.Paned to implement PanedWidget.
type gtkPaned struct {
	inner *gtk.Paned
}

func (p *gtkPaned) Show()                         { p.inner.Show() }
func (p *gtkPaned) Hide()                         { p.inner.Hide() }
func (p *gtkPaned) SetVisible(visible bool)       { p.inner.SetVisible(visible) }
func (p *gtkPaned) IsVisible() bool               { return p.inner.GetVisible() }
func (p *gtkPaned) GrabFocus() bool               { return p.inner.GrabFocus() }
func (p *gtkPaned) HasFocus() bool                { return p.inner.HasFocus() }
func (p *gtkPaned) SetCanFocus(canFocus bool)     { p.inner.SetCanFocus(canFocus) }
func (p *gtkPaned) SetFocusable(focusable bool)   { p.inner.SetFocusable(focusable) }
func (p *gtkPaned) SetFocusOnClick(focus bool)    { p.inner.SetFocusOnClick(focus) }
func (p *gtkPaned) SetCanTarget(canTarget bool)   { p.inner.SetCanTarget(canTarget) }
func (p *gtkPaned) SetHexpand(expand bool)        { p.inner.SetHexpand(expand) }
func (p *gtkPaned) SetVexpand(expand bool)        { p.inner.SetVexpand(expand) }
func (p *gtkPaned) GetHexpand() bool              { return p.inner.GetHexpand() }
func (p *gtkPaned) GetVexpand() bool              { return p.inner.GetVexpand() }
func (p *gtkPaned) SetHalign(align gtk.Align)     { p.inner.SetHalign(align) }
func (p *gtkPaned) SetValign(align gtk.Align)     { p.inner.SetValign(align) }
func (p *gtkPaned) SetSizeRequest(w, h int)       { p.inner.SetSizeRequest(w, h) }
func (p *gtkPaned) AddCssClass(class string)      { p.inner.AddCssClass(class) }
func (p *gtkPaned) RemoveCssClass(class string)   { p.inner.RemoveCssClass(class) }
func (p *gtkPaned) HasCssClass(class string) bool { return p.inner.HasCssClass(class) }
func (p *gtkPaned) Unparent()                     { p.inner.Unparent() }
func (p *gtkPaned) GtkWidget() *gtk.Widget        { return &p.inner.Widget }

func (p *gtkPaned) GetParent() Widget {
	parent := p.inner.GetParent()
	if parent == nil {
		return nil
	}
	return &gtkWidget{inner: parent}
}

func (p *gtkPaned) SetStartChild(child Widget) {
	if child == nil {
		p.inner.SetStartChild(nil)
		return
	}
	p.inner.SetStartChild(child.GtkWidget())
}

func (p *gtkPaned) SetEndChild(child Widget) {
	if child == nil {
		p.inner.SetEndChild(nil)
		return
	}
	p.inner.SetEndChild(child.GtkWidget())
}

func (p *gtkPaned) GetStartChild() Widget {
	child := p.inner.GetStartChild()
	if child == nil {
		return nil
	}
	return &gtkWidget{inner: child}
}

func (p *gtkPaned) GetEndChild() Widget {
	child := p.inner.GetEndChild()
	if child == nil {
		return nil
	}
	return &gtkWidget{inner: child}
}

func (p *gtkPaned) SetPosition(pos int)        { p.inner.SetPosition(pos) }
func (p *gtkPaned) GetPosition() int           { return p.inner.GetPosition() }
func (p *gtkPaned) SetResizeStartChild(r bool) { p.inner.SetResizeStartChild(r) }
func (p *gtkPaned) SetResizeEndChild(r bool)   { p.inner.SetResizeEndChild(r) }
func (p *gtkPaned) GetResizeStartChild() bool  { return p.inner.GetResizeStartChild() }
func (p *gtkPaned) GetResizeEndChild() bool    { return p.inner.GetResizeEndChild() }
func (p *gtkPaned) SetShrinkStartChild(s bool) { p.inner.SetShrinkStartChild(s) }
func (p *gtkPaned) SetShrinkEndChild(s bool)   { p.inner.SetShrinkEndChild(s) }
func (p *gtkPaned) GetShrinkStartChild() bool  { return p.inner.GetShrinkStartChild() }
func (p *gtkPaned) GetShrinkEndChild() bool    { return p.inner.GetShrinkEndChild() }
func (p *gtkPaned) SetWideHandle(wide bool)    { p.inner.SetWideHandle(wide) }
func (p *gtkPaned) GetWideHandle() bool        { return p.inner.GetWideHandle() }

func (p *gtkPaned) ConnectMap(callback func()) uint32 {
	cb := func(_ gtk.Widget) {
		callback()
	}
	return p.inner.ConnectMap(&cb)
}

func (p *gtkPaned) ConnectNotifyPosition(callback func()) uint32 {
	cb := func(_ gobject.Object, _ uintptr) {
		callback()
	}
	return p.inner.ConnectNotifyWithDetail("position", &cb)
}

func (p *gtkPaned) AddTickCallback(callback func() bool) uint {
	cb := gtk.TickCallback(func(widget uintptr, frameClock uintptr, userData uintptr) bool {
		return callback()
	})
	return p.inner.AddTickCallback(&cb, 0, nil)
}

func (p *gtkPaned) GetAllocatedWidth() int               { return p.inner.GetAllocatedWidth() }
func (p *gtkPaned) GetAllocatedHeight() int              { return p.inner.GetAllocatedHeight() }
func (p *gtkPaned) AddController(c *gtk.EventController) { p.inner.AddController(c) }

func (p *gtkPaned) ComputePoint(target Widget) (x, y float64, ok bool) {
	srcPoint := &graphene.Point{X: 0, Y: 0}
	outPoint := &graphene.Point{}

	var targetGtk *gtk.Widget
	if target != nil {
		targetGtk = target.GtkWidget()
	}

	ok = p.inner.ComputePoint(targetGtk, srcPoint, outPoint)
	if !ok {
		return 0, 0, false
	}
	return float64(outPoint.X), float64(outPoint.Y), true
}

// gtkBox wraps gtk.Box to implement BoxWidget.
type gtkBox struct {
	inner *gtk.Box
}

func (b *gtkBox) Show()                         { b.inner.Show() }
func (b *gtkBox) Hide()                         { b.inner.Hide() }
func (b *gtkBox) SetVisible(visible bool)       { b.inner.SetVisible(visible) }
func (b *gtkBox) IsVisible() bool               { return b.inner.GetVisible() }
func (b *gtkBox) GrabFocus() bool               { return b.inner.GrabFocus() }
func (b *gtkBox) HasFocus() bool                { return b.inner.HasFocus() }
func (b *gtkBox) SetCanFocus(canFocus bool)     { b.inner.SetCanFocus(canFocus) }
func (b *gtkBox) SetFocusable(focusable bool)   { b.inner.SetFocusable(focusable) }
func (b *gtkBox) SetFocusOnClick(focus bool)    { b.inner.SetFocusOnClick(focus) }
func (b *gtkBox) SetCanTarget(canTarget bool)   { b.inner.SetCanTarget(canTarget) }
func (b *gtkBox) SetHexpand(expand bool)        { b.inner.SetHexpand(expand) }
func (b *gtkBox) SetVexpand(expand bool)        { b.inner.SetVexpand(expand) }
func (b *gtkBox) GetHexpand() bool              { return b.inner.GetHexpand() }
func (b *gtkBox) GetVexpand() bool              { return b.inner.GetVexpand() }
func (b *gtkBox) SetHalign(align gtk.Align)     { b.inner.SetHalign(align) }
func (b *gtkBox) SetValign(align gtk.Align)     { b.inner.SetValign(align) }
func (b *gtkBox) SetSizeRequest(w, h int)       { b.inner.SetSizeRequest(w, h) }
func (b *gtkBox) AddCssClass(class string)      { b.inner.AddCssClass(class) }
func (b *gtkBox) RemoveCssClass(class string)   { b.inner.RemoveCssClass(class) }
func (b *gtkBox) HasCssClass(class string) bool { return b.inner.HasCssClass(class) }
func (b *gtkBox) Unparent()                     { b.inner.Unparent() }
func (b *gtkBox) GtkWidget() *gtk.Widget        { return &b.inner.Widget }

func (b *gtkBox) GetParent() Widget {
	parent := b.inner.GetParent()
	if parent == nil {
		return nil
	}
	return &gtkWidget{inner: parent}
}

func (b *gtkBox) Append(child Widget) {
	if child == nil {
		return
	}
	b.inner.Append(child.GtkWidget())
}

func (b *gtkBox) Prepend(child Widget) {
	if child == nil {
		return
	}
	b.inner.Prepend(child.GtkWidget())
}

func (b *gtkBox) Remove(child Widget) {
	if child == nil {
		return
	}
	b.inner.Remove(child.GtkWidget())
}

func (b *gtkBox) InsertChildAfter(child, sibling Widget) {
	if child == nil {
		return
	}
	var sibGtk *gtk.Widget
	if sibling != nil {
		sibGtk = sibling.GtkWidget()
	}
	b.inner.InsertChildAfter(child.GtkWidget(), sibGtk)
}

func (b *gtkBox) ReorderChildAfter(child, sibling Widget) {
	if child == nil {
		return
	}
	var sibGtk *gtk.Widget
	if sibling != nil {
		sibGtk = sibling.GtkWidget()
	}
	b.inner.ReorderChildAfter(child.GtkWidget(), sibGtk)
}

func (b *gtkBox) SetHomogeneous(h bool)        { b.inner.SetHomogeneous(h) }
func (b *gtkBox) GetHomogeneous() bool         { return b.inner.GetHomogeneous() }
func (b *gtkBox) SetSpacing(s int)             { b.inner.SetSpacing(s) }
func (b *gtkBox) GetSpacing() int              { return b.inner.GetSpacing() }
func (b *gtkBox) SetOrientation(o Orientation) { b.inner.SetOrientation(o) }
func (b *gtkBox) GetOrientation() Orientation  { return b.inner.GetOrientation() }

func (b *gtkBox) GetAllocatedWidth() int               { return b.inner.GetAllocatedWidth() }
func (b *gtkBox) GetAllocatedHeight() int              { return b.inner.GetAllocatedHeight() }
func (b *gtkBox) AddController(c *gtk.EventController) { b.inner.AddController(c) }

func (b *gtkBox) ComputePoint(target Widget) (x, y float64, ok bool) {
	srcPoint := &graphene.Point{X: 0, Y: 0}
	outPoint := &graphene.Point{}

	var targetGtk *gtk.Widget
	if target != nil {
		targetGtk = target.GtkWidget()
	}

	ok = b.inner.ComputePoint(targetGtk, srcPoint, outPoint)
	if !ok {
		return 0, 0, false
	}
	return float64(outPoint.X), float64(outPoint.Y), true
}

// gtkOverlay wraps gtk.Overlay to implement OverlayWidget.
type gtkOverlay struct {
	inner *gtk.Overlay
}

func (o *gtkOverlay) Show()                         { o.inner.Show() }
func (o *gtkOverlay) Hide()                         { o.inner.Hide() }
func (o *gtkOverlay) SetVisible(visible bool)       { o.inner.SetVisible(visible) }
func (o *gtkOverlay) IsVisible() bool               { return o.inner.GetVisible() }
func (o *gtkOverlay) GrabFocus() bool               { return o.inner.GrabFocus() }
func (o *gtkOverlay) HasFocus() bool                { return o.inner.HasFocus() }
func (o *gtkOverlay) SetCanFocus(canFocus bool)     { o.inner.SetCanFocus(canFocus) }
func (o *gtkOverlay) SetFocusable(focusable bool)   { o.inner.SetFocusable(focusable) }
func (o *gtkOverlay) SetFocusOnClick(focus bool)    { o.inner.SetFocusOnClick(focus) }
func (o *gtkOverlay) SetCanTarget(canTarget bool)   { o.inner.SetCanTarget(canTarget) }
func (o *gtkOverlay) SetHexpand(expand bool)        { o.inner.SetHexpand(expand) }
func (o *gtkOverlay) SetVexpand(expand bool)        { o.inner.SetVexpand(expand) }
func (o *gtkOverlay) GetHexpand() bool              { return o.inner.GetHexpand() }
func (o *gtkOverlay) GetVexpand() bool              { return o.inner.GetVexpand() }
func (o *gtkOverlay) SetHalign(align gtk.Align)     { o.inner.SetHalign(align) }
func (o *gtkOverlay) SetValign(align gtk.Align)     { o.inner.SetValign(align) }
func (o *gtkOverlay) SetSizeRequest(w, h int)       { o.inner.SetSizeRequest(w, h) }
func (o *gtkOverlay) AddCssClass(class string)      { o.inner.AddCssClass(class) }
func (o *gtkOverlay) RemoveCssClass(class string)   { o.inner.RemoveCssClass(class) }
func (o *gtkOverlay) HasCssClass(class string) bool { return o.inner.HasCssClass(class) }
func (o *gtkOverlay) Unparent()                     { o.inner.Unparent() }
func (o *gtkOverlay) GtkWidget() *gtk.Widget        { return &o.inner.Widget }

func (o *gtkOverlay) GetParent() Widget {
	parent := o.inner.GetParent()
	if parent == nil {
		return nil
	}
	return &gtkWidget{inner: parent}
}

func (o *gtkOverlay) SetChild(child Widget) {
	if child == nil {
		o.inner.SetChild(nil)
		return
	}
	o.inner.SetChild(child.GtkWidget())
}

func (o *gtkOverlay) GetChild() Widget {
	child := o.inner.GetChild()
	if child == nil {
		return nil
	}
	return &gtkWidget{inner: child}
}

func (o *gtkOverlay) AddOverlay(overlay Widget) {
	if overlay == nil {
		return
	}
	o.inner.AddOverlay(overlay.GtkWidget())
}

func (o *gtkOverlay) RemoveOverlay(overlay Widget) {
	if overlay == nil {
		return
	}
	o.inner.RemoveOverlay(overlay.GtkWidget())
}

func (o *gtkOverlay) SetClipOverlay(overlay Widget, clip bool) {
	if overlay == nil {
		return
	}
	o.inner.SetClipOverlay(overlay.GtkWidget(), clip)
}

func (o *gtkOverlay) GetClipOverlay(overlay Widget) bool {
	if overlay == nil {
		return false
	}
	return o.inner.GetClipOverlay(overlay.GtkWidget())
}

func (o *gtkOverlay) SetMeasureOverlay(overlay Widget, measure bool) {
	if overlay == nil {
		return
	}
	o.inner.SetMeasureOverlay(overlay.GtkWidget(), measure)
}

func (o *gtkOverlay) GetMeasureOverlay(overlay Widget) bool {
	if overlay == nil {
		return false
	}
	return o.inner.GetMeasureOverlay(overlay.GtkWidget())
}

func (o *gtkOverlay) GetAllocatedWidth() int               { return o.inner.GetAllocatedWidth() }
func (o *gtkOverlay) GetAllocatedHeight() int              { return o.inner.GetAllocatedHeight() }
func (o *gtkOverlay) AddController(c *gtk.EventController) { o.inner.AddController(c) }

func (o *gtkOverlay) ComputePoint(target Widget) (x, y float64, ok bool) {
	srcPoint := &graphene.Point{X: 0, Y: 0}
	outPoint := &graphene.Point{}

	var targetGtk *gtk.Widget
	if target != nil {
		targetGtk = target.GtkWidget()
	}

	ok = o.inner.ComputePoint(targetGtk, srcPoint, outPoint)
	if !ok {
		return 0, 0, false
	}
	return float64(outPoint.X), float64(outPoint.Y), true
}

// gtkLabel wraps gtk.Label to implement LabelWidget.
type gtkLabel struct {
	inner *gtk.Label
}

func (l *gtkLabel) Show()                         { l.inner.Show() }
func (l *gtkLabel) Hide()                         { l.inner.Hide() }
func (l *gtkLabel) SetVisible(visible bool)       { l.inner.SetVisible(visible) }
func (l *gtkLabel) IsVisible() bool               { return l.inner.GetVisible() }
func (l *gtkLabel) GrabFocus() bool               { return l.inner.GrabFocus() }
func (l *gtkLabel) HasFocus() bool                { return l.inner.HasFocus() }
func (l *gtkLabel) SetCanFocus(canFocus bool)     { l.inner.SetCanFocus(canFocus) }
func (l *gtkLabel) SetFocusable(focusable bool)   { l.inner.SetFocusable(focusable) }
func (l *gtkLabel) SetFocusOnClick(focus bool)    { l.inner.SetFocusOnClick(focus) }
func (l *gtkLabel) SetCanTarget(canTarget bool)   { l.inner.SetCanTarget(canTarget) }
func (l *gtkLabel) SetHexpand(expand bool)        { l.inner.SetHexpand(expand) }
func (l *gtkLabel) SetVexpand(expand bool)        { l.inner.SetVexpand(expand) }
func (l *gtkLabel) GetHexpand() bool              { return l.inner.GetHexpand() }
func (l *gtkLabel) GetVexpand() bool              { return l.inner.GetVexpand() }
func (l *gtkLabel) SetHalign(align gtk.Align)     { l.inner.SetHalign(align) }
func (l *gtkLabel) SetValign(align gtk.Align)     { l.inner.SetValign(align) }
func (l *gtkLabel) SetSizeRequest(w, h int)       { l.inner.SetSizeRequest(w, h) }
func (l *gtkLabel) AddCssClass(class string)      { l.inner.AddCssClass(class) }
func (l *gtkLabel) RemoveCssClass(class string)   { l.inner.RemoveCssClass(class) }
func (l *gtkLabel) HasCssClass(class string) bool { return l.inner.HasCssClass(class) }
func (l *gtkLabel) Unparent()                     { l.inner.Unparent() }
func (l *gtkLabel) GtkWidget() *gtk.Widget        { return &l.inner.Widget }

func (l *gtkLabel) GetParent() Widget {
	parent := l.inner.GetParent()
	if parent == nil {
		return nil
	}
	return &gtkWidget{inner: parent}
}

func (l *gtkLabel) SetText(text string)             { l.inner.SetText(text) }
func (l *gtkLabel) GetText() string                 { return l.inner.GetText() }
func (l *gtkLabel) SetMarkup(markup string)         { l.inner.SetMarkup(markup) }
func (l *gtkLabel) SetEllipsize(mode EllipsizeMode) { l.inner.SetEllipsize(pango.EllipsizeMode(mode)) }
func (l *gtkLabel) SetMaxWidthChars(n int)          { l.inner.SetMaxWidthChars(n) }
func (l *gtkLabel) SetXalign(x float32)             { l.inner.SetXalign(x) }

func (l *gtkLabel) GetAllocatedWidth() int               { return l.inner.GetAllocatedWidth() }
func (l *gtkLabel) GetAllocatedHeight() int              { return l.inner.GetAllocatedHeight() }
func (l *gtkLabel) AddController(c *gtk.EventController) { l.inner.AddController(c) }

func (l *gtkLabel) ComputePoint(target Widget) (x, y float64, ok bool) {
	srcPoint := &graphene.Point{X: 0, Y: 0}
	outPoint := &graphene.Point{}

	var targetGtk *gtk.Widget
	if target != nil {
		targetGtk = target.GtkWidget()
	}

	ok = l.inner.ComputePoint(targetGtk, srcPoint, outPoint)
	if !ok {
		return 0, 0, false
	}
	return float64(outPoint.X), float64(outPoint.Y), true
}

// gtkButton wraps gtk.Button to implement ButtonWidget.
type gtkButton struct {
	inner *gtk.Button
}

func (btn *gtkButton) Show()                         { btn.inner.Show() }
func (btn *gtkButton) Hide()                         { btn.inner.Hide() }
func (btn *gtkButton) SetVisible(visible bool)       { btn.inner.SetVisible(visible) }
func (btn *gtkButton) IsVisible() bool               { return btn.inner.GetVisible() }
func (btn *gtkButton) GrabFocus() bool               { return btn.inner.GrabFocus() }
func (btn *gtkButton) HasFocus() bool                { return btn.inner.HasFocus() }
func (btn *gtkButton) SetCanFocus(canFocus bool)     { btn.inner.SetCanFocus(canFocus) }
func (btn *gtkButton) SetFocusable(focusable bool)   { btn.inner.SetFocusable(focusable) }
func (btn *gtkButton) SetFocusOnClick(focus bool)    { btn.inner.SetFocusOnClick(focus) }
func (btn *gtkButton) SetCanTarget(canTarget bool)   { btn.inner.SetCanTarget(canTarget) }
func (btn *gtkButton) SetHexpand(expand bool)        { btn.inner.SetHexpand(expand) }
func (btn *gtkButton) SetVexpand(expand bool)        { btn.inner.SetVexpand(expand) }
func (btn *gtkButton) GetHexpand() bool              { return btn.inner.GetHexpand() }
func (btn *gtkButton) GetVexpand() bool              { return btn.inner.GetVexpand() }
func (btn *gtkButton) SetHalign(align gtk.Align)     { btn.inner.SetHalign(align) }
func (btn *gtkButton) SetValign(align gtk.Align)     { btn.inner.SetValign(align) }
func (btn *gtkButton) SetSizeRequest(w, h int)       { btn.inner.SetSizeRequest(w, h) }
func (btn *gtkButton) AddCssClass(class string)      { btn.inner.AddCssClass(class) }
func (btn *gtkButton) RemoveCssClass(class string)   { btn.inner.RemoveCssClass(class) }
func (btn *gtkButton) HasCssClass(class string) bool { return btn.inner.HasCssClass(class) }
func (btn *gtkButton) Unparent()                     { btn.inner.Unparent() }
func (btn *gtkButton) GtkWidget() *gtk.Widget        { return &btn.inner.Widget }

func (btn *gtkButton) GetParent() Widget {
	parent := btn.inner.GetParent()
	if parent == nil {
		return nil
	}
	return &gtkWidget{inner: parent}
}

func (btn *gtkButton) SetLabel(label string) { btn.inner.SetLabel(label) }
func (btn *gtkButton) GetLabel() string      { return btn.inner.GetLabel() }

func (btn *gtkButton) SetChild(child Widget) {
	if child == nil {
		btn.inner.SetChild(nil)
		return
	}
	btn.inner.SetChild(child.GtkWidget())
}

func (btn *gtkButton) GetChild() Widget {
	child := btn.inner.GetChild()
	if child == nil {
		return nil
	}
	return &gtkWidget{inner: child}
}

func (btn *gtkButton) ConnectClicked(callback func()) uint32 {
	cb := func(_ gtk.Button) {
		callback()
	}
	return btn.inner.ConnectClicked(&cb)
}

func (btn *gtkButton) GetAllocatedWidth() int               { return btn.inner.GetAllocatedWidth() }
func (btn *gtkButton) GetAllocatedHeight() int              { return btn.inner.GetAllocatedHeight() }
func (btn *gtkButton) AddController(c *gtk.EventController) { btn.inner.AddController(c) }

func (btn *gtkButton) ComputePoint(target Widget) (x, y float64, ok bool) {
	srcPoint := &graphene.Point{X: 0, Y: 0}
	outPoint := &graphene.Point{}

	var targetGtk *gtk.Widget
	if target != nil {
		targetGtk = target.GtkWidget()
	}

	ok = btn.inner.ComputePoint(targetGtk, srcPoint, outPoint)
	if !ok {
		return 0, 0, false
	}
	return float64(outPoint.X), float64(outPoint.Y), true
}

// gtkImage wraps gtk.Image to implement ImageWidget.
type gtkImage struct {
	inner *gtk.Image
}

func (img *gtkImage) Show()                         { img.inner.Show() }
func (img *gtkImage) Hide()                         { img.inner.Hide() }
func (img *gtkImage) SetVisible(visible bool)       { img.inner.SetVisible(visible) }
func (img *gtkImage) IsVisible() bool               { return img.inner.GetVisible() }
func (img *gtkImage) GrabFocus() bool               { return img.inner.GrabFocus() }
func (img *gtkImage) HasFocus() bool                { return img.inner.HasFocus() }
func (img *gtkImage) SetCanFocus(canFocus bool)     { img.inner.SetCanFocus(canFocus) }
func (img *gtkImage) SetFocusable(focusable bool)   { img.inner.SetFocusable(focusable) }
func (img *gtkImage) SetFocusOnClick(focus bool)    { img.inner.SetFocusOnClick(focus) }
func (img *gtkImage) SetCanTarget(canTarget bool)   { img.inner.SetCanTarget(canTarget) }
func (img *gtkImage) SetHexpand(expand bool)        { img.inner.SetHexpand(expand) }
func (img *gtkImage) SetVexpand(expand bool)        { img.inner.SetVexpand(expand) }
func (img *gtkImage) GetHexpand() bool              { return img.inner.GetHexpand() }
func (img *gtkImage) GetVexpand() bool              { return img.inner.GetVexpand() }
func (img *gtkImage) SetHalign(align gtk.Align)     { img.inner.SetHalign(align) }
func (img *gtkImage) SetValign(align gtk.Align)     { img.inner.SetValign(align) }
func (img *gtkImage) SetSizeRequest(w, h int)       { img.inner.SetSizeRequest(w, h) }
func (img *gtkImage) AddCssClass(class string)      { img.inner.AddCssClass(class) }
func (img *gtkImage) RemoveCssClass(class string)   { img.inner.RemoveCssClass(class) }
func (img *gtkImage) HasCssClass(class string) bool { return img.inner.HasCssClass(class) }
func (img *gtkImage) Unparent()                     { img.inner.Unparent() }
func (img *gtkImage) GtkWidget() *gtk.Widget        { return &img.inner.Widget }

func (img *gtkImage) GetParent() Widget {
	parent := img.inner.GetParent()
	if parent == nil {
		return nil
	}
	return &gtkWidget{inner: parent}
}

func (img *gtkImage) SetFromIconName(name string) { img.inner.SetFromIconName(&name) }
func (img *gtkImage) SetFromFile(filename string) { img.inner.SetFromFile(&filename) }
func (img *gtkImage) SetFromPaintable(p Paintable) {
	if p == nil {
		img.inner.Clear()
		return
	}
	// Create a gdk.Paintable from the pointer
	// gdk.Texture implements gdk.Paintable
	texture := &gdk.Texture{}
	texture.Ptr = p.GoPointer()
	img.inner.SetFromPaintable(texture)
}
func (img *gtkImage) SetPixelSize(size int) { img.inner.SetPixelSize(size) }
func (img *gtkImage) Clear()                { img.inner.Clear() }

func (img *gtkImage) GetAllocatedWidth() int               { return img.inner.GetAllocatedWidth() }
func (img *gtkImage) GetAllocatedHeight() int              { return img.inner.GetAllocatedHeight() }
func (img *gtkImage) AddController(c *gtk.EventController) { img.inner.AddController(c) }

func (img *gtkImage) ComputePoint(target Widget) (x, y float64, ok bool) {
	srcPoint := &graphene.Point{X: 0, Y: 0}
	outPoint := &graphene.Point{}

	var targetGtk *gtk.Widget
	if target != nil {
		targetGtk = target.GtkWidget()
	}

	ok = img.inner.ComputePoint(targetGtk, srcPoint, outPoint)
	if !ok {
		return 0, 0, false
	}
	return float64(outPoint.X), float64(outPoint.Y), true
}

// gtkProgressBar wraps gtk.ProgressBar to implement ProgressBarWidget.
type gtkProgressBar struct {
	inner *gtk.ProgressBar
}

func (p *gtkProgressBar) Show()                         { p.inner.Show() }
func (p *gtkProgressBar) Hide()                         { p.inner.Hide() }
func (p *gtkProgressBar) SetVisible(visible bool)       { p.inner.SetVisible(visible) }
func (p *gtkProgressBar) IsVisible() bool               { return p.inner.GetVisible() }
func (p *gtkProgressBar) GrabFocus() bool               { return p.inner.GrabFocus() }
func (p *gtkProgressBar) HasFocus() bool                { return p.inner.HasFocus() }
func (p *gtkProgressBar) SetCanFocus(canFocus bool)     { p.inner.SetCanFocus(canFocus) }
func (p *gtkProgressBar) SetFocusable(focusable bool)   { p.inner.SetFocusable(focusable) }
func (p *gtkProgressBar) SetFocusOnClick(focus bool)    { p.inner.SetFocusOnClick(focus) }
func (p *gtkProgressBar) SetCanTarget(canTarget bool)   { p.inner.SetCanTarget(canTarget) }
func (p *gtkProgressBar) SetHexpand(expand bool)        { p.inner.SetHexpand(expand) }
func (p *gtkProgressBar) SetVexpand(expand bool)        { p.inner.SetVexpand(expand) }
func (p *gtkProgressBar) GetHexpand() bool              { return p.inner.GetHexpand() }
func (p *gtkProgressBar) GetVexpand() bool              { return p.inner.GetVexpand() }
func (p *gtkProgressBar) SetHalign(align gtk.Align)     { p.inner.SetHalign(align) }
func (p *gtkProgressBar) SetValign(align gtk.Align)     { p.inner.SetValign(align) }
func (p *gtkProgressBar) SetSizeRequest(w, h int)       { p.inner.SetSizeRequest(w, h) }
func (p *gtkProgressBar) AddCssClass(class string)      { p.inner.AddCssClass(class) }
func (p *gtkProgressBar) RemoveCssClass(class string)   { p.inner.RemoveCssClass(class) }
func (p *gtkProgressBar) HasCssClass(class string) bool { return p.inner.HasCssClass(class) }
func (p *gtkProgressBar) Unparent()                     { p.inner.Unparent() }
func (p *gtkProgressBar) GtkWidget() *gtk.Widget        { return &p.inner.Widget }

func (p *gtkProgressBar) GetParent() Widget {
	parent := p.inner.GetParent()
	if parent == nil {
		return nil
	}
	return &gtkWidget{inner: parent}
}

func (p *gtkProgressBar) GetAllocatedWidth() int               { return p.inner.GetAllocatedWidth() }
func (p *gtkProgressBar) GetAllocatedHeight() int              { return p.inner.GetAllocatedHeight() }
func (p *gtkProgressBar) AddController(c *gtk.EventController) { p.inner.AddController(c) }

func (p *gtkProgressBar) ComputePoint(target Widget) (x, y float64, ok bool) {
	srcPoint := &graphene.Point{X: 0, Y: 0}
	outPoint := &graphene.Point{}

	var targetGtk *gtk.Widget
	if target != nil {
		targetGtk = target.GtkWidget()
	}

	ok = p.inner.ComputePoint(targetGtk, srcPoint, outPoint)
	if !ok {
		return 0, 0, false
	}
	return float64(outPoint.X), float64(outPoint.Y), true
}

func (p *gtkProgressBar) SetFraction(fraction float64) { p.inner.SetFraction(fraction) }
func (p *gtkProgressBar) GetFraction() float64         { return p.inner.GetFraction() }

// gtkSpinner wraps gtk.Spinner to implement SpinnerWidget.
type gtkSpinner struct {
	inner *gtk.Spinner
}

func (s *gtkSpinner) Show()                         { s.inner.Show() }
func (s *gtkSpinner) Hide()                         { s.inner.Hide() }
func (s *gtkSpinner) SetVisible(visible bool)       { s.inner.SetVisible(visible) }
func (s *gtkSpinner) IsVisible() bool               { return s.inner.GetVisible() }
func (s *gtkSpinner) GrabFocus() bool               { return s.inner.GrabFocus() }
func (s *gtkSpinner) HasFocus() bool                { return s.inner.HasFocus() }
func (s *gtkSpinner) SetCanFocus(canFocus bool)     { s.inner.SetCanFocus(canFocus) }
func (s *gtkSpinner) SetFocusable(focusable bool)   { s.inner.SetFocusable(focusable) }
func (s *gtkSpinner) SetFocusOnClick(focus bool)    { s.inner.SetFocusOnClick(focus) }
func (s *gtkSpinner) SetCanTarget(canTarget bool)   { s.inner.SetCanTarget(canTarget) }
func (s *gtkSpinner) SetHexpand(expand bool)        { s.inner.SetHexpand(expand) }
func (s *gtkSpinner) SetVexpand(expand bool)        { s.inner.SetVexpand(expand) }
func (s *gtkSpinner) GetHexpand() bool              { return s.inner.GetHexpand() }
func (s *gtkSpinner) GetVexpand() bool              { return s.inner.GetVexpand() }
func (s *gtkSpinner) SetHalign(align gtk.Align)     { s.inner.SetHalign(align) }
func (s *gtkSpinner) SetValign(align gtk.Align)     { s.inner.SetValign(align) }
func (s *gtkSpinner) SetSizeRequest(w, h int)       { s.inner.SetSizeRequest(w, h) }
func (s *gtkSpinner) AddCssClass(class string)      { s.inner.AddCssClass(class) }
func (s *gtkSpinner) RemoveCssClass(class string)   { s.inner.RemoveCssClass(class) }
func (s *gtkSpinner) HasCssClass(class string) bool { return s.inner.HasCssClass(class) }
func (s *gtkSpinner) Unparent()                     { s.inner.Unparent() }
func (s *gtkSpinner) GtkWidget() *gtk.Widget        { return &s.inner.Widget }

func (s *gtkSpinner) GetParent() Widget {
	parent := s.inner.GetParent()
	if parent == nil {
		return nil
	}
	return &gtkWidget{inner: parent}
}

func (s *gtkSpinner) GetAllocatedWidth() int               { return s.inner.GetAllocatedWidth() }
func (s *gtkSpinner) GetAllocatedHeight() int              { return s.inner.GetAllocatedHeight() }
func (s *gtkSpinner) AddController(c *gtk.EventController) { s.inner.AddController(c) }

func (s *gtkSpinner) ComputePoint(target Widget) (x, y float64, ok bool) {
	srcPoint := &graphene.Point{X: 0, Y: 0}
	outPoint := &graphene.Point{}

	var targetGtk *gtk.Widget
	if target != nil {
		targetGtk = target.GtkWidget()
	}

	ok = s.inner.ComputePoint(targetGtk, srcPoint, outPoint)
	if !ok {
		return 0, 0, false
	}
	return float64(outPoint.X), float64(outPoint.Y), true
}

func (s *gtkSpinner) Start()                    { s.inner.Start() }
func (s *gtkSpinner) Stop()                     { s.inner.Stop() }
func (s *gtkSpinner) SetSpinning(spinning bool) { s.inner.SetSpinning(spinning) }
func (s *gtkSpinner) GetSpinning() bool         { return s.inner.GetSpinning() }

// GtkWidgetFactory creates real GTK widgets.
type GtkWidgetFactory struct{}

// NewGtkWidgetFactory creates a new factory for real GTK widgets.
func NewGtkWidgetFactory() *GtkWidgetFactory {
	return &GtkWidgetFactory{}
}

func (f *GtkWidgetFactory) NewPaned(orientation Orientation) PanedWidget {
	paned := gtk.NewPaned(orientation)
	paned.SetHexpand(true)
	paned.SetVexpand(true)
	return &gtkPaned{inner: paned}
}

func (f *GtkWidgetFactory) NewBox(orientation Orientation, spacing int) BoxWidget {
	box := gtk.NewBox(orientation, spacing)
	box.SetHexpand(true)
	box.SetVexpand(true)
	return &gtkBox{inner: box}
}

func (f *GtkWidgetFactory) NewOverlay() OverlayWidget {
	overlay := gtk.NewOverlay()
	overlay.SetHexpand(true)
	overlay.SetVexpand(true)
	return &gtkOverlay{inner: overlay}
}

func (f *GtkWidgetFactory) NewLabel(text string) LabelWidget {
	label := gtk.NewLabel(&text)
	return &gtkLabel{inner: label}
}

func (f *GtkWidgetFactory) NewButton() ButtonWidget {
	button := gtk.NewButton()
	return &gtkButton{inner: button}
}

func (f *GtkWidgetFactory) NewImage() ImageWidget {
	image := gtk.NewImage()
	return &gtkImage{inner: image}
}

func (f *GtkWidgetFactory) NewProgressBar() ProgressBarWidget {
	progressBar := gtk.NewProgressBar()
	return &gtkProgressBar{inner: progressBar}
}

func (f *GtkWidgetFactory) NewSpinner() SpinnerWidget {
	spinner := gtk.NewSpinner()
	return &gtkSpinner{inner: spinner}
}

func (f *GtkWidgetFactory) WrapWidget(w *gtk.Widget) Widget {
	if w == nil {
		return nil
	}
	return &gtkWidget{inner: w}
}
