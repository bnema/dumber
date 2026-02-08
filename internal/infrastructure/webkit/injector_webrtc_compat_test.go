package webkit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildWebRTCCompatScript_AddsPrefixedAliases(t *testing.T) {
	script := buildWebRTCCompatScript()

	assert.Contains(t, script, "window.webkitRTCPeerConnection")
	assert.Regexp(t, `window\.RTCPeerConnection\s*=\s*window\.webkitRTCPeerConnection`, script)
	assert.Regexp(t, `window\.RTCSessionDescription\s*=\s*window\.webkitRTCSessionDescription`, script)
	assert.Regexp(t, `window\.RTCIceCandidate\s*=\s*window\.webkitRTCIceCandidate`, script)
}
