package browser

import "log"

func (wm *WorkspaceManager) paneCloseLogf(format string, args ...interface{}) {
	if wm == nil || !wm.debugPaneClose {
		return
	}
	log.Printf("[pane-close] "+format, args...)
}

func (wm *WorkspaceManager) dumpTreeState(label string) {
	if wm == nil || wm.diagnostics == nil {
		return
	}
	wm.diagnostics.Capture(label, wm.root)
}

func (wm *WorkspaceManager) DiagnosticsSnapshots() []TreeSnapshot {
	if wm == nil || wm.diagnostics == nil {
		return nil
	}
	return wm.diagnostics.Snapshots()
}

func (wm *WorkspaceManager) SetPaneCloseDebug(enabled bool) {
	if wm == nil {
		return
	}
	wm.debugPaneClose = enabled
	if wm.diagnostics == nil {
		wm.diagnostics = NewWorkspaceDiagnostics(enabled)
		return
	}
	wm.diagnostics.SetEnabled(enabled)
}
