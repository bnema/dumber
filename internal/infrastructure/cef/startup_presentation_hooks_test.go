package cef

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStartupPresentationHooksWireEveryUpstreamFirstPresentationCallback(t *testing.T) {
	hooks := startupPresentationHooks()

	require.NotNil(t, hooks.OnFirstAcceleratedPaint)
	require.NotNil(t, hooks.OnFirstDMABUFTextureSwap)
	require.NotNil(t, hooks.OnFirstPresentation)
	require.NotNil(t, hooks.OnDMABUFUnsupported)
}
