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
	ctl, app, _, _ := newResidencyTestController(time.Minute)
	require.True(t, ctl.Enabled())
	ctl.AcquireHold(app)
	ctl.AcquireHold(app)
	require.Equal(t, 1, app.holds, "exactly one hold")
	ctl.ReleaseHold(app)
	ctl.ReleaseHold(app)
	require.Equal(t, 1, app.releases, "exactly one release")
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
}
