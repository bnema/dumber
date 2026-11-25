//go:build linux && (amd64 || arm64)

package sigstack

import (
	"errors"
	"fmt"
	"slices"
	"unsafe"

	"golang.org/x/sys/unix"
)

// kernelSigaction matches the kernel rt_sigaction struct on linux/amd64 and
// linux/arm64 (same layout as runtime.sigactiont in defs_linux_{amd64,arm64}.go).
type kernelSigaction struct {
	handler  uintptr
	flags    uint64
	restorer uintptr
	mask     uint64
}

const kernelSigsetSize = 8

func rtSigaction(sig int, act, old *kernelSigaction) error {
	_, _, errno := unix.RawSyscall6(unix.SYS_RT_SIGACTION, uintptr(sig),
		uintptr(unsafe.Pointer(act)), uintptr(unsafe.Pointer(old)), kernelSigsetSize, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

// Repair adds SA_ONSTACK to every installed handler that lacks it, preserving
// handler, mask, restorer and other flags. Signals listed in skip are left
// untouched, for handlers that must run on the interrupted stack.
//
// Each signal is read and written back immediately, like runtime.setsigstack.
// The read-modify-write is not atomic: a handler installed by another thread
// between the two syscalls is overwritten, the same race the Go runtime accepts.
func Repair(skip ...int) Result {
	var res Result
	for sig := 1; sig <= 64; sig++ {
		if sig == int(unix.SIGKILL) || sig == int(unix.SIGSTOP) || slices.Contains(skip, sig) {
			continue
		}
		var act kernelSigaction
		if err := rtSigaction(sig, nil, &act); err != nil {
			if !errors.Is(err, unix.EINVAL) {
				res.Errors = append(res.Errors, fmt.Errorf("read signal %d: %w", sig, err))
			}
			continue
		}
		if !needsOnStack(act.handler, act.flags) {
			continue
		}
		act.flags |= saOnStackFlag
		if err := rtSigaction(sig, &act, nil); err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("repair signal %d: %w", sig, err))
			continue
		}
		res.Repaired = append(res.Repaired, Repaired{
			Signal:  sig,
			Name:    unix.SignalName(unix.Signal(sig)),
			Handler: act.handler,
		})
	}
	return res
}
