package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/infrastructure/handlers/homepage"
)

// TestWebUIContract_EveryMessageHasAHandlerAndMatchingCallbacks checks both
// directions of the WebUI message contract: every declared message has a Go
// handler, and every handler registers with the declared callbacks.
func TestWebUIContract_EveryMessageHasAHandlerAndMatchingCallbacks(t *testing.T) {
	ctx := context.Background()
	registered := map[string]port.WebUICallbacks{}

	router := mocks.NewMockWebUIHandlerRouter(t)
	router.EXPECT().
		RegisterHandlerWithCallbacks(mock.Anything, mock.Anything, mock.Anything, "", mock.Anything).
		Run(func(msgType, success, failure, _ string, _ port.WebUIMessageHandler) {
			registered[msgType] = port.WebUICallbacks{Success: success, Failure: failure}
		}).
		Return(nil)

	require.NoError(t, homepage.RegisterHandlers(ctx, router, homepage.Config{
		HistoryUC:   mocks.NewMockHomepageHistory(t),
		FavoritesUC: mocks.NewMockHomepageFavorites(t),
	}))
	require.NoError(t, RegisterConfigHandlers(ctx, router, func(context.Context, dto.WebUIConfig) error { return nil }))
	require.NoError(t, RegisterKeybindingsHandlers(ctx, router, &KeybindingsHandler{}))

	assert.Equal(t, port.WebUIMessageCallbacks, registered)
}

func TestRegisterWebUIMessage_RejectsUnknownType(t *testing.T) {
	router := mocks.NewMockWebUIHandlerRouter(t)

	err := port.RegisterWebUIMessage(router, "not_a_message", port.WebUIMessageHandlerFunc(nil))

	var unknown *port.UnknownWebUIMessageError
	require.ErrorAs(t, err, &unknown)
}
