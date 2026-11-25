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
		d.homeSelected = 0
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

// handleHomeKeyboard moves focus between suggestions, rows and tiles with
// the arrow keys. Enter on a focused link opens it natively.
func (d *browserDOM) handleHomeKeyboard(event js.Value) {
	key := strings.ToLower(event.Get("key").String())
	target := event.Get("target")
	if hasModifier(event) || !target.Truthy() || target.Get("matches").Type() != js.TypeFunction {
		return
	}
	inSearch := target.Call("matches", homeSearchSelector).Bool()
	inSuggestion := target.Call("matches", homeSuggestionSelector).Bool()
	if inSearch {
		d.handleHomeSearchKey(event, target, key)
		return
	}
	switch {
	case inSuggestion && (key == "arrowdown" || key == "arrowup"):
		event.Call("preventDefault")
		if !d.moveHomeFocus(target, homeSuggestionSelector, key == "arrowdown") && key == "arrowup" {
			if input := d.target.Call("querySelector", homeSearchSelector); input.Truthy() {
				input.Call("focus")
			}
		}
	case inSuggestion && key == "escape":
		if input := d.target.Call("querySelector", homeSearchSelector); input.Truthy() {
			input.Call("focus")
		}
	case target.Call("matches", homeNavSelector).Bool():
		d.handleHomeNavKey(event, target, key)
	}
}

// handleHomeSearchKey handles the keys typed in the search field.
func (d *browserDOM) handleHomeSearchKey(event, input js.Value, key string) {
	switch key {
	case "enter":
		d.openHomeSearch(event, input)
	case "arrowdown":
		if first := d.target.Call("querySelector", homeSuggestionSelector); first.Truthy() {
			event.Call("preventDefault")
			first.Call("focus")
		}
	case "escape":
		input.Call("blur")
	}
}

// openHomeSearch opens what Enter (or Shift+Enter) resolves to. Enter is
// handled in keydown because the submit event has no shiftKey.
func (d *browserDOM) openHomeSearch(event, input js.Value) {
	event.Call("preventDefault")
	view := d.applyHomeSearch(input)
	if url := view.Enter(event.Get("shiftKey").Bool()); url != "" && d.actionHandler != nil {
		d.actionHandler(DOMAction{Action: homeActionSearch, Data: map[string]string{"url": url}})
	}
}

// handleHomeNavKey moves between rows and tiles outside the suggestions.
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

// moveHomeFocus focuses the next or previous element matching selector
// after current, in document order. It reports false at either end.
func (d *browserDOM) moveHomeFocus(current js.Value, selector string, forward bool) bool {
	items := d.target.Call("querySelectorAll", selector)
	count := items.Get("length").Int()
	for i := 0; i < count; i++ {
		if !items.Index(i).Equal(current) {
			continue
		}
		next := i + 1
		if !forward {
			next = i - 1
		}
		if next < 0 || next >= count {
			return false
		}
		items.Index(next).Call("focus")
		return true
	}
	return false
}

// moveHomeGridFocus moves between service tiles in two dimensions: Left and
// Right step by one tile, Up and Down by the rendered column count, so the
// responsive collapse is respected. It reports false when the move leaves
// the grid, letting linear navigation take over.
func (d *browserDOM) moveHomeGridFocus(target js.Value, key string) bool {
	grid := target.Call("closest", ".sv-home-grid")
	if !grid.Truthy() {
		return false
	}
	tiles := grid.Call("querySelectorAll", homeNavSelector)
	count := tiles.Get("length").Int()
	index := -1
	for i := 0; i < count; i++ {
		if tiles.Index(i).Equal(target) {
			index = i
			break
		}
	}
	if index < 0 {
		return false
	}
	columns := renderedGridColumns(grid)
	var next int
	switch key {
	case "arrowleft":
		if index%columns == 0 {
			return true
		}
		next = index - 1
	case "arrowright":
		if index%columns == columns-1 || index+1 >= count {
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
	tiles.Index(next).Call("focus")
	return true
}

// focusAdjacentGrid moves Up or Down into the next or previous service grid,
// keeping the column. It reports false when there is no such grid.
func (d *browserDOM) focusAdjacentGrid(grid js.Value, column int, down bool) bool {
	grids := d.target.Call("querySelectorAll", ".sv-home-grid")
	count := grids.Get("length").Int()
	for i := 0; i < count; i++ {
		if !grids.Index(i).Equal(grid) {
			continue
		}
		next := i + 1
		if !down {
			next = i - 1
		}
		if next < 0 || next >= count {
			return false
		}
		tiles := grids.Index(next).Call("querySelectorAll", homeNavSelector)
		total := tiles.Get("length").Int()
		if total == 0 {
			return false
		}
		columns := renderedGridColumns(grids.Index(next))
		target := min(column, total-1)
		if !down {
			// Last row of the previous grid, same column when it exists.
			lastRow := (total - 1) / columns * columns
			target = min(lastRow+column, total-1)
		}
		tiles.Index(target).Call("focus")
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

// applyHomeSearch resolves the query of input and writes the result to the
// page: hidden and selected attributes on rows, the engine chip and the
// hint. No region is re-rendered, so embeds and the input keep their state.
func (d *browserDOM) applyHomeSearch(input js.Value) homeSearchView {
	form := input.Call("closest", "form")
	if !form.Truthy() {
		return homeSearchView{}
	}
	config := homeSearchConfig{
		EngineURL:  form.Call("getAttribute", "data-sv-home-engine-url").String(),
		EngineName: form.Call("getAttribute", "data-sv-home-engine-name").String(),
		Shortcuts:  parseHomeShortcutsJSON(form.Call("getAttribute", "data-sv-home-shortcuts").String()),
	}
	items, marks := d.homeSearchItems()
	view := buildHomeSearchView(config, input.Get("value").String(), items, d.homeSelected)
	matched := make(map[string]bool, len(view.Result.Matches))
	for _, key := range view.Result.Matches {
		matched[key] = true
	}
	for i, item := range items {
		setBoolAttribute(marks[i], "data-sv-home-hidden", view.Filtering && !matched[item.Key])
		setBoolAttribute(marks[i], "data-sv-home-selected", item.Key == view.Selected)
	}
	if root := d.target.Call("querySelector", ".sv-home"); root.Truthy() {
		setBoolAttribute(root, "data-sv-home-query", view.Filtering)
	}
	setBoolAttribute(form, "data-sv-home-bang", view.HideSuggestions)
	if chip := form.Call("querySelector", "[data-sv-home-engine]"); chip.Truthy() {
		chip.Call("setAttribute", "data-state", view.ChipState)
		chip.Set("innerHTML", mustRenderComponent(HomeSearchChip(view)))
	}
	if hint := form.Call("querySelector", "[data-sv-home-hint]"); hint.Truthy() {
		hint.Set("innerHTML", mustRenderComponent(HomeSearchHint(view)))
	}
	return view
}

func setBoolAttribute(el js.Value, name string, on bool) {
	if on {
		el.Call("setAttribute", name, "")
	} else {
		el.Call("removeAttribute", name)
	}
}
