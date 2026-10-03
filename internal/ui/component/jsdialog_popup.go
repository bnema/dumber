package component

import (
	"net/url"
	"strings"
	"sync"

	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/bnema/puregotk/v4/pango"

	"github.com/bnema/dumber/internal/application/port"
)

const (
	jsDialogMaxBodyHeight = 240
	jsDialogMaxWidthChars = 60
	jsDialogMaxHostChars  = 80
	jsDialogDialogWidth   = 420
)

// jsDialogContent is the plain-text content of a JS dialog.
type jsDialogContent struct {
	Heading     string
	Body        string
	OKLabel     string
	CancelLabel string // empty: no cancel button (alert)
	ShowInput   bool
}

// jsDialogOriginLabel returns a short, user-facing origin ("example.com") for
// the requesting URL. The origin is shown separately from the page-controlled
// message so a page cannot impersonate browser UI.
func jsDialogOriginLabel(origin string) string {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return "This page"
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		if err == nil && u.Scheme != "" {
			return truncateRunes(u.Scheme+":", jsDialogMaxHostChars)
		}
		return "This page"
	}
	return truncateRunes(u.Host, jsDialogMaxHostChars)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// buildJSDialogContent maps a request to the dialog texts and buttons.
// All strings are plain text (never Pango markup).
func buildJSDialogContent(req port.JSDialogRequest) jsDialogContent {
	says := jsDialogOriginLabel(req.Origin) + " says:"
	switch req.Type {
	case port.JSDialogConfirm:
		return jsDialogContent{Heading: says, Body: req.Message, OKLabel: "OK", CancelLabel: "Cancel"}
	case port.JSDialogPrompt:
		return jsDialogContent{Heading: says, Body: req.Message, OKLabel: "OK", CancelLabel: "Cancel", ShowInput: true}
	case port.JSDialogBeforeUnload:
		body := strings.TrimSpace(req.Message)
		if body == "" {
			body = "Changes you made may not be saved."
		}
		heading := "Leave page?"
		if req.IsReload {
			heading = "Reload page?"
		}
		return jsDialogContent{
			Heading:     heading,
			Body:        jsDialogOriginLabel(req.Origin) + "\n" + body,
			OKLabel:     "Leave",
			CancelLabel: "Stay",
		}
	default:
		return jsDialogContent{Heading: says, Body: req.Message, OKLabel: "OK"}
	}
}

// JSDialogPopup is a per-pane overlay that renders alert/confirm/prompt and
// beforeunload dialogs. It is owned by a PaneView, so a dialog belongs to its
// pane's WebView and never blocks other panes. It replaces Adwaita AlertDialog
// for the same reason as PermissionPopup (purego ConnectResponse bug).
type JSDialogPopup struct {
	outerBox *gtk.Box
	mainBox  *gtk.Box

	headingLabel *gtk.Label
	bodyLabel    *gtk.Label
	input        *gtk.Entry
	btnOK        *gtk.Button
	btnCancel    *gtk.Button

	mu       sync.Mutex
	visible  bool
	respond  func(ok bool, input string)
	onHidden func()

	retainedCallbacks []any
}

// NewJSDialogPopup creates the popup (hidden). Returns nil if widget creation fails.
func NewJSDialogPopup() *JSDialogPopup {
	p := &JSDialogPopup{}
	if err := p.createWidgets(); err != nil {
		return nil
	}
	p.attachKeyController()
	return p
}

// Widget returns the outer GTK widget for overlay registration.
func (p *JSDialogPopup) Widget() *gtk.Widget {
	if p == nil || p.outerBox == nil {
		return nil
	}
	return &p.outerBox.Widget
}

// SetOnHidden sets a callback run after the popup closes (e.g. to restore focus).
func (p *JSDialogPopup) SetOnHidden(fn func()) {
	p.mu.Lock()
	p.onHidden = fn
	p.mu.Unlock()
}

// Show displays the dialog. respond is invoked once with the user's answer.
// A dialog already visible is dismissed as canceled first.
func (p *JSDialogPopup) Show(req port.JSDialogRequest, respond func(ok bool, input string)) {
	p.dismiss(false)

	content := buildJSDialogContent(req)

	p.mu.Lock()
	p.visible = true
	p.respond = respond
	p.mu.Unlock()

	p.headingLabel.SetText(content.Heading)
	p.bodyLabel.SetText(content.Body)
	p.bodyLabel.SetVisible(content.Body != "")
	p.btnOK.SetLabel(content.OKLabel)
	p.btnCancel.SetVisible(content.CancelLabel != "")
	if content.CancelLabel != "" {
		p.btnCancel.SetLabel(content.CancelLabel)
	}
	p.input.SetVisible(content.ShowInput)
	if content.ShowInput {
		p.input.SetText(req.DefaultPrompt)
	} else {
		p.input.SetText("")
	}

	p.outerBox.SetVisible(true)
	if content.ShowInput {
		p.input.GrabFocus()
		p.input.SelectRegion(0, -1)
	} else {
		p.btnOK.GrabFocus()
	}
}

// Hide closes the popup without invoking the response callback. It is used when
// the engine canceled the dialog (navigation, WebView destruction).
func (p *JSDialogPopup) Hide() {
	if p == nil {
		return
	}
	p.mu.Lock()
	wasVisible := p.visible
	p.visible = false
	p.respond = nil
	hidden := p.onHidden
	p.mu.Unlock()
	if !wasVisible {
		return
	}
	p.outerBox.SetVisible(false)
	if hidden != nil {
		hidden()
	}
}

// IsVisible reports whether a dialog is displayed.
func (p *JSDialogPopup) IsVisible() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.visible
}

// dismiss closes the popup and answers. Safe to call when hidden (no-op).
func (p *JSDialogPopup) dismiss(ok bool) {
	p.mu.Lock()
	if !p.visible {
		p.mu.Unlock()
		return
	}
	p.visible = false
	respond := p.respond
	p.respond = nil
	hidden := p.onHidden
	p.mu.Unlock()

	text := ""
	if ok && p.input.GetVisible() {
		text = p.input.GetText()
	}
	p.outerBox.SetVisible(false)
	if respond != nil {
		respond(ok, text)
	}
	if hidden != nil {
		hidden()
	}
}

func (p *JSDialogPopup) createWidgets() error {
	if err := p.createContainers(); err != nil {
		return err
	}
	if err := p.createLabels(); err != nil {
		return err
	}
	scroll := gtk.NewScrolledWindow()
	if scroll == nil {
		return errNilWidget("jsDialogScroll")
	}
	scroll.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	scroll.SetPropagateNaturalHeight(true)
	scroll.SetMaxContentHeight(jsDialogMaxBodyHeight)
	scroll.SetChild(&p.bodyLabel.Widget)

	p.input = gtk.NewEntry()
	if p.input == nil {
		return errNilWidget("jsDialogEntry")
	}
	p.input.AddCssClass("jsdialog-entry")
	p.input.SetVisible(false)

	btnRow, err := p.createButtonRow()
	if err != nil {
		return err
	}

	p.mainBox.Append(&p.headingLabel.Widget)
	p.mainBox.Append(&scroll.Widget)
	p.mainBox.Append(&p.input.Widget)
	p.mainBox.Append(&btnRow.Widget)
	p.outerBox.Append(&p.mainBox.Widget)
	return nil
}

func (p *JSDialogPopup) createContainers() error {
	p.outerBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if p.outerBox == nil {
		return errNilWidget("jsDialogOuterBox")
	}
	// Fill the pane so the scrim swallows pointer input meant for the page.
	p.outerBox.AddCssClass("jsdialog-scrim")
	p.outerBox.SetHexpand(true)
	p.outerBox.SetVexpand(true)
	p.outerBox.SetHalign(gtk.AlignFillValue)
	p.outerBox.SetValign(gtk.AlignFillValue)
	p.outerBox.SetVisible(false)

	p.mainBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if p.mainBox == nil {
		return errNilWidget("jsDialogMainBox")
	}
	p.mainBox.AddCssClass("permission-popup-container")
	p.mainBox.SetHalign(gtk.AlignCenterValue)
	p.mainBox.SetValign(gtk.AlignCenterValue)
	p.mainBox.SetSizeRequest(jsDialogDialogWidth, -1)
	return nil
}

func (p *JSDialogPopup) createLabels() error {
	empty := ""
	p.headingLabel = gtk.NewLabel(&empty)
	p.bodyLabel = gtk.NewLabel(&empty)
	if p.headingLabel == nil || p.bodyLabel == nil {
		return errNilWidget("jsDialogLabels")
	}
	// Labels use SetText only: page text is never parsed as Pango markup.
	for _, l := range []*gtk.Label{p.headingLabel, p.bodyLabel} {
		l.SetUseMarkup(false)
		l.SetHalign(gtk.AlignStartValue)
		l.SetXalign(0)
		l.SetWrap(true)
		l.SetWrapMode(pango.WrapWordCharValue)
		l.SetMaxWidthChars(jsDialogMaxWidthChars)
	}
	p.headingLabel.AddCssClass("permission-popup-heading")
	p.bodyLabel.AddCssClass("permission-popup-body")
	return nil
}

func (p *JSDialogPopup) createButtonRow() (*gtk.Box, error) {
	btnRow := gtk.NewBox(gtk.OrientationHorizontalValue, buttonSpacing)
	if btnRow == nil {
		return nil, errNilWidget("jsDialogBtnRow")
	}
	btnRow.AddCssClass("permission-popup-btn-row")
	btnRow.SetHalign(gtk.AlignEndValue)

	p.btnCancel = gtk.NewButtonWithLabel("Cancel")
	p.btnOK = gtk.NewButtonWithLabel("OK")
	if p.btnCancel == nil || p.btnOK == nil {
		return nil, errNilWidget("jsDialogButtons")
	}
	p.btnCancel.AddCssClass("permission-popup-btn")
	p.btnCancel.AddCssClass("permission-popup-btn-deny")
	p.btnOK.AddCssClass("permission-popup-btn")
	p.btnOK.AddCssClass("permission-popup-btn-allow")

	cancelCb := func(_ gtk.Button) { p.dismiss(false) }
	okCb := func(_ gtk.Button) { p.dismiss(true) }
	p.retainedCallbacks = append(p.retainedCallbacks, cancelCb, okCb)
	p.btnCancel.ConnectClicked(&cancelCb)
	p.btnOK.ConnectClicked(&okCb)

	btnRow.Append(&p.btnCancel.Widget)
	btnRow.Append(&p.btnOK.Widget)
	return btnRow, nil
}

// attachKeyController maps Enter to OK and Escape to Cancel.
func (p *JSDialogPopup) attachKeyController() {
	controller := gtk.NewEventControllerKey()
	if controller == nil {
		return
	}
	controller.SetPropagationPhase(gtk.PhaseCaptureValue)
	keyPressedCb := func(_ gtk.EventControllerKey, keyval uint, _ uint, _ gdk.ModifierType) bool {
		switch keyval {
		case uint(gdk.KEY_Escape):
			p.dismiss(false)
			return true
		case uint(gdk.KEY_Return), uint(gdk.KEY_KP_Enter):
			p.dismiss(true)
			return true
		}
		return false
	}
	p.retainedCallbacks = append(p.retainedCallbacks, keyPressedCb)
	controller.ConnectKeyPressed(&keyPressedCb)
	p.outerBox.AddController(&controller.EventController)
}
