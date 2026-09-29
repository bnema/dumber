package usecase

import (
	"context"
	"errors"
	"math"
	"testing"

	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newLoadingNavigator(t *testing.T, url string, err error) *portmocks.MockPageNavigator {
	t.Helper()
	nav := portmocks.NewMockPageNavigator(t)
	nav.EXPECT().LoadURI(mock.Anything, url).Return(err).Once()
	return nav
}

func TestNavigateUseCase_ExecuteLoadsURLAndReturnsDefaultZoom(t *testing.T) {
	ctx := context.Background()
	wv := newLoadingNavigator(t, "https://example.com", nil)
	uc := NewNavigateUseCase(entity.ZoomDefault)

	out, err := uc.Execute(ctx, NavigateInput{URL: "https://example.com", PaneID: "pane-1", WebView: wv})

	require.NoError(t, err)
	require.InDelta(t, entity.ZoomDefault, out.AppliedZoom, 0.0001)
}

func TestNavigateUseCase_ExecuteUsesConfiguredOrFallbackZoom(t *testing.T) {
	tests := []struct {
		name           string
		configuredZoom float64
		wantZoom       float64
	}{
		{name: "zero falls back", configuredZoom: 0, wantZoom: entity.ZoomDefault},
		{name: "negative falls back", configuredZoom: -0.5, wantZoom: entity.ZoomDefault},
		{name: "NaN falls back", configuredZoom: math.NaN(), wantZoom: entity.ZoomDefault},
		{name: "positive infinity falls back", configuredZoom: math.Inf(1), wantZoom: entity.ZoomDefault},
		{name: "positive is preserved", configuredZoom: 1.25, wantZoom: 1.25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			wv := newLoadingNavigator(t, "https://example.com", nil)
			uc := NewNavigateUseCase(tt.configuredZoom)

			out, err := uc.Execute(ctx, NavigateInput{URL: "https://example.com", PaneID: "pane-1", WebView: wv})

			require.NoError(t, err)
			require.InDelta(t, tt.wantZoom, out.AppliedZoom, 0.0001)
		})
	}
}

func TestNavigateUseCase_ExecuteReturnsLoadError(t *testing.T) {
	ctx := context.Background()
	loadErr := errors.New("load failed")
	wv := newLoadingNavigator(t, "https://example.com", loadErr)
	uc := NewNavigateUseCase(entity.ZoomDefault)

	out, err := uc.Execute(ctx, NavigateInput{URL: "https://example.com", PaneID: "pane-1", WebView: wv})

	require.Nil(t, out)
	require.ErrorIs(t, err, loadErr)
	require.Contains(t, err.Error(), "failed to load URL")
}

func TestNavigateUseCase_ReloadHonoursBypassCache(t *testing.T) {
	ctx := context.Background()
	uc := NewNavigateUseCase(entity.ZoomDefault)

	nav := portmocks.NewMockPageNavigator(t)
	nav.EXPECT().ReloadBypassCache(mock.Anything).Return(nil).Once()
	require.NoError(t, uc.Reload(ctx, nav, true))

	nav = portmocks.NewMockPageNavigator(t)
	nav.EXPECT().Reload(mock.Anything).Return(nil).Once()
	require.NoError(t, uc.Reload(ctx, nav, false))
}
