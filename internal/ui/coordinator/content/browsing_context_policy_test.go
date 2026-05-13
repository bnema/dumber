package content

import (
	"testing"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/assert"
)

func TestBrowsingContextPolicyDecide(t *testing.T) {
	t.Parallel()

	policy := browsingContextPolicy{}
	tests := []struct {
		name              string
		request           dto.NewBrowsingContextRequest
		namedContextExist bool
		wantKind          dto.HostDecisionKind
		wantName          string
	}{
		{
			name: "blank target stays pane-hosted",
			request: dto.NewBrowsingContextRequest{
				TargetURI:                 "https://example.com/docs",
				TargetFrameName:           "_blank",
				TriggerKind:               dto.TriggerLinkNewPage,
				TargetDisposition:         dto.WindowDispositionNewTab,
				RequestContextDisposition: dto.RequestContextInheritParent,
			},
			wantKind: dto.HostDecisionCreatePane,
		},
		{
			name: "named target reuses existing pane in same window",
			request: dto.NewBrowsingContextRequest{
				TargetURI:                 "https://example.com/second",
				TargetFrameName:           "shared-pane",
				TriggerKind:               dto.TriggerNamedTargetNavigation,
				TargetDisposition:         dto.WindowDispositionNewPopup,
				RequestContextDisposition: dto.RequestContextInheritParent,
			},
			namedContextExist: true,
			wantKind:          dto.HostDecisionReuseNamedPane,
			wantName:          "shared-pane",
		},
		{
			name: "ordinary window.open stays pane-hosted",
			request: dto.NewBrowsingContextRequest{
				TargetURI:                 "https://example.com/help",
				TriggerKind:               dto.TriggerScriptWindowOpen,
				TargetDisposition:         dto.WindowDispositionNewPopup,
				NoJavaScriptAccess:        false,
				RequestContextDisposition: dto.RequestContextInheritParent,
			},
			wantKind: dto.HostDecisionCreatePane,
		},
		{
			name: "auth intent forces native window",
			request: dto.NewBrowsingContextRequest{
				TargetURI:                 "https://accounts.google.com/o/oauth2/v2/auth",
				TriggerKind:               dto.TriggerScriptWindowOpen,
				TargetDisposition:         dto.WindowDispositionNewPopup,
				AuthIntent:                true,
				RequestContextDisposition: dto.RequestContextInheritParent,
			},
			wantKind: dto.HostDecisionCreateNativeWin,
		},
		{
			name: "ambiguous opener-coupled request prefers native",
			request: dto.NewBrowsingContextRequest{
				TargetURI:                 "https://example.com/popup",
				TriggerKind:               dto.TriggerUnknown,
				TargetDisposition:         dto.WindowDispositionNewPopup,
				NoJavaScriptAccess:        false,
				RequestContextDisposition: dto.RequestContextInheritParent,
			},
			wantKind: dto.HostDecisionCreateNativeWin,
		},
		{
			name: "empty target is denied",
			request: dto.NewBrowsingContextRequest{
				TargetURI:                 "",
				TriggerKind:               dto.TriggerLinkNewPage,
				RequestContextDisposition: dto.RequestContextInheritParent,
			},
			wantKind: dto.HostDecisionDeny,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := policy.Decide(tt.request, tt.namedContextExist)
			assert.Equal(t, tt.wantKind, got.Kind)
			assert.Equal(t, tt.wantName, got.BrowsingContextName)
			assert.Equal(t, dto.RequestContextInheritParent, got.RequestContextDisposition)
		})
	}
}

func TestInferPopupTriggerKind_DoesNotTreatNoJavaScriptAccessFalseAsNativeSignal(t *testing.T) {
	t.Parallel()

	req := port.PopupRequest{
		TargetURI:          "https://example.com/window-open",
		NoJavaScriptAccess: false,
		TargetDisposition:  dto.WindowDispositionNewPopup,
	}

	assert.Equal(t, dto.TriggerScriptWindowOpen, inferPopupTriggerKind(req))
}

func TestInferPopupTriggerKind_ClassifiesCurrentTabAsNavigation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  port.PopupRequest
		want dto.TriggerKind
	}{
		{
			name: "unnamed current-tab request stays a navigation",
			req: port.PopupRequest{
				TargetURI:         "https://example.com/docs",
				TargetDisposition: dto.WindowDispositionCurrentTab,
			},
			want: dto.TriggerLinkNewPage,
		},
		{
			name: "named current-tab request is treated as named target navigation",
			req: port.PopupRequest{
				TargetURI:         "https://example.com/docs",
				FrameName:         "shared-pane",
				TargetDisposition: dto.WindowDispositionCurrentTab,
			},
			want: dto.TriggerNamedTargetNavigation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, inferPopupTriggerKind(tt.req))
		})
	}
}

func TestInferPopupWindowDisposition_NormalizesBlankTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  port.PopupRequest
		want dto.WindowDisposition
	}{
		{
			name: "mixed-case blank target is treated as new tab",
			req:  port.PopupRequest{FrameName: " _BlAnK "},
			want: dto.WindowDispositionNewTab,
		},
		{
			name: "named target still defaults to popup",
			req:  port.PopupRequest{FrameName: "shared-pane"},
			want: dto.WindowDispositionNewPopup,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, inferPopupWindowDisposition(tt.req))
		})
	}
}
