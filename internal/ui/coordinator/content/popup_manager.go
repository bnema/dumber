package content

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	domainerrors "github.com/bnema/dumber/internal/domain/errors"
	"github.com/bnema/dumber/internal/logging"
)

// popupManager owns popup-specific coordinator state. It stays in the UI
// adapter layer so popup pane bookkeeping and workspace orchestration do not
// leak into the application/usecase or domain layers.
type popupManager struct {
	factory                     port.WebViewFactory
	popupConfig                 *entity.PopupBehaviorConfig
	onInsertPopup               func(ctx context.Context, input InsertPopupInput) error
	onClosePane                 func(ctx context.Context, paneID entity.PaneID) error
	generatePaneID              func() string
	pendingPopups               map[port.WebViewID]*PendingPopup
	namedPopups                 map[namedPopupKey]*namedPopupState
	popupOAuth                  map[port.WebViewID]*popupOAuthState
	popupRefresh                map[entity.PaneID]*time.Timer
	relatedPopupUnsupported     bool
	relatedPopupSupportDetected bool
	mu                          sync.RWMutex
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
	if pm.namedPopups == nil {
		pm.namedPopups = make(map[namedPopupKey]*namedPopupState)
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
	popupConfig *entity.PopupBehaviorConfig,
	generateID func() string,
) {
	if pm == nil {
		return
	}
	pm.ensureInitialized()
	pm.factory = factory
	pm.popupConfig = popupConfig
	pm.generatePaneID = generateID

	pm.mu.Lock()
	pm.relatedPopupUnsupported = false
	pm.relatedPopupSupportDetected = false
	pm.mu.Unlock()
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

func (pm *popupManager) relatedPopupSupportDisabled() bool {
	if pm == nil {
		return false
	}
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.relatedPopupSupportDetected && pm.relatedPopupUnsupported
}

func (pm *popupManager) markRelatedPopupUnsupported() {
	if pm == nil {
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.relatedPopupSupportDetected = true
	pm.relatedPopupUnsupported = true
}

func (pm *popupManager) markRelatedPopupSupported() {
	if pm == nil {
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.relatedPopupSupportDetected = true
	pm.relatedPopupUnsupported = false
}

func (pm *popupManager) lookupReusableNamedPopup(parentPaneID entity.PaneID, frameName string) (*namedPopupState, bool) {
	if pm == nil || !isReusableNamedPopupFrame(frameName) {
		return nil, false
	}

	key := namedPopupKey{ParentPaneID: parentPaneID, FrameName: frameName}

	pm.mu.RLock()
	state, ok := pm.namedPopups[key]
	pm.mu.RUnlock()
	if !ok || state == nil || state.WebView == nil {
		return nil, false
	}

	if state.WebView.IsDestroyed() {
		pm.mu.Lock()
		if current, ok := pm.namedPopups[key]; ok && current == state {
			delete(pm.namedPopups, key)
		}
		pm.mu.Unlock()
		return nil, false
	}

	return state, true
}

func (pm *popupManager) storeReusableNamedPopup(parentPaneID entity.PaneID, frameName string, wv port.WebView) {
	if pm == nil || !isReusableNamedPopupFrame(frameName) || wv == nil {
		return
	}

	key := namedPopupKey{ParentPaneID: parentPaneID, FrameName: frameName}
	pm.mu.Lock()
	pm.namedPopups[key] = &namedPopupState{WebView: wv}
	pm.mu.Unlock()
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
	if pm == nil {
		return
	}
	pm.mu.Lock()
	for key, state := range pm.namedPopups {
		if state != nil && state.WebView != nil && state.WebView.ID() == popupID {
			delete(pm.namedPopups, key)
		}
	}
	pm.mu.Unlock()
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
	noJavaScriptAccess bool,
) (port.WebView, bool, error) {
	if pm == nil || pm.factory == nil {
		return nil, false, fmt.Errorf("no webview factory configured")
	}

	log := logging.FromContext(ctx)
	relatedErr := error(nil)

	if !noJavaScriptAccess && !pm.relatedPopupSupportDisabled() {
		popupWV, err := pm.factory.CreateRelated(ctx, parentID)
		if err == nil && popupWV != nil {
			pm.markRelatedPopupSupported()
			return popupWV, false, nil
		}
		if err == nil {
			relatedErr = fmt.Errorf("related popup webview factory returned nil without error")
			log.Warn().
				Uint64("parent_webview_id", uint64(parentID)).
				Str("target_uri", logging.TruncateURL(targetURI, logURLMaxLen)).
				Msg("related popup webview factory returned nil popup, falling back to regular webview")
		} else if errors.Is(err, domainerrors.ErrRelatedWebViewUnsupported) {
			relatedErr = err
			pm.markRelatedPopupUnsupported()
			log.Debug().
				Err(err).
				Uint64("parent_webview_id", uint64(parentID)).
				Str("target_uri", logging.TruncateURL(targetURI, logURLMaxLen)).
				Msg("related popup webview unavailable, falling back to regular webview")
		} else {
			relatedErr = err
			log.Warn().
				Err(err).
				Uint64("parent_webview_id", uint64(parentID)).
				Str("target_uri", logging.TruncateURL(targetURI, logURLMaxLen)).
				Msg("related popup webview creation failed, falling back to regular webview")
		}
	} else if noJavaScriptAccess {
		log.Debug().
			Uint64("parent_webview_id", uint64(parentID)).
			Str("target_uri", logging.TruncateURL(targetURI, logURLMaxLen)).
			Msg("popup requested no JavaScript opener access, creating regular webview")
	} else {
		log.Debug().
			Uint64("parent_webview_id", uint64(parentID)).
			Str("target_uri", logging.TruncateURL(targetURI, logURLMaxLen)).
			Msg("related popup webviews known unsupported, creating regular webview")
	}

	popupWV, fallbackErr := pm.factory.Create(ctx)
	if fallbackErr != nil {
		if relatedErr != nil {
			return nil, false, fmt.Errorf("create popup webview: related failed: %w; fallback failed: %w", relatedErr, fallbackErr)
		}
		return nil, false, fmt.Errorf("create popup webview: fallback failed: %w", fallbackErr)
	}
	if popupWV == nil {
		if relatedErr != nil {
			return nil, false, fmt.Errorf("create popup webview: related failed: %w; fallback returned nil", relatedErr)
		}
		return nil, false, fmt.Errorf("create popup webview: fallback returned nil")
	}

	return popupWV, true, nil
}

func (pm *popupManager) reuseNamedPopup(
	ctx context.Context,
	parentPaneID entity.PaneID,
	frameName string,
	targetURI string,
) (port.WebView, bool) {
	log := logging.FromContext(ctx)

	if existing, ok := pm.lookupReusableNamedPopup(parentPaneID, frameName); ok {
		pm.updatePendingPopupTarget(existing.WebView.ID(), targetURI)
		if err := existing.WebView.LoadURI(ctx, targetURI); err != nil {
			log.Warn().Err(err).
				Str("target_uri", logging.TruncateURL(targetURI, logURLMaxLen)).
				Msg("failed to load target URI in reused popup")
		}
		log.Info().
			Str("parent_pane", string(parentPaneID)).
			Str("frame_name", frameName).
			Str("target_uri", logging.TruncateURL(targetURI, logURLMaxLen)).
			Msg("reused named popup")
		return existing.WebView, true
	}

	return nil, false
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
	parentURIAtOpen := ""
	if pm.popupConfig != nil && pm.popupConfig.OAuthAutoClose && IsOAuthURL(req.TargetURI) {
		parentURIAtOpen = parentWV.URI()
		if parentURIAtOpen == "" && hooks.getWebView != nil {
			if parent := hooks.getWebView(parentPaneID); parent != nil {
				parentURIAtOpen = parent.URI()
			}
		}
	}
	if !req.NoJavaScriptAccess {
		if reused, ok := pm.reuseNamedPopup(ctx, parentPaneID, req.FrameName, req.TargetURI); ok {
			return reused
		}
	}

	popupWV, usedRegularFallback, err := pm.createPopupWebView(ctx, parentID, req.TargetURI, req.NoJavaScriptAccess)
	if err != nil {
		log.Error().Err(err).Msg("failed to create webview for popup")
		return nil
	}
	if usedRegularFallback {
		if openerBridge, ok := popupWV.(port.PopupOpenerCapable); ok {
			openerBridge.EnablePopupOpenerBridge(parentWV, req.NoJavaScriptAccess)
		}
	}

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
		pm.storeReusableNamedPopup(create.ParentPaneID, create.Request.FrameName, create.PopupWebView)
	}
	if _, hasNativePopupLifecycle := create.PopupWebView.(port.PopupLifecycleCapable); !hasNativePopupLifecycle {
		if closeCapable, ok := create.PopupWebView.(port.OAuthCallbackCapable); ok {
			closeCapable.AddCloseCallback(func() {
				pm.handlePopupClose(ctx, hooks, create.PopupID)
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
			pm.handlePopupReadyToShow(ctx, create.PopupID)
		})
		lifecycle.SetOnClose(func() {
			pm.handlePopupClose(ctx, hooks, create.PopupID)
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
		pm.handlePopupReadyToShow(ctx, create.PopupID)
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
					Bool("is_loading", wv.IsLoading()).
					Bool("synthetic_opener_active", popupUsesSyntheticOpenerSignals(wv))
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

	newWV, err := pm.factory.Create(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to create webview for middle-click")
		return false
	}

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

	behavior := entity.PopupBehaviorStacked
	if pm.popupConfig != nil && pm.popupConfig.BlankTargetBehavior != "" {
		behavior = entity.PopupBehavior(pm.popupConfig.BlankTargetBehavior)
	}
	placement := "right"
	if pm.popupConfig != nil {
		placement = pm.popupConfig.Placement
	}

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
