package cef

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWebViewEffectiveVisibilitySendsInitialStateAndDeduplicatesTransitions(t *testing.T) {
	host := &viewportSyncOrderHost{}
	wv := &WebView{}

	wv.applyEffectiveVisibility(host, false)
	require.Equal(t, []string{"WasHidden", "WasHidden(nonzero)"}, host.calls)

	host.calls = nil
	wv.applyEffectiveVisibility(host, false)
	require.Empty(t, host.calls, "unchanged hidden state must not be resent")

	wv.applyEffectiveVisibility(host, true)
	require.Equal(t, []string{"WasHidden"}, host.calls)

	host.calls = nil
	wv.applyEffectiveVisibility(host, true)
	require.Empty(t, host.calls, "unchanged visible state must not be resent")
}
