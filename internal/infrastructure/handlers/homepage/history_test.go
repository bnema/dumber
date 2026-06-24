package homepage

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestHandleTimelineWindowRejectsIncompleteCursor(t *testing.T) {
	t.Parallel()

	tests := map[string]json.RawMessage{
		"before without beforeId":          json.RawMessage(`{"requestId":"req-1","before":"2026-04-25T09:00:00Z"}`),
		"beforeId without before":          json.RawMessage(`{"requestId":"req-1","beforeId":42}`),
		"negative beforeId without before": json.RawMessage(`{"requestId":"req-1","beforeId":-1}`),
	}
	for name, payload := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			history := portmocks.NewMockHomepageHistory(t)
			handler := NewHistoryHandlers(history).HandleTimelineWindow()

			got, err := handler.Handle(context.Background(), port.WebViewID(0), payload)
			require.NoError(t, err)

			resp, ok := got.(Response)
			require.True(t, ok)
			require.False(t, resp.Success)
			require.Contains(t, resp.Error, "cursor")
		})
	}
}

func TestHandleTimelineWindowPassesCompleteCursor(t *testing.T) {
	t.Parallel()

	before := time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC)
	history := portmocks.NewMockHomepageHistory(t)
	history.EXPECT().
		GetRecentWindow(mock.Anything, before, int64(42), "example.com").
		Return(&entity.HistoryWindow{Before: before, CursorID: 42}, nil).
		Once()

	handler := NewHistoryHandlers(history).HandleTimelineWindow()
	payload := json.RawMessage(`{"requestId":"req-2","before":"2026-04-25T09:00:00Z","beforeId":42,"domain":"example.com"}`)

	got, err := handler.Handle(context.Background(), port.WebViewID(0), payload)
	require.NoError(t, err)

	resp, ok := got.(Response)
	require.True(t, ok)
	require.True(t, resp.Success)
}
