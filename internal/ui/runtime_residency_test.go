package ui

import (
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/stretchr/testify/require"
)

func TestResidencyTimeoutFromMillis(t *testing.T) {
	require.Equal(t, time.Duration(0), ResidencyTimeoutFromMillis(0))
	require.Equal(t, time.Duration(0), ResidencyTimeoutFromMillis(-5))
	require.Equal(t, 60*time.Second, ResidencyTimeoutFromMillis(60000))
	require.Equal(t, 300000*time.Millisecond, ResidencyTimeoutFromMillis(300000))
	require.Equal(t, 300000*time.Millisecond, ResidencyTimeoutFromMillis(999999))
}

func TestResidencyTimeoutLatchAndRestartRequired(t *testing.T) {
	app := &App{}
	require.Equal(t, time.Duration(0), app.LatchedResidencyTimeout())

	app.SetResidencyTimeout(60 * time.Second)
	require.Equal(t, 60*time.Second, app.LatchedResidencyTimeout())
	require.False(t, app.ResidencyTimeoutRestartRequired(60000))
	require.True(t, app.ResidencyTimeoutRestartRequired(0))
	require.True(t, app.ResidencyTimeoutRestartRequired(120000))

	// A nil app never panics and reports no change.
	var nilApp *App
	nilApp.SetResidencyTimeout(time.Minute)
	require.Equal(t, time.Duration(0), nilApp.LatchedResidencyTimeout())
	require.False(t, nilApp.ResidencyTimeoutRestartRequired(60000))
}

type residencyFakeApp struct {
	holds    int
	releases int
	quits    int
}

func (f *residencyFakeApp) Hold()    { f.holds++ }
func (f *residencyFakeApp) Release() { f.releases++ }
func (f *residencyFakeApp) Quit()    { f.quits++ }

type residencyFakeScheduler struct {
	scheduled   []time.Duration
	generations []uint64
	cancels     int
}

func (f *residencyFakeScheduler) schedule(d time.Duration, generation uint64) {
	f.scheduled = append(f.scheduled, d)
	f.generations = append(f.generations, generation)
}

func (f *residencyFakeScheduler) cancel() { f.cancels++ }

func newResidencyTestController(timeout time.Duration) (*ResidencyController, *residencyFakeApp, *residencyFakeScheduler, *int) {
	now := time.Now()
	clock := func() time.Time { return now }
	app := &residencyFakeApp{}
	sched := &residencyFakeScheduler{}
	quits := 0
	ctl := NewResidencyController(timeout, clock, sched.schedule, sched.cancel, func() { quits++ })
	return ctl, app, sched, &quits
}

func TestResidencyControllerDisabledPreservesQuit(t *testing.T) {
	ctl, app, sched, quits := newResidencyTestController(0)
	require.False(t, ctl.Enabled())
	ctl.AcquireHold(app)
	require.Equal(t, 0, app.holds, "disabled residency takes no hold")
	ctl.ReleaseHold(app)
	require.Equal(t, 0, app.releases)

	ctl.SetWindowCount(1)
	decision := ctl.SetWindowCount(0)
	require.Equal(t, usecase.ResidencyQuit, decision.Action)
	require.Equal(t, 1, *quits)
	require.Empty(t, sched.scheduled)
}

func TestResidencyControllerHoldMatchesReleaseOnce(t *testing.T) {
	ctl, app, _, quits := newResidencyTestController(time.Minute)
	require.True(t, ctl.Enabled())
	ctl.AcquireHold(app)
	ctl.AcquireHold(app)
	require.Equal(t, 1, app.holds, "exactly one hold")
	ctl.ReleaseHold(app)
	ctl.ReleaseHold(app)
	require.Equal(t, 1, app.releases, "exactly one release")
	require.Equal(t, 0, *quits, "hold cycling never quits")
}

func TestResidencyControllerLastWindowArmsAndExpiryQuits(t *testing.T) {
	ctl, _, sched, quits := newResidencyTestController(time.Minute)
	ctl.SetWindowCount(1)
	decision := ctl.SetWindowCount(0)
	require.Equal(t, usecase.ResidencyArm, decision.Action)
	require.NotEmpty(t, sched.scheduled)
	armed := sched.generations[len(sched.generations)-1]
	ctl.OnTimerFired(armed)
	require.Equal(t, 1, *quits)
}

func TestResidencyControllerReopenCancelsTimer(t *testing.T) {
	ctl, _, sched, _ := newResidencyTestController(time.Minute)
	ctl.SetWindowCount(1)
	ctl.SetWindowCount(0)
	require.NotEmpty(t, sched.scheduled)
	armed := sched.generations[len(sched.generations)-1]
	cancels := sched.cancels
	ctl.SetWindowCount(1)
	require.Greater(t, sched.cancels, cancels, "reopen cancels the armed deadline")
	ctl.OnTimerFired(armed)
}

func TestResidencyControllerExplicitQuitBypassesDeadline(t *testing.T) {
	ctl, _, sched, quits := newResidencyTestController(time.Minute)
	ctl.SetWindowCount(1)
	ctl.SetWindowCount(0)
	require.NotEmpty(t, sched.scheduled)
	armed := sched.generations[len(sched.generations)-1]
	ctl.NoteExplicitQuit()
	require.Equal(t, 0, *quits, "noting quit never invokes quit itself")
	ctl.OnTimerFired(armed)
	require.Equal(t, 0, *quits, "deadline never fires after explicit quit")
}

func TestResidencyControllerNilSafe(t *testing.T) {
	var ctl *ResidencyController
	require.False(t, ctl.Enabled())
	require.Equal(t, usecase.ResidencyQuit, ctl.SetWindowCount(0).Action)
	require.Equal(t, usecase.ResidencyQuit, ctl.Evaluate().Action)
	ctl.NoteExplicitQuit()
	ctl.OnTimerFired(7)
	ctl.AcquireHold(nil)
	ctl.ReleaseHold(nil)
	ctl.SetQuiescentFunc(nil)
	ctl.SetDispatchToGTK(nil)
	ctl.DispatchToGTK(nil)
	ctl.MaybeQuit()
	require.Equal(t, usecase.ResidencyNone, ctl.SetNativeBusy("x", true).Action)
}

func TestResidencyControllerDefersQuitUntilSettled(t *testing.T) {
	ctl, _, sched, quits := newResidencyTestController(time.Minute)
	quiescent := true
	ctl.SetQuiescentFunc(func() bool { return quiescent })
	ctl.SetWindowCount(1)
	decision := ctl.SetWindowCount(0)
	require.Equal(t, usecase.ResidencyArm, decision.Action)
	armed := sched.generations[len(sched.generations)-1]

	quiescent = false
	ctl.OnTimerFired(armed)
	require.Equal(t, 0, *quits, "expiry defers while native work is outstanding")

	quiescent = true
	ctl.MaybeQuit()
	require.Equal(t, 1, *quits, "settle recheck commits the deferred quit")
	ctl.MaybeQuit()
	require.Equal(t, 1, *quits, "quit commits exactly once")
}

func TestResidencyControllerNativeBusyDrivesPolicy(t *testing.T) {
	ctl, _, sched, quits := newResidencyTestController(time.Minute)
	ctl.SetWindowCount(1)

	decision := ctl.SetNativeBusy("cef-activity", true)
	require.Equal(t, usecase.ResidencyNone, decision.Action)
	decision = ctl.SetWindowCount(0)
	require.Equal(t, usecase.ResidencyNone, decision.Action, "close with native work outstanding arms nothing")
	require.Empty(t, sched.scheduled)

	decision = ctl.SetNativeBusy("cef-activity", false)
	require.Equal(t, usecase.ResidencyArm, decision.Action, "settle re-arms idle")
	require.NotEmpty(t, sched.scheduled)
	require.Equal(t, 0, *quits)
}

func TestResidencyControllerDuplicateBusyIsSilent(t *testing.T) {
	ctl, _, sched, _ := newResidencyTestController(time.Minute)
	ctl.SetNativeBusy("downloads", true)
	decision := ctl.SetNativeBusy("downloads", true)
	require.Equal(t, usecase.ResidencyNone, decision.Action)
	require.Empty(t, sched.scheduled)
}

func TestResidencyControllerDispatchRoutesCallbacks(t *testing.T) {
	ctl, _, _, quits := newResidencyTestController(time.Minute)
	var routed []string
	ctl.SetDispatchToGTK(func(fn func()) {
		routed = append(routed, "gtk")
		fn()
	})
	ctl.DispatchToGTK(func() { routed = append(routed, "cb") })
	require.Equal(t, []string{"gtk", "cb"}, routed)
	require.Equal(t, 0, *quits)
}

func TestResidencyControllerSealsBeforeCommit(t *testing.T) {
	ctl, _, sched, quits := newResidencyTestController(time.Minute)
	sealed := 0
	ctl.SetSealFunc(func() { sealed++ })
	ctl.SetQuiescentFunc(func() bool { return true })
	ctl.SetWindowCount(1)
	ctl.SetWindowCount(0)
	armed := sched.generations[len(sched.generations)-1]
	// Quiescent expiry seals admission, then commits the quit.
	ctl.OnTimerFired(armed)
	require.Equal(t, 1, sealed)
	require.Equal(t, 1, *quits)
}

func TestResidencyControllerReopenInvalidatesDeferredQuit(t *testing.T) {
	ctl, _, sched, quits := newResidencyTestController(time.Minute)
	quiescent := true
	ctl.SetQuiescentFunc(func() bool { return quiescent })
	ctl.SetWindowCount(1)
	ctl.SetWindowCount(0)
	armed := sched.generations[len(sched.generations)-1]
	quiescent = false
	ctl.OnTimerFired(armed)
	require.Equal(t, 0, *quits, "quit defers while unsettled")
	// A reopened window invalidates the deferral.
	ctl.SetWindowCount(1)
	quiescent = true
	ctl.MaybeQuit()
	require.Equal(t, 0, *quits, "obsolete quit must not run after reopen")
}

func TestResidencyControllerDuplicateEmptyStaysArmed(t *testing.T) {
	ctl, _, sched, quits := newResidencyTestController(time.Minute)
	ctl.SetWindowCount(1)
	first := ctl.SetWindowCount(0)
	require.Equal(t, usecase.ResidencyArm, first.Action)
	require.Len(t, sched.scheduled, 1)
	// A repeated empty update (e.g. tab-empty after window removal)
	// stays on the armed deadline instead of re-arming.
	again := ctl.SetWindowCount(0)
	require.Equal(t, usecase.ResidencyNone, again.Action)
	require.Len(t, sched.scheduled, 1, "no duplicate timer")
	require.Equal(t, 0, *quits)
}
