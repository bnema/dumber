package content

import (
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/assert"
)

func TestBrowsingContextPolicyDecide(t *testing.T) {
	t.Parallel()

	policy := browsingContextPolicy{}
	tests := []struct {
		name              string
		request           port.NewBrowsingContextRequest
		namedContextExist bool
		wantKind          port.HostDecisionKind
		wantName          string
	}{
		{
			name: "blank target stays pane-hosted",
			request: port.NewBrowsingContextRequest{
				TargetURI:                 "https://example.com/docs",
				TargetFrameName:           "_blank",
				TriggerKind:               port.TriggerLinkNewPage,
				TargetDisposition:         port.WindowDispositionNewTab,
				RequestContextDisposition: port.RequestContextInheritParent,
			},
			wantKind: port.HostDecisionCreatePane,
		},
		{
			name: "named target reuses existing pane in same window",
			request: port.NewBrowsingContextRequest{
				TargetURI:                 "https://example.com/second",
				TargetFrameName:           "shared-pane",
				TriggerKind:               port.TriggerNamedTargetNavigation,
				TargetDisposition:         port.WindowDispositionNewPopup,
				RequestContextDisposition: port.RequestContextInheritParent,
			},
			namedContextExist: true,
			wantKind:          port.HostDecisionReuseNamedPane,
			wantName:          "shared-pane",
		},
		{
			name: "ordinary window.open stays pane-hosted",
			request: port.NewBrowsingContextRequest{
				TargetURI:                 "https://example.com/help",
				TriggerKind:               port.TriggerScriptWindowOpen,
				TargetDisposition:         port.WindowDispositionNewPopup,
				NoJavaScriptAccess:        false,
				RequestContextDisposition: port.RequestContextInheritParent,
			},
			wantKind: port.HostDecisionCreatePane,
		},
		{
			name: "auth intent forces native window",
			request: port.NewBrowsingContextRequest{
				TargetURI:                 "https://accounts.google.com/o/oauth2/v2/auth",
				TriggerKind:               port.TriggerScriptWindowOpen,
				TargetDisposition:         port.WindowDispositionNewPopup,
				AuthIntent:                true,
				RequestContextDisposition: port.RequestContextInheritParent,
			},
			wantKind: port.HostDecisionCreateNativeWin,
		},
		{
			name: "ambiguous opener-coupled request prefers native",
			request: port.NewBrowsingContextRequest{
				TargetURI:                 "https://example.com/popup",
				TriggerKind:               port.TriggerUnknown,
				TargetDisposition:         port.WindowDispositionNewPopup,
				NoJavaScriptAccess:        false,
				RequestContextDisposition: port.RequestContextInheritParent,
			},
			wantKind: port.HostDecisionCreateNativeWin,
		},
		{
			name: "empty target is denied",
			request: port.NewBrowsingContextRequest{
				TargetURI:                 "",
				TriggerKind:               port.TriggerLinkNewPage,
				RequestContextDisposition: port.RequestContextInheritParent,
			},
			wantKind: port.HostDecisionDeny,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := policy.Decide(tt.request, tt.namedContextExist)
			assert.Equal(t, tt.wantKind, got.Kind)
			assert.Equal(t, tt.wantName, got.BrowsingContextName)
			assert.Equal(t, port.RequestContextInheritParent, got.RequestContextDisposition)
		})
	}
}

func TestInferPopupTriggerKind_DoesNotTreatNoJavaScriptAccessFalseAsNativeSignal(t *testing.T) {
	t.Parallel()

	req := port.PopupRequest{
		TargetURI:          "https://example.com/window-open",
		NoJavaScriptAccess: false,
		TargetDisposition:  port.WindowDispositionNewPopup,
	}

	assert.Equal(t, port.TriggerScriptWindowOpen, inferPopupTriggerKind(req))
}
