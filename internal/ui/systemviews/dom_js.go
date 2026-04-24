//go:build js && wasm

package systemviews

import (
	"fmt"
	"strconv"
	"strings"
	"syscall/js"
)

type browserDOM struct {
	document js.Value
	target   js.Value
	bindings []js.Func
}

func NewDOM() DOM {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		return &browserDOM{}
	}

	target := doc.Call("getElementById", "app")
	if !target.Truthy() {
		target = doc.Get("body")
	}

	return &browserDOM{document: doc, target: target}
}

func (d *browserDOM) Mount(html string) error {
	if d == nil || !d.target.Truthy() {
		return fmt.Errorf("DOM target not available")
	}
	d.target.Set("innerHTML", html)
	return nil
}

func (d *browserDOM) BindActions(handler DOMActionHandler) error {
	if d == nil || !d.target.Truthy() || handler == nil {
		return fmt.Errorf("DOM action binding target not available")
	}

	clickHandler := js.FuncOf(func(_ js.Value, args []js.Value) any {
		event := firstJSArg(args)
		if !event.Truthy() {
			return nil
		}
		element := closestActionElement(event)
		if !element.Truthy() {
			return nil
		}
		action := element.Call("getAttribute", "data-sv-action").String()
		if action == "" {
			return nil
		}
		if !confirmAction(element) {
			return nil
		}
		event.Call("preventDefault")
		handler(DOMAction{Action: action, Data: collectActionData(element)})
		return nil
	})
	d.target.Call("addEventListener", "click", clickHandler)
	d.bindings = append(d.bindings, clickHandler)

	submitHandler := js.FuncOf(func(_ js.Value, args []js.Value) any {
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
		data := collectActionData(form)
		for key, value := range collectFormData(form) {
			data[key] = value
		}
		handler(DOMAction{Action: action, Data: data})
		return nil
	})
	d.target.Call("addEventListener", "submit", submitHandler)
	d.bindings = append(d.bindings, submitHandler)

	keydownHandler := js.FuncOf(func(_ js.Value, args []js.Value) any {
		d.handleKeyboard(firstJSArg(args))
		return nil
	})
	d.document.Call("addEventListener", "keydown", keydownHandler)
	d.bindings = append(d.bindings, keydownHandler)

	return nil
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
		data[pair.Index(0).String()] = pair.Index(1).String()
	}
	return data
}

func confirmAction(element js.Value) bool {
	message := element.Call("getAttribute", "data-sv-confirm").String()
	if message == "" {
		return true
	}
	confirm := js.Global().Get("window").Get("confirm")
	if !confirm.Truthy() {
		return true
	}
	return confirm.Invoke(message).Bool()
}

func (d *browserDOM) handleKeyboard(event js.Value) {
	if d == nil || !d.target.Truthy() || !event.Truthy() {
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
	if isEditableTarget(target) && key != "escape" {
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
		if row := d.activeHistoryRow(); row.Truthy() {
			event.Call("preventDefault")
			if link := row.Call("querySelector", ".sv-history-open"); link.Truthy() {
				link.Call("click")
			}
		}
	case "d", "delete", "backspace":
		if row := d.activeHistoryRow(); row.Truthy() {
			event.Call("preventDefault")
			if button := row.Call("querySelector", `[data-sv-action="history.deleteEntry"]`); button.Truthy() {
				button.Call("click")
			}
		}
	case "escape":
		if target.Truthy() && isEditableTarget(target) {
			target.Call("blur")
		}
	}
}

func (d *browserDOM) isHistoryRoute() bool {
	return d.target.Call("querySelector", `[data-route="history"]`).Truthy()
}

func (d *browserDOM) isFavoritesRoute() bool {
	return d.target.Call("querySelector", `[data-route="favorites"]`).Truthy()
}

func (d *browserDOM) handleFavoritesKeyboard(event js.Value) {
	key := strings.ToLower(event.Get("key").String())
	target := event.Get("target")
	if isEditableTarget(target) && key != "escape" {
		return
	}

	switch key {
	case "j", "arrowdown":
		event.Call("preventDefault")
		d.focusFavoriteRow(1)
	case "k", "arrowup":
		event.Call("preventDefault")
		d.focusFavoriteRow(-1)
	case "enter":
		if row := d.activeFavoriteRow(); row.Truthy() {
			event.Call("preventDefault")
			if link := row.Call("querySelector", ".sv-favorite-open"); link.Truthy() {
				link.Call("click")
			}
		}
	case "e":
		if row := d.activeFavoriteRow(); row.Truthy() {
			event.Call("preventDefault")
			if input := row.Call("querySelector", `[name="title"]`); input.Truthy() {
				input.Call("focus")
			}
		}
	case "d", "delete", "backspace":
		if row := d.activeFavoriteRow(); row.Truthy() {
			event.Call("preventDefault")
			if button := row.Call("querySelector", `[data-sv-action="favorite.delete"]`); button.Truthy() {
				button.Call("click")
			}
		}
	case "escape":
		if target.Truthy() && isEditableTarget(target) {
			target.Call("blur")
		}
	}
}

func isEditableTarget(target js.Value) bool {
	if !target.Truthy() {
		return false
	}
	tag := strings.ToLower(target.Get("tagName").String())
	return tag == "input" || tag == "textarea" || tag == "select" || target.Get("isContentEditable").Bool()
}

func (d *browserDOM) focusHistoryRow(delta int) {
	rows := d.target.Call("querySelectorAll", "[data-sv-history-row]")
	length := rows.Get("length").Int()
	if length == 0 {
		return
	}
	idx := d.activeHistoryIndex()
	if idx < 0 {
		idx = 0
	} else {
		idx += delta
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= length {
		idx = length - 1
	}
	for i := 0; i < length; i++ {
		rows.Index(i).Get("classList").Call("remove", "sv-focused")
	}
	row := rows.Index(idx)
	row.Get("classList").Call("add", "sv-focused")
	d.target.Get("dataset").Set("svActiveHistoryIndex", strconv.Itoa(idx))
	row.Call("scrollIntoView", map[string]any{"block": "nearest"})
}

func (d *browserDOM) activeHistoryRow() js.Value {
	rows := d.target.Call("querySelectorAll", "[data-sv-history-row]")
	length := rows.Get("length").Int()
	if length == 0 {
		return js.Value{}
	}
	idx := d.activeHistoryIndex()
	if idx < 0 || idx >= length {
		idx = 0
		d.target.Get("dataset").Set("svActiveHistoryIndex", "0")
		rows.Index(0).Get("classList").Call("add", "sv-focused")
	}
	return rows.Index(idx)
}

func (d *browserDOM) activeHistoryIndex() int {
	value := d.target.Get("dataset").Get("svActiveHistoryIndex").String()
	idx, err := strconv.Atoi(value)
	if err != nil {
		return -1
	}
	return idx
}

func (d *browserDOM) focusFavoriteRow(delta int) {
	rows := d.target.Call("querySelectorAll", "[data-sv-favorite-row]")
	length := rows.Get("length").Int()
	if length == 0 {
		return
	}
	idx := d.activeFavoriteIndex()
	if idx < 0 {
		idx = 0
	} else {
		idx += delta
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= length {
		idx = length - 1
	}
	for i := 0; i < length; i++ {
		rows.Index(i).Get("classList").Call("remove", "sv-focused")
	}
	row := rows.Index(idx)
	row.Get("classList").Call("add", "sv-focused")
	d.target.Get("dataset").Set("svActiveFavoriteIndex", strconv.Itoa(idx))
	row.Call("scrollIntoView", map[string]any{"block": "nearest"})
}

func (d *browserDOM) activeFavoriteRow() js.Value {
	rows := d.target.Call("querySelectorAll", "[data-sv-favorite-row]")
	length := rows.Get("length").Int()
	if length == 0 {
		return js.Value{}
	}
	idx := d.activeFavoriteIndex()
	if idx < 0 || idx >= length {
		idx = 0
		d.target.Get("dataset").Set("svActiveFavoriteIndex", "0")
		rows.Index(0).Get("classList").Call("add", "sv-focused")
	}
	return rows.Index(idx)
}

func (d *browserDOM) activeFavoriteIndex() int {
	value := d.target.Get("dataset").Get("svActiveFavoriteIndex").String()
	idx, err := strconv.Atoi(value)
	if err != nil {
		return -1
	}
	return idx
}
