package browserjs

import (
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

	// DOMParser
	vm.Set("DOMParser", func(call sobek.ConstructorCall) *sobek.Object {
		parser := call.This
		_ = parser.Set("parseFromString", func(call sobek.FunctionCall) sobek.Value {
			doc := vm.NewObject()
			_ = doc.Set("documentElement", vm.NewObject())
			_ = doc.Set("querySelector", func(sobek.FunctionCall) sobek.Value { return sobek.Null() })
			_ = doc.Set("querySelectorAll", func(sobek.FunctionCall) sobek.Value {
				return vm.ToValue([]interface{}{})
			})
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
		signal := vm.NewObject()
		_ = signal.Set("aborted", false)
		_ = signal.Set("reason", sobek.Undefined())
		_ = signal.Set("addEventListener", func(sobek.FunctionCall) sobek.Value {
			return sobek.Undefined()
		})
		_ = signal.Set("removeEventListener", func(sobek.FunctionCall) sobek.Value {
			return sobek.Undefined()
		})

		controller := call.This
		_ = controller.Set("signal", signal)
		_ = controller.Set("abort", func(call sobek.FunctionCall) sobek.Value {
			if !aborted {
				aborted = true
				_ = signal.Set("aborted", true)
				reason := "AbortError"
				if len(call.Arguments) > 0 {
					reason = call.Arguments[0].String()
				}
				_ = signal.Set("reason", reason)
			}
			return sobek.Undefined()
		})
		return nil
	})

	vm.Set("AbortSignal", func(call sobek.ConstructorCall) *sobek.Object {
		signal := call.This
		_ = signal.Set("aborted", false)
		_ = signal.Set("reason", sobek.Undefined())
		return nil
	})

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
