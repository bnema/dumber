package webkit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveReusePolicy(t *testing.T) {
	t.Setenv("DUMBER_WEBVIEW_REUSE_POLICY", "")
	assert.Equal(t, reusePolicyOff, resolveReusePolicy(""))
	assert.Equal(t, reusePolicySafe, resolveReusePolicy("safe"))
	assert.Equal(t, reusePolicyAggressive, resolveReusePolicy("aggressive"))
	assert.Equal(t, reusePolicyOff, resolveReusePolicy("unknown"))

	t.Setenv("DUMBER_WEBVIEW_REUSE_POLICY", "safe")
	assert.Equal(t, reusePolicySafe, resolveReusePolicy(""))
}
