package ui

import (
	"context"
	"sync"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
)

func (a *App) modeFrameTarget(bw *browserWindow, mode input.Mode) layout.Widget {
	wsView := a.activeWorkspaceViewForBrowserWindow(bw)
	if wsView == nil {
		return nil
	}
	if mode == input.ModeTab || mode == input.ModeSession || mode == input.ModePrefix {
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
	// Rebuilds can temporarily lack a pane widget. Drop the old border while
	// the next focus/rebuild event retries the target.
	bw.modeFrame.setTarget(a.modeFrameTarget(bw, mode), modeFrameClass(mode))
	if bw.modeFrame.target != nil {
		bw.modeFrame.refreshGeometry()
	}
}

// updateModeFrame reconciles the window's mode border and mode toaster with
// mode and the live configuration.
func (a *App) updateModeFrame(bw *browserWindow, mode input.Mode) {
	if bw == nil {
		return
	}
	a.setModeFrameTab(bw)
	cfg := a.runtimeConfigSnapshot().UI
	if bw.modeFrame != nil {
		bw.modeFrame.setMode(mode, a.modeFrameTarget(bw, mode))
	}
	if bw.modeToaster != nil {
		styling := cfg.Workspace.Styling
		styling.ModeLegendCollapsed = a.modeLegendCollapsed()
		bw.modeToaster.apply(mode, styling, cfg.Omnibox.Style, legendActions(mode, cfg.Workspace, cfg.Session))
	}
}

func (a *App) setModeFrameTab(bw *browserWindow) {
	if bw.modeFrame == nil {
		return
	}
	tabID := bw.modeFrame.tabID
	bw.modeFrame.tabID = ""
	if tab := a.activeTabForBrowserWindow(bw); tab != nil {
		bw.modeFrame.tabID = tab.ID
	}
	if tabID != bw.modeFrame.tabID {
		bw.modeToaster.dismissLinger()
	}
}

func (a *App) modeFrameAction(bw *browserWindow, action input.Action) {
	if bw != nil && bw.modeToaster != nil {
		bw.modeToaster.flash(action)
	}
}

// modeLegendPreference is the app-wide "legend collapsed" state, shared by
// every mode and window. A click is applied at once and saved in the
// background. It keeps priority over the configured value, which may still be
// the previous one while saves are in flight, until a configuration reload
// received after the writer drained settles it (see configReloaded).
type modeLegendPreference struct {
	mu        sync.Mutex
	collapsed bool // last click; meaningful while clicked is set
	clicked   bool
	// pending is the next value to save; saving reports a running writer. The
	// writer drains pending until it is empty, so saves go out in click order
	// and the file ends on the last click.
	pending *bool
	saving  bool
}

// current returns the state to show, given the configured one.
func (p *modeLegendPreference) current(configured bool) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.clicked {
		return p.collapsed
	}
	return configured
}

// click records a click and returns whether a writer must be started.
func (p *modeLegendPreference) click(collapsed bool) (startWriter bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clicked, p.collapsed = true, collapsed
	p.pending = &collapsed
	if p.saving {
		return false
	}
	p.saving = true
	return true
}

// next hands the writer its next value, or ends it when none is left.
func (p *modeLegendPreference) next() (collapsed, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending == nil {
		p.saving = false
		return false, false
	}
	collapsed, p.pending = *p.pending, nil
	return collapsed, true
}

// configReloaded hands the state back to the configuration on a reload
// received once the writer has drained: that reload reflects the saved clicks
// (or an external edit, or a failed save). Reloads received while the writer
// runs are ignored: they may come from an earlier save and would undo the
// latest click.
func (p *modeLegendPreference) configReloaded(_ bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.saving {
		p.clicked = false
	}
}

// modeLegendCollapsed returns the state new windows and mode changes start from.
func (a *App) modeLegendCollapsed() bool {
	return a.modeLegend.current(a.runtimeConfigSnapshot().UI.Workspace.Styling.ModeLegendCollapsed)
}

// setModeLegendCollapsed applies a click on a mode toaster: it updates every
// open window at once, then saves the choice off the GTK thread. A failed save
// is logged; the state stays applied until the next configuration reload.
func (a *App) setModeLegendCollapsed(ctx context.Context, collapsed bool) {
	for _, bw := range a.browserWindows {
		if bw != nil && bw.modeToaster != nil {
			bw.modeToaster.setCollapsed(collapsed)
		}
	}
	start := a.modeLegend.click(collapsed)
	if !start {
		return
	}
	var save func(context.Context, bool) error
	if a.deps != nil {
		save = a.deps.HandlerDeps.SaveModeLegendCollapsed
	}
	go func() {
		for {
			next, ok := a.modeLegend.next()
			if !ok {
				return
			}
			if save == nil {
				continue
			}
			if err := save(ctx, next); err != nil {
				logging.FromContext(ctx).Warn().Err(err).Bool("collapsed", next).
					Msg("failed to save mode legend collapsed preference")
			}
		}
	}()
}
