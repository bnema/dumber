package content

import (
	"testing"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/assert"
)

func TestBuildPopupBrowsingContextRequestCarriesNormalizedMetadata(t *testing.T) {
	t.Parallel()

	features := dto.PopupFeatures{State: dto.PopupFeaturesSpecified, Width: 320, WidthSet: true}
	got := buildPopupBrowsingContextRequest(port.PopupRequest{Engine: dto.BrowserEngineCEF, PopupFeatures: features, TargetURI: "https://example.com", ParentViewID: 42})
	assert.Equal(t, dto.BrowserEngineCEF, got.Engine)
	assert.Equal(t, features, got.PopupFeatures)
}

func TestInferPopupTriggerKind(t *testing.T) {
	t.Parallel()
	assert.Equal(t, dto.TriggerScriptWindowOpen, inferPopupTriggerKind(port.PopupRequest{TargetURI: "https://example.com/window-open", TargetDisposition: dto.WindowDispositionNewPopup}))
	assert.Equal(t, dto.TriggerLinkNewPage, inferPopupTriggerKind(port.PopupRequest{TargetDisposition: dto.WindowDispositionCurrentTab}))
	assert.Equal(t, dto.TriggerNamedTargetNavigation, inferPopupTriggerKind(port.PopupRequest{FrameName: "shared", TargetDisposition: dto.WindowDispositionCurrentTab}))
}

func TestInferPopupWindowDisposition(t *testing.T) {
	t.Parallel()
	assert.Equal(t, dto.WindowDispositionNewTab, inferPopupWindowDisposition(port.PopupRequest{FrameName: " _BlAnK "}))
	assert.Equal(t, dto.WindowDispositionNewPopup, inferPopupWindowDisposition(port.PopupRequest{FrameName: "shared-pane"}))
}
