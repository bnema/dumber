package ui

import (
	"context"
	"testing"
	"time"

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

var visualOutcome = hintsOutcome(dto.VimPageVisual)

func hintsOutcome(kind dto.VimPageInteractionKind) dto.VimNavigationOutcome {
	return dto.VimNavigationOutcome{CapturePageKeys: true, PageKind: kind, PageMode: dto.VimPageModeHints}
}

// Starting a key-capturing interaction must leave the sub-mode label on the
// toast. The capture path used to reset the pending sequence with
// notification, which repainted the toast back to plain "VIM MODE" in the
// same instant and made the interaction look as if it had ended.
func TestCaptureVimPageKeysKeepsSubModeLabelOnToast(t *testing.T) {
	app, bw, shown := newVimPageTestWindow(t)
	wv := portmocks.NewMockWebView(t)

	app.captureVimPageKeys(context.Background(), bw, wv, visualOutcome)

	require.NotEmpty(t, *shown)
	require.Equal(t, "VIM MODE · HINTS", (*shown)[len(*shown)-1], "every interaction opens on hints")
	require.True(t, bw.keyboardHandler.PageKeyCaptureActive())
}

// A completed sequence is reported with an empty pending string before its
// action runs. When that action starts a page interaction, the stale pending
// notification must not reach the toast after the sub-mode label.
func TestVimPageSubModeLabelSurvivesSequenceCompletion(t *testing.T) {
	app, bw, shown := newVimPageTestWindow(t)
	wv := portmocks.NewMockWebView(t)

	app.showPendingSequence(context.Background(), bw, "")
	app.captureVimPageKeys(context.Background(), bw, wv, hintsOutcome(dto.VimPageHintFollow))
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

	app.captureVimPageKeys(context.Background(), bw, wv, visualOutcome)
	require.True(t, frame.suspended, "legend must be suspended while the page captures keys")

	wv.EXPECT().ID().Return(1).Maybe()
	app.endVimPageInteraction(context.Background(), bw, true)
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
	app.captureVimPageKeys(context.Background(), bw, wv, visualOutcome)
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

// exitModeWithin runs fn and fails the test if it does not return in time,
// so a lock re-entry deadlock shows up as a failure instead of a hung run.
func exitModeWithin(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("ExitMode did not return: mode-change callback re-entered the modal lock")
	}
}

// ExitMode runs the mode-change callback under the modal lock. Releasing the
// page capture from that callback must not read the mode back through the lock.
func TestExitModeWithPageCaptureDoesNotDeadlock(t *testing.T) {
	app, bw, _ := newVimPageTestWindow(t)
	bw.keyboardHandler.SetOnModeChange(func(from, to input.Mode) {
		app.handleModeChange(context.Background(), bw, from, to)
	})
	wv := portmocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(1).Maybe()
	app.captureVimPageKeys(context.Background(), bw, wv, visualOutcome)

	exitModeWithin(t, bw.keyboardHandler.ExitMode)

	require.False(t, bw.keyboardHandler.PageKeyCaptureActive())
	require.Nil(t, bw.vimPageInteractionWebView)
	require.Equal(t, input.ModeNormal, bw.keyboardHandler.Mode())
}

func TestVimPageModeLabel(t *testing.T) {
	for mode, want := range map[dto.VimPageMode]string{
		dto.VimPageModeHints:      "HINTS",
		dto.VimPageModeCaret:      "CARET",
		dto.VimPageModeVisual:     "VISUAL",
		dto.VimPageModeVisualLine: "VISUAL LINE",
	} {
		got, ok := vimPageModeLabel(mode)
		require.True(t, ok, mode)
		require.Equal(t, want, got)
	}
	_, ok := vimPageModeLabel("bogus")
	require.False(t, ok)
}

// One mapping drives every label: the sub-mode, plus the target of hints that
// do more than follow. Later page-reported modes drop the hint target.
func TestVimPageLabel(t *testing.T) {
	tests := []struct {
		kind dto.VimPageInteractionKind
		mode dto.VimPageMode
		want string
	}{
		{dto.VimPageVisual, dto.VimPageModeHints, "HINTS"},
		{dto.VimPageHintFollow, dto.VimPageModeHints, "HINTS"},
		{dto.VimPageHintFollowNew, dto.VimPageModeHints, "HINTS · NEW PANE"},
		{dto.VimPageHintYankURL, dto.VimPageModeHints, "HINTS · YANK URL"},
		{dto.VimPageVisual, dto.VimPageModeCaret, "CARET"},
		{dto.VimPageVisual, dto.VimPageModeVisual, "VISUAL"},
		{dto.VimPageVisual, dto.VimPageModeVisualLine, "VISUAL LINE"},
	}
	for _, tt := range tests {
		got, ok := vimPageLabel(tt.kind, tt.mode)
		require.True(t, ok)
		require.Equal(t, tt.want, got, "kind %d mode %s", tt.kind, tt.mode)
	}
	_, ok := vimPageLabel(dto.VimPageVisual, "bogus")
	require.False(t, ok)
}

func TestVimPageInteractionLabelsFollowTheOutcome(t *testing.T) {
	app, bw, shown := newVimPageTestWindow(t)
	wv := portmocks.NewMockWebView(t)

	app.captureVimPageKeys(context.Background(), bw, wv, hintsOutcome(dto.VimPageHintFollowNew))
	require.Equal(t, "VIM MODE · HINTS · NEW PANE", (*shown)[len(*shown)-1])
	app.releaseVimPageKeys(context.Background(), bw, "test", true)

	app.captureVimPageKeys(context.Background(), bw, wv, hintsOutcome(dto.VimPageHintYankURL))
	require.Equal(t, "VIM MODE · HINTS · YANK URL", (*shown)[len(*shown)-1])
}
