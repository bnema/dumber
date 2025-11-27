package browserjs

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/grafana/sobek"
)

// LocalStorageBackend defines the interface for persistent localStorage
type LocalStorageBackend interface {
	GetItem(key string) (string, bool)
	SetItem(key, value string) error
	RemoveItem(key string) error
	Clear() error
	Keys() []string
	Length() int
}

// WebAPIsManager provides additional web APIs like XMLHttpRequest, Storage, etc.
type WebAPIsManager struct {
	vm                *sobek.Runtime
	tasks             chan func()
	httpClient        HTTPDoer
	startTime         time.Time
	origin            string
	localStorage      LocalStorageBackend // Optional persistent localStorage backend
	extensionBasePath string              // Filesystem path for extension:// URL handling
}

// NewWebAPIsManager creates a new web APIs manager.
// localStorage is optional - if nil, an in-memory implementation is used.
func NewWebAPIsManager(vm *sobek.Runtime, tasks chan func(), httpClient HTTPDoer, startTime time.Time, origin string, localStorage LocalStorageBackend, extensionBasePath string) *WebAPIsManager {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if startTime.IsZero() {
		startTime = time.Now()
	}
	if origin == "" {
		origin = "null"
	}
	return &WebAPIsManager{
		vm:                vm,
		tasks:             tasks,
		httpClient:        httpClient,
		startTime:         startTime,
		origin:            origin,
		localStorage:      localStorage,
		extensionBasePath: extensionBasePath,
	}
}

// Install adds additional web APIs like XMLHttpRequest, WebSocket, Storage, etc.
func (wm *WebAPIsManager) Install() error {
	vm := wm.vm

	if err := wm.installXMLHttpRequest(); err != nil {
		return err
	}

	if err := wm.installStorage(); err != nil {
		return err
	}

	if err := wm.installMessageChannel(); err != nil {
		return err
	}

	if err := wm.installMiscAPIs(); err != nil {
		return err
	}

	// structuredClone
	vm.Set("structuredClone", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return sobek.Undefined()
		}
		// Simple implementation using JSON roundtrip
		data := call.Arguments[0].Export()
		jsonBytes, err := json.Marshal(data)
		if err != nil {
			return sobek.Undefined()
		}
		var cloned interface{}
		if err := json.Unmarshal(jsonBytes, &cloned); err != nil {
			return sobek.Undefined()
		}
		return vm.ToValue(cloned)
	})

	// DOMException
	vm.Set("DOMException", func(call sobek.ConstructorCall) *sobek.Object {
		message := ""
		name := "Error"
		if len(call.Arguments) > 0 {
			message = call.Arguments[0].String()
		}
		if len(call.Arguments) > 1 {
			name = call.Arguments[1].String()
		}
		exc := call.This
		_ = exc.Set("message", message)
		_ = exc.Set("name", name)
		_ = exc.Set("code", 0)
		return nil
	})

	// Intl namespace (minimal)
	intl := vm.NewObject()
	_ = intl.Set("DateTimeFormat", func(call sobek.ConstructorCall) *sobek.Object {
		formatter := call.This
		_ = formatter.Set("format", func(call sobek.FunctionCall) sobek.Value {
			return vm.ToValue("")
		})
		_ = formatter.Set("resolvedOptions", func(sobek.FunctionCall) sobek.Value {
			return vm.NewObject()
		})
		return nil
	})
	_ = intl.Set("NumberFormat", func(call sobek.ConstructorCall) *sobek.Object {
		formatter := call.This
		_ = formatter.Set("format", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				return vm.ToValue(call.Arguments[0].String())
			}
			return vm.ToValue("")
		})
		return nil
	})
	_ = intl.Set("Collator", func(call sobek.ConstructorCall) *sobek.Object {
		collator := call.This
		_ = collator.Set("compare", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 2 {
				a, b := call.Arguments[0].String(), call.Arguments[1].String()
				if a < b {
					return vm.ToValue(-1)
				} else if a > b {
					return vm.ToValue(1)
				}
				return vm.ToValue(0)
			}
			return vm.ToValue(0)
		})
		return nil
	})
	vm.Set("Intl", intl)

	return nil
}

// installXMLHttpRequest adds XMLHttpRequest.
func (wm *WebAPIsManager) installXMLHttpRequest() error {
	vm := wm.vm

	vm.Set("XMLHttpRequest", func(call sobek.ConstructorCall) *sobek.Object {
		xhr := call.This
		var method, url string
		var async bool = true
		var reqHeaders = make(map[string]string)
		var responseText string
		var responseHeaders = make(map[string]string)
		var status int
		var statusText string
		var readyState int = 0
		var onreadystatechange sobek.Callable

		_ = xhr.Set("readyState", readyState)
		_ = xhr.Set("status", status)
		_ = xhr.Set("statusText", statusText)
		_ = xhr.Set("responseText", responseText)
		_ = xhr.Set("response", responseText)
		_ = xhr.Set("responseType", "")
		_ = xhr.Set("timeout", 0)
		_ = xhr.Set("withCredentials", false)

		setReadyState := func(state int) {
			readyState = state
			_ = xhr.Set("readyState", state)
			if onreadystatechange != nil {
				_, _ = onreadystatechange(xhr)
			}
		}

		_ = xhr.DefineAccessorProperty("onreadystatechange",
			vm.ToValue(func(sobek.FunctionCall) sobek.Value {
				if onreadystatechange == nil {
					return sobek.Null()
				}
				return vm.ToValue(onreadystatechange)
			}),
			vm.ToValue(func(call sobek.FunctionCall) sobek.Value {
				if len(call.Arguments) > 0 {
					if cb, ok := sobek.AssertFunction(call.Arguments[0]); ok {
						onreadystatechange = cb
					}
				}
				return sobek.Undefined()
			}),
			sobek.FLAG_FALSE, sobek.FLAG_TRUE)

		_ = xhr.Set("open", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 2 {
				method = strings.ToUpper(call.Arguments[0].String())
				url = call.Arguments[1].String()
				// Resolve relative URLs against origin
				if strings.HasPrefix(url, "/") && wm.origin != "" && wm.origin != "null" {
					url = wm.origin + url
					log.Printf("[browserjs/xhr] Resolved relative URL to: %s", url)
				}
				if len(call.Arguments) > 2 {
					async = call.Arguments[2].ToBoolean()
				}
			}
			setReadyState(1) // OPENED
			return sobek.Undefined()
		})

		_ = xhr.Set("setRequestHeader", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 2 {
				reqHeaders[call.Arguments[0].String()] = call.Arguments[1].String()
			}
			return sobek.Undefined()
		})

		_ = xhr.Set("getResponseHeader", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				name := strings.ToLower(call.Arguments[0].String())
				if v, ok := responseHeaders[name]; ok {
					return vm.ToValue(v)
				}
			}
			return sobek.Null()
		})

		_ = xhr.Set("getAllResponseHeaders", func(call sobek.FunctionCall) sobek.Value {
			var sb strings.Builder
			for k, v := range responseHeaders {
				sb.WriteString(k)
				sb.WriteString(": ")
				sb.WriteString(v)
				sb.WriteString("\r\n")
			}
			return vm.ToValue(sb.String())
		})

		_ = xhr.Set("send", func(call sobek.FunctionCall) sobek.Value {
			log.Printf("[browserjs/xhr] XHR send: %s %s (async=%v)", method, url, async)
			var body io.Reader
			if len(call.Arguments) > 0 && call.Arguments[0] != sobek.Null() && call.Arguments[0] != sobek.Undefined() {
				body = strings.NewReader(call.Arguments[0].String())
			}

			doRequest := func() {
				log.Printf("[browserjs/xhr] XHR executing: %s %s", method, url)

				// Handle dumb-extension:// URLs by reading from filesystem
				if strings.HasPrefix(url, "dumb-extension://") {
					// Parse URL: dumb-extension://extension-id/path/to/file
					urlPath := strings.TrimPrefix(url, "dumb-extension://")
					parts := strings.SplitN(urlPath, "/", 2)
					var filePath string
					if len(parts) >= 2 {
						filePath = "/" + parts[1]
					} else {
						filePath = "/"
					}

					if wm.extensionBasePath == "" {
						log.Printf("[browserjs/xhr] Extension base path not set for URL: %s", url)
						status = 500
						statusText = "500 Internal Server Error"
						responseText = "Extension base path not configured"
						_ = xhr.Set("status", status)
						_ = xhr.Set("statusText", statusText)
						_ = xhr.Set("responseText", responseText)
						_ = xhr.Set("response", responseText)
						setReadyState(4)
						return
					}

					fullPath := filepath.Join(wm.extensionBasePath, filePath)
					log.Printf("[browserjs/xhr] Reading extension file: %s", fullPath)

					data, err := os.ReadFile(fullPath)
					if err != nil {
						log.Printf("[browserjs/xhr] Failed to read extension file: %v", err)
						status = 404
						statusText = "404 Not Found"
						responseText = "File not found: " + filePath
						_ = xhr.Set("status", status)
						_ = xhr.Set("statusText", statusText)
						_ = xhr.Set("responseText", responseText)
						_ = xhr.Set("response", responseText)
						setReadyState(4)
						return
					}

					responseText = string(data)
					status = 200
					statusText = "200 OK"
					// Set content-type based on file extension
					switch {
					case strings.HasSuffix(filePath, ".json"):
						responseHeaders["content-type"] = "application/json"
					case strings.HasSuffix(filePath, ".js"):
						responseHeaders["content-type"] = "application/javascript"
					case strings.HasSuffix(filePath, ".html"):
						responseHeaders["content-type"] = "text/html"
					case strings.HasSuffix(filePath, ".css"):
						responseHeaders["content-type"] = "text/css"
					default:
						responseHeaders["content-type"] = "text/plain"
					}

					_ = xhr.Set("status", status)
					_ = xhr.Set("statusText", statusText)
					_ = xhr.Set("responseText", responseText)
					_ = xhr.Set("response", responseText)

					setReadyState(2) // HEADERS_RECEIVED
					setReadyState(3) // LOADING
					setReadyState(4) // DONE
					return
				}

				// Standard HTTP request
				req, err := http.NewRequest(method, url, body)
				if err != nil {
					setReadyState(4)
					return
				}
				for k, v := range reqHeaders {
					req.Header.Set(k, v)
				}

				resp, err := wm.httpClient.Do(req)
				if err != nil {
					setReadyState(4)
					return
				}
				defer resp.Body.Close()

				respBody, _ := io.ReadAll(resp.Body)
				responseText = string(respBody)
				status = resp.StatusCode
				statusText = resp.Status
				for k, v := range resp.Header {
					responseHeaders[strings.ToLower(k)] = strings.Join(v, ", ")
				}

				_ = xhr.Set("status", status)
				_ = xhr.Set("statusText", statusText)
				_ = xhr.Set("responseText", responseText)
				_ = xhr.Set("response", responseText)

				setReadyState(2) // HEADERS_RECEIVED
				setReadyState(3) // LOADING
				setReadyState(4) // DONE
			}

			if async {
				go func() {
					if wm.tasks != nil {
						wm.tasks <- doRequest
					} else {
						doRequest()
					}
				}()
			} else {
				doRequest()
			}

			return sobek.Undefined()
		})

		_ = xhr.Set("abort", func(call sobek.FunctionCall) sobek.Value {
			setReadyState(4)
			return sobek.Undefined()
		})

		// Constants
		_ = xhr.Set("UNSENT", 0)
		_ = xhr.Set("OPENED", 1)
		_ = xhr.Set("HEADERS_RECEIVED", 2)
		_ = xhr.Set("LOADING", 3)
		_ = xhr.Set("DONE", 4)

		return nil
	})

	return nil
}

// installStorage adds localStorage and sessionStorage.
func (wm *WebAPIsManager) installStorage() error {
	vm := wm.vm

	// In-memory storage factory (used for sessionStorage and fallback localStorage)
	createInMemoryStorage := func() *sobek.Object {
		data := make(map[string]string)
		var mu sync.RWMutex

		storage := vm.NewObject()

		_ = storage.Set("getItem", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				mu.RLock()
				v, ok := data[call.Arguments[0].String()]
				mu.RUnlock()
				if ok {
					return vm.ToValue(v)
				}
			}
			return sobek.Null()
		})

		_ = storage.Set("setItem", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 2 {
				mu.Lock()
				data[call.Arguments[0].String()] = call.Arguments[1].String()
				mu.Unlock()
			}
			return sobek.Undefined()
		})

		_ = storage.Set("removeItem", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				mu.Lock()
				delete(data, call.Arguments[0].String())
				mu.Unlock()
			}
			return sobek.Undefined()
		})

		_ = storage.Set("clear", func(call sobek.FunctionCall) sobek.Value {
			mu.Lock()
			data = make(map[string]string)
			mu.Unlock()
			return sobek.Undefined()
		})

		_ = storage.Set("key", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				index := int(call.Arguments[0].ToInteger())
				mu.RLock()
				i := 0
				for k := range data {
					if i == index {
						mu.RUnlock()
						return vm.ToValue(k)
					}
					i++
				}
				mu.RUnlock()
			}
			return sobek.Null()
		})

		_ = storage.DefineAccessorProperty("length",
			vm.ToValue(func(sobek.FunctionCall) sobek.Value {
				mu.RLock()
				l := len(data)
				mu.RUnlock()
				return vm.ToValue(l)
			}), nil, sobek.FLAG_FALSE, sobek.FLAG_TRUE)

		return storage
	}

	// Persistent storage factory (uses backend)
	createPersistentStorage := func(backend LocalStorageBackend) *sobek.Object {
		storage := vm.NewObject()

		_ = storage.Set("getItem", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				v, ok := backend.GetItem(call.Arguments[0].String())
				if ok {
					return vm.ToValue(v)
				}
			}
			return sobek.Null()
		})

		_ = storage.Set("setItem", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 2 {
				key := call.Arguments[0].String()
				value := call.Arguments[1].String()
				log.Printf("[browserjs] localStorage.setItem called: key=%s, len=%d", key, len(value))
				_ = backend.SetItem(key, value)
			}
			return sobek.Undefined()
		})

		_ = storage.Set("removeItem", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				_ = backend.RemoveItem(call.Arguments[0].String())
			}
			return sobek.Undefined()
		})

		_ = storage.Set("clear", func(call sobek.FunctionCall) sobek.Value {
			_ = backend.Clear()
			return sobek.Undefined()
		})

		_ = storage.Set("key", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				index := int(call.Arguments[0].ToInteger())
				keys := backend.Keys()
				if index >= 0 && index < len(keys) {
					return vm.ToValue(keys[index])
				}
			}
			return sobek.Null()
		})

		_ = storage.DefineAccessorProperty("length",
			vm.ToValue(func(sobek.FunctionCall) sobek.Value {
				return vm.ToValue(backend.Length())
			}), nil, sobek.FLAG_FALSE, sobek.FLAG_TRUE)

		return storage
	}

	// localStorage: use persistent backend if provided, otherwise in-memory
	if wm.localStorage != nil {
		log.Printf("[browserjs] Installing persistent localStorage backend")
		vm.Set("localStorage", createPersistentStorage(wm.localStorage))
	} else {
		log.Printf("[browserjs] Installing in-memory localStorage (no backend)")
		vm.Set("localStorage", createInMemoryStorage())
	}

	// sessionStorage: always in-memory (correct Web API behavior)
	vm.Set("sessionStorage", createInMemoryStorage())

	return nil
}

// installMessageChannel adds MessageChannel and MessagePort.
func (wm *WebAPIsManager) installMessageChannel() error {
	vm := wm.vm

	vm.Set("MessageChannel", func(call sobek.ConstructorCall) *sobek.Object {
		channel := call.This

		// Create two ports
		port1 := vm.NewObject()
		port2 := vm.NewObject()

		var port1OnMessage, port2OnMessage sobek.Callable

		_ = port1.Set("postMessage", func(call sobek.FunctionCall) sobek.Value {
			if port2OnMessage != nil && len(call.Arguments) > 0 {
				evt := vm.NewObject()
				_ = evt.Set("data", call.Arguments[0])
				_, _ = port2OnMessage(port2, evt)
			}
			return sobek.Undefined()
		})

		_ = port2.Set("postMessage", func(call sobek.FunctionCall) sobek.Value {
			if port1OnMessage != nil && len(call.Arguments) > 0 {
				evt := vm.NewObject()
				_ = evt.Set("data", call.Arguments[0])
				_, _ = port1OnMessage(port1, evt)
			}
			return sobek.Undefined()
		})

		_ = port1.DefineAccessorProperty("onmessage",
			vm.ToValue(func(sobek.FunctionCall) sobek.Value {
				if port1OnMessage == nil {
					return sobek.Null()
				}
				return vm.ToValue(port1OnMessage)
			}),
			vm.ToValue(func(call sobek.FunctionCall) sobek.Value {
				if len(call.Arguments) > 0 {
					if cb, ok := sobek.AssertFunction(call.Arguments[0]); ok {
						port1OnMessage = cb
					}
				}
				return sobek.Undefined()
			}),
			sobek.FLAG_FALSE, sobek.FLAG_TRUE)

		_ = port2.DefineAccessorProperty("onmessage",
			vm.ToValue(func(sobek.FunctionCall) sobek.Value {
				if port2OnMessage == nil {
					return sobek.Null()
				}
				return vm.ToValue(port2OnMessage)
			}),
			vm.ToValue(func(call sobek.FunctionCall) sobek.Value {
				if len(call.Arguments) > 0 {
					if cb, ok := sobek.AssertFunction(call.Arguments[0]); ok {
						port2OnMessage = cb
					}
				}
				return sobek.Undefined()
			}),
			sobek.FLAG_FALSE, sobek.FLAG_TRUE)

		_ = port1.Set("start", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = port2.Set("start", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = port1.Set("close", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = port2.Set("close", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })

		_ = channel.Set("port1", port1)
		_ = channel.Set("port2", port2)

		return nil
	})

	// BroadcastChannel (simplified)
	vm.Set("BroadcastChannel", func(call sobek.ConstructorCall) *sobek.Object {
		name := ""
		if len(call.Arguments) > 0 {
			name = call.Arguments[0].String()
		}
		bc := call.This
		_ = bc.Set("name", name)
		_ = bc.Set("onmessage", sobek.Null())
		_ = bc.Set("postMessage", func(sobek.FunctionCall) sobek.Value {
			return sobek.Undefined()
		})
		_ = bc.Set("close", func(sobek.FunctionCall) sobek.Value {
			return sobek.Undefined()
		})
		return nil
	})

	return nil
}

// installMiscAPIs adds various other APIs.
func (wm *WebAPIsManager) installMiscAPIs() error {
	vm := wm.vm

	// MutationObserver (no-op in background)
	vm.Set("MutationObserver", func(call sobek.ConstructorCall) *sobek.Object {
		observer := call.This
		_ = observer.Set("observe", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = observer.Set("disconnect", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = observer.Set("takeRecords", func(sobek.FunctionCall) sobek.Value { return vm.ToValue([]interface{}{}) })
		return nil
	})

	// ResizeObserver (no-op in background)
	vm.Set("ResizeObserver", func(call sobek.ConstructorCall) *sobek.Object {
		observer := call.This
		_ = observer.Set("observe", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = observer.Set("unobserve", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = observer.Set("disconnect", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		return nil
	})

	// IntersectionObserver (no-op in background)
	vm.Set("IntersectionObserver", func(call sobek.ConstructorCall) *sobek.Object {
		observer := call.This
		_ = observer.Set("observe", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = observer.Set("unobserve", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = observer.Set("disconnect", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = observer.Set("takeRecords", func(sobek.FunctionCall) sobek.Value { return vm.ToValue([]interface{}{}) })
		return nil
	})

	// getComputedStyle (returns empty style)
	vm.Set("getComputedStyle", func(call sobek.FunctionCall) sobek.Value {
		style := vm.NewObject()
		_ = style.Set("getPropertyValue", func(sobek.FunctionCall) sobek.Value { return vm.ToValue("") })
		return style
	})

	// matchMedia (returns no-match)
	vm.Set("matchMedia", func(call sobek.FunctionCall) sobek.Value {
		result := vm.NewObject()
		_ = result.Set("matches", false)
		_ = result.Set("media", "")
		_ = result.Set("addEventListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = result.Set("removeEventListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = result.Set("addListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = result.Set("removeListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		if len(call.Arguments) > 0 {
			_ = result.Set("media", call.Arguments[0].String())
		}
		return result
	})

	// requestAnimationFrame / cancelAnimationFrame
	frameID := 0
	vm.Set("requestAnimationFrame", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			if cb, ok := sobek.AssertFunction(call.Arguments[0]); ok {
				frameID++
				id := frameID
				go func() {
					run := func() {
						_, _ = cb(sobek.Undefined(), vm.ToValue(float64(wm.startTime.UnixMilli())))
					}
					if wm.tasks != nil {
						wm.tasks <- run
					} else {
						run()
					}
				}()
				return vm.ToValue(id)
			}
		}
		return vm.ToValue(0)
	})

	vm.Set("cancelAnimationFrame", func(call sobek.FunctionCall) sobek.Value {
		return sobek.Undefined()
	})

	// requestIdleCallback / cancelIdleCallback
	vm.Set("requestIdleCallback", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			if cb, ok := sobek.AssertFunction(call.Arguments[0]); ok {
				frameID++
				id := frameID
				go func() {
					run := func() {
						deadline := vm.NewObject()
						_ = deadline.Set("didTimeout", false)
						_ = deadline.Set("timeRemaining", func(sobek.FunctionCall) sobek.Value {
							return vm.ToValue(50) // 50ms remaining
						})
						_, _ = cb(sobek.Undefined(), deadline)
					}
					if wm.tasks != nil {
						wm.tasks <- run
					} else {
						run()
					}
				}()
				return vm.ToValue(id)
			}
		}
		return vm.ToValue(0)
	})

	vm.Set("cancelIdleCallback", func(call sobek.FunctionCall) sobek.Value {
		return sobek.Undefined()
	})

	// location (minimal)
	location := vm.NewObject()
	_ = location.Set("href", "about:blank")
	_ = location.Set("protocol", "about:")
	_ = location.Set("host", "")
	_ = location.Set("hostname", "")
	_ = location.Set("port", "")
	_ = location.Set("pathname", "blank")
	_ = location.Set("search", "")
	_ = location.Set("hash", "")
	_ = location.Set("origin", "null")
	_ = location.Set("reload", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	_ = location.Set("replace", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	_ = location.Set("assign", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	vm.Set("location", location)

	// history (minimal)
	history := vm.NewObject()
	_ = history.Set("length", 1)
	_ = history.Set("state", sobek.Null())
	_ = history.Set("pushState", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	_ = history.Set("replaceState", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	_ = history.Set("go", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	_ = history.Set("back", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	_ = history.Set("forward", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	vm.Set("history", history)

	// screen (minimal)
	screen := vm.NewObject()
	_ = screen.Set("width", 1920)
	_ = screen.Set("height", 1080)
	_ = screen.Set("availWidth", 1920)
	_ = screen.Set("availHeight", 1080)
	_ = screen.Set("colorDepth", 24)
	_ = screen.Set("pixelDepth", 24)
	_ = screen.Set("orientation", vm.NewObject())
	vm.Set("screen", screen)

	// devicePixelRatio
	vm.Set("devicePixelRatio", 1.0)

	// innerWidth, innerHeight, outerWidth, outerHeight
	vm.Set("innerWidth", 1920)
	vm.Set("innerHeight", 1080)
	vm.Set("outerWidth", 1920)
	vm.Set("outerHeight", 1080)
	vm.Set("scrollX", 0)
	vm.Set("scrollY", 0)
	vm.Set("pageXOffset", 0)
	vm.Set("pageYOffset", 0)

	// scroll functions (no-op)
	vm.Set("scroll", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	vm.Set("scrollTo", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	vm.Set("scrollBy", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })

	// alert, confirm, prompt (no-op)
	vm.Set("alert", func(sobek.FunctionCall) sobek.Value {
		return sobek.Undefined()
	})
	vm.Set("confirm", func(sobek.FunctionCall) sobek.Value {
		return vm.ToValue(false)
	})
	vm.Set("prompt", func(sobek.FunctionCall) sobek.Value {
		return sobek.Null()
	})

	// print (no-op)
	vm.Set("print", func(sobek.FunctionCall) sobek.Value {
		return sobek.Undefined()
	})

	// focus, blur (no-op)
	vm.Set("focus", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	vm.Set("blur", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })

	// open, close (minimal)
	vm.Set("open", func(sobek.FunctionCall) sobek.Value { return sobek.Null() })
	vm.Set("close", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })

	// postMessage (no-op for top-level)
	vm.Set("postMessage", func(sobek.FunctionCall) sobek.Value {
		return sobek.Undefined()
	})

	// origin (configurable)
	vm.Set("origin", wm.origin)

	// isSecureContext
	vm.Set("isSecureContext", true)

	// crossOriginIsolated
	vm.Set("crossOriginIsolated", false)

	return nil
}
