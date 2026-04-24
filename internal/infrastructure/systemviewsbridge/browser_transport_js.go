//go:build js && wasm

package systemviewsbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"syscall/js"
)

type callbackPlan struct {
	success string
	failure string
}

func NewBrowserClient() *Client {
	return NewClient(&webkitTransport{}, &browserTransport{})
}

func callbackPlanForMessage(msgType string) (callbackPlan, bool) {
	switch msgType {
	case "history_timeline", "history_timeline_by_domain", "history_search_fts", "history_delete_entry", "history_delete_range", "history_analytics",
		"history_domain_stats", "history_delete_domain", "favorite_list", "favorite_create", "favorite_update", "favorite_delete", "folder_list", "tag_list",
		"favorite_set_shortcut", "favorite_set_folder", "folder_create", "folder_update", "folder_delete",
		"tag_create", "tag_update", "tag_delete", "tag_assign", "tag_remove":
		return callbackPlan{success: "__dumber_homepage_response", failure: "__dumber_error"}, true
	case "save_config":
		return callbackPlan{success: "__dumber_config_saved", failure: "__dumber_config_error"}, true
	case "get_keybindings":
		return callbackPlan{success: "__dumber_keybindings_loaded", failure: "__dumber_keybindings_error"}, true
	case "set_keybinding":
		return callbackPlan{success: "__dumber_keybinding_set", failure: "__dumber_keybinding_set_error"}, true
	case "reset_keybinding":
		return callbackPlan{success: "__dumber_keybinding_reset", failure: "__dumber_keybinding_reset_error"}, true
	case "reset_all_keybindings":
		return callbackPlan{success: "__dumber_keybindings_reset_all", failure: "__dumber_keybindings_reset_all_error"}, true
	default:
		return callbackPlan{}, false
	}
}

type browserTransport struct{}

func (*browserTransport) Available() bool {
	window := js.Global().Get("window")
	if !window.Truthy() {
		return false
	}
	return window.Get("fetch").Truthy() || window.Get("dumber").Truthy()
}

func (*browserTransport) Send(ctx context.Context, body []byte) ([]byte, error) {
	req, err := decodeBrowserRequest(body)
	if err != nil {
		return nil, err
	}
	if req.directAPI {
		return fetchDirectAPI(ctx, req.requestID, req.messageType)
	}

	window := js.Global().Get("window")
	bridge := window.Get("dumber")
	if !bridge.Truthy() || !bridge.Get("postMessage").Truthy() {
		return nil, errors.New("browser bridge shim not available")
	}

	promise := bridge.Call("postMessage", js.Global().Get("JSON").Call("parse", string(req.body)))
	value, err := awaitPromise(ctx, promise)
	if err != nil {
		return nil, err
	}
	raw, err := stringifyValue(value)
	if err != nil {
		return nil, err
	}
	return normalizeBridgeShimResponse(req.requestID, raw)
}

type webkitTransport struct{}

func (*webkitTransport) Available() bool {
	window := js.Global().Get("window")
	if !window.Truthy() {
		return false
	}
	webkit := window.Get("webkit")
	if !webkit.Truthy() {
		return false
	}
	handlers := webkit.Get("messageHandlers")
	if !handlers.Truthy() {
		return false
	}
	return handlers.Get("dumber").Truthy()
}

func (*webkitTransport) Send(ctx context.Context, body []byte) ([]byte, error) {
	req, err := decodeBrowserRequest(body)
	if err != nil {
		return nil, err
	}
	if req.directAPI {
		return fetchDirectAPI(ctx, req.requestID, req.messageType)
	}

	plan, ok := callbackPlanForMessage(req.messageType)
	if !ok {
		return nil, fmt.Errorf("no WebKit callback plan for message type %q", req.messageType)
	}

	window := js.Global().Get("window")
	handler := window.Get("webkit").Get("messageHandlers").Get("dumber")
	if !handler.Truthy() {
		return nil, errors.New("webkit message handler not available")
	}

	type result struct {
		body []byte
		err  error
	}
	resultCh := make(chan result, 1)
	var callbackMu sync.Mutex
	callbacksActive := true
	publish := func(res result) {
		callbackMu.Lock()
		active := callbacksActive
		callbackMu.Unlock()
		if !active {
			return
		}
		select {
		case resultCh <- res:
		default:
		}
	}

	successFn := js.FuncOf(func(_ js.Value, args []js.Value) any {
		payload, err := stringifyArgs(args)
		if err != nil {
			publish(result{err: err})
			return nil
		}
		normalized, err := normalizeNativeSuccessResponse(req.requestID, payload)
		publish(result{body: normalized, err: err})
		return nil
	})
	errorFn := js.FuncOf(func(_ js.Value, args []js.Value) any {
		payload, err := stringifyArgs(args)
		if err != nil {
			publish(result{err: err})
			return nil
		}
		normalized, err := normalizeNativeErrorResponse(req.requestID, payload)
		publish(result{body: normalized, err: err})
		return nil
	})

	previousSuccess := window.Get(plan.success)
	previousError := window.Get(plan.failure)
	window.Set(plan.success, successFn)
	window.Set(plan.failure, errorFn)
	defer func() {
		callbackMu.Lock()
		callbacksActive = false
		callbackMu.Unlock()
		window.Set(plan.success, previousSuccess)
		window.Set(plan.failure, previousError)
		successFn.Release()
		errorFn.Release()
	}()

	handler.Call("postMessage", js.Global().Get("JSON").Call("parse", string(req.body)))

	select {
	case res := <-resultCh:
		return res.body, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func fetchDirectAPI(ctx context.Context, requestID, endpoint string) ([]byte, error) {
	response, err := awaitPromise(ctx, js.Global().Call("fetch", endpoint))
	if err != nil {
		return nil, err
	}
	if !response.Get("ok").Bool() {
		status := response.Get("status").Int()
		statusText := response.Get("statusText").String()
		if statusText == "" {
			statusText = "request failed"
		}
		return nil, fmt.Errorf("direct API %s failed: %d %s", endpoint, status, statusText)
	}
	data, err := awaitPromise(ctx, response.Call("json"))
	if err != nil {
		return nil, err
	}
	raw, err := stringifyValue(data)
	if err != nil {
		return nil, err
	}
	return wrapDirectAPIResponse(requestID, raw)
}

func normalizeNativeSuccessResponse(requestID string, payload []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return json.Marshal(bridgeResponse{RequestID: requestID, Success: true})
	}

	var resp bridgeResponse
	if err := json.Unmarshal(trimmed, &resp); err == nil {
		hasEnvelope := resp.Success || resp.RequestID != "" || resp.Error != ""
		if hasEnvelope {
			if resp.RequestID == "" {
				resp.RequestID = requestID
			}
			return json.Marshal(resp)
		}
	}

	return json.Marshal(bridgeResponse{RequestID: requestID, Success: true, Data: json.RawMessage(trimmed)})
}

func normalizeNativeErrorResponse(requestID string, payload []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(payload)
	var message string
	if len(trimmed) == 0 {
		message = "bridge request failed"
	} else if err := json.Unmarshal(trimmed, &message); err != nil {
		message = strings.Trim(string(trimmed), `"`)
	}
	if message == "" {
		message = "bridge request failed"
	}
	return json.Marshal(bridgeResponse{RequestID: requestID, Error: message})
}

func awaitPromise(ctx context.Context, promise js.Value) (js.Value, error) {
	type result struct {
		value js.Value
		err   error
	}
	resultCh := make(chan result, 1)

	thenFn := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 {
			resultCh <- result{value: js.Undefined()}
			return nil
		}
		resultCh <- result{value: args[0]}
		return nil
	})
	catchFn := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 {
			resultCh <- result{err: errors.New("promise rejected")}
			return nil
		}
		resultCh <- result{err: errors.New(args[0].String())}
		return nil
	})
	defer thenFn.Release()
	defer catchFn.Release()

	promise.Call("then", thenFn).Call("catch", catchFn)

	select {
	case res := <-resultCh:
		return res.value, res.err
	case <-ctx.Done():
		return js.Undefined(), ctx.Err()
	}
}

func stringifyArgs(args []js.Value) ([]byte, error) {
	if len(args) == 0 {
		return nil, nil
	}
	return stringifyValue(args[0])
}

func stringifyValue(value js.Value) ([]byte, error) {
	jsonValue := js.Global().Get("JSON")
	if !jsonValue.Truthy() {
		return nil, errors.New("JSON global not available")
	}
	stringified := jsonValue.Call("stringify", value)
	if !stringified.Truthy() {
		return nil, errors.New("failed to stringify JS value")
	}
	return []byte(stringified.String()), nil
}
