package webkit

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/infrastructure/webutil"
)

type recordedResponse struct {
	webviewID WebViewID
	callback  string
	payload   string
}

// newTestDispatchRouter wires the async seams: goroutines are real, the
// "main thread" is a queue drained by the test.
func newTestDispatchRouter(t *testing.T) (*MessageRouter, chan func(), *[]recordedResponse, *sync.Mutex) {
	t.Helper()
	r := NewMessageRouter(context.Background())
	mainQueue := make(chan func(), 16)
	r.onMain = func(f func()) { mainQueue <- f }
	r.stampFor = func(WebViewID) (deliveryStamp, bool) { return deliveryStamp{view: "page"}, true }
	var mu sync.Mutex
	var got []recordedResponse
	r.respond = func(_ context.Context, id WebViewID, callback, _ string, data json.RawMessage) error {
		mu.Lock()
		got = append(got, recordedResponse{webviewID: id, callback: callback, payload: string(data)})
		mu.Unlock()
		return nil
	}
	return r, mainQueue, &got, &mu
}

func TestRunHandlerDoesNotBlockOnSlowHandler(t *testing.T) {
	r, mainQueue, got, mu := newTestDispatchRouter(t)
	release := make(chan struct{})
	entry := handlerEntry{
		callback:      "__ok",
		errorCallback: "__err",
		handler: port.WebUIMessageHandlerFunc(func(context.Context, port.WebViewID, json.RawMessage) (any, error) {
			<-release
			return map[string]int{"n": 1}, nil
		}),
	}

	returned := make(chan struct{})
	go func() {
		r.runHandler(context.Background(), entry, Message{Type: "home_widget_data", WebViewID: 7, Payload: json.RawMessage(`{"requestId":"r1"}`)})
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("runHandler blocked on a slow handler")
	}

	close(release)
	select {
	case f := <-mainQueue:
		f()
	case <-time.After(2 * time.Second):
		t.Fatal("response never scheduled on the main thread")
	}
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, *got, 1)
	assert.Equal(t, WebViewID(7), (*got)[0].webviewID)
	assert.Equal(t, "__ok", (*got)[0].callback)
	assert.JSONEq(t, `{"requestId":"r1","success":true,"data":{"n":1}}`, (*got)[0].payload)
}

func TestRunHandlerOutOfOrderResponsesKeepRequestIDs(t *testing.T) {
	r, mainQueue, got, mu := newTestDispatchRouter(t)
	gates := map[string]chan struct{}{"slow": make(chan struct{}), "fast": make(chan struct{})}
	entry := handlerEntry{
		callback: "__ok",
		handler: port.WebUIMessageHandlerFunc(func(_ context.Context, _ port.WebViewID, p json.RawMessage) (any, error) {
			id := bridgeRequestID(p)
			<-gates[id]
			return id, nil
		}),
	}
	r.runHandler(context.Background(), entry, Message{Type: "home_widget_data", WebViewID: 1, Payload: json.RawMessage(`{"requestId":"slow"}`)})
	r.runHandler(context.Background(), entry, Message{Type: "home_widget_data", WebViewID: 1, Payload: json.RawMessage(`{"requestId":"fast"}`)})
	close(gates["fast"])
	(<-mainQueue)()
	close(gates["slow"])
	(<-mainQueue)()

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, *got, 2)
	assert.JSONEq(t, `{"requestId":"fast","success":true,"data":"fast"}`, (*got)[0].payload)
	assert.JSONEq(t, `{"requestId":"slow","success":true,"data":"slow"}`, (*got)[1].payload)
}

func TestRunHandlerErrorsAndExistingEnvelopes(t *testing.T) {
	r, mainQueue, got, mu := newTestDispatchRouter(t)
	failing := handlerEntry{callback: "__ok", errorCallback: "__err", handler: port.WebUIMessageHandlerFunc(
		func(context.Context, port.WebViewID, json.RawMessage) (any, error) { return nil, errors.New("boom") })}
	enveloped := handlerEntry{callback: "__ok", handler: port.WebUIMessageHandlerFunc(
		func(context.Context, port.WebViewID, json.RawMessage) (any, error) {
			return map[string]any{"requestId": "r3", "success": true, "data": 5}, nil
		})}
	panicking := handlerEntry{callback: "__ok", errorCallback: "__err", handler: port.WebUIMessageHandlerFunc(
		func(context.Context, port.WebViewID, json.RawMessage) (any, error) { panic("oops") })}
	noID := handlerEntry{callback: "__ok", handler: port.WebUIMessageHandlerFunc(
		func(context.Context, port.WebViewID, json.RawMessage) (any, error) { return []int{1}, nil })}

	for _, tc := range []struct {
		entry   handlerEntry
		payload string
	}{
		{failing, `{"requestId":"r2"}`},
		{enveloped, `{"requestId":"r3"}`},
		{panicking, `{"requestId":"r4"}`},
		{noID, `{}`},
	} {
		r.runHandler(context.Background(), tc.entry, Message{Type: "x", WebViewID: 1, Payload: json.RawMessage(tc.payload)})
		(<-mainQueue)()
	}

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, *got, 4)
	assert.Equal(t, "__err", (*got)[0].callback)
	assert.JSONEq(t, `{"requestId":"r2","error":"boom"}`, (*got)[0].payload)
	assert.JSONEq(t, `{"requestId":"r3","success":true,"data":5}`, (*got)[1].payload, "existing envelope is not wrapped again")
	assert.Equal(t, "__err", (*got)[2].callback)
	assert.Contains(t, (*got)[2].payload, "panicked")
	assert.JSONEq(t, `[1]`, (*got)[3].payload, "requests without requestId keep the raw payload")
}

func TestRunHandlerSyncTypesStayInline(t *testing.T) {
	r, mainQueue, got, mu := newTestDispatchRouter(t)
	r.goAsync = func(func()) { t.Fatal("sync message type dispatched to a goroutine") }
	entry := handlerEntry{callback: "__ok", handler: port.WebUIMessageHandlerFunc(
		func(context.Context, port.WebViewID, json.RawMessage) (any, error) { return "ok", nil })}
	for _, msgType := range webutil.InlineBridgeMessageTypes() {
		r.runHandler(context.Background(), entry, Message{Type: msgType, WebViewID: 1})
	}
	assert.Empty(t, mainQueue)
	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, *got, len(webutil.InlineBridgeMessageTypes()))
}

func TestRunHandlerDropsResponseWhenThePageChanged(t *testing.T) {
	for _, tc := range []struct {
		name  string
		after deliveryStamp
		ok    bool
	}{
		{"navigated to another page", deliveryStamp{view: "wv", generation: 1, navigation: 2}, true},
		{"pool reuse", deliveryStamp{view: "wv", generation: 2, navigation: 1}, true},
		{"same id, different view", deliveryStamp{view: "other", generation: 1, navigation: 1}, true},
		{"no longer a trusted system view", deliveryStamp{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, mainQueue, got, mu := newTestDispatchRouter(t)
			current, ok := deliveryStamp{view: "wv", generation: 1, navigation: 1}, true
			var stampMu sync.Mutex
			r.stampFor = func(WebViewID) (deliveryStamp, bool) {
				stampMu.Lock()
				defer stampMu.Unlock()
				return current, ok
			}
			release := make(chan struct{})
			entry := handlerEntry{callback: "__ok", errorCallback: "__err", handler: port.WebUIMessageHandlerFunc(
				func(context.Context, port.WebViewID, json.RawMessage) (any, error) {
					<-release
					return "secret", nil
				})}
			r.runHandler(context.Background(), entry, Message{Type: "home_widget_data", WebViewID: 3, Payload: json.RawMessage(`{"requestId":"r"}`)})

			stampMu.Lock()
			current, ok = tc.after, tc.ok
			stampMu.Unlock()
			close(release)
			(<-mainQueue)()

			mu.Lock()
			defer mu.Unlock()
			assert.Empty(t, *got, "no callback may run for a page that changed")
		})
	}
}

func TestRunHandlerIgnoresRequestsFromUntrustedPages(t *testing.T) {
	r := NewMessageRouter(context.Background())
	r.stampFor = func(WebViewID) (deliveryStamp, bool) { return deliveryStamp{}, false }
	r.goAsync = func(func()) { t.Fatal("handler must not run") }
	entry := handlerEntry{callback: "__ok", handler: port.WebUIMessageHandlerFunc(
		func(context.Context, port.WebViewID, json.RawMessage) (any, error) { return nil, nil })}
	r.runHandler(context.Background(), entry, Message{Type: "home_widget_data", WebViewID: 3})
}
