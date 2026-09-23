package ui

import (
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
)

func (a *App) modeFrameTarget(bw *browserWindow, mode input.Mode) layout.Widget {
	wsView := a.activeWorkspaceViewForBrowserWindow(bw)
	if wsView == nil {
		return nil
	}
	if mode == input.ModeTab || mode == input.ModeSession {
		return wsView.Widget()
	}
	ws := a.activeWorkspaceForBrowserWindow(bw)
	if ws == nil {
		return nil
	}
	if mode == input.ModePane || mode == input.ModeResize {
		if stack := wsView.GetStackContainerWidget(ws.ActivePaneID); stack != nil {
			return stack
		}
	}
	if mode == input.ModeVim || mode == input.ModePane || mode == input.ModeResize {
		return wsView.GetPaneWidget(ws.ActivePaneID)
	}
	return nil
}

func (a *App) retargetModeFrame(bw *browserWindow, mode input.Mode) {
	if bw == nil || bw.modeFrame == nil {
		return
	}
	a.setModeFrameTab(bw)
	newTarget := a.modeFrameTarget(bw, mode)
	// Rebuilds can temporarily lack a pane widget. Drop the old border while
	// the next focus/rebuild event retries the target.
	bw.modeFrame.setTarget(newTarget, modeFrameClass(mode))
	if bw.modeFrame.visible {
		bw.modeFrame.refreshGeometry()
	}
}

func (a *App) updateModeFrame(bw *browserWindow, mode input.Mode) {
	if bw == nil || bw.modeFrame == nil {
		return
	}
	a.setModeFrameTab(bw)
	cfg := a.runtimeConfigSnapshot().UI
	bw.modeFrame.setMode(mode, a.modeFrameTarget(bw, mode), cfg.Workspace.Styling,
		cfg.Omnibox.Style, legendActions(mode, cfg.Workspace, cfg.Session))
}

func (a *App) setModeFrameTab(bw *browserWindow) {
	tabID := bw.modeFrame.tabID
	bw.modeFrame.tabID = ""
	if tab := a.activeTabForBrowserWindow(bw); tab != nil {
		bw.modeFrame.tabID = tab.ID
	}
	if tabID != bw.modeFrame.tabID {
		bw.modeFrame.dismissLinger()
	}
}

func (a *App) modeFrameAction(bw *browserWindow, action input.Action) {
	if bw != nil && bw.modeFrame != nil {
		bw.modeFrame.flash(action)
	}
}
