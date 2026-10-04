package ui

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/dto"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout/mocks"
)

// newVimPageTestWindow builds a window in Vim Mode with a real mode toaster
// bound to the pending-sequence callback, as initChrome does. It records every
// text shown on the toast so tests can assert what the user sees.
func newVimPageTestWindow(t *testing.T) (*App, *browserWindow, *[]string) {
	t.Helper()
	factory := mocks.NewMockWidgetFactory(t)
	toaster, container, label := newModeToasterForVimModeTest(t, factory)
	shown := &[]string{}
	container.EXPECT().RemoveCssClass(mock.Anything).Maybe()
	container.EXPECT().AddCssClass(mock.Anything).Maybe()
	container.EXPECT().SetHalign(mock.Anything).Maybe()
	container.EXPECT().SetValign(mock.Anything).Maybe()
	container.EXPECT().SetVisible(mock.Anything).Maybe()
	label.EXPECT().SetText(mock.Anything).Run(func(text string) { *shown = append(*shown, text) }).Maybe()

	app := &App{runtimeConfig: runtimeConfigStateFromSnapshotForTest(entity.RuntimeConfigSnapshot{
		UI: entity.RuntimeUIConfig{Workspace: entity.WorkspaceConfig{Styling: entity.WorkspaceStylingConfig{
			ModeIndicatorToasterEnabled: true,
		}}},
	})}
	bw := &browserWindow{id: "win-1", modeToaster: toaster}
	app.browserWindows = map[string]*browserWindow{bw.id: bw}
	bw.keyboardHandler = newKeyboardHandlerInVimMode(t)
	app.bindVimModeSequenceToaster(context.Background(), bw)
	return app, bw, shown
}

// Starting a key-capturing interaction must leave the sub-mode label on the
// toast. The capture path used to reset the pending sequence with
// notification, which repainted the toast back to plain "VIM MODE" in the
// same instant and made the interaction look as if it had ended.
func TestCaptureVimPageKeysKeepsSubModeLabelOnToast(t *testing.T) {
	app, bw, shown := newVimPageTestWindow(t)
	wv := portmocks.NewMockWebView(t)

	app.captureVimPageKeys(context.Background(), bw, wv, "VISUAL")

	require.NotEmpty(t, *shown)
	require.Equal(t, "VIM MODE · VISUAL", (*shown)[len(*shown)-1])
	require.True(t, bw.keyboardHandler.PageKeyCaptureActive())
}

// A completed sequence is reported with an empty pending string before its
// action runs. When that action starts a page interaction, the stale pending
// notification must not reach the toast after the sub-mode label.
func TestVimPageSubModeLabelSurvivesSequenceCompletion(t *testing.T) {
	app, bw, shown := newVimPageTestWindow(t)
	wv := portmocks.NewMockWebView(t)

	app.showPendingSequence(context.Background(), bw, "")
	app.captureVimPageKeys(context.Background(), bw, wv, "HINTS")
	// Any pending notification emitted while the page owns the keys is noise.
	app.showPendingSequence(context.Background(), bw, "")

	require.Equal(t, "VIM MODE · HINTS", (*shown)[len(*shown)-1])
}

// The legend is a key reference for Vim Mode bindings; while the page owns the
// keys it would describe keys that do not apply, so it must be hidden and
// restored when the interaction ends.
func TestVimPageInteractionHidesLegendWhileCaptured(t *testing.T) {
	app, bw, _ := newVimPageTestWindow(t)
	frame := &modeFrame{mode: input.ModeVim, visible: true}
	bw.modeFrame = frame
	wv := portmocks.NewMockWebView(t)

	app.captureVimPageKeys(context.Background(), bw, wv, "VISUAL")
	require.True(t, frame.suspended, "legend must be suspended while the page captures keys")

	wv.EXPECT().ID().Return(1).Maybe()
	app.endVimPageInteraction(context.Background(), bw)
	require.False(t, frame.suspended, "legend must be restored when the interaction ends")
	require.False(t, bw.keyboardHandler.PageKeyCaptureActive())
}

// The page moves between hints, caret, visual and visual line without ending
// the interaction: the toast follows each state while key capture and the
// suspended legend stay in place.
func TestVimPageModeChangesRepaintToastWithoutReleasingCapture(t *testing.T) {
	app, bw, shown := newVimPageTestWindow(t)
	frame := &modeFrame{mode: input.ModeVim, visible: true}
	bw.modeFrame = frame
	wv := portmocks.NewMockWebView(t)
	app.captureVimPageKeys(context.Background(), bw, wv, vimPageInteractionLabel("visual"))
	require.Equal(t, "VIM MODE · HINTS", (*shown)[len(*shown)-1])

	for mode, want := range map[dto.VimPageMode]string{
		dto.VimPageModeCaret:      "VIM MODE · CARET",
		dto.VimPageModeVisual:     "VIM MODE · VISUAL",
		dto.VimPageModeVisualLine: "VIM MODE · VISUAL LINE",
		dto.VimPageModeHints:      "VIM MODE · HINTS",
	} {
		app.showVimPageMode(context.Background(), bw, mode)
		require.Equal(t, want, (*shown)[len(*shown)-1])
		require.True(t, bw.keyboardHandler.PageKeyCaptureActive(), "mode %s must not release key capture", mode)
		require.NotNil(t, bw.vimPageInteractionWebView)
		require.True(t, frame.suspended, "legend stays hidden while the page owns the keys")
	}
}

func TestVimPageModeChangeIgnoredWithoutInteraction(t *testing.T) {
	app, bw, shown := newVimPageTestWindow(t)

	app.showVimPageMode(context.Background(), bw, dto.VimPageModeCaret)

	require.Empty(t, *shown, "a late mode report must not repaint the indicator once the interaction ended")
}
