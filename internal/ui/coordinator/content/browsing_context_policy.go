package content

import (
	"strings"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/rs/zerolog"
)

type browsingContextPolicy struct{}

func (browsingContextPolicy) Decide(req dto.NewBrowsingContextRequest, namedContextExists bool) dto.HostDecision {
	decision := dto.HostDecision{
		SourceHost:                req.SourceHost,
		RequestContextDisposition: req.RequestContextDisposition,
		RequiresNativeOpener:      req.RequiresNativeOpener,
	}
	if decision.RequestContextDisposition == "" {
		decision.RequestContextDisposition = dto.RequestContextInheritParent
	}

	switch {
	case strings.TrimSpace(req.TargetURI) == "":
		return completeHostDecision(decision, dto.HostDecisionDeny, dto.HostDecisionReasonEmptyTarget, "empty target URI")
	case req.AuthIntent:
		return completeHostDecision(
			decision, dto.HostDecisionCreateNativePopup, dto.HostDecisionReasonAuthNativePopup,
			"authentication intent requires a related native popup",
		)
	case req.RequiresNativeOpener:
		return completeHostDecision(
			decision, dto.HostDecisionCreateNativePopup, dto.HostDecisionReasonNativeOpenerRequired,
			"native opener relationship is required",
		)
	case isAmbiguousNativeBrowsingContext(req):
		return completeHostDecision(
			decision, dto.HostDecisionCreateNativePopup, dto.HostDecisionReasonAmbiguousOpenerNative,
			"ambiguous opener-coupled request prefers a related native popup",
		)
	}

	name := reusableBrowsingContextName(req.TargetFrameName)
	if namedContextExists && name != "" && !req.NoJavaScriptAccess {
		decision.ReuseContextName = name
		decision.BrowsingContextName = name
		return completeHostDecision(
			decision, dto.HostDecisionReuseNamedPane, dto.HostDecisionReasonNamedContextReuse,
			"reuse live named browsing context",
		)
	}
	decision.BrowsingContextName = name

	if req.SourceHost == dto.SourceHostFloating {
		return decideFloatingHost(decision, req)
	}
	return completeHostDecision(
		decision, dto.HostDecisionCreatePane, dto.HostDecisionReasonWorkspacePane,
		"workspace request uses pane hosting",
	)
}

func decideFloatingHost(decision dto.HostDecision, req dto.NewBrowsingContextRequest) dto.HostDecision {
	switch {
	case req.PopupFeatures.State == dto.PopupFeaturesUnknown && req.TriggerKind != dto.TriggerLinkNewPage:
		return completeHostDecision(
			decision, dto.HostDecisionAwaitPopupFeatures, dto.HostDecisionReasonFloatingFeaturesPending,
			"popup features unavailable until ready-to-show",
		)
	case req.PopupFeatures.RequestsPopupHost():
		return completeHostDecision(
			decision, dto.HostDecisionCreateNativePopup, dto.HostDecisionReasonFloatingPopupFeatures,
			"explicit popup features requested",
		)
	case reusableBrowsingContextName(req.TargetFrameName) == "":
		return completeHostDecision(
			decision, dto.HostDecisionNavigateSource, dto.HostDecisionReasonFloatingFeaturelessBlank,
			"featureless blank request navigates the source floating pane",
		)
	default:
		return completeHostDecision(
			decision, dto.HostDecisionCreateBrowserWindow, dto.HostDecisionReasonFloatingBrowserWindow,
			"floating request opens a detached browser window",
		)
	}
}

func completeHostDecision(
	decision dto.HostDecision,
	kind dto.HostDecisionKind,
	code dto.HostDecisionReasonCode,
	reason string,
) dto.HostDecision {
	decision.Kind = kind
	decision.ReasonCode = code
	decision.Reason = reason
	return decision
}

func logBrowsingContextDecision(logger zerolog.Logger, req dto.NewBrowsingContextRequest, decision dto.HostDecision) {
	logger.Debug().
		Str("engine", string(req.Engine)).
		Str("source_host", string(req.SourceHost)).
		Str("decision", string(decision.Kind)).
		Str("target_disposition", string(req.TargetDisposition)).
		Str("reason_code", string(decision.ReasonCode)).
		Msg("browsing context host decided")
}

func logBrowsingContextFailure(
	logger zerolog.Logger,
	req dto.NewBrowsingContextRequest,
	decision dto.HostDecision,
	code dto.BrowsingContextFailureCode,
	err error,
) {
	logger.Error().Err(err).
		Str("engine", string(req.Engine)).
		Str("source_host", string(req.SourceHost)).
		Str("decision", string(decision.Kind)).
		Str("target_disposition", string(req.TargetDisposition)).
		Str("reason_code", string(code)).
		Msg("browsing context hosting failed")
}

func buildPopupBrowsingContextRequest(req port.PopupRequest) dto.NewBrowsingContextRequest {
	return dto.NewBrowsingContextRequest{
		ParentWebViewID:           uint64(req.ParentViewID),
		Engine:                    req.Engine,
		SourceBrowserID:           req.SourceBrowserID,
		SourceFrameID:             req.SourceFrameID,
		SourceFrameURL:            req.SourceFrameURL,
		TargetURI:                 req.TargetURI,
		TargetFrameName:           req.FrameName,
		TargetDisposition:         inferPopupWindowDisposition(req),
		IsUserGesture:             req.IsUserGesture,
		NoJavaScriptAccess:        req.NoJavaScriptAccess,
		PopupFeatures:             req.PopupFeatures,
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
