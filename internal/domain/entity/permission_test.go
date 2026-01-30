package entity_test

import (
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
)

func TestPermissionType_Constants(t *testing.T) {
	tests := []struct {
		permType entity.PermissionType
		expected string
	}{
		{entity.PermissionTypeMicrophone, "microphone"},
		{entity.PermissionTypeCamera, "camera"},
		{entity.PermissionTypeDisplay, "display"},
		{entity.PermissionTypeDeviceInfo, "device_info"},
		{entity.PermissionTypeClipboard, "clipboard"},
		{entity.PermissionTypeNotification, "notification"},
		{entity.PermissionTypeGeolocation, "geolocation"},
		{entity.PermissionTypePointerLock, "pointer_lock"},
		{entity.PermissionTypeMediaKeySystem, "media_key_system"},
		{entity.PermissionTypeWebsiteDataAccess, "website_data_access"},
	}

	for _, tt := range tests {
		t.Run(string(tt.permType), func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.permType))
		})
	}
}

func TestPermissionDecision_Constants(t *testing.T) {
	tests := []struct {
		decision entity.PermissionDecision
		expected string
	}{
		{entity.PermissionGranted, "granted"},
		{entity.PermissionDenied, "denied"},
		{entity.PermissionPrompt, "prompt"},
	}

	for _, tt := range tests {
		t.Run(string(tt.decision), func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.decision))
		})
	}
}

func TestPermissionRecord_IsGranted(t *testing.T) {
	tests := []struct {
		name     string
		decision entity.PermissionDecision
		expected bool
	}{
		{"granted", entity.PermissionGranted, true},
		{"denied", entity.PermissionDenied, false},
		{"prompt", entity.PermissionPrompt, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := &entity.PermissionRecord{
				Origin:    "https://example.com",
				Type:      entity.PermissionTypeMicrophone,
				Decision:  tt.decision,
				UpdatedAt: time.Now().Unix(),
			}
			assert.Equal(t, tt.expected, record.IsGranted())
		})
	}
}

func TestPermissionRecord_IsDenied(t *testing.T) {
	tests := []struct {
		name     string
		decision entity.PermissionDecision
		expected bool
	}{
		{"granted", entity.PermissionGranted, false},
		{"denied", entity.PermissionDenied, true},
		{"prompt", entity.PermissionPrompt, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := &entity.PermissionRecord{
				Origin:    "https://example.com",
				Type:      entity.PermissionTypeCamera,
				Decision:  tt.decision,
				UpdatedAt: time.Now().Unix(),
			}
			assert.Equal(t, tt.expected, record.IsDenied())
		})
	}
}

func TestCanPersist(t *testing.T) {
	tests := []struct {
		name     string
		permType entity.PermissionType
		expected bool
	}{
		// Non-persistable per W3C spec
		{"display capture", entity.PermissionTypeDisplay, false},
		{"device info", entity.PermissionTypeDeviceInfo, false},
		{"pointer lock", entity.PermissionTypePointerLock, false},

		// Persistable
		{"microphone", entity.PermissionTypeMicrophone, true},
		{"camera", entity.PermissionTypeCamera, true},
		{"clipboard", entity.PermissionTypeClipboard, true},
		{"notification", entity.PermissionTypeNotification, true},
		{"geolocation", entity.PermissionTypeGeolocation, true},
		{"media key system", entity.PermissionTypeMediaKeySystem, true},
		{"website data access", entity.PermissionTypeWebsiteDataAccess, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, entity.CanPersist(tt.permType))
		})
	}
}

func TestIsAutoAllow(t *testing.T) {
	tests := []struct {
		name     string
		permType entity.PermissionType
		expected bool
	}{
		// Auto-allowed (portal handles UI or low risk)
		{"display capture", entity.PermissionTypeDisplay, true},
		{"device info", entity.PermissionTypeDeviceInfo, true},
		{"pointer lock", entity.PermissionTypePointerLock, true},

		// Not auto-allowed (require user consent)
		{"microphone", entity.PermissionTypeMicrophone, false},
		{"camera", entity.PermissionTypeCamera, false},
		{"clipboard", entity.PermissionTypeClipboard, false},
		{"notification", entity.PermissionTypeNotification, false},
		{"geolocation", entity.PermissionTypeGeolocation, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, entity.IsAutoAllow(tt.permType))
		})
	}
}

func TestPermissionSet(t *testing.T) {
	set := entity.PermissionSet{
		Types:    []entity.PermissionType{entity.PermissionTypeMicrophone, entity.PermissionTypeCamera},
		Decision: entity.PermissionGranted,
	}

	assert.Len(t, set.Types, 2)
	assert.Contains(t, set.Types, entity.PermissionTypeMicrophone)
	assert.Contains(t, set.Types, entity.PermissionTypeCamera)
	assert.Equal(t, entity.PermissionGranted, set.Decision)
}
