package browserjs

import (
	"regexp"
	"strings"
	"time"

	"github.com/grafana/sobek"
)

// DOMManager provides DOM constructors and shims.
type DOMManager struct {
	vm    *sobek.Runtime
	tasks chan func()
}

// NewDOMManager creates a new DOM manager.
func NewDOMManager(vm *sobek.Runtime, tasks chan func()) *DOMManager {
	return &DOMManager{vm: vm, tasks: tasks}
}

// InstallConstructors adds DOM class constructors for instanceof checks.
func (dm *DOMManager) InstallConstructors() error {
	vm := dm.vm

	// Base classes
	vm.Set("Node", func(call sobek.ConstructorCall) *sobek.Object { return nil })
	vm.Set("Element", func(call sobek.ConstructorCall) *sobek.Object { return nil })
	vm.Set("HTMLElement", func(call sobek.ConstructorCall) *sobek.Object { return nil })
	vm.Set("HTMLDocument", func(call sobek.ConstructorCall) *sobek.Object { return nil })
	vm.Set("XMLDocument", func(call sobek.ConstructorCall) *sobek.Object { return nil })
	vm.Set("Document", func(call sobek.ConstructorCall) *sobek.Object { return nil })
	vm.Set("DocumentFragment", func(call sobek.ConstructorCall) *sobek.Object { return nil })
	vm.Set("CharacterData", func(call sobek.ConstructorCall) *sobek.Object { return nil })
	vm.Set("Text", func(call sobek.ConstructorCall) *sobek.Object { return nil })
	vm.Set("Comment", func(call sobek.ConstructorCall) *sobek.Object { return nil })

	// HTML Element types
	htmlElements := []string{
		"HTMLDivElement", "HTMLSpanElement", "HTMLScriptElement", "HTMLStyleElement",
		"HTMLLinkElement", "HTMLAnchorElement", "HTMLImageElement", "HTMLInputElement",
		"HTMLButtonElement", "HTMLFormElement", "HTMLTextAreaElement", "HTMLSelectElement",
		"HTMLOptionElement", "HTMLTableElement", "HTMLTableRowElement", "HTMLTableCellElement",
		"HTMLCanvasElement", "HTMLVideoElement", "HTMLAudioElement", "HTMLIFrameElement",
		"HTMLLabelElement", "HTMLFieldSetElement", "HTMLLegendElement", "HTMLUListElement",
		"HTMLOListElement", "HTMLLIElement", "HTMLParagraphElement", "HTMLHeadingElement",
		"HTMLPreElement", "HTMLBRElement", "HTMLHRElement", "HTMLMetaElement",
		"HTMLTitleElement", "HTMLBaseElement", "HTMLBodyElement", "HTMLHeadElement",
		"HTMLHtmlElement", "HTMLTemplateElement", "HTMLSlotElement", "HTMLDataElement",
		"HTMLTimeElement", "HTMLMeterElement", "HTMLProgressElement", "HTMLOutputElement",
		"HTMLDetailsElement", "HTMLSummaryElement", "HTMLDialogElement", "HTMLMenuElement",
		"HTMLPictureElement", "HTMLSourceElement", "HTMLTrackElement", "HTMLEmbedElement",
		"HTMLObjectElement", "HTMLAreaElement", "HTMLMapElement", "HTMLTableSectionElement",
		"HTMLTableColElement", "HTMLTableCaptionElement", "HTMLFrameSetElement",
	}
	for _, name := range htmlElements {
		vm.Set(name, func(call sobek.ConstructorCall) *sobek.Object { return nil })
	}

	// SVG
	vm.Set("SVGElement", func(call sobek.ConstructorCall) *sobek.Object { return nil })
	vm.Set("SVGSVGElement", func(call sobek.ConstructorCall) *sobek.Object { return nil })
	vm.Set("SVGGraphicsElement", func(call sobek.ConstructorCall) *sobek.Object { return nil })

	// DOMParser - provides basic parsing functionality
	vm.Set("DOMParser", func(call sobek.ConstructorCall) *sobek.Object {
		parser := call.This
		_ = parser.Set("parseFromString", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) < 1 {
				return sobek.Null()
			}

			htmlStr := call.Arguments[0].String()
			mimeType := "text/html"
			if len(call.Arguments) > 1 {
				mimeType = call.Arguments[1].String()
			}

			// Create a parsed document object
			doc := dm.createParsedDocument(htmlStr, mimeType)
			return doc
		})
		return nil
	})

	// XMLSerializer
	vm.Set("XMLSerializer", func(call sobek.ConstructorCall) *sobek.Object {
		serializer := call.This
		_ = serializer.Set("serializeToString", func(call sobek.FunctionCall) sobek.Value {
			return vm.ToValue("")
		})
		return nil
	})

	// Blob
	vm.Set("Blob", func(call sobek.ConstructorCall) *sobek.Object {
		var data []byte
		blobType := ""
		if len(call.Arguments) > 0 {
			if arr, ok := call.Arguments[0].Export().([]interface{}); ok {
				for _, item := range arr {
					switch v := item.(type) {
					case string:
						data = append(data, []byte(v)...)
					case []byte:
						data = append(data, v...)
					}
				}
			}
		}
		if len(call.Arguments) > 1 {
			if opts := call.Arguments[1].Export(); opts != nil {
				if m, ok := opts.(map[string]interface{}); ok {
					if t, ok := m["type"].(string); ok {
						blobType = t
					}
				}
			}
		}
		blob := call.This
		_ = blob.Set("size", len(data))
		_ = blob.Set("type", blobType)
		_ = blob.Set("text", func(sobek.FunctionCall) sobek.Value {
			p, res, _ := vm.NewPromise()
			go func() {
				if dm.tasks != nil {
					dm.tasks <- func() { _ = res(vm.ToValue(string(data))) }
				} else {
					_ = res(vm.ToValue(string(data)))
				}
			}()
			return vm.ToValue(p)
		})
		_ = blob.Set("arrayBuffer", func(sobek.FunctionCall) sobek.Value {
			p, res, _ := vm.NewPromise()
			go func() {
				if dm.tasks != nil {
					dm.tasks <- func() { _ = res(vm.ToValue(data)) }
				} else {
					_ = res(vm.ToValue(data))
				}
			}()
			return vm.ToValue(p)
		})
		_ = blob.Set("slice", func(call sobek.FunctionCall) sobek.Value {
			start := 0
			end := len(data)
			if len(call.Arguments) > 0 {
				start = int(call.Arguments[0].ToInteger())
			}
			if len(call.Arguments) > 1 {
				end = int(call.Arguments[1].ToInteger())
			}
			sliced := data[start:end]
			newBlob := vm.NewObject()
			_ = newBlob.Set("size", len(sliced))
			_ = newBlob.Set("type", blobType)
			return newBlob
		})
		return nil
	})

	// File
	vm.Set("File", func(call sobek.ConstructorCall) *sobek.Object {
		var data []byte
		fileName := ""
		fileType := ""
		if len(call.Arguments) > 0 {
			if arr, ok := call.Arguments[0].Export().([]interface{}); ok {
				for _, item := range arr {
					switch v := item.(type) {
					case string:
						data = append(data, []byte(v)...)
					case []byte:
						data = append(data, v...)
					}
				}
			}
		}
		if len(call.Arguments) > 1 {
			fileName = call.Arguments[1].String()
		}
		if len(call.Arguments) > 2 {
			if opts := call.Arguments[2].Export(); opts != nil {
				if m, ok := opts.(map[string]interface{}); ok {
					if t, ok := m["type"].(string); ok {
						fileType = t
					}
				}
			}
		}
		file := call.This
		_ = file.Set("name", fileName)
		_ = file.Set("size", len(data))
		_ = file.Set("type", fileType)
		_ = file.Set("lastModified", time.Now().UnixMilli())
		return nil
	})

	// FileReader
	vm.Set("FileReader", func(call sobek.ConstructorCall) *sobek.Object {
		reader := call.This
		_ = reader.Set("result", sobek.Null())
		_ = reader.Set("readyState", 0)
		_ = reader.Set("error", sobek.Null())
		_ = reader.Set("onload", sobek.Null())
		_ = reader.Set("onerror", sobek.Null())
		_ = reader.Set("onloadend", sobek.Null())
		_ = reader.Set("readAsText", func(call sobek.FunctionCall) sobek.Value {
			// Simplified - just set result
			if len(call.Arguments) > 0 {
				_ = reader.Set("readyState", 2)
				_ = reader.Set("result", "")
			}
			return sobek.Undefined()
		})
		_ = reader.Set("readAsDataURL", func(sobek.FunctionCall) sobek.Value {
			_ = reader.Set("readyState", 2)
			_ = reader.Set("result", "data:;base64,")
			return sobek.Undefined()
		})
		_ = reader.Set("readAsArrayBuffer", func(sobek.FunctionCall) sobek.Value {
			_ = reader.Set("readyState", 2)
			_ = reader.Set("result", []byte{})
			return sobek.Undefined()
		})
		_ = reader.Set("abort", func(sobek.FunctionCall) sobek.Value {
			return sobek.Undefined()
		})
		return nil
	})

	// AbortController & AbortSignal
	vm.Set("AbortController", func(call sobek.ConstructorCall) *sobek.Object {
		aborted := false
		var reason sobek.Value = sobek.Undefined()

		// Store event listeners
		type listenerEntry struct {
			callback sobek.Callable
			value    sobek.Value
		}
		abortListeners := make([]listenerEntry, 0)
		var onabort sobek.Callable

		signal := vm.NewObject()
		_ = signal.Set("aborted", false)
		_ = signal.Set("reason", sobek.Undefined())

		// throwIfAborted throws if signal is aborted
		_ = signal.Set("throwIfAborted", func(call sobek.FunctionCall) sobek.Value {
			if aborted {
				panic(reason)
			}
			return sobek.Undefined()
		})

		_ = signal.Set("addEventListener", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 2 {
				eventType := call.Arguments[0].String()
				if eventType == "abort" {
					if cb, ok := sobek.AssertFunction(call.Arguments[1]); ok {
						// Check for duplicates
						listenerVal := call.Arguments[1]
						for _, entry := range abortListeners {
							if entry.value.SameAs(listenerVal) {
								return sobek.Undefined()
							}
						}
						abortListeners = append(abortListeners, listenerEntry{callback: cb, value: listenerVal})
					}
				}
			}
			return sobek.Undefined()
		})

		_ = signal.Set("removeEventListener", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 2 {
				eventType := call.Arguments[0].String()
				if eventType == "abort" {
					listenerVal := call.Arguments[1]
					for i, entry := range abortListeners {
						if entry.value.SameAs(listenerVal) {
							abortListeners = append(abortListeners[:i], abortListeners[i+1:]...)
							break
						}
					}
				}
			}
			return sobek.Undefined()
		})

		_ = signal.Set("dispatchEvent", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				evt := call.Arguments[0].ToObject(vm)
				eventType := evt.Get("type").String()
				if eventType == "abort" {
					// Copy listeners to avoid modification during iteration
					listenersCopy := make([]listenerEntry, len(abortListeners))
					copy(listenersCopy, abortListeners)
					for _, entry := range listenersCopy {
						_, _ = entry.callback(signal, evt)
					}
				}
			}
			return vm.ToValue(true)
		})

		// onabort property accessor
		_ = signal.DefineAccessorProperty("onabort",
			vm.ToValue(func(sobek.FunctionCall) sobek.Value {
				if onabort == nil {
					return sobek.Null()
				}
				return vm.ToValue(onabort)
			}),
			vm.ToValue(func(call sobek.FunctionCall) sobek.Value {
				if len(call.Arguments) > 0 {
					if cb, ok := sobek.AssertFunction(call.Arguments[0]); ok {
						onabort = cb
					} else if call.Arguments[0] == sobek.Null() || sobek.IsUndefined(call.Arguments[0]) {
						onabort = nil
					}
				}
				return sobek.Undefined()
			}),
			sobek.FLAG_FALSE, sobek.FLAG_TRUE)

		controller := call.This
		_ = controller.Set("signal", signal)
		_ = controller.Set("abort", func(call sobek.FunctionCall) sobek.Value {
			if !aborted {
				aborted = true
				_ = signal.Set("aborted", true)

				// Set reason
				if len(call.Arguments) > 0 && !sobek.IsUndefined(call.Arguments[0]) {
					reason = call.Arguments[0]
				} else {
					// Create default DOMException for AbortError
					exc := vm.NewObject()
					_ = exc.Set("name", "AbortError")
					_ = exc.Set("message", "The operation was aborted.")
					_ = exc.Set("code", 20) // ABORT_ERR
					reason = exc
				}
				_ = signal.Set("reason", reason)

				// Create abort event
				evt := vm.NewObject()
				_ = evt.Set("type", "abort")
				_ = evt.Set("target", signal)
				_ = evt.Set("currentTarget", signal)
				_ = evt.Set("bubbles", false)
				_ = evt.Set("cancelable", false)
				_ = evt.Set("defaultPrevented", false)
				_ = evt.Set("isTrusted", true)
				_ = evt.Set("timeStamp", float64(time.Now().UnixMilli()))

				// Fire onabort handler
				if onabort != nil {
					_, _ = onabort(signal, evt)
				}

				// Fire all abort event listeners
				listenersCopy := make([]listenerEntry, len(abortListeners))
				copy(listenersCopy, abortListeners)
				for _, entry := range listenersCopy {
					_, _ = entry.callback(signal, evt)
				}
			}
			return sobek.Undefined()
		})
		return nil
	})

	// AbortSignal static methods and constructor
	abortSignalStatic := vm.NewObject()

	// AbortSignal.abort() - creates an already-aborted signal
	_ = abortSignalStatic.Set("abort", func(call sobek.FunctionCall) sobek.Value {
		signal := vm.NewObject()
		_ = signal.Set("aborted", true)

		var reason sobek.Value
		if len(call.Arguments) > 0 && !sobek.IsUndefined(call.Arguments[0]) {
			reason = call.Arguments[0]
		} else {
			exc := vm.NewObject()
			_ = exc.Set("name", "AbortError")
			_ = exc.Set("message", "The operation was aborted.")
			_ = exc.Set("code", 20)
			reason = exc
		}
		_ = signal.Set("reason", reason)

		_ = signal.Set("throwIfAborted", func(call sobek.FunctionCall) sobek.Value {
			panic(reason)
		})

		_ = signal.Set("addEventListener", func(sobek.FunctionCall) sobek.Value {
			return sobek.Undefined()
		})
		_ = signal.Set("removeEventListener", func(sobek.FunctionCall) sobek.Value {
			return sobek.Undefined()
		})
		_ = signal.Set("onabort", sobek.Null())

		return signal
	})

	// AbortSignal.timeout() - creates a signal that aborts after timeout
	_ = abortSignalStatic.Set("timeout", func(call sobek.FunctionCall) sobek.Value {
		ms := int64(0)
		if len(call.Arguments) > 0 {
			ms = call.Arguments[0].ToInteger()
		}

		aborted := false
		var reason sobek.Value = sobek.Undefined()
		type listenerEntry struct {
			callback sobek.Callable
			value    sobek.Value
		}
		abortListeners := make([]listenerEntry, 0)
		var onabort sobek.Callable

		signal := vm.NewObject()
		_ = signal.Set("aborted", false)
		_ = signal.Set("reason", sobek.Undefined())

		_ = signal.Set("throwIfAborted", func(call sobek.FunctionCall) sobek.Value {
			if aborted {
				panic(reason)
			}
			return sobek.Undefined()
		})

		_ = signal.Set("addEventListener", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 2 {
				eventType := call.Arguments[0].String()
				if eventType == "abort" {
					if cb, ok := sobek.AssertFunction(call.Arguments[1]); ok {
						listenerVal := call.Arguments[1]
						for _, entry := range abortListeners {
							if entry.value.SameAs(listenerVal) {
								return sobek.Undefined()
							}
						}
						abortListeners = append(abortListeners, listenerEntry{callback: cb, value: listenerVal})
					}
				}
			}
			return sobek.Undefined()
		})

		_ = signal.Set("removeEventListener", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 2 {
				eventType := call.Arguments[0].String()
				if eventType == "abort" {
					listenerVal := call.Arguments[1]
					for i, entry := range abortListeners {
						if entry.value.SameAs(listenerVal) {
							abortListeners = append(abortListeners[:i], abortListeners[i+1:]...)
							break
						}
					}
				}
			}
			return sobek.Undefined()
		})

		_ = signal.DefineAccessorProperty("onabort",
			vm.ToValue(func(sobek.FunctionCall) sobek.Value {
				if onabort == nil {
					return sobek.Null()
				}
				return vm.ToValue(onabort)
			}),
			vm.ToValue(func(call sobek.FunctionCall) sobek.Value {
				if len(call.Arguments) > 0 {
					if cb, ok := sobek.AssertFunction(call.Arguments[0]); ok {
						onabort = cb
					} else {
						onabort = nil
					}
				}
				return sobek.Undefined()
			}),
			sobek.FLAG_FALSE, sobek.FLAG_TRUE)

		// Set up timeout - use tasks channel if available
		go func() {
			time.Sleep(time.Duration(ms) * time.Millisecond)
			if aborted {
				return
			}

			triggerAbort := func() {
				if aborted {
					return
				}
				aborted = true
				_ = signal.Set("aborted", true)

				// TimeoutError for timeout signal
				exc := vm.NewObject()
				_ = exc.Set("name", "TimeoutError")
				_ = exc.Set("message", "The operation timed out.")
				_ = exc.Set("code", 23) // TIMEOUT_ERR
				reason = exc
				_ = signal.Set("reason", reason)

				// Create abort event
				evt := vm.NewObject()
				_ = evt.Set("type", "abort")
				_ = evt.Set("target", signal)
				_ = evt.Set("currentTarget", signal)
				_ = evt.Set("bubbles", false)
				_ = evt.Set("cancelable", false)
				_ = evt.Set("isTrusted", true)
				_ = evt.Set("timeStamp", float64(time.Now().UnixMilli()))

				if onabort != nil {
					_, _ = onabort(signal, evt)
				}

				for _, entry := range abortListeners {
					_, _ = entry.callback(signal, evt)
				}
			}

			if dm.tasks != nil {
				dm.tasks <- triggerAbort
			} else {
				triggerAbort()
			}
		}()

		return signal
	})

	// AbortSignal.any() - creates a signal that aborts when any of the given signals abort
	_ = abortSignalStatic.Set("any", func(call sobek.FunctionCall) sobek.Value {
		// Create a new signal that tracks multiple sources
		aborted := false
		var reason sobek.Value = sobek.Undefined()
		type listenerEntry struct {
			callback sobek.Callable
			value    sobek.Value
		}
		abortListeners := make([]listenerEntry, 0)
		var onabort sobek.Callable

		signal := vm.NewObject()
		_ = signal.Set("aborted", false)
		_ = signal.Set("reason", sobek.Undefined())

		_ = signal.Set("throwIfAborted", func(call sobek.FunctionCall) sobek.Value {
			if aborted {
				panic(reason)
			}
			return sobek.Undefined()
		})

		_ = signal.Set("addEventListener", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 2 {
				eventType := call.Arguments[0].String()
				if eventType == "abort" {
					if cb, ok := sobek.AssertFunction(call.Arguments[1]); ok {
						listenerVal := call.Arguments[1]
						for _, entry := range abortListeners {
							if entry.value.SameAs(listenerVal) {
								return sobek.Undefined()
							}
						}
						abortListeners = append(abortListeners, listenerEntry{callback: cb, value: listenerVal})
					}
				}
			}
			return sobek.Undefined()
		})

		_ = signal.Set("removeEventListener", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 2 {
				eventType := call.Arguments[0].String()
				if eventType == "abort" {
					listenerVal := call.Arguments[1]
					for i, entry := range abortListeners {
						if entry.value.SameAs(listenerVal) {
							abortListeners = append(abortListeners[:i], abortListeners[i+1:]...)
							break
						}
					}
				}
			}
			return sobek.Undefined()
		})

		_ = signal.DefineAccessorProperty("onabort",
			vm.ToValue(func(sobek.FunctionCall) sobek.Value {
				if onabort == nil {
					return sobek.Null()
				}
				return vm.ToValue(onabort)
			}),
			vm.ToValue(func(call sobek.FunctionCall) sobek.Value {
				if len(call.Arguments) > 0 {
					if cb, ok := sobek.AssertFunction(call.Arguments[0]); ok {
						onabort = cb
					} else {
						onabort = nil
					}
				}
				return sobek.Undefined()
			}),
			sobek.FLAG_FALSE, sobek.FLAG_TRUE)

		// Process input signals
		if len(call.Arguments) > 0 {
			signalsArray := call.Arguments[0].Export()
			if signals, ok := signalsArray.([]interface{}); ok {
				for _, sigRaw := range signals {
					if sigVal, ok := sigRaw.(*sobek.Object); ok {
						// Check if already aborted
						abortedVal := sigVal.Get("aborted")
						if abortedVal != nil && abortedVal.ToBoolean() {
							aborted = true
							_ = signal.Set("aborted", true)
							reasonVal := sigVal.Get("reason")
							if reasonVal != nil && !sobek.IsUndefined(reasonVal) {
								reason = reasonVal
							} else {
								exc := vm.NewObject()
								_ = exc.Set("name", "AbortError")
								_ = exc.Set("message", "The operation was aborted.")
								_ = exc.Set("code", 20)
								reason = exc
							}
							_ = signal.Set("reason", reason)
							return signal
						}

						// Add listener to each source signal
						addListenerVal := sigVal.Get("addEventListener")
						if addListener, ok := sobek.AssertFunction(addListenerVal); ok {
							handler := vm.ToValue(func(call sobek.FunctionCall) sobek.Value {
								if aborted {
									return sobek.Undefined()
								}
								aborted = true
								_ = signal.Set("aborted", true)

								// Get reason from source signal
								if len(call.Arguments) > 0 {
									evt := call.Arguments[0].ToObject(vm)
									target := evt.Get("target")
									if target != nil {
										targetObj := target.ToObject(vm)
										reasonVal := targetObj.Get("reason")
										if reasonVal != nil && !sobek.IsUndefined(reasonVal) {
											reason = reasonVal
										}
									}
								}
								if sobek.IsUndefined(reason) {
									exc := vm.NewObject()
									_ = exc.Set("name", "AbortError")
									_ = exc.Set("message", "The operation was aborted.")
									_ = exc.Set("code", 20)
									reason = exc
								}
								_ = signal.Set("reason", reason)

								// Fire abort event
								evt := vm.NewObject()
								_ = evt.Set("type", "abort")
								_ = evt.Set("target", signal)
								_ = evt.Set("currentTarget", signal)
								_ = evt.Set("bubbles", false)
								_ = evt.Set("cancelable", false)
								_ = evt.Set("isTrusted", true)
								_ = evt.Set("timeStamp", float64(time.Now().UnixMilli()))

								if onabort != nil {
									_, _ = onabort(signal, evt)
								}
								for _, entry := range abortListeners {
									_, _ = entry.callback(signal, evt)
								}

								return sobek.Undefined()
							})
							_, _ = addListener(sigVal, vm.ToValue("abort"), handler)
						}
					}
				}
			}
		}

		return signal
	})

	vm.Set("AbortSignal", abortSignalStatic)

	// FormData
	vm.Set("FormData", func(call sobek.ConstructorCall) *sobek.Object {
		entries := make([][2]string, 0)
		fd := call.This
		_ = fd.Set("append", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 2 {
				entries = append(entries, [2]string{
					call.Arguments[0].String(),
					call.Arguments[1].String(),
				})
			}
			return sobek.Undefined()
		})
		_ = fd.Set("get", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				name := call.Arguments[0].String()
				for _, e := range entries {
					if e[0] == name {
						return vm.ToValue(e[1])
					}
				}
			}
			return sobek.Null()
		})
		_ = fd.Set("getAll", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				name := call.Arguments[0].String()
				var values []string
				for _, e := range entries {
					if e[0] == name {
						values = append(values, e[1])
					}
				}
				return vm.ToValue(values)
			}
			return vm.ToValue([]string{})
		})
		_ = fd.Set("has", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				name := call.Arguments[0].String()
				for _, e := range entries {
					if e[0] == name {
						return vm.ToValue(true)
					}
				}
			}
			return vm.ToValue(false)
		})
		_ = fd.Set("delete", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				name := call.Arguments[0].String()
				newEntries := make([][2]string, 0)
				for _, e := range entries {
					if e[0] != name {
						newEntries = append(newEntries, e)
					}
				}
				entries = newEntries
			}
			return sobek.Undefined()
		})
		_ = fd.Set("set", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 2 {
				name := call.Arguments[0].String()
				value := call.Arguments[1].String()
				found := false
				for i, e := range entries {
					if e[0] == name {
						entries[i][1] = value
						found = true
						break
					}
				}
				if !found {
					entries = append(entries, [2]string{name, value})
				}
			}
			return sobek.Undefined()
		})
		return nil
	})

	// Image constructor (creates HTMLImageElement)
	vm.Set("Image", func(call sobek.ConstructorCall) *sobek.Object {
		img := call.This
		_ = img.Set("src", "")
		_ = img.Set("width", 0)
		_ = img.Set("height", 0)
		_ = img.Set("complete", false)
		_ = img.Set("naturalWidth", 0)
		_ = img.Set("naturalHeight", 0)
		_ = img.Set("onload", sobek.Null())
		_ = img.Set("onerror", sobek.Null())
		if len(call.Arguments) > 0 {
			_ = img.Set("width", call.Arguments[0].ToInteger())
		}
		if len(call.Arguments) > 1 {
			_ = img.Set("height", call.Arguments[1].ToInteger())
		}
		return nil
	})

	// Audio constructor
	vm.Set("Audio", func(call sobek.ConstructorCall) *sobek.Object {
		audio := call.This
		_ = audio.Set("src", "")
		_ = audio.Set("paused", true)
		_ = audio.Set("volume", 1.0)
		_ = audio.Set("muted", false)
		_ = audio.Set("currentTime", 0)
		_ = audio.Set("duration", 0)
		_ = audio.Set("play", func(sobek.FunctionCall) sobek.Value {
			p, res, _ := vm.NewPromise()
			go func() {
				if dm.tasks != nil {
					dm.tasks <- func() { _ = res(sobek.Undefined()) }
				} else {
					_ = res(sobek.Undefined())
				}
			}()
			return vm.ToValue(p)
		})
		_ = audio.Set("pause", func(sobek.FunctionCall) sobek.Value {
			return sobek.Undefined()
		})
		_ = audio.Set("load", func(sobek.FunctionCall) sobek.Value {
			return sobek.Undefined()
		})
		if len(call.Arguments) > 0 {
			_ = audio.Set("src", call.Arguments[0].String())
		}
		return nil
	})

	return nil
}

// parsedElement represents a simplified DOM element from parsed HTML
type parsedElement struct {
	tagName    string
	id         string
	classes    []string
	attributes map[string]string
	textContent string
	innerHTML  string
	children   []*parsedElement
	parent     *parsedElement
}

// createParsedDocument creates a document object from parsed HTML/XML
func (dm *DOMManager) createParsedDocument(content, mimeType string) sobek.Value {
	vm := dm.vm

	// Parse the HTML content into a simple structure
	root := dm.parseHTML(content)

	// Create the document object
	doc := vm.NewObject()

	// Set document properties
	_ = doc.Set("nodeType", 9) // DOCUMENT_NODE
	_ = doc.Set("nodeName", "#document")

	// documentElement
	docElement := dm.createElementObject(root)
	_ = doc.Set("documentElement", docElement)

	// body - find or create
	bodyElem := dm.findElement(root, "body")
	if bodyElem != nil {
		_ = doc.Set("body", dm.createElementObject(bodyElem))
	} else {
		_ = doc.Set("body", sobek.Null())
	}

	// head - find or create
	headElem := dm.findElement(root, "head")
	if headElem != nil {
		_ = doc.Set("head", dm.createElementObject(headElem))
	} else {
		_ = doc.Set("head", sobek.Null())
	}

	// querySelector
	_ = doc.Set("querySelector", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return sobek.Null()
		}
		selector := call.Arguments[0].String()
		found := dm.querySelector(root, selector)
		if found != nil {
			return dm.createElementObject(found)
		}
		return sobek.Null()
	})

	// querySelectorAll
	_ = doc.Set("querySelectorAll", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return vm.ToValue([]interface{}{})
		}
		selector := call.Arguments[0].String()
		found := dm.querySelectorAll(root, selector)
		result := make([]interface{}, len(found))
		for i, elem := range found {
			result[i] = dm.createElementObject(elem)
		}
		return vm.ToValue(result)
	})

	// getElementById
	_ = doc.Set("getElementById", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return sobek.Null()
		}
		id := call.Arguments[0].String()
		found := dm.findElementByID(root, id)
		if found != nil {
			return dm.createElementObject(found)
		}
		return sobek.Null()
	})

	// getElementsByTagName
	_ = doc.Set("getElementsByTagName", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return vm.ToValue([]interface{}{})
		}
		tagName := strings.ToLower(call.Arguments[0].String())
		found := dm.findElementsByTag(root, tagName)
		result := make([]interface{}, len(found))
		for i, elem := range found {
			result[i] = dm.createElementObject(elem)
		}
		return vm.ToValue(result)
	})

	// getElementsByClassName
	_ = doc.Set("getElementsByClassName", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return vm.ToValue([]interface{}{})
		}
		className := call.Arguments[0].String()
		found := dm.findElementsByClass(root, className)
		result := make([]interface{}, len(found))
		for i, elem := range found {
			result[i] = dm.createElementObject(elem)
		}
		return vm.ToValue(result)
	})

	// createElement (stub)
	_ = doc.Set("createElement", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return sobek.Null()
		}
		tagName := call.Arguments[0].String()
		elem := &parsedElement{
			tagName:    strings.ToLower(tagName),
			attributes: make(map[string]string),
			children:   make([]*parsedElement, 0),
		}
		return dm.createElementObject(elem)
	})

	// createTextNode (stub)
	_ = doc.Set("createTextNode", func(call sobek.FunctionCall) sobek.Value {
		text := ""
		if len(call.Arguments) > 0 {
			text = call.Arguments[0].String()
		}
		node := vm.NewObject()
		_ = node.Set("nodeType", 3) // TEXT_NODE
		_ = node.Set("textContent", text)
		_ = node.Set("nodeValue", text)
		return node
	})

	return doc
}

// parseHTML parses HTML string into a simple element tree
func (dm *DOMManager) parseHTML(html string) *parsedElement {
	root := &parsedElement{
		tagName:    "html",
		attributes: make(map[string]string),
		children:   make([]*parsedElement, 0),
	}

	// Simple tag regex - matches opening/closing tags and content
	tagRegex := regexp.MustCompile(`<\s*(/?)(\w+)([^>]*)>`)
	attrRegex := regexp.MustCompile(`(\w+)\s*=\s*["']([^"']*)["']`)

	var stack []*parsedElement
	stack = append(stack, root)
	lastEnd := 0

	matches := tagRegex.FindAllStringSubmatchIndex(html, -1)

	for _, match := range matches {
		// Text content before this tag
		if match[0] > lastEnd {
			textContent := strings.TrimSpace(html[lastEnd:match[0]])
			if textContent != "" && len(stack) > 0 {
				current := stack[len(stack)-1]
				current.textContent += textContent
			}
		}

		isClosing := html[match[2]:match[3]] == "/"
		tagName := strings.ToLower(html[match[4]:match[5]])
		attrStr := html[match[6]:match[7]]

		if isClosing {
			// Closing tag - pop from stack
			if len(stack) > 1 {
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i].tagName == tagName {
						stack = stack[:i]
						break
					}
				}
			}
		} else {
			// Opening tag
			elem := &parsedElement{
				tagName:    tagName,
				attributes: make(map[string]string),
				children:   make([]*parsedElement, 0),
			}

			// Parse attributes
			attrMatches := attrRegex.FindAllStringSubmatch(attrStr, -1)
			for _, am := range attrMatches {
				attrName := strings.ToLower(am[1])
				attrValue := am[2]
				elem.attributes[attrName] = attrValue

				if attrName == "id" {
					elem.id = attrValue
				} else if attrName == "class" {
					elem.classes = strings.Fields(attrValue)
				}
			}

			// Add to parent
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				elem.parent = parent
				parent.children = append(parent.children, elem)
			}

			// Self-closing tags don't go on the stack
			selfClosing := strings.HasSuffix(strings.TrimSpace(attrStr), "/") ||
				dm.isSelfClosingTag(tagName)
			if !selfClosing {
				stack = append(stack, elem)
			}
		}

		lastEnd = match[1]
	}

	// Store innerHTML
	root.innerHTML = html

	return root
}

// isSelfClosingTag returns true for HTML void elements
func (dm *DOMManager) isSelfClosingTag(tagName string) bool {
	switch tagName {
	case "area", "base", "br", "col", "embed", "hr", "img", "input",
		"link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

// createElementObject creates a Sobek object from a parsedElement
func (dm *DOMManager) createElementObject(elem *parsedElement) sobek.Value {
	if elem == nil {
		return sobek.Null()
	}

	vm := dm.vm
	obj := vm.NewObject()

	// Basic properties
	_ = obj.Set("nodeType", 1) // ELEMENT_NODE
	_ = obj.Set("nodeName", strings.ToUpper(elem.tagName))
	_ = obj.Set("tagName", strings.ToUpper(elem.tagName))
	_ = obj.Set("id", elem.id)
	_ = obj.Set("className", strings.Join(elem.classes, " "))
	_ = obj.Set("textContent", dm.getTextContent(elem))
	_ = obj.Set("innerHTML", elem.innerHTML)

	// classList
	classList := vm.NewObject()
	_ = classList.Set("contains", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return vm.ToValue(false)
		}
		className := call.Arguments[0].String()
		for _, c := range elem.classes {
			if c == className {
				return vm.ToValue(true)
			}
		}
		return vm.ToValue(false)
	})
	_ = classList.Set("add", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) >= 1 {
			elem.classes = append(elem.classes, call.Arguments[0].String())
		}
		return sobek.Undefined()
	})
	_ = classList.Set("remove", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) >= 1 {
			className := call.Arguments[0].String()
			newClasses := make([]string, 0)
			for _, c := range elem.classes {
				if c != className {
					newClasses = append(newClasses, c)
				}
			}
			elem.classes = newClasses
		}
		return sobek.Undefined()
	})
	_ = obj.Set("classList", classList)

	// attributes
	_ = obj.Set("getAttribute", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return sobek.Null()
		}
		name := strings.ToLower(call.Arguments[0].String())
		if val, ok := elem.attributes[name]; ok {
			return vm.ToValue(val)
		}
		return sobek.Null()
	})

	_ = obj.Set("setAttribute", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) >= 2 {
			name := strings.ToLower(call.Arguments[0].String())
			value := call.Arguments[1].String()
			elem.attributes[name] = value
			if name == "id" {
				elem.id = value
			} else if name == "class" {
				elem.classes = strings.Fields(value)
			}
		}
		return sobek.Undefined()
	})

	_ = obj.Set("hasAttribute", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return vm.ToValue(false)
		}
		name := strings.ToLower(call.Arguments[0].String())
		_, ok := elem.attributes[name]
		return vm.ToValue(ok)
	})

	_ = obj.Set("removeAttribute", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) >= 1 {
			name := strings.ToLower(call.Arguments[0].String())
			delete(elem.attributes, name)
		}
		return sobek.Undefined()
	})

	// querySelector/querySelectorAll on element
	_ = obj.Set("querySelector", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return sobek.Null()
		}
		selector := call.Arguments[0].String()
		found := dm.querySelector(elem, selector)
		if found != nil {
			return dm.createElementObject(found)
		}
		return sobek.Null()
	})

	_ = obj.Set("querySelectorAll", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return vm.ToValue([]interface{}{})
		}
		selector := call.Arguments[0].String()
		found := dm.querySelectorAll(elem, selector)
		result := make([]interface{}, len(found))
		for i, e := range found {
			result[i] = dm.createElementObject(e)
		}
		return vm.ToValue(result)
	})

	// children
	_ = obj.Set("children", func() []interface{} {
		result := make([]interface{}, len(elem.children))
		for i, child := range elem.children {
			result[i] = dm.createElementObject(child)
		}
		return result
	}())

	_ = obj.Set("childNodes", func() []interface{} {
		result := make([]interface{}, len(elem.children))
		for i, child := range elem.children {
			result[i] = dm.createElementObject(child)
		}
		return result
	}())

	return obj
}

// getTextContent recursively gets text content from an element
func (dm *DOMManager) getTextContent(elem *parsedElement) string {
	if elem == nil {
		return ""
	}

	text := elem.textContent
	for _, child := range elem.children {
		text += dm.getTextContent(child)
	}
	return text
}

// findElement finds first element with given tag name
func (dm *DOMManager) findElement(root *parsedElement, tagName string) *parsedElement {
	if root == nil {
		return nil
	}
	tagName = strings.ToLower(tagName)
	if root.tagName == tagName {
		return root
	}
	for _, child := range root.children {
		if found := dm.findElement(child, tagName); found != nil {
			return found
		}
	}
	return nil
}

// findElementByID finds element by ID
func (dm *DOMManager) findElementByID(root *parsedElement, id string) *parsedElement {
	if root == nil {
		return nil
	}
	if root.id == id {
		return root
	}
	for _, child := range root.children {
		if found := dm.findElementByID(child, id); found != nil {
			return found
		}
	}
	return nil
}

// findElementsByTag finds all elements with given tag name
func (dm *DOMManager) findElementsByTag(root *parsedElement, tagName string) []*parsedElement {
	var result []*parsedElement
	if root == nil {
		return result
	}
	if root.tagName == tagName || tagName == "*" {
		result = append(result, root)
	}
	for _, child := range root.children {
		result = append(result, dm.findElementsByTag(child, tagName)...)
	}
	return result
}

// findElementsByClass finds all elements with given class
func (dm *DOMManager) findElementsByClass(root *parsedElement, className string) []*parsedElement {
	var result []*parsedElement
	if root == nil {
		return result
	}
	for _, c := range root.classes {
		if c == className {
			result = append(result, root)
			break
		}
	}
	for _, child := range root.children {
		result = append(result, dm.findElementsByClass(child, className)...)
	}
	return result
}

// querySelector implements basic CSS selector matching
func (dm *DOMManager) querySelector(root *parsedElement, selector string) *parsedElement {
	results := dm.querySelectorAll(root, selector)
	if len(results) > 0 {
		return results[0]
	}
	return nil
}

// querySelectorAll implements basic CSS selector matching
func (dm *DOMManager) querySelectorAll(root *parsedElement, selector string) []*parsedElement {
	var result []*parsedElement
	if root == nil {
		return result
	}

	// Split by comma for multiple selectors
	selectors := strings.Split(selector, ",")
	for _, sel := range selectors {
		sel = strings.TrimSpace(sel)
		result = append(result, dm.matchSelector(root, sel)...)
	}

	return result
}

// matchSelector matches a single selector against element tree
func (dm *DOMManager) matchSelector(root *parsedElement, selector string) []*parsedElement {
	var result []*parsedElement

	// Parse the selector
	selector = strings.TrimSpace(selector)

	// Handle simple selectors: tag, #id, .class, [attr]
	switch {
	case strings.HasPrefix(selector, "#"):
		// ID selector
		id := selector[1:]
		if found := dm.findElementByID(root, id); found != nil {
			result = append(result, found)
		}

	case strings.HasPrefix(selector, "."):
		// Class selector
		className := selector[1:]
		result = dm.findElementsByClass(root, className)

	case strings.HasPrefix(selector, "["):
		// Attribute selector
		result = dm.matchAttributeSelector(root, selector)

	default:
		// Tag selector (possibly with class/id appended)
		tagName := selector
		var classFilter, idFilter string

		// Extract class from selector like "div.myclass"
		if idx := strings.Index(selector, "."); idx > 0 {
			tagName = selector[:idx]
			rest := selector[idx+1:]
			if idx2 := strings.IndexAny(rest, ".#["); idx2 >= 0 {
				classFilter = rest[:idx2]
			} else {
				classFilter = rest
			}
		}

		// Extract ID from selector like "div#myid"
		if idx := strings.Index(selector, "#"); idx > 0 {
			if tagName == selector {
				tagName = selector[:idx]
			}
			rest := selector[idx+1:]
			if idx2 := strings.IndexAny(rest, ".#["); idx2 >= 0 {
				idFilter = rest[:idx2]
			} else {
				idFilter = rest
			}
		}

		// Find by tag
		elements := dm.findElementsByTag(root, strings.ToLower(tagName))

		// Filter by class and/or ID if specified
		for _, elem := range elements {
			match := true
			if classFilter != "" {
				hasClass := false
				for _, c := range elem.classes {
					if c == classFilter {
						hasClass = true
						break
					}
				}
				if !hasClass {
					match = false
				}
			}
			if idFilter != "" && elem.id != idFilter {
				match = false
			}
			if match {
				result = append(result, elem)
			}
		}
	}

	return result
}

// matchAttributeSelector matches [attr] or [attr=value] selectors
func (dm *DOMManager) matchAttributeSelector(root *parsedElement, selector string) []*parsedElement {
	var result []*parsedElement

	// Parse attribute selector
	selector = strings.TrimPrefix(selector, "[")
	selector = strings.TrimSuffix(selector, "]")

	var attrName, attrValue string
	var hasValue bool

	if idx := strings.Index(selector, "="); idx >= 0 {
		attrName = strings.ToLower(selector[:idx])
		attrValue = strings.Trim(selector[idx+1:], "\"'")
		hasValue = true
	} else {
		attrName = strings.ToLower(selector)
	}

	// Find all elements and filter
	allElements := dm.findElementsByTag(root, "*")
	for _, elem := range allElements {
		if val, ok := elem.attributes[attrName]; ok {
			if !hasValue || val == attrValue {
				result = append(result, elem)
			}
		}
	}

	return result
}
