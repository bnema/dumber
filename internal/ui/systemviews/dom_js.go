//go:build js && wasm

package systemviews

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"syscall/js"
	"time"
)

// pagehideYields bounds how long the pagehide callback lets queued work run.
const pagehideYields = 64

// liveSearchDebounce waits for a short typing pause before searching.
const liveSearchDebounce = 150 * time.Millisecond

type browserDOM struct {
	document                js.Value
	target                  js.Value
	bindings                []jsEventBinding
	historyObserver         js.Value
	historyObserverCallback js.Func
	activeConfirmCleanup    func()
	activeConfirmID         uint64
	liveSearchTimer         *time.Timer
	actionHandler           DOMActionHandler
	// restoreFocusIndex keeps keyboard focus at the same row position after
	// an undoable delete re-renders the list (-1 when unused).
	restoreFocusIndex int
	// undoFocusKey is the focus key of the row an undoable delete removed;
	// an undo that brings the row back focuses it again.
	undoFocusKey string
	paletteOpen  bool
}

type jsEventBinding struct {
	target js.Value
	event  string
	fn     js.Func
}

func NewDOM() DOM {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		return &browserDOM{}
	}

	target := doc.Call("getElementById", "app")
	return &browserDOM{document: doc, target: target, restoreFocusIndex: -1}
}

func (d *browserDOM) Mount(markup string) error {
	if d == nil || !d.target.Truthy() {
		return fmt.Errorf("DOM mount target #app not found")
	}
	focus := d.captureFocusKey()
	search := d.captureHistorySearch()
	d.target.Set("innerHTML", markup)
	d.updateDocumentTitle()
	d.scheduleAlertDismissal()
	d.syncPerformanceCustomInputStates()
	d.focusAutofocusTarget()
	d.setupHistoryInfiniteScroll()
	if visualLayerHas("enhance") {
		js.Global().Get("dumberSystemviews").Call("enhance", d.target)
	}
	d.restoreFocusKey(focus)
	d.restoreHistorySearch(search)
	return nil
}

// historySearchState is the focused history search field's text and caret,
// kept across a full Mount so typing during the first load is not lost.
type historySearchState struct {
	value      string
	start, end js.Value
	ok         bool
}

func (d *browserDOM) captureHistorySearch() historySearchState {
	active := activeElement()
	if !active.Truthy() || active.Get("matches").Type() != js.TypeFunction ||
		!active.Call("matches", "[data-sv-history-search]").Bool() {
		return historySearchState{}
	}
	return historySearchState{
		value: active.Get("value").String(),
		start: active.Get("selectionStart"),
		end:   active.Get("selectionEnd"),
		ok:    true,
	}
}

func (d *browserDOM) restoreHistorySearch(state historySearchState) {
	if !state.ok {
		return
	}
	input := d.target.Call("querySelector", "[data-sv-history-search]")
	if !input.Truthy() {
		return
	}
	input.Set("value", state.value)
	if !activeElement().Equal(input) {
		input.Call("focus", map[string]any{"preventScroll": true})
	}
	if state.start.Type() == js.TypeNumber && state.end.Type() == js.TypeNumber {
		input.Call("setSelectionRange", state.start, state.end)
	}
}

// focusableSelector finds the element to focus inside a keyed container.
const focusableSelector = "a[href], button:not([disabled]), input:not([type=hidden]):not([disabled]), select, textarea"

// captureFocusKey names the focused element so a re-render can focus its
// replacement: the nearest [data-sv-focus-key] container, else its id.
func (d *browserDOM) captureFocusKey() string {
	active := activeElement()
	if !active.Truthy() || active.Equal(d.target) || !d.target.Call("contains", active).Bool() {
		return ""
	}
	if keyed := active.Call("closest", "[data-sv-focus-key]"); keyed.Truthy() {
		return "key:" + keyed.Call("getAttribute", "data-sv-focus-key").String()
	}
	if id := active.Get("id").String(); id != "" {
		return "id:" + id
	}
	if name := active.Call("getAttribute", "name"); name.Truthy() && name.String() != "" {
		// Radios and checkboxes share a name: the value tells them apart.
		return `sel:[name="` + cssAttrEscape(name.String()) + `"][value="` + cssAttrEscape(active.Get("value").String()) + `"]`
	}
	return ""
}

func activeInsideRow() bool {
	active := activeElement()
	return active.Truthy() && active.Get("closest").Type() == js.TypeFunction &&
		active.Call("closest", "[data-sv-history-row], [data-sv-undo-row]").Truthy()
}

func (d *browserDOM) focusKeyPresent(key string) bool {
	name := strings.TrimPrefix(key, "key:")
	return d.target.Call("querySelector", `[data-sv-focus-key="`+cssAttrEscape(name)+`"]`).Truthy()
}

// restoreFocusKey focuses the element captureFocusKey named, unless the
// re-render already put focus somewhere on the page.
func (d *browserDOM) restoreFocusKey(key string) {
	if key == "" {
		return
	}
	if active := activeElement(); active.Truthy() && !active.Equal(d.document.Get("body")) &&
		!active.Equal(d.target) && d.target.Call("contains", active).Bool() {
		return
	}
	var element js.Value
	if name, ok := strings.CutPrefix(key, "key:"); ok {
		element = d.target.Call("querySelector", `[data-sv-focus-key="`+cssAttrEscape(name)+`"]`)
		if element.Truthy() && !element.Call("matches", focusableSelector).Bool() {
			element = element.Call("querySelector", focusableSelector)
		}
	} else if id, ok := strings.CutPrefix(key, "id:"); ok {
		element = d.target.Call("querySelector", `[id="`+cssAttrEscape(id)+`"]`)
	} else if selector, ok := strings.CutPrefix(key, "sel:"); ok {
		element = d.target.Call("querySelector", selector)
	}
	if element.Truthy() {
		element.Call("focus", map[string]any{"preventScroll": true})
		if activeElement().Equal(element) {
			return
		}
	}
	// The element is gone (a deleted tag) or cannot take focus (hidden, or
	// inside a closed popover): use the page's fallback.
	fallback := d.target.Call("querySelector", "[data-sv-focus-fallback]")
	if fallback.Truthy() && !fallback.Call("matches", focusableSelector).Bool() {
		fallback = fallback.Call("querySelector", focusableSelector)
	}
	if fallback.Truthy() {
		fallback.Call("focus", map[string]any{"preventScroll": true})
	}
}

// SignalReady marks the shell ready after mount and action binding: the
// loading busy flag is dropped and a data-ready marker is set for
// fixtures. Mount replaces #app content, so readiness must be signaled
// after every mount that completes binding.
func (d *browserDOM) SignalReady() error {
	if d == nil || !d.target.Truthy() {
		return fmt.Errorf("DOM readiness target #app not found")
	}
	d.target.Call("removeAttribute", "aria-busy")
	d.target.Call("setAttribute", "data-ready", "true")
	return nil
}

// SignalError clears the loading state with a fatal message when mount or
// binding fails. textContent assignment keeps the message inert.
func (d *browserDOM) SignalError(message string) error {
	if d == nil || !d.target.Truthy() {
		return fmt.Errorf("DOM readiness target #app not found")
	}
	if message == "" {
		message = "systemviews failed to start"
	}
	d.target.Call("removeAttribute", "aria-busy")
	d.target.Call("setAttribute", "data-failed", "true")
	d.target.Set("innerHTML", "")
	doc := d.document
	if !doc.Truthy() {
		doc = js.Global().Get("document")
	}
	if doc.Truthy() {
		failure := doc.Call("createElement", "div")
		failure.Set("className", "sv-error")
		failure.Set("textContent", message)
		d.target.Call("appendChild", failure)
	}
	return nil
}

func (d *browserDOM) clearHistorySearchInput() {
	d.stopLiveSearch()
	if input := d.target.Call("querySelector", "[data-sv-history-search]"); input.Truthy() {
		input.Set("value", "")
	}
}

// isPaletteShortcut matches Ctrl+K (Cmd+K on macOS keyboards).
func isPaletteShortcut(event js.Value) bool {
	if !strings.EqualFold(event.Get("key").String(), "k") || event.Get("altKey").Bool() || event.Get("shiftKey").Bool() {
		return false
	}
	return event.Get("ctrlKey").Bool() || event.Get("metaKey").Bool()
}

// paletteItems converts the request items for systemviews.js and indexes
// them by id.
func paletteItems(request PaletteRequest) ([]any, map[string]PaletteItem) {
	items := make([]any, 0, len(request.Items))
	byID := make(map[string]PaletteItem, len(request.Items))
	for i := range request.Items {
		item := &request.Items[i]
		byID[item.ID] = *item
		items = append(items, map[string]any{
			"id":       item.ID,
			"label":    item.Label,
			"group":    item.Group,
			"hint":     item.Hint,
			"keywords": item.Keywords,
			"danger":   item.Danger,
		})
	}
	return items, byID
}

// ShowPalette opens the action palette from systemviews.js and runs the
// chosen item's action (after a confirm dialog when the item asks for one).
func (d *browserDOM) ShowPalette(request PaletteRequest) {
	if d == nil || d.actionHandler == nil || d.paletteOpen || !visualLayerHas("palette") {
		return
	}
	items, byID := paletteItems(request)
	visual := js.Global().Get("dumberSystemviews")
	all := js.ValueOf(items)
	placeholder := request.Placeholder
	if d.isConfigRoute() && visual.Get("settingsIndex").Type() == js.TypeFunction {
		// Every setting on the page becomes searchable; choosing one jumps to it.
		all = all.Call("concat", visual.Call("settingsIndex", d.target))
		placeholder = "Search settings or type a command…"
	}
	result := visual.Call("palette", map[string]any{
		"placeholder": placeholder,
		"items":       all,
		"theme":       d.target.Call("querySelector", ".sv-app"),
	})
	if !result.Truthy() || result.Get("then").Type() != js.TypeFunction {
		return
	}
	d.paletteOpen = true
	var settled js.Func
	settled = js.FuncOf(func(_ js.Value, args []js.Value) any {
		settled.Release()
		d.paletteOpen = false
		if len(args) > 0 && args[0].Type() == js.TypeString {
			d.runPaletteChoice(visual, byID, args[0].String())
		}
		return nil
	})
	result.Call("then", settled, settled)
}

// runPaletteChoice jumps to a chosen setting or runs a chosen item's action.
func (d *browserDOM) runPaletteChoice(visual js.Value, byID map[string]PaletteItem, id string) {
	if strings.HasPrefix(id, "setting:") {
		visual.Call("jumpToSetting", id)
		return
	}
	item, ok := byID[id]
	if !ok {
		return
	}
	run := func() { d.runPaletteAction(item.Action) }
	if item.Confirm == "" || !d.showConfirmDialog(item.Confirm, run) {
		run()
	}
}

func (d *browserDOM) runPaletteAction(action DOMAction) {
	switch action.Action {
	case uiActionNavigate:
		d.navigateToRoute(Route(action.Data["route"]))
	case uiActionConfigSave:
		d.submitConfigForm()
	case uiActionConfigDiscard:
		if form := d.target.Call("querySelector", "[data-sv-config-form]"); form.Truthy() {
			form.Call("reset")
		}
	case uiActionConfigSection:
		if section := d.target.Call("querySelector", "#"+cssIdentifier(action.Data["section"])); section.Truthy() {
			section.Call("scrollIntoView", map[string]any{"behavior": scrollBehavior(), "block": "start"})
			// Move focus too, so the next Tab continues inside the section.
			if visualLayerHas("focusSection") {
				js.Global().Get("dumberSystemviews").Call("focusSection", section)
			} else {
				if !section.Call("hasAttribute", "tabindex").Bool() {
					section.Call("setAttribute", "tabindex", "-1")
				}
				section.Call("focus", map[string]any{"preventScroll": true})
			}
		}
	case uiActionFocusSearch:
		if input := d.target.Call("querySelector", "[data-sv-history-search]"); input.Truthy() {
			input.Call("focus")
			input.Call("select")
		}
	case uiActionFocus:
		if element := d.target.Call("querySelector", action.Data["selector"]); element.Truthy() {
			element.Call("focus")
		}
	case uiActionOpenPopover:
		if popover := d.target.Call("querySelector", "#"+cssIdentifier(action.Data["id"])); popover.Truthy() &&
			popover.Get("showPopover").Type() == js.TypeFunction {
			popover.Call("showPopover")
		}
	default:
		if action.Action == historyActionClear {
			d.clearHistorySearchInput()
		}
		d.actionHandler(action)
	}
}

// navigateToRoute opens another system view in the same tab, keeping the
// scheme the page was loaded with (dumb:// or the internal https origin).
func (d *browserDOM) navigateToRoute(route Route) {
	switch route {
	case RouteHistory, RouteFavorites, RouteConfig:
	default:
		return
	}
	location := js.Global().Get("location")
	target := location.Get("origin").String() + "/" + string(route)
	if strings.EqualFold(location.Get("protocol").String(), "dumb:") {
		target = "dumb://" + string(route)
	}
	location.Call("assign", target)
}

// rowLeaveDuration matches .sv-row-leaving in systemviews.css.
const rowLeaveDuration = 160 * time.Millisecond

// startUndoableAction handles a delete that the visual layer can undo: no
// confirm dialog, the row animates out, and the action is dispatched once
// the animation ends. It reports false when the regular path must run.
func (d *browserDOM) startUndoableAction(element js.Value, action string, data map[string]string, handler DOMActionHandler) bool {
	if !element.Call("hasAttribute", "data-sv-undoable").Bool() || !visualLayerHas("toast") {
		return false
	}
	row := element.Call("closest", "[data-sv-history-row], [data-sv-undo-row]")
	if row.Truthy() && (row.Get("classList").Call("contains", "sv-row-leaving").Bool() || row.Call("hasAttribute", "data-sv-pending").Bool()) {
		// A repeated key during the leave animation: already dispatched.
		return true
	}
	delete(data, "svUndoable")
	data[undoDataKey] = "true"

	d.undoFocusKey = ""
	if keyed := element.Call("closest", "[data-sv-focus-key]"); keyed.Truthy() {
		d.undoFocusKey = "key:" + keyed.Call("getAttribute", "data-sv-focus-key").String()
	}
	if popover := element.Call("closest", "[popover]"); popover.Truthy() && popover.Get("hidePopover").Type() == js.TypeFunction {
		popover.Call("hidePopover")
	}
	if row.Truthy() {
		row.Call("setAttribute", "data-sv-pending", "")
	}
	if row.Truthy() && (row.Get("classList").Call("contains", "sv-focused").Bool() || row.Call("contains", activeElement()).Bool()) {
		// Focus moves to the row that takes the deleted row's place.
		if row.Call("matches", "[data-sv-favorite-row]").Bool() {
			d.restoreFocusIndex = d.rowIndex(favoriteRowSelector, row)
		} else if row.Call("matches", "[data-sv-history-row]").Bool() {
			d.restoreFocusIndex = d.rowIndex(historyRowSelector, row)
		}
	}
	dispatch := func() { handler(DOMAction{Action: action, Data: data}) }
	if !row.Truthy() || prefersReducedMotion() {
		dispatch()
		return true
	}
	row.Get("classList").Call("add", "sv-row-leaving")
	time.AfterFunc(rowLeaveDuration, dispatch)
	return true
}

// ShowToast renders an action toast through systemviews.js and dispatches
// the request's follow-up action when the toast settles.
func (d *browserDOM) ShowToast(request ToastRequest) bool {
	if d == nil || d.actionHandler == nil || !visualLayerHas("toast") {
		return false
	}
	options := map[string]any{
		"message":     request.Message,
		"actionLabel": request.ActionLabel,
		"timeoutMs":   request.Timeout.Milliseconds(),
		"theme":       d.target.Call("querySelector", ".sv-app"),
	}
	result := js.Global().Get("dumberSystemviews").Call("toast", options)
	if !result.Truthy() || result.Get("then").Type() != js.TypeFunction {
		return false
	}
	handler := d.actionHandler
	// The key of the row this toast can bring back. A newer delete replaces
	// it, so only this toast's own key is cleared below.
	undoKey := d.undoFocusKey
	var settled js.Func
	settled = js.FuncOf(func(_ js.Value, args []js.Value) any {
		settled.Release()
		result := ""
		if len(args) > 0 && args[0].Type() == js.TypeString {
			result = args[0].String()
		}
		next := request.OnDismiss
		if result == "action" {
			next = request.OnAction
		} else if result != "replaced" && undoKey != "" && d.undoFocusKey == undoKey {
			// No undo is coming: later swaps must not chase the removed row.
			d.undoFocusKey = ""
		}
		if next.Action != "" {
			handler(next)
		}
		return nil
	})
	result.Call("then", settled, settled)
	return true
}

func (d *browserDOM) dismissToast() {
	if visualLayerHas("dismissToast") {
		js.Global().Get("dumberSystemviews").Call("dismissToast")
	}
}

func visualLayerHas(name string) bool {
	visual := js.Global().Get("dumberSystemviews")
	return visual.Truthy() && visual.Get(name).Type() == js.TypeFunction
}

func scrollBehavior() string {
	if prefersReducedMotion() {
		return "auto"
	}
	return "smooth"
}

func prefersReducedMotion() bool {
	matchMedia := js.Global().Get("matchMedia")
	if matchMedia.Type() != js.TypeFunction {
		return false
	}
	return matchMedia.Invoke("(prefers-reduced-motion: reduce)").Get("matches").Bool()
}

// scheduleLiveSearch restarts the debounce timer; only the query typed last
// before the pause is sent.
func (d *browserDOM) scheduleLiveSearch(query string, handler DOMActionHandler) {
	d.stopLiveSearch()
	d.liveSearchTimer = time.AfterFunc(liveSearchDebounce, func() {
		handler(DOMAction{Action: historyActionSearchLive, Data: map[string]string{"query": query}})
	})
}

func (d *browserDOM) stopLiveSearch() {
	if d.liveSearchTimer != nil {
		d.liveSearchTimer.Stop()
		d.liveSearchTimer = nil
	}
}

// UpdateRegions replaces each [data-sv-region] in place. The optional visual
// layer (systemviews.js) performs the whole swap in one call so it can
// animate the new rows; otherwise the markup is set directly.
func (d *browserDOM) UpdateRegions(update RegionUpdate) error {
	if d == nil || !d.target.Truthy() {
		return fmt.Errorf("DOM mount target #app not found")
	}
	focus := d.captureFocusKey()
	if d.undoFocusKey != "" && !d.focusKeyPresent(d.undoFocusKey) && (focus == "" || activeInsideRow()) {
		// An undo brings the removed row back: return focus to it, unless
		// the user has moved on to something other than the list.
		focus = d.undoFocusKey
	}
	defer func() {
		if d.undoFocusKey != "" && d.focusKeyPresent(d.undoFocusKey) {
			// The undo restored the row: the key is consumed.
			d.undoFocusKey = ""
		}
		d.restoreFocusKey(focus)
	}()
	elements := make([]js.Value, 0, len(update.Regions))
	for _, region := range update.Regions {
		element := d.target.Call("querySelector", `[data-sv-region="`+region.Name+`"]`)
		if !element.Truthy() {
			return fmt.Errorf("systemview region %q not found", region.Name)
		}
		elements = append(elements, element)
	}

	visual := js.Global().Get("dumberSystemviews")
	if visual.Truthy() && visual.Get("swap").Type() == js.TypeFunction {
		entries := make([]any, 0, len(update.Regions))
		for i, region := range update.Regions {
			entries = append(entries, []any{elements[i], region.HTML})
		}
		visual.Call("swap", entries, map[string]any{"transition": update.Transition})
	} else {
		for i, region := range update.Regions {
			elements[i].Set("innerHTML", region.HTML)
		}
	}
	d.afterRegionUpdate()
	return nil
}

func (d *browserDOM) afterRegionUpdate() {
	if dataset := d.target.Get("dataset"); dataset.Truthy() {
		dataset.Delete(historyIndexKey)
		dataset.Delete(favoriteIndexKey)
	}
	if index := d.restoreFocusIndex; index >= 0 {
		d.restoreFocusIndex = -1
		if d.isFavoritesRoute() {
			d.focusRowAt(favoriteRowSelector, favoriteIndexKey, index)
		} else {
			d.focusRowAt(historyRowSelector, historyIndexKey, index)
		}
	}
	d.updateDocumentTitle()
	d.scheduleAlertDismissal()
	d.setupHistoryInfiniteScroll()
	d.focusErrorField()
}

// focusErrorField moves focus to the form field named by a validation error
// ([data-sv-error-field]) and marks it invalid until it is edited.
func (d *browserDOM) focusErrorField() {
	status := d.target.Call("querySelector", "[data-sv-error-field]")
	if !status.Truthy() {
		return
	}
	name := status.Call("getAttribute", "data-sv-error-field").String()
	if name == "" {
		return
	}
	field := d.target.Call("querySelector", `[name="`+cssAttrEscape(name)+`"]:not([type="hidden"])`)
	if !field.Truthy() {
		return
	}
	field.Call("setAttribute", "aria-invalid", "true")
	field.Call("scrollIntoView", map[string]any{"block": "center"})
	field.Call("focus", map[string]any{"preventScroll": true})
}

// cssAttrEscape escapes a value for a double-quoted CSS attribute selector.
func cssAttrEscape(value string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
}

// resetKeybindingRow puts a keybinding row's field back to its default keys.
// The input event lets the visual layer refresh the row and the save bar.
func resetKeybindingRow(button js.Value) {
	row := button.Call("closest", "[data-sv-kb-row]")
	if !row.Truthy() {
		return
	}
	input := row.Call("querySelector", "[data-sv-kb-input]")
	if !input.Truthy() {
		return
	}
	input.Set("value", input.Call("getAttribute", "data-default-keys").String())
	input.Call("dispatchEvent", js.Global().Get("Event").New("input", map[string]any{"bubbles": true}))
	input.Call("focus")
}

func (d *browserDOM) AppendHistoryTimeline(markup string) error {
	if d == nil || !d.target.Truthy() {
		return fmt.Errorf("DOM mount target #app not found")
	}
	timeline := d.target.Call("querySelector", "[data-sv-history-timeline]")
	if !timeline.Truthy() {
		return fmt.Errorf("history timeline target not found")
	}
	if old := timeline.Call("querySelector", "[data-sv-history-load-more-container]"); old.Truthy() && old.Get("remove").Truthy() {
		old.Call("remove")
	}

	container := d.document.Call("createElement", "div")
	container.Set("innerHTML", markup)
	markEnteringRows(container)
	d.mergeLeadingHistoryGroup(timeline, container)
	for container.Get("firstChild").Truthy() {
		timeline.Call("appendChild", container.Get("firstChild"))
	}
	d.setupHistoryInfiniteScroll()
	return nil
}

const (
	// maxEnterStagger caps the fade-in stagger index of appended rows.
	maxEnterStagger = 8
)

// markEnteringRows tags freshly appended rows so CSS can fade them in with a
// short stagger; prefers-reduced-motion disables the animation in CSS.
func markEnteringRows(container js.Value) {
	rows := container.Call("querySelectorAll", "[data-sv-history-row]")
	for i := 0; i < rows.Get("length").Int(); i++ {
		row := rows.Index(i)
		row.Get("classList").Call("add", "sv-enter")
		row.Get("style").Call("setProperty", "--sv-enter-index", strconv.Itoa(min(i, maxEnterStagger)))
	}
}

func (d *browserDOM) mergeLeadingHistoryGroup(timeline, container js.Value) {
	if !timeline.Truthy() || !container.Truthy() {
		return
	}
	first := container.Get("firstElementChild")
	if !first.Truthy() || !first.Get("hasAttribute").Truthy() || !first.Call("hasAttribute", "data-sv-history-merge-date").Bool() {
		return
	}
	date := strings.TrimSpace(first.Call("getAttribute", "data-sv-history-merge-date").String())
	if date == "" {
		return
	}
	groups := timeline.Call("querySelectorAll", "[data-sv-history-date]")
	if !groups.Truthy() || groups.Get("length").Int() == 0 {
		return
	}
	lastGroup := groups.Index(groups.Get("length").Int() - 1)
	if strings.TrimSpace(lastGroup.Call("getAttribute", "data-sv-history-date").String()) != date {
		return
	}
	list := lastGroup.Call("querySelector", "ul")
	if !list.Truthy() {
		return
	}
	for first.Get("firstChild").Truthy() {
		list.Call("appendChild", first.Get("firstChild"))
	}
	if first.Get("remove").Truthy() {
		first.Call("remove")
	}
}

func (d *browserDOM) updateDocumentTitle() {
	if d == nil || !d.document.Truthy() || !d.target.Truthy() {
		return
	}
	root := d.target.Call("querySelector", "[data-page-title]")
	if !root.Truthy() {
		return
	}
	title := strings.TrimSpace(root.Call("getAttribute", "data-page-title").String())
	if title == "" {
		return
	}
	d.document.Set("title", title)
}

func (d *browserDOM) focusAutofocusTarget() {
	if d == nil || !d.target.Truthy() {
		return
	}
	target := d.target.Call("querySelector", "[data-sv-autofocus]")
	if !target.Truthy() || !target.Get("focus").Truthy() {
		return
	}
	target.Call("focus")
}

func (d *browserDOM) scheduleAlertDismissal() {
	if d == nil || !d.target.Truthy() {
		return
	}
	alerts := d.target.Call("querySelectorAll", ".sv-alert")
	length := alerts.Get("length").Int()
	if length == 0 {
		return
	}
	setTimeout := js.Global().Get("setTimeout")
	if !setTimeout.Truthy() {
		return
	}
	for i := 0; i < length; i++ {
		alert := alerts.Index(i)
		if !alert.Truthy() {
			continue
		}
		scheduleAlertRemoval(setTimeout, alert)
	}
}

const (
	// alertFadeDelayMS is how long an alert stays before fading out.
	alertFadeDelayMS = 4200
	// alertRemoveDelayMS lets the fade-out transition finish before removal.
	alertRemoveDelayMS = 220
)

func scheduleAlertRemoval(setTimeout, alert js.Value) {
	var fadeFn js.Func
	fadeFn = js.FuncOf(func(js.Value, []js.Value) any {
		defer fadeFn.Release()
		if !alert.Truthy() || !alert.Get("classList").Truthy() {
			return nil
		}
		alert.Get("classList").Call("add", "sv-alert-dismiss")

		var removeFn js.Func
		removeFn = js.FuncOf(func(js.Value, []js.Value) any {
			defer removeFn.Release()
			if alert.Truthy() && alert.Get("remove").Truthy() {
				alert.Call("remove")
			}
			return nil
		})
		setTimeout.Invoke(removeFn, alertRemoveDelayMS)
		return nil
	})
	setTimeout.Invoke(fadeFn, alertFadeDelayMS)
}

func (d *browserDOM) BindActions(handler DOMActionHandler) error {
	if d == nil || !d.target.Truthy() || handler == nil {
		return fmt.Errorf("DOM action binding target not available")
	}
	d.Release()
	d.actionHandler = handler

	d.addEventBinding(d.target, "click", d.newClickHandler(handler))
	d.addEventBinding(d.target, "submit", d.newSubmitHandler(handler))
	d.addEventBinding(d.target, "change", newPerformanceProfileChangeHandler())
	d.addEventBinding(d.target, "reset", d.newResetHandler())
	d.addEventBinding(d.target, "input", d.newLiveSearchInputHandler(handler))
	d.addEventBinding(js.Global(), "pagehide", d.newPagehideHandler(handler))
	d.addEventBinding(d.document, "keydown", d.newKeydownHandler())
	return nil
}

// newClickHandler dispatches clicks on [data-sv-action] elements.
func (d *browserDOM) newClickHandler(handler DOMActionHandler) js.Func {
	return js.FuncOf(func(_ js.Value, args []js.Value) any {
		event := firstJSArg(args)
		if !event.Truthy() {
			return nil
		}
		element := closestActionElement(event)
		if !element.Truthy() {
			return nil
		}
		// Forms dispatch only through submit, never on a click in their box.
		if strings.EqualFold(element.Get("tagName").String(), "form") {
			return nil
		}
		action := element.Call("getAttribute", "data-sv-action").String()
		if action == "" {
			return nil
		}
		event.Call("preventDefault")
		d.dispatchClickAction(element, action, handler)
		return nil
	})
}

func (d *browserDOM) dispatchClickAction(element js.Value, action string, handler DOMActionHandler) {
	if action == uiActionConfigResetKeys {
		resetKeybindingRow(element)
		return
	}
	data := collectActionData(element)
	if d.startUndoableAction(element, action, data, handler) {
		return
	}
	if action == historyActionClear {
		// The search field lives outside the swapped regions.
		d.clearHistorySearchInput()
	}
	d.confirmAction(element, func() {
		handler(DOMAction{Action: action, Data: data})
	})
}

// newSubmitHandler dispatches submits of [data-sv-action] forms with their
// field values.
func (d *browserDOM) newSubmitHandler(handler DOMActionHandler) js.Func {
	return js.FuncOf(func(_ js.Value, args []js.Value) any {
		event := firstJSArg(args)
		if !event.Truthy() {
			return nil
		}
		form := event.Get("target")
		if !form.Truthy() {
			return nil
		}
		action := form.Call("getAttribute", "data-sv-action").String()
		if action == "" {
			return nil
		}
		event.Call("preventDefault")
		updatePerformanceCustomInputs(form)
		data := collectActionData(form)
		for key, value := range collectFormData(form) {
			data[key] = value
		}
		d.confirmAction(form, func() {
			handler(DOMAction{Action: action, Data: data})
		})
		return nil
	})
}

// eventTargetMatches returns the event target when it matches selector.
func eventTargetMatches(args []js.Value, selector string) (js.Value, bool) {
	event := firstJSArg(args)
	if !event.Truthy() {
		return js.Undefined(), false
	}
	target := event.Get("target")
	if !target.Truthy() || !target.Get("matches").Truthy() || !target.Call("matches", selector).Bool() {
		return js.Undefined(), false
	}
	return target, true
}

func newPerformanceProfileChangeHandler() js.Func {
	return js.FuncOf(func(_ js.Value, args []js.Value) any {
		target, ok := eventTargetMatches(args, "[data-sv-performance-profile]")
		if !ok {
			return nil
		}
		if form := target.Get("form"); form.Truthy() {
			updatePerformanceCustomInputs(form)
		}
		return nil
	})
}

// newResetHandler handles Discard, which calls form.reset(). Reset restores
// values after the event, so the custom performance inputs are synced on the
// next tick.
func (d *browserDOM) newResetHandler() js.Func {
	return js.FuncOf(func(js.Value, []js.Value) any {
		time.AfterFunc(0, d.syncPerformanceCustomInputStates)
		return nil
	})
}

func (d *browserDOM) newLiveSearchInputHandler(handler DOMActionHandler) js.Func {
	return js.FuncOf(func(_ js.Value, args []js.Value) any {
		target, ok := eventTargetMatches(args, "[data-sv-history-search]")
		if !ok {
			return nil
		}
		d.scheduleLiveSearch(target.Get("value").String(), handler)
		return nil
	})
}

// newPagehideHandler makes pending undoable deletes reach the bridge before
// the page goes away. The flush is queued like any action, then this callback
// yields so the worker goroutine posts the bridge messages before control
// returns to the browser and the page is torn down. Blocking here on anything
// that needs the JS event loop would deadlock, so it only yields.
func (d *browserDOM) newPagehideHandler(handler DOMActionHandler) js.Func {
	return js.FuncOf(func(js.Value, []js.Value) any {
		// Settle the toast first so a bfcache-restored page shows no stale
		// Undo; the flush commits pending deletes either way.
		d.dismissToast()
		handler(DOMAction{Action: actionFlushDeletes})
		for range pagehideYields {
			runtime.Gosched()
		}
		return nil
	})
}

func (d *browserDOM) newKeydownHandler() js.Func {
	return js.FuncOf(func(_ js.Value, args []js.Value) any {
		event := firstJSArg(args)
		if !d.eventTargetInsideMount(event) {
			return nil
		}
		d.handleKeyboard(event)
		return nil
	})
}

func (d *browserDOM) eventTargetInsideMount(event js.Value) bool {
	if d == nil || !d.target.Truthy() || !event.Truthy() {
		return false
	}
	target := event.Get("target")
	if !target.Truthy() {
		target = event.Get("srcElement")
	}
	if !target.Truthy() {
		return false
	}
	if d.document.Truthy() && target.Equal(d.document) {
		return true
	}
	if strings.EqualFold(target.Get("nodeName").String(), "body") {
		return true
	}
	return d.target.Call("contains", target).Bool()
}

func (d *browserDOM) addEventBinding(target js.Value, event string, fn js.Func) {
	if !target.Truthy() {
		fn.Release()
		return
	}
	target.Call("addEventListener", event, fn)
	d.bindings = append(d.bindings, jsEventBinding{target: target, event: event, fn: fn})
}

func (d *browserDOM) setupHistoryInfiniteScroll() {
	if d == nil || !d.target.Truthy() {
		return
	}
	// setupHistoryInfiniteScroll callbacks close over the current load-more
	// button. disconnectHistoryObserver owns the cleanup chain: it calls
	// d.historyObserver.disconnect(), releases d.historyObserverCallback, and
	// lets setupHistoryInfiniteScroll overwrite d.historyObserver with the new
	// observer so old button closures are not retained.
	d.disconnectHistoryObserver()
	observerCtor := js.Global().Get("IntersectionObserver")
	if !observerCtor.Truthy() {
		return
	}
	button := d.target.Call("querySelector", "[data-sv-history-load-more]")
	if !button.Truthy() {
		return
	}
	callback := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 || !button.Truthy() {
			return nil
		}
		entries := args[0]
		for i := 0; i < entries.Get("length").Int(); i++ {
			entry := entries.Index(i)
			if !entry.Truthy() || !entry.Get("isIntersecting").Bool() {
				continue
			}
			if button.Get("disabled").Bool() || button.Get("dataset").Get("svLoading").String() == "true" {
				return nil
			}
			// A disabled button does not dispatch click events. Dispatch first, then
			// mark the control busy so the observer cannot enqueue duplicate loads
			// while the WASM action worker fetches the next history window.
			button.Call("click")
			button.Get("dataset").Set("svLoading", "true")
			button.Set("disabled", true)
			button.Set("textContent", "Loading older visits…")
			return nil
		}
		return nil
	})
	d.historyObserverCallback = callback
	options := js.Global().Get("Object").New()
	options.Set("rootMargin", "400px 0px")
	d.historyObserver = observerCtor.New(callback, options)
	d.historyObserver.Call("observe", button)
}

func (d *browserDOM) disconnectHistoryObserver() {
	if d == nil {
		return
	}
	if d.historyObserver.Truthy() {
		d.historyObserver.Call("disconnect")
		d.historyObserver = js.Value{}
	}
	if d.historyObserverCallback.Truthy() {
		d.historyObserverCallback.Release()
		d.historyObserverCallback = js.Func{}
	}
}

func (d *browserDOM) Release() {
	if d == nil {
		return
	}
	d.disconnectHistoryObserver()
	d.cleanupActiveConfirmDialog()
	d.stopLiveSearch()
	for _, binding := range d.bindings {
		if binding.target.Truthy() {
			binding.target.Call("removeEventListener", binding.event, binding.fn)
		}
		binding.fn.Release()
	}
	d.bindings = nil
}

func firstJSArg(args []js.Value) js.Value {
	if len(args) == 0 {
		return js.Value{}
	}
	return args[0]
}

func closestActionElement(event js.Value) js.Value {
	target := event.Get("target")
	if !target.Truthy() || !target.Get("closest").Truthy() {
		return js.Value{}
	}
	return target.Call("closest", "[data-sv-action]")
}

func collectActionData(element js.Value) map[string]string {
	data := map[string]string{}
	if !element.Truthy() {
		return data
	}
	dataset := element.Get("dataset")
	if !dataset.Truthy() {
		return data
	}
	keys := js.Global().Get("Object").Call("keys", dataset)
	for i := 0; i < keys.Get("length").Int(); i++ {
		key := keys.Index(i).String()
		if key == "svAction" || key == "svConfirm" {
			continue
		}
		data[key] = dataset.Get(key).String()
	}
	return data
}

func (d *browserDOM) syncPerformanceCustomInputStates() {
	if d == nil || !d.target.Truthy() || !d.target.Get("querySelectorAll").Truthy() {
		return
	}
	forms := d.target.Call("querySelectorAll", "[data-sv-performance-form]")
	for i := 0; i < forms.Get("length").Int(); i++ {
		updatePerformanceCustomInputs(forms.Index(i))
	}
}

func updatePerformanceCustomInputs(form js.Value) {
	if !form.Truthy() || !form.Get("querySelector").Truthy() {
		return
	}
	profile := form.Call("querySelector", "[data-sv-performance-profile]:checked")
	if !profile.Truthy() {
		return
	}
	enabled := strings.EqualFold(profile.Get("value").String(), customPerformanceProfile)
	inputs := form.Call("querySelectorAll", "[data-sv-performance-custom]")
	for i := 0; i < inputs.Get("length").Int(); i++ {
		inputs.Index(i).Set("disabled", !enabled)
	}
}

func collectFormData(form js.Value) map[string]string {
	data := map[string]string{}
	if !form.Truthy() || !js.Global().Get("FormData").Truthy() {
		return data
	}
	formData := js.Global().Get("FormData").New(form)
	entries := js.Global().Get("Array").Call("from", formData.Call("entries"))
	for i := 0; i < entries.Get("length").Int(); i++ {
		pair := entries.Index(i)
		if pair.Get("length").Int() < 2 {
			continue
		}
		key := pair.Index(0).String()
		value := pair.Index(1).String()
		if existing := data[key]; existing != "" {
			data[key] = existing + "," + value
			continue
		}
		data[key] = value
	}
	return data
}

func (d *browserDOM) confirmAction(element js.Value, onConfirm func()) {
	if onConfirm == nil {
		return
	}
	confirmAttr := element.Call("getAttribute", "data-sv-confirm")
	if confirmAttr.Type() != js.TypeString {
		onConfirm()
		return
	}
	message := strings.TrimSpace(confirmAttr.String())
	if message == "" {
		onConfirm()
		return
	}
	if !d.showConfirmDialog(message, onConfirm) {
		onConfirm()
	}
}

func (d *browserDOM) cleanupActiveConfirmDialog() {
	if d == nil || d.activeConfirmCleanup == nil {
		return
	}
	cleanup := d.activeConfirmCleanup
	d.activeConfirmCleanup = nil
	cleanup()
}

// inheritAppTheme copies the palette variables and theme/style classes of the
// mounted .sv-app root onto element, which lives outside it under <body>.
func (d *browserDOM) inheritAppTheme(element js.Value) {
	if !d.target.Truthy() {
		return
	}
	root := d.target.Call("querySelector", ".sv-app")
	if !root.Truthy() {
		return
	}
	if style := root.Call("getAttribute", "style"); style.Type() == js.TypeString {
		element.Call("setAttribute", "style", style.String())
	}
	classes := root.Get("classList")
	for i := 0; i < classes.Get("length").Int(); i++ {
		if name := classes.Call("item", i).String(); name != "sv-app" {
			element.Get("classList").Call("add", name)
		}
	}
}

// confirmDialogElements are the nodes of one confirm dialog.
type confirmDialogElements struct {
	overlay       js.Value
	dialog        js.Value
	cancelButton  js.Value
	confirmButton js.Value
}

// appendElement creates a tag element and appends it to parent.
func (d *browserDOM) appendElement(tag string, parent js.Value) js.Value {
	element := d.document.Call("createElement", tag)
	parent.Call("appendChild", element)
	return element
}

func (d *browserDOM) buildConfirmDialog(message string) confirmDialogElements {
	overlay := d.document.Call("createElement", "div")
	overlay.Get("classList").Call("add", "sv-confirm-backdrop")
	overlay.Set("role", "presentation")
	d.inheritAppTheme(overlay)

	dialog := d.appendElement("div", overlay)
	dialog.Get("classList").Call("add", "sv-confirm-dialog")
	dialog.Set("role", "dialog")
	dialog.Call("setAttribute", "aria-modal", "true")
	dialog.Call("setAttribute", "aria-labelledby", "sv-confirm-title")
	dialog.Call("setAttribute", "aria-describedby", "sv-confirm-message")

	title := d.appendElement("h3", dialog)
	title.Set("id", "sv-confirm-title")
	title.Set("textContent", "Confirm action")

	text := d.appendElement("p", dialog)
	text.Set("id", "sv-confirm-message")
	text.Set("textContent", message)

	actions := d.appendElement("div", dialog)
	actions.Get("classList").Call("add", "sv-confirm-actions")

	cancelButton := d.appendElement("button", actions)
	cancelButton.Set("type", "button")
	cancelButton.Set("className", "sv-button sv-button-neutral")
	cancelButton.Set("textContent", "Cancel")

	confirmButton := d.appendElement("button", actions)
	confirmButton.Set("type", "button")
	confirmButton.Set("className", "sv-button sv-button-danger")
	confirmButton.Set("textContent", "Confirm")

	return confirmDialogElements{overlay: overlay, dialog: dialog, cancelButton: cancelButton, confirmButton: confirmButton}
}

// confirmSession is one open confirm dialog: its listeners and the page
// state to restore when it closes.
type confirmSession struct {
	d         *browserDOM
	ui        confirmDialogElements
	id        uint64
	onConfirm func()
	invoker   js.Value
	wasInert  bool

	cancelFn   js.Func
	confirmFn  js.Func
	keyFn      js.Func
	backdropFn js.Func
	cleaned    bool
	confirmed  bool
}

func (s *confirmSession) cleanup() {
	if s.cleaned {
		return
	}
	s.cleaned = true
	d := s.d
	if d.activeConfirmID == s.id {
		d.activeConfirmCleanup = nil
	}
	if s.ui.overlay.Truthy() {
		s.ui.overlay.Call("remove")
	}
	// Lift inert before focusing the invoker, which lives in the page.
	d.target.Set("inert", s.wasInert)
	if d.document.Truthy() && s.keyFn.Truthy() {
		d.document.Call("removeEventListener", "keydown", s.keyFn)
	}
	s.cancelFn.Release()
	s.confirmFn.Release()
	s.keyFn.Release()
	s.backdropFn.Release()
	if s.invoker.Truthy() && s.invoker.Get("isConnected").Bool() {
		s.invoker.Call("focus")
	}
}

func (s *confirmSession) runConfirm() {
	if s.confirmed {
		return
	}
	s.confirmed = true
	s.cleanup()
	s.onConfirm()
}

func (s *confirmSession) handleKey(event js.Value) {
	switch strings.ToLower(event.Get("key").String()) {
	case "tab":
		// aria-modal: keep focus on the dialog's two buttons.
		event.Call("preventDefault")
		if activeElement().Equal(s.ui.confirmButton) {
			s.ui.cancelButton.Call("focus")
		} else {
			s.ui.confirmButton.Call("focus")
		}
	case "escape":
		event.Call("preventDefault")
		event.Call("stopPropagation")
		s.cleanup()
	case "enter":
		if confirmDialogEnterShouldAutoConfirm(s.d.document, s.ui.dialog, s.ui.confirmButton) {
			event.Call("preventDefault")
			event.Call("stopPropagation")
			s.runConfirm()
		}
	}
}

// bind creates the session's js.Funcs and attaches them.
func (s *confirmSession) bind() {
	s.cancelFn = js.FuncOf(func(js.Value, []js.Value) any {
		s.cleanup()
		return nil
	})
	s.confirmFn = js.FuncOf(func(js.Value, []js.Value) any {
		s.runConfirm()
		return nil
	})
	s.keyFn = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if event := firstJSArg(args); event.Truthy() {
			s.handleKey(event)
		}
		return nil
	})
	s.backdropFn = js.FuncOf(func(_ js.Value, args []js.Value) any {
		event := firstJSArg(args)
		if event.Truthy() && event.Get("target").Equal(s.ui.overlay) {
			s.cleanup()
		}
		return nil
	})
	s.ui.cancelButton.Call("addEventListener", "click", s.cancelFn)
	s.ui.confirmButton.Call("addEventListener", "click", s.confirmFn)
	s.d.document.Call("addEventListener", "keydown", s.keyFn)
	s.ui.overlay.Call("addEventListener", "click", s.backdropFn)
}

func (d *browserDOM) showConfirmDialog(message string, onConfirm func()) bool {
	if d == nil || !d.document.Truthy() {
		return false
	}
	body := d.document.Get("body")
	if !body.Truthy() {
		return false
	}
	// Always close an existing modal through its cleanup path so key handlers and
	// js.Func values are released before the replacement is mounted.
	d.cleanupActiveConfirmDialog()

	ui := d.buildConfirmDialog(message)
	d.activeConfirmID++
	session := &confirmSession{
		d:         d,
		ui:        ui,
		id:        d.activeConfirmID,
		onConfirm: onConfirm,
		// Focus goes back to the control that asked, so a re-render after the
		// action can find its place again.
		invoker: activeElement(),
		// The page behind the dialog is inert: assistive tech and pointer or
		// keyboard input cannot reach it while the dialog is open.
		wasInert: d.target.Get("inert").Truthy(),
	}
	session.bind()
	d.activeConfirmCleanup = session.cleanup
	body.Call("appendChild", ui.overlay)
	d.target.Set("inert", true)
	ui.confirmButton.Call("focus")
	return true
}

func confirmDialogEnterShouldAutoConfirm(document, dialog, confirmButton js.Value) bool {
	if !document.Truthy() || !dialog.Truthy() || !confirmButton.Truthy() {
		return false
	}
	active := document.Get("activeElement")
	if !active.Truthy() {
		return true
	}
	if active.Equal(confirmButton) {
		return true
	}
	if dialog.Call("contains", active).Bool() {
		return false
	}
	return true
}

func (d *browserDOM) handleKeyboard(event js.Value) {
	if d == nil || !d.target.Truthy() || !event.Truthy() {
		return
	}
	if d.activeConfirmCleanup != nil {
		// The confirm dialog owns the keyboard while it is open.
		return
	}
	if isPaletteShortcut(event) {
		event.Call("preventDefault")
		if d.actionHandler != nil && !d.paletteOpen {
			d.actionHandler(DOMAction{Action: paletteActionOpen})
		}
		return
	}
	if d.isConfigRoute() {
		d.handleConfigKeyboard(event)
		return
	}
	if d.isHistoryRoute() {
		d.handleHistoryKeyboard(event)
		return
	}
	if d.isFavoritesRoute() {
		d.handleFavoritesKeyboard(event)
	}
}

func (d *browserDOM) handleHistoryKeyboard(event js.Value) {
	if d == nil || !d.target.Truthy() || !event.Truthy() {
		return
	}
	key := strings.ToLower(event.Get("key").String())
	target := event.Get("target")
	if key == "arrowdown" && isHistorySearchInput(target) {
		// Leave the search field for the results, like a command palette.
		event.Call("preventDefault")
		target.Call("blur")
		d.focusHistoryRow(1)
		return
	}
	if key == "escape" && isTextEntry(target) {
		target.Call("blur")
		return
	}
	if !navigableTarget(target) || hasModifier(event) {
		return
	}

	switch key {
	case "/":
		if input := d.target.Call("querySelector", `[data-sv-history-search]`); input.Truthy() {
			event.Call("preventDefault")
			input.Call("focus")
		}
	case "j", "arrowdown":
		event.Call("preventDefault")
		d.focusHistoryRow(1)
	case "k", "arrowup":
		event.Call("preventDefault")
		d.focusHistoryRow(-1)
	case "enter":
		if !isActivatable(target) { // a focused link or button handles Enter natively
			clickInRow(event, d.activeHistoryRow(), ".sv-history-open")
		}
	case "d", "delete", "backspace":
		// Same delegated click path as pointer users, including the
		// data-sv-confirm guard on the delete button.
		clickInRow(event, d.activeHistoryRow(), `[data-sv-action="history.deleteEntry"]`)
	}
}

// clickInRow clicks selector inside row, through the same delegated path
// as pointer users.
func clickInRow(event, row js.Value, selector string) {
	if !row.Truthy() {
		return
	}
	if element := row.Call("querySelector", selector); element.Truthy() {
		event.Call("preventDefault")
		element.Call("click")
	}
}

func (d *browserDOM) isHistoryRoute() bool {
	return d.target.Call("querySelector", `[data-route="history"]`).Truthy()
}

func (d *browserDOM) isConfigRoute() bool {
	return d.target.Call("querySelector", "[data-sv-config-form]").Truthy()
}

// handleConfigKeyboard saves with Ctrl+S and keeps Enter in the keybinding
// filter from submitting the settings form.
func (d *browserDOM) handleConfigKeyboard(event js.Value) {
	key := strings.ToLower(event.Get("key").String())
	ctrl := event.Get("ctrlKey").Bool() || event.Get("metaKey").Bool()
	if ctrl && key == "s" && !event.Get("altKey").Bool() && !event.Get("shiftKey").Bool() {
		event.Call("preventDefault")
		d.submitConfigForm()
		return
	}
	target := event.Get("target")
	if key == "enter" && target.Truthy() && target.Get("matches").Truthy() && target.Call("matches", "[data-sv-keybinding-filter]").Bool() {
		event.Call("preventDefault")
	}
}

func (d *browserDOM) submitConfigForm() {
	form := d.target.Call("querySelector", "[data-sv-config-form]")
	if form.Truthy() && form.Get("requestSubmit").Type() == js.TypeFunction {
		form.Call("requestSubmit")
	}
}

func (d *browserDOM) isFavoritesRoute() bool {
	return d.target.Call("querySelector", `[data-route="favorites"]`).Truthy()
}

func (d *browserDOM) handleFavoritesKeyboard(event js.Value) {
	key := strings.ToLower(event.Get("key").String())
	target := event.Get("target")
	if key == "arrowdown" && target.Truthy() && target.Call("matches", "[data-sv-favorite-filter]").Bool() {
		event.Call("preventDefault")
		d.focusFavoriteRow(1)
		return
	}
	if !navigableTarget(target) || hasModifier(event) {
		return
	}

	switch key {
	case "/":
		if input := d.target.Call("querySelector", "[data-sv-favorite-filter]"); input.Truthy() {
			event.Call("preventDefault")
			input.Call("focus")
			input.Call("select")
		}
	case "n":
		if input := d.target.Call("querySelector", "[data-sv-favorite-add-url]"); input.Truthy() {
			event.Call("preventDefault")
			input.Call("focus")
		}
	case "j", "arrowdown":
		event.Call("preventDefault")
		d.focusFavoriteRow(1)
	case "k", "arrowup":
		event.Call("preventDefault")
		d.focusFavoriteRow(-1)
	case "enter":
		if !isActivatable(target) {
			clickInRow(event, d.activeFavoriteRow(), ".sv-favorite-open")
		}
	case "e":
		clickInRow(event, d.activeFavoriteRow(), "[data-sv-favorite-edit]")
	case "d", "delete", "backspace":
		clickInRow(event, d.activeFavoriteRow(), `[data-sv-action="favorite.delete"]`)
	}
}

// navigableTarget reports whether single-key list shortcuts may run with
// focus on target: not in a text field, and not inside a form, panel or
// popover, where arrows and letters belong to the controls.
func navigableTarget(target js.Value) bool {
	if isTextEntry(target) {
		return false
	}
	if !target.Truthy() || target.Get("closest").Type() != js.TypeFunction {
		return true
	}
	return !target.Call("closest", "form, [popover], [data-sv-favorite-panel], [role=radiogroup]").Truthy()
}

// isTextEntry reports fields where letters are typed rather than shortcuts.
func isTextEntry(target js.Value) bool {
	if !target.Truthy() || target.Get("matches").Type() != js.TypeFunction {
		return false
	}
	return target.Call("matches", textEntrySelector).Bool() || target.Get("isContentEditable").Bool()
}

const textEntrySelector = "textarea, select, " +
	"input:not([type=button]):not([type=submit]):not([type=reset]):not([type=checkbox]):not([type=radio]):not([type=color]):not([type=range])"

// isActivatable reports elements that handle Enter themselves.
func isActivatable(target js.Value) bool {
	return target.Truthy() && target.Get("matches").Type() == js.TypeFunction &&
		target.Call("matches", "a[href], button, summary, [role=button], input").Bool()
}

func hasModifier(event js.Value) bool {
	return event.Get("ctrlKey").Bool() || event.Get("metaKey").Bool() || event.Get("altKey").Bool()
}

func activeElement() js.Value {
	return js.Global().Get("document").Get("activeElement")
}

func isHistorySearchInput(target js.Value) bool {
	return target.Truthy() && target.Get("matches").Type() == js.TypeFunction &&
		target.Call("matches", "[data-sv-history-search]").Bool()
}

// Keyboard row navigation: the active index lives in the mount's dataset.
const (
	historyRowSelector  = "[data-sv-history-row]"
	favoriteRowSelector = "[data-sv-favorite-row]:not([hidden])"
	historyIndexKey     = "svActiveHistoryIndex"
	favoriteIndexKey    = "svActiveFavoriteIndex"
)

// focusRowAt focuses the row at index, clamped to the last row.
func (d *browserDOM) focusRowAt(selector, datasetKey string, index int) {
	d.target.Get("dataset").Set(datasetKey, strconv.Itoa(index-1))
	d.focusRow(selector, datasetKey, 1)
}

func (d *browserDOM) focusHistoryRow(delta int) {
	d.focusRow(historyRowSelector, historyIndexKey, delta)
}

func (d *browserDOM) activeHistoryRow() js.Value {
	return d.activeRow(historyRowSelector, historyIndexKey)
}

func (d *browserDOM) focusFavoriteRow(delta int) {
	d.focusRow(favoriteRowSelector, favoriteIndexKey, delta)
}

func (d *browserDOM) activeFavoriteRow() js.Value {
	return d.activeRow(favoriteRowSelector, favoriteIndexKey)
}

// focusRow moves the highlight by delta and gives real focus to the row's
// main link, so screen readers follow and Tab continues from there.
func (d *browserDOM) focusRow(selector, datasetKey string, delta int) {
	rows := d.target.Call("querySelectorAll", selector)
	length := rows.Get("length").Int()
	if length <= 0 {
		return
	}
	idx := d.currentRowIndex(rows, selector, datasetKey)
	if idx < 0 {
		idx = 0
	} else {
		idx += delta
	}
	idx = max(0, min(idx, length-1))
	if previous := d.target.Call("querySelector", ".sv-focused"); previous.Truthy() {
		previous.Get("classList").Call("remove", "sv-focused")
	}
	row := rows.Index(idx)
	row.Get("classList").Call("add", "sv-focused")
	d.target.Get("dataset").Set(datasetKey, strconv.Itoa(idx))
	if link := row.Call("querySelector", "a.sv-row-title"); link.Truthy() {
		link.Call("focus", map[string]any{"preventScroll": true})
	}
	row.Call("scrollIntoView", map[string]any{"block": "nearest"})
}

// currentRowIndex prefers the row holding focus (the user may have tabbed
// there) over the stored index.
func (d *browserDOM) currentRowIndex(rows js.Value, selector, datasetKey string) int {
	active := activeElement()
	if row := d.rowOf(selector, active); row.Truthy() {
		return nodeIndex(rows, row)
	}
	return d.activeIndex(datasetKey)
}

func (d *browserDOM) rowIndex(selector string, row js.Value) int {
	return nodeIndex(d.target.Call("querySelectorAll", selector), row)
}

// nodeIndex is Array.prototype.indexOf over a NodeList.
func nodeIndex(nodes, node js.Value) int {
	return js.Global().Get("Array").Get("prototype").Get("indexOf").Call("call", nodes, node).Int()
}

// rowOf is the row (matching selector) inside the mount that holds element.
func (d *browserDOM) rowOf(selector string, element js.Value) js.Value {
	if !element.Truthy() || element.Get("closest").Type() != js.TypeFunction {
		return js.Value{}
	}
	row := element.Call("closest", selector)
	if !row.Truthy() || !d.target.Call("contains", row).Bool() {
		return js.Value{}
	}
	return row
}

// activeRow is the row keyboard actions apply to: the row holding focus,
// else (focus on the page itself) the row j/k last highlighted.
func (d *browserDOM) activeRow(selector, datasetKey string) js.Value {
	rows := d.target.Call("querySelectorAll", selector)
	length := rows.Get("length").Int()
	if length <= 0 {
		return js.Value{}
	}
	active := activeElement()
	if active.Truthy() && !active.Equal(d.document.Get("body")) {
		return d.rowOf(selector, active)
	}
	idx := d.activeIndex(datasetKey)
	if idx < 0 || idx >= length {
		return js.Value{}
	}
	return rows.Index(idx)
}

func (d *browserDOM) activeIndex(datasetKey string) int {
	value := d.target.Get("dataset").Get(datasetKey).String()
	idx, err := strconv.Atoi(value)
	if err != nil {
		return -1
	}
	return idx
}
