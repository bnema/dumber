//go:build linux && (amd64 || arm64)

package cef

import (
	"errors"
	"fmt"
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

// repairSignalOnStack adds SA_ONSTACK to every installed handler that lacks
// it, preserving handler, mask, restorer and other flags.
func repairSignalOnStack() signalOnStackResult {
	var res signalOnStackResult
	actions := make(map[int]kernelSigaction, 64)
	states := make([]signalHandlerState, 0, 64)
	for sig := 1; sig <= 64; sig++ {
		if sig == int(unix.SIGKILL) || sig == int(unix.SIGSTOP) {
			continue
		}
		var old kernelSigaction
		if err := rtSigaction(sig, nil, &old); err != nil {
			if errors.Is(err, unix.EINVAL) {
				continue
			}
			res.Errors = append(res.Errors, fmt.Errorf("read signal %d: %w", sig, err))
			continue
		}
		actions[sig] = old
		states = append(states, signalHandlerState{Signal: sig, Handler: old.handler, Flags: old.flags})
	}
	for _, st := range signalsNeedingOnStack(states) {
		act := actions[st.Signal]
		act.flags = st.Flags
		if err := rtSigaction(st.Signal, &act, nil); err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("repair signal %d: %w", st.Signal, err))
			continue
		}
		res.Repaired = append(res.Repaired, repairedSignal{
			Signal:  st.Signal,
			Name:    unix.SignalName(unix.Signal(st.Signal)),
			Handler: st.Handler,
		})
	}
	return res
}
