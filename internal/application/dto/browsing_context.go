package dto

// SourceHostKind identifies the product host that originated a browsing-context request.
type SourceHostKind string

const (
	// SourceHostWorkspace is the default persisted workspace pane host.
	SourceHostWorkspace SourceHostKind = "workspace"
	// SourceHostFloating is a transient floating pane outside the workspace tree.
	SourceHostFloating SourceHostKind = "floating"
)

// BrowserEngineKind identifies the browser engine that produced a normalized request.
type BrowserEngineKind string

const (
	BrowserEngineUnknown BrowserEngineKind = "unknown"
	BrowserEngineCEF     BrowserEngineKind = "cef"
	BrowserEngineWebKit  BrowserEngineKind = "webkit"
)

// PopupFeatureState describes whether popup metadata is available and explicit.
type PopupFeatureState string

const (
	PopupFeaturesUnknown   PopupFeatureState = "unknown"
	PopupFeaturesNone      PopupFeatureState = "none"
	PopupFeaturesSpecified PopupFeatureState = "specified"
)

// PopupFeatures is the engine-independent subset of window.open features used by host policy.
type PopupFeatures struct {
	State PopupFeatureState

	X, Y, Width, Height                int
	XSet, YSet, WidthSet, HeightSet    bool
	ToolbarVisible, LocationbarVisible bool
	ToolbarVisibilitySet               bool
	LocationbarVisibilitySet           bool
	Resizable, ResizableSet            bool
	IsPopup, IsPopupSet                bool
}

// RequestsPopupHost reports whether explicit features require a constrained native popup.
func (f PopupFeatures) RequestsPopupHost() bool {
	return f.State == PopupFeaturesSpecified &&
		((f.WidthSet && f.Width > 0) ||
			(f.HeightSet && f.Height > 0) ||
			(f.ToolbarVisibilitySet && !f.ToolbarVisible) ||
			(f.LocationbarVisibilitySet && !f.LocationbarVisible) ||
			(f.ResizableSet && !f.Resizable) ||
			(f.IsPopupSet && f.IsPopup))
}

// WindowDisposition describes how the browsing context should be presented.
type WindowDisposition string

const (
	WindowDispositionCurrentTab WindowDisposition = "current-tab"
	WindowDispositionNewTab     WindowDisposition = "new-tab"
	WindowDispositionNewPopup   WindowDisposition = "new-popup"
	WindowDispositionNewWindow  WindowDisposition = "new-window"
)

// TriggerKind categorizes what triggered a browsing-context request.
type TriggerKind string

const (
	TriggerUnknown               TriggerKind = "unknown"
	TriggerLinkNewPage           TriggerKind = "link-new-page"
	TriggerScriptWindowOpen      TriggerKind = "script-window-open"
	TriggerNamedTargetNavigation TriggerKind = "named-target-navigation"
	TriggerAuthPopupRequest      TriggerKind = "auth-popup-request"
)

// RequestContextDisposition makes request-context inheritance explicit at the policy seam.
type RequestContextDisposition string

const (
	RequestContextInheritParent RequestContextDisposition = "inherit-parent"
	RequestContextIsolate       RequestContextDisposition = "isolate"
)

// NewBrowsingContextRequest is the normalized request captured before host policy runs.
type NewBrowsingContextRequest struct {
	ParentWebViewID uint64

	Engine     BrowserEngineKind
	SourceHost SourceHostKind

	SourceBrowserID int32
	SourceFrameID   string
	SourceFrameURL  string

	TargetURI         string
	TargetFrameName   string
	TargetDisposition WindowDisposition

	IsUserGesture      bool
	NoJavaScriptAccess bool
	PopupFeatures      PopupFeatures

	TriggerKind               TriggerKind
	AuthIntent                bool
	RequiresNativeOpener      bool
	RequestContextDisposition RequestContextDisposition
}

// HostDecisionKind categorizes approved hosts for a new browsing context.
type HostDecisionKind string

const (
	HostDecisionReuseNamedPane      HostDecisionKind = "reuse-named-pane"
	HostDecisionCreatePane          HostDecisionKind = "create-pane"
	HostDecisionCreateBrowserWindow HostDecisionKind = "create-browser-window"
	HostDecisionCreateNativePopup   HostDecisionKind = "create-native-popup"
	HostDecisionAwaitPopupFeatures  HostDecisionKind = "await-popup-features"
	HostDecisionDeny                HostDecisionKind = "deny"
)

// HostDecisionReasonCode is a stable machine-readable policy explanation.
type HostDecisionReasonCode string

const (
	HostDecisionReasonEmptyTarget             HostDecisionReasonCode = "empty-target"
	HostDecisionReasonAuthNativePopup         HostDecisionReasonCode = "auth-native-popup"
	HostDecisionReasonNativeOpenerRequired    HostDecisionReasonCode = "native-opener-required"
	HostDecisionReasonAmbiguousOpenerNative   HostDecisionReasonCode = "ambiguous-opener-native"
	HostDecisionReasonNamedContextReuse       HostDecisionReasonCode = "named-context-reuse"
	HostDecisionReasonFloatingFeaturesPending HostDecisionReasonCode = "floating-features-pending"
	HostDecisionReasonFloatingPopupFeatures   HostDecisionReasonCode = "floating-popup-features"
	HostDecisionReasonFloatingBrowserWindow   HostDecisionReasonCode = "floating-browser-window"
	HostDecisionReasonWorkspacePane           HostDecisionReasonCode = "workspace-pane"
)

// BrowsingContextFailureCode is a stable machine-readable operational failure.
type BrowsingContextFailureCode string

const (
	BrowsingContextFailureHostUnavailable   BrowsingContextFailureCode = "host-unavailable"
	BrowsingContextFailureHostFailed        BrowsingContextFailureCode = "host-failed"
	BrowsingContextFailureFeatureResolution BrowsingContextFailureCode = "feature-resolution-failed"
	BrowsingContextFailureStagingDetach     BrowsingContextFailureCode = "staging-detach-failed"
	BrowsingContextFailureNativeArm         BrowsingContextFailureCode = "native-arm-failed"
	BrowsingContextFailureFallback          BrowsingContextFailureCode = "fallback-failed"
)

// HostDecision represents the output of browsing-context policy.
type HostDecision struct {
	Kind HostDecisionKind

	ReuseContextName    string
	BrowsingContextName string

	RequestContextDisposition RequestContextDisposition
	RequiresNativeOpener      bool
	ReasonCode                HostDecisionReasonCode
	Reason                    string
}
