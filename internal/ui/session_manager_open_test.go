package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/stretchr/testify/require"
)

func TestSessionManagerOpenHandlerReportsFailure(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "accepted", true: "failed"}[failed], func(t *testing.T) {
			ctx := context.Background()
			called, notified := false, false
			handler := sessionManagerOpenHandler(ctx, func(id entity.SessionID) error {
				called = true
				require.Equal(t, entity.SessionID("saved"), id)
				if failed {
					return errors.New("snapshot unavailable")
				}
				return nil
			}, func(got context.Context, message string, level component.ToastLevel) {
				notified = true
				require.Equal(t, ctx, got)
				if failed {
					require.Equal(t, "Session restore failed: snapshot unavailable", message)
					require.Equal(t, component.ToastWarning, level)
				} else {
					require.Equal(t, "Session restored", message)
					require.Equal(t, component.ToastSuccess, level)
				}
			})
			handler("saved")
			require.True(t, called)
			require.True(t, notified)
		})
	}
}

func TestSessionManagerOpenHandlerReportsAlreadyOpen(t *testing.T) {
	sessionManagerOpenHandler(context.Background(), func(entity.SessionID) error {
		return usecase.ErrSessionAlreadyOpen
	}, func(_ context.Context, message string, level component.ToastLevel) {
		require.Equal(t, "Session already open", message)
		require.Equal(t, component.ToastWarning, level)
	})("active")
}
