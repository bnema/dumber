package usecase

import (
	"strings"

	"github.com/bnema/dumber/internal/application/dto"
)

// BrowsingContextPolicy selects the product host for a normalized browsing-context request.
type BrowsingContextPolicy struct{}

// Decide returns the host decision without performing any I/O.
func (BrowsingContextPolicy) Decide(req dto.NewBrowsingContextRequest, namedContextExists bool) dto.HostDecision {
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
			decision,
			dto.HostDecisionCreateNativePopup,
			dto.HostDecisionReasonAuthNativePopup,
			"authentication intent requires a related native popup",
		)
	case req.RequiresNativeOpener:
		return completeHostDecision(
			decision,
			dto.HostDecisionCreateNativePopup,
			dto.HostDecisionReasonNativeOpenerRequired,
			"native opener relationship is required",
		)
	case isAmbiguousNativeBrowsingContext(req):
		return completeHostDecision(
			decision,
			dto.HostDecisionCreateNativePopup,
			dto.HostDecisionReasonAmbiguousOpenerNative,
			"ambiguous opener-coupled request prefers a related native popup",
		)
	}

	name := ReusableBrowsingContextName(req.TargetFrameName)
	if namedContextExists && name != "" && !req.NoJavaScriptAccess {
		decision.ReuseContextName = name
		decision.BrowsingContextName = name
		return completeHostDecision(
			decision,
			dto.HostDecisionReuseNamedPane,
			dto.HostDecisionReasonNamedContextReuse,
			"reuse live named browsing context",
		)
	}
	decision.BrowsingContextName = name

	if req.SourceHost == dto.SourceHostFloating {
		return decideFloatingHost(decision, req)
	}
	return completeHostDecision(
		decision,
		dto.HostDecisionCreatePane,
		dto.HostDecisionReasonWorkspacePane,
		"workspace request uses pane hosting",
	)
}

func decideFloatingHost(decision dto.HostDecision, req dto.NewBrowsingContextRequest) dto.HostDecision {
	switch {
	case req.PopupFeatures.State == dto.PopupFeaturesUnknown && req.TriggerKind != dto.TriggerLinkNewPage:
		return completeHostDecision(
			decision,
			dto.HostDecisionAwaitPopupFeatures,
			dto.HostDecisionReasonFloatingFeaturesPending,
			"popup features unavailable until ready-to-show",
		)
	case popupFeaturesRequestHost(req.PopupFeatures):
		return completeHostDecision(
			decision,
			dto.HostDecisionCreateNativePopup,
			dto.HostDecisionReasonFloatingPopupFeatures,
			"explicit popup features requested",
		)
	case ReusableBrowsingContextName(req.TargetFrameName) == "":
		return completeHostDecision(
			decision,
			dto.HostDecisionNavigateSource,
			dto.HostDecisionReasonFloatingFeaturelessBlank,
			"featureless blank request navigates the source floating pane",
		)
	default:
		return completeHostDecision(
			decision,
			dto.HostDecisionCreateBrowserWindow,
			dto.HostDecisionReasonFloatingBrowserWindow,
			"floating request opens a detached browser window",
		)
	}
}

func popupFeaturesRequestHost(features dto.PopupFeatures) bool {
	return features.State == dto.PopupFeaturesSpecified &&
		((features.WidthSet && features.Width > 0) ||
			(features.HeightSet && features.Height > 0) ||
			(features.ToolbarVisibilitySet && !features.ToolbarVisible) ||
			(features.LocationbarVisibilitySet && !features.LocationbarVisible) ||
			(features.ResizableSet && !features.Resizable) ||
			(features.IsPopupSet && features.IsPopup))
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

func isAmbiguousNativeBrowsingContext(req dto.NewBrowsingContextRequest) bool {
	return req.TriggerKind == dto.TriggerUnknown && !req.NoJavaScriptAccess
}

// ReusableBrowsingContextName returns a live-context key for named targets.
func ReusableBrowsingContextName(frameName string) string {
	name := strings.TrimSpace(frameName)
	if name == "" || strings.EqualFold(name, "_blank") {
		return ""
	}
	return name
}
