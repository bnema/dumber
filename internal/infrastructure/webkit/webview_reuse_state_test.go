package webkit

import (
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/assert"
)

func TestResetForPoolReuse_ClearsBrowsingContextHostState(t *testing.T) {
	wv := &WebView{}
	abortCalled := false

	wv.SetBrowsingContextHostDecision(port.HostDecision{Kind: port.HostDecisionCreateNativeWin})
	wv.SetNativePopupHostAbort(func() { abortCalled = true })

	wv.ResetForPoolReuse()

	decision, hasDecision := wv.BrowsingContextHostDecision()
	assert.False(t, hasDecision)
	assert.Equal(t, port.HostDecision{}, decision)

	wv.AbortNativePopupHost()
	assert.False(t, abortCalled)
}

func TestDestroyWithPolicy_ClearsBrowsingContextHostState(t *testing.T) {
	wv := &WebView{}
	abortCalled := false

	wv.SetBrowsingContextHostDecision(port.HostDecision{Kind: port.HostDecisionCreatePane})
	wv.SetNativePopupHostAbort(func() { abortCalled = true })

	wv.DestroyWithPolicy(terminatePolicyNever)

	decision, hasDecision := wv.BrowsingContextHostDecision()
	assert.False(t, hasDecision)
	assert.Equal(t, port.HostDecision{}, decision)

	wv.AbortNativePopupHost()
	assert.False(t, abortCalled)
}
