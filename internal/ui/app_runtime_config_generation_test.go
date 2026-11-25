package ui

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyRuntimeConfigChangeDropsStaleGenerations(t *testing.T) {
	bw, view := newFakeModeToasterWindow("win-1")
	bw.keyboardHandler = newKeyboardHandlerInVimMode(t)
	app := &App{
		runtimeConfig:  runtimeConfigStateFromSnapshotForTest(enabledIndicatorSnapshot()),
		browserWindows: map[string]*browserWindow{bw.id: bw},
	}
	app.updateModeFrame(bw, bw.keyboardHandler.Mode())

	newer := enabledIndicatorSnapshot()
	newer.Generation = 2
	newer.UI.Workspace.Styling.ModeIndicatorToasterEnabled = false
	app.applyRuntimeConfigChange(context.Background(), newer)
	assert.False(t, view.pillShown)

	older := enabledIndicatorSnapshot()
	older.Generation = 1
	app.applyRuntimeConfigChange(context.Background(), older)
	assert.False(t, view.pillShown, "an older generation must not override a newer one")
	assert.False(t, app.runtimeConfigSnapshot().UI.Workspace.Styling.ModeIndicatorToasterEnabled)

	unordered := enabledIndicatorSnapshot()
	app.applyRuntimeConfigChange(context.Background(), unordered)
	assert.True(t, view.pillShown, "generation 0 snapshots are always applied")
}
