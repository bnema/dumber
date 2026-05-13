package content

import (
	"strings"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
)

type browsingContextPolicy struct{}

func (browsingContextPolicy) Decide(req dto.NewBrowsingContextRequest, namedContextExists bool) dto.HostDecision {
	decision := dto.HostDecision{
		RequestContextDisposition: req.RequestContextDisposition,
		RequiresNativeOpener:      req.RequiresNativeOpener,
	}
	if decision.RequestContextDisposition == "" {
		decision.RequestContextDisposition = dto.RequestContextInheritParent
	}

	if strings.TrimSpace(req.TargetURI) == "" {
		decision.Kind = dto.HostDecisionDeny
		decision.Reason = "empty target URI"
		return decision
	}

	if req.AuthIntent {
		decision.Kind = dto.HostDecisionCreateNativeWin
		decision.Reason = "auth intent requires native window"
		return decision
	}
	if req.RequiresNativeOpener {
		decision.Kind = dto.HostDecisionCreateNativeWin
		decision.Reason = "native opener required"
		return decision
	}
	if isAmbiguousNativeBrowsingContext(req) {
		decision.Kind = dto.HostDecisionCreateNativeWin
		decision.Reason = "ambiguous opener-coupled request prefers native window"
		return decision
	}

	name := reusableBrowsingContextName(req.TargetFrameName)
	if namedContextExists && name != "" {
		decision.Kind = dto.HostDecisionReuseNamedPane
		decision.ReuseContextName = name
		decision.BrowsingContextName = name
		decision.Reason = "reuse named browsing context in current window"
		return decision
	}

	decision.Kind = dto.HostDecisionCreatePane
	decision.BrowsingContextName = name
	decision.Reason = "pane-hosted browsing context"
	return decision
}

func buildPopupBrowsingContextRequest(req port.PopupRequest) dto.NewBrowsingContextRequest {
	return dto.NewBrowsingContextRequest{
		ParentWebViewID:           uint64(req.ParentViewID),
		SourceBrowserID:           req.SourceBrowserID,
		SourceFrameID:             req.SourceFrameID,
		SourceFrameURL:            req.SourceFrameURL,
		TargetURI:                 req.TargetURI,
		TargetFrameName:           req.FrameName,
		TargetDisposition:         inferPopupWindowDisposition(req),
		IsUserGesture:             req.IsUserGesture,
		NoJavaScriptAccess:        req.NoJavaScriptAccess,
		WindowFeatures:            req.WindowFeatures,
		TriggerKind:               inferPopupTriggerKind(req),
		AuthIntent:                IsOAuthURL(req.TargetURI),
		RequiresNativeOpener:      false,
		RequestContextDisposition: dto.RequestContextInheritParent,
	}
}

func buildLinkBrowsingContextRequest(parentWebViewID port.WebViewID, uri string) dto.NewBrowsingContextRequest {
	return dto.NewBrowsingContextRequest{
		ParentWebViewID:           uint64(parentWebViewID),
		TargetURI:                 uri,
		TargetFrameName:           "_blank",
		TargetDisposition:         dto.WindowDispositionNewTab,
		IsUserGesture:             true,
		NoJavaScriptAccess:        true,
		TriggerKind:               dto.TriggerLinkNewPage,
		AuthIntent:                IsOAuthURL(uri),
		RequestContextDisposition: dto.RequestContextInheritParent,
	}
}

func inferPopupWindowDisposition(req port.PopupRequest) dto.WindowDisposition {
	if req.TargetDisposition != "" {
		return req.TargetDisposition
	}
	if strings.EqualFold(strings.TrimSpace(req.FrameName), "_blank") {
		return dto.WindowDispositionNewTab
	}
	return dto.WindowDispositionNewPopup
}

func inferPopupTriggerKind(req port.PopupRequest) dto.TriggerKind {
	switch inferPopupWindowDisposition(req) {
	case dto.WindowDispositionCurrentTab:
		if reusableBrowsingContextName(req.FrameName) != "" {
			return dto.TriggerNamedTargetNavigation
		}
		return dto.TriggerLinkNewPage
	case dto.WindowDispositionNewTab:
		if reusableBrowsingContextName(req.FrameName) != "" {
			return dto.TriggerNamedTargetNavigation
		}
		return dto.TriggerLinkNewPage
	case dto.WindowDispositionNewPopup, dto.WindowDispositionNewWindow:
		if reusableBrowsingContextName(req.FrameName) != "" {
			return dto.TriggerNamedTargetNavigation
		}
		return dto.TriggerScriptWindowOpen
	default:
		if reusableBrowsingContextName(req.FrameName) != "" {
			return dto.TriggerNamedTargetNavigation
		}
		return dto.TriggerUnknown
	}
}

func isAmbiguousNativeBrowsingContext(req dto.NewBrowsingContextRequest) bool {
	return req.TriggerKind == dto.TriggerUnknown && !req.NoJavaScriptAccess
}

func reusableBrowsingContextName(frameName string) string {
	name := strings.TrimSpace(frameName)
	if name == "" || strings.EqualFold(name, "_blank") {
		return ""
	}
	return name
}
