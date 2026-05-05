package coordinator

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
)

// TestNavigationCoordinator_WebViewTargetedActionsUseProvidedWebView verifies that
// the explicit WebView-targeted navigation methods call the correct methods on the
// provided WebView rather than resolving through the content coordinator.
func TestNavigationCoordinator_WebViewTargetedActionsUseProvidedWebView(t *testing.T) {
	ctx := context.Background()

	t.Run("NavigateWebView with nil webview returns error", func(t *testing.T) {
		testNavigateWebViewNilWebView(t, ctx)
	})
	t.Run("NavigateWebView targets provided webview", func(t *testing.T) {
		testNavigateWebViewTargetsProvidedWebView(t, ctx)
	})
	t.Run("ReloadWebView with nil webview returns error", func(t *testing.T) {
		testReloadWebViewNilWebView(t, ctx)
	})
	t.Run("ReloadWebView bypassCache=false calls Reload once", func(t *testing.T) {
		testReloadWebViewCallsReload(t, ctx)
	})
	t.Run("ReloadWebView bypassCache=true calls ReloadBypassCache once", func(t *testing.T) {
		testReloadWebViewCallsReloadBypassCache(t, ctx)
	})
	t.Run("StopWebView with nil webview returns error", func(t *testing.T) {
		testStopWebViewNilWebView(t, ctx)
	})
	t.Run("StopWebView calls Stop once", func(t *testing.T) {
		testStopWebViewCallsStop(t, ctx)
	})
	t.Run("GoBackWebView with nil webview returns error", func(t *testing.T) {
		testGoBackWebViewNilWebView(t, ctx)
	})
	t.Run("GoBackWebView calls GoBack once", func(t *testing.T) {
		testGoBackWebViewCallsGoBack(t, ctx)
	})
	t.Run("GoForwardWebView with nil webview returns error", func(t *testing.T) {
		testGoForwardWebViewNilWebView(t, ctx)
	})
	t.Run("GoForwardWebView calls GoForward once", func(t *testing.T) {
		testGoForwardWebViewCallsGoForward(t, ctx)
	})
}

func testNavigateWebViewNilWebView(t *testing.T, ctx context.Context) {
	t.Helper()
	c := &NavigationCoordinator{}
	err := c.NavigateWebView(ctx, "https://example.com", entity.PaneID("p1"), nil)
	if err == nil {
		t.Fatal("expected error for nil webview, got nil")
	}
}

func testNavigateWebViewTargetsProvidedWebView(t *testing.T, ctx context.Context) {
	t.Helper()
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().LoadURI(ctx, "https://example.com").Return(nil).Once()
	wv.EXPECT().ID().Return(port.WebViewID(42)).Once()
	c := &NavigationCoordinator{}
	err := c.NavigateWebView(ctx, "https://example.com", entity.PaneID("p1"), wv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func testReloadWebViewNilWebView(t *testing.T, ctx context.Context) {
	t.Helper()
	c := &NavigationCoordinator{}
	err := c.ReloadWebView(ctx, nil, false)
	if err == nil {
		t.Fatal("expected error for nil webview, got nil")
	}
}

func testReloadWebViewCallsReload(t *testing.T, ctx context.Context) {
	t.Helper()
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(port.WebViewID(1)).Once()
	wv.EXPECT().Reload(ctx).Return(nil).Once()
	c := &NavigationCoordinator{}
	err := c.ReloadWebView(ctx, wv, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func testReloadWebViewCallsReloadBypassCache(t *testing.T, ctx context.Context) {
	t.Helper()
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(port.WebViewID(1)).Once()
	wv.EXPECT().ReloadBypassCache(ctx).Return(nil).Once()
	c := &NavigationCoordinator{}
	err := c.ReloadWebView(ctx, wv, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func testStopWebViewNilWebView(t *testing.T, ctx context.Context) {
	t.Helper()
	c := &NavigationCoordinator{}
	err := c.StopWebView(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil webview, got nil")
	}
}

func testStopWebViewCallsStop(t *testing.T, ctx context.Context) {
	t.Helper()
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(port.WebViewID(1)).Once()
	wv.EXPECT().Stop(ctx).Return(nil).Once()
	c := &NavigationCoordinator{}
	err := c.StopWebView(ctx, wv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func testGoBackWebViewNilWebView(t *testing.T, ctx context.Context) {
	t.Helper()
	c := &NavigationCoordinator{}
	err := c.GoBackWebView(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil webview, got nil")
	}
}

func testGoBackWebViewCallsGoBack(t *testing.T, ctx context.Context) {
	t.Helper()
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(port.WebViewID(1)).Once()
	wv.EXPECT().GoBack(ctx).Return(nil).Once()
	c := &NavigationCoordinator{}
	err := c.GoBackWebView(ctx, wv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func testGoForwardWebViewNilWebView(t *testing.T, ctx context.Context) {
	t.Helper()
	c := &NavigationCoordinator{}
	err := c.GoForwardWebView(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil webview, got nil")
	}
}

func testGoForwardWebViewCallsGoForward(t *testing.T, ctx context.Context) {
	t.Helper()
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(port.WebViewID(1)).Once()
	wv.EXPECT().GoForward(ctx).Return(nil).Once()
	c := &NavigationCoordinator{}
	err := c.GoForwardWebView(ctx, wv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
