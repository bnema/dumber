package model

import (
	"context"
	"fmt"
	portmocks "bnema.dev/dumber/internal/application/port/mocks"
	"bnema.dev/dumber/internal/application/usecase"
	"bnema.dev/dumber/internal/domain/entity"
	repomocks "bnema.dev/dumber/internal/domain/repository/mocks"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInteractiveRestoreFailureExitsWithError(t *testing.T) {
	model, cmd := (SessionsModel{}).Update(sessionRestoredMsg{err: fmt.Errorf("relay unavailable")})
	require.NotNil(t, cmd)
	require.ErrorContains(t, model.(SessionsModel).RestoreError(), "relay unavailable")
}

func TestInteractiveRestoreValidatesBeforeColdDestination(t *testing.T) {
	ctx := context.Background()
	sessions := repomocks.NewMockSessionRepository(t)
	states := repomocks.NewMockSessionStateRepository(t)
	// No relay or launcher call is permitted: an active source is ineligible
	// even when the destination would otherwise cold-start a browser.
	launcher := portmocks.NewMockSessionRestoreLauncher(t)
	sessions.EXPECT().FindByID(ctx, entity.SessionID("active-source")).Return(&entity.Session{ID: "active-source"}, nil)
	m := SessionsModel{ctx: ctx, restoreUC: usecase.NewRestoreSessionUseCase(states, sessions), sessionRestoreLauncher: launcher, currentSession: "other"}
	result := m.restoreSession("active-source")().(sessionRestoredMsg)
	require.ErrorIs(t, result.err, usecase.ErrSessionAlreadyOpen)
}

func TestInteractiveRestoreStatusDistinguishesColdStart(t *testing.T) {
	for _, cold := range []bool{false, true} {
		model, _ := (SessionsModel{}).Update(sessionRestoredMsg{sessionID: "saved", started: cold})
		if cold {
			require.Equal(t, "browser starting, restoring session saved", model.(SessionsModel).RestoreStatus())
		} else {
			require.Equal(t, "Session restored", model.(SessionsModel).RestoreStatus())
		}
	}
}
