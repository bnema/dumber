//go:build js && wasm

package systemviews

import (
	"fmt"
	"strconv"
	"strings"
	"syscall/js"
)

type browserDOM struct {
	document                js.Value
	target                  js.Value
	bindings                []jsEventBinding
	historyObserver         js.Value
	historyObserverCallback js.Func
	activeConfirmCleanup    func()
	activeConfirmID         uint64
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
	d.syncPerformanceCustomInputStates()
	d.focusAutofocusTarget()
	d.setupHistoryInfiniteScroll()
	return nil
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
	d.mergeLeadingHistoryGroup(timeline, container)
	for container.Get("firstChild").Truthy() {
		timeline.Call("appendChild", container.Get("firstChild"))
	}
	d.setupHistoryInfiniteScroll()
	return nil
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
		event.Call("preventDefault")
		data := collectActionData(element)
		d.confirmAction(element, func() {
			handler(DOMAction{Action: action, Data: data})
		})
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
	d.addEventBinding(d.target, "submit", submitHandler)

	changeHandler := js.FuncOf(func(_ js.Value, args []js.Value) any {
		event := firstJSArg(args)
		if !event.Truthy() {
			return nil
		}
		target := event.Get("target")
		if !target.Truthy() || !target.Get("matches").Truthy() || !target.Call("matches", "[data-sv-performance-profile]").Bool() {
			return nil
		}
		form := target.Get("form")
		if form.Truthy() {
			updatePerformanceCustomInputs(form)
		}
		return nil
	})
	d.addEventBinding(d.target, "change", changeHandler)

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
	selectEl := form.Call("querySelector", "[data-sv-performance-profile]")
	if !selectEl.Truthy() {
		return
	}
	enabled := strings.EqualFold(selectEl.Get("value").String(), "custom")
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

	overlay := d.document.Call("createElement", "div")
	overlay.Get("classList").Call("add", "sv-confirm-backdrop")
	overlay.Set("role", "presentation")

	dialog := d.document.Call("createElement", "div")
	dialog.Get("classList").Call("add", "sv-confirm-dialog")
	dialog.Set("role", "dialog")
	dialog.Call("setAttribute", "aria-modal", "true")
	dialog.Call("setAttribute", "aria-labelledby", "sv-confirm-title")
	dialog.Call("setAttribute", "aria-describedby", "sv-confirm-message")
	overlay.Call("appendChild", dialog)

	title := d.document.Call("createElement", "h3")
	title.Set("id", "sv-confirm-title")
	title.Set("textContent", "Confirm action")
	dialog.Call("appendChild", title)

	text := d.document.Call("createElement", "p")
	text.Set("id", "sv-confirm-message")
	text.Set("textContent", message)
	dialog.Call("appendChild", text)

	actions := d.document.Call("createElement", "div")
	actions.Get("classList").Call("add", "sv-confirm-actions")
	dialog.Call("appendChild", actions)

	cancelButton := d.document.Call("createElement", "button")
	cancelButton.Set("type", "button")
	cancelButton.Set("className", "sv-button sv-button-secondary")
	cancelButton.Set("textContent", "Cancel")
	actions.Call("appendChild", cancelButton)

	confirmButton := d.document.Call("createElement", "button")
	confirmButton.Set("type", "button")
	confirmButton.Set("className", "sv-button sv-button-danger")
	confirmButton.Set("textContent", "Confirm")
	actions.Call("appendChild", confirmButton)

	d.activeConfirmID++
	confirmID := d.activeConfirmID

	var cleanup func()
	var cancelFn js.Func
	var confirmFn js.Func
	var keyFn js.Func
	var backdropFn js.Func
	cleaned := false
	confirmed := false
	cleanup = func() {
		if cleaned {
			return
		}
		cleaned = true
		if d.activeConfirmID == confirmID {
			d.activeConfirmCleanup = nil
		}
		if overlay.Truthy() {
			overlay.Call("remove")
		}
		if d.document.Truthy() && keyFn.Value.Truthy() {
			d.document.Call("removeEventListener", "keydown", keyFn)
		}
		cancelFn.Release()
		confirmFn.Release()
		keyFn.Release()
		backdropFn.Release()
	}
	runConfirm := func() {
		if confirmed {
			return
		}
		confirmed = true
		cleanup()
		onConfirm()
	}
	cancelFn = js.FuncOf(func(js.Value, []js.Value) any {
		cleanup()
		return nil
	})
	confirmFn = js.FuncOf(func(js.Value, []js.Value) any {
		runConfirm()
		return nil
	})
	keyFn = js.FuncOf(func(_ js.Value, args []js.Value) any {
		event := firstJSArg(args)
		if !event.Truthy() {
			return nil
		}
		switch strings.ToLower(event.Get("key").String()) {
		case "escape":
			event.Call("preventDefault")
			event.Call("stopPropagation")
			cleanup()
		case "enter":
			if confirmDialogEnterShouldAutoConfirm(d.document, dialog, confirmButton) {
				event.Call("preventDefault")
				event.Call("stopPropagation")
				runConfirm()
			}
		}
		return nil
	})
	backdropFn = js.FuncOf(func(_ js.Value, args []js.Value) any {
		event := firstJSArg(args)
		if event.Truthy() && event.Get("target").Equal(overlay) {
			cleanup()
		}
		return nil
	})

	cancelButton.Call("addEventListener", "click", cancelFn)
	confirmButton.Call("addEventListener", "click", confirmFn)
	document := d.document
	document.Call("addEventListener", "keydown", keyFn)
	overlay.Call("addEventListener", "click", backdropFn)
	d.activeConfirmCleanup = cleanup
	body.Call("appendChild", overlay)
	confirmButton.Call("focus")
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
