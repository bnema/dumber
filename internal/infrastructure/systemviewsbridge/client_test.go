package systemviewsbridge

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
)

func TestBuildMessageEnvelope(t *testing.T) {
	t.Parallel()

	payload := struct {
		RequestID string `json:"requestId"`
		Limit     int    `json:"limit"`
	}{RequestID: "req-1", Limit: 25}

	got, err := buildMessageEnvelope("history_timeline", payload)
	if err != nil {
		t.Fatalf("buildMessageEnvelope() error = %v", err)
	}

	var msg port.WebUIMessage
	if err := json.Unmarshal(got, &msg); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}

	if msg.Type != "history_timeline" {
		t.Fatalf("msg.Type = %q, want %q", msg.Type, "history_timeline")
	}

	if gotPayload := string(msg.Payload); gotPayload != `{"requestId":"req-1","limit":25}` {
		t.Fatalf("msg.Payload = %s, want %s", gotPayload, `{"requestId":"req-1","limit":25}`)
	}
}

func TestClientSendPrefersNativeTransport(t *testing.T) {
	t.Parallel()

	native := &fakeTransport{available: true, response: []byte(`{"transport":"native"}`)}
	fetch := &fakeTransport{available: true, response: []byte(`{"transport":"fetch"}`)}

	client := NewClient(native, fetch)
	got, err := client.Send(context.Background(), "favorite_list", struct {
		RequestID string `json:"requestId"`
	}{RequestID: "req-2"})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if string(got) != `{"transport":"native"}` {
		t.Fatalf("Send() = %s, want %s", got, `{"transport":"native"}`)
	}

	if !native.called {
		t.Fatal("native transport was not used")
	}
	if fetch.called {
		t.Fatal("fetch transport was used unexpectedly")
	}

	var msg port.WebUIMessage
	if err := json.Unmarshal(native.last, &msg); err != nil {
		t.Fatalf("unmarshal sent envelope: %v", err)
	}
	if msg.Type != "favorite_list" {
		t.Fatalf("sent type = %q, want %q", msg.Type, "favorite_list")
	}
}

func TestClientSendFallsBackToFetchTransport(t *testing.T) {
	t.Parallel()

	native := &fakeTransport{available: false, response: []byte(`{"transport":"native"}`)}
	fetch := &fakeTransport{available: true, response: []byte(`{"transport":"fetch"}`)}

	client := NewClient(native, fetch)
	got, err := client.Send(context.Background(), "favorite_list", struct {
		RequestID string `json:"requestId"`
	}{RequestID: "req-3"})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if string(got) != `{"transport":"fetch"}` {
		t.Fatalf("Send() = %s, want %s", got, `{"transport":"fetch"}`)
	}

	if native.called {
		t.Fatal("native transport was used unexpectedly")
	}
	if !fetch.called {
		t.Fatal("fetch transport was not used")
	}
}

type fakeTransport struct {
	available bool
	called    bool
	last      []byte
	response  []byte
}

func (t *fakeTransport) Available() bool { return t.available }

func (t *fakeTransport) Send(_ context.Context, body []byte) ([]byte, error) {
	t.called = true
	t.last = append(t.last[:0], body...)
	return t.response, nil
}
