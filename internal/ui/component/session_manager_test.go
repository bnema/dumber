package component

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/require"
)

func TestSessionManagerOpenCurrentSessionShowsAlreadyOpen(t *testing.T) {
	ctx := context.Background()
	notified := false
	sm := &SessionManager{
		ctx:           ctx,
		sessions:      []entity.SessionInfo{{Session: &entity.Session{ID: "current"}, IsCurrent: true}},
		selectedIndex: 0,
		onOpen:        func(entity.SessionID) { t.Fatal("current session must not be restored") },
		onToast: func(got context.Context, message string, level ToastLevel) {
			notified = true
			require.Equal(t, ctx, got)
			require.Equal(t, "Session already open", message)
			require.Equal(t, ToastWarning, level)
		},
	}
	sm.openSelected()
	require.True(t, notified)
}
