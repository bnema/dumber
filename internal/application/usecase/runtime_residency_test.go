package usecase

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRuntimeResidencyDisabledQuitsOnLastWindow(t *testing.T) {
	r := NewRuntimeResidency(0)
	r.WindowOpened(time.Now())
	decision := r.WindowClosed(time.Now())
	require.Equal(t, ResidencyQuit, decision.Action)
	require.False(t, r.Armed())
}

func TestRuntimeResidencyNegativeTimeoutDisables(t *testing.T) {
	r := NewRuntimeResidency(-time.Second)
	r.WindowOpened(time.Now())
	require.Equal(t, ResidencyQuit, r.WindowClosed(time.Now()).Action)
}

func TestRuntimeResidencyFirstIdleArmsDeadline(t *testing.T) {
	now := time.Now()
	r := NewRuntimeResidency(time.Minute)
	r.WindowOpened(now)
	decision := r.WindowClosed(now)
	require.Equal(t, ResidencyArm, decision.Action)
	require.Equal(t, now.Add(time.Minute), decision.Deadline)
	require.True(t, r.Armed())
	gen := decision.Generation

	expiry := r.HandleExpiry(now.Add(2*time.Minute), gen)
	require.Equal(t, ResidencyQuit, expiry.Action)
	require.False(t, r.Armed())
}

func TestRuntimeResidencyReopenCancelsDeadline(t *testing.T) {
	now := time.Now()
	r := NewRuntimeResidency(time.Minute)
	r.WindowOpened(now)
	arm := r.WindowClosed(now)
	require.Equal(t, ResidencyArm, arm.Action)

	cancel := r.WindowOpened(now)
	require.Equal(t, ResidencyCancel, cancel.Action)
	require.False(t, r.Armed())

	stale := r.HandleExpiry(now.Add(2*time.Minute), arm.Generation)
	require.Equal(t, ResidencyNone, stale.Action)
}

func TestRuntimeResidencyFailedReopenRestoresIdle(t *testing.T) {
	now := time.Now()
	r := NewRuntimeResidency(time.Minute)
	r.WindowOpened(now)
	require.Equal(t, ResidencyArm, r.WindowClosed(now).Action)

	// Optimistic open followed by failure: the hold must not leak, idle
	// eligibility returns.
	r.WindowOpened(now)
	decision := r.OpenFailed(now)
	require.Equal(t, ResidencyArm, decision.Action)
	require.True(t, r.Armed())
}

func TestRuntimeResidencyPopupRemainingPreventsIdle(t *testing.T) {
	now := time.Now()
	r := NewRuntimeResidency(time.Minute)
	r.WindowOpened(now)
	r.WindowOpened(now)
	decision := r.WindowClosed(now)
	require.Equal(t, ResidencyNone, decision.Action)
	require.False(t, r.Armed())
}

func TestRuntimeResidencyDuplicateCloseIsIdempotent(t *testing.T) {
	r := NewRuntimeResidency(0)
	decision := r.WindowClosed(time.Now())
	require.Equal(t, ResidencyQuit, decision.Action)
	again := r.WindowClosed(time.Now())
	require.Equal(t, ResidencyQuit, again.Action)
}

func TestRuntimeResidencyStaleExpiryIgnored(t *testing.T) {
	now := time.Now()
	r := NewRuntimeResidency(time.Minute)
	r.WindowOpened(now)
	first := r.WindowClosed(now)
	r.WindowOpened(now)
	second := r.WindowClosed(now)
	require.NotEqual(t, first.Generation, second.Generation)

	require.Equal(t, ResidencyNone, r.HandleExpiry(now, first.Generation).Action)
	require.True(t, r.Armed())
	require.Equal(t, ResidencyQuit, r.HandleExpiry(now, second.Generation).Action)
}

func TestRuntimeResidencyExplicitQuitNeverWaits(t *testing.T) {
	now := time.Now()
	r := NewRuntimeResidency(time.Minute)
	r.WindowOpened(now)
	require.Equal(t, ResidencyArm, r.WindowClosed(now).Action)
	require.True(t, r.Armed())

	decision := r.ShutdownRequested()
	require.Equal(t, ResidencyQuit, decision.Action)
	require.False(t, r.Armed())
}

func TestRuntimeResidencyWorkBlocksIdleUntilDrained(t *testing.T) {
	now := time.Now()
	r := NewRuntimeResidency(time.Minute)
	r.WindowOpened(now)
	r.WorkStarted(now)
	decision := r.WindowClosed(now)
	require.Equal(t, ResidencyNone, decision.Action)
	require.False(t, r.Armed())

	drained := r.WorkFinished(now)
	require.Equal(t, ResidencyArm, drained.Action)
	require.True(t, r.Armed())
}

func TestRuntimeResidencyWorkCancelsArmedDeadline(t *testing.T) {
	now := time.Now()
	r := NewRuntimeResidency(time.Minute)
	r.WindowOpened(now)
	require.Equal(t, ResidencyArm, r.WindowClosed(now).Action)
	require.Equal(t, ResidencyCancel, r.WorkStarted(now).Action)
	require.False(t, r.Armed())
}
