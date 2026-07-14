package core

import (
	"errors"
	"unsafe"

	portout "github.com/bnema/purego-pipewire/internal/ports/out"
)

var ErrMainLoopCreate = errors.New("failed to create main loop")

type MainLoop struct {
	ptr  unsafe.Pointer
	capi portout.CAPI
}

func (r *Runtime) NewMainLoop() (*MainLoop, error) {
	ptr := r.capi.PWMainLoopNew(nil)
	if ptr == nil {
		return nil, ErrMainLoopCreate
	}
	return &MainLoop{ptr: ptr, capi: r.capi}, nil
}

func (ml *MainLoop) Run() int32 {
	return ml.capi.PWMainLoopRun(ml.ptr)
}

func (ml *MainLoop) Destroy() {
	ml.capi.PWMainLoopDestroy(ml.ptr)
}

func (ml *MainLoop) Ptr() unsafe.Pointer {
	return ml.ptr
}
