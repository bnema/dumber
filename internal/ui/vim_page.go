package ui

import (
	"context"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
)

// captureVimPageKeys routes this window's Vim Mode keys to the in-page
// interaction running in wv until the page reports its end. The outcome names
// the interaction and the sub-mode it starts in.
func (a *App) captureVimPageKeys(ctx context.Context, bw *browserWindow, wv port.WebView, outcome dto.VimNavigationOutcome) {
	if bw == nil || bw.keyboardHandler == nil || wv == nil {
		return
	}
	bw.vimPageInteractionWebView = wv
	bw.vimPageInteractionKind = outcome.PageKind
	label, _ := vimPageLabel(outcome.PageKind, outcome.PageMode)
	// Capture first: it resets the pending sequence, and the toast and legend
	// below must reflect the sub-mode, not a cleared sequence.
	bw.keyboardHandler.SetPageKeyCapture(func(key string) {
		if err := a.vimNavigationUseCase().SendPageKey(ctx, wv, key); err != nil {
			logging.FromContext(ctx).Debug().Err(err).Msg("vim page key forwarding failed")
			a.endVimPageInteraction(ctx, bw, a.vimModeActiveForBrowserWindow(bw))
		}
	})
	a.showVimModeToast(ctx, bw, label)
	if bw.modeFrame != nil {
		bw.modeFrame.setSuspended(true)
	}
	logging.FromContext(ctx).Debug().
		Str("window_id", bw.id).
		Str("sub_mode", label).
		Msg("vim page interaction capturing keys")
}

// releaseVimPageKeys returns this window's keys to Vim Mode bindings and
// restores the plain indicator and legend when stillVim reports the window
// remains in Vim Mode. The caller supplies it because the mode-change callback
// runs under the modal lock, where reading the mode back would deadlock.
// It reports the WebView that owned the interaction, if any.
func (a *App) releaseVimPageKeys(ctx context.Context, bw *browserWindow, reason string, stillVim bool) port.WebView {
	wv := bw.vimPageInteractionWebView
	bw.vimPageInteractionWebView = nil
	bw.vimPageInteractionKind = 0
	if bw.keyboardHandler != nil {
		bw.keyboardHandler.ClearPageKeyCapture()
	}
	if wv == nil {
		return nil
	}
	if stillVim {
		a.showVimModeToast(ctx, bw, "")
		if bw.modeFrame != nil {
			bw.modeFrame.setSuspended(false)
		}
	}
	logging.FromContext(ctx).Debug().
		Str("window_id", bw.id).
		Str("reason", reason).
		Msg("vim page interaction released keys")
	return wv
}

// vimPageInteractionOwner returns the window whose key capture belongs to the
// interaction running in paneID's WebView, or nil when that pane owns none.
func (a *App) vimPageInteractionOwner(paneID entity.PaneID) *browserWindow {
	bw := a.browserWindowForAnyPane(paneID)
	if bw == nil || bw.vimPageInteractionWebView == nil || a.contentCoord == nil {
		return nil
	}
	if a.contentCoord.GetWebView(paneID) != bw.vimPageInteractionWebView {
		return nil
	}
	return bw
}

// handleVimPageInteractionEnded releases key capture when the page reports
// that its hint or visual interaction finished.
func (a *App) handleVimPageInteractionEnded(ctx context.Context, paneID entity.PaneID) {
	bw := a.vimPageInteractionOwner(paneID)
	if bw == nil {
		return
	}
	a.releaseVimPageKeys(ctx, bw, "page-reported-end", a.vimModeActiveForBrowserWindow(bw))
}

// handleVimPageModeChanged repaints the indicator when the page moves its
// interaction to another sub-mode (hints, caret, visual). Key capture and the
// legend suspension stay as they are.
func (a *App) handleVimPageModeChanged(ctx context.Context, paneID entity.PaneID, mode dto.VimPageMode) {
	if bw := a.vimPageInteractionOwner(paneID); bw != nil {
		a.showVimPageMode(ctx, bw, mode)
	}
}

// showVimPageMode paints the sub-mode label for the interaction owned by bw.
func (a *App) showVimPageMode(ctx context.Context, bw *browserWindow, mode dto.VimPageMode) {
	if bw.vimPageInteractionWebView == nil {
		return
	}
	if label, ok := vimPageLabel(bw.vimPageInteractionKind, mode); ok {
		a.showVimModeToast(ctx, bw, label)
	}
}

// vimPageModeLabel names a page-reported sub-mode for the Vim Mode indicator.
func vimPageModeLabel(mode dto.VimPageMode) (string, bool) {
	switch mode {
	case dto.VimPageModeHints:
		return "HINTS", true
	case dto.VimPageModeCaret:
		return "CARET", true
	case dto.VimPageModeVisual:
		return "VISUAL", true
	case dto.VimPageModeVisualLine:
		return "VISUAL LINE", true
	default:
		return "", false
	}
}

// vimPageLabel is the single source of the indicator label: the sub-mode name,
// plus the target of link hints that do more than follow (new pane, yank URL).
func vimPageLabel(kind dto.VimPageInteractionKind, mode dto.VimPageMode) (string, bool) {
	label, ok := vimPageModeLabel(mode)
	if !ok || mode != dto.VimPageModeHints {
		return label, ok
	}
	switch kind {
	case dto.VimPageHintFollowNew:
		return label + " · NEW PANE", true
	case dto.VimPageHintYankURL:
		return label + " · YANK URL", true
	default:
		return label, true
	}
}

// endVimPageInteraction cancels any in-page interaction owned by this window.
// stillVim reports whether the window stays in Vim Mode (see releaseVimPageKeys).
func (a *App) endVimPageInteraction(ctx context.Context, bw *browserWindow, stillVim bool) {
	if bw == nil {
		return
	}
	wv := a.releaseVimPageKeys(ctx, bw, "canceled", stillVim)
	if wv == nil {
		return
	}
	if err := a.vimNavigationUseCase().CancelPageInteraction(ctx, wv); err != nil {
		logging.FromContext(ctx).Debug().Err(err).Msg("failed to cancel vim page interaction")
	}
}
