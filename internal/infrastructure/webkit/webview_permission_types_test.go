package webkit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyUserMediaPermissionTypes(t *testing.T) {
	tests := []struct {
		name      string
		isAudio   bool
		isVideo   bool
		isDisplay bool
		expected  []string
	}{
		{
			name:      "audio and camera",
			isAudio:   true,
			isVideo:   true,
			isDisplay: false,
			expected:  []string{"microphone", "camera"},
		},
		{
			name:      "screen only",
			isAudio:   false,
			isVideo:   false,
			isDisplay: true,
			expected:  []string{"display"},
		},
		{
			name:      "screen with audio",
			isAudio:   true,
			isVideo:   false,
			isDisplay: true,
			expected:  []string{"microphone", "display"},
		},
		{
			name:      "webkit display fallback when all flags false",
			isAudio:   false,
			isVideo:   false,
			isDisplay: false,
			expected:  []string{"display"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, classifyUserMediaPermissionTypes(tt.isAudio, tt.isVideo, tt.isDisplay))
		})
	}
}

func TestClassifyPermissionRequestTypes(t *testing.T) {
	tests := []struct {
		name      string
		kind      permissionRequestKind
		isAudio   bool
		isVideo   bool
		isDisplay bool
		expected  []string
	}{
		{
			name:     "device info request stays device_info",
			kind:     permissionRequestKindDeviceInfo,
			expected: []string{"device_info"},
		},
		{
			name:      "user media camera",
			kind:      permissionRequestKindUserMedia,
			isAudio:   false,
			isVideo:   true,
			isDisplay: false,
			expected:  []string{"camera"},
		},
		{
			name:      "user media fallback display",
			kind:      permissionRequestKindUserMedia,
			isAudio:   false,
			isVideo:   false,
			isDisplay: false,
			expected:  []string{"display"},
		},
		{
			name:     "unknown request denied",
			kind:     permissionRequestKindUnknown,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, classifyPermissionRequestTypes(tt.kind, tt.isAudio, tt.isVideo, tt.isDisplay))
		})
	}
}
