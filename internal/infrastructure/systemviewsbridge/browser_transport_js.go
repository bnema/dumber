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
	case "history_timeline", "history_timeline_by_domain", "history_timeline_window", "history_search_fts", "history_delete_entry", "history_delete_range", "history_stats", "history_analytics",
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
	bridge := window.Get("dumber")
	return bridge.Truthy() && bridge.Get("postMessage").Truthy()
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

type webkitCallbackResult struct {
	body []byte
	err  error
}

type webkitCallbackWaiter struct {
	requestID   string
	successName string
	failureName string
	resultCh    chan webkitCallbackResult
}

// webkitCallbacks uses one page-lifetime dispatcher per native callback name.
// Requests sharing the same callback names are intentionally serialized because
// older native shims do not always echo requestId; late callbacks without a
// matching waiter are dropped instead of targeting released js.Func handles.
var webkitCallbacks = struct {
	sync.Mutex
	installed map[string]js.Func
	waiters   map[string]*webkitCallbackWaiter
}{
	installed: make(map[string]js.Func),
	waiters:   make(map[string]*webkitCallbackWaiter),
}

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

	ensureWebKitCallback(window, plan.success, false)
	ensureWebKitCallback(window, plan.failure, true)

	resultCh := make(chan webkitCallbackResult, 1)
	unregister, err := registerWebKitWaiter(req.requestID, plan, resultCh)
	if err != nil {
		return nil, err
	}

	handler.Call("postMessage", js.Global().Get("JSON").Call("parse", string(req.body)))

	select {
	case res := <-resultCh:
		return res.body, res.err
	case <-ctx.Done():
		unregister()
		return nil, ctx.Err()
	}
}

func ensureWebKitCallback(window js.Value, name string, failure bool) {
	webkitCallbacks.Lock()
	if _, ok := webkitCallbacks.installed[name]; ok {
		webkitCallbacks.Unlock()
		return
	}
	callback := js.FuncOf(func(_ js.Value, args []js.Value) any {
		handleWebKitCallback(name, failure, args)
		return nil
	})
	webkitCallbacks.installed[name] = callback
	// Installed callbacks are page-lifetime dispatchers. Per-request waiters are
	// removed on completion/cancellation, so late native responses are dropped
	// without invoking released js.Func handles.
	window.Set(name, callback)
	webkitCallbacks.Unlock()
}

func registerWebKitWaiter(requestID string, plan callbackPlan, resultCh chan webkitCallbackResult) (func(), error) {
	waiter := &webkitCallbackWaiter{
		requestID:   requestID,
		successName: plan.success,
		failureName: plan.failure,
		resultCh:    resultCh,
	}
	webkitCallbacks.Lock()
	defer webkitCallbacks.Unlock()
	if webkitCallbacks.waiters[plan.success] != nil || webkitCallbacks.waiters[plan.failure] != nil {
		return nil, fmt.Errorf("webkit callback already pending for %s", plan.success)
	}
	webkitCallbacks.waiters[plan.success] = waiter
	webkitCallbacks.waiters[plan.failure] = waiter
	return func() {
		webkitCallbacks.Lock()
		defer webkitCallbacks.Unlock()
		if webkitCallbacks.waiters[plan.success] == waiter {
			delete(webkitCallbacks.waiters, plan.success)
		}
		if webkitCallbacks.waiters[plan.failure] == waiter {
			delete(webkitCallbacks.waiters, plan.failure)
		}
	}, nil
}

func handleWebKitCallback(name string, failure bool, args []js.Value) {
	payload, payloadErr := stringifyArgs(args)
	payloadRequestID := requestIDFromBridgePayload(payload)

	webkitCallbacks.Lock()
	waiter := webkitCallbacks.waiters[name]
	if waiter != nil && payloadRequestID != "" && waiter.requestID != "" && payloadRequestID != waiter.requestID {
		webkitCallbacks.Unlock()
		warnDroppedBridgeResult("webkit callback request mismatch", fmt.Errorf("got %s, want %s", payloadRequestID, waiter.requestID))
		return
	}
	if waiter != nil {
		delete(webkitCallbacks.waiters, waiter.successName)
		delete(webkitCallbacks.waiters, waiter.failureName)
	}
	webkitCallbacks.Unlock()

	if waiter == nil {
		warnDroppedBridgeResult("webkit callback without waiter", nil)
		return
	}

	res := webkitCallbackResult{err: payloadErr}
	if payloadErr == nil {
		if failure {
			res.body, res.err = normalizeNativeErrorResponse(waiter.requestID, payload)
		} else {
			res.body, res.err = normalizeNativeSuccessResponse(waiter.requestID, payload)
		}
	}
	select {
	case waiter.resultCh <- res:
	default:
		warnDroppedBridgeResult("webkit callback", res.err)
	}
}

func requestIDFromBridgePayload(payload []byte) string {
	if len(bytes.TrimSpace(payload)) == 0 {
		return ""
	}
	var envelope struct {
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return ""
	}
	return envelope.RequestID
}

func fetchDirectAPI(ctx context.Context, requestID, endpoint string) ([]byte, error) {
	fetch := js.Global().Get("fetch")
	if !fetch.Truthy() {
		return nil, errors.New("fetch API not available")
	}
	response, err := awaitPromise(ctx, fetch.Invoke(endpoint))
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
	if ctx == nil {
		ctx = context.Background()
	}
	type result struct {
		value js.Value
		err   error
	}
	resultCh := make(chan result, 1)

	promiseCtor := js.Global().Get("Promise")
	if !promiseCtor.Truthy() {
		return js.Undefined(), errors.New("Promise global not available")
	}
	var (
		cancelMu sync.Mutex
		reject   = js.Undefined()
	)
	executor := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) >= 2 {
			cancelMu.Lock()
			reject = args[1]
			cancelMu.Unlock()
		}
		return nil
	})
	cancelPromise := promiseCtor.New(executor)
	executor.Release()
	raceInput := js.Global().Get("Array").New(promise, cancelPromise)
	racePromise := promiseCtor.Call("race", raceInput)

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			cancelMu.Lock()
			rejectFn := reject
			cancelMu.Unlock()
			if rejectFn.Truthy() {
				rejectFn.Invoke(ctx.Err().Error())
			}
		case <-done:
		}
	}()
	defer close(done)

	var (
		callbackMu sync.Mutex
		released   bool
		thenCb     js.Func
		catchCb    js.Func
	)
	releaseCallbacks := func() {
		callbackMu.Lock()
		defer callbackMu.Unlock()
		if released {
			return
		}
		released = true
		thenCb.Release()
		catchCb.Release()
	}
	publish := func(res result) {
		select {
		case resultCh <- res:
		default:
			warnDroppedBridgeResult("promise", res.err)
		}
	}

	thenCb = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer releaseCallbacks()
		if len(args) == 0 {
			publish(result{value: js.Undefined()})
			return nil
		}
		publish(result{value: args[0]})
		return nil
	})
	catchCb = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer releaseCallbacks()
		if len(args) == 0 {
			publish(result{err: errors.New("promise rejected")})
			return nil
		}
		message := args[0].String()
		if ctxErr := ctx.Err(); ctxErr != nil && message == ctxErr.Error() {
			publish(result{err: ctxErr})
			return nil
		}
		publish(result{err: errors.New(message)})
		return nil
	})
	attachPromiseCallbacks(racePromise, thenCb, catchCb)

	res := <-resultCh
	return res.value, res.err
}

func attachPromiseCallbacks(promise js.Value, thenCb, catchCb js.Func) {
	promise.Call("then", thenCb, catchCb)
}

func warnDroppedBridgeResult(scope string, err error) {
	console := js.Global().Get("console")
	if !console.Truthy() || !console.Get("warn").Truthy() {
		return
	}
	if err != nil {
		console.Call("warn", "dumber systemviews bridge dropped "+scope+" result", err.Error())
		return
	}
	console.Call("warn", "dumber systemviews bridge dropped "+scope+" result")
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
	if stringified.Type() == js.TypeString {
		return []byte(stringified.String()), nil
	}
	if stringified.IsUndefined() && (value.IsUndefined() || value.IsNull()) {
		return []byte("null"), nil
	}
	return nil, errors.New("failed to stringify JS value")
}
