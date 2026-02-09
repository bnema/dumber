package component

import (
	"context"
	"sync"
	"testing"

	layoutmocks "github.com/bnema/dumber/internal/ui/layout/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFloatingPane_HideShowPreservesState(t *testing.T) {
	overlay := layoutmocks.NewMockOverlayWidget(t)
	overlay.EXPECT().GetAllocatedWidth().Return(1200).Maybe()
	overlay.EXPECT().GetAllocatedHeight().Return(800).Maybe()

	navigated := make([]string, 0)
	fp := NewFloatingPane(overlay, FloatingPaneOptions{
		WidthPct:       0.82,
		HeightPct:      0.72,
		FallbackWidth:  1200,
		FallbackHeight: 800,
		OnNavigate: func(_ context.Context, url string) error {
			navigated = append(navigated, url)
			return nil
		},
	})

	require.NoError(t, fp.ShowToggle(context.Background()))
	require.Equal(t, "about:blank", fp.CurrentURL())

	require.NoError(t, fp.Navigate(context.Background(), "https://google.com"))
	fp.Hide(context.Background())
	require.NoError(t, fp.ShowToggle(context.Background()))

	assert.Equal(t, "https://google.com", fp.CurrentURL())
	assert.False(t, fp.IsOmniboxVisible())
	assert.Equal(t, []string{"about:blank", "https://google.com"}, navigated)
}

func TestFloatingPane_ShowURLLoadsTarget(t *testing.T) {
	overlay := layoutmocks.NewMockOverlayWidget(t)
	overlay.EXPECT().GetAllocatedWidth().Return(1000).Maybe()
	overlay.EXPECT().GetAllocatedHeight().Return(700).Maybe()

	fp := NewFloatingPane(overlay, FloatingPaneOptions{
		WidthPct:       0.82,
		HeightPct:      0.72,
		FallbackWidth:  1000,
		FallbackHeight: 700,
	})

	require.NoError(t, fp.ShowURL(context.Background(), "https://github.com"))

	assert.True(t, fp.IsVisible())
	assert.False(t, fp.IsOmniboxVisible())
	assert.Equal(t, "https://github.com", fp.CurrentURL())
}

func TestFloatingPane_ResizeUsesWorkspacePercentages(t *testing.T) {
	overlay := layoutmocks.NewMockOverlayWidget(t)
	overlay.EXPECT().GetAllocatedWidth().Return(1000).Maybe()
	overlay.EXPECT().GetAllocatedHeight().Return(700).Maybe()

	fp := NewFloatingPane(overlay, FloatingPaneOptions{
		WidthPct:       0.82,
		HeightPct:      0.72,
		FallbackWidth:  1000,
		FallbackHeight: 700,
	})

	fp.Resize()
	width, height := fp.Dimensions()

	assert.Equal(t, 820, width)
	assert.Equal(t, 504, height)
}

func TestFloatingPane_FirstToggleOpensBlankAndOmnibox(t *testing.T) {
	overlay := layoutmocks.NewMockOverlayWidget(t)
	overlay.EXPECT().GetAllocatedWidth().Return(1200).Maybe()
	overlay.EXPECT().GetAllocatedHeight().Return(800).Maybe()

	fp := NewFloatingPane(overlay, FloatingPaneOptions{
		WidthPct:       0.82,
		HeightPct:      0.72,
		FallbackWidth:  1200,
		FallbackHeight: 800,
	})

	require.NoError(t, fp.ShowToggle(context.Background()))

	assert.True(t, fp.IsVisible())
	assert.True(t, fp.IsOmniboxVisible())
	assert.Equal(t, "about:blank", fp.CurrentURL())
}

func TestFloatingPane_SetOmniboxVisible(t *testing.T) {
	fp := NewFloatingPane(nil, FloatingPaneOptions{
		WidthPct:       0.82,
		HeightPct:      0.72,
		FallbackWidth:  1200,
		FallbackHeight: 800,
	})

	fp.SetOmniboxVisible(true)
	assert.True(t, fp.IsOmniboxVisible())

	fp.SetOmniboxVisible(false)
	assert.False(t, fp.IsOmniboxVisible())
}

func TestFloatingPane_ConcurrentResizeAndParentSwitch(t *testing.T) {
	overlayA := layoutmocks.NewMockOverlayWidget(t)
	overlayA.EXPECT().GetAllocatedWidth().Return(1200).Maybe()
	overlayA.EXPECT().GetAllocatedHeight().Return(800).Maybe()

	overlayB := layoutmocks.NewMockOverlayWidget(t)
	overlayB.EXPECT().GetAllocatedWidth().Return(1000).Maybe()
	overlayB.EXPECT().GetAllocatedHeight().Return(700).Maybe()

	fp := NewFloatingPane(overlayA, FloatingPaneOptions{
		WidthPct:       0.82,
		HeightPct:      0.72,
		FallbackWidth:  1200,
		FallbackHeight: 800,
	})

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			fp.Resize()
		}()
		go func(iter int) {
			defer wg.Done()
			if iter%2 == 0 {
				fp.SetParentOverlay(overlayA)
				return
			}
			fp.SetParentOverlay(overlayB)
		}(i)
	}
	wg.Wait()

	width, height := fp.Dimensions()
	assert.Positive(t, width)
	assert.Positive(t, height)
}
