package cef

import (
	"testing"
	"time"
)

func TestNavigationFrameKick(t *testing.T) {
	now := time.Now()
	for _, tt := range []struct {
		name     string
		deadline time.Time
		loading  bool
		paints   int
		want     bool
	}{
		{"idle", time.Time{}, true, 0, false},
		{"loading after paint", now.Add(time.Second), true, 11, true},
		{"waiting for paint", now.Add(time.Second), false, 10, true},
		{"paint arrived", now.Add(time.Second), false, 11, false},
		{"deadline while loading", now, true, 10, false},
		{"expired", now.Add(-time.Second), false, 10, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			k := navigationFrameKick{deadline: tt.deadline, paints: 10}
			if got := k.active(now, tt.loading, tt.paints); got != tt.want {
				t.Fatalf("active = %v, want %v", got, tt.want)
			}
		})
	}
	for _, tt := range []struct {
		name string
		tick time.Time
		want bool
	}{
		{"no tick", time.Time{}, true},
		{"recent tick", now.Add(-49 * time.Millisecond), false},
		{"stale tick", now.Add(-50 * time.Millisecond), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			k := navigationFrameKick{lastTick: tt.tick}
			if got := k.fallbackDue(now); got != tt.want {
				t.Fatalf("fallbackDue = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStopBeginFrameLoopClearsNavigationKick(t *testing.T) {
	for _, destroyed := range []bool{false, true} {
		wv := &WebView{beginFrameKick: navigationFrameKick{deadline: time.Now().Add(time.Second), lastTick: time.Now(), paints: 3}}
		wv.destroyed.Store(destroyed)
		wv.stopBeginFrameLoop()
		wv.stopBeginFrameLoop()
		if wv.beginFrameKick != (navigationFrameKick{}) || wv.beginFrameFallbackID != 0 || wv.beginFrameClock != nil {
			t.Fatal("stop must clear navigation kick resources")
		}
	}
}
