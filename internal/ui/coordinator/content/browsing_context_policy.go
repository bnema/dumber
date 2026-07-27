package content

import (
	"strings"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/rs/zerolog"
)

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
	if usecase.ReusableBrowsingContextName(req.FrameName) != "" {
		return dto.TriggerNamedTargetNavigation
	}
	switch inferPopupWindowDisposition(req) {
	case dto.WindowDispositionCurrentTab, dto.WindowDispositionNewTab:
		return dto.TriggerLinkNewPage
	case dto.WindowDispositionNewPopup, dto.WindowDispositionNewWindow:
		return dto.TriggerScriptWindowOpen
	default:
		return dto.TriggerUnknown
	}
}
