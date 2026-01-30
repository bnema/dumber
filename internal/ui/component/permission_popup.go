package component

import (
	"context"
	"sync"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

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

func (pp *PermissionPopup) createWidgets() error {
	// Outer box - positioned in overlay, hidden by default
	pp.outerBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if pp.outerBox == nil {
		return errNilWidget("permissionPopupOuterBox")
	}
	pp.outerBox.AddCssClass("permission-popup-outer")
	pp.outerBox.SetHalign(gtk.AlignCenterValue)
	pp.outerBox.SetValign(gtk.AlignStartValue)
	pp.outerBox.SetVisible(false)

	// Main box - styled container
	pp.mainBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if pp.mainBox == nil {
		return errNilWidget("permissionPopupMainBox")
	}
	pp.mainBox.AddCssClass("permission-popup-container")

	// Heading label
	emptyText := ""
	pp.headingLabel = gtk.NewLabel(&emptyText)
	if pp.headingLabel == nil {
		return errNilWidget("permissionPopupHeadingLabel")
	}
	pp.headingLabel.AddCssClass("permission-popup-heading")
	pp.headingLabel.SetHalign(gtk.AlignStartValue)

	// Body label
	pp.bodyLabel = gtk.NewLabel(&emptyText)
	if pp.bodyLabel == nil {
		return errNilWidget("permissionPopupBodyLabel")
	}
	pp.bodyLabel.AddCssClass("permission-popup-body")
	pp.bodyLabel.SetHalign(gtk.AlignStartValue)
	pp.bodyLabel.SetWrap(true)

	// Button row
	btnRow := gtk.NewBox(gtk.OrientationHorizontalValue, 6)
	if btnRow == nil {
		return errNilWidget("permissionPopupBtnRow")
	}
	btnRow.AddCssClass("permission-popup-btn-row")
	btnRow.SetHalign(gtk.AlignEndValue)

	// Create buttons: Always Deny | Deny | Allow | Always Allow
	pp.btnAlwaysDeny = gtk.NewButtonWithLabel("Always Deny")
	if pp.btnAlwaysDeny == nil {
		return errNilWidget("permissionPopupBtnAlwaysDeny")
	}
	pp.btnAlwaysDeny.AddCssClass("permission-popup-btn")
	pp.btnAlwaysDeny.AddCssClass("permission-popup-btn-destructive")

	pp.btnDeny = gtk.NewButtonWithLabel("Deny")
	if pp.btnDeny == nil {
		return errNilWidget("permissionPopupBtnDeny")
	}
	pp.btnDeny.AddCssClass("permission-popup-btn")
	pp.btnDeny.AddCssClass("permission-popup-btn-deny")

	pp.btnAllow = gtk.NewButtonWithLabel("Allow")
	if pp.btnAllow == nil {
		return errNilWidget("permissionPopupBtnAllow")
	}
	pp.btnAllow.AddCssClass("permission-popup-btn")
	pp.btnAllow.AddCssClass("permission-popup-btn-allow")

	pp.btnAlwaysAllow = gtk.NewButtonWithLabel("Always Allow")
	if pp.btnAlwaysAllow == nil {
		return errNilWidget("permissionPopupBtnAlwaysAllow")
	}
	pp.btnAlwaysAllow.AddCssClass("permission-popup-btn")
	pp.btnAlwaysAllow.AddCssClass("permission-popup-btn-allow")

	// Wire button clicks
	alwaysDenyCb := func(_ gtk.Button) { pp.dismiss(false, true) }
	pp.retainedCallbacks = append(pp.retainedCallbacks, alwaysDenyCb)
	pp.btnAlwaysDeny.ConnectClicked(&alwaysDenyCb)

	denyCb := func(_ gtk.Button) { pp.dismiss(false, false) }
	pp.retainedCallbacks = append(pp.retainedCallbacks, denyCb)
	pp.btnDeny.ConnectClicked(&denyCb)

	allowCb := func(_ gtk.Button) { pp.dismiss(true, false) }
	pp.retainedCallbacks = append(pp.retainedCallbacks, allowCb)
	pp.btnAllow.ConnectClicked(&allowCb)

	alwaysAllowCb := func(_ gtk.Button) { pp.dismiss(true, true) }
	pp.retainedCallbacks = append(pp.retainedCallbacks, alwaysAllowCb)
	pp.btnAlwaysAllow.ConnectClicked(&alwaysAllowCb)

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
