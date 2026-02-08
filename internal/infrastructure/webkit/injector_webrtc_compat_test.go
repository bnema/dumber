package webkit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildWebRTCCompatScript_AddsPrefixedAliases(t *testing.T) {
	script := buildWebRTCCompatScript()

	assert.Contains(t, script, "window.webkitRTCPeerConnection")
	assert.Contains(t, script, "window.RTCPeerConnection = window.webkitRTCPeerConnection")
	assert.Contains(t, script, "window.RTCSessionDescription = window.webkitRTCSessionDescription")
	assert.Contains(t, script, "window.RTCIceCandidate = window.webkitRTCIceCandidate")
}
