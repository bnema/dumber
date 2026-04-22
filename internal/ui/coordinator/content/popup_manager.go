package content

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
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

func newPopupManager() *popupManager {
	pm := &popupManager{}
	pm.ensureInitialized()
	return pm
}

func (pm *popupManager) ensureInitialized() {
	if pm == nil {
		return
	}
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
