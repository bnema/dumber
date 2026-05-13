package content

import (
	"strings"

	"github.com/bnema/dumber/internal/application/port"
)

type browsingContextPolicy struct{}

func (browsingContextPolicy) Decide(req port.NewBrowsingContextRequest, namedContextExists bool) port.HostDecision {
	decision := port.HostDecision{
		RequestContextDisposition: req.RequestContextDisposition,
		RequiresNativeOpener:      req.RequiresNativeOpener,
	}
	if decision.RequestContextDisposition == "" {
		decision.RequestContextDisposition = port.RequestContextInheritParent
	}

	if strings.TrimSpace(req.TargetURI) == "" {
		decision.Kind = port.HostDecisionDeny
		decision.Reason = "empty target URI"
		return decision
	}

	if req.AuthIntent {
		decision.Kind = port.HostDecisionCreateNativeWin
		decision.Reason = "auth intent requires native window"
		return decision
	}
	if req.RequiresNativeOpener {
		decision.Kind = port.HostDecisionCreateNativeWin
		decision.Reason = "native opener required"
		return decision
	}
	if isAmbiguousNativeBrowsingContext(req) {
		decision.Kind = port.HostDecisionCreateNativeWin
		decision.Reason = "ambiguous opener-coupled request prefers native window"
		return decision
	}

	name := reusableBrowsingContextName(req.TargetFrameName)
	if namedContextExists && name != "" {
		decision.Kind = port.HostDecisionReuseNamedPane
		decision.ReuseContextName = name
		decision.BrowsingContextName = name
		decision.Reason = "reuse named browsing context in current window"
		return decision
	}

	decision.Kind = port.HostDecisionCreatePane
	decision.BrowsingContextName = name
	decision.Reason = "pane-hosted browsing context"
	return decision
}

func buildPopupBrowsingContextRequest(req port.PopupRequest) port.NewBrowsingContextRequest {
	return port.NewBrowsingContextRequest{
		ParentWebViewID:            req.ParentViewID,
		SourceBrowserID:            req.SourceBrowserID,
		SourceFrameID:              req.SourceFrameID,
		SourceFrameURL:             req.SourceFrameURL,
		TargetURI:                  req.TargetURI,
		TargetFrameName:            req.FrameName,
		TargetDisposition:          inferPopupWindowDisposition(req),
		IsUserGesture:              req.IsUserGesture,
		NoJavaScriptAccess:         req.NoJavaScriptAccess,
		WindowFeatures:             req.WindowFeatures,
		TriggerKind:                inferPopupTriggerKind(req),
		AuthIntent:                 IsOAuthURL(req.TargetURI),
		RequiresNativeOpener:       false,
		RequestContextDisposition:  port.RequestContextInheritParent,
	}
}

func buildLinkBrowsingContextRequest(parentWebViewID port.WebViewID, uri string) port.NewBrowsingContextRequest {
	return port.NewBrowsingContextRequest{
		ParentWebViewID:            parentWebViewID,
		TargetURI:                  uri,
		TargetFrameName:            "_blank",
		TargetDisposition:          port.WindowDispositionNewTab,
		IsUserGesture:              true,
		NoJavaScriptAccess:         true,
		TriggerKind:                port.TriggerLinkNewPage,
		AuthIntent:                 IsOAuthURL(uri),
		RequestContextDisposition:  port.RequestContextInheritParent,
	}
}

func inferPopupWindowDisposition(req port.PopupRequest) port.WindowDisposition {
	if req.TargetDisposition != "" {
		return req.TargetDisposition
	}
	if req.FrameName == "_blank" {
		return port.WindowDispositionNewTab
	}
	return port.WindowDispositionNewPopup
}

func inferPopupTriggerKind(req port.PopupRequest) port.TriggerKind {
	switch inferPopupWindowDisposition(req) {
	case port.WindowDispositionNewTab:
		if reusableBrowsingContextName(req.FrameName) != "" {
			return port.TriggerNamedTargetNavigation
		}
		return port.TriggerLinkNewPage
	case port.WindowDispositionNewPopup, port.WindowDispositionNewWindow:
		if reusableBrowsingContextName(req.FrameName) != "" {
			return port.TriggerNamedTargetNavigation
		}
		return port.TriggerScriptWindowOpen
	default:
		if reusableBrowsingContextName(req.FrameName) != "" {
			return port.TriggerNamedTargetNavigation
		}
		return port.TriggerUnknown
	}
}

func isAmbiguousNativeBrowsingContext(req port.NewBrowsingContextRequest) bool {
	return req.TriggerKind == port.TriggerUnknown && !req.NoJavaScriptAccess
}

func reusableBrowsingContextName(frameName string) string {
	name := strings.TrimSpace(frameName)
	if name == "" || strings.EqualFold(name, "_blank") {
		return ""
	}
	return name
}
