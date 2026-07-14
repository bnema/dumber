package cef

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffectiveViewportVisibilityRequiresMappedVisibleAncestors(t *testing.T) {
	tests := []struct {
		name    string
		states  []viewportWidgetVisibility
		visible bool
	}{
		{name: "mapped visible widget", states: []viewportWidgetVisibility{{visible: true, mapped: true}}, visible: true},
		{name: "unmapped widget", states: []viewportWidgetVisibility{{visible: true, mapped: false}}},
		{name: "hidden widget", states: []viewportWidgetVisibility{{visible: false, mapped: true}}},
		{name: "hidden ancestor", states: []viewportWidgetVisibility{{visible: true, mapped: true}, {visible: false, mapped: true}}},
		{name: "unmapped ancestor", states: []viewportWidgetVisibility{{visible: true, mapped: true}, {visible: true, mapped: false}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.visible, effectiveViewportVisibility(tt.states))
		})
	}
}

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
