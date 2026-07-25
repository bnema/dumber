package webkit

import (
	"testing"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/stretchr/testify/assert"
)

func TestResolvePopupFeatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		x, y, width, height int
		toolbar, location   bool
		resizable           bool
		wantState           dto.PopupFeatureState
		wantPopup           bool
	}{
		{name: "default properties are featureless", toolbar: true, location: true, resizable: true, wantState: dto.PopupFeaturesNone},
		{name: "geometry is popup like", width: 640, height: 480, toolbar: true, location: true, resizable: true, wantState: dto.PopupFeaturesSpecified, wantPopup: true},
		{name: "hidden toolbar is popup like", toolbar: false, location: true, resizable: true, wantState: dto.PopupFeaturesSpecified, wantPopup: true},
		{name: "hidden location bar is popup like", toolbar: true, location: false, resizable: true, wantState: dto.PopupFeaturesSpecified, wantPopup: true},
		{name: "disabled resizing is popup like", toolbar: true, location: true, resizable: false, wantState: dto.PopupFeaturesSpecified, wantPopup: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := popupFeaturesFromWindowProperties(tt.x, tt.y, tt.width, tt.height, tt.toolbar, tt.location, tt.resizable)
			assert.Equal(t, tt.wantState, got.State)
			assert.Equal(t, tt.wantPopup, got.RequestsPopupHost())
			assert.Equal(t, tt.width > 0, got.WidthSet)
			assert.Equal(t, tt.height > 0, got.HeightSet)
			assert.True(t, got.ToolbarVisibilitySet)
			assert.True(t, got.LocationbarVisibilitySet)
			assert.True(t, got.ResizableSet)
			assert.Equal(t, tt.wantPopup && (!tt.toolbar || !tt.location || !tt.resizable), got.IsPopup)
		})
	}
}

func TestMapPopupRequestClassifiesBlankAndDefersAmbiguousScript(t *testing.T) {
	t.Parallel()

	blank := mapPopupRequest(PopupRequest{TargetURI: "https://example.com/new", FrameName: " _BLANK ", ParentID: 7})
	assert.Equal(t, dto.BrowserEngineWebKit, blank.Engine)
	assert.Equal(t, dto.WindowDispositionNewTab, blank.TargetDisposition)
	assert.Equal(t, dto.PopupFeaturesNone, blank.PopupFeatures.State)

	script := mapPopupRequest(PopupRequest{TargetURI: "https://example.com/popup", FrameName: "oauth", ParentID: 7})
	assert.Equal(t, dto.BrowserEngineWebKit, script.Engine)
	assert.Equal(t, dto.PopupFeaturesUnknown, script.PopupFeatures.State)
}

func TestResolvePopupFeaturesNilWebViewIsUnknown(t *testing.T) {
	t.Parallel()
	var wv *WebView
	assert.Equal(t, dto.PopupFeaturesUnknown, wv.ResolvePopupFeatures().State)
}
