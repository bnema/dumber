package ui

import (
	"context"
	"time"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/input"
)

func (a *App) vimModePolicy() *usecase.VimModePolicyUseCase {
	if a == nil {
		return usecase.NewVimModePolicyUseCase()
	}
	if a.vimModePolicyUC == nil {
		a.vimModePolicyUC = usecase.NewVimModePolicyUseCase()
	}
	return a.vimModePolicyUC
}

func (a *App) pageEditableFocused(paneID entity.PaneID) bool {
	return a != nil && a.vimMode.editableFocused(paneID)
}

func (a *App) setPageEditableFocused(paneID entity.PaneID, editable bool) {
	if a != nil {
		a.vimMode.setEditableFocused(paneID, editable)
	}
}

func (a *App) clearPageEditableFocusState(paneID entity.PaneID) {
	a.setPageEditableFocused(paneID, false)
}

func (a *App) vimModeActiveForBrowserWindow(bw *browserWindow) bool {
	return bw != nil && bw.keyboardHandler != nil && bw.keyboardHandler.Mode() == input.ModeVim
}

func (a *App) applyVimModePolicyTransition(_ context.Context, bw *browserWindow, transition usecase.VimModePolicyTransition) {
	if transition != usecase.VimModePolicyTransitionExit || bw == nil || bw.keyboardHandler == nil {
		return
	}
	if bw.keyboardHandler.Mode() != input.ModeVim {
		return
	}
	bw.keyboardHandler.ExitMode()
}

func (a *App) handleVimModeFocusTrigger(ctx context.Context, bw *browserWindow, trigger usecase.VimModePolicyTrigger) {
	if bw == nil {
		return
	}
	transition := a.vimModePolicy().Evaluate(usecase.VimModePolicyInput{
		Trigger:       trigger,
		VimModeActive: a.vimModeActiveForBrowserWindow(bw),
	})
	a.applyVimModePolicyTransition(ctx, bw, transition)
}

func (a *App) handleVimModeTabSwitch(_ context.Context, bw *browserWindow) {
	if bw == nil || bw.tabs == nil || !a.vimModeActiveForBrowserWindow(bw) {
		return
	}
	transition := a.vimModePolicy().Evaluate(usecase.VimModePolicyInput{
		Trigger:                 usecase.VimModePolicyTriggerContextChanged,
		VimModeActive:           true,
		PreserveOnContextChange: false,
	})
	if transition != usecase.VimModePolicyTransitionExit {
		return
	}
	if prevTabID := bw.tabs.PreviousActiveTabID; prevTabID != "" {
		if prevView := a.workspaceViews[prevTabID]; prevView != nil {
			if pv := prevView.GetPaneView(bw.vimModePaneID); pv != nil {
				pv.SetVimMode(false)
			}
		}
	}
	bw.vimModePaneID = ""
	bw.keyboardHandler.ExitMode()
}

func (a *App) handlePageEditableFocusChanged(ctx context.Context, paneID entity.PaneID, editable bool) {
	if paneID == "" {
		return
	}
	a.setPageEditableFocused(paneID, editable)

	bw := a.browserWindowForAnyPane(paneID)
	if bw == nil {
		return
	}
	ws := a.activeWorkspaceForBrowserWindow(bw)
	activeContext := ws != nil && ws.ActivePaneID == paneID && a.lastFocusedBrowserWindow() == bw
	transition := a.vimModePolicy().Evaluate(usecase.VimModePolicyInput{
		Trigger:              usecase.VimModePolicyTriggerPageEditableFocusChanged,
		VimModeActive:        a.vimModeActiveForBrowserWindow(bw),
		PageEditableFocused:  editable,
		EventInActiveContext: activeContext,
	})
	a.applyVimModePolicyTransition(ctx, bw, transition)
}

func (*App) shouldBypassVimModeActivation(_ *browserWindow) bool {
	return false
}

// handleModeChange is called when the input mode changes for a specific browser window.
func (a *App) handleModeChange(ctx context.Context, bw *browserWindow, from, to input.Mode) {
	log := logging.FromContext(ctx)
	log.Debug().Str("from", from.String()).Str("to", to.String()).Msg("input mode changed")

	if to == input.ModeVim && from != input.ModeVim {
		a.enableAccessibilityForVimMode(ctx, bw)
	}
	if from == input.ModeVim && to != input.ModeVim {
		a.clearVimNavigationHighlight(ctx, bw)
	}

	// Handle pane-local Vim Mode visual ownership.
	// Entering Vim Mode accents the active pane; leaving removes the accent.
	a.handleVimModeOwnership(ctx, bw, to, from)

	a.updateModeFrame(bw, to)

	// Show/hide this window's mode indicator toaster based on mode and config.
	a.updateModeIndicatorToaster(ctx, bw, to)
}

// transferVimModeOwnershipToPane transfers the pane-local Vim Mode accent
// from the current owning pane to another pane in the same browser window.
// The transfer only happens while that window is in Vim Mode.
func (a *App) transferVimModeOwnershipToPane(ctx context.Context, bw *browserWindow, newPaneID entity.PaneID) {
	if bw == nil {
		return
	}

	// Only transfer if vim mode is currently active and the owner is changing.
	if bw.vimModePaneID == "" || bw.vimModePaneID == newPaneID {
		return
	}

	// Check if this window is actually in vim mode.
	if bw.keyboardHandler == nil || bw.keyboardHandler.Mode() != input.ModeVim {
		// Vim mode not active on this window; just clear stale ownership.
		bw.vimModePaneID = ""
		return
	}

	wsView := a.activeWorkspaceViewForBrowserWindow(bw)
	if wsView == nil {
		return
	}

	transition := a.vimModePolicy().Evaluate(usecase.VimModePolicyInput{
		Trigger:                 usecase.VimModePolicyTriggerContextChanged,
		VimModeActive:           true,
		PreserveOnContextChange: !a.pageEditableFocused(newPaneID),
	})
	if transition == usecase.VimModePolicyTransitionExit {
		a.applyVimModePolicyTransition(ctx, bw, transition)
		return
	}

	oldPaneID := bw.vimModePaneID

	// Deactivate the old owning pane.
	if oldPV := wsView.GetPaneView(oldPaneID); oldPV != nil {
		oldPV.SetVimMode(false)
	}

	// Activate the new pane.
	if newPV := wsView.GetPaneView(newPaneID); newPV != nil {
		newPV.SetVimMode(true)
	}
	bw.vimModePaneID = newPaneID

	logging.FromContext(ctx).Debug().
		Str("window_id", bw.id).
		Str("old_pane_id", string(oldPaneID)).
		Str("new_pane_id", string(newPaneID)).
		Msg("vim mode ownership transferred")
}

// handleVimModeOwnership manages the pane-local Vim Mode accent and pulse
// owner for a specific browser window when input modes change.
func (a *App) handleVimModeOwnership(ctx context.Context, bw *browserWindow, to, from input.Mode) {
	if bw == nil {
		return
	}

	ws := a.activeWorkspaceForBrowserWindow(bw)
	wsView := a.activeWorkspaceViewForBrowserWindow(bw)
	if ws == nil || wsView == nil {
		// If there's no workspace yet (startup), just track the intent.
		if to != input.ModeVim && from == input.ModeVim {
			bw.vimModePaneID = ""
		}
		return
	}

	if to == input.ModeVim && from != input.ModeVim {
		// Entering vim mode: activate on the current active pane of this window.
		paneID := ws.ActivePaneID
		if pv := wsView.GetPaneView(paneID); pv != nil {
			pv.SetVimMode(true)
			bw.vimModePaneID = paneID
			logging.FromContext(ctx).Debug().
				Str("window_id", bw.id).
				Str("pane_id", string(paneID)).
				Msg("vim mode activated on pane")
		}
	} else if from == input.ModeVim && to != input.ModeVim {
		// Leaving Vim Mode: deactivate the pane that owns the visual accent.
		a.clearVimModeOwnership(ctx, bw)
	}
}

// triggerVimModePulse triggers a pane-local vim mode pulse on the
// last-focused browser window's owning pane. This is called by the
// keyboard dispatcher after a page scroll action. The bw used is the
// window that was active when the action was dispatched (via
// dispatchBrowserWindowAction which calls activateBrowserWindow first).
// If no pane is currently in vim mode, the pulse is a no-op.
//
// Pulses are debounced at vimModePulseInterval to prevent excessive GTK CSS
// class churn during held-key repeats. Smooth scroll cadence is owned by the
// Vim Mode repeater and backend scroll path; pulse feedback is deliberately
// slower so indicator/overlay animation work does not become part of the
// scrolling critical path.
func (a *App) triggerVimModePulse(_ context.Context, fast bool) {
	// Debounce: skip if called again too soon during a continuous hold.
	// We still serialize the timestamp update so repeated dispatcher calls do
	// not restart CSS animations at scroll cadence.
	if !a.vimMode.allowPulse(time.Now()) {
		return
	}

	bw := a.lastFocusedBrowserWindow()
	if bw == nil || bw.vimModePaneID == "" {
		return
	}
	if bw.modeFrame != nil {
		bw.modeFrame.pulse(fast)
	}
}

// clearVimModeOwnership deactivates vim mode on the owning pane for a
// specific browser window and resets its tracked pane ID.
// Also resets the pulse debounce timer so the first pulse on re-entry
// is never skipped.
func (a *App) clearVimModeOwnership(ctx context.Context, bw *browserWindow) {
	if bw == nil || bw.vimModePaneID == "" {
		return
	}

	wsView := a.activeWorkspaceViewForBrowserWindow(bw)
	if wsView != nil {
		if pv := wsView.GetPaneView(bw.vimModePaneID); pv != nil {
			pv.SetVimMode(false)
			logging.FromContext(ctx).Debug().
				Str("window_id", bw.id).
				Str("pane_id", string(bw.vimModePaneID)).
				Msg("vim mode deactivated on pane")
		}
	}
	bw.vimModePaneID = ""

	a.vimMode.resetPulse()
}

// updateModeIndicatorToaster reconciles one window's mode toaster with its
// current mode and the live mode-indicator preference.
func (a *App) updateModeIndicatorToaster(ctx context.Context, bw *browserWindow, mode input.Mode) {
	if bw == nil || bw.modeToaster == nil {
		return
	}

	if !a.runtimeConfigSnapshot().UI.Workspace.Styling.ModeIndicatorToasterEnabled || mode == input.ModeNormal {
		bw.modeToaster.Hide()
		return
	}

	bw.modeToaster.Show(ctx, mode.DisplayName(), component.ToastInfo,
		component.WithDuration(0),
		component.WithPosition(component.ToastPositionBottomLeft),
		component.WithModeClass(getModeToastClass(mode)),
	)
}

// bindVimModeSequenceToaster wires pending Vim-sequence text onto this
// window's modeToaster only (never appToaster / pane toasters).
func (a *App) bindVimModeSequenceToaster(ctx context.Context, bw *browserWindow) {
	if a == nil || bw == nil || bw.keyboardHandler == nil {
		return
	}
	bw.keyboardHandler.SetOnPendingSequenceChange(func(pending string) {
		a.showPendingSequence(ctx, bw, pending)
	})
	logging.FromContext(ctx).Debug().
		Str("window_id", bw.id).
		Msg("vim mode pending sequence toaster bound")
}

// showPendingSequence shows "VIM MODE · pending" on the per-window modeToaster
// while a sequence is incomplete, and restores stable "VIM MODE" when pending
// clears. Mode exit/hide is handled by handleModeChange/updateModeIndicatorToaster;
// silent reset under ModalState avoids stale pending callbacks.
func (a *App) showPendingSequence(ctx context.Context, bw *browserWindow, pending string) {
	if a == nil || bw == nil {
		return
	}
	if bw.modeFrame != nil {
		cfg := a.runtimeConfigSnapshot().UI.Workspace
		bw.modeFrame.setPending(pending, cfg.VimMode.Actions)
	}
	if bw.modeToaster == nil {
		return
	}
	log := logging.FromContext(ctx)
	if !a.runtimeConfigSnapshot().UI.Workspace.Styling.ModeIndicatorToasterEnabled {
		bw.modeToaster.Hide()
		log.Debug().Str("window_id", bw.id).Msg("vim mode pending toaster hidden (disabled)")
		return
	}

	text := input.ModeVim.DisplayName()
	if pending != "" {
		text = text + " · " + pending
	}
	bw.modeToaster.Show(ctx, text, component.ToastInfo,
		component.WithDuration(0),
		component.WithPosition(component.ToastPositionBottomLeft),
		component.WithModeClass(getModeToastClass(input.ModeVim)),
	)
	log.Debug().
		Str("window_id", bw.id).
		Str("pending", pending).
		Str("toast_text", text).
		Msg("vim mode pending toaster updated")
}
