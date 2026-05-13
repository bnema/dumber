package content

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
)

// popupManager owns popup-specific coordinator state. It stays in the UI
// adapter layer so popup pane bookkeeping and workspace orchestration do not
// leak into the application/usecase or domain layers.
type popupManager struct {
	factory           port.WebViewFactory
	popupConfig       *entity.BrowsingContextConfig
	onInsertPopup     func(ctx context.Context, input InsertPopupInput) error
	onOpenNativePopup func(ctx context.Context, input NativePopupInput) error
	onClosePane       func(ctx context.Context, paneID entity.PaneID) error
	generatePaneID    func() string
	windowIDForPane   func(entity.PaneID) (string, bool)
	policy            browsingContextPolicy
	namedContexts     *namedBrowsingContextRegistry
	pendingPopups     map[port.WebViewID]*PendingPopup
	popupOAuth        map[port.WebViewID]*popupOAuthState
	popupRefresh      map[entity.PaneID]*time.Timer
	mu                sync.RWMutex
}

type popupOAuthState struct {
	ParentPaneID    entity.PaneID
	ParentURIAtOpen string
	CallbackURI     string
	Success         bool
	Error           bool
	Seen            bool
}

type popupCreateContext struct {
	ParentPaneID    entity.PaneID
	ParentWebViewID port.WebViewID
	ParentURIAtOpen string
	PopupID         port.WebViewID
	PopupWebView    port.WebView
	PopupPane       *entity.Pane
	PopupPaneID     entity.PaneID
	PopupType       PopupType
	Behavior        entity.PopupBehavior
	Placement       string
	Request         port.PopupRequest
}

type popupCoordinatorHooks struct {
	setupWebViewCallbacks func(context.Context, entity.PaneID, port.WebView)
	registerPopupWebView  func(entity.PaneID, port.WebView)
	setWebView            func(entity.PaneID, port.WebView)
	getWebView            func(entity.PaneID) port.WebView
	deleteWebView         func(entity.PaneID) port.WebView
	releaseWebView        func(context.Context, entity.PaneID)
	findPaneByWebViewID   func(port.WebViewID) (entity.PaneID, bool)
	setupOAuthAutoClose   func(context.Context, entity.PaneID, port.WebViewID, port.WebView)
	handlePopupOAuthClose func(context.Context, port.WebViewID)
}

func newPopupManager() *popupManager {
	pm := &popupManager{}
	pm.ensureInitialized()
	return pm
}

func (pm *popupManager) ensureInitialized() {
	if pm == nil {
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.pendingPopups == nil {
		pm.pendingPopups = make(map[port.WebViewID]*PendingPopup)
	}
	if pm.namedContexts == nil {
		pm.namedContexts = newNamedBrowsingContextRegistry()
	}
	if pm.popupOAuth == nil {
		pm.popupOAuth = make(map[port.WebViewID]*popupOAuthState)
	}
	if pm.popupRefresh == nil {
		pm.popupRefresh = make(map[entity.PaneID]*time.Timer)
	}
}

func (pm *popupManager) setConfig(
	factory port.WebViewFactory,
	popupConfig *entity.BrowsingContextConfig,
	generateID func() string,
) {
	if pm == nil {
		return
	}
	pm.ensureInitialized()
	pm.factory = factory
	pm.popupConfig = popupConfig
	pm.generatePaneID = generateID
}

func (pm *popupManager) setWindowIDResolver(fn func(entity.PaneID) (string, bool)) {
	if pm == nil {
		return
	}
	pm.windowIDForPane = fn
}

func popupTabInsertionConfig(cfg *entity.BrowsingContextConfig) (entity.PopupBehavior, string) {
	behavior := GetBehavior(PopupTypeTab, cfg)
	placement := "right"
	if cfg != nil {
		placement = cfg.Placement
	}
	return behavior, placement
}

func (*popupManager) setBrowsingContextDecision(wv port.WebView, decision port.HostDecision) {
	if carrier, ok := wv.(port.BrowsingContextHostDecisionCapable); ok {
		carrier.SetBrowsingContextHostDecision(decision)
	}
}

func (pm *popupManager) setOnInsertPopup(fn func(ctx context.Context, input InsertPopupInput) error) {
	if pm == nil {
		return
	}
	pm.onInsertPopup = fn
}

func (pm *popupManager) setOnClosePane(fn func(ctx context.Context, paneID entity.PaneID) error) {
	if pm == nil {
		return
	}
	pm.onClosePane = fn
}

func (pm *popupManager) setOnOpenNativePopup(fn func(ctx context.Context, input NativePopupInput) error) {
	if pm == nil {
		return
	}
	pm.onOpenNativePopup = fn
}

func (pm *popupManager) createPopupPane(
	popupID port.WebViewID,
	parentPaneID entity.PaneID,
	targetURI string,
) (entity.PaneID, *entity.Pane) {
	var paneID entity.PaneID
	if pm != nil && pm.generatePaneID != nil {
		paneID = entity.PaneID(pm.generatePaneID())
	} else {
		paneID = entity.PaneID(fmt.Sprintf("popup_%d", popupID))
	}

	popupPane := entity.NewPane(paneID)
	popupPane.WindowType = entity.WindowPopup
	popupPane.IsRelated = true
	popupPane.ParentPaneID = &parentPaneID
	popupPane.URI = targetURI

	return paneID, popupPane
}

func (pm *popupManager) lookupReusableNamedPopup(
	parentPaneID entity.PaneID,
	frameName string,
	hooks popupCoordinatorHooks,
) (port.WebView, bool) {
	if pm == nil || pm.namedContexts == nil {
		return nil, false
	}

	name := reusableBrowsingContextName(frameName)
	if name == "" || pm.windowIDForPane == nil || hooks.getWebView == nil {
		return nil, false
	}

	windowID, ok := pm.windowIDForPane(parentPaneID)
	if !ok || windowID == "" {
		return nil, false
	}

	_, wv, ok := pm.namedContexts.Lookup(windowID, name, hooks.getWebView, pm.windowIDForPane)
	if !ok {
		return nil, false
	}
	return wv, true
}

func (pm *popupManager) storeReusableNamedPopup(parentPaneID entity.PaneID, frameName string, paneID entity.PaneID, wv port.WebView) {
	if pm == nil || pm.namedContexts == nil || wv == nil {
		return
	}
	name := reusableBrowsingContextName(frameName)
	if name == "" || pm.windowIDForPane == nil {
		return
	}
	windowID, ok := pm.windowIDForPane(parentPaneID)
	if !ok || windowID == "" {
		return
	}
	pm.namedContexts.Register(windowID, name, paneID, wv.ID())
}

func (pm *popupManager) updatePendingPopupTarget(popupID port.WebViewID, targetURI string) {
	if pm == nil {
		return
	}
	pm.mu.Lock()
	if pending, ok := pm.pendingPopups[popupID]; ok && pending != nil {
		pending.TargetURI = targetURI
	}
	pm.mu.Unlock()
}

func (pm *popupManager) clearReusableNamedPopupByWebViewID(popupID port.WebViewID) {
	if pm == nil || pm.namedContexts == nil {
		return
	}
	pm.namedContexts.UnregisterByWebViewID(popupID)
}

func (pm *popupManager) clearReusableNamedPopupByPaneID(paneID entity.PaneID) {
	if pm == nil || pm.namedContexts == nil {
		return
	}
	pm.namedContexts.UnregisterByPaneID(paneID)
}

func (pm *popupManager) clearReusableNamedPopupsForWindow(windowID string) {
	if pm == nil || pm.namedContexts == nil {
		return
	}
	pm.namedContexts.UnregisterWindow(windowID)
}

func (pm *popupManager) storePendingPopup(popupID port.WebViewID, pending *PendingPopup) {
	if pm == nil || pending == nil {
		return
	}
	pm.mu.Lock()
	pm.pendingPopups[popupID] = pending
	pm.mu.Unlock()
}

func (pm *popupManager) takePendingPopup(popupID port.WebViewID) (*PendingPopup, bool) {
	if pm == nil {
		return nil, false
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pending, ok := pm.pendingPopups[popupID]
	if ok {
		delete(pm.pendingPopups, popupID)
	}
	return pending, ok
}

func (pm *popupManager) trackOAuthPopup(popupID port.WebViewID, parentPaneID entity.PaneID, parentURIAtOpen string) {
	if pm == nil {
		return
	}
	pm.mu.Lock()
	pm.popupOAuth[popupID] = &popupOAuthState{
		ParentPaneID:    parentPaneID,
		ParentURIAtOpen: strings.TrimSpace(parentURIAtOpen),
	}
	pm.mu.Unlock()
}

func (pm *popupManager) capturePopupOAuthState(popupID port.WebViewID, uri string) {
	if pm == nil {
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	state, ok := pm.popupOAuth[popupID]
	if !ok {
		return
	}
	state.Seen = true
	state.CallbackURI = uri
	state.Success = IsOAuthSuccess(uri)
	state.Error = IsOAuthError(uri)
}

func (pm *popupManager) capturePopupOAuthMessage(popupID port.WebViewID) {
	if pm == nil {
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	state, ok := pm.popupOAuth[popupID]
	if !ok {
		return
	}
	state.Seen = true
	if state.CallbackURI == "" {
		state.CallbackURI = "postmessage://oauth-complete"
	}
	if !state.Error {
		state.Success = true
	}
}

func (pm *popupManager) takePopupOAuthState(popupID port.WebViewID) (*popupOAuthState, bool) {
	if pm == nil {
		return nil, false
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	state, ok := pm.popupOAuth[popupID]
	if ok {
		delete(pm.popupOAuth, popupID)
	}
	return state, ok
}

func (pm *popupManager) schedulePopupRefresh(parentPaneID entity.PaneID, debounce time.Duration, fn func()) {
	if pm == nil || fn == nil {
		return
	}

	var timer *time.Timer
	pm.mu.Lock()
	if existing := pm.popupRefresh[parentPaneID]; existing != nil {
		existing.Stop()
	}
	timer = time.AfterFunc(debounce, func() {
		pm.mu.Lock()
		if pm.popupRefresh[parentPaneID] != timer {
			pm.mu.Unlock()
			return
		}
		delete(pm.popupRefresh, parentPaneID)
		pm.mu.Unlock()
		fn()
	})
	pm.popupRefresh[parentPaneID] = timer
	pm.mu.Unlock()
}

func (pm *popupManager) createPopupWebView(
	ctx context.Context,
	parentID port.WebViewID,
	targetURI string,
	preparePaneHosted bool,
) (port.WebView, error) {
	if pm == nil || pm.factory == nil {
		return nil, fmt.Errorf("no webview factory configured")
	}

	popupWV, err := pm.factory.CreateRelated(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("create related popup webview for %s: %w", logging.TruncateURL(targetURI, logURLMaxLen), err)
	}
	if popupWV == nil {
		return nil, fmt.Errorf("related popup webview factory returned nil for %s", logging.TruncateURL(targetURI, logURLMaxLen))
	}
	if preparePaneHosted {
		if paneHosted, ok := popupWV.(port.PaneHostedBrowsingContextCapable); ok {
			paneHosted.PreparePaneHostedBrowsingContext()
		}
	}
	return popupWV, nil
}

func (pm *popupManager) reuseNamedPopup(
	ctx context.Context,
	hooks popupCoordinatorHooks,
	parentPaneID entity.PaneID,
	frameName string,
	targetURI string,
) (port.WebView, bool) {
	log := logging.FromContext(ctx)

	existingWV, ok := pm.lookupReusableNamedPopup(parentPaneID, frameName, hooks)
	if !ok {
		return nil, false
	}

	pm.updatePendingPopupTarget(existingWV.ID(), targetURI)
	if err := existingWV.LoadURI(ctx, targetURI); err != nil {
		log.Warn().Err(err).
			Str("target_uri", logging.TruncateURL(targetURI, logURLMaxLen)).
			Msg("failed to load target URI in reused popup")
	}
	log.Info().
		Str("parent_pane", string(parentPaneID)).
		Str("frame_name", frameName).
		Str("target_uri", logging.TruncateURL(targetURI, logURLMaxLen)).
		Msg("reused named popup")
	return existingWV, true
}

func (pm *popupManager) popupParentURIAtOpen(
	parentPaneID entity.PaneID,
	parentWV port.WebView,
	hooks popupCoordinatorHooks,
	targetURI string,
) string {
	if pm.popupConfig == nil || !pm.popupConfig.OAuthAutoClose || !IsOAuthURL(targetURI) {
		return ""
	}
	parentURIAtOpen := parentWV.URI()
	if parentURIAtOpen == "" && hooks.getWebView != nil {
		if parent := hooks.getWebView(parentPaneID); parent != nil {
			parentURIAtOpen = parent.URI()
		}
	}
	return parentURIAtOpen
}

func (pm *popupManager) openNativePopup(
	ctx context.Context,
	parentPaneID entity.PaneID,
	parentID port.WebViewID,
	parentURIAtOpen string,
	req port.PopupRequest,
	decision port.HostDecision,
) port.WebView {
	log := logging.FromContext(ctx)
	if pm.onOpenNativePopup == nil {
		log.Warn().Msg("native popup host callback not configured")
		return nil
	}
	popupWV, err := pm.createPopupWebView(ctx, parentID, req.TargetURI, false)
	if err != nil {
		log.Error().Err(err).Msg("failed to create webview for native popup")
		return nil
	}
	pm.setBrowsingContextDecision(popupWV, decision)
	if err := pm.onOpenNativePopup(ctx, NativePopupInput{
		ParentPaneID:          parentPaneID,
		ParentWebViewID:       parentID,
		ParentURIAtOpen:       parentURIAtOpen,
		PopupWebView:          popupWV,
		TargetURI:             req.TargetURI,
		Request:               req,
		ObserveOAuthAutoClose: pm.popupConfig != nil && pm.popupConfig.OAuthAutoClose && IsOAuthURL(req.TargetURI),
	}); err != nil {
		log.Error().Err(err).Msg("failed to open native popup host")
		popupWV.Destroy()
		return nil
	}
	if lifecycle, ok := popupWV.(port.PopupLifecycleCapable); ok {
		lifecycle.PrimePopupNavigation(req.TargetURI)
	}
	return popupWV
}

func (pm *popupManager) handlePopupCreate(
	ctx context.Context,
	hooks popupCoordinatorHooks,
	parentPaneID entity.PaneID,
	parentWV port.WebView,
	req port.PopupRequest,
) port.WebView {
	log := logging.FromContext(ctx)

	log.Debug().
		Str("parent_pane", string(parentPaneID)).
		Str("target_uri", logging.TruncateURL(req.TargetURI, logURLMaxLen)).
		Str("frame_name", req.FrameName).
		Bool("user_gesture", req.IsUserGesture).
		Bool("no_javascript_access", req.NoJavaScriptAccess).
		Msg("popup create request")

	if pm.popupConfig != nil && !pm.popupConfig.OpenInNewPane {
		log.Debug().Msg("popups disabled by config, blocking")
		return nil
	}
	if pm.factory == nil {
		log.Warn().Msg("no webview factory, cannot create popup")
		return nil
	}

	parentID := parentWV.ID()
	parentURIAtOpen := pm.popupParentURIAtOpen(parentPaneID, parentWV, hooks, req.TargetURI)

	request := buildPopupBrowsingContextRequest(req)
	namedContextExists := false
	if !req.NoJavaScriptAccess {
		_, namedContextExists = pm.lookupReusableNamedPopup(parentPaneID, req.FrameName, hooks)
	}
	decision := pm.policy.Decide(request, namedContextExists)
	log.Debug().
		Str("decision", string(decision.Kind)).
		Str("reason", decision.Reason).
		Str("context_name", decision.BrowsingContextName).
		Msg("browsing context host decision")

	switch decision.Kind {
	case port.HostDecisionDeny:
		return nil
	case port.HostDecisionReuseNamedPane:
		if req.NoJavaScriptAccess {
			log.Warn().Str("frame_name", req.FrameName).Msg("named browsing context reuse denied for noopener popup")
			return nil
		}
		reused, ok := pm.reuseNamedPopup(ctx, hooks, parentPaneID, req.FrameName, req.TargetURI)
		if !ok {
			log.Warn().Str("frame_name", req.FrameName).Msg("named browsing context reuse requested but target was unavailable")
			return nil
		}
		pm.setBrowsingContextDecision(reused, decision)
		return reused
	case port.HostDecisionCreateNativeWin:
		return pm.openNativePopup(ctx, parentPaneID, parentID, parentURIAtOpen, req, decision)
	case port.HostDecisionCreatePane:
		// Continue below.
		break
	}

	popupWV, err := pm.createPopupWebView(ctx, parentID, req.TargetURI, true)
	if err != nil {
		log.Error().Err(err).Msg("failed to create webview for popup")
		return nil
	}

	pm.setBrowsingContextDecision(popupWV, decision)

	popupType := DetectPopupType(req.FrameName)
	popupID := popupWV.ID()
	behavior := GetBehavior(popupType, pm.popupConfig)
	placement := "right"
	if pm.popupConfig != nil {
		placement = pm.popupConfig.Placement
	}
	paneID, popupPane := pm.createPopupPane(popupID, parentPaneID, req.TargetURI)

	return pm.finishPopupCreate(ctx, hooks, popupCreateContext{
		ParentPaneID:    parentPaneID,
		ParentWebViewID: parentID,
		ParentURIAtOpen: parentURIAtOpen,
		PopupID:         popupID,
		PopupWebView:    popupWV,
		PopupPane:       popupPane,
		PopupPaneID:     paneID,
		PopupType:       popupType,
		Behavior:        behavior,
		Placement:       placement,
		Request:         req,
	})
}

func (pm *popupManager) finishPopupCreate(
	ctx context.Context,
	hooks popupCoordinatorHooks,
	create popupCreateContext,
) port.WebView {
	log := logging.FromContext(ctx)
	hasConfig := pm.popupConfig != nil
	oauthEnabled := hasConfig && pm.popupConfig.OAuthAutoClose
	isOAuth := IsOAuthURL(create.Request.TargetURI)

	log.Debug().
		Bool("has_config", hasConfig).
		Bool("oauth_enabled", oauthEnabled).
		Bool("is_oauth", isOAuth).
		Str("uri", logging.TruncateURL(create.Request.TargetURI, logURLMaxLen)).
		Msg("popup OAuth check")

	if hooks.setupWebViewCallbacks != nil {
		hooks.setupWebViewCallbacks(ctx, create.PopupPaneID, create.PopupWebView)
	}
	if hooks.registerPopupWebView != nil {
		hooks.registerPopupWebView(create.PopupPaneID, create.PopupWebView)
	}

	inserted := false
	defer func() {
		if inserted {
			return
		}
		if hooks.getWebView != nil && hooks.deleteWebView != nil {
			if current := hooks.getWebView(create.PopupPaneID); current == create.PopupWebView {
				hooks.deleteWebView(create.PopupPaneID)
			}
		}
	}()

	if pm.onInsertPopup != nil {
		popupInput := InsertPopupInput{
			ParentPaneID: create.ParentPaneID,
			PopupPane:    create.PopupPane,
			WebView:      create.PopupWebView,
			Behavior:     create.Behavior,
			Placement:    create.Placement,
			PopupType:    create.PopupType,
			TargetURI:    create.Request.TargetURI,
		}
		if err := pm.onInsertPopup(ctx, popupInput); err != nil {
			log.Error().Err(err).Msg("failed to insert popup into workspace")
			create.PopupWebView.Destroy()
			return nil
		}
	}
	inserted = true

	if lifecycle, ok := create.PopupWebView.(port.PopupLifecycleCapable); ok {
		lifecycle.PrimePopupNavigation(create.Request.TargetURI)
	}
	if !create.Request.NoJavaScriptAccess {
		pm.storeReusableNamedPopup(create.ParentPaneID, create.Request.FrameName, create.PopupPaneID, create.PopupWebView)
	}
	if _, hasNativePopupLifecycle := create.PopupWebView.(port.PopupLifecycleCapable); !hasNativePopupLifecycle {
		if closeCapable, ok := create.PopupWebView.(port.OAuthCallbackCapable); ok {
			closeCapable.AddCloseCallback(func() {
				pm.handlePopupClose(context.Background(), hooks, create.PopupID)
			})
		}
	}

	if hasConfig && oauthEnabled && isOAuth {
		create.PopupPane.AutoClose = true
		pm.trackOAuthPopup(create.PopupID, create.ParentPaneID, create.ParentURIAtOpen)
		if hooks.setupOAuthAutoClose != nil {
			hooks.setupOAuthAutoClose(ctx, create.PopupPaneID, create.PopupID, create.PopupWebView)
		}
		log.Debug().Str("pane_id", string(create.PopupPaneID)).Msg("OAuth auto-close enabled for popup")
	}

	pending := &PendingPopup{
		PaneID:          create.PopupPaneID,
		WebView:         create.PopupWebView,
		ParentPaneID:    create.ParentPaneID,
		ParentWebViewID: create.ParentWebViewID,
		TargetURI:       create.Request.TargetURI,
		FrameName:       create.Request.FrameName,
		IsUserGesture:   create.Request.IsUserGesture,
		PopupType:       create.PopupType,
		CreatedAt:       time.Now(),
	}
	pm.storePendingPopup(create.PopupID, pending)

	if lifecycle, ok := create.PopupWebView.(port.PopupLifecycleCapable); ok {
		lifecycle.SetOnReadyToShow(func() {
			pm.handlePopupReadyToShow(context.Background(), create.PopupID)
		})
		lifecycle.SetOnClose(func() {
			pm.handlePopupClose(context.Background(), hooks, create.PopupID)
		})
		log.Info().
			Uint64("popup_id", uint64(create.PopupID)).
			Str("pane_id", string(create.PopupPaneID)).
			Str("popup_type", create.PopupType.String()).
			Str("target_uri", logging.TruncateURL(create.Request.TargetURI, logURLMaxLen)).
			Msg("popup inserted (hidden), awaiting ready-to-show for visibility")
	} else {
		log.Info().
			Uint64("popup_id", uint64(create.PopupID)).
			Str("pane_id", string(create.PopupPaneID)).
			Str("popup_type", create.PopupType.String()).
			Str("target_uri", logging.TruncateURL(create.Request.TargetURI, logURLMaxLen)).
			Msg("popup inserted, immediately ready (no PopupLifecycleCapable)")
		pm.handlePopupReadyToShow(context.Background(), create.PopupID)
	}

	return create.PopupWebView
}

func (pm *popupManager) handlePopupReadyToShow(ctx context.Context, popupID port.WebViewID) {
	log := logging.FromContext(ctx)
	pending, ok := pm.takePendingPopup(popupID)
	if !ok || pending == nil {
		log.Warn().Uint64("popup_id", uint64(popupID)).Msg("ready-to-show for unknown popup")
		return
	}

	log.Debug().
		Uint64("popup_id", uint64(popupID)).
		Str("popup_type", pending.PopupType.String()).
		Msg("popup ready to show - making visible")

	if pending.WebView != nil {
		lifecycle, preloadsNavigation := pending.WebView.(port.PopupLifecycleCapable)
		if preloadsNavigation {
			lifecycle.Show()
		}
		if !preloadsNavigation && pending.TargetURI != "" && !pending.WebView.IsLoading() && pending.WebView.URI() == "" {
			if err := pending.WebView.LoadURI(ctx, pending.TargetURI); err != nil {
				log.Warn().Err(err).
					Str("uri", logging.TruncateURL(pending.TargetURI, logURLMaxLen)).
					Msg("failed to load target URI in popup")
			}
		}
	}

	log.Info().
		Uint64("popup_id", uint64(popupID)).
		Str("target_uri", logging.TruncateURL(pending.TargetURI, logURLMaxLen)).
		Msg("popup now visible")
}

func (pm *popupManager) logPopupCloseSignal(ctx context.Context, hooks popupCoordinatorHooks, popupID port.WebViewID) {
	fields := logging.FromContext(ctx).Debug().Uint64("popup_id", uint64(popupID))
	if hooks.findPaneByWebViewID != nil && hooks.getWebView != nil {
		if paneID, ok := hooks.findPaneByWebViewID(popupID); ok && paneID != "" {
			fields = fields.Str("pane_id", string(paneID))
			if wv := hooks.getWebView(paneID); wv != nil {
				fields = fields.
					Str("current_uri", logging.TruncateURL(wv.URI(), logURLMaxLen)).
					Bool("is_loading", wv.IsLoading())
			}
		}
	}
	fields.Msg("popup close signal received")
}

func (pm *popupManager) findPopupPaneForClose(hooks popupCoordinatorHooks, popupID port.WebViewID) (entity.PaneID, bool) {
	if hooks.findPaneByWebViewID == nil {
		return "", false
	}
	paneID, ok := hooks.findPaneByWebViewID(popupID)
	return paneID, ok && paneID != ""
}

func (pm *popupManager) releasePopupWebView(ctx context.Context, hooks popupCoordinatorHooks, paneID entity.PaneID) {
	if paneID == "" || hooks.getWebView == nil || hooks.releaseWebView == nil {
		return
	}
	if hooks.getWebView(paneID) != nil {
		hooks.releaseWebView(ctx, paneID)
	}
}

func (pm *popupManager) cleanupPopupClose(
	ctx context.Context,
	hooks popupCoordinatorHooks,
	popupID port.WebViewID,
	paneID entity.PaneID,
	closeErrMsg string,
) {
	log := logging.FromContext(ctx)
	if hooks.handlePopupOAuthClose != nil {
		hooks.handlePopupOAuthClose(ctx, popupID)
	}
	if paneID != "" && pm.onClosePane != nil {
		if err := pm.onClosePane(ctx, paneID); err != nil {
			log.Warn().Err(err).Str("pane_id", string(paneID)).Msg(closeErrMsg)
		}
	}
	pm.clearReusableNamedPopupByWebViewID(popupID)
	pm.releasePopupWebView(ctx, hooks, paneID)
}

func (pm *popupManager) handlePopupClose(ctx context.Context, hooks popupCoordinatorHooks, popupID port.WebViewID) {
	log := logging.FromContext(ctx)
	pm.logPopupCloseSignal(ctx, hooks, popupID)

	pending, wasPending := pm.takePendingPopup(popupID)
	if wasPending && pending != nil {
		pm.cleanupPopupClose(ctx, hooks, popupID, pending.PaneID, "failed to close pending popup pane")
		log.Debug().Str("pane_id", string(pending.PaneID)).Msg("cleaned up pending popup that was never shown")
		return
	}

	paneID, ok := pm.findPopupPaneForClose(hooks, popupID)
	if !ok {
		pm.cleanupPopupClose(ctx, hooks, popupID, "", "")
		log.Warn().Msg("popup close: could not find pane for webview")
		return
	}

	pm.cleanupPopupClose(ctx, hooks, popupID, paneID, "failed to close popup pane")
	log.Info().Str("pane_id", string(paneID)).Msg("popup closed")
}

func (pm *popupManager) handleLinkMiddleClick(
	ctx context.Context,
	hooks popupCoordinatorHooks,
	parentPaneID entity.PaneID,
	uri string,
) bool {
	log := logging.FromContext(ctx)

	log.Info().
		Str("parent_pane", string(parentPaneID)).
		Str("uri", logging.TruncateURL(uri, logURLMaxLen)).
		Msg("middle-click/ctrl+click on link")

	if pm.popupConfig != nil && !pm.popupConfig.OpenInNewPane {
		log.Debug().Msg("popups disabled by config, ignoring middle-click")
		return false
	}
	if pm.factory == nil {
		log.Warn().Msg("no webview factory, cannot handle middle-click")
		return false
	}
	if hooks.getWebView == nil {
		log.Warn().Msg("no parent webview lookup available for middle-click browsing context")
		return false
	}
	parentWV := hooks.getWebView(parentPaneID)
	if parentWV == nil {
		log.Warn().Str("parent_pane", string(parentPaneID)).Msg("parent webview not found for middle-click browsing context")
		return false
	}

	decision := pm.policy.Decide(buildLinkBrowsingContextRequest(parentWV.ID(), uri), false)
	log.Debug().
		Str("decision", string(decision.Kind)).
		Str("reason", decision.Reason).
		Msg("middle-click browsing context host decision")
	if decision.Kind != port.HostDecisionCreatePane {
		log.Info().Str("decision", string(decision.Kind)).Msg("middle-click browsing context not pane-hosted")
		return false
	}

	newWV, err := pm.createPopupWebView(ctx, parentWV.ID(), uri, true)
	if err != nil {
		log.Error().Err(err).Msg("failed to create webview for middle-click")
		return false
	}
	pm.setBrowsingContextDecision(newWV, decision)

	var paneID entity.PaneID
	if pm.generatePaneID != nil {
		paneID = entity.PaneID(pm.generatePaneID())
	} else {
		paneID = entity.PaneID(fmt.Sprintf("link_%d", newWV.ID()))
	}

	newPane := entity.NewPane(paneID)
	newPane.WindowType = entity.WindowPopup
	newPane.URI = uri

	if hooks.setWebView != nil {
		hooks.setWebView(paneID, newWV)
	}
	if hooks.setupWebViewCallbacks != nil {
		hooks.setupWebViewCallbacks(ctx, paneID, newWV)
	}

	behavior, placement := popupTabInsertionConfig(pm.popupConfig)

	if pm.onInsertPopup != nil {
		popupInput := InsertPopupInput{
			ParentPaneID: parentPaneID,
			PopupPane:    newPane,
			WebView:      newWV,
			Behavior:     behavior,
			Placement:    placement,
			PopupType:    PopupTypeTab,
			TargetURI:    uri,
		}
		if err := pm.onInsertPopup(ctx, popupInput); err != nil {
			log.Error().Err(err).Msg("failed to insert middle-click pane into workspace")
			if hooks.deleteWebView != nil {
				hooks.deleteWebView(paneID)
			}
			newWV.Destroy()
			return false
		}
	}

	if err := newWV.LoadURI(ctx, uri); err != nil {
		log.Error().Err(err).Str("uri", logging.TruncateURL(uri, logURLMaxLen)).Msg("failed to load URI in new pane")
	}

	log.Info().
		Str("pane_id", string(paneID)).
		Str("behavior", string(behavior)).
		Str("uri", logging.TruncateURL(uri, logURLMaxLen)).
		Msg("middle-click link opened in new pane")

	return true
}
