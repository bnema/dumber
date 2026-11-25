//go:build linux && (amd64 || arm64)

package sigstack

import (
	"os"
	"os/signal"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRepairSignalOnStackRestoresFlag(t *testing.T) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, unix.SIGUSR2)
	defer signal.Stop(ch)

	var orig kernelSigaction
	if err := rtSigaction(int(unix.SIGUSR2), nil, &orig); err != nil {
		t.Fatalf("read: %v", err)
	}
	if orig.flags&saOnStackFlag == 0 {
		t.Fatalf("expected Go handler to use SA_ONSTACK, flags=%#x", orig.flags)
	}
	t.Cleanup(func() {
		o := orig
		if err := rtSigaction(int(unix.SIGUSR2), &o, nil); err != nil {
			t.Errorf("restore: %v", err)
		}
	})

	broken := orig
	broken.flags &^= saOnStackFlag
	if err := rtSigaction(int(unix.SIGUSR2), &broken, nil); err != nil {
		t.Fatalf("clear onstack: %v", err)
	}

	res := Repair()
	found := false
	for _, r := range res.Repaired {
		if r.Signal == int(unix.SIGUSR2) {
			found = true
			if r.Name != "SIGUSR2" || r.Handler != orig.handler {
				t.Fatalf("unexpected repair entry %+v", r)
			}
		}
	}
	if !found {
		t.Fatalf("SIGUSR2 not repaired: %+v", res)
	}

	var after kernelSigaction
	if err := rtSigaction(int(unix.SIGUSR2), nil, &after); err != nil {
		t.Fatalf("read after: %v", err)
	}
	if after != orig {
		t.Fatalf("after repair %+v, want %+v", after, orig)
	}
}

func TestRepairSkipsListedSignals(t *testing.T) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, unix.SIGUSR2)
	defer signal.Stop(ch)

	var orig kernelSigaction
	if err := rtSigaction(int(unix.SIGUSR2), nil, &orig); err != nil {
		t.Fatalf("read: %v", err)
	}
	t.Cleanup(func() {
		o := orig
		if err := rtSigaction(int(unix.SIGUSR2), &o, nil); err != nil {
			t.Errorf("restore: %v", err)
		}
	})

	broken := orig
	broken.flags &^= saOnStackFlag
	if err := rtSigaction(int(unix.SIGUSR2), &broken, nil); err != nil {
		t.Fatalf("clear onstack: %v", err)
	}

	res := Repair(int(unix.SIGUSR2))
	for _, r := range res.Repaired {
		if r.Signal == int(unix.SIGUSR2) {
			t.Fatalf("skipped signal was repaired: %+v", r)
		}
	}
	var after kernelSigaction
	if err := rtSigaction(int(unix.SIGUSR2), nil, &after); err != nil {
		t.Fatalf("read after: %v", err)
	}
	if after.flags&saOnStackFlag != 0 {
		t.Fatalf("skipped signal gained SA_ONSTACK: flags=%#x", after.flags)
	}
}
