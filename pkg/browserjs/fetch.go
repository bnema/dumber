package browserjs

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/grafana/sobek"
)

// FetchHandler is called for custom URL schemes (e.g., extension://).
// Return nil to fall through to normal HTTP fetch.
type FetchHandler func(url string) *FetchResult

// FetchResult represents the result of a custom fetch handler.
type FetchResult struct {
	Body       []byte
	Status     int
	StatusText string
	Headers    map[string]string
	Error      error
}

// FetchManager provides fetch, Headers, Request, Response APIs.
type FetchManager struct {
	vm         *sobek.Runtime
	httpClient HTTPDoer
	tasks      chan func()
	handlers   []FetchHandler
}

// NewFetchManager creates a new fetch manager.
func NewFetchManager(vm *sobek.Runtime, httpClient HTTPDoer, tasks chan func()) *FetchManager {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &FetchManager{
		vm:         vm,
		httpClient: httpClient,
		tasks:      tasks,
	}
}

// RegisterHandler adds a custom URL handler (e.g., for extension:// URLs).
func (fm *FetchManager) RegisterHandler(handler FetchHandler) {
	fm.handlers = append(fm.handlers, handler)
}

// Install registers fetch-related globals on the VM.
func (fm *FetchManager) Install() error {
	fm.installHeaders()
	fm.installRequest()
	fm.installResponse()
	fm.installFetch()
	return nil
}

func (fm *FetchManager) installHeaders() {
	vm := fm.vm
	vm.Set("Headers", func(call sobek.ConstructorCall) *sobek.Object {
		headers := make(map[string]string)
		if len(call.Arguments) > 0 {
			if init := call.Arguments[0].Export(); init != nil {
				if m, ok := init.(map[string]interface{}); ok {
					for k, v := range m {
						headers[strings.ToLower(k)] = fmt.Sprintf("%v", v)
					}
				}
			}
		}
		obj := call.This
		_ = obj.Set("append", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 2 {
				name := strings.ToLower(call.Arguments[0].String())
				value := call.Arguments[1].String()
				if existing, ok := headers[name]; ok {
					headers[name] = existing + ", " + value
				} else {
					headers[name] = value
				}
			}
			return sobek.Undefined()
		})
		_ = obj.Set("delete", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 1 {
				delete(headers, strings.ToLower(call.Arguments[0].String()))
			}
			return sobek.Undefined()
		})
		_ = obj.Set("get", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 1 {
				if v, ok := headers[strings.ToLower(call.Arguments[0].String())]; ok {
					return vm.ToValue(v)
				}
			}
			return sobek.Null()
		})
		_ = obj.Set("has", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 1 {
				_, ok := headers[strings.ToLower(call.Arguments[0].String())]
				return vm.ToValue(ok)
			}
			return vm.ToValue(false)
		})
		_ = obj.Set("set", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 2 {
				headers[strings.ToLower(call.Arguments[0].String())] = call.Arguments[1].String()
			}
			return sobek.Undefined()
		})
		_ = obj.Set("forEach", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) >= 1 {
				if cb, ok := sobek.AssertFunction(call.Arguments[0]); ok {
					for k, v := range headers {
						_, _ = cb(sobek.Undefined(), vm.ToValue(v), vm.ToValue(k), obj)
					}
				}
			}
			return sobek.Undefined()
		})
		return nil
	})
}

func (fm *FetchManager) installRequest() {
	vm := fm.vm
	vm.Set("Request", func(call sobek.ConstructorCall) *sobek.Object {
		url := ""
		method := "GET"
		reqHeaders := make(map[string]string)
		var body string

		if len(call.Arguments) > 0 {
			url = call.Arguments[0].String()
		}
		if len(call.Arguments) > 1 {
			if opts := call.Arguments[1].Export(); opts != nil {
				if m, ok := opts.(map[string]interface{}); ok {
					if meth, ok := m["method"].(string); ok {
						method = strings.ToUpper(meth)
					}
					if b, ok := m["body"].(string); ok {
						body = b
					}
					if hdrs, ok := m["headers"].(map[string]interface{}); ok {
						for k, v := range hdrs {
							reqHeaders[strings.ToLower(k)] = fmt.Sprintf("%v", v)
						}
					}
				}
			}
		}

		obj := call.This
		_ = obj.Set("url", url)
		_ = obj.Set("method", method)
		_ = obj.Set("headers", reqHeaders)
		_ = obj.Set("body", body)
		_ = obj.Set("clone", func(sobek.FunctionCall) sobek.Value {
			return obj
		})
		return nil
	})
}

func (fm *FetchManager) installResponse() {
	vm := fm.vm
	tasks := fm.tasks

	vm.Set("Response", func(call sobek.ConstructorCall) *sobek.Object {
		var body []byte
		if len(call.Arguments) > 0 && call.Arguments[0] != sobek.Null() && call.Arguments[0] != sobek.Undefined() {
			switch v := call.Arguments[0].Export().(type) {
			case string:
				body = []byte(v)
			case []byte:
				body = v
			}
		}
		status := 200
		statusText := "OK"
		if len(call.Arguments) > 1 {
			if opts := call.Arguments[1].Export(); opts != nil {
				if m, ok := opts.(map[string]interface{}); ok {
					if s, ok := m["status"].(int64); ok {
						status = int(s)
					}
					if s, ok := m["statusText"].(string); ok {
						statusText = s
					}
				}
			}
		}

		obj := call.This
		_ = obj.Set("ok", status >= 200 && status < 300)
		_ = obj.Set("status", status)
		_ = obj.Set("statusText", statusText)
		_ = obj.Set("bodyUsed", false)

		_ = obj.Set("text", func(sobek.FunctionCall) sobek.Value {
			promise, resolve, _ := vm.NewPromise()
			go func() {
				if tasks != nil {
					tasks <- func() { _ = resolve(vm.ToValue(string(body))) }
				} else {
					_ = resolve(vm.ToValue(string(body)))
				}
			}()
			return vm.ToValue(promise)
		})
		_ = obj.Set("json", func(sobek.FunctionCall) sobek.Value {
			promise, resolve, reject := vm.NewPromise()
			go func() {
				run := func() {
					result, err := vm.RunString(fmt.Sprintf("JSON.parse(%q)", string(body)))
					if err != nil {
						_ = reject(vm.ToValue(err.Error()))
					} else {
						_ = resolve(result)
					}
				}
				if tasks != nil {
					tasks <- run
				} else {
					run()
				}
			}()
			return vm.ToValue(promise)
		})
		_ = obj.Set("arrayBuffer", func(sobek.FunctionCall) sobek.Value {
			promise, resolve, _ := vm.NewPromise()
			go func() {
				if tasks != nil {
					tasks <- func() { _ = resolve(vm.ToValue(body)) }
				} else {
					_ = resolve(vm.ToValue(body))
				}
			}()
			return vm.ToValue(promise)
		})
		_ = obj.Set("blob", func(sobek.FunctionCall) sobek.Value {
			promise, resolve, _ := vm.NewPromise()
			go func() {
				run := func() {
					blob := vm.NewObject()
					_ = blob.Set("size", len(body))
					_ = blob.Set("type", "")
					_ = resolve(blob)
				}
				if tasks != nil {
					tasks <- run
				} else {
					run()
				}
			}()
			return vm.ToValue(promise)
		})
		_ = obj.Set("clone", func(sobek.FunctionCall) sobek.Value {
			cloned := vm.NewObject()
			_ = cloned.Set("ok", status >= 200 && status < 300)
			_ = cloned.Set("status", status)
			_ = cloned.Set("statusText", statusText)
			return cloned
		})
		return nil
	})
}

func (fm *FetchManager) installFetch() {
	vm := fm.vm
	tasks := fm.tasks

	vm.Set("fetch", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("fetch requires a URL"))
		}

		url := call.Arguments[0].String()
		method := "GET"
		var reqBody io.Reader
		reqHeaders := make(map[string]string)

		// Check custom handlers first
		for _, handler := range fm.handlers {
			if result := handler(url); result != nil {
				return fm.handleCustomResult(url, result)
			}
		}

		if len(call.Arguments) > 1 {
			if opts := call.Arguments[1].Export(); opts != nil {
				if m, ok := opts.(map[string]interface{}); ok {
					if meth, ok := m["method"].(string); ok {
						method = strings.ToUpper(meth)
					}
					if body, ok := m["body"].(string); ok {
						reqBody = strings.NewReader(body)
					}
					if hdrs, ok := m["headers"].(map[string]interface{}); ok {
						for k, v := range hdrs {
							reqHeaders[k] = fmt.Sprintf("%v", v)
						}
					}
				}
			}
		}

		promise, resolve, reject := vm.NewPromise()
		go func() {
			req, err := http.NewRequest(method, url, reqBody)
			if err != nil {
				fm.rejectPromise(reject, err.Error())
				return
			}

			for k, v := range reqHeaders {
				req.Header.Set(k, v)
			}

			resp, err := fm.httpClient.Do(req)
			if err != nil {
				fm.rejectPromise(reject, err.Error())
				return
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				fm.rejectPromise(reject, err.Error())
				return
			}

			run := func() {
				respObj := fm.createResponseObject(url, resp.StatusCode, resp.Status, body, resp.Header)
				_ = resolve(respObj)
			}
			if tasks != nil {
				tasks <- run
			} else {
				run()
			}
		}()

		return vm.ToValue(promise)
	})
}

func (fm *FetchManager) handleCustomResult(url string, result *FetchResult) sobek.Value {
	vm := fm.vm
	tasks := fm.tasks
	promise, resolve, reject := vm.NewPromise()

	go func() {
		if result.Error != nil {
			fm.rejectPromise(reject, result.Error.Error())
			return
		}

		run := func() {
			status := result.Status
			if status == 0 {
				status = 200
			}
			statusText := result.StatusText
			if statusText == "" {
				statusText = "OK"
			}
			respObj := fm.createResponseObject(url, status, statusText, result.Body, nil)
			_ = resolve(respObj)
		}
		if tasks != nil {
			tasks <- run
		} else {
			run()
		}
	}()

	return vm.ToValue(promise)
}

func (fm *FetchManager) rejectPromise(reject func(interface{}) error, msg string) {
	if fm.tasks != nil {
		fm.tasks <- func() { _ = reject(fm.vm.ToValue(msg)) }
	} else {
		_ = reject(fm.vm.ToValue(msg))
	}
}

func (fm *FetchManager) createResponseObject(url string, status int, statusText string, body []byte, headers http.Header) *sobek.Object {
	vm := fm.vm
	tasks := fm.tasks

	respObj := vm.NewObject()
	_ = respObj.Set("ok", status >= 200 && status < 300)
	_ = respObj.Set("status", status)
	_ = respObj.Set("statusText", statusText)
	_ = respObj.Set("url", url)

	bodyBytes := body
	_ = respObj.Set("text", func(sobek.FunctionCall) sobek.Value {
		p, res, _ := vm.NewPromise()
		go func() {
			if tasks != nil {
				tasks <- func() { _ = res(vm.ToValue(string(bodyBytes))) }
			} else {
				_ = res(vm.ToValue(string(bodyBytes)))
			}
		}()
		return vm.ToValue(p)
	})
	_ = respObj.Set("json", func(sobek.FunctionCall) sobek.Value {
		p, res, rej := vm.NewPromise()
		go func() {
			run := func() {
				result, err := vm.RunString(fmt.Sprintf("JSON.parse(%q)", string(bodyBytes)))
				if err != nil {
					_ = rej(vm.ToValue(err.Error()))
				} else {
					_ = res(result)
				}
			}
			if tasks != nil {
				tasks <- run
			} else {
				run()
			}
		}()
		return vm.ToValue(p)
	})
	_ = respObj.Set("arrayBuffer", func(sobek.FunctionCall) sobek.Value {
		p, res, _ := vm.NewPromise()
		go func() {
			if tasks != nil {
				tasks <- func() { _ = res(vm.ToValue(bodyBytes)) }
			} else {
				_ = res(vm.ToValue(bodyBytes))
			}
		}()
		return vm.ToValue(p)
	})

	// Headers object
	hdrs := vm.NewObject()
	if headers != nil {
		for k, v := range headers {
			_ = hdrs.Set(strings.ToLower(k), strings.Join(v, ", "))
		}
		_ = hdrs.Set("get", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				name := strings.ToLower(call.Arguments[0].String())
				if v := headers.Get(name); v != "" {
					return vm.ToValue(v)
				}
			}
			return sobek.Null()
		})
	} else {
		_ = hdrs.Set("get", func(call sobek.FunctionCall) sobek.Value {
			return sobek.Null()
		})
	}
	_ = respObj.Set("headers", hdrs)

	return respObj
}
