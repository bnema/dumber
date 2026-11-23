//go:build stress

package stress

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestStressLoop(t *testing.T) {
	requireTools(t)
	duration := defaultDuration
	if v := os.Getenv("DUMBER_STRESS_DURATION"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatalf("DUMBER_STRESS_DURATION: %v", err)
		}
		duration = d
	}
	if dl, ok := t.Deadline(); ok {
		if need := duration + sessionTTLExtra; time.Until(dl) < need {
			t.Fatalf("test timeout leaves %s, need at least %s (duration + %s): raise -timeout",
				time.Until(dl).Round(time.Second), need, sessionTTLExtra)
		}
	}
	seed := uint64(time.Now().UnixNano())
	if v := os.Getenv("DUMBER_STRESS_SEED"); v != "" {
		s, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			t.Fatalf("DUMBER_STRESS_SEED: %v", err)
		}
		seed = s
	}
	t.Logf("seed=%d duration=%s", seed, duration)
	artifacts := filepath.Join("artifacts", time.Now().UTC().Format(artifactLayout))

	r := &stressRun{
		t: t, seed: seed, duration: duration, start: time.Now(),
		rng: rand.New(rand.NewPCG(seed, seed>>1)), tabs: 1, panes: 1,
	}
	r.ps = newPageServer(t)
	r.sb = newSandbox(t)
	r.logDir = filepath.Join(r.sb.root, "out")
	// Registered before launch so a failed launch still leaves a summary.
	t.Cleanup(func() {
		if r.sb.bin != "" {
			stopAll(t, r.sb.bin)
		}
		if t.Failed() {
			r.collect(artifacts)
		}
	})
	r.sb.bin = buildDumber(t, r.sb.root)
	startNeferwl(t, r.sb, sessionID(t), duration+sessionTTLExtra)
	r.proc = launchDumber(t, r.sb, r.ps.url(hostIP, "/page"), r.logDir)
	t.Logf("dumber pid=%d sandbox=%s", r.pid(), r.sb.root)

	if elapsed, ok := r.shotCheck("startup", startupRetries); ok {
		r.firstPaint = elapsed
		t.Logf("first_paint_s=%.1f", elapsed.Seconds())
		if elapsed > slowFirstPaint {
			t.Logf("WARN slow first paint: %.1fs > %s (product issue, not a failure)", elapsed.Seconds(), slowFirstPaint)
		}
	}
	if r.died {
		r.assertLogs()
		return
	}
	r.loop()
	if r.died {
		r.assertLogs()
		return
	}
	if !r.proc.alive() {
		r.failf("dumber died before the end navigation (after %d actions, last %q)", r.actions, r.last)
		r.assertLogs()
		return
	}
	r.navigate(hostIP, "/page")
	r.shotCheck("end", shotRetries)
	time.Sleep(metricsWait)
	if !r.proc.alive() {
		r.failf("dumber died before SIGTERM (after %d actions, last %q)", r.actions, r.last)
	} else if killed, err := stopDumber(r.proc, stopTimeout); err != nil {
		r.failf("shutdown hang: %v", err)
	} else if killed {
		r.failf("shutdown hang: dumber still alive %s after SIGTERM", stopTimeout)
	}
	r.assertLogs()
	t.Logf("actions=%d", r.actions)
}
