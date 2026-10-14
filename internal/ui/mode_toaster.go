package ui

import (
	"reflect"
	"strings"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/puregotk/v4/glib"
)

const legendFlashDurationMs = 120

// chevronState is the expand/collapse affordance in the toaster header.
type chevronState int

const (
	// chevronNone: nothing to expand (legend off, or no bindings).
	chevronNone chevronState = iota
	chevronExpanded
	chevronCollapsed
)

// modeToasterView is the toolkit side of the mode toaster: a header pill and
// the legend body that expands from it. Implementations ignore calls that
// change nothing, since the toaster reconciles on every state change.
type modeToasterView interface {
	setHeader(text string)
	// setMode colors the pill and the legend for mode.
	setMode(mode input.Mode)
	setChevron(chevronState)
	setPillVisible(bool)
	// setPillInteractive makes the pill, and only the pill, receive pointer
	// input.
	setPillInteractive(bool)
	setLegend(groups []legendGroup)
	setStyle(style string)
	setAnimations(bool)
	setBodyVisible(bool)
	setBodyLingering(bool)
	setActionDimmed(action string, dimmed bool)
	// flashAction lights the keycaps of action and reports whether it has any.
	flashAction(action string) bool
	clearFlash()
	// lingerRect is the screen area of the legend body, in overlay coordinates,
	// while it is on screen.
	lingerRect() (x, y, width, height int, ok bool)
	destroy()
}

// modeToaster is the per-window mode indicator: a pill anchored bottom-left
// that names the active mode and any pending key sequence or Vim sub-mode,
// plus the key legend that expands from it. The mode border around the target
// pane is modeFrame's.
//
// It holds all state and decisions; its view only paints. Main thread only.
type modeToaster struct {
	view modeToasterView

	mode    input.Mode
	detail  string // pending sequence or Vim sub-mode, appended to the header
	pending string // pending sequence the legend rows are dimmed for
	cfg     entity.WorkspaceStylingConfig
	style   string
	actions map[string]entity.ActionBinding
	groups  []legendGroup

	// collapsed hides the legend body; the pill stays. It is shared by every
	// mode and window: the app owns it (modeLegendPreference), pushes it with
	// each configuration and broadcasts it on a click.
	collapsed bool
	// ready: the legend's show delay has elapsed.
	ready bool
	// suspended hides the legend while an in-page interaction (link hints,
	// visual selection) owns the keys.
	suspended bool
	// lingering keeps a read-only legend on screen after its mode ended while
	// the pointer was over it, until the next key press or click.
	lingering  bool
	generation uint64
	showTimer  uint
	flashTimer uint
	// onToggle receives the collapsed state a click on the pill asks for. The
	// app applies it to every window. Without it the toaster applies it alone.
	onToggle func(collapsed bool)

	pressInFlight      bool
	pressIdle          uint
	pointerInside      bool
	pointerX, pointerY float64
}

func newModeToasterWithView(view modeToasterView, onToggle func(bool)) *modeToaster {
	return &modeToaster{view: view, mode: input.ModeNormal, onToggle: onToggle}
}

// modeToasterHeader is the pill text: the mode name, then " · detail".
func modeToasterHeader(mode input.Mode, detail string) string {
	if detail == "" {
		return mode.DisplayName()
	}
	return mode.DisplayName() + " · " + detail
}

func (t *modeToaster) live() bool { return t != nil && t.view != nil }

// hasLegend reports whether a legend can expand from the pill.
func (t *modeToaster) hasLegend() bool {
	return t.mode != input.ModeNormal && modeLegendEnabled(t.cfg) && len(t.groups) > 0
}

func (t *modeToaster) bodyShown() bool {
	return t.lingering || (t.hasLegend() && legendBodyShown(t.ready, t.collapsed, t.suspended))
}

func (t *modeToaster) chevron() chevronState {
	switch {
	case !t.hasLegend():
		return chevronNone
	case t.collapsed:
		return chevronCollapsed
	default:
		return chevronExpanded
	}
}

// reconcile pushes the current state to the view.
func (t *modeToaster) reconcile() {
	if !t.live() {
		return
	}
	pill := modeToasterVisible(t.mode, t.cfg)
	t.view.setPillVisible(pill)
	if pill {
		// A lingering legend has no pill (its mode ended) and keeps the colors
		// of the mode it was shown for.
		t.view.setMode(t.mode)
		t.view.setHeader(modeToasterHeader(t.mode, t.detail))
		t.view.setChevron(t.chevron())
		t.view.setPillInteractive(t.hasLegend())
	}
	t.view.setBodyVisible(t.bodyShown())
}

// sameLegendInputs reports whether cfg, style and actions leave the built
// legend as it is. The indicator preference and the collapsed state are not
// part of it.
func (t *modeToaster) sameLegendInputs(cfg entity.WorkspaceStylingConfig, style string, actions map[string]entity.ActionBinding) bool {
	return cfg.ModeLegend == t.cfg.ModeLegend &&
		cfg.ModeLegendDelayMs == t.cfg.ModeLegendDelayMs &&
		cfg.ModeLegendAnimations == t.cfg.ModeLegendAnimations &&
		cfg.ModeLegendLinger == t.cfg.ModeLegendLinger &&
		style == t.style && reflect.DeepEqual(actions, t.actions)
}

// apply reconciles the toaster with mode and the live configuration. A mode
// change, or a change to what the legend lists or how it shows, rebuilds it.
// Anything else (a config reload caused by saving the collapsed state, say)
// only updates the preferences, so the legend neither flickers nor restarts
// its show delay.
func (t *modeToaster) apply(mode input.Mode, cfg entity.WorkspaceStylingConfig, style string, actions map[string]entity.ActionBinding) {
	if !t.live() {
		return
	}
	t.collapsed = cfg.ModeLegendCollapsed
	if mode == t.mode && (mode == input.ModeNormal || t.sameLegendInputs(cfg, style, actions)) {
		t.cfg = cfg
		t.reconcile()
		return
	}
	t.setMode(mode, cfg, style, actions)
}

func (t *modeToaster) setMode(mode input.Mode, cfg entity.WorkspaceStylingConfig, style string, actions map[string]entity.ActionBinding) {
	t.cancelTimers()
	// Normal-mode refreshes must preserve an already lingering legend.
	keepLinger := mode == input.ModeNormal && t.mode == input.ModeNormal && t.lingering
	linger := shouldStartLinger(mode == input.ModeNormal && !t.lingering && t.bodyShown(), t.pressInFlight,
		cfg.ModeLegendLinger, t.pointerInside, t.pointerX, t.pointerY, t.view.lingerRect)
	if !keepLinger {
		t.clearLinger()
	}
	t.generation++
	t.mode, t.cfg, t.style, t.actions = mode, cfg, style, actions
	t.detail, t.pending = "", ""
	t.suspended = false
	t.ready = false
	t.view.setStyle(style)
	if linger {
		t.lingering = true
		t.clearFeedback()
		t.view.setBodyLingering(true)
	}
	if mode == input.ModeNormal || !modeLegendEnabled(cfg) {
		if !linger && !keepLinger {
			t.groups = nil
			t.view.setLegend(nil)
		}
		t.reconcile()
		return
	}
	t.groups = buildLegendGroups(mode, actions)
	t.view.setLegend(t.groups)
	t.view.setAnimations(cfg.ModeLegendAnimations)
	if cfg.ModeLegend == "delay" && cfg.ModeLegendDelayMs > 0 {
		generation := t.generation
		cb := glib.SourceFunc(func(_ uintptr) bool {
			t.showTimer = 0
			if generation == t.generation && t.live() {
				t.ready = true
				t.reconcile()
			}
			return false
		})
		t.showTimer = glib.TimeoutAdd(uint(cfg.ModeLegendDelayMs), &cb, 0)
	} else {
		t.ready = true
	}
	t.reconcile()
}

// setDetail shows detail after the Vim Mode name in the header: the sub-mode of
// an in-page interaction, or "" for the plain name. Other modes have none.
func (t *modeToaster) setDetail(detail string) {
	if !t.live() || t.mode != input.ModeVim || t.detail == detail {
		return
	}
	t.detail = detail
	t.reconcile()
}

// setPending shows the pending key sequence in the header and dims the legend
// rows it rules out.
func (t *modeToaster) setPending(pending string) {
	if !t.live() || t.mode != input.ModeVim {
		return
	}
	t.detail = pending
	t.pending = pending
	for _, group := range t.groups {
		for _, row := range group.Rows {
			t.view.setActionDimmed(row.Action, !legendSequenceMatches(pending, row.Keys))
		}
	}
	t.reconcile()
}

// setSuspended hides the legend while suspended is true and brings it back
// otherwise, without touching the mode or a pending show delay. Used while an
// in-page interaction owns the keys, when the legend would list bindings that
// do not apply. The header keeps showing the sub-mode.
func (t *modeToaster) setSuspended(suspended bool) {
	if !t.live() || t.suspended == suspended {
		return
	}
	t.suspended = suspended
	t.reconcile()
}

// setCollapsed collapses or expands the legend.
func (t *modeToaster) setCollapsed(collapsed bool) {
	if !t.live() || t.collapsed == collapsed {
		return
	}
	t.collapsed = collapsed
	if !collapsed && t.showTimer != 0 {
		// Expanding is a request to see the legend now, not after its delay.
		glib.SourceRemove(t.showTimer)
		t.showTimer = 0
		t.ready = true
	}
	t.reconcile()
}

// pillClicked handles a click on the header: it toggles the legend.
func (t *modeToaster) pillClicked() {
	if !t.live() || t.lingering || !t.hasLegend() {
		return
	}
	next := nextModeLegendCollapsed(t.collapsed)
	if t.onToggle != nil {
		t.onToggle(next)
		return
	}
	t.setCollapsed(next)
}

// release drops the legend of a tab whose workspace is going away. The header
// stays for the active mode.
func (t *modeToaster) release() {
	if !t.live() {
		return
	}
	t.cancelTimers()
	t.clearLinger()
	t.generation++
	t.ready = false
	t.reconcile()
}

func (t *modeToaster) clearLinger() {
	if !t.lingering {
		return
	}
	t.lingering = false
	t.view.setBodyLingering(false)
}

func (t *modeToaster) dismissLinger() {
	if !t.live() || !t.lingering {
		return
	}
	t.clearLinger()
	t.reconcile()
}

func (t *modeToaster) clearFeedback() {
	for _, group := range t.groups {
		for _, row := range group.Rows {
			t.view.setActionDimmed(row.Action, false)
		}
	}
	t.view.clearFlash()
}

func (t *modeToaster) cancelTimers() {
	if t.showTimer != 0 {
		glib.SourceRemove(t.showTimer)
		t.showTimer = 0
	}
	if t.flashTimer != 0 {
		glib.SourceRemove(t.flashTimer)
		t.flashTimer = 0
	}
}

// flash lights the keycaps of a dispatched action.
func (t *modeToaster) flash(action input.Action) {
	if !t.live() || t.lingering || !t.bodyShown() || !t.cfg.ModeLegendAnimations {
		return
	}
	name := string(action)
	if !t.view.flashAction(name) {
		name = strings.ReplaceAll(name, "_", "-")
		if !t.view.flashAction(name) {
			return
		}
	}
	if t.flashTimer != 0 {
		glib.SourceRemove(t.flashTimer)
	}
	cb := glib.SourceFunc(func(_ uintptr) bool {
		t.flashTimer = 0
		if t.live() {
			t.view.clearFlash()
		}
		return false
	})
	t.flashTimer = glib.TimeoutAdd(legendFlashDurationMs, &cb, 0)
}

func (t *modeToaster) pointerMoved(x, y float64) {
	t.pointerX, t.pointerY, t.pointerInside = x, y, true
}

func (t *modeToaster) pointerLeft() { t.pointerInside = false }

// keyPressed ends a lingering legend: the next key press means the user moved on.
func (t *modeToaster) keyPressed() { t.dismissLinger() }

// pressStarted records a click outside the pill: it ends a lingering legend,
// and a mode that exits because of it must not start one.
func (t *modeToaster) pressStarted() {
	if !t.live() {
		return
	}
	t.dismissLinger()
	t.pressInFlight = true
	if t.pressIdle != 0 {
		glib.SourceRemove(t.pressIdle)
	}
	cb := glib.SourceFunc(func(_ uintptr) bool {
		t.pressInFlight = false
		t.pressIdle = 0
		return false
	})
	t.pressIdle = glib.IdleAdd(&cb, 0)
}

func (t *modeToaster) destroy() {
	if t == nil {
		return
	}
	t.cancelTimers()
	if t.pressIdle != 0 {
		glib.SourceRemove(t.pressIdle)
		t.pressIdle = 0
	}
	if t.view != nil {
		t.view.destroy()
		t.view = nil
	}
}

func pointInRect(x, y float64, rx, ry, width, height int) bool {
	return x >= float64(rx) && y >= float64(ry) && x < float64(rx+width) && y < float64(ry+height)
}

func shouldLinger(enabled, inside bool, x, y float64, placement func() (int, int, int, int, bool)) bool {
	if !enabled || !inside || placement == nil {
		return false
	}
	rx, ry, w, h, ok := placement()
	return ok && pointInRect(x, y, rx, ry, w, h)
}

func shouldStartLinger(exiting, pressInFlight, enabled, inside bool, x, y float64, placement func() (int, int, int, int, bool)) bool {
	return exiting && !pressInFlight && shouldLinger(enabled, inside, x, y, placement)
}
