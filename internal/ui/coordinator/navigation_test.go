package coordinator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeOmniboxProvider struct {
	toggled     bool
	zoomUpdates []float64
}

func (f *fakeOmniboxProvider) ToggleOmnibox(context.Context) { f.toggled = true }

func (f *fakeOmniboxProvider) UpdateOmniboxZoom(factor float64) {
	f.zoomUpdates = append(f.zoomUpdates, factor)
}

func TestNavigationCoordinator_OmniboxProviderOpenAndZoom(t *testing.T) {
	c := &NavigationCoordinator{}
	provider := &fakeOmniboxProvider{}

	c.SetOmniboxProvider(provider)
	require.NoError(t, c.OpenOmnibox(context.Background()))
	c.NotifyZoomChanged(context.Background(), 1.25)

	require.True(t, provider.toggled)
	require.Equal(t, []float64{1.25}, provider.zoomUpdates)
}
