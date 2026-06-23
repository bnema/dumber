package coordinator

import (
	"context"
	"testing"

	coordinatormocks "github.com/bnema/dumber/internal/ui/coordinator/mocks"
	"github.com/stretchr/testify/mock"
)

func TestNavigationCoordinator_OmniboxProviderOpenAndZoom(t *testing.T) {
	c := &NavigationCoordinator{}
	provider := coordinatormocks.NewMockOmniboxProvider(t)
	provider.EXPECT().ToggleOmnibox(mock.Anything).Once()
	provider.EXPECT().UpdateOmniboxZoom(1.25).Once()

	c.SetOmniboxProvider(provider)
	if err := c.OpenOmnibox(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.NotifyZoomChanged(context.Background(), 1.25)
}
