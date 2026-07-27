package cef

import (
	"bytes"
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/logging"
)

func TestMapCEFPopupFeatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   *purecef.PopupFeatures
		want dto.PopupFeatures
	}{
		{
			name: "nil is known featureless",
			want: dto.PopupFeatures{State: dto.PopupFeaturesNone},
		},
		{
			name: "positive width is popup-like",
			in:   &purecef.PopupFeatures{Width: 640, Widthset: 1},
			want: dto.PopupFeatures{
				State: dto.PopupFeaturesSpecified, Width: 640, WidthSet: true, IsPopupSet: true,
			},
		},
		{
			name: "geometry and flags are normalized",
			in: &purecef.PopupFeatures{
				X: 10, Xset: 1, Y: 20, Yset: 1,
				Width: 800, Widthset: 1, Height: 600, Heightset: 1,
			},
			want: dto.PopupFeatures{
				State: dto.PopupFeaturesSpecified,
				X:     10, XSet: true, Y: 20, YSet: true,
				Width: 800, WidthSet: true, Height: 600, HeightSet: true,
				IsPopupSet: true,
			},
		},
		{
			name: "is popup flag survives without inventing geometry",
			in:   &purecef.PopupFeatures{Ispopup: 1},
			want: dto.PopupFeatures{
				State: dto.PopupFeaturesSpecified, IsPopup: true, IsPopupSet: true,
			},
		},
		{
			name: "allocated defaults are known featureless",
			in:   &purecef.PopupFeatures{},
			want: dto.PopupFeatures{State: dto.PopupFeaturesNone, IsPopupSet: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapCEFPopupFeatures(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOnBeforePopup_PropagatesCEFEngineFeaturesGestureAndNoJavaScriptAccess(t *testing.T) {
	parentWV := &WebView{ctx: context.Background(), id: 16}
	popupWV := &WebView{ctx: context.Background(), id: 24, pendingCreate: &pendingBrowserCreate{}}
	popupWV.markNativePopupCandidate(parentWV)
	popupWV.SetBrowsingContextHostDecision(dto.HostDecision{Kind: dto.HostDecisionCreateBrowserWindow})
	noJavaScriptAccess := true
	parentWV.SetCallbacks(&port.WebViewCallbacks{OnCreate: func(req port.PopupRequest) port.WebView {
		assert.Equal(t, dto.BrowserEngineCEF, req.Engine)
		assert.Equal(t, dto.PopupFeatures{
			State: dto.PopupFeaturesSpecified, X: 10, XSet: true, Width: 640, WidthSet: true,
			IsPopup: true, IsPopupSet: true,
		}, req.PopupFeatures)
		assert.Equal(t, dto.WindowDispositionNewPopup, req.TargetDisposition)
		assert.True(t, req.IsUserGesture)
		assert.True(t, req.NoJavaScriptAccess)
		return popupWV
	}})

	blocked := (&handlerSet{wv: parentWV}).OnBeforePopup(
		nil, nil, 90, "https://example.com/popup", "named-popup",
		purecef.WindowOpenDispositionWodNewPopup, 1,
		&purecef.PopupFeatures{X: 10, Xset: 1, Width: 640, Widthset: 1, Ispopup: 1},
		nil, nil, nil, nil, &noJavaScriptAccess,
	)

	require.True(t, blocked)
	require.True(t, popupWV.nativePopupFallbackStarted)
	require.Nil(t, popupWV.popupOpenerBridgeParent, "noopener must not install the synthetic opener bridge")
}

func TestOnBeforePopup_FeaturelessBlankNavigateSourceBlocksNativePopup(t *testing.T) {
	const targetURI = "https://example.com/source-navigation"
	parentWV := &WebView{ctx: context.Background(), id: 17}
	candidate := portmocks.NewMockWebView(t)
	candidate.EXPECT().Destroy().Once()

	var callbackResult port.WebView = candidate
	parentWV.SetCallbacks(&port.WebViewCallbacks{OnCreate: func(req port.PopupRequest) port.WebView {
		require.Equal(t, dto.BrowserEngineCEF, req.Engine)
		require.Equal(t, "_blank", req.FrameName)
		require.Equal(t, dto.WindowDispositionNewTab, req.TargetDisposition)
		require.Equal(t, dto.PopupFeaturesNone, req.PopupFeatures.State)

		// NavigateSource ownership belongs to the coordinator callback: navigate
		// the source, destroy its unused candidate exactly once, then return nil.
		require.NoError(t, parentWV.LoadURI(context.Background(), req.TargetURI))
		candidate.Destroy()
		callbackResult = nil
		return callbackResult
	}})

	blocked := (&handlerSet{wv: parentWV}).OnBeforePopup(
		nil, nil, 91, targetURI, "_blank",
		purecef.WindowOpenDispositionWodNewForegroundTab, 1, nil, nil, nil, nil, nil, nil,
	)

	require.True(t, blocked, "CEF native popup creation must remain blocked")
	require.Nil(t, callbackResult, "NavigateSource callback must return nil")
	require.Equal(t, targetURI, parentWV.pendingNavigationURI())
}

func TestOnBeforePopup_BrowserWindowDecisionPreservesSyntheticOpenerBridge(t *testing.T) {
	parentWV := &WebView{ctx: context.Background(), id: 17}
	parentWV.updateURI("https://example.com/opener")
	popupWV := &WebView{ctx: context.Background(), id: 25, pendingCreate: &pendingBrowserCreate{}}
	popupWV.markNativePopupCandidate(parentWV)
	popupWV.SetBrowsingContextHostDecision(dto.HostDecision{Kind: dto.HostDecisionCreateBrowserWindow})
	parentWV.SetCallbacks(&port.WebViewCallbacks{OnCreate: func(port.PopupRequest) port.WebView { return popupWV }})

	blocked := (&handlerSet{wv: parentWV}).OnBeforePopup(
		nil, nil, 91, "https://example.com/child", "child",
		purecef.WindowOpenDispositionWodNewPopup, 1, nil, nil, nil, nil, nil, nil,
	)

	require.True(t, blocked)
	require.True(t, popupWV.nativePopupFallbackStarted)
	require.Same(t, parentWV, popupWV.popupOpenerBridgeParent)
	require.Equal(t, "https://example.com/opener", popupWV.popupOpenerBridgeParentURI)
}

func TestOnBeforePopup_NativeArmFailurePreparesEligibleFallbackBeforeAbort(t *testing.T) {
	parentWV := &WebView{ctx: context.Background(), id: 18}
	parentWV.updateURI("https://example.com/opener")
	popupWV := &WebView{ctx: context.Background(), id: 26, pendingCreate: &pendingBrowserCreate{}}
	popupWV.markNativePopupCandidate(parentWV)
	popupWV.SetBrowsingContextHostDecision(dto.HostDecision{Kind: dto.HostDecisionCreateNativePopup})
	abortCalls := 0
	popupWV.SetNativePopupHostAbort(func() {
		abortCalls++
		assert.True(t, popupWV.nativePopupFallbackStarted)
		assert.Same(t, parentWV, popupWV.popupOpenerBridgeParent)
	})
	parentWV.SetCallbacks(&port.WebViewCallbacks{OnCreate: func(port.PopupRequest) port.WebView { return popupWV }})

	blocked := (&handlerSet{wv: parentWV}).OnBeforePopup(
		nil, nil, 92, "https://example.com/visual", "visual",
		purecef.WindowOpenDispositionWodNewPopup, 1,
		&purecef.PopupFeatures{Width: 640, Widthset: 1}, nil, nil, nil, nil, nil,
	)

	require.True(t, blocked)
	require.Equal(t, 1, abortCalls)
	require.False(t, popupWV.IsDestroyed())
}

func TestOnBeforePopup_OpenerRequiredNativeArmFailureDeniesWithoutFallback(t *testing.T) {
	var output bytes.Buffer
	logger := zerolog.New(&output).Level(zerolog.WarnLevel)
	ctx := logging.WithContext(context.Background(), logger)
	parentWV := &WebView{ctx: ctx, id: 19}
	popupWV := &WebView{ctx: context.Background(), id: 27, pendingCreate: &pendingBrowserCreate{}}
	popupWV.markNativePopupCandidate(parentWV)
	popupWV.SetBrowsingContextHostDecision(dto.HostDecision{
		Kind: dto.HostDecisionCreateNativePopup, SourceHost: dto.SourceHostFloating, RequiresNativeOpener: true,
	})
	abortCalls := 0
	popupWV.SetNativePopupHostAbort(func() {
		abortCalls++
		popupWV.Destroy()
	})
	parentWV.SetCallbacks(&port.WebViewCallbacks{OnCreate: func(port.PopupRequest) port.WebView { return popupWV }})

	blocked := (&handlerSet{wv: parentWV}).OnBeforePopup(
		nil, nil, 93, "https://example.com/opener-required", "required",
		purecef.WindowOpenDispositionWodNewPopup, 1, nil, nil, nil, nil, nil, nil,
	)

	require.True(t, blocked)
	require.Equal(t, 1, abortCalls)
	require.False(t, popupWV.nativePopupFallbackStarted)
	require.Nil(t, popupWV.popupOpenerBridgeParent)
	require.True(t, popupWV.IsDestroyed())
	var record map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &record))
	assert.Equal(t, "cef", record["engine"])
	assert.Equal(t, "floating", record["source_host"])
	assert.Equal(t, "create-native-popup", record["decision"])
	assert.Equal(t, "new-popup", record["target_disposition"])
	assert.Equal(t, "native-arm-failed", record["reason_code"])
	assert.Equal(t, true, record["host_abort_invoked"])
	assert.Equal(t, "cef: native popup arming failed", record["message"])
}

func TestOnBeforePopup_NativeArmFailureWithoutHostAbortLogsDirectCleanup(t *testing.T) {
	var output bytes.Buffer
	logger := zerolog.New(&output).Level(zerolog.WarnLevel)
	ctx := logging.WithContext(context.Background(), logger)
	parentWV := &WebView{ctx: ctx, id: 20}
	popupWV := &WebView{ctx: context.Background(), id: 28, pendingCreate: &pendingBrowserCreate{}}
	popupWV.markNativePopupCandidate(parentWV)
	popupWV.SetBrowsingContextHostDecision(dto.HostDecision{
		Kind: dto.HostDecisionCreateNativePopup, RequiresNativeOpener: true,
	})
	parentWV.SetCallbacks(&port.WebViewCallbacks{OnCreate: func(port.PopupRequest) port.WebView { return popupWV }})

	blocked := (&handlerSet{wv: parentWV}).OnBeforePopup(
		nil, nil, 94, "https://example.com/opener-required", "required",
		purecef.WindowOpenDispositionWodNewPopup, 1, nil, nil, nil, nil, nil, nil,
	)

	require.True(t, blocked)
	require.True(t, popupWV.IsDestroyed())
	var record map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &record))
	assert.Equal(t, false, record["host_abort_invoked"])
	assert.Equal(t, "cef: native popup arming failed", record["message"])
}

func TestOnBeforePopup_AuthNativeArmFailureDeniesWithoutFallback(t *testing.T) {
	parentWV := &WebView{ctx: context.Background(), id: 20}
	popupWV := &WebView{ctx: context.Background(), id: 28, pendingCreate: &pendingBrowserCreate{}}
	popupWV.markNativePopupCandidate(parentWV)
	popupWV.SetBrowsingContextHostDecision(dto.HostDecision{
		Kind: dto.HostDecisionCreateNativePopup, ReasonCode: dto.HostDecisionReasonAuthNativePopup,
	})
	popupWV.SetNativePopupHostAbort(func() { popupWV.Destroy() })
	parentWV.SetCallbacks(&port.WebViewCallbacks{OnCreate: func(port.PopupRequest) port.WebView { return popupWV }})

	blocked := (&handlerSet{wv: parentWV}).OnBeforePopup(
		nil, nil, 94, "https://accounts.example.test/oauth", "oauth",
		purecef.WindowOpenDispositionWodNewPopup, 1, nil, nil, nil, nil, nil, nil,
	)

	require.True(t, blocked)
	require.False(t, popupWV.nativePopupFallbackStarted)
	require.True(t, popupWV.IsDestroyed())
}

func TestOnBeforePopup_IsPopupFlagCanSelectNativePopup(t *testing.T) {
	parentWV := &WebView{ctx: context.Background(), id: 22}
	popupWV := &WebView{ctx: context.Background(), id: 30, pendingCreate: &pendingBrowserCreate{}}
	popupWV.markNativePopupCandidate(parentWV)
	abortCalls := 0
	popupWV.SetNativePopupHostAbort(func() { abortCalls++ })
	parentWV.SetCallbacks(&port.WebViewCallbacks{OnCreate: func(req port.PopupRequest) port.WebView {
		require.Equal(t, dto.PopupFeaturesSpecified, req.PopupFeatures.State)
		require.True(t, req.PopupFeatures.IsPopup)
		require.True(t, req.PopupFeatures.IsPopupSet)
		popupWV.SetBrowsingContextHostDecision(dto.HostDecision{Kind: dto.HostDecisionCreateNativePopup})
		return popupWV
	}})

	blocked := (&handlerSet{wv: parentWV}).OnBeforePopup(
		nil, nil, 96, "https://example.com/chrome-restricted", "restricted",
		purecef.WindowOpenDispositionWodNewPopup, 1,
		&purecef.PopupFeatures{Ispopup: 1}, nil, nil, nil, nil, nil,
	)

	require.True(t, blocked)
	require.Equal(t, 1, abortCalls)
}

func TestOnBeforePopup_ReuseNamedPaneBlocksNativePopup(t *testing.T) {
	parentWV := &WebView{ctx: context.Background(), id: 23}
	popupWV := &WebView{ctx: context.Background(), id: 31, pendingCreate: &pendingBrowserCreate{}}
	popupWV.markNativePopupCandidate(parentWV)
	popupWV.SetBrowsingContextHostDecision(dto.HostDecision{Kind: dto.HostDecisionReuseNamedPane})
	parentWV.SetCallbacks(&port.WebViewCallbacks{OnCreate: func(port.PopupRequest) port.WebView { return popupWV }})

	blocked := (&handlerSet{wv: parentWV}).OnBeforePopup(
		nil, nil, 97, "https://example.com/named", "shared",
		purecef.WindowOpenDispositionWodNewPopup, 1, nil, nil, nil, nil, nil, nil,
	)

	require.True(t, blocked)
	require.True(t, popupWV.nativePopupFallbackStarted)
}

func TestOnBeforePopup_HostedDecisionDiscardsUnpreparedNativeCandidate(t *testing.T) {
	parentWV := &WebView{ctx: context.Background(), id: 24}
	popupWV := &WebView{ctx: context.Background(), id: 32}
	popupWV.markNativePopupCandidate(parentWV)
	popupWV.SetBrowsingContextHostDecision(dto.HostDecision{Kind: dto.HostDecisionCreateBrowserWindow})
	parentWV.SetCallbacks(&port.WebViewCallbacks{OnCreate: func(port.PopupRequest) port.WebView { return popupWV }})

	blocked := (&handlerSet{wv: parentWV}).OnBeforePopup(
		nil, nil, 98, "https://example.com/window", "window",
		purecef.WindowOpenDispositionWodNewPopup, 1, nil, nil, nil, nil, nil, nil,
	)

	require.True(t, blocked)
	require.False(t, popupWV.isNativePopupCandidate())
	require.False(t, popupWV.nativePopupFallbackStarted)
}

func TestOnBeforePopup_HostedDecisionPreservesAlreadyPreparedFallbackOpener(t *testing.T) {
	parentWV := &WebView{ctx: context.Background(), id: 25}
	parentWV.updateURI("https://example.com/opener")
	popupWV := &WebView{ctx: context.Background(), id: 33, pendingCreate: &pendingBrowserCreate{}}
	popupWV.markNativePopupCandidate(parentWV)
	require.True(t, popupWV.preparePopupShellDirectBrowserCreation())
	popupWV.SetBrowsingContextHostDecision(dto.HostDecision{Kind: dto.HostDecisionCreateBrowserWindow})
	parentWV.SetCallbacks(&port.WebViewCallbacks{OnCreate: func(port.PopupRequest) port.WebView { return popupWV }})

	blocked := (&handlerSet{wv: parentWV}).OnBeforePopup(
		nil, nil, 99, "https://example.com/window", "window",
		purecef.WindowOpenDispositionWodNewPopup, 1, nil, nil, nil, nil, nil, nil,
	)

	require.True(t, blocked)
	require.True(t, popupWV.nativePopupFallbackStarted)
	require.Same(t, parentWV, popupWV.popupOpenerBridgeParent)
	require.Equal(t, "https://example.com/opener", popupWV.popupOpenerBridgeParentURI)
}

func TestOnBeforePopup_DenyCleansUp(t *testing.T) {
	parentWV := &WebView{ctx: context.Background(), id: 24}
	popupWV := &WebView{ctx: context.Background(), id: 32}
	popupWV.SetBrowsingContextHostDecision(dto.HostDecision{Kind: dto.HostDecisionDeny})
	parentWV.SetCallbacks(&port.WebViewCallbacks{OnCreate: func(port.PopupRequest) port.WebView { return popupWV }})

	blocked := (&handlerSet{wv: parentWV}).OnBeforePopup(
		nil, nil, 98, "https://example.com/denied", "denied",
		purecef.WindowOpenDispositionWodNewPopup, 1, nil, nil, nil, nil, nil, nil,
	)

	require.True(t, blocked)
	require.True(t, popupWV.IsDestroyed())
}

func TestOnBeforePopup_AwaitFeaturesIsImpossibleAndCleansUp(t *testing.T) {
	parentWV := &WebView{ctx: context.Background(), id: 21}
	popupWV := &WebView{ctx: context.Background(), id: 29}
	popupWV.SetBrowsingContextHostDecision(dto.HostDecision{Kind: dto.HostDecisionAwaitPopupFeatures})
	parentWV.SetCallbacks(&port.WebViewCallbacks{OnCreate: func(port.PopupRequest) port.WebView { return popupWV }})

	blocked := (&handlerSet{wv: parentWV}).OnBeforePopup(
		nil, nil, 95, "https://example.com/impossible", "impossible",
		purecef.WindowOpenDispositionWodNewPopup, 1, nil, nil, nil, nil, nil, nil,
	)

	require.True(t, blocked)
	require.True(t, popupWV.IsDestroyed())
}

func TestOnBeforePopup_TimesOutGTKDispatchAndBlocksPopup(t *testing.T) {
	delayed := make(chan func(), 1)
	parentWV := &WebView{
		ctx:            context.Background(),
		id:             16,
		gtkSyncTimeout: 5 * time.Millisecond,
		gtkSyncIsOwner: func() bool { return false },
		gtkSyncDispatch: func(fn func()) {
			delayed <- fn
		},
	}
	var createCalls atomic.Int32
	parentWV.SetCallbacks(&port.WebViewCallbacks{
		OnCreate: func(_ port.PopupRequest) port.WebView {
			createCalls.Add(1)
			return &WebView{ctx: context.Background(), id: 24}
		},
	})

	h := &handlerSet{wv: parentWV}
	blocked := h.OnBeforePopup(nil, nil, 90, "https://example.com/slow-popup", "slow-popup", 0, 1, nil, nil, nil, nil, nil, nil)

	require.True(t, blocked)
	require.Zero(t, createCalls.Load())

	select {
	case fn := <-delayed:
		fn()
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delayed popup dispatch")
	}
	require.Zero(t, createCalls.Load())
}

func TestOnBeforePopup_PrimesPopupNavigationWhenCEFPopupBlocksNativeCreation(t *testing.T) {
	parentWV := &WebView{ctx: context.Background(), id: 17}
	popupWV := &WebView{ctx: context.Background(), id: 23}
	parentWV.SetCallbacks(&port.WebViewCallbacks{
		OnCreate: func(req port.PopupRequest) port.WebView {
			require.Equal(t, "https://example.com/popup", req.TargetURI)
			require.Equal(t, "auth-popup", req.FrameName)
			require.True(t, req.IsUserGesture)
			popupWV.PrimePopupNavigation(req.TargetURI)
			return popupWV
		},
	})

	h := &handlerSet{wv: parentWV}
	blocked := h.OnBeforePopup(nil, nil, 91, "https://example.com/popup", "auth-popup", 0, 1, nil, nil, nil, nil, nil, nil)

	require.True(t, blocked)
	require.Equal(t, "https://example.com/popup", popupWV.pendingNavigationURI())
}

func TestOnBeforePopup_PrimesPopupShellWhenNativePopupCannotBeArmed(t *testing.T) {
	parentWV := &WebView{ctx: context.Background(), id: 31}
	popupWV := &WebView{ctx: context.Background(), id: 32}
	popupWV.markNativePopupCandidate(parentWV)
	parentWV.SetCallbacks(&port.WebViewCallbacks{
		OnCreate: func(req port.PopupRequest) port.WebView {
			require.Equal(t, "https://example.com/login", req.TargetURI)
			popupWV.PrimePopupNavigation(req.TargetURI)
			return popupWV
		},
	})

	h := &handlerSet{wv: parentWV}
	blocked := h.OnBeforePopup(nil, nil, 77, "https://example.com/login", "Google login", 0, 1, nil, nil, nil, nil, nil, nil)

	require.True(t, blocked)
	require.False(t, popupWV.isNativePopupCandidate())
	require.Equal(t, "https://example.com/login", popupWV.pendingNavigationURI())
}

func TestOnBeforePopup_PaneDecisionBlocksNativePopupAndKeepsPanePath(t *testing.T) {
	parentWV := &WebView{ctx: context.Background(), id: 41}
	popupWV := &WebView{ctx: context.Background(), id: 42}
	popupWV.SetBrowsingContextHostDecision(dto.HostDecision{Kind: dto.HostDecisionCreatePane})
	parentWV.SetCallbacks(&port.WebViewCallbacks{
		OnCreate: func(req port.PopupRequest) port.WebView {
			popupWV.PreparePaneHostedBrowsingContext()
			popupWV.PrimePopupNavigation(req.TargetURI)
			return popupWV
		},
	})

	h := &handlerSet{wv: parentWV}
	blocked := h.OnBeforePopup(nil, nil, 81, "https://example.com/pane", "_blank", 0, 1, nil, nil, nil, nil, nil, nil)

	require.True(t, blocked)
	require.Equal(t, "https://example.com/pane", popupWV.pendingNavigationURI())
}

func TestOnBeforePopup_NativeDecisionAbortsHostWhenArmingFails(t *testing.T) {
	parentWV := &WebView{ctx: context.Background(), id: 51}
	popupWV := &WebView{ctx: context.Background(), id: 52}
	popupWV.markNativePopupCandidate(parentWV)
	popupWV.SetBrowsingContextHostDecision(dto.HostDecision{Kind: dto.HostDecisionCreateNativePopup})
	aborted := false
	popupWV.SetNativePopupHostAbort(func() { aborted = true })
	parentWV.SetCallbacks(&port.WebViewCallbacks{
		OnCreate: func(req port.PopupRequest) port.WebView {
			popupWV.PrimePopupNavigation(req.TargetURI)
			return popupWV
		},
	})

	h := &handlerSet{wv: parentWV}
	blocked := h.OnBeforePopup(nil, nil, 82, "https://accounts.google.com/o/oauth2/v2/auth", "oauth", 0, 1, nil, nil, nil, nil, nil, nil)

	require.True(t, blocked)
	require.True(t, aborted)
}
