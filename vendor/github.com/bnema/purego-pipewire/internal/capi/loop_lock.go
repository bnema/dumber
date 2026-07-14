package capi

import (
	"unsafe"

	"github.com/bnema/purego"
)

// pw_loop_lock/pw_loop_unlock are kept as handwritten internal bindings
// because they are only used inside capi to satisfy PipeWire's thread/locking
// requirements for stream lifecycle calls. Exposing them through generated
// ports would leak a low-level implementation detail upward.
type pwLoopControlFunc func(loop unsafe.Pointer) int32

var (
	pw_loop_lock   pwLoopControlFunc
	pw_loop_unlock pwLoopControlFunc
)

func registerLoopLock(handle uintptr) {
	purego.RegisterLibFunc(&pw_loop_lock, handle, "pw_loop_lock")
	purego.RegisterLibFunc(&pw_loop_unlock, handle, "pw_loop_unlock")
}

var withLoopLock = defaultWithLoopLock

func defaultWithLoopLock(loop unsafe.Pointer, fn func() int32) int32 {
	if loop == nil || pw_loop_lock == nil || pw_loop_unlock == nil {
		return -1
	}
	if ret := pw_loop_lock(loop); ret < 0 {
		return ret
	}
	ret := fn()
	unlockRet := pw_loop_unlock(loop)
	if ret >= 0 && unlockRet < 0 {
		return unlockRet
	}
	return ret
}
