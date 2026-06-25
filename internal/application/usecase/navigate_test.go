package usecase

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/require"
)

type fakeWebView struct {
	loaded  string
	loadErr error
}

func (f *fakeWebView) LoadURI(_ context.Context, uri string) error {
	if f.loadErr != nil {
		return f.loadErr
	}
	f.loaded = uri
	return nil
}
func (*fakeWebView) LoadHTML(context.Context, string, string) error { return nil }
func (*fakeWebView) Reload(context.Context) error                   { return nil }
func (*fakeWebView) ReloadBypassCache(context.Context) error        { return nil }
func (*fakeWebView) Stop(context.Context) error                     { return nil }
func (*fakeWebView) GoBack(context.Context) error                   { return nil }
func (*fakeWebView) GoForward(context.Context) error                { return nil }
func (*fakeWebView) ID() port.WebViewID                             { return 1 }
func (*fakeWebView) State() port.WebViewState                       { return port.WebViewState{} }
func (f *fakeWebView) URI() string                                  { return f.loaded }
func (*fakeWebView) Title() string                                  { return "" }
func (*fakeWebView) IsLoading() bool                                { return false }
func (*fakeWebView) EstimatedProgress() float64                     { return 1 }
func (*fakeWebView) CanGoBack() bool                                { return false }
func (*fakeWebView) CanGoForward() bool                             { return false }
func (*fakeWebView) SetZoomLevel(context.Context, float64) error    { return nil }
func (*fakeWebView) GetZoomLevel() float64                          { return 1 }
func (*fakeWebView) GetFindController() port.FindController         { return nil }
func (*fakeWebView) SetCallbacks(*port.WebViewCallbacks)            {}
func (*fakeWebView) RunJavaScript(context.Context, string)          {}
func (*fakeWebView) SetBackgroundColor(float64, float64, float64, float64) {
}
func (*fakeWebView) ResetBackgroundToDefault() {}
func (*fakeWebView) Favicon() port.Texture     { return nil }
func (*fakeWebView) Generation() uint64        { return 0 }
func (*fakeWebView) IsFullscreen() bool        { return false }
func (*fakeWebView) IsPlayingAudio() bool      { return false }
func (*fakeWebView) IsDestroyed() bool         { return false }
func (*fakeWebView) Destroy()                  {}

func TestNavigateUseCase_ExecuteLoadsURLAndReturnsDefaultZoom(t *testing.T) {
	ctx := context.Background()
	wv := &fakeWebView{}
	uc := NewNavigateUseCase(entity.ZoomDefault)

	out, err := uc.Execute(ctx, NavigateInput{URL: "https://example.com", PaneID: "pane-1", WebView: wv})

	require.NoError(t, err)
	require.Equal(t, "https://example.com", wv.loaded)
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
			wv := &fakeWebView{}
			uc := NewNavigateUseCase(tt.configuredZoom)

			out, err := uc.Execute(ctx, NavigateInput{URL: "https://example.com", PaneID: "pane-1", WebView: wv})

			require.NoError(t, err)
			require.Equal(t, "https://example.com", wv.loaded)
			require.InDelta(t, tt.wantZoom, out.AppliedZoom, 0.0001)
		})
	}
}

func TestNavigateUseCase_ExecuteReturnsLoadError(t *testing.T) {
	ctx := context.Background()
	loadErr := errors.New("load failed")
	wv := &fakeWebView{loadErr: loadErr}
	uc := NewNavigateUseCase(entity.ZoomDefault)

	out, err := uc.Execute(ctx, NavigateInput{URL: "https://example.com", PaneID: "pane-1", WebView: wv})

	require.Nil(t, out)
	require.ErrorIs(t, err, loadErr)
	require.Contains(t, err.Error(), "failed to load URL")
}
