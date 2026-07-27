package usecase

import (
	"testing"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/stretchr/testify/assert"
)

func TestPopupFeaturesRequestHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		features dto.PopupFeatures
		want     bool
	}{
		{name: "known featureless", features: dto.PopupFeatures{State: dto.PopupFeaturesNone}},
		{name: "zero width", features: dto.PopupFeatures{State: dto.PopupFeaturesSpecified, Width: 0, WidthSet: true}},
		{name: "positive width", features: dto.PopupFeatures{State: dto.PopupFeaturesSpecified, Width: 1, WidthSet: true}, want: true},
		{name: "positive height", features: dto.PopupFeatures{State: dto.PopupFeaturesSpecified, Height: 1, HeightSet: true}, want: true},
		{name: "hidden toolbar", features: dto.PopupFeatures{State: dto.PopupFeaturesSpecified, ToolbarVisible: false, ToolbarVisibilitySet: true}, want: true},
		{name: "hidden location bar", features: dto.PopupFeatures{State: dto.PopupFeaturesSpecified, LocationbarVisible: false, LocationbarVisibilitySet: true}, want: true},
		{name: "disabled resize", features: dto.PopupFeatures{State: dto.PopupFeaturesSpecified, Resizable: false, ResizableSet: true}, want: true},
		{name: "engine popup flag", features: dto.PopupFeatures{State: dto.PopupFeaturesSpecified, IsPopup: true, IsPopupSet: true}, want: true},
		{name: "visible chrome", features: dto.PopupFeatures{State: dto.PopupFeaturesSpecified, ToolbarVisible: true, ToolbarVisibilitySet: true}},
		{name: "resizable", features: dto.PopupFeatures{State: dto.PopupFeaturesSpecified, Resizable: true, ResizableSet: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, popupFeaturesRequestHost(tt.features))
		})
	}
}

func TestBrowsingContextPolicyDecide(t *testing.T) {
	t.Parallel()

	workspace := dto.NewBrowsingContextRequest{SourceHost: dto.SourceHostWorkspace, TargetURI: "https://example.com/docs", TargetDisposition: dto.WindowDispositionNewTab, TriggerKind: dto.TriggerLinkNewPage}
	tests := []struct {
		name              string
		request           dto.NewBrowsingContextRequest
		namedContextExist bool
		wantKind          dto.HostDecisionKind
		wantName          string
		wantReason        dto.HostDecisionReasonCode
	}{
		{name: "workspace behavior remains pane hosted", request: workspace, wantKind: dto.HostDecisionCreatePane, wantReason: dto.HostDecisionReasonWorkspacePane},
		{name: "zero source preserves workspace behavior", request: dto.NewBrowsingContextRequest{TargetURI: "https://example.com/docs", TriggerKind: dto.TriggerLinkNewPage}, wantKind: dto.HostDecisionCreatePane, wantReason: dto.HostDecisionReasonWorkspacePane},
		{name: "floating blank target navigates source", request: dto.NewBrowsingContextRequest{SourceHost: dto.SourceHostFloating, TargetURI: "https://example.com/docs", TargetFrameName: "_blank", TargetDisposition: dto.WindowDispositionNewTab, TriggerKind: dto.TriggerLinkNewPage, PopupFeatures: dto.PopupFeatures{State: dto.PopupFeaturesNone}}, wantKind: dto.HostDecisionNavigateSource, wantReason: dto.HostDecisionReasonFloatingFeaturelessBlank},
		{name: "floating featureless script navigates source", request: dto.NewBrowsingContextRequest{SourceHost: dto.SourceHostFloating, TargetURI: "https://example.com/help", TargetFrameName: "_blank", TargetDisposition: dto.WindowDispositionNewPopup, TriggerKind: dto.TriggerScriptWindowOpen, PopupFeatures: dto.PopupFeatures{State: dto.PopupFeaturesNone}}, wantKind: dto.HostDecisionNavigateSource, wantReason: dto.HostDecisionReasonFloatingFeaturelessBlank},
		{name: "floating featured script opens native popup", request: dto.NewBrowsingContextRequest{SourceHost: dto.SourceHostFloating, TargetURI: "https://example.com/help", TargetDisposition: dto.WindowDispositionNewPopup, TriggerKind: dto.TriggerScriptWindowOpen, PopupFeatures: dto.PopupFeatures{State: dto.PopupFeaturesSpecified, Width: 640, WidthSet: true}}, wantKind: dto.HostDecisionCreateNativePopup, wantReason: dto.HostDecisionReasonFloatingPopupFeatures},
		{name: "floating unknown script waits", request: dto.NewBrowsingContextRequest{SourceHost: dto.SourceHostFloating, TargetURI: "https://example.com/help", TargetDisposition: dto.WindowDispositionNewPopup, TriggerKind: dto.TriggerScriptWindowOpen, PopupFeatures: dto.PopupFeatures{State: dto.PopupFeaturesUnknown}}, wantKind: dto.HostDecisionAwaitPopupFeatures, wantReason: dto.HostDecisionReasonFloatingFeaturesPending},
		{name: "floating named unknown waits", request: dto.NewBrowsingContextRequest{SourceHost: dto.SourceHostFloating, TargetURI: "https://example.com/help", TargetFrameName: "oauth", TargetDisposition: dto.WindowDispositionNewPopup, TriggerKind: dto.TriggerNamedTargetNavigation, PopupFeatures: dto.PopupFeatures{State: dto.PopupFeaturesUnknown}}, wantKind: dto.HostDecisionAwaitPopupFeatures, wantName: "oauth", wantReason: dto.HostDecisionReasonFloatingFeaturesPending},
		{name: "auth intent precedes floating", request: dto.NewBrowsingContextRequest{SourceHost: dto.SourceHostFloating, TargetURI: "https://example.com/auth", AuthIntent: true}, wantKind: dto.HostDecisionCreateNativePopup, wantReason: dto.HostDecisionReasonAuthNativePopup},
		{name: "native opener precedes floating", request: dto.NewBrowsingContextRequest{SourceHost: dto.SourceHostFloating, TargetURI: "https://example.com/open", RequiresNativeOpener: true}, wantKind: dto.HostDecisionCreateNativePopup, wantReason: dto.HostDecisionReasonNativeOpenerRequired},
		{name: "ambiguous workspace opener stays native", request: dto.NewBrowsingContextRequest{SourceHost: dto.SourceHostWorkspace, TargetURI: "https://example.com/ambiguous", TriggerKind: dto.TriggerUnknown}, wantKind: dto.HostDecisionCreateNativePopup, wantReason: dto.HostDecisionReasonAmbiguousOpenerNative},
		{name: "floating live named context is reused", request: dto.NewBrowsingContextRequest{SourceHost: dto.SourceHostFloating, TargetURI: "https://example.com/named", TargetFrameName: "shared", TriggerKind: dto.TriggerNamedTargetNavigation, PopupFeatures: dto.PopupFeatures{State: dto.PopupFeaturesNone}}, namedContextExist: true, wantKind: dto.HostDecisionReuseNamedPane, wantName: "shared", wantReason: dto.HostDecisionReasonNamedContextReuse},
		{name: "floating noopener skips named reuse", request: dto.NewBrowsingContextRequest{SourceHost: dto.SourceHostFloating, TargetURI: "https://example.com/named", TargetFrameName: "shared", TriggerKind: dto.TriggerNamedTargetNavigation, NoJavaScriptAccess: true, PopupFeatures: dto.PopupFeatures{State: dto.PopupFeaturesNone}}, namedContextExist: true, wantKind: dto.HostDecisionCreateBrowserWindow, wantName: "shared", wantReason: dto.HostDecisionReasonFloatingBrowserWindow},
		{name: "empty target is denied first", request: dto.NewBrowsingContextRequest{SourceHost: dto.SourceHostFloating, AuthIntent: true}, wantKind: dto.HostDecisionDeny, wantReason: dto.HostDecisionReasonEmptyTarget},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := (BrowsingContextPolicy{}).Decide(tt.request, tt.namedContextExist)
			assert.Equal(t, tt.wantKind, got.Kind)
			assert.Equal(t, tt.wantName, got.BrowsingContextName)
			assert.Equal(t, tt.wantReason, got.ReasonCode)
			assert.NotEmpty(t, got.Reason)
			assert.Equal(t, dto.RequestContextInheritParent, got.RequestContextDisposition)
		})
	}
}
