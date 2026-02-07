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
