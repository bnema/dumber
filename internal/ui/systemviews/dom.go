package systemviews

import (
	"fmt"
	"html"
	"strings"
	"time"
)

type DOM interface {
	Mount(markup string) error
}

// DOMAction is a browser event delegated from the mounted systemview DOM.
type DOMAction struct {
	Action string
	Data   map[string]string
}

// DOMActionHandler handles delegated browser actions.
type DOMActionHandler func(DOMAction)

// DOMActionBinder is implemented by DOM adapters that can delegate events back
// to the App. Tests can keep using the smaller DOM interface.
type DOMActionBinder interface {
	BindActions(handler DOMActionHandler) error
}

// DOMReadinessSignaler is implemented by DOM adapters that can mark the
// loading shell ready or terminally failed. The App calls it after the
// initial mount and action binding complete; history-data completion is a
// separate concern and never gates readiness. Adapters without it keep
// legacy behavior, leaving shell state to the page script.
type DOMReadinessSignaler interface {
	// SignalReady clears the shell loading state after mount and binding.
	SignalReady() error
	// SignalError clears the shell loading state with a fatal message
	// when mount or binding fails.
	SignalError(message string) error
}

// DOMHistoryTimelineAppender is implemented by DOM adapters that can append
// older history windows without reparsing and remounting the whole page.
type DOMHistoryTimelineAppender interface {
	AppendHistoryTimeline(markup string) error
}

// DOMRegion is the new inner markup of one element marked data-sv-region.
type DOMRegion struct {
	Name string
	HTML string
}

// RegionUpdate swaps several regions as one visual change.
type RegionUpdate struct {
	Regions []DOMRegion
	// Transition asks the visual layer to animate the swap when it can.
	// Live typing keeps it off so results never lag behind keystrokes.
	Transition bool
	// Focus, when set, names the element to focus after the swap, as a
	// focus key: "sel:<css selector>", "key:<data-sv-focus-key>" or
	// "id:<id>". It replaces the previously focused element, and like it is
	// only applied when the swap left focus outside the page content.
	Focus string
}

// DOMRegionUpdater is implemented by DOM adapters that can replace named
// regions in place, leaving the rest of the page (and its focus) untouched.
type DOMRegionUpdater interface {
	UpdateRegions(update RegionUpdate) error
}

// ToastRequest describes a transient notification, with at most one action.
// The DOM dispatches OnAction when the user takes the action, and OnDismiss
// when the toast times out, is replaced by a newer toast, or the page is
// leaving. Empty actions are not dispatched; without ActionLabel the toast
// is a plain status message.
type ToastRequest struct {
	Message     string
	ActionLabel string
	Timeout     time.Duration
	OnAction    DOMAction
	OnDismiss   DOMAction
}

// DOMToaster is implemented by DOM adapters that can show an action toast.
// ShowToast reports false when no toast UI is available, so callers can fall
// back to an immediate, confirmed operation.
type DOMToaster interface {
	ShowToast(request ToastRequest) bool
}

// DOM-handled actions: the adapter performs them itself (navigation, focus)
// instead of sending them to the App.
const (
	uiActionNavigate    = "ui.navigate"
	uiActionFocusSearch = "ui.focusSearch"
	// uiActionConfigSection scrolls the settings page to Data["section"].
	uiActionConfigSection = "ui.config.section"
	// uiActionConfigSave submits the settings form (same as Ctrl+S).
	uiActionConfigSave = "ui.config.save"
	// uiActionConfigDiscard resets the settings form to the stored values.
	uiActionConfigDiscard = "ui.config.discard"
	// uiActionConfigResetKeys puts a keybinding row back to its default keys
	// in the form; the change is saved with the rest of the form.
	uiActionConfigResetKeys = "ui.config.resetKeys"
)

// PaletteItem is one command offered by the action palette.
type PaletteItem struct {
	ID    string
	Label string
	Group string
	// Hint is secondary text shown on the right (shortcut or context).
	Hint string
	// Keywords widen matching beyond the label.
	Keywords string
	Danger   bool
	// Confirm, when set, asks for confirmation before Action runs.
	Confirm string
	Action  DOMAction
}

// PaletteRequest opens the action palette with Items.
type PaletteRequest struct {
	Placeholder string
	Items       []PaletteItem
}

// DOMPalette is implemented by DOM adapters that can show the action
// palette. Without a palette UI it does nothing.
type DOMPalette interface {
	ShowPalette(request PaletteRequest)
}

// cssQuotedReplacer escapes what can end or break a double-quoted CSS string:
// the quote, backslash, line breaks (as hex escapes, the space ends them) and
// NUL, which CSS replaces with U+FFFD anyway.
var cssQuotedReplacer = strings.NewReplacer(
	`\`, `\\`,
	`"`, `\"`,
	"\n", `\a `,
	"\r", `\d `,
	"\f", `\c `,
	"\x00", "\uFFFD",
)

// cssQuoted escapes s for a double-quoted CSS attribute selector value.
func cssQuoted(s string) string {
	return cssQuotedReplacer.Replace(s)
}

func placeholderHTML(route Route) string {
	name := html.EscapeString(string(route))
	if name == "" || route == RouteUnknown {
		name = "unknown"
	}

	return fmt.Sprintf(`<p>systemviews %s</p>`, name)
}
