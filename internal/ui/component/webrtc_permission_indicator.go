package component

import (
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// revealerTransitionDuration is the animation duration in milliseconds
// for the details panel slide-down.
const revealerTransitionDuration = 180

var trackedWebRTCPermissionTypes = []entity.PermissionType{
	entity.PermissionTypeMicrophone,
	entity.PermissionTypeCamera,
	entity.PermissionTypeDisplay,
}

// WebRTCPermissionIndicator is a discrete top-right overlay that shows
// permission activity for microphone, camera, and display capture.
//
// Collapsed: a small colored dot.
// Hovered: expands left and downward to show per-device states.
// Uses GtkRevealer for smooth slide-down animation of the details panel.
type WebRTCPermissionIndicator struct {
	outerBox    *gtk.Box
	cardBox     *gtk.Box
	dotBox      *gtk.Box
	originLabel *gtk.Label
	detailsBox  *gtk.Box
	revealer    *gtk.Revealer

	rowButtons map[entity.PermissionType]*gtk.Button
	states     map[entity.PermissionType]webrtcPermissionState
	stored     map[entity.PermissionType]entity.PermissionDecision

	currentOrigin string
	onToggleLock  func(origin string, permType entity.PermissionType, state string, hasStored bool)

	mu                sync.Mutex
	expanded          bool
	retainedCallbacks []interface{}
}

// NewWebRTCPermissionIndicator creates a top-right WebRTC activity indicator.
func NewWebRTCPermissionIndicator() *WebRTCPermissionIndicator {
	indicator := buildWebRTCPermissionIndicatorWidgets()
	if indicator == nil {
		return nil
	}

	indicator.wireRowButtons()
	indicator.attachHoverController()
	indicator.refreshLocked()

	return indicator
}

// buildWebRTCPermissionIndicatorWidgets creates and assembles all GTK widgets
// for the indicator: outer container, card, dot, revealer, details, and rows.
func buildWebRTCPermissionIndicatorWidgets() *WebRTCPermissionIndicator {
	outer, card := createIndicatorContainers()
	if outer == nil || card == nil {
		return nil
	}

	dotRow, dot, originLabel := createIndicatorDot()
	if dotRow == nil || dot == nil || originLabel == nil {
		return nil
	}

	details := createIndicatorDetails()
	if details == nil {
		return nil
	}

	rowButtons, ok := createWebRTCPermissionRows(details)
	if !ok {
		return nil
	}

	revealer := createIndicatorRevealer(details)
	if revealer == nil {
		return nil
	}

	card.Append(&dotRow.Widget)
	card.Append(&revealer.Widget)
	outer.Append(&card.Widget)

	return &WebRTCPermissionIndicator{
		outerBox:    outer,
		cardBox:     card,
		dotBox:      dot,
		originLabel: originLabel,
		detailsBox:  details,
		revealer:    revealer,
		rowButtons:  rowButtons,
		states:      make(map[entity.PermissionType]webrtcPermissionState, len(trackedWebRTCPermissionTypes)),
		stored:      make(map[entity.PermissionType]entity.PermissionDecision, len(trackedWebRTCPermissionTypes)),
	}
}

func createIndicatorContainers() (*gtk.Box, *gtk.Box) {
	outer := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if outer == nil {
		return nil, nil
	}
	outer.AddCssClass("webrtc-indicator-outer")
	outer.SetHalign(gtk.AlignEndValue)
	outer.SetValign(gtk.AlignStartValue)
	outer.SetHexpand(true)
	outer.SetVexpand(true)
	outer.SetVisible(false)

	card := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if card == nil {
		return nil, nil
	}
	card.AddCssClass("webrtc-indicator-card")

	return outer, card
}

func createIndicatorDot() (*gtk.Box, *gtk.Box, *gtk.Label) {
	dotRow := gtk.NewBox(gtk.OrientationHorizontalValue, 0)
	if dotRow == nil {
		return nil, nil, nil
	}
	dotRow.AddCssClass("webrtc-indicator-header")
	dotRow.SetHalign(gtk.AlignEndValue)

	// Origin label on the left, dot on the right.
	originText := ""
	originLabel := gtk.NewLabel(&originText)
	if originLabel == nil {
		return nil, nil, nil
	}
	originLabel.AddCssClass("webrtc-indicator-origin")
	originLabel.SetHalign(gtk.AlignStartValue)
	originLabel.SetVisible(false) // Hidden when collapsed; shown on expand.

	dot := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if dot == nil {
		return nil, nil, nil
	}
	dot.AddCssClass("webrtc-indicator-dot")
	dot.SetHalign(gtk.AlignEndValue)
	dot.SetValign(gtk.AlignCenterValue)

	dotRow.Append(&originLabel.Widget)
	dotRow.Append(&dot.Widget)

	return dotRow, dot, originLabel
}

func createIndicatorDetails() *gtk.Box {
	details := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if details == nil {
		return nil
	}
	details.AddCssClass("webrtc-indicator-details")
	return details
}

func createIndicatorRevealer(child *gtk.Box) *gtk.Revealer {
	revealer := gtk.NewRevealer()
	if revealer == nil {
		return nil
	}
	revealer.SetTransitionType(gtk.RevealerTransitionTypeSlideDownValue)
	revealer.SetTransitionDuration(revealerTransitionDuration)
	revealer.SetRevealChild(false)
	revealer.AddCssClass("webrtc-indicator-revealer")
	revealer.SetChild(&child.Widget)

	return revealer
}

func createWebRTCPermissionRows(details *gtk.Box) (map[entity.PermissionType]*gtk.Button, bool) {
	rows := make(map[entity.PermissionType]*gtk.Button, len(trackedWebRTCPermissionTypes))
	for _, permType := range trackedWebRTCPermissionTypes {
		text := formatRowLabel(permType, webrtcPermissionStateIdle, "", false)
		row := gtk.NewButtonWithLabel(text)
		if row == nil {
			return nil, false
		}
		row.AddCssClass("webrtc-indicator-row")
		row.SetHalign(gtk.AlignFillValue)
		details.Append(&row.Widget)
		rows[permType] = row
	}
	return rows, true
}

func (w *WebRTCPermissionIndicator) wireRowButtons() {
	for _, permType := range trackedWebRTCPermissionTypes {
		capturedPermType := permType
		btn := w.rowButtons[capturedPermType]
		if btn == nil {
			continue
		}

		clickCb := func(_ gtk.Button) {
			w.mu.Lock()
			origin := w.currentOrigin
			state := w.states[capturedPermType]
			_, hasStored := w.stored[capturedPermType]
			toggle := w.onToggleLock
			w.mu.Unlock()

			if toggle == nil {
				return
			}
			toggle(origin, capturedPermType, string(state), hasStored)
		}

		w.retainedCallbacks = append(w.retainedCallbacks, clickCb)
		btn.ConnectClicked(&clickCb)
	}
}

// Widget returns the GTK widget for adding to the main overlay.
func (w *WebRTCPermissionIndicator) Widget() *gtk.Widget {
	if w == nil || w.outerBox == nil {
		return nil
	}
	return &w.outerBox.Widget
}

// MarkRequesting marks tracked permissions as currently requesting access.
func (w *WebRTCPermissionIndicator) MarkRequesting(permTypes []entity.PermissionType) {
	w.setState(permTypes, webrtcPermissionStateRequesting)
}

// MarkAllowed marks tracked permissions as allowed.
func (w *WebRTCPermissionIndicator) MarkAllowed(permTypes []entity.PermissionType) {
	w.setState(permTypes, webrtcPermissionStateAllowed)
}

// MarkBlocked marks tracked permissions as blocked.
func (w *WebRTCPermissionIndicator) MarkBlocked(permTypes []entity.PermissionType) {
	w.setState(permTypes, webrtcPermissionStateBlocked)
}

// Origin returns the current origin tracked by the indicator.
func (w *WebRTCPermissionIndicator) Origin() string {
	if w == nil {
		return ""
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.currentOrigin
}

// SetOrigin sets the active origin for lock/unlock actions shown in details.
func (w *WebRTCPermissionIndicator) SetOrigin(origin string) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.currentOrigin = origin
	w.refreshLocked()
}

// SetStoredDecision updates whether a permission type has an explicit stored decision.
func (w *WebRTCPermissionIndicator) SetStoredDecision(
	permType entity.PermissionType,
	decision entity.PermissionDecision,
	hasStored bool,
) {
	if w == nil || !isTrackedWebRTCPermissionType(permType) {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if hasStored {
		w.stored[permType] = decision
	} else {
		delete(w.stored, permType)
	}

	w.refreshLocked()
}

// Reset clears all permission states and stored decisions, hiding the indicator.
// Call this when the active page navigates to a different origin.
func (w *WebRTCPermissionIndicator) Reset() {
	if w == nil {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	clear(w.states)
	clear(w.stored)
	w.currentOrigin = ""

	w.refreshLocked()
}

// SetOnToggleLock sets callback invoked when a row lock/unlock action is clicked.
func (w *WebRTCPermissionIndicator) SetOnToggleLock(
	callback func(origin string, permType entity.PermissionType, state string, hasStored bool),
) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onToggleLock = callback
}

func (w *WebRTCPermissionIndicator) setState(permTypes []entity.PermissionType, state webrtcPermissionState) {
	if w == nil {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	for _, permType := range permTypes {
		if !isTrackedWebRTCPermissionType(permType) {
			continue
		}
		w.states[permType] = state
	}

	w.refreshLocked()
}

func (w *WebRTCPermissionIndicator) refreshLocked() {
	if w.outerBox == nil {
		return
	}

	visible := shouldShowWebRTCPermissionIndicator(w.states)
	w.outerBox.SetVisible(visible)

	summary := summarizeWebRTCPermissionState(w.states)
	if w.dotBox != nil {
		updateStateCSSClass(&w.dotBox.Widget, summary)
	}

	if w.originLabel != nil {
		origin := w.currentOrigin
		if origin == "" {
			origin = "Current site"
		}
		w.originLabel.SetText(origin)
	}

	for _, permType := range trackedWebRTCPermissionTypes {
		row := w.rowButtons[permType]
		if row == nil {
			continue
		}

		state := w.states[permType]
		if state == "" {
			state = webrtcPermissionStateIdle
		}

		storedDecision, hasStored := w.stored[permType]
		label := formatRowLabel(permType, state, storedDecision, hasStored)
		row.SetLabel(label)
		updateStateCSSClass(&row.Widget, state)
		updateLockedCSSClass(&row.Widget, hasStored)
	}

	if !visible {
		w.setExpandedLocked(false)
	}
}

func (w *WebRTCPermissionIndicator) attachHoverController() {
	if w == nil || w.outerBox == nil {
		return
	}

	controller := gtk.NewEventControllerMotion()
	if controller == nil {
		return
	}

	enterCb := func(_ gtk.EventControllerMotion, _ float64, _ float64) {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.setExpandedLocked(true)
	}
	leaveCb := func(_ gtk.EventControllerMotion) {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.setExpandedLocked(false)
	}

	w.retainedCallbacks = append(w.retainedCallbacks, enterCb, leaveCb)
	controller.ConnectEnter(&enterCb)
	controller.ConnectLeave(&leaveCb)
	w.outerBox.AddController(&controller.EventController)
}

func (w *WebRTCPermissionIndicator) setExpandedLocked(expanded bool) {
	if w.revealer == nil || w.cardBox == nil {
		return
	}
	if w.expanded == expanded {
		return
	}

	w.expanded = expanded
	w.revealer.SetRevealChild(expanded)

	// Show/hide origin label so it doesn't take space when collapsed to just a dot.
	// Toggle hexpand alongside visibility to prevent expand propagation when collapsed.
	if w.originLabel != nil {
		w.originLabel.SetVisible(expanded)
		w.originLabel.SetHexpand(expanded)
	}

	if expanded {
		w.cardBox.AddCssClass("expanded")
	} else {
		w.cardBox.RemoveCssClass("expanded")
	}
}

func isTrackedWebRTCPermissionType(permType entity.PermissionType) bool {
	switch permType {
	case entity.PermissionTypeMicrophone, entity.PermissionTypeCamera, entity.PermissionTypeDisplay:
		return true
	default:
		return false
	}
}

func permissionDisplayName(permType entity.PermissionType) string {
	switch permType {
	case entity.PermissionTypeMicrophone:
		return "\U0001F3A4 Mic" // 🎤
	case entity.PermissionTypeCamera:
		return "\U0001F4F7 Cam" // 📷
	case entity.PermissionTypeDisplay:
		return "\U0001F4BB Screen" // 💻
	default:
		return "Unknown"
	}
}

func stateDisplayName(state webrtcPermissionState) string {
	switch state {
	case webrtcPermissionStateRequesting:
		return "\u26A0 requesting" // ⚠
	case webrtcPermissionStateAllowed:
		return "\u2713 active" // ✓
	case webrtcPermissionStateBlocked:
		return "\u2717 blocked" // ✗
	default:
		return "idle"
	}
}

func formatRowLabel(
	permType entity.PermissionType,
	state webrtcPermissionState,
	storedDecision entity.PermissionDecision,
	hasStored bool,
) string {
	name := permissionDisplayName(permType)
	status := stateDisplayName(state)

	if hasStored {
		// Locked: show current policy and offer to reset.
		policy := "allow"
		if storedDecision == entity.PermissionDenied {
			policy = "deny"
		}
		return fmt.Sprintf("%s  %s  \U0001F512 %s  \u00B7  reset", name, status, policy)
	}

	// Not locked: show the flip action.
	switch state {
	case webrtcPermissionStateAllowed, webrtcPermissionStateRequesting:
		return fmt.Sprintf("%s  %s  \u00B7  block", name, status)
	case webrtcPermissionStateBlocked:
		return fmt.Sprintf("%s  %s  \u00B7  allow", name, status)
	default:
		return fmt.Sprintf("%s  %s", name, status)
	}
}

func updateLockedCSSClass(widget *gtk.Widget, locked bool) {
	if widget == nil {
		return
	}
	if locked {
		widget.AddCssClass("row-locked")
	} else {
		widget.RemoveCssClass("row-locked")
	}
}

func updateStateCSSClass(widget *gtk.Widget, state webrtcPermissionState) {
	if widget == nil {
		return
	}

	widget.RemoveCssClass("state-idle")
	widget.RemoveCssClass("state-requesting")
	widget.RemoveCssClass("state-allowed")
	widget.RemoveCssClass("state-blocked")

	switch state {
	case webrtcPermissionStateRequesting:
		widget.AddCssClass("state-requesting")
	case webrtcPermissionStateAllowed:
		widget.AddCssClass("state-allowed")
	case webrtcPermissionStateBlocked:
		widget.AddCssClass("state-blocked")
	default:
		widget.AddCssClass("state-idle")
	}
}
