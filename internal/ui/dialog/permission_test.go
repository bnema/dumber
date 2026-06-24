package dialog

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	dialogmocks "github.com/bnema/dumber/internal/ui/dialog/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type permissionPopupShowCall struct {
	heading string
	body    string
}

func TestPermissionDialog_QueuesRequestsWhilePopupVisible(t *testing.T) {
	popup := dialogmocks.NewMockPermissionPopup(t)
	var showCalls []permissionPopupShowCall
	var callback func(allowed, persistent bool)
	popup.EXPECT().
		Show(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ context.Context, heading string, body string, cb func(allowed, persistent bool)) {
			showCalls = append(showCalls, permissionPopupShowCall{
				heading: heading,
				body:    body,
			})
			callback = cb
		}).
		Twice()

	respond := func(allowed, persistent bool) {
		if callback == nil {
			return
		}
		cb := callback
		callback = nil
		cb(allowed, persistent)
	}

	d := &PermissionDialog{popup: popup}

	var firstResult port.PermissionDialogResult
	firstCalled := false
	d.ShowPermissionDialog(
		context.Background(),
		"https://example.com",
		[]entity.PermissionType{entity.PermissionTypeMicrophone},
		nil,
		func(result port.PermissionDialogResult) {
			firstCalled = true
			firstResult = result
		},
	)

	secondCalled := false
	var secondResult port.PermissionDialogResult
	d.ShowPermissionDialog(
		context.Background(),
		"https://example.com",
		[]entity.PermissionType{entity.PermissionTypeCamera},
		nil,
		func(result port.PermissionDialogResult) {
			secondCalled = true
			secondResult = result
		},
	)

	if assert.Len(t, showCalls, 1) {
		assert.Equal(t, "Allow Microphone Access?", showCalls[0].heading)
	}
	assert.False(t, firstCalled)
	assert.False(t, secondCalled)

	respond(true, false)
	assert.True(t, firstCalled)
	assert.Equal(t, port.PermissionDialogResult{Allowed: true, Persistent: false}, firstResult)

	if assert.Len(t, showCalls, 2) {
		assert.Equal(t, "Allow Camera Access?", showCalls[1].heading)
	}
	assert.False(t, secondCalled)

	respond(false, true)
	assert.True(t, secondCalled)
	assert.Equal(t, port.PermissionDialogResult{Allowed: false, Persistent: true}, secondResult)
}

func TestPermissionDialog_NoPopup_DeniesRequest(t *testing.T) {
	d := &PermissionDialog{popup: nil}

	called := false
	result := port.PermissionDialogResult{}
	d.ShowPermissionDialog(
		context.Background(),
		"https://example.com",
		[]entity.PermissionType{entity.PermissionTypeMicrophone},
		nil,
		func(r port.PermissionDialogResult) {
			called = true
			result = r
		},
	)

	assert.True(t, called)
	assert.Equal(t, port.PermissionDialogResult{Allowed: false, Persistent: false}, result)
}

func TestPermissionDialog_BuildHeadingAndBody_DisplayCombinations(t *testing.T) {
	d := &PermissionDialog{}
	origin := "https://meet.example.com"

	tests := []struct {
		name           string
		permTypes      []entity.PermissionType
		expectHeading  string
		expectedAction string
	}{
		{
			name:           "display only",
			permTypes:      []entity.PermissionType{entity.PermissionTypeDisplay},
			expectHeading:  "Allow Screen Sharing?",
			expectedAction: "share your screen",
		},
		{
			name:           "microphone and display",
			permTypes:      []entity.PermissionType{entity.PermissionTypeMicrophone, entity.PermissionTypeDisplay},
			expectHeading:  "Allow Microphone and Screen Sharing?",
			expectedAction: "access your microphone and share your screen",
		},
		{
			name:           "camera and display",
			permTypes:      []entity.PermissionType{entity.PermissionTypeCamera, entity.PermissionTypeDisplay},
			expectHeading:  "Allow Camera and Screen Sharing?",
			expectedAction: "access your camera and share your screen",
		},
		{
			name:           "all three",
			permTypes:      []entity.PermissionType{entity.PermissionTypeMicrophone, entity.PermissionTypeCamera, entity.PermissionTypeDisplay},
			expectHeading:  "Allow Microphone, Camera, and Screen Sharing?",
			expectedAction: "access your microphone and camera, and share your screen",
		},
		{
			name:           "website data access only",
			permTypes:      []entity.PermissionType{entity.PermissionTypeWebsiteDataAccess},
			expectHeading:  "Allow Third-Party Data Access?",
			expectedAction: "access its stored data while you browse this site",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectHeading, d.buildHeading(tt.permTypes))
			assert.Equal(t, origin+" wants to "+tt.expectedAction+".", d.buildBody(origin, tt.permTypes, nil))
		})
	}

	// Test website_data_access with populated metadata (Epiphany-style rich message)
	t.Run("website data access with both domains", func(t *testing.T) {
		meta := entity.PermissionMetadata{
			entity.PermissionMetadataKeyRequestingDomain: "accounts.google.com",
			entity.PermissionMetadataKeyCurrentDomain:    "shop.example.com",
		}
		body := d.buildBody(origin, []entity.PermissionType{entity.PermissionTypeWebsiteDataAccess}, meta)
		assert.Equal(t, origin+" wants to allow accounts.google.com to access its data (including cookies) while you browse shop.example.com.", body)
	})
}
