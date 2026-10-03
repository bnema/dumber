package content

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
)

func TestHandlePermissionRequest_BindsDialogBeforeUseCaseForNonWebRTCRequest(t *testing.T) {
	permRepo := portmocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)
	// The use case starts without a dialog presenter, like in production.
	uc := usecase.NewHandlePermissionUseCase(permRepo, nil, zerolog.Ctx)

	permRepo.EXPECT().Get(mock.Anything, "https://maps.example.com", entity.PermissionTypeGeolocation).
		Return(nil, nil)

	var order []string
	dialog.EXPECT().
		ShowPermissionDialog(mock.Anything, "https://maps.example.com", []entity.PermissionType{entity.PermissionTypeGeolocation}, mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ string, _ []entity.PermissionType, _ entity.PermissionMetadata, onResult func(port.PermissionDialogResult)) {
			order = append(order, "dialog")
			onResult(port.PermissionDialogResult{Allowed: true})
		}).Return()

	pane := entity.PaneID("pane-1")
	var gotPane entity.PaneID
	activityCalls := 0
	c := &Coordinator{
		permissionUC: uc,
		onPermissionPrompt: func(paneID entity.PaneID) {
			order = append(order, "bind")
			gotPane = paneID
			uc.SetDialogPresenter(dialog)
		},
		onPermissionActivity: func(entity.PaneID, string, []entity.PermissionType, PermissionActivityState) {
			activityCalls++
		},
	}

	allowed, denied := 0, 0
	handled := c.handlePermissionRequest(context.Background(), pane, "https://maps.example.com",
		[]string{"geolocation"}, nil, func() { allowed++ }, func() { denied++ })

	require.True(t, handled)
	assert.Equal(t, pane, gotPane)
	assert.Equal(t, []string{"bind", "dialog"}, order)
	assert.Equal(t, 1, allowed)
	assert.Zero(t, denied)
	assert.Zero(t, activityCalls, "geolocation is not tracked by the WebRTC indicator")
}
