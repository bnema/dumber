package systemviewsbridge

import (
	"context"
	"encoding/json"
	"testing"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/application/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sentMessage(t *testing.T, raw []byte) (string, map[string]any) {
	t.Helper()
	var msg port.WebUIMessage
	require.NoError(t, json.Unmarshal(raw, &msg))
	payload := map[string]any{}
	require.NoError(t, json.Unmarshal(msg.Payload, &payload))
	return msg.Type, payload
}

func TestClientHomeLoadDecodesResponse(t *testing.T) {
	t.Parallel()
	native := newTransportRecorder(t, true, []byte(`{"success":true,"data":{"layout":{"settings":{"title":"Lab","columns":4}},"config_path":"/c/home.toml","parse_error":{"file":"home.toml","line":3,"column":5,"message":"bad"},"search_engine":"https://s.test/?q=%s"}}`))
	resp, err := NewClient(native, nil).HomeLoad(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Lab", resp.Layout.Settings.Title)
	assert.Equal(t, 4, resp.Layout.Settings.Columns)
	require.NotNil(t, resp.ParseError)
	assert.Equal(t, 3, resp.ParseError.Line)
	assert.Equal(t, "https://s.test/?q=%s", resp.SearchEngine)
	typ, payload := sentMessage(t, native.last)
	assert.Equal(t, "home_load", typ)
	assert.NotEmpty(t, payload["requestId"])
}

func TestClientHomeSitesSendsRequest(t *testing.T) {
	t.Parallel()
	native := newTransportRecorder(t, true, []byte(`{"success":true,"data":{"sites":[{"url":"https://a.test","title":"A","visit_count":7}]}}`))
	resp, err := NewClient(native, nil).HomeSites(context.Background(), dto.HomeSitesRequest{Kind: dto.HomeSitesTop, Limit: 5})
	require.NoError(t, err)
	require.Len(t, resp.Sites, 1)
	assert.Equal(t, int64(7), resp.Sites[0].VisitCount)
	typ, payload := sentMessage(t, native.last)
	assert.Equal(t, "home_sites", typ)
	assert.Equal(t, "top", payload["kind"])
	assert.InDelta(t, 5, payload["limit"], 0)
}

func TestClientHomeSitesRejectsBadInput(t *testing.T) {
	t.Parallel()
	native := newTransportRecorder(t, true, nil)
	client := NewClient(native, nil)
	_, err := client.HomeSites(context.Background(), dto.HomeSitesRequest{Kind: "nope"})
	require.Error(t, err)
	_, err = client.HomeSites(context.Background(), dto.HomeSitesRequest{Kind: dto.HomeSitesRecent, Limit: -1})
	require.Error(t, err)
	assert.False(t, native.called)
}

func TestClientHomeOpenConfigDecodesResponse(t *testing.T) {
	t.Parallel()
	native := newTransportRecorder(t, true, []byte(`{"success":true,"data":{"path":"/c/home.toml","created":true}}`))
	resp, err := NewClient(native, nil).HomeOpenConfig(context.Background())
	require.NoError(t, err)
	assert.Equal(t, dto.HomeOpenConfigResponse{Path: "/c/home.toml", Created: true}, resp)
	typ, _ := sentMessage(t, native.last)
	assert.Equal(t, "home_open_config", typ)
}

func TestClientHomeWidgetDataSendsRequest(t *testing.T) {
	t.Parallel()
	native := newTransportRecorder(t, true, []byte(`{"success":true,"data":{"service_id":"media","status":"error","fields":[{"label":"Movies","value":"12","raw":12}],"error":"HTTP 503","fetched_at":"2026-01-01T12:00:00Z","stale":true}}`))
	resp, err := NewClient(native, nil).HomeWidgetData(context.Background(), "media")
	require.NoError(t, err)
	assert.Equal(t, dto.HomeStatusError, resp.Status)
	assert.True(t, resp.Stale)
	assert.Equal(t, "HTTP 503", resp.Error)
	require.Len(t, resp.Fields, 1)
	assert.Equal(t, "12", resp.Fields[0].Value)
	typ, payload := sentMessage(t, native.last)
	assert.Equal(t, "home_widget_data", typ)
	assert.Equal(t, "media", payload["service_id"])
	assert.NotEmpty(t, payload["requestId"])
}

func TestClientHomeActionSendsRequest(t *testing.T) {
	t.Parallel()
	native := newTransportRecorder(t, true, []byte(`{"success":true,"data":{"status":"ok","http_status":204,"message":"HTTP 204"}}`))
	resp, err := NewClient(native, nil).HomeAction(context.Background(), dto.HomeActionRequest{
		ServiceID: "media", ActionID: "restart", Inputs: map[string]string{"q": "x"},
	})
	require.NoError(t, err)
	assert.Equal(t, dto.HomeActionResponse{Status: dto.HomeStatusOK, HTTPStatus: 204, Message: "HTTP 204"}, resp)
	typ, payload := sentMessage(t, native.last)
	assert.Equal(t, "home_action", typ)
	assert.Equal(t, "media", payload["service_id"])
	assert.Equal(t, "restart", payload["action_id"])
	assert.Equal(t, map[string]any{"q": "x"}, payload["inputs"])
}

func TestClientHomeWidgetAndActionRejectMissingIDs(t *testing.T) {
	t.Parallel()
	native := newTransportRecorder(t, true, nil)
	client := NewClient(native, nil)
	_, err := client.HomeWidgetData(context.Background(), "")
	require.Error(t, err)
	_, err = client.HomeAction(context.Background(), dto.HomeActionRequest{ServiceID: "media"})
	require.Error(t, err)
	assert.False(t, native.called)
}

func TestClientHomeStatusSendsRequest(t *testing.T) {
	t.Parallel()
	native := newTransportRecorder(t, true, []byte(`{"success":true,"data":{"service_id":"media","state":"up","latency_ms":12,"code":401}}`))
	resp, err := NewClient(native, nil).HomeStatus(context.Background(), "media")
	require.NoError(t, err)
	assert.Equal(t, dto.HomeStatusResponse{ServiceID: "media", State: dto.HomeServiceStateUp, LatencyMS: 12, Code: 401}, resp)
	typ, payload := sentMessage(t, native.last)
	assert.Equal(t, "home_status", typ)
	assert.Equal(t, "media", payload["service_id"])
	assert.NotEmpty(t, payload["requestId"])

	empty := newTransportRecorder(t, true, nil)
	_, err = NewClient(empty, nil).HomeStatus(context.Background(), "")
	require.Error(t, err)
	assert.False(t, empty.called)
}
