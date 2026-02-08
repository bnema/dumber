package component

import "github.com/bnema/dumber/internal/domain/entity"

type webrtcPermissionState string

const (
	webrtcPermissionStateIdle       webrtcPermissionState = "idle"
	webrtcPermissionStateRequesting webrtcPermissionState = "requesting"
	webrtcPermissionStateAllowed    webrtcPermissionState = "allowed"
	webrtcPermissionStateBlocked    webrtcPermissionState = "blocked"
)

func summarizeWebRTCPermissionState(states map[entity.PermissionType]webrtcPermissionState) webrtcPermissionState {
	if len(states) == 0 {
		return webrtcPermissionStateIdle
	}

	hasAllowed := false
	hasBlocked := false

	for _, state := range states {
		switch state {
		case webrtcPermissionStateRequesting:
			return webrtcPermissionStateRequesting
		case webrtcPermissionStateBlocked:
			hasBlocked = true
		case webrtcPermissionStateAllowed:
			hasAllowed = true
		}
	}

	if hasBlocked {
		return webrtcPermissionStateBlocked
	}
	if hasAllowed {
		return webrtcPermissionStateAllowed
	}

	return webrtcPermissionStateIdle
}

func shouldShowWebRTCPermissionIndicator(states map[entity.PermissionType]webrtcPermissionState) bool {
	if len(states) == 0 {
		return false
	}

	for _, state := range states {
		if state != webrtcPermissionStateIdle {
			return true
		}
	}

	return false
}
