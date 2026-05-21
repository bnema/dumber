package cef

import (
	"testing"

	"github.com/bnema/puregotk/v4/gtk"
	"github.com/stretchr/testify/require"
)

func TestWebViewBridgeInputOptions_LeavesTargetSelectionToAdapter(t *testing.T) {
	nativeWidget := &gtk.Widget{}
	wv := &WebView{nativeWidget: nativeWidget}

	opts := wv.bridgeInputOptions()

	require.Zero(t, opts.Scale)
	require.NotNil(t, opts.OnMiddleClick)
	require.NotNil(t, opts.SelectionText)
	require.NotNil(t, opts.OnClipboardShortcut)
	require.Same(t, nativeWidget, wv.nativeWidget)
}
