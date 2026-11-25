//go:build js && wasm

package systemviews

import (
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
	return d.target.Call("querySelector", `[data-sv-region="home"]`).Truthy()
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
	switch {
	case inSearch && key == "arrowdown":
		if first := d.target.Call("querySelector", homeSuggestionSelector); first.Truthy() {
			event.Call("preventDefault")
			first.Call("focus")
		}
	case inSearch && key == "escape":
		target.Call("blur")
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

// handleHomeNavKey moves between rows and tiles outside the suggestions.
func (d *browserDOM) handleHomeNavKey(event, target js.Value, key string) {
	selector := homeNavSelector + ":not(" + homeSuggestionSelector + ")"
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
