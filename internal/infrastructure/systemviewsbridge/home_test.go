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
