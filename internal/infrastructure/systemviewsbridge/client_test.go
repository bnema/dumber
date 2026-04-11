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

func TestClientTimelineDecodesEntries(t *testing.T) {
	t.Parallel()

	native := &fakeTransport{available: true, response: []byte(`{"requestId":"req-9","success":true,"data":[{"id":1,"url":"https://example.com","title":"Example"}]}`)}
	client := NewClient(native, nil)

	entries, err := client.Timeline(context.Background(), 25, 0)
	if err != nil {
		t.Fatalf("Timeline() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Timeline() len = %d, want 1", len(entries))
	}
	if entries[0].URL != "https://example.com" || entries[0].Title != "Example" {
		t.Fatalf("Timeline() entry = %+v", entries[0])
	}

	var msg port.WebUIMessage
	if err := json.Unmarshal(native.last, &msg); err != nil {
		t.Fatalf("unmarshal sent envelope: %v", err)
	}
	if msg.Type != "history_timeline" {
		t.Fatalf("sent type = %q, want %q", msg.Type, "history_timeline")
	}
	if gotPayload := string(msg.Payload); gotPayload == "" {
		t.Fatal("sent payload was empty")
	}
}

func TestClientDeleteRangeSendsRange(t *testing.T) {
	t.Parallel()

	native := &fakeTransport{available: true, response: []byte(`{"requestId":"req-10","success":true}`)}
	client := NewClient(native, nil)

	if err := client.DeleteRange(context.Background(), "week"); err != nil {
		t.Fatalf("DeleteRange() error = %v", err)
	}

	var msg port.WebUIMessage
	if err := json.Unmarshal(native.last, &msg); err != nil {
		t.Fatalf("unmarshal sent envelope: %v", err)
	}
	if msg.Type != "history_delete_range" {
		t.Fatalf("sent type = %q, want %q", msg.Type, "history_delete_range")
	}
}

func TestClientListDecodesFavorites(t *testing.T) {
	t.Parallel()

	native := &fakeTransport{available: true, response: []byte(`{"requestId":"req-11","success":true,"data":[{"id":1,"url":"https://example.com","title":"Example"}]}`)}
	client := NewClient(native, nil)

	favorites, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(favorites) != 1 {
		t.Fatalf("List() len = %d, want 1", len(favorites))
	}
	if favorites[0].URL != "https://example.com" || favorites[0].Title != "Example" {
		t.Fatalf("List() favorite = %+v", favorites[0])
	}

	var msg port.WebUIMessage
	if err := json.Unmarshal(native.last, &msg); err != nil {
		t.Fatalf("unmarshal sent envelope: %v", err)
	}
	if msg.Type != "favorite_list" {
		t.Fatalf("sent type = %q, want %q", msg.Type, "favorite_list")
	}
}

func TestClientListFoldersDecodesFolders(t *testing.T) {
	t.Parallel()

	native := &fakeTransport{available: true, response: []byte(`{"requestId":"req-12","success":true,"data":[{"id":9,"name":"Read Later"}]}`)}
	client := NewClient(native, nil)

	folders, err := client.ListFolders(context.Background())
	if err != nil {
		t.Fatalf("ListFolders() error = %v", err)
	}
	if len(folders) != 1 {
		t.Fatalf("ListFolders() len = %d, want 1", len(folders))
	}
	if folders[0].Name != "Read Later" {
		t.Fatalf("ListFolders() folder = %+v", folders[0])
	}

	var msg port.WebUIMessage
	if err := json.Unmarshal(native.last, &msg); err != nil {
		t.Fatalf("unmarshal sent envelope: %v", err)
	}
	if msg.Type != "folder_list" {
		t.Fatalf("sent type = %q, want %q", msg.Type, "folder_list")
	}
}

func TestClientListTagsDecodesTags(t *testing.T) {
	t.Parallel()

	native := &fakeTransport{available: true, response: []byte(`{"requestId":"req-13","success":true,"data":[{"id":5,"name":"Go","color":"#00ff00"}]}`)}
	client := NewClient(native, nil)

	tags, err := client.ListTags(context.Background())
	if err != nil {
		t.Fatalf("ListTags() error = %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("ListTags() len = %d, want 1", len(tags))
	}
	if tags[0].Name != "Go" || tags[0].Color != "#00ff00" {
		t.Fatalf("ListTags() tag = %+v", tags[0])
	}

	var msg port.WebUIMessage
	if err := json.Unmarshal(native.last, &msg); err != nil {
		t.Fatalf("unmarshal sent envelope: %v", err)
	}
	if msg.Type != "tag_list" {
		t.Fatalf("sent type = %q, want %q", msg.Type, "tag_list")
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
