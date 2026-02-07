package webkit

import (
	"testing"

	webkitlib "github.com/bnema/puregotk-webkit/webkit"
	"github.com/stretchr/testify/assert"
)

func TestWebProcessTerminationReasonString(t *testing.T) {
	assert.Equal(t, "crashed", webProcessTerminationReasonString(webkitlib.WebProcessCrashedValue))
	assert.Equal(t, "exceeded_memory", webProcessTerminationReasonString(webkitlib.WebProcessExceededMemoryLimitValue))
	assert.Equal(t, "terminated_by_api", webProcessTerminationReasonString(webkitlib.WebProcessTerminatedByApiValue))
	assert.Equal(t, "unknown", webProcessTerminationReasonString(webkitlib.WebProcessTerminationReason(99)))
}

func TestResolveTerminatePolicy(t *testing.T) {
	t.Setenv("DUMBER_WEBVIEW_TERMINATE_POLICY", "")
	assert.Equal(t, terminatePolicyAuto, resolveTerminatePolicy(""))
	assert.Equal(t, terminatePolicyAlways, resolveTerminatePolicy("always"))
	assert.Equal(t, terminatePolicyNever, resolveTerminatePolicy("never"))
	assert.Equal(t, terminatePolicyAuto, resolveTerminatePolicy("invalid"))

	t.Setenv("DUMBER_WEBVIEW_TERMINATE_POLICY", "always")
	assert.Equal(t, terminatePolicyAlways, resolveTerminatePolicy(""))
}

func TestShouldTerminateWebProcess(t *testing.T) {
	assert.False(t, shouldTerminateWebProcess(terminatePolicyAlways, true, true))
	assert.False(t, shouldTerminateWebProcess(terminatePolicyNever, false, true))
	assert.True(t, shouldTerminateWebProcess(terminatePolicyAlways, false, false))
	assert.True(t, shouldTerminateWebProcess(terminatePolicyAuto, false, true))
	assert.False(t, shouldTerminateWebProcess(terminatePolicyAuto, false, false))
}
