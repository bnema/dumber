package process

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAdmissionGateAcquireRelease(t *testing.T) {
	gate := NewAdmissionGate()
	require.True(t, gate.Acquire())
	require.True(t, gate.Acquire())
	require.Equal(t, 2, gate.Active())
	gate.Release()
	require.Equal(t, 1, gate.Active())
	gate.Release()
	require.Equal(t, 0, gate.Active())
}

func TestAdmissionGateCloseStopsAdmission(t *testing.T) {
	gate := NewAdmissionGate()
	require.True(t, gate.Acquire())
	gate.Close()
	require.True(t, gate.Closed())
	require.False(t, gate.Acquire(), "closed gate must refuse new leases")
	require.Equal(t, 1, gate.Active(), "in-flight lease survives close")
	gate.Release()
	require.Equal(t, 0, gate.Active())
}

func TestAdmissionGateWaitDrained(t *testing.T) {
	gate := NewAdmissionGate()
	gate.Acquire()
	done := make(chan bool, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		done <- gate.WaitDrained(ctx)
	}()
	select {
	case <-done:
		t.Fatal("drain must not complete while a lease is held")
	case <-time.After(50 * time.Millisecond):
	}
	gate.Release()
	select {
	case drained := <-done:
		require.True(t, drained)
	case <-time.After(5 * time.Second):
		t.Fatal("drain must complete after release")
	}
}

func TestAdmissionGateWaitDrainedExpiry(t *testing.T) {
	gate := NewAdmissionGate()
	gate.Acquire()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.False(t, gate.WaitDrained(ctx), "expiry must not hang shutdown")
	gate.Release()
}

func TestAdmissionGateOnDrained(t *testing.T) {
	gate := NewAdmissionGate()
	var mu sync.Mutex
	calls := 0
	gate.OnDrained(func() {
		mu.Lock()
		calls++
		mu.Unlock()
	})
	gate.Acquire()
	gate.Release()
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, calls)
}

func TestAdmissionGateConcurrent(t *testing.T) {
	gate := NewAdmissionGate()
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if gate.Acquire() {
				gate.Release()
			}
		}()
	}
	wg.Wait()
	require.Equal(t, 0, gate.Active())
}
