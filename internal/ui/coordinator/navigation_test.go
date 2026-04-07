package coordinator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeOmniboxProvider struct {
	onNavigate func(string)
}

func (f *fakeOmniboxProvider) ToggleOmnibox(context.Context) {}

func (f *fakeOmniboxProvider) UpdateOmniboxZoom(float64) {}

func (f *fakeOmniboxProvider) SetOmniboxOnNavigate(fn func(url string)) {
	f.onNavigate = fn
}

func TestNavigationCoordinator_NavigateWithoutContentCoordinatorReturnsError(t *testing.T) {
	c := &NavigationCoordinator{}

	err := c.Navigate(context.Background(), "https://example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content coordinator")
}

func TestNavigationCoordinator_OmniboxCallbackDoesNotPanicOnNavigateError(t *testing.T) {
	c := &NavigationCoordinator{}
	provider := &fakeOmniboxProvider{}

	c.SetOmniboxProvider(provider)
	require.NotNil(t, provider.onNavigate)

	assert.NotPanics(t, func() {
		provider.onNavigate("https://example.com")
	})
}
