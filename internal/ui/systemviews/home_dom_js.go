//go:build js && wasm

package systemviews

import (
	"fmt"
	"strconv"
	"strings"
	"syscall/js"
	"time"

	"bnema.dev/dumber/internal/application/dto"
)

const (
	homeSearchSelector     = "[data-sv-home-search]"
	homeNavSelector        = "[data-sv-home-nav]"
	homeSuggestionSelector = "[data-sv-home-suggestion]"
)

// NavigateTo opens rawURL in the current tab.
func (d *browserDOM) NavigateTo(rawURL string) error {
	js.Global().Get("location").Call("assign", rawURL)
	return nil
}

// bindHomeEvents wires the home-only listeners: the native change event,
// the image fallback, and debounced suggestions.
func (d *browserDOM) bindHomeEvents(handler DOMActionHandler) {
	d.homeSelected = ""
	if d.isHomeRoute() {
		d.bindHomeChangeEvent(handler)
	}
	d.bindHomeImageAndInputEvents(handler)
}

// bindHomeChangeEvent forwards home.toml change notifications. Only the home
// route listens: native notifies every system view pane.
func (d *browserDOM) bindHomeChangeEvent(handler DOMActionHandler) {
	d.addEventBinding(js.Global(), dto.HomeChangedEvent, js.FuncOf(func(_ js.Value, args []js.Value) any {
		event := firstJSArg(args)
		if !event.Truthy() {
			return nil
		}
		topic := ""
		if detail := event.Get("detail"); detail.Truthy() && detail.Get("topic").Type() == js.TypeString {
			topic = detail.Get("topic").String()
		}
		handler(DOMAction{Action: actionSystemviewChanged, Data: map[string]string{"topic": topic}})
		return nil
	}))
}

// bindHomeImageAndInputEvents wires the image fallback and debounced
// suggestions.
func (d *browserDOM) bindHomeImageAndInputEvents(handler DOMActionHandler) {
	// Image errors do not bubble: listen in the capture phase.
	d.addCaptureBinding(d.target, "error", js.FuncOf(func(_ js.Value, args []js.Value) any {
		img, ok := eventTargetMatches(args, "img[data-sv-img-fallback]")
		if !ok {
			return nil
		}
		if parent := img.Get("parentElement"); parent.Truthy() {
			parent.Get("classList").Call("remove", img.Call("getAttribute", "data-sv-img-fallback").String())
		}
		img.Call("remove")
		return nil
	}))
	d.addEventBinding(d.target, "input", js.FuncOf(func(_ js.Value, args []js.Value) any {
		target, ok := eventTargetMatches(args, homeSearchSelector)
		if !ok {
			return nil
		}
		// Filtering runs now, on every keystroke; only history
		// suggestions go through the debounced action queue.
		d.homeSelected = ""
		d.homeQuery = target.Get("value").String()
		d.applyHomeSearch(target)
		d.scheduleHomeSuggest(target.Get("value").String(), handler)
		return nil
	}))
}

func (d *browserDOM) scheduleHomeSuggest(query string, handler DOMActionHandler) {
	d.stopHomeSuggest()
	d.homeSuggestTimer = time.AfterFunc(liveSearchDebounce, func() {
		handler(DOMAction{Action: homeActionSuggest, Data: map[string]string{"query": query}})
	})
}

func (d *browserDOM) stopHomeSuggest() {
	if d.homeSuggestTimer != nil {
		d.homeSuggestTimer.Stop()
		d.homeSuggestTimer = nil
	}
}

func (d *browserDOM) addCaptureBinding(target js.Value, event string, fn js.Func) {
	if !target.Truthy() {
		fn.Release()
		return
	}
	target.Call("addEventListener", event, fn, true)
	d.bindings = append(d.bindings, jsEventBinding{target: target, event: event, fn: fn, capture: true})
}

func (d *browserDOM) isHomeRoute() bool {
	// data-route is on the app root even when the home load failed, so hot
	// reload can still recover the page.
	return d.target.Call("querySelector", `[data-route="home"]`).Truthy()
}

// handleHomeKeyboard is the Home keyboard model. In the search field,
// focus stays put and the keys drive the selection; elsewhere "/" focuses
// the search and the arrows move between rows and group columns.
func (d *browserDOM) handleHomeKeyboard(event js.Value) {
	key := strings.ToLower(event.Get("key").String())
	target := event.Get("target")
	if hasModifier(event) || !target.Truthy() || target.Get("matches").Type() != js.TypeFunction {
		return
	}
	if target.Call("matches", homeSearchSelector).Bool() {
		d.handleHomeSearchKey(event, target, key)
		return
	}
	if homePageKey(key, isTextEntry(target)) == homeKeyFocusSearch {
		if input := d.target.Call("querySelector", homeSearchSelector); input.Truthy() {
			event.Call("preventDefault")
			input.Call("focus")
		}
		return
	}
	if target.Call("matches", homeNavSelector).Bool() {
		d.handleHomeNavKey(event, target, key)
	}
}

// handleHomeSearchKey handles the keys typed in the search field.
func (d *browserDOM) handleHomeSearchKey(event, input js.Value, key string) {
	// Keys that confirm or cancel an IME composition belong to the IME.
	if event.Get("isComposing").Bool() || event.Get("keyCode").Int() == imeProcessKeyCode {
		return
	}
	query := input.Get("value").String()
	switch action := homeSearchKey(key, event.Get("shiftKey").Bool(), query); action {
	case homeKeyOpen, homeKeyOpenWeb:
		// Enter is handled here: the submit event has no shiftKey.
		event.Call("preventDefault")
		view := d.applyHomeSearch(input)
		if url := view.Enter(action == homeKeyOpenWeb); url != "" && d.actionHandler != nil {
			d.actionHandler(DOMAction{Action: homeActionSearch, Data: map[string]string{"url": url}})
		}
	case homeKeySelectNext, homeKeySelectPrev:
		event.Call("preventDefault")
		view := d.applyHomeSearch(input)
		d.homeSelected = view.MoveSelection(action == homeKeySelectNext)
		d.applyHomeSearch(input)
		d.scrollHomeSelection()
	case homeKeyClear:
		event.Call("preventDefault")
		input.Set("value", "")
		d.homeQuery = ""
		d.homeSelected = ""
		d.applyHomeSearch(input)
		if d.actionHandler != nil {
			d.scheduleHomeSuggest("", d.actionHandler)
		}
	case homeKeyBlur:
		event.Call("preventDefault")
		input.Call("blur")
	}
}

// scrollHomeSelection keeps the selected row in view.
func (d *browserDOM) scrollHomeSelection() {
	if row := d.target.Call("querySelector", "[data-sv-home-selected]"); row.Truthy() {
		row.Call("scrollIntoView", map[string]any{"block": "nearest"})
	}
}

// handleHomeNavKey moves between rows and tiles outside the suggestions.
// Rows the search hid are skipped everywhere.
func (d *browserDOM) handleHomeNavKey(event, target js.Value, key string) {
	selector := homeNavSelector + ":not(" + homeSuggestionSelector + ")"
	if d.moveHomeGridFocus(target, key) {
		event.Call("preventDefault")
		return
	}
	switch key {
	case "arrowdown", "arrowright":
		event.Call("preventDefault")
		d.moveHomeFocus(target, selector, true)
	case "arrowup", "arrowleft":
		event.Call("preventDefault")
		d.moveHomeFocus(target, selector, false)
	}
}

// visibleNodes returns the elements of root matching selector that are
// rendered: hidden rows, and rows in hidden groups or sections, have no
// client rects.
func visibleNodes(root js.Value, selector string) []js.Value {
	nodes := root.Call("querySelectorAll", selector)
	count := nodes.Get("length").Int()
	visible := make([]js.Value, 0, count)
	for i := 0; i < count; i++ {
		node := nodes.Index(i)
		if node.Call("getClientRects").Get("length").Int() > 0 {
			visible = append(visible, node)
		}
	}
	return visible
}

func indexOfNode(nodes []js.Value, node js.Value) int {
	for i := range nodes {
		if nodes[i].Equal(node) {
			return i
		}
	}
	return -1
}

// moveHomeFocus focuses the next or previous visible element matching
// selector after current, in document order. It stays put at either end.
func (d *browserDOM) moveHomeFocus(current js.Value, selector string, forward bool) {
	items := visibleNodes(d.target, selector)
	i := indexOfNode(items, current)
	if i < 0 {
		return
	}
	next := i + 1
	if !forward {
		next = i - 1
	}
	if next >= 0 && next < len(items) {
		items[next].Call("focus")
	}
}

// moveHomeGridFocus moves between visible service tiles in two
// dimensions: Left and Right step by one tile, Up and Down by the rendered
// column count, so the responsive collapse is respected. It reports false
// when the move leaves the grid, letting linear navigation take over.
func (d *browserDOM) moveHomeGridFocus(target js.Value, key string) bool {
	grid := target.Call("closest", ".sv-home-grid")
	if !grid.Truthy() {
		return false
	}
	tiles := visibleNodes(grid, homeNavSelector)
	count := len(tiles)
	index := indexOfNode(tiles, target)
	if index < 0 {
		return false
	}
	columns := renderedGridColumns(grid)
	var next int
	switch key {
	case "arrowleft":
		if index%columns == 0 {
			d.focusSideGrid(grid, index/columns, false)
			return true
		}
		next = index - 1
	case "arrowright":
		if index%columns == columns-1 || index+1 >= count {
			d.focusSideGrid(grid, index/columns, true)
			return true
		}
		next = index + 1
	case "arrowup":
		next = index - columns
	case "arrowdown":
		next = index + columns
		if next >= count && index/columns < (count-1)/columns {
			// Short last row: land on its last tile.
			next = count - 1
		}
	default:
		return false
	}
	if next < 0 || next >= count {
		return d.focusAdjacentGrid(grid, index%columns, key == "arrowdown")
	}
	tiles[next].Call("focus")
	return true
}

// homeGridIndex returns the index of grid among the service grids.
func homeGridIndex(grids, grid js.Value) int {
	for i := 0; i < grids.Get("length").Int(); i++ {
		if grids.Index(i).Equal(grid) {
			return i
		}
	}
	return -1
}

// focusSideGrid moves Left or Right into the previous or next group column
// with visible tiles, at the same visible row index (clamped).
func (d *browserDOM) focusSideGrid(grid js.Value, row int, right bool) {
	grids := d.target.Call("querySelectorAll", ".sv-home-grid")
	current := homeGridIndex(grids, grid)
	if current < 0 {
		return
	}
	step := -1
	if right {
		step = 1
	}
	for i := current + step; i >= 0 && i < grids.Get("length").Int(); i += step {
		tiles := visibleNodes(grids.Index(i), homeNavSelector)
		if len(tiles) == 0 {
			continue
		}
		tiles[min(row, len(tiles)-1)].Call("focus")
		return
	}
}

// focusAdjacentGrid moves Up or Down into the next or previous service grid
// with visible tiles, keeping the column. It reports false when there is
// no such grid.
func (d *browserDOM) focusAdjacentGrid(grid js.Value, column int, down bool) bool {
	grids := d.target.Call("querySelectorAll", ".sv-home-grid")
	current := homeGridIndex(grids, grid)
	if current < 0 {
		return false
	}
	step := -1
	if down {
		step = 1
	}
	for i := current + step; i >= 0 && i < grids.Get("length").Int(); i += step {
		next := grids.Index(i)
		tiles := visibleNodes(next, homeNavSelector)
		total := len(tiles)
		if total == 0 {
			continue
		}
		target := min(column, total-1)
		if !down {
			// Last row of the previous grid, same column when it exists.
			columns := renderedGridColumns(next)
			lastRow := (total - 1) / columns * columns
			target = min(lastRow+column, total-1)
		}
		tiles[target].Call("focus")
		return true
	}
	return false
}

// renderedGridColumns counts the tracks of the computed grid-template-columns.
func renderedGridColumns(grid js.Value) int {
	style := js.Global().Call("getComputedStyle", grid)
	if !style.Truthy() {
		return 1
	}
	columns := len(strings.Fields(style.Get("gridTemplateColumns").String()))
	if columns < 1 {
		return 1
	}
	return columns
}

func (d *browserDOM) anchorElement(anchor string) js.Value {
	return d.target.Call("querySelector", `[data-sv-anchor="`+cssQuoted(anchor)+`"]`)
}

// InsertAnchorMarkup inserts markup into the parent anchor, before the child
// anchor before (or at the end), without re-creating the siblings.
func (d *browserDOM) InsertAnchorMarkup(parent, before, markup string) error {
	if d == nil || !d.target.Truthy() {
		return fmt.Errorf("DOM mount target #app not found")
	}
	container := d.anchorElement(parent)
	if !container.Truthy() {
		return fmt.Errorf("systemview anchor %q not found", parent)
	}
	template := d.document.Call("createElement", "template")
	template.Set("innerHTML", markup)
	fragment := template.Get("content")
	next := js.Null()
	if before != "" {
		next = d.anchorElement(before)
		if !next.Truthy() || !next.Get("parentElement").Equal(container) {
			return fmt.Errorf("systemview anchor %q not found in %q", before, parent)
		}
	}
	container.Call("insertBefore", fragment, next)
	return nil
}

// RemoveAnchor removes the element of anchor; a missing anchor is already
// removed.
func (d *browserDOM) RemoveAnchor(anchor string) error {
	if d == nil || !d.target.Truthy() {
		return fmt.Errorf("DOM mount target #app not found")
	}
	if element := d.anchorElement(anchor); element.Truthy() {
		element.Call("remove")
	}
	return nil
}

// SetAnchorStyleProperty sets a CSS custom property on an anchor.
func (d *browserDOM) SetAnchorStyleProperty(anchor, property, value string) error {
	element := d.anchorElement(anchor)
	if !element.Truthy() {
		return fmt.Errorf("systemview anchor %q not found", anchor)
	}
	element.Get("style").Call("setProperty", property, value)
	return nil
}

// homeSearchItems returns the searchable items of the page, services
// first, with the element each one marks: the service row, or the link row.
func (d *browserDOM) homeSearchItems() ([]homeQueryItem, []js.Value) {
	var items []homeQueryItem
	var marks []js.Value
	for _, selector := range []string{`[data-sv-home-item="service"]`, `[data-sv-home-item="row"]`} {
		nodes := d.target.Call("querySelectorAll", selector)
		for i := 0; i < nodes.Get("length").Int(); i++ {
			node := nodes.Index(i)
			mark := node
			if row := node.Call("closest", ".sv-home-service"); row.Truthy() {
				mark = row
			}
			items = append(items, homeQueryItem{
				Key:  strconv.Itoa(len(items)),
				Name: node.Call("getAttribute", "data-sv-home-name").String(),
				Text: node.Call("getAttribute", "data-sv-home-text").String(),
				URL:  node.Call("getAttribute", "data-sv-home-url").String(),
			})
			marks = append(marks, mark)
		}
	}
	return items, marks
}

// homeSuggestionItems returns the history suggestions as selectable items,
// keyed apart from the page items.
func (d *browserDOM) homeSuggestionItems() ([]homeQueryItem, []js.Value) {
	nodes := d.target.Call("querySelectorAll", homeSuggestionSelector)
	count := nodes.Get("length").Int()
	items := make([]homeQueryItem, 0, count)
	marks := make([]js.Value, 0, count)
	for i := 0; i < count; i++ {
		node := nodes.Index(i)
		name := ""
		if title := node.Call("querySelector", ".sv-home-row-title"); title.Truthy() {
			name = title.Get("textContent").String()
		}
		items = append(items, homeQueryItem{Key: "s" + strconv.Itoa(i), Name: name, URL: node.Call("getAttribute", "href").String()})
		marks = append(marks, node)
	}
	return items, marks
}

// applyHomeSearch resolves the query of input and writes the result to the
// page: hidden and selected attributes on rows, the engine chip and the
// hint. No region is re-rendered, so embeds and the input keep their state.
func (d *browserDOM) applyHomeSearch(input js.Value) homeSearchView {
	form := input.Call("closest", "form")
	if !form.Truthy() {
		return homeSearchView{}
	}
	config := d.homeSearchConfigOf(form)
	items, marks := d.homeSearchItems()
	suggestions, suggestionMarks := d.homeSuggestionItems()
	view := buildHomeSearchView(config, input.Get("value").String(), items, suggestions, d.homeSelected)
	matched := make(map[string]bool, len(view.Result.Matches))
	for _, key := range view.Result.Matches {
		matched[key] = true
	}
	for i, item := range items {
		setBoolAttribute(marks[i], "data-sv-home-hidden", view.Filtering && !matched[item.Key])
		setBoolAttribute(marks[i], "data-sv-home-selected", item.Key == view.Selected)
	}
	for i, item := range suggestions {
		setBoolAttribute(suggestionMarks[i], "data-sv-home-selected", item.Key == view.Selected)
	}
	if root := d.target.Call("querySelector", ".sv-home"); root.Truthy() {
		setBoolAttribute(root, "data-sv-home-query", view.Filtering)
	}
	setBoolAttribute(form, "data-sv-home-bang", view.HideSuggestions)
	if chip := form.Call("querySelector", "[data-sv-home-engine]"); chip.Truthy() {
		chip.Call("setAttribute", "data-state", view.ChipState)
		setInnerHTMLIfChanged(chip, mustRenderComponent(HomeSearchChip(view)))
	}
	if hint := form.Call("querySelector", "[data-sv-home-hint]"); hint.Truthy() {
		setInnerHTMLIfChanged(hint, mustRenderComponent(HomeSearchHint(view)))
	}
	return view
}

// imeProcessKeyCode is the keyCode of keys handled by an IME.
const imeProcessKeyCode = 229

// homeSearchConfigOf returns the engine data of the search form, parsed
// once per form element: a remount brings a new form, and a new config.
func (d *browserDOM) homeSearchConfigOf(form js.Value) homeSearchConfig {
	if d.homeForm.Truthy() && d.homeForm.Equal(form) {
		return d.homeConfig
	}
	d.homeForm = form
	d.homeConfig = homeSearchConfig{
		EngineURL:  form.Call("getAttribute", "data-sv-home-engine-url").String(),
		EngineName: form.Call("getAttribute", "data-sv-home-engine-name").String(),
		Shortcuts:  parseHomeShortcutsJSON(form.Call("getAttribute", "data-sv-home-shortcuts").String()),
	}
	return d.homeConfig
}

// setInnerHTMLIfChanged writes markup into el only when it differs from
// the last write there, so images do not reload and aria-live regions do
// not announce the same text again.
func setInnerHTMLIfChanged(el js.Value, markup string) {
	if last := el.Get("svHomeMarkup"); last.Type() == js.TypeString && last.String() == markup {
		return
	}
	el.Set("innerHTML", markup)
	el.Set("svHomeMarkup", markup)
}

func setBoolAttribute(el js.Value, name string, on bool) {
	if on {
		el.Call("setAttribute", name, "")
	} else {
		el.Call("removeAttribute", name)
	}
}

// ReapplyHomeSearch re-applies the typed query after the page changed: new
// rows are filtered, the selection is clamped to the rows left. After a
// remount the input is new, so the last query is typed back into it.
func (d *browserDOM) ReapplyHomeSearch() {
	if d == nil || !d.target.Truthy() {
		return
	}
	input := d.target.Call("querySelector", homeSearchSelector)
	if !input.Truthy() {
		return
	}
	if input.Get("value").String() == "" && d.homeQuery != "" {
		input.Set("value", d.homeQuery)
	}
	d.applyHomeSearch(input)
}
