package component

import (
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
)

func TestWebRTCSummaryState_RequestingWins(t *testing.T) {
	states := map[entity.PermissionType]webrtcPermissionState{
		entity.PermissionTypeMicrophone: webrtcPermissionStateAllowed,
		entity.PermissionTypeCamera:     webrtcPermissionStateRequesting,
	}

	summary := summarizeWebRTCPermissionState(states)
	assert.Equal(t, webrtcPermissionStateRequesting, summary)
}

func TestWebRTCSummaryState_BlockedBeatsAllowed(t *testing.T) {
	states := map[entity.PermissionType]webrtcPermissionState{
		entity.PermissionTypeMicrophone: webrtcPermissionStateAllowed,
		entity.PermissionTypeCamera:     webrtcPermissionStateBlocked,
	}

	summary := summarizeWebRTCPermissionState(states)
	assert.Equal(t, webrtcPermissionStateBlocked, summary)
}

func TestWebRTCSummaryState_AllowedWhenOnlyAllowed(t *testing.T) {
	states := map[entity.PermissionType]webrtcPermissionState{
		entity.PermissionTypeDisplay: webrtcPermissionStateAllowed,
	}

	summary := summarizeWebRTCPermissionState(states)
	assert.Equal(t, webrtcPermissionStateAllowed, summary)
}

func TestWebRTCSummaryState_IdleWhenEmpty(t *testing.T) {
	assert.Equal(t, webrtcPermissionStateIdle, summarizeWebRTCPermissionState(nil))
	assert.Equal(t, webrtcPermissionStateIdle, summarizeWebRTCPermissionState(map[entity.PermissionType]webrtcPermissionState{}))
}

func TestWebRTCSummaryState_IdleWhenAllIdle(t *testing.T) {
	states := map[entity.PermissionType]webrtcPermissionState{
		entity.PermissionTypeMicrophone: webrtcPermissionStateIdle,
		entity.PermissionTypeCamera:     webrtcPermissionStateIdle,
	}

	assert.Equal(t, webrtcPermissionStateIdle, summarizeWebRTCPermissionState(states))
}

func TestWebRTCShouldShowIndicator(t *testing.T) {
	assert.False(t, shouldShowWebRTCPermissionIndicator(nil))
	assert.False(t, shouldShowWebRTCPermissionIndicator(map[entity.PermissionType]webrtcPermissionState{
		entity.PermissionTypeMicrophone: webrtcPermissionStateIdle,
	}))
	assert.True(t, shouldShowWebRTCPermissionIndicator(map[entity.PermissionType]webrtcPermissionState{
		entity.PermissionTypeMicrophone: webrtcPermissionStateAllowed,
	}))
}
