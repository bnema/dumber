package browserjs

import (
	"time"

	"github.com/grafana/sobek"
)

// EventManager provides Event constructors and EventTarget.
type EventManager struct {
	vm        *sobek.Runtime
	startTime time.Time
}

// NewEventManager creates a new event manager.
func NewEventManager(vm *sobek.Runtime, startTime time.Time) *EventManager {
	if startTime.IsZero() {
		startTime = time.Now()
	}
	return &EventManager{vm: vm, startTime: startTime}
}

// Install adds Event constructors and EventTarget.
func (em *EventManager) Install() error {
	vm := em.vm

	// Base Event
	vm.Set("Event", func(call sobek.ConstructorCall) *sobek.Object {
		eventType := ""
		if len(call.Arguments) > 0 {
			eventType = call.Arguments[0].String()
		}
		bubbles := false
		cancelable := false
		if len(call.Arguments) > 1 {
			if opts := call.Arguments[1].Export(); opts != nil {
				if m, ok := opts.(map[string]interface{}); ok {
					if b, ok := m["bubbles"].(bool); ok {
						bubbles = b
					}
					if c, ok := m["cancelable"].(bool); ok {
						cancelable = c
					}
				}
			}
		}
		evt := call.This
		_ = evt.Set("type", eventType)
		_ = evt.Set("bubbles", bubbles)
		_ = evt.Set("cancelable", cancelable)
		_ = evt.Set("defaultPrevented", false)
		_ = evt.Set("target", sobek.Null())
		_ = evt.Set("currentTarget", sobek.Null())
		_ = evt.Set("eventPhase", 0)
		_ = evt.Set("isTrusted", false)
		_ = evt.Set("timeStamp", float64(em.startTime.UnixMilli()))
		_ = evt.Set("preventDefault", func(sobek.FunctionCall) sobek.Value {
			_ = evt.Set("defaultPrevented", true)
			return sobek.Undefined()
		})
		_ = evt.Set("stopPropagation", func(sobek.FunctionCall) sobek.Value {
			return sobek.Undefined()
		})
		_ = evt.Set("stopImmediatePropagation", func(sobek.FunctionCall) sobek.Value {
			return sobek.Undefined()
		})
		return nil
	})

	// CustomEvent
	vm.Set("CustomEvent", func(call sobek.ConstructorCall) *sobek.Object {
		eventType := ""
		if len(call.Arguments) > 0 {
			eventType = call.Arguments[0].String()
		}
		var detail interface{}
		if len(call.Arguments) > 1 {
			if opts := call.Arguments[1].Export(); opts != nil {
				if m, ok := opts.(map[string]interface{}); ok {
					detail = m["detail"]
				}
			}
		}
		evt := call.This
		_ = evt.Set("type", eventType)
		_ = evt.Set("detail", detail)
		_ = evt.Set("bubbles", false)
		_ = evt.Set("cancelable", false)
		_ = evt.Set("defaultPrevented", false)
		_ = evt.Set("preventDefault", func(sobek.FunctionCall) sobek.Value {
			_ = evt.Set("defaultPrevented", true)
			return sobek.Undefined()
		})
		return nil
	})

	// MouseEvent
	vm.Set("MouseEvent", func(call sobek.ConstructorCall) *sobek.Object {
		eventType := ""
		if len(call.Arguments) > 0 {
			eventType = call.Arguments[0].String()
		}
		evt := call.This
		_ = evt.Set("type", eventType)
		_ = evt.Set("bubbles", true)
		_ = evt.Set("cancelable", true)
		_ = evt.Set("defaultPrevented", false)
		_ = evt.Set("button", 0)
		_ = evt.Set("buttons", 0)
		_ = evt.Set("clientX", 0)
		_ = evt.Set("clientY", 0)
		_ = evt.Set("screenX", 0)
		_ = evt.Set("screenY", 0)
		_ = evt.Set("pageX", 0)
		_ = evt.Set("pageY", 0)
		_ = evt.Set("offsetX", 0)
		_ = evt.Set("offsetY", 0)
		_ = evt.Set("movementX", 0)
		_ = evt.Set("movementY", 0)
		_ = evt.Set("ctrlKey", false)
		_ = evt.Set("shiftKey", false)
		_ = evt.Set("altKey", false)
		_ = evt.Set("metaKey", false)
		_ = evt.Set("relatedTarget", sobek.Null())
		_ = evt.Set("preventDefault", func(sobek.FunctionCall) sobek.Value {
			_ = evt.Set("defaultPrevented", true)
			return sobek.Undefined()
		})
		_ = evt.Set("stopPropagation", func(sobek.FunctionCall) sobek.Value {
			return sobek.Undefined()
		})
		return nil
	})

	// KeyboardEvent
	vm.Set("KeyboardEvent", func(call sobek.ConstructorCall) *sobek.Object {
		eventType := ""
		if len(call.Arguments) > 0 {
			eventType = call.Arguments[0].String()
		}
		evt := call.This
		_ = evt.Set("type", eventType)
		_ = evt.Set("bubbles", true)
		_ = evt.Set("cancelable", true)
		_ = evt.Set("key", "")
		_ = evt.Set("code", "")
		_ = evt.Set("keyCode", 0)
		_ = evt.Set("charCode", 0)
		_ = evt.Set("which", 0)
		_ = evt.Set("location", 0)
		_ = evt.Set("repeat", false)
		_ = evt.Set("isComposing", false)
		_ = evt.Set("ctrlKey", false)
		_ = evt.Set("shiftKey", false)
		_ = evt.Set("altKey", false)
		_ = evt.Set("metaKey", false)
		_ = evt.Set("preventDefault", func(sobek.FunctionCall) sobek.Value {
			_ = evt.Set("defaultPrevented", true)
			return sobek.Undefined()
		})
		return nil
	})

	// FocusEvent
	vm.Set("FocusEvent", func(call sobek.ConstructorCall) *sobek.Object {
		eventType := ""
		if len(call.Arguments) > 0 {
			eventType = call.Arguments[0].String()
		}
		evt := call.This
		_ = evt.Set("type", eventType)
		_ = evt.Set("bubbles", false)
		_ = evt.Set("cancelable", false)
		_ = evt.Set("relatedTarget", sobek.Null())
		return nil
	})

	// InputEvent
	vm.Set("InputEvent", func(call sobek.ConstructorCall) *sobek.Object {
		eventType := ""
		if len(call.Arguments) > 0 {
			eventType = call.Arguments[0].String()
		}
		evt := call.This
		_ = evt.Set("type", eventType)
		_ = evt.Set("bubbles", true)
		_ = evt.Set("cancelable", false)
		_ = evt.Set("data", "")
		_ = evt.Set("inputType", "")
		_ = evt.Set("isComposing", false)
		return nil
	})

	// MessageEvent
	vm.Set("MessageEvent", func(call sobek.ConstructorCall) *sobek.Object {
		eventType := ""
		if len(call.Arguments) > 0 {
			eventType = call.Arguments[0].String()
		}
		var data interface{}
		origin := ""
		if len(call.Arguments) > 1 {
			if opts := call.Arguments[1].Export(); opts != nil {
				if m, ok := opts.(map[string]interface{}); ok {
					data = m["data"]
					if o, ok := m["origin"].(string); ok {
						origin = o
					}
				}
			}
		}
		evt := call.This
		_ = evt.Set("type", eventType)
		_ = evt.Set("data", data)
		_ = evt.Set("origin", origin)
		_ = evt.Set("lastEventId", "")
		_ = evt.Set("source", sobek.Null())
		_ = evt.Set("ports", []interface{}{})
		return nil
	})

	// ErrorEvent
	vm.Set("ErrorEvent", func(call sobek.ConstructorCall) *sobek.Object {
		eventType := ""
		if len(call.Arguments) > 0 {
			eventType = call.Arguments[0].String()
		}
		evt := call.This
		_ = evt.Set("type", eventType)
		_ = evt.Set("message", "")
		_ = evt.Set("filename", "")
		_ = evt.Set("lineno", 0)
		_ = evt.Set("colno", 0)
		_ = evt.Set("error", sobek.Null())
		return nil
	})

	// ProgressEvent
	vm.Set("ProgressEvent", func(call sobek.ConstructorCall) *sobek.Object {
		eventType := ""
		if len(call.Arguments) > 0 {
			eventType = call.Arguments[0].String()
		}
		evt := call.This
		_ = evt.Set("type", eventType)
		_ = evt.Set("lengthComputable", false)
		_ = evt.Set("loaded", 0)
		_ = evt.Set("total", 0)
		return nil
	})

	// CloseEvent (for WebSocket)
	vm.Set("CloseEvent", func(call sobek.ConstructorCall) *sobek.Object {
		eventType := ""
		if len(call.Arguments) > 0 {
			eventType = call.Arguments[0].String()
		}
		evt := call.This
		_ = evt.Set("type", eventType)
		_ = evt.Set("code", 1000)
		_ = evt.Set("reason", "")
		_ = evt.Set("wasClean", true)
		return nil
	})

	// StorageEvent
	vm.Set("StorageEvent", func(call sobek.ConstructorCall) *sobek.Object {
		eventType := ""
		if len(call.Arguments) > 0 {
			eventType = call.Arguments[0].String()
		}
		evt := call.This
		_ = evt.Set("type", eventType)
		_ = evt.Set("key", sobek.Null())
		_ = evt.Set("oldValue", sobek.Null())
		_ = evt.Set("newValue", sobek.Null())
		_ = evt.Set("url", "")
		_ = evt.Set("storageArea", sobek.Null())
		return nil
	})

	// PopStateEvent
	vm.Set("PopStateEvent", func(call sobek.ConstructorCall) *sobek.Object {
		eventType := ""
		if len(call.Arguments) > 0 {
			eventType = call.Arguments[0].String()
		}
		evt := call.This
		_ = evt.Set("type", eventType)
		_ = evt.Set("state", sobek.Null())
		return nil
	})

	// HashChangeEvent
	vm.Set("HashChangeEvent", func(call sobek.ConstructorCall) *sobek.Object {
		eventType := ""
		if len(call.Arguments) > 0 {
			eventType = call.Arguments[0].String()
		}
		evt := call.This
		_ = evt.Set("type", eventType)
		_ = evt.Set("oldURL", "")
		_ = evt.Set("newURL", "")
		return nil
	})

	// DragEvent
	vm.Set("DragEvent", func(call sobek.ConstructorCall) *sobek.Object {
		eventType := ""
		if len(call.Arguments) > 0 {
			eventType = call.Arguments[0].String()
		}
		evt := call.This
		_ = evt.Set("type", eventType)
		_ = evt.Set("dataTransfer", sobek.Null())
		return nil
	})

	// WheelEvent
	vm.Set("WheelEvent", func(call sobek.ConstructorCall) *sobek.Object {
		eventType := ""
		if len(call.Arguments) > 0 {
			eventType = call.Arguments[0].String()
		}
		evt := call.This
		_ = evt.Set("type", eventType)
		_ = evt.Set("deltaX", 0)
		_ = evt.Set("deltaY", 0)
		_ = evt.Set("deltaZ", 0)
		_ = evt.Set("deltaMode", 0)
		return nil
	})

	// TouchEvent
	vm.Set("TouchEvent", func(call sobek.ConstructorCall) *sobek.Object {
		eventType := ""
		if len(call.Arguments) > 0 {
			eventType = call.Arguments[0].String()
		}
		evt := call.This
		_ = evt.Set("type", eventType)
		_ = evt.Set("touches", []interface{}{})
		_ = evt.Set("targetTouches", []interface{}{})
		_ = evt.Set("changedTouches", []interface{}{})
		return nil
	})

	// EventTarget
	vm.Set("EventTarget", func(call sobek.ConstructorCall) *sobek.Object {
		listeners := make(map[string][]sobek.Callable)
		target := call.This
		_ = target.Set("addEventListener", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 2 {
				eventType := call.Arguments[0].String()
				if cb, ok := sobek.AssertFunction(call.Arguments[1]); ok {
					listeners[eventType] = append(listeners[eventType], cb)
				}
			}
			return sobek.Undefined()
		})
		_ = target.Set("removeEventListener", func(sobek.FunctionCall) sobek.Value {
			return sobek.Undefined()
		})
		_ = target.Set("dispatchEvent", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				evt := call.Arguments[0].ToObject(vm)
				eventType := evt.Get("type").String()
				if cbs, ok := listeners[eventType]; ok {
					for _, cb := range cbs {
						_, _ = cb(target, evt)
					}
				}
			}
			return vm.ToValue(true)
		})
		return nil
	})

	// Add EventTarget methods to window/global object
	// Many scripts expect window.addEventListener to work
	windowListeners := make(map[string][]sobek.Callable)
	global := vm.GlobalObject()

	global.Set("addEventListener", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) >= 2 {
			eventType := call.Arguments[0].String()
			if cb, ok := sobek.AssertFunction(call.Arguments[1]); ok {
				windowListeners[eventType] = append(windowListeners[eventType], cb)
			}
		}
		return sobek.Undefined()
	})

	global.Set("removeEventListener", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) >= 2 {
			eventType := call.Arguments[0].String()
			// Simple removal - just filter out (doesn't handle same callback added multiple times)
			if cbs, ok := windowListeners[eventType]; ok {
				windowListeners[eventType] = cbs[:0]
			}
		}
		return sobek.Undefined()
	})

	global.Set("dispatchEvent", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			evt := call.Arguments[0].ToObject(vm)
			eventType := evt.Get("type").String()
			if cbs, ok := windowListeners[eventType]; ok {
				for _, cb := range cbs {
					_, _ = cb(global, evt)
				}
			}
		}
		return vm.ToValue(true)
	})

	return nil
}
