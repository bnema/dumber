package content

import (
	"context"
	"fmt"

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

func (*popupManager) readyOnCreate(dto.BrowserEngineKind) bool {
	// Deferred WebKit requests are staged and routed by a later phase. Every
	// immediate route must be visible without depending on another signal.
	return true
}
