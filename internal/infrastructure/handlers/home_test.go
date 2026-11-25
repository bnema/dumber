package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/application/port/mocks"
)

func TestDecodeHomeSitesRequest(t *testing.T) {
	tests := []struct {
		payload string
		want    dto.HomeSitesRequest
		err     bool
	}{
		{payload: `{"kind":"top","limit":10}`, want: dto.HomeSitesRequest{Kind: dto.HomeSitesTop, Limit: 10}},
		{payload: `{"kind":"recent"}`, want: dto.HomeSitesRequest{Kind: dto.HomeSitesRecent}},
		{payload: `{"kind":"top","limit":51}`, err: true},
		{payload: `{"kind":"top","limit":-1}`, err: true},
		{payload: `{"kind":"other"}`, err: true},
		{payload: `{"kind":`, err: true},
		{payload: `null`, err: true},
		{payload: ``, err: true},
	}
	for _, tt := range tests {
		t.Run(tt.payload, func(t *testing.T) {
			got, err := decodeHomeSitesRequest(json.RawMessage(tt.payload))
			if tt.err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRegisterHomeHandlers_Dispatch(t *testing.T) {
	ctx := context.Background()
	handlers := map[string]port.WebUIMessageHandler{}
	router := mocks.NewMockWebUIHandlerRouter(t)
	router.EXPECT().
		RegisterHandlerWithCallbacks(mock.Anything, mock.Anything, mock.Anything, "", mock.Anything).
		Run(func(msgType, _, _, _ string, h port.WebUIMessageHandler) { handlers[msgType] = h }).
		Return(nil)

	home := mocks.NewMockHomePage(t)
	require.NoError(t, RegisterHomeHandlers(ctx, router, home))

	home.EXPECT().HomeSites(ctx, dto.HomeSitesRequest{Kind: dto.HomeSitesTop, Limit: 3}).
		Return(&dto.HomeSitesResponse{}, nil)
	_, err := handlers[port.MsgHomeSites].Handle(ctx, 1, json.RawMessage(`{"kind":"top","limit":3}`))
	require.NoError(t, err)

	_, err = handlers[port.MsgHomeSites].Handle(ctx, 1, json.RawMessage(`{"kind":"top","limit":99}`))
	require.Error(t, err)

	home.EXPECT().OpenHomeConfig(ctx).Return(&dto.HomeOpenConfigResponse{Path: "/c/home.toml"}, nil)
	_, err = handlers[port.MsgHomeOpenConfig].Handle(ctx, 1, json.RawMessage(`{"path":"/etc/passwd"}`))
	require.NoError(t, err)

	home.EXPECT().LoadHome(ctx).Return(&dto.HomeLoadResponse{}, nil)
	_, err = handlers[port.MsgHomeLoad].Handle(ctx, 1, nil)
	require.NoError(t, err)
}
