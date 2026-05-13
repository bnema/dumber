// Package port defines interfaces (ports) for use cases to depend on.
// These types are the active seam for the approved browsing-context architecture
// shared across policy, UI, and engine integrations.

package port

// WindowDisposition describes how the browsing context should be presented
// relative to the source frame. Values mirror Chromium/CEF/WebKit navigation
// dispositions at a normalized product seam.
type WindowDisposition string

const (
	// WindowDispositionCurrentTab replaces the current document in the same browsing context.
	WindowDispositionCurrentTab WindowDisposition = "current-tab"
	// WindowDispositionNewTab opens in a new tab-like context.
	WindowDispositionNewTab WindowDisposition = "new-tab"
	// WindowDispositionNewPopup opens in a popup-style context.
	WindowDispositionNewPopup WindowDisposition = "new-popup"
	// WindowDispositionNewWindow opens in a new top-level window.
	WindowDispositionNewWindow WindowDisposition = "new-window"
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

// RequestContextDisposition makes request-context inheritance explicit at the
// policy seam without leaking infrastructure request-context types upward.
type RequestContextDisposition string

const (
	// RequestContextInheritParent preserves the parent request context/session.
	RequestContextInheritParent RequestContextDisposition = "inherit-parent"
	// RequestContextIsolate starts from a fresh unrelated request context.
	RequestContextIsolate RequestContextDisposition = "isolate"
)

// NewBrowsingContextRequest is the normalized request captured from engine/UI
// signals before policy decides how it should be hosted.
type NewBrowsingContextRequest struct {
	ParentWebViewID WebViewID

	SourceBrowserID int32
	SourceFrameID   string
	SourceFrameURL  string

	TargetURI         string
	TargetFrameName   string
	TargetDisposition WindowDisposition

	IsUserGesture      bool
	NoJavaScriptAccess bool
	WindowFeatures     string

	TriggerKind               TriggerKind
	AuthIntent                bool
	RequiresNativeOpener      bool
	RequestContextDisposition RequestContextDisposition
}

// HostDecisionKind categorizes the approved host choices for a new browsing context.
type HostDecisionKind string

const (
	HostDecisionReuseNamedPane  HostDecisionKind = "reuse-named-pane"
	HostDecisionCreatePane      HostDecisionKind = "create-pane"
	HostDecisionCreateNativeWin HostDecisionKind = "create-native-window"
	HostDecisionDeny            HostDecisionKind = "deny"
)

// HostDecision represents the output of browsing-context policy.
type HostDecision struct {
	Kind HostDecisionKind

	ReuseContextName    string
	BrowsingContextName string

	RequestContextDisposition RequestContextDisposition
	RequiresNativeOpener      bool
	Reason                    string
}

// PaneHostedBrowsingContextCapable is implemented by engines that need an
// explicit transition from a related/native-popup shell into a pane-hosted
// child browsing context path.
type PaneHostedBrowsingContextCapable interface {
	PreparePaneHostedBrowsingContext()
}

// BrowsingContextHostDecisionCapable lets UI coordinators attach the chosen
// host decision to an engine webview so lower-level popup handlers can honor
// pane-vs-native behavior synchronously.
type BrowsingContextHostDecisionCapable interface {
	SetBrowsingContextHostDecision(decision HostDecision)
	BrowsingContextHostDecision() (HostDecision, bool)
}

// NativePopupHostAbortCapable lets the engine-level popup seam abort an
// already-created native popup shell/window when native arming fails.
type NativePopupHostAbortCapable interface {
	SetNativePopupHostAbort(fn func())
	AbortNativePopupHost()
}
