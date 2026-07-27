package content

import (
	"context"
	"time"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
)

func (pm *popupManager) awaitPopupFeatures(
	ctx context.Context,
	hooks popupCoordinatorHooks,
	parentPaneID entity.PaneID,
	parentWebViewID port.WebViewID,
	parentWebView port.WebView,
	parentURIAtOpen string,
	req port.PopupRequest,
	decision dto.HostDecision,
) port.WebView {
	log := logging.FromContext(ctx)
	normalized := buildPopupBrowsingContextRequest(req)
	normalized.SourceHost = pm.resolveSourceHost(parentPaneID)
	popupWV, err := pm.createPopupWebView(ctx, parentWebViewID, req.TargetURI, false)
	if err != nil {
		logBrowsingContextFailure(*log, normalized, decision, dto.BrowsingContextFailureHostFailed, err)
		return nil
	}
	resolver, resolverOK := popupWV.(port.PopupFeatureResolver)
	lifecycle, lifecycleOK := popupWV.(port.PopupLifecycleCapable)
	if !resolverOK || !lifecycleOK || pm.onStagePopup == nil {
		logBrowsingContextFailure(*log, normalized, decision, dto.BrowsingContextFailureHostUnavailable, nil)
		popupWV.Destroy()
		return nil
	}
	staging, err := pm.onStagePopup(ctx, StagePopupInput{PopupWebView: popupWV})
	if err != nil || staging == nil {
		logBrowsingContextFailure(*log, normalized, decision, dto.BrowsingContextFailureHostFailed, err)
		popupWV.Destroy()
		return nil
	}
	pm.setBrowsingContextDecision(popupWV, decision)
	paneID, _ := pm.createPopupPane(popupWV.ID(), parentPaneID, req.TargetURI)
	ownerWindowID := ""
	if pm.windowIDForPane != nil {
		ownerWindowID, _ = pm.windowIDForPane(parentPaneID)
	}
	pending := &deferredPendingPopup{
		PendingPopup: &PendingPopup{
			PaneID: paneID, WebView: popupWV, ParentPaneID: parentPaneID,
			ParentWebViewID: parentWebViewID, TargetURI: req.TargetURI, FrameName: req.FrameName,
			IsUserGesture: req.IsUserGesture, PopupType: DetectPopupType(req.FrameName), CreatedAt: time.Now(),
		},
		Request: req, Decision: decision, StagingHost: staging, OwnerWindowID: ownerWindowID,
		ParentWebView: parentWebView,
	}
	pm.storeDeferredPopup(popupWV.ID(), pending)
	callbackCtx := logging.WithContext(context.Background(), *log)
	lifecycle.SetOnReadyToShow(func() {
		pm.resolvePendingPopupFeatures(callbackCtx, hooks, popupWV.ID(), parentURIAtOpen, resolver)
	})
	lifecycle.SetOnClose(func() {
		pm.failPendingPopup(callbackCtx, popupWV.ID(), dto.BrowsingContextFailureHostFailed, nil)
	})
	return popupWV
}

func (pm *popupManager) cleanupDeferredPending(pending *deferredPendingPopup) {
	if pending == nil {
		return
	}
	pending.cleanupOnce.Do(func() {
		if lifecycle, ok := pending.WebView.(port.PopupLifecycleCapable); ok {
			lifecycle.SetOnReadyToShow(nil)
			lifecycle.SetOnClose(nil)
		}
		if pending.StagingHost != nil {
			pending.StagingHost.Destroy()
		}
		if pending.WebView != nil {
			pending.WebView.Destroy()
		}
	})
}

func (pm *popupManager) cancelDeferredPopupsMatching(match func(*deferredPendingPopup) bool) {
	if pm == nil || match == nil {
		return
	}
	pm.mu.Lock()
	canceled := make([]*deferredPendingPopup, 0)
	for popupID, pending := range pm.deferredPopups {
		if pending == nil || !match(pending) {
			continue
		}
		delete(pm.deferredPopups, popupID)
		delete(pm.pendingPopups, popupID)
		canceled = append(canceled, pending)
	}
	pm.mu.Unlock()
	for _, pending := range canceled {
		pm.cleanupDeferredPending(pending)
	}
}

func (pm *popupManager) cancelDeferredPopupsForParent(parentPaneID entity.PaneID) {
	pm.cancelDeferredPopupsMatching(func(pending *deferredPendingPopup) bool {
		return pending.ParentPaneID == parentPaneID
	})
}

func (pm *popupManager) cancelDeferredPopupsForWindow(windowID string) {
	if pm == nil || windowID == "" {
		return
	}
	pm.cancelDeferredPopupsMatching(func(pending *deferredPendingPopup) bool {
		return pending.OwnerWindowID == windowID
	})
}

func (pm *popupManager) cancelAllDeferredPopups() {
	pm.cancelDeferredPopupsMatching(func(*deferredPendingPopup) bool { return true })
}

func (pm *popupManager) failPendingPopup(
	ctx context.Context,
	popupID port.WebViewID,
	code dto.BrowsingContextFailureCode,
	err error,
) {
	pending, ok := pm.takeDeferredPopup(popupID)
	if !ok || pending == nil {
		return
	}
	req := buildPopupBrowsingContextRequest(pending.Request)
	req.SourceHost = pm.resolveSourceHost(pending.ParentPaneID)
	logBrowsingContextFailure(*logging.FromContext(ctx), req, pending.Decision, code, err)
	pm.cleanupDeferredPending(pending)
}

func (pm *popupManager) resolvePendingPopupFeatures(
	ctx context.Context,
	hooks popupCoordinatorHooks,
	popupID port.WebViewID,
	parentURIAtOpen string,
	resolver port.PopupFeatureResolver,
) {
	pending, ok := pm.takeDeferredPopup(popupID)
	if !ok || pending == nil {
		return
	}
	resolved := resolver.ResolvePopupFeatures()
	if resolved.State == dto.PopupFeaturesUnknown {
		req := buildPopupBrowsingContextRequest(pending.Request)
		req.SourceHost = pm.resolveSourceHost(pending.ParentPaneID)
		logBrowsingContextFailure(*logging.FromContext(ctx), req, pending.Decision, dto.BrowsingContextFailureFeatureResolution, nil)
		pm.cleanupDeferredPending(pending)
		return
	}
	if err := pending.StagingHost.Detach(); err != nil {
		req := buildPopupBrowsingContextRequest(pending.Request)
		req.SourceHost = pm.resolveSourceHost(pending.ParentPaneID)
		logBrowsingContextFailure(*logging.FromContext(ctx), req, pending.Decision, dto.BrowsingContextFailureStagingDetach, err)
		pm.cleanupDeferredPending(pending)
		return
	}

	request := pending.Request
	request.PopupFeatures = resolved
	normalized := buildPopupBrowsingContextRequest(request)
	normalized.SourceHost = pm.resolveSourceHost(pending.ParentPaneID)
	namedContextExists := false
	if !request.NoJavaScriptAccess {
		_, namedContextExists = pm.lookupReusableNamedPopup(pending.ParentPaneID, request.FrameName, hooks)
	}
	decision := pm.policy.Decide(normalized, namedContextExists)
	logBrowsingContextDecision(*logging.FromContext(ctx), normalized, decision)
	pm.setBrowsingContextDecision(pending.WebView, decision)

	transferred := false
	switch decision.Kind {
	case dto.HostDecisionNavigateSource:
		if pending.ParentWebView == nil {
			logBrowsingContextFailure(*logging.FromContext(ctx), normalized, decision, dto.BrowsingContextFailureHostUnavailable, nil)
			break
		}
		if err := pending.ParentWebView.LoadURI(ctx, request.TargetURI); err != nil {
			logBrowsingContextFailure(*logging.FromContext(ctx), normalized, decision, dto.BrowsingContextFailureHostFailed, err)
		}
	case dto.HostDecisionCreateBrowserWindow:
		transferred = pm.openExistingPopupInBrowserWindow(
			ctx, hooks, pending.ParentPaneID, pending.ParentWebViewID,
			pending.WebView, request, decision, true,
		)
	case dto.HostDecisionCreateNativePopup:
		transferred = pm.openExistingPopupInNativePopup(
			ctx, hooks, pending.ParentPaneID, pending.ParentWebViewID,
			parentURIAtOpen, pending.WebView, request, decision,
		)
	case dto.HostDecisionReuseNamedPane:
		// Reuse transfers no ownership: the existing context is navigated while
		// the newly staged WebView remains ours and must be discarded below.
		_, _ = pm.reuseNamedPopup(ctx, hooks, pending.ParentPaneID, request.FrameName, request.TargetURI)
	default:
		logBrowsingContextFailure(*logging.FromContext(ctx), normalized, decision, dto.BrowsingContextFailureFeatureResolution, nil)
	}
	if !transferred {
		pm.cleanupDeferredPending(pending)
	}
}
