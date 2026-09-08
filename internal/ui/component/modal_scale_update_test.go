package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModalUIScaleUpdate(t *testing.T) {
	o := &Omnibox{uiScale: 1, sizeCfg: OmniboxSizeDefaults}
	o.measuredHeights.valid = true
	sm := &SessionManager{uiScale: 1}
	sm.measuredHeights.valid = true
	for _, scale := range []float64{1.5, 2, 1} {
		o.SetUIScale(scale)
		sm.SetUIScale(scale)
		assert.InDelta(t, scale, o.uiScale, 0.000001)
		assert.InDelta(t, scale, sm.uiScale, 0.000001)
		assert.False(t, o.measuredHeights.valid)
		assert.False(t, sm.measuredHeights.valid)
		width, _ := o.requestedDimensions()
		assert.Equal(t, int(640*scale), width)
	}
}

func TestWorkspaceOmniboxUIScaleUpdatePreservesConfig(t *testing.T) {
	o := &Omnibox{uiScale: 1}
	wv := &WorkspaceView{omnibox: o, omniboxCfg: OmniboxConfig{UIScale: 1, SizeConfig: ModalSizeConfig{FixedWidth: 800}}}
	wv.SetOmniboxUIScale(1.5)
	assert.InDelta(t, 1.5, o.uiScale, 0.000001)
	assert.InDelta(t, 1.5, wv.omniboxCfg.UIScale, 0.000001)
	assert.Equal(t, 800, wv.omniboxCfg.SizeConfig.FixedWidth)
	wv.omnibox = nil
	wv.SetOmniboxUIScale(2)
	assert.InDelta(t, 2.0, wv.omniboxCfg.UIScale, 0.000001)
}
