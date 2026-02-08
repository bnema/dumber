package component

import (
	"context"
	"sync"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// buttonSpacing is the spacing between buttons in the permission popup.
const buttonSpacing = 6

// PermissionPopup is a custom overlay component for permission prompts.
// It replaces the Adwaita AlertDialog to sidestep the purego ConnectResponse bug
// and match the app's custom UI style.
type PermissionPopup struct {
	outerBox *gtk.Box
	mainBox  *gtk.Box

	headingLabel *gtk.Label
	bodyLabel    *gtk.Label

	btnAllow       *gtk.Button
	btnAlwaysAllow *gtk.Button
	btnDeny        *gtk.Button
	btnAlwaysDeny  *gtk.Button

	parentOverlay layout.OverlayWidget
	uiScale       float64

	mu       sync.Mutex
	visible  bool
	callback func(allowed, persistent bool)

	retainedCallbacks []interface{}
}

// NewPermissionPopup creates a new permission popup component.
func NewPermissionPopup(parentOverlay layout.OverlayWidget, uiScale float64) *PermissionPopup {
	if uiScale <= 0 {
		uiScale = 1.0
	}

	pp := &PermissionPopup{
		parentOverlay: parentOverlay,
		uiScale:       uiScale,
	}

	if err := pp.createWidgets(); err != nil {
		return nil
	}
	pp.attachKeyController()
	return pp
}

// Widget returns the outer GTK widget for overlay registration.
func (pp *PermissionPopup) Widget() *gtk.Widget {
	if pp.outerBox == nil {
		return nil
	}
	return &pp.outerBox.Widget
}

// Show displays the permission popup with the given heading and body text.
// The callback receives (allowed, persistent) when the user makes a choice.
func (pp *PermissionPopup) Show(ctx context.Context, heading, body string, callback func(allowed, persistent bool)) {
	log := logging.FromContext(ctx)

	pp.mu.Lock()
	if pp.visible {
		pp.mu.Unlock()
		log.Warn().Msg("permission popup already visible, ignoring Show")
		return
	}
	pp.visible = true
	pp.callback = callback
	pp.mu.Unlock()

	if pp.headingLabel != nil {
		pp.headingLabel.SetText(heading)
	}
	if pp.bodyLabel != nil {
		pp.bodyLabel.SetText(body)
	}

	pp.resizeAndCenter()
	if pp.outerBox != nil {
		pp.outerBox.SetVisible(true)
	}
	// Focus the Deny button as the conservative default
	if pp.btnDeny != nil {
		pp.btnDeny.GrabFocus()
	}
}

// Hide hides the popup without invoking the callback.
func (pp *PermissionPopup) Hide() {
	pp.mu.Lock()
	if !pp.visible {
		pp.mu.Unlock()
		return
	}
	pp.visible = false
	pp.callback = nil
	pp.mu.Unlock()

	if pp.outerBox != nil {
		pp.outerBox.SetVisible(false)
	}
}

// IsVisible returns whether the popup is currently displayed.
func (pp *PermissionPopup) IsVisible() bool {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	return pp.visible
}

func (pp *PermissionPopup) dismiss(allowed, persistent bool) {
	pp.mu.Lock()
	if !pp.visible {
		pp.mu.Unlock()
		return
	}
	pp.visible = false
	cb := pp.callback
	pp.callback = nil
	pp.mu.Unlock()

	if pp.outerBox != nil {
		pp.outerBox.SetVisible(false)
	}
	if cb != nil {
		cb(allowed, persistent)
	}
}

// setupContainers creates and configures the outer and main boxes.
func (pp *PermissionPopup) setupContainers() error {
	pp.outerBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if pp.outerBox == nil {
		return errNilWidget("permissionPopupOuterBox")
	}
	pp.outerBox.AddCssClass("permission-popup-outer")
	pp.outerBox.SetHalign(gtk.AlignCenterValue)
	pp.outerBox.SetValign(gtk.AlignStartValue)
	pp.outerBox.SetVisible(false)

	pp.mainBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if pp.mainBox == nil {
		return errNilWidget("permissionPopupMainBox")
	}
	pp.mainBox.AddCssClass("permission-popup-container")
	return nil
}

// setupLabels creates and configures the heading and body labels.
func (pp *PermissionPopup) setupLabels() error {
	emptyText := ""
	pp.headingLabel = gtk.NewLabel(&emptyText)
	if pp.headingLabel == nil {
		return errNilWidget("permissionPopupHeadingLabel")
	}
	pp.headingLabel.AddCssClass("permission-popup-heading")
	pp.headingLabel.SetHalign(gtk.AlignStartValue)

	pp.bodyLabel = gtk.NewLabel(&emptyText)
	if pp.bodyLabel == nil {
		return errNilWidget("permissionPopupBodyLabel")
	}
	pp.bodyLabel.AddCssClass("permission-popup-body")
	pp.bodyLabel.SetHalign(gtk.AlignStartValue)
	pp.bodyLabel.SetWrap(true)
	return nil
}

func (pp *PermissionPopup) createWidgets() error {
	if err := pp.setupContainers(); err != nil {
		return err
	}

	if err := pp.setupLabels(); err != nil {
		return err
	}

	// Button row
	btnRow := gtk.NewBox(gtk.OrientationHorizontalValue, buttonSpacing)
	if btnRow == nil {
		return errNilWidget("permissionPopupBtnRow")
	}
	btnRow.AddCssClass("permission-popup-btn-row")
	btnRow.SetHalign(gtk.AlignEndValue)

	// Create buttons: Always Deny | Deny | Allow | Always Allow
	var err error
	pp.btnAlwaysDeny, err = pp.createPermissionButton("Always Deny", []string{"permission-popup-btn", "permission-popup-btn-destructive"})
	if err != nil {
		return err
	}

	pp.btnDeny, err = pp.createPermissionButton("Deny", []string{"permission-popup-btn", "permission-popup-btn-deny"})
	if err != nil {
		return err
	}

	pp.btnAllow, err = pp.createPermissionButton("Allow", []string{"permission-popup-btn", "permission-popup-btn-allow"})
	if err != nil {
		return err
	}

	pp.btnAlwaysAllow, err = pp.createPermissionButton("Always Allow", []string{"permission-popup-btn", "permission-popup-btn-allow"})
	if err != nil {
		return err
	}

	// Wire button clicks
	pp.wireButton(pp.btnAlwaysDeny, false, true)
	pp.wireButton(pp.btnDeny, false, false)
	pp.wireButton(pp.btnAllow, true, false)
	pp.wireButton(pp.btnAlwaysAllow, true, true)

	// Assemble button row
	btnRow.Append(&pp.btnAlwaysDeny.Widget)
	btnRow.Append(&pp.btnDeny.Widget)
	btnRow.Append(&pp.btnAllow.Widget)
	btnRow.Append(&pp.btnAlwaysAllow.Widget)

	// Assemble main box
	pp.mainBox.Append(&pp.headingLabel.Widget)
	pp.mainBox.Append(&pp.bodyLabel.Widget)
	pp.mainBox.Append(&btnRow.Widget)

	// Assemble outer box
	pp.outerBox.Append(&pp.mainBox.Widget)

	return nil
}

// createPermissionButton creates a permission button with the given label and CSS classes.
func (pp *PermissionPopup) createPermissionButton(label string, cssClasses []string) (*gtk.Button, error) {
	btn := gtk.NewButtonWithLabel(label)
	if btn == nil {
		return nil, errNilWidget("permissionPopupBtn" + label)
	}
	for _, class := range cssClasses {
		btn.AddCssClass(class)
	}
	return btn, nil
}

// wireButton connects a button click to the dismiss callback.
func (pp *PermissionPopup) wireButton(btn *gtk.Button, allowed, persistent bool) {
	cb := func(_ gtk.Button) { pp.dismiss(allowed, persistent) }
	pp.retainedCallbacks = append(pp.retainedCallbacks, cb)
	btn.ConnectClicked(&cb)
}

func (pp *PermissionPopup) attachKeyController() {
	if pp.outerBox == nil {
		return
	}
	controller := gtk.NewEventControllerKey()
	if controller == nil {
		return
	}
	controller.SetPropagationPhase(gtk.PhaseCaptureValue)

	keyPressedCb := func(_ gtk.EventControllerKey, keyval uint, _ uint, _ gdk.ModifierType) bool {
		if keyval == uint(gdk.KEY_Escape) {
			// Escape = deny (conservative default)
			pp.dismiss(false, false)
			return true
		}
		return false
	}
	pp.retainedCallbacks = append(pp.retainedCallbacks, keyPressedCb)
	controller.ConnectKeyPressed(&keyPressedCb)
	pp.outerBox.AddController(&controller.EventController)
}

func (pp *PermissionPopup) resizeAndCenter() {
	if pp.outerBox == nil || pp.mainBox == nil {
		return
	}

	width, marginTop := CalculateModalDimensions(pp.parentOverlay, PermissionPopupSizeDefaults)
	pp.mainBox.SetSizeRequest(width, -1)
	pp.outerBox.SetMarginTop(marginTop)
}
