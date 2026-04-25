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
	bindings []jsEventBinding
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
	return &browserDOM{document: doc, target: target}
}

func (d *browserDOM) Mount(markup string) error {
	if d == nil || !d.target.Truthy() {
		return fmt.Errorf("DOM mount target #app not found")
	}
	d.target.Set("innerHTML", markup)
	d.updateDocumentTitle()
	d.scheduleAlertDismissal()
	return nil
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
		setTimeout.Invoke(removeFn, 220)
		return nil
	})
	setTimeout.Invoke(fadeFn, 4200)
}

func (d *browserDOM) BindActions(handler DOMActionHandler) error {
	if d == nil || !d.target.Truthy() || handler == nil {
		return fmt.Errorf("DOM action binding target not available")
	}
	d.Release()

	clickHandler := js.FuncOf(func(_ js.Value, args []js.Value) any {
		event := firstJSArg(args)
		if !event.Truthy() {
			return nil
		}
		element := closestActionElement(event)
		if !element.Truthy() {
			return nil
		}
		if strings.EqualFold(element.Get("tagName").String(), "form") {
			target := event.Get("target")
			if !target.Truthy() {
				target = event.Get("srcElement")
			}
			if !target.Truthy() || !element.Equal(target) {
				return nil
			}
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
	d.addEventBinding(d.target, "click", clickHandler)

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
		if !confirmAction(form) {
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
	d.addEventBinding(d.target, "submit", submitHandler)

	keydownHandler := js.FuncOf(func(_ js.Value, args []js.Value) any {
		event := firstJSArg(args)
		if !d.eventTargetInsideMount(event) {
			return nil
		}
		d.handleKeyboard(event)
		return nil
	})
	d.addEventBinding(d.document, "keydown", keydownHandler)

	return nil
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

func (d *browserDOM) Release() {
	if d == nil {
		return
	}
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
		// Delete the focused row through the same delegated click path as pointer users,
		// including the data-sv-confirm guard on the delete button.
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
	role := strings.ToLower(target.Get("role").String())
	return tag == "input" || tag == "textarea" || tag == "select" || tag == "button" ||
		(tag == "a" && target.Get("href").String() != "") || role == "button" || role == "link" ||
		target.Get("isContentEditable").Bool()
}

func (d *browserDOM) focusHistoryRow(delta int) {
	d.focusRow("[data-sv-history-row]", "svActiveHistoryIndex", delta)
}

func (d *browserDOM) activeHistoryRow() js.Value {
	return d.activeRow("[data-sv-history-row]", "svActiveHistoryIndex")
}

func (d *browserDOM) focusFavoriteRow(delta int) {
	d.focusRow("[data-sv-favorite-row]", "svActiveFavoriteIndex", delta)
}

func (d *browserDOM) activeFavoriteRow() js.Value {
	return d.activeRow("[data-sv-favorite-row]", "svActiveFavoriteIndex")
}

func (d *browserDOM) focusRow(selector, datasetKey string, delta int) {
	rows := d.target.Call("querySelectorAll", selector)
	length := rows.Get("length").Int()
	if length == 0 {
		return
	}
	idx := d.activeIndex(datasetKey)
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
	d.target.Get("dataset").Set(datasetKey, strconv.Itoa(idx))
	row.Call("scrollIntoView", map[string]any{"block": "nearest"})
}

func (d *browserDOM) activeRow(selector, datasetKey string) js.Value {
	rows := d.target.Call("querySelectorAll", selector)
	length := rows.Get("length").Int()
	if length <= 0 {
		return js.Value{}
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
