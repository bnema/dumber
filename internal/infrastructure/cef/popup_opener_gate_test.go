package cef

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPopupOpenerNavigationNeverMovesARegularPageToASystemView(t *testing.T) {
	const web = "https://example.com/app"
	assert.Empty(t, resolvePopupOpenerNavigationTarget("https://dumber.invalid/history", web))
	assert.Empty(t, resolvePopupOpenerNavigationTarget("https://DUMBER.INVALID:443/config", web))
	assert.Equal(t, "https://example.com/next", resolvePopupOpenerNavigationTarget("/next", web))
	assert.Equal(t, "https://dumber.invalid/config",
		resolvePopupOpenerNavigationTarget("https://dumber.invalid/config", "https://dumber.invalid/home"),
		"a system view opener may move between system views")
}

func TestPopupFromSystemViewSubframe(t *testing.T) {
	assert.True(t, popupFromSystemViewSubframe(false, "https://dumber.invalid/home"))
	assert.False(t, popupFromSystemViewSubframe(true, "https://dumber.invalid/home"))
	assert.False(t, popupFromSystemViewSubframe(false, "https://example.com/"))
}

func TestDeniedPopupOpenerBridgeStaysDisabled(t *testing.T) {
	parent := &WebView{ctx: context.Background(), uri: "https://dumber.invalid/home"}
	popup := &WebView{ctx: context.Background()}
	popup.EnablePopupOpenerBridge(parent, false)
	require.Same(t, parent, popup.popupOpenerBridgeParent)

	popup.denyPopupOpenerBridge()
	require.Nil(t, popup.popupOpenerBridgeParent)

	popup.EnablePopupOpenerBridge(parent, false)
	assert.Nil(t, popup.popupOpenerBridgeParent, "a later enable must not restore the bridge")
	assert.Empty(t, popup.popupOpenerBridgeParentURI)
}

func TestSyntheticPopupToSystemViewIsRefusedFromRegularPages(t *testing.T) {
	web := &WebView{ctx: context.Background(), uri: "https://example.com/"}
	assert.True(t, web.rejectSyntheticPopupTarget("https://dumber.invalid/history"))
	assert.True(t, web.rejectSyntheticPopupTarget("dumb://config"))
	assert.False(t, web.rejectSyntheticPopupTarget("https://example.org/"))

	sys := &WebView{ctx: context.Background(), uri: "https://dumber.invalid/home"}
	assert.False(t, sys.rejectSyntheticPopupTarget("dumb://config"))
}
