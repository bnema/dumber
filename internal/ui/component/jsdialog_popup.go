package component

import (
	"net/url"
	"strings"
	"sync"

	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/bnema/puregotk/v4/pango"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/ui/layout"
)

const (
	jsDialogMaxBodyHeight = 240
	jsDialogMaxWidthChars = 60
	jsDialogMaxHostChars  = 80
	// jsDialogMaxTextRunes caps page-controlled text (message, prompt default)
	// so a hostile page can't make GTK lay out megabytes of text.
	jsDialogMaxTextRunes = 3000
	// jsDialogBeforeUnloadText is Dumber's own beforeunload text: CEF always
	// passes a fixed string, and browsers no longer show page text here.
	jsDialogBeforeUnloadText = "Changes you made may not be saved."
)

// JSDialogSizeDefaults sizes the dialog box relative to its pane.
var JSDialogSizeDefaults = ModalSizeConfig{
	WidthPct:       0.9,
	MaxWidth:       420,
	TopMarginPct:   0.3,
	FallbackWidth:  420,
	FallbackHeight: 300,
}

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
			return truncateRunesLeft(u.Scheme+":", jsDialogMaxHostChars)
		}
		return "This page"
	}
	// Keep the end of the host: the registrable domain is what identifies it.
	return truncateRunesLeft(u.Host, jsDialogMaxHostChars)
}

// truncateRunesLeft keeps the last n runes, prefixing "…" when it cut.
func truncateRunesLeft(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return "…" + string(r[len(r)-n:])
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
	msg := truncateRunes(req.Message, jsDialogMaxTextRunes)
	switch req.Type {
	case port.JSDialogConfirm:
		return jsDialogContent{Heading: says, Body: msg, OKLabel: "OK", CancelLabel: "Cancel"}
	case port.JSDialogPrompt:
		return jsDialogContent{Heading: says, Body: msg, OKLabel: "OK", CancelLabel: "Cancel", ShowInput: true}
	case port.JSDialogBeforeUnload:
		body := jsDialogBeforeUnloadText
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
		return jsDialogContent{Heading: says, Body: msg, OKLabel: "OK"}
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

	parent     layout.OverlayWidget
	controller *gtk.EventControllerKey
	signals    []popupSignal

	mu        sync.Mutex
	visible   bool
	wantFocus bool // this dialog should own keyboard focus (pane is active)
	destroyed bool
	respond   func(ok bool, input string)
	onHidden  func(hadFocus bool)

	retainedCallbacks []any
}

// popupSignal remembers a connected handler so Destroy can disconnect it.
type popupSignal struct {
	obj interface{ DisconnectSignal(uint) }
	id  uint
}

// NewJSDialogPopup creates the popup (hidden). Returns nil if widget creation fails.
// parent is the pane overlay used to size the dialog.
func NewJSDialogPopup(parent layout.OverlayWidget) *JSDialogPopup {
	p := &JSDialogPopup{parent: parent}
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

// SetOnHidden sets (or with nil clears) a callback run after the popup closes. hadFocus reports
// whether keyboard focus was inside the dialog when it closed, so the owner
// only restores focus when the dialog had taken it.
func (p *JSDialogPopup) SetOnHidden(fn func(hadFocus bool)) {
	p.mu.Lock()
	p.onHidden = fn
	p.mu.Unlock()
}

// Show displays the dialog. respond is invoked once with the user's answer.
// A dialog already visible is dismissed as canceled first. Keyboard focus is
// only taken when focus is true (the pane is active); otherwise RequestFocus
// grabs it when the pane becomes active.
func (p *JSDialogPopup) Show(req port.JSDialogRequest, respond func(ok bool, input string), focus bool, uiScale float64) {
	p.Dismiss(false)

	content := buildJSDialogContent(req)

	p.mu.Lock()
	if p.destroyed {
		p.mu.Unlock()
		if respond != nil {
			respond(false, "")
		}
		return
	}
	p.visible = true
	p.wantFocus = focus
	p.respond = respond
	p.mu.Unlock()

	width, _ := CalculateModalDimensionsWithScale(p.parent, JSDialogSizeDefaults, uiScale)
	p.mainBox.SetSizeRequest(width, -1)

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
		p.input.SetText(truncateRunes(req.DefaultPrompt, jsDialogMaxTextRunes))
	} else {
		p.input.SetText("")
	}

	p.outerBox.SetVisible(true)
	if focus {
		p.grabDefaultFocus()
	}
}

// RequestFocus marks the dialog as the focus owner and grabs focus if shown.
// Called when the owning pane becomes active.
func (p *JSDialogPopup) RequestFocus() {
	p.mu.Lock()
	if !p.visible || p.destroyed {
		p.mu.Unlock()
		return
	}
	p.wantFocus = true
	p.mu.Unlock()
	p.grabDefaultFocus()
}

// grabDefaultFocus focuses the entry (prompt) or the OK button.
func (p *JSDialogPopup) grabDefaultFocus() {
	if p.input.GetVisible() {
		p.input.GrabFocus()
		p.input.SelectRegion(0, -1)
		return
	}
	p.btnOK.GrabFocus()
}

// Hide closes the popup without invoking the response callback. It is used when
// the engine canceled the dialog (navigation, WebView destruction).
func (p *JSDialogPopup) Hide() {
	if p == nil {
		return
	}
	p.close(nil)
}

// Dismiss closes the popup and answers with ok. No-op when hidden.
func (p *JSDialogPopup) Dismiss(ok bool) {
	p.close(&ok)
}

// close hides the popup. When answer is non-nil the response callback is run.
func (p *JSDialogPopup) close(answer *bool) {
	p.mu.Lock()
	if !p.visible {
		p.mu.Unlock()
		return
	}
	p.visible = false
	p.wantFocus = false
	respond := p.respond
	p.respond = nil
	hidden := p.onHidden
	p.mu.Unlock()

	hadFocus := p.outerBox.GetFocusChild() != nil
	text := ""
	if answer != nil && *answer && p.input.GetVisible() {
		text = p.input.GetText()
	}
	p.outerBox.SetVisible(false)
	if answer != nil && respond != nil {
		respond(*answer, text)
	}
	if hidden != nil {
		hidden(hadFocus)
	}
}

// Destroy disconnects every handler and drops callbacks. The popup must not be
// used afterwards. Call after the dialog was dismissed.
func (p *JSDialogPopup) Destroy() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.destroyed {
		p.mu.Unlock()
		return
	}
	p.destroyed = true
	p.visible = false
	p.respond = nil
	p.onHidden = nil
	p.mu.Unlock()

	for _, s := range p.signals {
		s.obj.DisconnectSignal(s.id)
	}
	p.signals = nil
	if p.controller != nil && p.outerBox != nil {
		p.outerBox.RemoveController(&p.controller.EventController)
		p.controller = nil
	}
	p.retainedCallbacks = nil
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
	p.input.SetMaxLength(jsDialogMaxTextRunes)
	p.input.SetVisible(false)
	// Enter in the entry = OK. Focused buttons activate themselves, so Enter on
	// Cancel/Stay still cancels.
	activateCb := func(gtk.Entry) { p.Dismiss(true) }
	p.retainedCallbacks = append(p.retainedCallbacks, activateCb)
	p.signals = append(p.signals, popupSignal{p.input, p.input.ConnectActivate(&activateCb)})

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

	cancelCb := func(_ gtk.Button) { p.Dismiss(false) }
	okCb := func(_ gtk.Button) { p.Dismiss(true) }
	p.retainedCallbacks = append(p.retainedCallbacks, cancelCb, okCb)
	p.signals = append(p.signals,
		popupSignal{p.btnCancel, p.btnCancel.ConnectClicked(&cancelCb)},
		popupSignal{p.btnOK, p.btnOK.ConnectClicked(&okCb)},
	)

	btnRow.Append(&p.btnCancel.Widget)
	btnRow.Append(&p.btnOK.Widget)
	return btnRow, nil
}

// attachKeyController maps Escape to Cancel (capture phase, so the page never
// sees it). Enter is not handled here: see the entry's activate handler.
func (p *JSDialogPopup) attachKeyController() {
	controller := gtk.NewEventControllerKey()
	if controller == nil {
		return
	}
	controller.SetPropagationPhase(gtk.PhaseCaptureValue)
	keyPressedCb := func(_ gtk.EventControllerKey, keyval uint, _ uint, _ gdk.ModifierType) bool {
		if keyval == uint(gdk.KEY_Escape) {
			p.Dismiss(false)
			return true
		}
		return false
	}
	p.retainedCallbacks = append(p.retainedCallbacks, keyPressedCb)
	p.signals = append(p.signals, popupSignal{controller, controller.ConnectKeyPressed(&keyPressedCb)})
	p.outerBox.AddController(&controller.EventController)
	p.controller = controller

	// A dialog shown while its tab was in the background gets focus once mapped.
	mapCb := func(gtk.Widget) {
		p.mu.Lock()
		want := p.visible && p.wantFocus
		p.mu.Unlock()
		if want {
			p.grabDefaultFocus()
		}
	}
	p.retainedCallbacks = append(p.retainedCallbacks, mapCb)
	p.signals = append(p.signals, popupSignal{&p.outerBox.Widget, p.outerBox.ConnectMap(&mapCb)})
}
