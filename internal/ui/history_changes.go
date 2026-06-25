package ui

import (
	"context"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

var _ port.HistoryChangeSink = (*historyChangeAdapter)(nil)

type historySidebarReloader interface {
	RequestReloadIfVisible(reason string)
}

type historyChangeAdapter struct {
	app *App
}

func newHistoryChangeAdapter(app *App) *historyChangeAdapter {
	return &historyChangeAdapter{app: app}
}

func (h *historyChangeAdapter) OnHistoryChanged(ctx context.Context, change dto.HistoryChange) {
	if h == nil || h.app == nil {
		return
	}
	app := h.app
	run := func() {
		app.refreshVisibleHistorySidebars(change)
	}
	if app.dispatchOnMainThread == nil {
		logging.FromContext(ctx).Warn().
			Msg("history sidebar refresh skipped: main-thread dispatcher unavailable")
		return
	}
	result := app.dispatchOnMainThread("ui.history_sidebar_refresh", run)
	if !result.Completed() {
		logging.FromContext(ctx).Warn().
			Str("status", string(result.Status)).
			Dur("elapsed", result.Elapsed).
			Msg("history sidebar refresh dispatch did not complete")
	}
}

func (a *App) refreshVisibleHistorySidebars(change dto.HistoryChange) {
	if a == nil {
		return
	}
	reason := historyChangeReasonLabel(change)
	for _, bw := range a.browserWindows {
		if bw == nil || !bw.sidebarVisible {
			continue
		}
		reloader := bw.historySidebarReloader
		if reloader == nil {
			reloader = bw.historySidebar
		}
		if reloader == nil {
			continue
		}
		reloader.RequestReloadIfVisible(reason)
	}
}

func historyChangeReasonLabel(change dto.HistoryChange) string {
	if len(change.Reasons) == 0 {
		return "history-change"
	}
	return string(change.Reasons[0])
}
