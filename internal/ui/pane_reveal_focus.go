package ui

import (
	"bnema.dev/dumber/internal/domain/entity"
	"bnema.dev/dumber/internal/ui/component"
	"bnema.dev/puregotk/v4/gtk"
)

// focusRevealedActivePane gives keyboard focus to a pane whose WebView was
// just revealed, when it is the active pane of its window's active workspace.
//
// WebView widgets stay hidden until their first paint and GTK refuses to
// focus a hidden widget, so the FocusPane calls made when the pane was
// created do nothing on WebKit (CEF's widget is focusable earlier). Focus
// only moves when it is unset, on another pane's WebView, or on a layout
// container above this pane (where a workspace rebuild leaves it), so an
// open omnibox, find bar, sidebar or dialog keeps its focus.
func (a *App) focusRevealedActivePane(paneID entity.PaneID) {
	bw := a.browserWindowForPane(paneID)
	if bw == nil || bw.mainWindow == nil || bw.mainWindow.Window() == nil {
		return
	}
	ws := a.activeWorkspaceForBrowserWindow(bw)
	wsView := a.activeWorkspaceViewForBrowserWindow(bw)
	if ws == nil || wsView == nil || ws.ActivePaneID != paneID {
		return
	}
	pv := wsView.GetPaneView(paneID)
	if pv == nil || pv.HasFocus() {
		return
	}
	if !focusMayMoveToPane(bw.mainWindow.Window().GetFocus(), pv, wsView) {
		return
	}
	wsView.FocusPane(paneID)
}

func focusMayMoveToPane(focus *gtk.Widget, pv *component.PaneView, wsView *component.WorkspaceView) bool {
	if focus == nil {
		return true
	}
	if w := pv.WebViewWidget(); w != nil && w.GtkWidget() != nil && w.GtkWidget().IsAncestor(focus) {
		return true
	}
	for _, id := range wsView.GetPaneIDs() {
		if other := wsView.GetPaneView(id); other != nil && other.HasFocus() {
			return true
		}
	}
	return false
}
