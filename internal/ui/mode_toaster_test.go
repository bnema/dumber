package ui

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/stretchr/testify/require"
)

var testVimActions = map[string]entity.ActionBinding{
	"vim-scroll-down": {Keys: []string{"j"}, Desc: "Scroll down"},
	"heading-next":    {Keys: []string{"]]"}},
	"visual":          {Keys: []string{"v"}},
	"cancel":          {Keys: []string{"esc"}},
}

func legendOn(mode string) entity.WorkspaceStylingConfig {
	return entity.WorkspaceStylingConfig{ModeIndicatorToasterEnabled: true, ModeLegend: mode, ModeLegendAnimations: true}
}

func newTestToaster(onToggle func(bool)) (*modeToaster, *fakeModeToasterView) {
	view := newFakeModeToasterView()
	return newModeToasterWithView(view, onToggle), view
}

func TestModeToasterHeader(t *testing.T) {
	require.Equal(t, "VIM MODE", modeToasterHeader(input.ModeVim, ""))
	require.Equal(t, "VIM MODE · 2g", modeToasterHeader(input.ModeVim, "2g"))
	require.Equal(t, "VIM MODE · HINTS · NEW PANE", modeToasterHeader(input.ModeVim, "HINTS · NEW PANE"))
	require.Equal(t, "PANE MODE", modeToasterHeader(input.ModePane, ""))
}

func TestModeToasterShowsPillAndLegendOnEnter(t *testing.T) {
	toaster, view := newTestToaster(nil)

	toaster.apply(input.ModeVim, legendOn("always"), "command", testVimActions)

	require.True(t, view.pillShown)
	require.Equal(t, "VIM MODE", view.header)
	require.Equal(t, input.ModeVim, view.mode)
	require.True(t, view.bodyShown)
	require.Equal(t, chevronExpanded, view.chevron)
	require.True(t, view.interactive)
	require.Equal(t, "command", view.style)
	var names []string
	for _, group := range view.groups {
		names = append(names, group.Name)
	}
	require.Equal(t, []string{"SCROLL", "JUMP", "SELECT", "EXIT"}, names)
}

func TestModeToasterDelayWithholdsLegendNotPill(t *testing.T) {
	toaster, view := newTestToaster(nil)
	cfg := legendOn("delay")
	cfg.ModeLegendDelayMs = 20

	toaster.apply(input.ModeVim, cfg, "", testVimActions)
	require.True(t, view.pillShown, "the pill appears at once")
	require.False(t, view.bodyShown, "the legend waits for the delay")

	waitFor(t, func() bool { return view.bodyShown })
}

func TestModeToasterDelayZeroShowsLegendAtOnce(t *testing.T) {
	toaster, view := newTestToaster(nil)
	cfg := legendOn("delay")
	cfg.ModeLegendDelayMs = 0

	toaster.apply(input.ModeVim, cfg, "", testVimActions)

	require.True(t, view.bodyShown)
}

func TestModeToasterModeExitBeforeDelayNeverShowsLegend(t *testing.T) {
	toaster, view := newTestToaster(nil)
	cfg := legendOn("delay")
	cfg.ModeLegendDelayMs = 30
	toaster.apply(input.ModeVim, cfg, "", testVimActions)
	toaster.apply(input.ModeNormal, cfg, "", nil)

	time.Sleep(80 * time.Millisecond)
	pumpMainContext()

	require.False(t, view.bodyShown)
	require.False(t, view.pillShown)
}

func TestModeToasterLegendOffIsPillOnlyWithoutChevron(t *testing.T) {
	toaster, view := newTestToaster(nil)

	toaster.apply(input.ModeVim, legendOn("off"), "", testVimActions)

	require.True(t, view.pillShown)
	require.False(t, view.bodyShown)
	require.Equal(t, chevronNone, view.chevron)
	require.False(t, view.interactive, "nothing to toggle: the pill ignores the pointer")
	toaster.pillClicked()
	require.False(t, toaster.collapsed, "a click on a legend-less pill does nothing")
}

func TestModeToasterNothingWhenIndicatorAndLegendOff(t *testing.T) {
	toaster, view := newTestToaster(nil)
	cfg := entity.WorkspaceStylingConfig{ModeLegend: "off"}

	toaster.apply(input.ModeVim, cfg, "", testVimActions)

	require.False(t, view.pillShown)
	require.False(t, view.bodyShown)
}

func TestModeToasterPillClickTogglesThroughTheApp(t *testing.T) {
	var asked []bool
	toaster, view := newTestToaster(func(collapsed bool) { asked = append(asked, collapsed) })
	toaster.apply(input.ModeVim, legendOn("always"), "", testVimActions)

	toaster.pillClicked()

	require.Equal(t, []bool{true}, asked)
	require.True(t, view.bodyShown, "the toaster waits for the app to broadcast the new state")
}

func TestModeToasterCollapseHidesBodyKeepsPill(t *testing.T) {
	toaster, view := newTestToaster(nil)
	toaster.apply(input.ModeVim, legendOn("always"), "", testVimActions)

	toaster.setCollapsed(true)

	require.False(t, view.bodyShown)
	require.True(t, view.pillShown, "collapsed leaves the pill, clickable to re-expand")
	require.True(t, view.interactive)
	require.Equal(t, chevronCollapsed, view.chevron)

	toaster.setCollapsed(false)
	require.True(t, view.bodyShown)
	require.Equal(t, chevronExpanded, view.chevron)
}

func TestModeToasterPillClickWithoutAppTogglesLocally(t *testing.T) {
	toaster, view := newTestToaster(nil)
	toaster.apply(input.ModeVim, legendOn("always"), "", testVimActions)

	toaster.pillClicked()
	require.False(t, view.bodyShown)
	toaster.pillClicked()
	require.True(t, view.bodyShown)
}

func TestModeToasterStartsCollapsedFromConfiguration(t *testing.T) {
	toaster, view := newTestToaster(nil)
	cfg := legendOn("always")
	cfg.ModeLegendCollapsed = true

	toaster.apply(input.ModePane, cfg, "", map[string]entity.ActionBinding{"split-right": {Keys: []string{"r"}}})

	require.True(t, view.pillShown)
	require.False(t, view.bodyShown)
	require.Equal(t, chevronCollapsed, view.chevron)
}

func TestModeToasterCollapsedStateSurvivesModeChanges(t *testing.T) {
	toaster, view := newTestToaster(nil)
	cfg := legendOn("always")
	cfg.ModeLegendCollapsed = true
	pane := map[string]entity.ActionBinding{"split-right": {Keys: []string{"r"}}}

	toaster.apply(input.ModeVim, cfg, "", testVimActions)
	toaster.apply(input.ModePane, cfg, "", pane)

	require.False(t, view.bodyShown, "the collapsed state is shared by all modes")
	require.Equal(t, input.ModePane, view.mode)
}

func TestModeToasterExpandingSkipsRemainingDelay(t *testing.T) {
	toaster, view := newTestToaster(nil)
	cfg := legendOn("delay")
	cfg.ModeLegendDelayMs = 10_000
	cfg.ModeLegendCollapsed = true
	toaster.apply(input.ModeVim, cfg, "", testVimActions)

	require.False(t, view.bodyShown)

	toaster.setCollapsed(false)
	require.True(t, view.bodyShown, "expanding is a request to see the legend now, not after the delay")
	require.Zero(t, toaster.showTimer)
}

func TestModeToasterConfigReloadDoesNotRestartLegend(t *testing.T) {
	toaster, view := newTestToaster(nil)
	cfg := legendOn("always")
	toaster.apply(input.ModeVim, cfg, "", testVimActions)
	generation := toaster.generation
	view.groups = nil

	// A reload that only flips the collapsed preference (the app's own save).
	cfg.ModeLegendCollapsed = true
	toaster.apply(input.ModeVim, cfg, "", testVimActions)

	require.Equal(t, generation, toaster.generation, "not a mode change")
	require.Nil(t, view.groups, "the legend is not rebuilt")
	require.False(t, view.bodyShown)
}

func TestModeToasterConfigReloadRebuildsWhenBindingsChange(t *testing.T) {
	toaster, view := newTestToaster(nil)
	toaster.apply(input.ModeVim, legendOn("always"), "", testVimActions)

	changed := map[string]entity.ActionBinding{"visual": {Keys: []string{"v"}}}
	toaster.apply(input.ModeVim, legendOn("always"), "", changed)

	require.Len(t, view.groups, 1)
	require.Equal(t, "SELECT", view.groups[0].Name)
}

func TestModeToasterIndicatorPreferenceReloadKeepsLegend(t *testing.T) {
	toaster, view := newTestToaster(nil)
	toaster.apply(input.ModeVim, legendOn("always"), "", testVimActions)

	noIndicator := legendOn("always")
	noIndicator.ModeIndicatorToasterEnabled = false
	toaster.apply(input.ModeVim, noIndicator, "", testVimActions)
	require.True(t, view.pillShown, "legend still on: the pill remains as its header")
	require.True(t, view.bodyShown)

	toaster.apply(input.ModeVim, legendOn("off"), "", testVimActions)
	require.True(t, view.pillShown, "indicator back on, legend off: plain pill")
	require.False(t, view.bodyShown)
}

func TestModeToasterPendingSequenceShowsInHeaderAndDimsRows(t *testing.T) {
	toaster, view := newTestToaster(nil)
	toaster.apply(input.ModeVim, legendOn("always"), "", testVimActions)

	toaster.setPending("]")

	require.Equal(t, "VIM MODE · ]", view.header)
	require.False(t, view.dimmed["heading-next"])
	require.True(t, view.dimmed["vim-scroll-down"])
	require.True(t, view.dimmed["visual"])

	toaster.setPending("")
	require.Equal(t, "VIM MODE", view.header)
	require.False(t, view.dimmed["vim-scroll-down"])
}

func TestModeToasterPendingOnlyAppliesToVimMode(t *testing.T) {
	toaster, view := newTestToaster(nil)
	toaster.apply(input.ModePane, legendOn("always"), "", map[string]entity.ActionBinding{"split-right": {Keys: []string{"r"}}})

	toaster.setPending("x")

	require.Equal(t, "PANE MODE", view.header)
	require.False(t, view.dimmed["split-right"])
}

func TestModeToasterSubModeSuspendsLegendButKeepsHeader(t *testing.T) {
	toaster, view := newTestToaster(nil)
	toaster.apply(input.ModeVim, legendOn("always"), "", testVimActions)

	toaster.setDetail("HINTS")
	toaster.setSuspended(true)
	require.Equal(t, "VIM MODE · HINTS", view.header)
	require.False(t, view.bodyShown, "the page owns the keys: the legend would list keys that do not apply")
	require.True(t, view.pillShown)

	toaster.setDetail("")
	toaster.setSuspended(false)
	require.Equal(t, "VIM MODE", view.header)
	require.True(t, view.bodyShown)
}

func TestModeToasterModeChangeResetsDetailAndSuspension(t *testing.T) {
	toaster, view := newTestToaster(nil)
	toaster.apply(input.ModeVim, legendOn("always"), "", testVimActions)
	toaster.setDetail("VISUAL")
	toaster.setSuspended(true)

	toaster.apply(input.ModeNormal, legendOn("always"), "", nil)
	toaster.apply(input.ModeVim, legendOn("always"), "", testVimActions)

	require.Equal(t, "VIM MODE", view.header)
	require.True(t, view.bodyShown)
}

func TestModeToasterSameModeRebuildKeepsInteractionState(t *testing.T) {
	toaster, view := newTestToaster(nil)
	toaster.apply(input.ModeVim, legendOn("always"), "", testVimActions)
	toaster.setDetail("HINTS")
	toaster.setSuspended(true)
	require.False(t, view.bodyShown)

	// A hot reload that changes the bindings rebuilds the legend mid-capture.
	changed := map[string]entity.ActionBinding{"visual": {Keys: []string{"v"}}, "cancel": {Keys: []string{"esc"}}}
	toaster.apply(input.ModeVim, legendOn("always"), "", changed)

	require.Equal(t, "VIM MODE · HINTS", view.header, "the sub-mode label survives the rebuild")
	require.False(t, view.bodyShown, "the legend stays suspended while the page owns the keys")
	require.True(t, toaster.suspended)

	toaster.setDetail("")
	toaster.setSuspended(false)
	require.True(t, view.bodyShown)
	require.Equal(t, "VIM MODE", view.header)
}

func TestModeToasterSameModeRebuildReappliesPendingDimming(t *testing.T) {
	toaster, view := newTestToaster(nil)
	toaster.apply(input.ModeVim, legendOn("always"), "", testVimActions)
	toaster.setPending("]")

	changed := map[string]entity.ActionBinding{
		"heading-next": {Keys: []string{"]]"}},
		"visual":       {Keys: []string{"v"}},
	}
	toaster.apply(input.ModeVim, legendOn("always"), "", changed)
	// The rebuilt legend starts undimmed, as a real view's new rows would.
	require.Equal(t, "]", toaster.pending)
	require.Equal(t, "VIM MODE · ]", view.header)
	require.False(t, view.dimmed["heading-next"])
	require.True(t, view.dimmed["visual"], "rows the pending sequence rules out are dimmed again")
}

func TestModeToasterFlashLightsKeycapsAndClears(t *testing.T) {
	toaster, view := newTestToaster(nil)
	toaster.apply(input.ModeVim, legendOn("always"), "", testVimActions)

	toaster.flash(input.Action("vim-scroll-down"))
	require.True(t, view.flashed["vim-scroll-down"])

	// Underscore action names map onto dashed legend rows.
	toaster.flash(input.Action("heading_next"))
	require.True(t, view.flashed["heading-next"])

	toaster.flash(input.Action("unknown-action"))
	require.True(t, view.flashed["heading-next"], "an action without keycaps leaves the flash alone")

	waitFor(t, func() bool { return len(view.flashed) == 0 })
}

func TestModeToasterFlashNeedsVisibleLegendAndAnimations(t *testing.T) {
	toaster, view := newTestToaster(nil)
	cfg := legendOn("always")
	cfg.ModeLegendAnimations = false
	toaster.apply(input.ModeVim, cfg, "", testVimActions)
	toaster.flash(input.Action("vim-scroll-down"))
	require.Empty(t, view.flashed)

	toaster.apply(input.ModeVim, legendOn("always"), "", testVimActions)
	toaster.setCollapsed(true)
	toaster.flash(input.Action("vim-scroll-down"))
	require.Empty(t, view.flashed, "collapsed legend: nothing to flash")
}

func TestModeToasterLingersWhenPointerIsOverLegend(t *testing.T) {
	toaster, view := newTestToaster(nil)
	cfg := legendOn("always")
	cfg.ModeLegendLinger = true
	toaster.apply(input.ModeVim, cfg, "", testVimActions)
	view.rect = func() (int, int, int, int, bool) { return 10, 10, 400, 200, true }
	toaster.pointerMoved(50, 50)

	toaster.apply(input.ModeNormal, cfg, "", nil)

	require.True(t, toaster.lingering)
	require.True(t, view.lingering)
	require.True(t, view.bodyShown, "the legend stays on screen, read-only")
	require.False(t, view.pillShown, "the mode ended: the header goes")
	require.True(t, toaster.hasLegend() || toaster.lingering)

	toaster.keyPressed()
	require.False(t, toaster.lingering)
	require.False(t, view.bodyShown)
}

func TestModeToasterDoesNotLingerWhenPointerElsewhereOrPressInFlight(t *testing.T) {
	cfg := legendOn("always")
	cfg.ModeLegendLinger = true

	toaster, view := newTestToaster(nil)
	toaster.apply(input.ModeVim, cfg, "", testVimActions)
	view.rect = func() (int, int, int, int, bool) { return 10, 10, 400, 200, true }
	toaster.pointerMoved(900, 900)
	toaster.apply(input.ModeNormal, cfg, "", nil)
	require.False(t, toaster.lingering)

	toaster, view = newTestToaster(nil)
	toaster.apply(input.ModeVim, cfg, "", testVimActions)
	view.rect = func() (int, int, int, int, bool) { return 10, 10, 400, 200, true }
	toaster.pointerMoved(50, 50)
	toaster.pressStarted() // the exit was caused by a click
	toaster.apply(input.ModeNormal, cfg, "", nil)
	require.False(t, toaster.lingering)
}

func TestModeToasterLingerEndsOnClickAndTabSwitch(t *testing.T) {
	cfg := legendOn("always")
	cfg.ModeLegendLinger = true
	toaster, view := newTestToaster(nil)
	toaster.apply(input.ModeVim, cfg, "", testVimActions)
	view.rect = func() (int, int, int, int, bool) { return 0, 0, 500, 500, true }
	toaster.pointerMoved(5, 5)
	toaster.apply(input.ModeNormal, cfg, "", nil)
	require.True(t, toaster.lingering)

	toaster.pressStarted()
	require.False(t, toaster.lingering)
	pumpMainContext() // the press is over; a later exit may linger again

	toaster.apply(input.ModeVim, cfg, "", testVimActions)
	toaster.pointerMoved(5, 5)
	toaster.apply(input.ModeNormal, cfg, "", nil)
	require.True(t, toaster.lingering)
	toaster.dismissLinger()
	require.False(t, toaster.lingering)
}

func TestModeToasterReleaseDropsLegendKeepsPill(t *testing.T) {
	toaster, view := newTestToaster(nil)
	cfg := legendOn("delay")
	cfg.ModeLegendDelayMs = 40
	toaster.apply(input.ModeVim, cfg, "", testVimActions)

	toaster.release()
	time.Sleep(90 * time.Millisecond)
	pumpMainContext()

	require.True(t, view.pillShown, "tab teardown keeps the mode header")
	require.False(t, view.bodyShown, "the pending show for the released workspace never fires")
}

func TestModeToasterNilAndDestroyedAreSafe(t *testing.T) {
	var nilToaster *modeToaster
	nilToaster.destroy()
	nilToaster.release()
	nilToaster.dismissLinger()
	nilToaster.setSuspended(true)
	nilToaster.setCollapsed(true)
	nilToaster.setPending("x")
	nilToaster.setDetail("x")
	nilToaster.flash(input.Action("x"))
	nilToaster.apply(input.ModeVim, legendOn("always"), "", nil)

	toaster, view := newTestToaster(nil)
	toaster.destroy()
	require.True(t, view.destroyed)
	toaster.apply(input.ModeVim, legendOn("always"), "", testVimActions)
	toaster.setPending("x")
	toaster.pillClicked()
}

func TestModeLegendPreferenceClickWinsUntilSettled(t *testing.T) {
	var p modeLegendPreference
	require.False(t, p.current(false))
	require.True(t, p.current(true), "no click yet: the configuration decides")

	require.True(t, p.click(true), "first click starts the writer")
	require.True(t, p.current(false), "the click wins while the config still says otherwise")

	p.configReloaded(false) // a reload from before the save landed
	require.True(t, p.current(false), "a stale reload does not undo the click")

	next, ok := p.next()
	require.True(t, ok)
	require.True(t, next)
	_, ok = p.next()
	require.False(t, ok, "writer drained")

	p.configReloaded(true) // the save's own reload
	require.False(t, p.current(false), "settled: the configuration is the truth again")
	require.True(t, p.current(true))
}

func TestModeLegendPreferenceCoalescesClicksAndKeepsLast(t *testing.T) {
	var p modeLegendPreference
	require.True(t, p.click(true))
	require.False(t, p.click(false), "a running writer picks the new value up")
	require.False(t, p.click(true))

	next, ok := p.next()
	require.True(t, ok)
	require.True(t, next, "the file ends on the last click")
	_, ok = p.next()
	require.False(t, ok)

	require.True(t, p.click(false), "after draining, a new click starts a new writer")
	_, _ = p.next()
	_, _ = p.next()
}

func TestModeLegendPreferenceExternalEditWinsWhenNoSaveInFlight(t *testing.T) {
	var p modeLegendPreference
	require.True(t, p.click(true))
	_, _ = p.next()
	_, _ = p.next() // save finished (and possibly failed)

	p.configReloaded(false) // the file still says false
	require.False(t, p.current(false))
}

func TestSetModeLegendCollapsedUpdatesEveryWindowAndSavesAsync(t *testing.T) {
	saved := make(chan bool, 4)
	var mu sync.Mutex
	app := &App{}
	app.deps = &Dependencies{}
	app.deps.HandlerDeps.SaveModeLegendCollapsed = func(_ context.Context, collapsed bool) error {
		mu.Lock()
		defer mu.Unlock()
		saved <- collapsed
		return nil
	}
	bw1, view1 := newFakeModeToasterWindow("win-1")
	bw2, view2 := newFakeModeToasterWindow("win-2")
	app.browserWindows = map[string]*browserWindow{bw1.id: bw1, bw2.id: bw2}
	bw1.modeToaster.apply(input.ModeVim, legendOn("always"), "", testVimActions)
	bw2.modeToaster.apply(input.ModePane, legendOn("always"), "", map[string]entity.ActionBinding{"split-right": {Keys: []string{"r"}}})

	app.setModeLegendCollapsed(context.Background(), true)

	require.False(t, view1.bodyShown)
	require.False(t, view2.bodyShown, "the toggle applies immediately in every window")
	require.True(t, view1.pillShown)
	require.True(t, view2.pillShown)
	select {
	case got := <-saved:
		require.True(t, got)
	case <-time.After(2 * time.Second):
		t.Fatal("collapsed preference was not saved")
	}
	require.True(t, app.modeLegendCollapsed())
}

func TestSetModeLegendCollapsedLogsSaveFailureAndKeepsState(t *testing.T) {
	done := make(chan struct{}, 1)
	app := &App{deps: &Dependencies{}}
	app.deps.HandlerDeps.SaveModeLegendCollapsed = func(context.Context, bool) error {
		defer func() { done <- struct{}{} }()
		return context.DeadlineExceeded
	}
	bw, view := newFakeModeToasterWindow("win-1")
	app.browserWindows = map[string]*browserWindow{bw.id: bw}
	bw.modeToaster.apply(input.ModeVim, legendOn("always"), "", testVimActions)

	app.setModeLegendCollapsed(context.Background(), true)
	<-done

	require.False(t, view.bodyShown, "a failed save does not revert the visible state")
}

func TestUpdateModeFrameUsesSharedCollapsedState(t *testing.T) {
	snapshot := entity.RuntimeConfigSnapshot{UI: entity.RuntimeUIConfig{Workspace: entity.WorkspaceConfig{
		Styling: legendOn("always"),
		VimMode: entity.VimModeConfig{Actions: testVimActions},
	}}}
	app := &App{runtimeConfig: runtimeConfigStateFromSnapshotForTest(snapshot)}
	require.True(t, app.modeLegend.click(true))
	bw, view := newFakeModeToasterWindow("win-1")

	app.updateModeFrame(bw, input.ModeVim)

	require.True(t, view.pillShown)
	require.False(t, view.bodyShown, "a window entering a mode honors the shared collapsed state")
	_, _ = app.modeLegend.next()
	_, _ = app.modeLegend.next()
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		pumpMainContext()
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not reached in time")
}
