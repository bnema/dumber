package content

import (
	"context"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
)

// openBrowserWindow transfers a related WebView to a complete browser window.
// The popup manager retains ownership until the host callback succeeds.
func (pm *popupManager) openBrowserWindow(
	ctx context.Context,
	hooks popupCoordinatorHooks,
	parentPaneID entity.PaneID,
	parentWebViewID port.WebViewID,
	req port.PopupRequest,
	decision dto.HostDecision,
	ready bool,
) port.WebView {
	log := logging.FromContext(ctx)
	normalized := buildPopupBrowsingContextRequest(req)
	normalized.SourceHost = pm.resolveSourceHost(parentPaneID)
	if pm.onOpenBrowserWindow == nil {
		logBrowsingContextFailure(*log, normalized, decision, dto.BrowsingContextFailureHostUnavailable, nil)
		return nil
	}
	popupWV, err := pm.createPopupWebView(ctx, parentWebViewID, req.TargetURI, true)
	if err != nil {
		logBrowsingContextFailure(*log, normalized, decision, dto.BrowsingContextFailureHostFailed, err)
		return nil
	}
	pm.setBrowsingContextDecision(popupWV, decision)
	paneID, popupPane := pm.createPopupPane(popupWV.ID(), parentPaneID, req.TargetURI)
	if hooks.setupWebViewCallbacks != nil {
		hooks.setupWebViewCallbacks(ctx, paneID, popupWV)
	}
	result, err := pm.onOpenBrowserWindow(ctx, BrowserWindowInput{
		ParentPaneID: parentPaneID, ParentWebViewID: parentWebViewID, PopupPane: popupPane,
		PopupWebView: popupWV, TargetURI: req.TargetURI, Request: req, Ready: ready,
	})
	if err != nil || result.WindowID == "" {
		if err == nil {
			err = fmt.Errorf("browser window host returned an empty window ID")
		}
		logBrowsingContextFailure(*log, normalized, decision, dto.BrowsingContextFailureHostFailed, err)
		popupWV.Destroy()
		return nil
	}
	if !req.NoJavaScriptAccess {
		pm.storeReusableNamedPopupWithHost(parentPaneID, req.FrameName, paneID, popupWV, result.WindowID)
	}
	if lifecycle, ok := popupWV.(port.PopupLifecycleCapable); ok {
		lifecycle.PrimePopupNavigation(req.TargetURI)
	}
	return popupWV
}

func (pm *popupManager) navigatePopupSource(
	ctx context.Context,
	parentID port.WebViewID,
	parentWV port.WebView,
	req port.PopupRequest,
	request dto.NewBrowsingContextRequest,
	decision dto.HostDecision,
) {
	log := logging.FromContext(ctx)
	popupWV, err := pm.createPopupWebView(ctx, parentID, req.TargetURI, false)
	if err != nil {
		logBrowsingContextFailure(*log, request, decision, dto.BrowsingContextFailureHostFailed, err)
		return
	}
	pm.setBrowsingContextDecision(popupWV, decision)
	popupWV.Destroy()
	if err := parentWV.LoadURI(ctx, req.TargetURI); err != nil {
		logBrowsingContextFailure(*log, request, decision, dto.BrowsingContextFailureHostFailed, err)
	}
}

func (*popupManager) navigateSource(ctx context.Context, parentWV port.WebView, uri string) bool {
	if err := parentWV.LoadURI(ctx, uri); err != nil {
		logging.FromContext(ctx).Error().Err(err).
			Str("uri", logging.TruncateURL(uri, logURLMaxLen)).
			Msg("failed to load URI in source floating pane")
		return false
	}
	return true
}

func popupSupportsOpenerBridge(wv port.WebView) bool {
	_, ok := wv.(port.PopupOpenerCapable)
	return ok
}

func (pm *popupManager) openNativePopup(
	ctx context.Context,
	hooks popupCoordinatorHooks,
	parentPaneID entity.PaneID,
	parentID port.WebViewID,
	parentURIAtOpen string,
	req port.PopupRequest,
	decision dto.HostDecision,
) port.WebView {
	log := logging.FromContext(ctx)
	normalized := buildPopupBrowsingContextRequest(req)
	normalized.SourceHost = pm.resolveSourceHost(parentPaneID)
	if pm.onOpenNativePopup == nil {
		logBrowsingContextFailure(*log, normalized, decision, dto.BrowsingContextFailureHostUnavailable, nil)
		return nil
	}
	popupWV, err := pm.createPopupWebView(ctx, parentID, req.TargetURI, false)
	if err != nil {
		logBrowsingContextFailure(*log, normalized, decision, dto.BrowsingContextFailureHostFailed, err)
		return nil
	}
	cfg := pm.currentPopupConfig()
	pm.setBrowsingContextDecision(popupWV, decision)
	allowFallback := !normalized.AuthIntent && !decision.RequiresNativeOpener &&
		(req.NoJavaScriptAccess || popupSupportsOpenerBridge(popupWV))
	var abortOnce sync.Once
	abortResult := false
	onAbort := func(abortCtx context.Context, abortedWV port.WebView) bool {
		abortOnce.Do(func() {
			if !allowFallback {
				logBrowsingContextFailure(*log, normalized, decision, dto.BrowsingContextFailureNativeArm, nil)
				return
			}
			abortResult = pm.openExistingPopupInBrowserWindow(
				abortCtx, hooks, parentPaneID, parentID, abortedWV, req, decision, true,
			)
		})
		return abortResult
	}
	if err := pm.onOpenNativePopup(ctx, NativePopupInput{
		ParentPaneID:               parentPaneID,
		ParentWebViewID:            parentID,
		ParentURIAtOpen:            parentURIAtOpen,
		PopupWebView:               popupWV,
		TargetURI:                  req.TargetURI,
		Request:                    req,
		ObserveOAuthAutoClose:      cfg != nil && cfg.OAuthAutoClose && IsOAuthURL(req.TargetURI),
		AllowBrowserWindowFallback: allowFallback,
		OnNativeHostAbort:          onAbort,
	}); err != nil {
		logBrowsingContextFailure(*log, normalized, decision, dto.BrowsingContextFailureNativeArm, err)
		popupWV.Destroy()
		return nil
	}
	if lifecycle, ok := popupWV.(port.PopupLifecycleCapable); ok {
		lifecycle.PrimePopupNavigation(req.TargetURI)
	}
	return popupWV
}

// openExistingPopupInBrowserWindow attempts to transfer an already-created
// WebView. On failure ownership remains with the caller.
func (pm *popupManager) openExistingPopupInBrowserWindow(
	ctx context.Context,
	hooks popupCoordinatorHooks,
	parentPaneID entity.PaneID,
	parentWebViewID port.WebViewID,
	popupWV port.WebView,
	req port.PopupRequest,
	decision dto.HostDecision,
	ready bool,
) bool {
	log := logging.FromContext(ctx)
	normalized := buildPopupBrowsingContextRequest(req)
	normalized.SourceHost = pm.resolveSourceHost(parentPaneID)
	if popupWV == nil || pm.onOpenBrowserWindow == nil {
		logBrowsingContextFailure(*log, normalized, decision, dto.BrowsingContextFailureFallback, nil)
		return false
	}

	paneID, popupPane := pm.createPopupPane(popupWV.ID(), parentPaneID, req.TargetURI)
	if hooks.setupWebViewCallbacks != nil {
		hooks.setupWebViewCallbacks(ctx, paneID, popupWV)
	}
	result, err := pm.onOpenBrowserWindow(ctx, BrowserWindowInput{
		ParentPaneID: parentPaneID, ParentWebViewID: parentWebViewID, PopupPane: popupPane,
		PopupWebView: popupWV, TargetURI: req.TargetURI, Request: req, Ready: ready,
	})
	if err != nil || result.WindowID == "" {
		if err == nil {
			err = fmt.Errorf("browser window fallback returned an empty window ID")
		}
		logBrowsingContextFailure(*log, normalized, decision, dto.BrowsingContextFailureFallback, err)
		return false
	}
	if !req.NoJavaScriptAccess {
		pm.storeReusableNamedPopupWithHost(parentPaneID, req.FrameName, paneID, popupWV, result.WindowID)
	}
	return true
}

func (*popupManager) readyOnCreate(dto.BrowserEngineKind) bool {
	// Deferred WebKit requests are staged and routed by a later phase. Every
	// immediate route must be visible without depending on another signal.
	return true
}
