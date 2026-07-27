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
	if !pm.adoptPopupInBrowserWindow(
		ctx, hooks, parentPaneID, parentWebViewID, popupWV, req, decision, ready,
		dto.BrowsingContextFailureHostUnavailable, dto.BrowsingContextFailureHostFailed,
		"browser window host returned an empty window ID",
	) {
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
	return pm.adoptPopupInBrowserWindow(
		ctx, hooks, parentPaneID, parentWebViewID, popupWV, req, decision, ready,
		dto.BrowsingContextFailureFallback, dto.BrowsingContextFailureFallback,
		"browser window fallback returned an empty window ID",
	)
}

func (pm *popupManager) adoptPopupInBrowserWindow(
	ctx context.Context,
	hooks popupCoordinatorHooks,
	parentPaneID entity.PaneID,
	parentWebViewID port.WebViewID,
	popupWV port.WebView,
	req port.PopupRequest,
	decision dto.HostDecision,
	ready bool,
	unavailableCode dto.BrowsingContextFailureCode,
	failureCode dto.BrowsingContextFailureCode,
	emptyWindowError string,
) bool {
	log := logging.FromContext(ctx)
	normalized := buildPopupBrowsingContextRequest(req)
	normalized.SourceHost = pm.resolveSourceHost(parentPaneID)
	if popupWV == nil || pm.onOpenBrowserWindow == nil {
		logBrowsingContextFailure(*log, normalized, decision, unavailableCode, nil)
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
			err = fmt.Errorf("%s", emptyWindowError)
		}
		logBrowsingContextFailure(*log, normalized, decision, failureCode, err)
		return false
	}
	if !req.NoJavaScriptAccess {
		pm.storeReusableNamedPopupWithHost(parentPaneID, req.FrameName, paneID, popupWV, result.WindowID)
	}
	return true
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
	pm.setBrowsingContextDecision(popupWV, decision)
	if !pm.adoptPopupInNativePopup(
		ctx, hooks, parentPaneID, parentID, parentURIAtOpen, popupWV, req, decision,
		dto.BrowsingContextFailureNativeArm,
	) {
		popupWV.Destroy()
		return nil
	}
	return popupWV
}

// openExistingPopupInNativePopup attempts to transfer an already-created
// WebView. On failure ownership remains with the caller.
func (pm *popupManager) openExistingPopupInNativePopup(
	ctx context.Context,
	hooks popupCoordinatorHooks,
	parentPaneID entity.PaneID,
	parentWebViewID port.WebViewID,
	parentURIAtOpen string,
	popupWV port.WebView,
	req port.PopupRequest,
	decision dto.HostDecision,
) bool {
	return pm.adoptPopupInNativePopup(
		ctx, hooks, parentPaneID, parentWebViewID, parentURIAtOpen, popupWV, req, decision,
		dto.BrowsingContextFailureHostFailed,
	)
}

func (pm *popupManager) adoptPopupInNativePopup(
	ctx context.Context,
	hooks popupCoordinatorHooks,
	parentPaneID entity.PaneID,
	parentWebViewID port.WebViewID,
	parentURIAtOpen string,
	popupWV port.WebView,
	req port.PopupRequest,
	decision dto.HostDecision,
	failureCode dto.BrowsingContextFailureCode,
) bool {
	log := logging.FromContext(ctx)
	normalized := buildPopupBrowsingContextRequest(req)
	normalized.SourceHost = pm.resolveSourceHost(parentPaneID)
	if popupWV == nil || pm.onOpenNativePopup == nil {
		logBrowsingContextFailure(*log, normalized, decision, dto.BrowsingContextFailureHostUnavailable, nil)
		return false
	}
	cfg := pm.currentPopupConfig()
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
				abortCtx, hooks, parentPaneID, parentWebViewID, abortedWV, req, decision, true,
			)
		})
		return abortResult
	}
	if err := pm.onOpenNativePopup(ctx, NativePopupInput{
		ParentPaneID: parentPaneID, ParentWebViewID: parentWebViewID, ParentURIAtOpen: parentURIAtOpen,
		PopupWebView: popupWV, TargetURI: req.TargetURI, Request: req,
		ObserveOAuthAutoClose:      cfg != nil && cfg.OAuthAutoClose && IsOAuthURL(req.TargetURI),
		AllowBrowserWindowFallback: allowFallback, OnNativeHostAbort: onAbort,
	}); err != nil {
		logBrowsingContextFailure(*log, normalized, decision, failureCode, err)
		return false
	}
	if lifecycle, ok := popupWV.(port.PopupLifecycleCapable); ok {
		lifecycle.PrimePopupNavigation(req.TargetURI)
	}
	return true
}
