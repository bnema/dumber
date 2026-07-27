package content

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bnema/dumber/internal/domain/entity"
)

// ---------------------------------------------------------------------------
// DetectPopupType
// ---------------------------------------------------------------------------

func TestDetectPopupType_BlankIsTab(t *testing.T) {
	t.Parallel()

	assert.Equal(t, PopupTypeTab, DetectPopupType("_blank"))
}

func TestDetectPopupType_EmptyIsPopup(t *testing.T) {
	t.Parallel()

	assert.Equal(t, PopupTypePopup, DetectPopupType(""))
}

func TestDetectPopupType_NamedFrameIsPopup(t *testing.T) {
	t.Parallel()

	assert.Equal(t, PopupTypePopup, DetectPopupType("myFrame"))
}

// ---------------------------------------------------------------------------
// PopupType.String()
// ---------------------------------------------------------------------------

func TestPopupTypeString_Tab(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "tab", PopupTypeTab.String())
}

func TestPopupTypeString_Popup(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "popup", PopupTypePopup.String())
}

func TestPopupTypeString_Unknown(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "unknown", PopupType(99).String())
}

// ---------------------------------------------------------------------------
// GetBehavior
// ---------------------------------------------------------------------------

func TestGetBehavior_NilConfigReturnsSplit(t *testing.T) {
	t.Parallel()

	assert.Equal(t, entity.PopupBehaviorSplit, GetBehavior(PopupTypeTab, nil))
	assert.Equal(t, entity.PopupBehaviorSplit, GetBehavior(PopupTypePopup, nil))
}

func TestGetBehavior_TabPopup_DefaultConfig(t *testing.T) {
	t.Parallel()

	// BlankTargetBehavior is empty → falls through to default "stacked"
	cfg := &entity.BrowsingContextConfig{}
	assert.Equal(t, entity.PopupBehaviorStacked, GetBehavior(PopupTypeTab, cfg))
}

func TestGetBehavior_TabPopup_SplitConfig(t *testing.T) {
	t.Parallel()

	cfg := &entity.BrowsingContextConfig{BlankTargetBehavior: "split"}
	assert.Equal(t, entity.PopupBehaviorSplit, GetBehavior(PopupTypeTab, cfg))
}

func TestGetBehavior_TabPopup_StackedConfig(t *testing.T) {
	t.Parallel()

	cfg := &entity.BrowsingContextConfig{BlankTargetBehavior: "stacked"}
	assert.Equal(t, entity.PopupBehaviorStacked, GetBehavior(PopupTypeTab, cfg))
}

func TestGetBehavior_TabPopup_TabbedConfig(t *testing.T) {
	t.Parallel()

	cfg := &entity.BrowsingContextConfig{BlankTargetBehavior: "tabbed"}
	assert.Equal(t, entity.PopupBehaviorTabbed, GetBehavior(PopupTypeTab, cfg))
}

func TestGetBehavior_JSPopup_UsesBehaviorField(t *testing.T) {
	t.Parallel()

	cfg := &entity.BrowsingContextConfig{Behavior: entity.PopupBehaviorWindowed}
	assert.Equal(t, entity.PopupBehaviorWindowed, GetBehavior(PopupTypePopup, cfg))
}

func TestGetBehavior_JSPopup_DefaultConfig(t *testing.T) {
	t.Parallel()

	// Behavior zero-value is empty string; GetBehavior returns it as-is.
	cfg := &entity.BrowsingContextConfig{}
	assert.Equal(t, entity.PopupBehavior(""), GetBehavior(PopupTypePopup, cfg))
}

func TestGetBehavior_TabPopup_BlocksWhenOpenInNewPaneFalse(t *testing.T) {
	t.Parallel()

	// GetBehavior itself does not honor OpenInNewPane — that is enforced by
	// handlePopupCreate. GetBehavior still returns the configured value.
	cfg := &entity.BrowsingContextConfig{
		OpenInNewPane:       false,
		BlankTargetBehavior: "split",
	}
	assert.Equal(t, entity.PopupBehaviorSplit, GetBehavior(PopupTypeTab, cfg))
}
