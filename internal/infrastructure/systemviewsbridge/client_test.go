package systemviewsbridge

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	systemviewsbridgemocks "github.com/bnema/dumber/internal/infrastructure/systemviewsbridge/mocks"
	"github.com/stretchr/testify/mock"
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

	native := newTransportRecorder(t, true, []byte(`{"transport":"native"}`))
	fetch := newTransportRecorder(t, true, []byte(`{"transport":"fetch"}`))

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

	native := newTransportRecorder(t, false, []byte(`{"transport":"native"}`))
	fetch := newTransportRecorder(t, true, []byte(`{"transport":"fetch"}`))

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

	native := newTransportRecorder(t, true, []byte(`{"requestId":"req-9","success":true,"data":[{"id":1,"url":"https://example.com","title":"Example"}]}`))
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

func TestClientTimelineWindowOmitsZeroCursor(t *testing.T) {
	t.Parallel()

	native := newTransportRecorder(t, true, []byte(`{"requestId":"req-window","success":true}`))
	client := NewClient(native, nil)

	_, err := client.TimelineWindow(context.Background(), time.Time{}, 0, "")
	if err != nil {
		t.Fatalf("TimelineWindow() error = %v", err)
	}

	var msg port.WebUIMessage
	if err := json.Unmarshal(native.last, &msg); err != nil {
		t.Fatalf("unmarshal sent envelope: %v", err)
	}
	if msg.Type != "history_timeline_window" {
		t.Fatalf("sent type = %q, want %q", msg.Type, "history_timeline_window")
	}
	var payload map[string]any
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if _, ok := payload["before"]; ok {
		t.Fatalf("zero TimelineWindow cursor should be omit before, payload = %s", msg.Payload)
	}
	if _, ok := payload["beforeId"]; ok {
		t.Fatalf("zero TimelineWindow cursor should omit beforeId, payload = %s", msg.Payload)
	}
}

func TestClientTimelineWindowSendsCompleteCursor(t *testing.T) {
	t.Parallel()

	native := newTransportRecorder(t, true, []byte(`{"requestId":"req-window","success":true}`))
	client := NewClient(native, nil)
	before := time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC)

	_, err := client.TimelineWindow(context.Background(), before, 42, "example.com")
	if err != nil {
		t.Fatalf("TimelineWindow() error = %v", err)
	}

	var msg port.WebUIMessage
	if err := json.Unmarshal(native.last, &msg); err != nil {
		t.Fatalf("unmarshal sent envelope: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["before"] != before.Format(time.RFC3339Nano) {
		t.Fatalf("before = %v, want %s", payload["before"], before.Format(time.RFC3339Nano))
	}
	if payload["beforeId"] != float64(42) {
		t.Fatalf("beforeId = %v, want 42", payload["beforeId"])
	}
}

func TestClientTimelineWindowRejectsPartialCursor(t *testing.T) {
	t.Parallel()

	native := newTransportRecorder(t, true, []byte(`{"requestId":"req-window","success":true}`))
	client := NewClient(native, nil)
	before := time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC)

	if _, err := client.TimelineWindow(context.Background(), before, 0, ""); err == nil {
		t.Fatal("TimelineWindow(before without beforeID) error = nil")
	}
	if _, err := client.TimelineWindow(context.Background(), time.Time{}, 42, ""); err == nil {
		t.Fatal("TimelineWindow(beforeID without before) error = nil")
	}
	if _, err := client.TimelineWindow(context.Background(), time.Time{}, -1, ""); err == nil {
		t.Fatal("TimelineWindow(negative beforeID without before) error = nil")
	}
	if native.called {
		t.Fatal("TimelineWindow sent request for invalid partial cursor")
	}
}

func TestClientDeleteRangeRejectsEmptyRange(t *testing.T) {
	t.Parallel()

	transport := newTransportRecorder(t, true, []byte(`{"requestId":"req-range-empty","success":true}`))
	client := NewClient(transport, nil)

	err := client.DeleteRange(context.Background(), "  ")
	if err == nil {
		t.Fatal("DeleteRange() error = nil, want validation error")
	}
	if transport.called {
		t.Fatal("DeleteRange() sent a request for an invalid range")
	}
}

func TestClientDeleteRangeSendsRange(t *testing.T) {
	t.Parallel()

	native := newTransportRecorder(t, true, []byte(`{"requestId":"req-10","success":true}`))
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

	native := newTransportRecorder(t, true, []byte(`{"requestId":"req-11","success":true,"data":[{"id":1,"url":"https://example.com","title":"Example"}]}`))
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

	native := newTransportRecorder(t, true, []byte(`{"requestId":"req-12","success":true,"data":[{"id":9,"name":"Read Later"}]}`))
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

	native := newTransportRecorder(t, true, []byte(`{"requestId":"req-13","success":true,"data":[{"id":5,"name":"Go","color":"#00ff00"}]}`))
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

func TestClientCurrentAndDefaultDecodeConfigPayload(t *testing.T) {
	t.Parallel()

	resp := []byte(`{"requestId":"req-14","success":true,"data":{"engine_type":"webkit","default_search_engine":"DuckDuckGo"}}`)

	currentTransport := newTransportRecorder(t, true, resp)
	currentClient := NewClient(currentTransport, nil)
	current, err := currentClient.Current(context.Background())
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if current.EngineType != "webkit" || current.DefaultSearchEngine != "DuckDuckGo" {
		t.Fatalf("Current() = %+v", current)
	}

	var msg port.WebUIMessage
	if unmarshalErr := json.Unmarshal(currentTransport.last, &msg); unmarshalErr != nil {
		t.Fatalf("unmarshal sent envelope: %v", unmarshalErr)
	}
	if msg.Type != "/api/config" {
		t.Fatalf("sent type = %q, want %q", msg.Type, "/api/config")
	}

	defaultTransport := newTransportRecorder(t, true, resp)
	defaultClient := NewClient(defaultTransport, nil)
	defaultCfg, err := defaultClient.Default(context.Background())
	if err != nil {
		t.Fatalf("Default() error = %v", err)
	}
	if defaultCfg.EngineType != "webkit" || defaultCfg.DefaultSearchEngine != "DuckDuckGo" {
		t.Fatalf("Default() = %+v", defaultCfg)
	}

	if err := json.Unmarshal(defaultTransport.last, &msg); err != nil {
		t.Fatalf("unmarshal sent envelope: %v", err)
	}
	if msg.Type != "/api/config/default" {
		t.Fatalf("sent type = %q, want %q", msg.Type, "/api/config/default")
	}
}

func TestClientConfigActionsUseExpectedMessageTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		call    func(*testing.T, *Client) error
		want    string
		payload any
	}{
		{
			name: "save config",
			call: func(_ *testing.T, client *Client) error {
				return client.Save(context.Background(), dto.WebUIConfig{DefaultSearchEngine: "DuckDuckGo"})
			},
			want:    "save_config",
			payload: dto.WebUIConfig{DefaultSearchEngine: "DuckDuckGo"},
		},
		{
			name: "get keybindings",
			call: func(t *testing.T, client *Client) error {
				got, err := client.GetKeybindings(context.Background())
				if err != nil {
					return err
				}
				if len(got.Groups) != 1 || got.Groups[0].Mode != "default" {
					t.Fatalf("GetKeybindings() = %#v", got)
				}
				return nil
			},
			want: "get_keybindings",
		},
		{
			name: "set keybinding",
			call: func(t *testing.T, client *Client) error {
				got, err := client.SetKeybinding(context.Background(), port.SetKeybindingRequest{RequestID: "req-1", Mode: "default", Action: "open", Keys: []string{"ctrl+o"}})
				if err != nil {
					return err
				}
				if len(got.Conflicts) != 0 {
					t.Fatalf("SetKeybinding() = %#v", got)
				}
				return nil
			},
			want:    "set_keybinding",
			payload: port.SetKeybindingRequest{RequestID: "req-1", Mode: "default", Action: "open", Keys: []string{"ctrl+o"}},
		},
		{
			name: "reset keybinding",
			call: func(_ *testing.T, client *Client) error {
				return client.ResetKeybinding(context.Background(), port.ResetKeybindingRequest{RequestID: "req-2", Mode: "default", Action: "open"})
			},
			want:    "reset_keybinding",
			payload: port.ResetKeybindingRequest{RequestID: "req-2", Mode: "default", Action: "open"},
		},
		{
			name: "reset all keybindings",
			call: func(_ *testing.T, client *Client) error {
				return client.ResetAllKeybindings(context.Background())
			},
			want: "reset_all_keybindings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			transport := newTransportRecorder(t, true, []byte(`{"requestId":"req-15","success":true,"data":{"status":"success","conflicts":[]}}`))
			if tt.name == "save config" || tt.name == "reset keybinding" || tt.name == "reset all keybindings" {
				transport.response = []byte(`{"requestId":"req-15","success":true}`)
			}
			if tt.name == "get keybindings" {
				transport.response = []byte(`{"requestId":"req-15","success":true,"data":{"groups":[{"mode":"default","display_name":"Default","bindings":[{"action":"open","description":"Open","keys":["ctrl+o"],"default_keys":["ctrl+o"],"is_custom":false}]}]}}`)
			}
			client := NewClient(transport, nil)

			if err := tt.call(t, client); err != nil {
				t.Fatalf("call() error = %v", err)
			}

			var msg port.WebUIMessage
			if err := json.Unmarshal(transport.last, &msg); err != nil {
				t.Fatalf("unmarshal sent envelope: %v", err)
			}
			if msg.Type != tt.want {
				t.Fatalf("sent type = %q, want %q", msg.Type, tt.want)
			}
			if tt.payload != nil && len(msg.Payload) == 0 {
				t.Fatal("sent payload was empty")
			}
		})
	}
}

func TestClientRequestUsesDefaultTimeoutWhenCallerHasNoDeadline(t *testing.T) {
	t.Parallel()

	transport := &blockingTransport{available: true}
	client := NewClient(transport, nil, WithRequestTimeout(time.Millisecond))

	_, err := client.Timeline(context.Background(), 25, 0)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Timeline() error = %v, want context deadline exceeded", err)
	}
	if !transport.called {
		t.Fatal("transport was not called")
	}
}

func TestClientCreateFolderIncludesParentID(t *testing.T) {
	t.Parallel()

	parentID := int64(42)
	transport := newTransportRecorder(t, true, []byte(`{"requestId":"req-16","success":true,"data":{"id":9,"name":"Nested","icon":"📁","parent_id":42}}`))
	client := NewClient(transport, nil)

	folder, err := client.CreateFolder(context.Background(), "Nested", " 📁 ", &parentID)
	if err != nil {
		t.Fatalf("CreateFolder() error = %v", err)
	}
	if folder == nil {
		t.Fatal("CreateFolder() returned nil folder")
	}
	if folder.Name != "Nested" {
		t.Fatalf("CreateFolder() folder name = %q, want %q", folder.Name, "Nested")
	}
	if folder.Icon != "📁" {
		t.Fatalf("CreateFolder() folder icon = %q, want 📁", folder.Icon)
	}
	if folder.ParentID == nil || int64(*folder.ParentID) != parentID {
		t.Fatalf("CreateFolder() folder parent_id = %#v, want %d", folder.ParentID, parentID)
	}

	var msg port.WebUIMessage
	if err := json.Unmarshal(transport.last, &msg); err != nil {
		t.Fatalf("unmarshal sent envelope: %v", err)
	}
	if msg.Type != "folder_create" {
		t.Fatalf("sent type = %q, want %q", msg.Type, "folder_create")
	}

	var payload struct {
		RequestID string  `json:"requestId"`
		Name      string  `json:"name"`
		Icon      *string `json:"icon"`
		ParentID  *int64  `json:"parent_id"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatalf("unmarshal sent payload: %v", err)
	}
	if payload.ParentID == nil || *payload.ParentID != parentID {
		t.Fatalf("sent parent_id = %#v, want %d", payload.ParentID, parentID)
	}
	if payload.Name != "Nested" {
		t.Fatalf("sent name = %q, want %q", payload.Name, "Nested")
	}
	if payload.Icon == nil || *payload.Icon != "📁" {
		t.Fatalf("sent icon = %#v, want 📁", payload.Icon)
	}
	if payload.RequestID == "" {
		t.Fatal("sent requestId was empty")
	}
}

func TestClientDomainStatsRejectsNegativeLimit(t *testing.T) {
	t.Parallel()

	transport := newTransportRecorder(t, true, []byte(`{"requestId":"req-17","success":true,"data":[]}`))
	client := NewClient(transport, nil)

	_, err := client.DomainStats(context.Background(), -1)
	if err == nil {
		t.Fatal("DomainStats() error = nil, want validation error")
	}
	if transport.called {
		t.Fatal("DomainStats() sent a request for a negative limit")
	}
}

func TestClientDeleteDomainRejectsEmptyDomain(t *testing.T) {
	t.Parallel()

	transport := newTransportRecorder(t, true, []byte(`{"requestId":"req-domain-empty","success":true}`))
	client := NewClient(transport, nil)

	err := client.DeleteDomain(context.Background(), "  ")
	if err == nil {
		t.Fatal("DeleteDomain() error = nil, want validation error")
	}
	if transport.called {
		t.Fatal("DeleteDomain() sent a request for an empty domain")
	}
}

func TestClientUpdateFolderTrimsIcon(t *testing.T) {
	t.Parallel()

	transport := newTransportRecorder(t, true, []byte(`{"requestId":"req-18","success":true}`))
	client := NewClient(transport, nil)

	if err := client.UpdateFolder(context.Background(), 9, "Read", " 📚 "); err != nil {
		t.Fatalf("UpdateFolder() error = %v", err)
	}

	var msg port.WebUIMessage
	if err := json.Unmarshal(transport.last, &msg); err != nil {
		t.Fatalf("unmarshal sent envelope: %v", err)
	}
	if msg.Type != "folder_update" {
		t.Fatalf("sent type = %q, want folder_update", msg.Type)
	}
	var payload struct {
		Icon *string `json:"icon"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatalf("unmarshal sent payload: %v", err)
	}
	if payload.Icon == nil || *payload.Icon != "📚" {
		t.Fatalf("sent icon = %#v, want 📚", payload.Icon)
	}
}

func TestClientUpdateFolderOmitsBlankIcon(t *testing.T) {
	t.Parallel()

	transport := newTransportRecorder(t, true, []byte(`{"requestId":"req-19","success":true}`))
	client := NewClient(transport, nil)

	if err := client.UpdateFolder(context.Background(), 9, "Read", "   "); err != nil {
		t.Fatalf("UpdateFolder() error = %v", err)
	}

	var msg port.WebUIMessage
	if err := json.Unmarshal(transport.last, &msg); err != nil {
		t.Fatalf("unmarshal sent envelope: %v", err)
	}
	var payload struct {
		Icon *string `json:"icon"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatalf("unmarshal sent payload: %v", err)
	}
	if payload.Icon != nil {
		t.Fatalf("sent icon = %#v, want nil", payload.Icon)
	}
}

func TestClientDeleteEntryRejectsInvalidID(t *testing.T) {
	t.Parallel()

	transport := newTransportRecorder(t, true, []byte(`{"requestId":"req-20","success":true}`))
	client := NewClient(transport, nil)

	err := client.DeleteEntry(context.Background(), 0)
	if err == nil {
		t.Fatal("DeleteEntry() error = nil, want validation error")
	}
	if transport.called {
		t.Fatal("DeleteEntry() sent a request for an invalid id")
	}
}

func TestClientTagActionsTrimFields(t *testing.T) {
	t.Parallel()

	createTransport := newTransportRecorder(t, true, []byte(`{"requestId":"req-21","success":true,"data":{"id":5,"name":"Go","color":"#00add8"}}`))
	client := NewClient(createTransport, nil)
	if _, err := client.CreateTag(context.Background(), " Go ", " #00add8 "); err != nil {
		t.Fatalf("CreateTag() error = %v", err)
	}
	var msg port.WebUIMessage
	if err := json.Unmarshal(createTransport.last, &msg); err != nil {
		t.Fatalf("unmarshal create envelope: %v", err)
	}
	var createPayload struct {
		Name  string  `json:"name"`
		Color *string `json:"color"`
	}
	if err := json.Unmarshal(msg.Payload, &createPayload); err != nil {
		t.Fatalf("unmarshal create payload: %v", err)
	}
	if createPayload.Name != "Go" || createPayload.Color == nil || *createPayload.Color != "#00add8" {
		t.Fatalf("CreateTag payload = %#v", createPayload)
	}

	updateTransport := newTransportRecorder(t, true, []byte(`{"requestId":"req-22","success":true}`))
	client = NewClient(updateTransport, nil)
	if err := client.UpdateTag(context.Background(), 5, " Go ", "   "); err != nil {
		t.Fatalf("UpdateTag() error = %v", err)
	}
	if err := json.Unmarshal(updateTransport.last, &msg); err != nil {
		t.Fatalf("unmarshal update envelope: %v", err)
	}
	var updatePayload struct {
		Name  *string `json:"name"`
		Color *string `json:"color"`
	}
	if err := json.Unmarshal(msg.Payload, &updatePayload); err != nil {
		t.Fatalf("unmarshal update payload: %v", err)
	}
	if updatePayload.Name == nil || *updatePayload.Name != "Go" || updatePayload.Color != nil {
		t.Fatalf("UpdateTag payload = %#v", updatePayload)
	}
}

type transportRecorder struct {
	*systemviewsbridgemocks.MockTransport

	available bool
	called    bool
	last      []byte
	response  []byte
}

func newTransportRecorder(t *testing.T, available bool, response []byte) *transportRecorder {
	t.Helper()

	recorder := &transportRecorder{
		MockTransport: systemviewsbridgemocks.NewMockTransport(t),
		available:     available,
		response:      response,
	}
	recorder.EXPECT().Available().RunAndReturn(func() bool { return recorder.available }).Maybe()
	recorder.EXPECT().Send(mock.Anything, mock.Anything).RunAndReturn(recorder.send).Maybe()
	return recorder
}

func (t *transportRecorder) send(_ context.Context, body []byte) ([]byte, error) {
	t.called = true
	t.last = append(t.last[:0], body...)
	return t.response, nil
}

type blockingTransport struct {
	available bool
	called    bool
}

func (t *blockingTransport) Available() bool { return t.available }

func (t *blockingTransport) Send(ctx context.Context, _ []byte) ([]byte, error) {
	t.called = true
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestClientRejectsInvalidFavoriteMutationIDs(t *testing.T) {
	t.Parallel()

	transport := newTransportRecorder(t, true, []byte(`{"requestId":"req","success":true}`))
	client := NewClient(transport, nil)

	checks := []struct {
		name string
		call func() error
	}{
		{name: "delete favorite", call: func() error { return client.DeleteFavorite(context.Background(), 0) }},
		{name: "set shortcut", call: func() error { return client.SetShortcut(context.Background(), 0, nil) }},
		{name: "set folder", call: func() error { return client.SetFolder(context.Background(), 0, nil) }},
		{name: "create folder", call: func() error { _, err := client.CreateFolder(context.Background(), " ", "", nil); return err }},
		{name: "update folder id", call: func() error { return client.UpdateFolder(context.Background(), 0, "Folder", "") }},
		{name: "update folder name", call: func() error { return client.UpdateFolder(context.Background(), 1, " ", "") }},
		{name: "delete folder", call: func() error { return client.DeleteFolder(context.Background(), 0) }},
		{name: "create tag", call: func() error { _, err := client.CreateTag(context.Background(), " ", ""); return err }},
		{name: "update tag", call: func() error { return client.UpdateTag(context.Background(), 0, "Tag", "") }},
		{name: "update tag name", call: func() error { return client.UpdateTag(context.Background(), 1, " ", "") }},
		{name: "delete tag", call: func() error { return client.DeleteTag(context.Background(), 0) }},
		{name: "assign favorite", call: func() error { return client.AssignTag(context.Background(), 0, 1) }},
		{name: "assign tag", call: func() error { return client.AssignTag(context.Background(), 1, 0) }},
		{name: "remove favorite", call: func() error { return client.RemoveTag(context.Background(), 0, 1) }},
		{name: "remove tag", call: func() error { return client.RemoveTag(context.Background(), 1, 0) }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); err == nil {
				t.Fatal("error = nil, want validation error")
			}
		})
	}
	if transport.called {
		t.Fatal("transport called for invalid favorite mutation IDs")
	}
}

func TestClientRejectsNegativeHistoryLimits(t *testing.T) {
	t.Parallel()

	client := NewClient(newTransportRecorder(t, true, nil), nil)

	if _, err := client.Timeline(context.Background(), -1, 0); err == nil {
		t.Fatal("Timeline() error = nil, want error")
	}
	if _, err := client.Timeline(context.Background(), 1, -1); err == nil {
		t.Fatal("Timeline() error = nil, want error")
	}
	if _, err := client.TimelineByDomain(context.Background(), "", 1, 0); err == nil {
		t.Fatal("TimelineByDomain() error = nil, want error")
	}
	if _, err := client.TimelineByDomain(context.Background(), "example.com", -1, 0); err == nil {
		t.Fatal("TimelineByDomain() error = nil, want error")
	}
	if _, err := client.TimelineByDomain(context.Background(), "example.com", 1, -1); err == nil {
		t.Fatal("TimelineByDomain() error = nil, want error")
	}
	if _, err := client.Search(context.Background(), "example", -1); err == nil {
		t.Fatal("Search() error = nil, want error")
	}
}
