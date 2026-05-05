package coordinator

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
)

type mockDevToolsWebView struct {
	*mocks.MockWebView
	*mocks.MockDevToolsOpener
}

type mockPrinterWebView struct {
	*mocks.MockWebView
	*mocks.MockPrinter
}

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
	t.Run("OpenDevToolsWebView with nil webview returns error", func(t *testing.T) {
		testOpenDevToolsWebViewNilWebView(t, ctx)
	})
	t.Run("OpenDevToolsWebView unsupported capability returns error", func(t *testing.T) {
		testOpenDevToolsWebViewUnsupportedCapability(t, ctx)
	})
	t.Run("OpenDevToolsWebView calls OpenDevTools once", func(t *testing.T) {
		testOpenDevToolsWebViewTargetsProvidedWebView(t, ctx)
	})
	t.Run("PrintWebView with nil webview returns error", func(t *testing.T) {
		testPrintWebViewNilWebView(t, ctx)
	})
	t.Run("PrintWebView unsupported capability returns error", func(t *testing.T) {
		testPrintWebViewUnsupportedCapability(t, ctx)
	})
	t.Run("PrintWebView calls PrintPage once", func(t *testing.T) {
		testPrintWebViewTargetsProvidedWebView(t, ctx)
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

func testOpenDevToolsWebViewNilWebView(t *testing.T, ctx context.Context) {
	t.Helper()
	c := &NavigationCoordinator{}
	err := c.OpenDevToolsWebView(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil webview, got nil")
	}
}

func testOpenDevToolsWebViewUnsupportedCapability(t *testing.T, ctx context.Context) {
	t.Helper()
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(port.WebViewID(1)).Once()
	c := &NavigationCoordinator{}
	err := c.OpenDevToolsWebView(ctx, wv)
	if err == nil {
		t.Fatal("expected unsupported devtools error, got nil")
	}
	if got, want := err.Error(), "webview does not support devtools"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func testOpenDevToolsWebViewTargetsProvidedWebView(t *testing.T, ctx context.Context) {
	t.Helper()
	base := mocks.NewMockWebView(t)
	opener := mocks.NewMockDevToolsOpener(t)
	wv := &mockDevToolsWebView{MockWebView: base, MockDevToolsOpener: opener}
	base.EXPECT().ID().Return(port.WebViewID(1)).Once()
	opener.EXPECT().OpenDevTools().Return().Once()
	c := &NavigationCoordinator{}
	err := c.OpenDevToolsWebView(ctx, wv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func testPrintWebViewNilWebView(t *testing.T, ctx context.Context) {
	t.Helper()
	c := &NavigationCoordinator{}
	err := c.PrintWebView(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil webview, got nil")
	}
}

func testPrintWebViewUnsupportedCapability(t *testing.T, ctx context.Context) {
	t.Helper()
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(port.WebViewID(1)).Once()
	c := &NavigationCoordinator{}
	err := c.PrintWebView(ctx, wv)
	if err == nil {
		t.Fatal("expected unsupported printing error, got nil")
	}
	if got, want := err.Error(), "webview does not support printing"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func testPrintWebViewTargetsProvidedWebView(t *testing.T, ctx context.Context) {
	t.Helper()
	base := mocks.NewMockWebView(t)
	printer := mocks.NewMockPrinter(t)
	wv := &mockPrinterWebView{MockWebView: base, MockPrinter: printer}
	base.EXPECT().ID().Return(port.WebViewID(1)).Once()
	printer.EXPECT().PrintPage().Return().Once()
	c := &NavigationCoordinator{}
	err := c.PrintWebView(ctx, wv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
