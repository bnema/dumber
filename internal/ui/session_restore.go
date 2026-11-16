package ui

import (
	"context"
	"errors"
	"fmt"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
)

// restoreSessionInNewWindows imports a snapshot into the current live session.
// Unlike startup restoration it never clears or reuses existing windows.
func (a *App) restoreSessionInNewWindows(ctx context.Context, id entity.SessionID) error {
	if a.deps == nil || a.deps.SessionRepo == nil || a.deps.SessionStateRepo == nil {
		return fmt.Errorf("session restoration unavailable")
	}
	uc := usecase.NewRestoreSessionUseCase(a.deps.SessionStateRepo, a.deps.SessionRepo)
	output, err := uc.ExecuteImport(ctx, usecase.RestoreInput{SessionID: id}, a.deps.CurrentSessionID)
	if err != nil {
		return err
	}
	state := output.State
	restored := entity.WindowTabListsFromSnapshot(state, a.generateID)
	if len(restored) == 0 {
		if state.Version < entity.SessionStateVersion {
			return fmt.Errorf("restored session contains no windows")
		}
		restored = []entity.WindowTabListState{{Tabs: entity.NewTabList()}}
	}
	windows := make([]*browserWindow, 0, len(restored))
	for i := range restored {
		bw, createErr := a.createBrowserWindow(ctx, "")
		if createErr != nil {
			a.cleanupCreatedBrowserWindows(windows)
			return fmt.Errorf("create restored window %d: %w", i+1, createErr)
		}
		windows = append(windows, bw)
	}
	a.assignRuntimeWindowTabLists(windows, restored, false)
	active := windows[safeWindowIndex(state.ActiveWindowIndex, len(windows))]
	for _, bw := range windows {
		for _, tab := range bw.tabs.Tabs {
			a.syncDerivedGlobalTabMirror(tab)
		}
	}
	previousFocus := a.lastFocusedBrowserWindow()
	if err := a.buildRestoredWindowUI(ctx, windows); err != nil {
		a.cleanupCreatedBrowserWindows(windows)
		a.activateBrowserWindow(previousFocus)
		return err
	}
	a.activateBrowserWindow(active)
	a.showRestoredRuntimeWindows(windows)
	if a.snapshotService != nil {
		a.snapshotService.MarkDirty()
	}
	return nil
}

// RestoreSession accepts external restore intent on the GTK main thread.
func (a *App) RestoreSession(ctx context.Context, id entity.SessionID) error {
	return a.dispatchWindowURLWork(ctx, "ui.restore_session", "", func(ctx context.Context) error {
		requester := a.lastFocusedBrowserWindow()
		err := a.restoreSessionInNewWindows(ctx, id)
		message, level := "Session restored", component.ToastSuccess
		if errors.Is(err, usecase.ErrSessionAlreadyOpen) {
			message, level = "Session already open", component.ToastWarning
		} else if err != nil {
			message, level = "Session restore failed: "+err.Error(), component.ToastWarning
		}
		if requester != nil && requester.appToaster != nil {
			requester.appToaster.Show(ctx, message, level)
		}
		return err
	})
}

// rollbackStartupRestore leaves one empty shell for createInitialTab's fallback.
func (a *App) rollbackStartupRestore(ctx context.Context, windows []*browserWindow) {
	a.clearRuntimeWindowUI(windows)
	for _, bw := range windows {
		a.releaseBrowserWindowTabs(ctx, bw.id, bw)
		bw.tabs = entity.NewTabList()
	}
	a.cleanupCreatedBrowserWindows(windows[1:])
	first := windows[0]
	a.tabs.ReplaceFrom(entity.NewTabList())
	a.activateBrowserWindow(first)
}
