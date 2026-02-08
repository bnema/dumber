package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func permissionLoggerFromContext(ctx context.Context) *zerolog.Logger {
	return zerolog.Ctx(ctx)
}

func TestHandlePermissionUseCase_AutoAllowDisplayCapture(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog, permissionLoggerFromContext)

	permRepo.EXPECT().Get(mock.Anything, "https://meet.example.com", entity.PermissionTypeDisplay).
		Return(nil, nil)

	allowed := false
	denied := false
	callback := usecase.PermissionCallback{
		Allow: func() { allowed = true },
		Deny:  func() { denied = true },
	}

	// Display capture should be auto-allowed (portal handles UI)
	uc.HandlePermissionRequest(ctx, "https://meet.example.com", []entity.PermissionType{
		entity.PermissionTypeDisplay,
	}, callback)

	assert.True(t, allowed, "display capture should be auto-allowed")
	assert.False(t, denied)
	dialog.AssertNotCalled(t, "ShowPermissionDialog")
}

func TestHandlePermissionUseCase_AutoAllowDeviceInfo(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog, permissionLoggerFromContext)

	permRepo.EXPECT().Get(mock.Anything, "https://example.com", entity.PermissionTypeDeviceInfo).
		Return(nil, nil)

	allowed := false
	callback := usecase.PermissionCallback{
		Allow: func() { allowed = true },
		Deny:  func() {},
	}

	// Device info should be auto-allowed
	uc.HandlePermissionRequest(ctx, "https://example.com", []entity.PermissionType{
		entity.PermissionTypeDeviceInfo,
	}, callback)

	assert.True(t, allowed, "device info should be auto-allowed")
}

func TestHandlePermissionUseCase_StoredPermissionGranted(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog, permissionLoggerFromContext)

	// Return stored granted permission
	permRepo.EXPECT().Get(mock.Anything, "https://meet.example.com", entity.PermissionTypeMicrophone).
		Return(&entity.PermissionRecord{
			Origin:    "https://meet.example.com",
			Type:      entity.PermissionTypeMicrophone,
			Decision:  entity.PermissionGranted,
			UpdatedAt: time.Now().Unix(),
		}, nil)

	allowed := false
	callback := usecase.PermissionCallback{
		Allow: func() { allowed = true },
		Deny:  func() {},
	}

	uc.HandlePermissionRequest(ctx, "https://meet.example.com", []entity.PermissionType{
		entity.PermissionTypeMicrophone,
	}, callback)

	assert.True(t, allowed, "should use stored granted permission")
	dialog.AssertNotCalled(t, "ShowPermissionDialog")
}

func TestHandlePermissionUseCase_StoredPermissionDenied(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog, permissionLoggerFromContext)

	// Return stored denied permission
	permRepo.EXPECT().Get(mock.Anything, "https://meet.example.com", entity.PermissionTypeCamera).
		Return(&entity.PermissionRecord{
			Origin:    "https://meet.example.com",
			Type:      entity.PermissionTypeCamera,
			Decision:  entity.PermissionDenied,
			UpdatedAt: time.Now().Unix(),
		}, nil)

	denied := false
	callback := usecase.PermissionCallback{
		Allow: func() {},
		Deny:  func() { denied = true },
	}

	uc.HandlePermissionRequest(ctx, "https://meet.example.com", []entity.PermissionType{
		entity.PermissionTypeCamera,
	}, callback)

	assert.True(t, denied, "should use stored denied permission")
	dialog.AssertNotCalled(t, "ShowPermissionDialog")
}

func TestHandlePermissionUseCase_ShowDialogForPrompt(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog, permissionLoggerFromContext)

	// No stored permission (returns nil)
	permRepo.EXPECT().Get(mock.Anything, "https://meet.example.com", entity.PermissionTypeMicrophone).
		Return(nil, nil)

	allowed := false
	callback := usecase.PermissionCallback{
		Allow: func() { allowed = true },
		Deny:  func() {},
	}

	// Dialog shows and user clicks Allow (not persistent)
	dialog.EXPECT().ShowPermissionDialog(mock.Anything, "https://meet.example.com", []entity.PermissionType{
		entity.PermissionTypeMicrophone,
	}, mock.Anything).Run(func(_ context.Context, _ string, _ []entity.PermissionType, callback func(port.PermissionDialogResult)) {
		callback(port.PermissionDialogResult{Allowed: true, Persistent: false})
	})

	uc.HandlePermissionRequest(ctx, "https://meet.example.com", []entity.PermissionType{
		entity.PermissionTypeMicrophone,
	}, callback)

	assert.True(t, allowed, "user allowed permission")
	// Should not persist since Persistent=false
	permRepo.AssertNotCalled(t, "Set")
}

func TestHandlePermissionUseCase_PersistAllowed(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog, permissionLoggerFromContext)

	// No stored permission
	permRepo.EXPECT().Get(mock.Anything, "https://meet.example.com", entity.PermissionTypeMicrophone).
		Return(nil, nil)

	allowed := false
	callback := usecase.PermissionCallback{
		Allow: func() { allowed = true },
		Deny:  func() {},
	}

	// Dialog shows and user clicks "Always Allow"
	dialog.EXPECT().ShowPermissionDialog(mock.Anything, "https://meet.example.com", []entity.PermissionType{
		entity.PermissionTypeMicrophone,
	}, mock.Anything).Run(func(_ context.Context, _ string, _ []entity.PermissionType, cb func(port.PermissionDialogResult)) {
		cb(port.PermissionDialogResult{Allowed: true, Persistent: true})
	})

	// Should persist the granted permission
	permRepo.EXPECT().Set(mock.Anything, mock.AnythingOfType("*entity.PermissionRecord")).
		Run(func(_ context.Context, r *entity.PermissionRecord) {
			assert.Equal(t, "https://meet.example.com", r.Origin)
			assert.Equal(t, entity.PermissionTypeMicrophone, r.Type)
			assert.Equal(t, entity.PermissionGranted, r.Decision)
		}).Return(nil)

	uc.HandlePermissionRequest(ctx, "https://meet.example.com", []entity.PermissionType{
		entity.PermissionTypeMicrophone,
	}, callback)

	assert.True(t, allowed)
}

func TestHandlePermissionUseCase_PersistDenied(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog, permissionLoggerFromContext)

	// No stored permission
	permRepo.EXPECT().Get(mock.Anything, "https://meet.example.com", entity.PermissionTypeCamera).
		Return(nil, nil)

	denied := false
	callback := usecase.PermissionCallback{
		Allow: func() {},
		Deny:  func() { denied = true },
	}

	// Dialog shows and user clicks "Always Deny"
	dialog.EXPECT().ShowPermissionDialog(mock.Anything, "https://meet.example.com", []entity.PermissionType{
		entity.PermissionTypeCamera,
	}, mock.Anything).Run(func(_ context.Context, _ string, _ []entity.PermissionType, cb func(port.PermissionDialogResult)) {
		// User denied but wants to persist this decision
		cb(port.PermissionDialogResult{Allowed: false, Persistent: true})
	})

	// Should persist the denied permission
	permRepo.EXPECT().Set(mock.Anything, mock.AnythingOfType("*entity.PermissionRecord")).
		Run(func(_ context.Context, r *entity.PermissionRecord) {
			assert.Equal(t, entity.PermissionDenied, r.Decision)
		}).Return(nil)

	uc.HandlePermissionRequest(ctx, "https://meet.example.com", []entity.PermissionType{
		entity.PermissionTypeCamera,
	}, callback)

	assert.True(t, denied)
}

func TestHandlePermissionUseCase_EmptyOrigin(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog, permissionLoggerFromContext)

	denied := false
	callback := usecase.PermissionCallback{
		Allow: func() {},
		Deny:  func() { denied = true },
	}

	// Empty origin should be denied
	uc.HandlePermissionRequest(ctx, "", []entity.PermissionType{
		entity.PermissionTypeMicrophone,
	}, callback)

	assert.True(t, denied, "empty origin should be denied")
}

func TestHandlePermissionUseCase_CombinedPermissions(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog, permissionLoggerFromContext)

	// Both mic and camera - one denied means whole request denied
	permRepo.EXPECT().Get(mock.Anything, "https://meet.example.com", entity.PermissionTypeMicrophone).
		Return(&entity.PermissionRecord{
			Origin:   "https://meet.example.com",
			Type:     entity.PermissionTypeMicrophone,
			Decision: entity.PermissionGranted,
		}, nil)

	permRepo.EXPECT().Get(mock.Anything, "https://meet.example.com", entity.PermissionTypeCamera).
		Return(&entity.PermissionRecord{
			Origin:   "https://meet.example.com",
			Type:     entity.PermissionTypeCamera,
			Decision: entity.PermissionDenied,
		}, nil)

	denied := false
	callback := usecase.PermissionCallback{
		Allow: func() {},
		Deny:  func() { denied = true },
	}

	uc.HandlePermissionRequest(ctx, "https://meet.example.com", []entity.PermissionType{
		entity.PermissionTypeMicrophone,
		entity.PermissionTypeCamera,
	}, callback)

	assert.True(t, denied, "if any permission is denied, whole request is denied")
}

func TestHandlePermissionUseCase_QueryPermissionState_AutoAllow(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog, permissionLoggerFromContext)
	permRepo.EXPECT().Get(mock.Anything, "https://example.com", entity.PermissionTypeDisplay).
		Return(nil, nil)

	state := uc.QueryPermissionState(ctx, "https://example.com", entity.PermissionTypeDisplay)

	assert.Equal(t, entity.PermissionGranted, state, "display capture should auto-return granted")
}

func TestHandlePermissionUseCase_QueryPermissionState_AutoAllowManualOverride(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog, permissionLoggerFromContext)

	permRepo.EXPECT().Get(mock.Anything, "https://example.com", entity.PermissionTypeDisplay).
		Return(&entity.PermissionRecord{
			Origin:   "https://example.com",
			Type:     entity.PermissionTypeDisplay,
			Decision: entity.PermissionDenied,
		}, nil)

	state := uc.QueryPermissionState(ctx, "https://example.com", entity.PermissionTypeDisplay)

	assert.Equal(t, entity.PermissionDenied, state, "stored manual deny should override auto-allow")
}

func TestHandlePermissionUseCase_QueryPermissionState_Stored(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog, permissionLoggerFromContext)

	permRepo.EXPECT().Get(mock.Anything, "https://meet.example.com", entity.PermissionTypeMicrophone).
		Return(&entity.PermissionRecord{
			Origin:   "https://meet.example.com",
			Type:     entity.PermissionTypeMicrophone,
			Decision: entity.PermissionDenied,
		}, nil)

	state := uc.QueryPermissionState(ctx, "https://meet.example.com", entity.PermissionTypeMicrophone)

	assert.Equal(t, entity.PermissionDenied, state)
}

func TestHandlePermissionUseCase_QueryPermissionState_NoRecord(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog, permissionLoggerFromContext)

	permRepo.EXPECT().Get(mock.Anything, "https://meet.example.com", entity.PermissionTypeCamera).
		Return(nil, nil)

	state := uc.QueryPermissionState(ctx, "https://meet.example.com", entity.PermissionTypeCamera)

	assert.Equal(t, entity.PermissionPrompt, state, "no record should return prompt")
}

func TestHandlePermissionUseCase_NonPersistableNotSaved(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog, permissionLoggerFromContext)

	// Display capture is not persistable per W3C spec
	// A stored override may exist, so a Get call is expected before auto-allow.
	// No Set call should happen even if we try to persist
	permRepo.EXPECT().Get(mock.Anything, "https://meet.example.com", entity.PermissionTypeDisplay).
		Return(nil, nil)

	allowed := false
	callback := usecase.PermissionCallback{
		Allow: func() { allowed = true },
		Deny:  func() {},
	}

	uc.HandlePermissionRequest(ctx, "https://meet.example.com", []entity.PermissionType{
		entity.PermissionTypeDisplay,
	}, callback)

	assert.True(t, allowed)
	permRepo.AssertNotCalled(t, "Set")
}

func TestHandlePermissionUseCase_NoDialogPresenter(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)

	// Create use case with nil dialog presenter
	uc := usecase.NewHandlePermissionUseCase(permRepo, nil, permissionLoggerFromContext)

	denied := false
	callback := usecase.PermissionCallback{
		Allow: func() {},
		Deny:  func() { denied = true },
	}

	// Mic request with no stored permission and no dialog should be denied
	permRepo.EXPECT().Get(mock.Anything, "https://meet.example.com", entity.PermissionTypeMicrophone).
		Return(nil, nil)

	uc.HandlePermissionRequest(ctx, "https://meet.example.com", []entity.PermissionType{
		entity.PermissionTypeMicrophone,
	}, callback)

	assert.True(t, denied, "should deny when no dialog presenter is available")
}

func TestHandlePermissionUseCase_DisplayOverrideDenied(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog, permissionLoggerFromContext)

	permRepo.EXPECT().Get(mock.Anything, "https://meet.example.com", entity.PermissionTypeDisplay).
		Return(&entity.PermissionRecord{
			Origin:   "https://meet.example.com",
			Type:     entity.PermissionTypeDisplay,
			Decision: entity.PermissionDenied,
		}, nil)

	denied := false
	callback := usecase.PermissionCallback{
		Allow: func() {},
		Deny:  func() { denied = true },
	}

	uc.HandlePermissionRequest(ctx, "https://meet.example.com", []entity.PermissionType{
		entity.PermissionTypeDisplay,
	}, callback)

	assert.True(t, denied, "display should be denied when manual override is denied")
	dialog.AssertNotCalled(t, "ShowPermissionDialog")
}

func TestHandlePermissionUseCase_GetSetResetManualDecision(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	uc := usecase.NewHandlePermissionUseCase(permRepo, nil, permissionLoggerFromContext)

	permRepo.EXPECT().Set(mock.Anything, mock.AnythingOfType("*entity.PermissionRecord")).
		Run(func(_ context.Context, record *entity.PermissionRecord) {
			assert.Equal(t, "https://app.zoom.us", record.Origin)
			assert.Equal(t, entity.PermissionTypeMicrophone, record.Type)
			assert.Equal(t, entity.PermissionGranted, record.Decision)
		}).
		Return(nil)

	err := uc.SetManualPermissionDecision(
		ctx,
		"https://app.zoom.us",
		entity.PermissionTypeMicrophone,
		entity.PermissionGranted,
	)
	require.NoError(t, err)

	permRepo.EXPECT().Get(mock.Anything, "https://app.zoom.us", entity.PermissionTypeMicrophone).
		Return(&entity.PermissionRecord{
			Origin:   "https://app.zoom.us",
			Type:     entity.PermissionTypeMicrophone,
			Decision: entity.PermissionGranted,
		}, nil)

	record, err := uc.GetManualPermissionDecision(ctx, "https://app.zoom.us", entity.PermissionTypeMicrophone)
	require.NoError(t, err)
	if assert.NotNil(t, record) {
		assert.Equal(t, entity.PermissionGranted, record.Decision)
	}

	permRepo.EXPECT().Delete(mock.Anything, "https://app.zoom.us", entity.PermissionTypeMicrophone).
		Return(nil)
	err = uc.ResetManualPermissionDecision(ctx, "https://app.zoom.us", entity.PermissionTypeMicrophone)
	require.NoError(t, err)
}

func TestHandlePermissionUseCase_SetManualPermissionDecision_NonPersistable(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	uc := usecase.NewHandlePermissionUseCase(permRepo, nil, permissionLoggerFromContext)

	err := uc.SetManualPermissionDecision(
		ctx,
		"https://app.zoom.us",
		entity.PermissionTypeDisplay,
		entity.PermissionDenied,
	)

	require.Error(t, err)
	require.ErrorContains(t, err, "permission type not persistable")
	permRepo.AssertNotCalled(t, "Set")
	permRepo.AssertNotCalled(t, "Delete")
}

func TestHandlePermissionUseCase_UsesInjectedLoggerFactory(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	called := false
	loggerFactory := func(ctx context.Context) *zerolog.Logger {
		called = true
		return zerolog.Ctx(ctx)
	}

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog, loggerFactory)
	uc.HandlePermissionRequest(ctx, "", []entity.PermissionType{entity.PermissionTypeMicrophone}, usecase.PermissionCallback{
		Allow: func() {},
		Deny:  func() {},
	})

	assert.True(t, called)
}
