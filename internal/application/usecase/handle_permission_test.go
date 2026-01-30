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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestHandlePermissionUseCase_AutoAllowDisplayCapture(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog)

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
	permRepo.AssertNotCalled(t, "Get")
	dialog.AssertNotCalled(t, "ShowPermissionDialog")
}

func TestHandlePermissionUseCase_AutoAllowDeviceInfo(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog)

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

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog)

	// Return stored granted permission
	permRepo.EXPECT().Get(mock.Anything, "https://meet.example.com", entity.PermissionTypeMicrophone).
		Return(&entity.PermissionRecord{
			Origin:    "https://meet.example.com",
			Type:      entity.PermissionTypeMicrophone,
			Decision:  entity.PermissionGranted,
			UpdatedAt: time.Now(),
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

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog)

	// Return stored denied permission
	permRepo.EXPECT().Get(mock.Anything, "https://meet.example.com", entity.PermissionTypeCamera).
		Return(&entity.PermissionRecord{
			Origin:    "https://meet.example.com",
			Type:      entity.PermissionTypeCamera,
			Decision:  entity.PermissionDenied,
			UpdatedAt: time.Now(),
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

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog)

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

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog)

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

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog)

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

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog)

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

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog)

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

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog)

	state := uc.QueryPermissionState(ctx, "https://example.com", entity.PermissionTypeDisplay)

	assert.Equal(t, entity.PermissionGranted, state, "display capture should auto-return granted")
}

func TestHandlePermissionUseCase_QueryPermissionState_Stored(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog)

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

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog)

	permRepo.EXPECT().Get(mock.Anything, "https://meet.example.com", entity.PermissionTypeCamera).
		Return(nil, nil)

	state := uc.QueryPermissionState(ctx, "https://meet.example.com", entity.PermissionTypeCamera)

	assert.Equal(t, entity.PermissionPrompt, state, "no record should return prompt")
}

func TestHandlePermissionUseCase_NonPersistableNotSaved(t *testing.T) {
	ctx := testContext()
	permRepo := repomocks.NewMockPermissionRepository(t)
	dialog := portmocks.NewMockPermissionDialogPresenter(t)

	uc := usecase.NewHandlePermissionUseCase(permRepo, dialog)

	// Display capture is not persistable per W3C spec
	// No Get call since it's auto-allowed
	// No Set call should happen even if we try to persist

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

func TestExtractOrigin_ValidURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{
			name:     "https with port",
			uri:      "https://example.com:8443/path?query=1",
			expected: "https://example.com:8443",
		},
		{
			name:     "https without port",
			uri:      "https://example.com/path",
			expected: "https://example.com",
		},
		{
			name:     "http",
			uri:      "http://localhost:8080/app",
			expected: "http://localhost:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origin, err := usecase.ExtractOrigin(tt.uri)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, origin)
		})
	}
}

func TestExtractOrigin_InvalidURI(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{name: "empty", uri: ""},
		{name: "no scheme", uri: "example.com"},
		{name: "no host", uri: "https://"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origin, err := usecase.ExtractOrigin(tt.uri)
			require.Error(t, err)
			assert.Empty(t, origin)
		})
	}
}
