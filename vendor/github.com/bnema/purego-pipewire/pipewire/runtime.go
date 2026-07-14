package pipewire

import (
	"errors"

	"github.com/bnema/purego-pipewire/internal/capi"
	"github.com/bnema/purego-pipewire/internal/core"
)

// runtimeWrapper wraps the internal core.Runtime to implement the public Runtime interface.
type runtimeWrapper struct {
	inner *core.Runtime
}

func (rw runtimeWrapper) Init() error {
	return rw.inner.Init()
}

func (rw runtimeWrapper) Deinit() {
	rw.inner.Deinit()
}

func (rw runtimeWrapper) NewMainLoop() (MainLoop, error) {
	ml, err := rw.inner.NewMainLoop()
	if err != nil {
		return nil, err
	}
	return mainLoopWrapper{inner: ml}, nil
}

// mainLoopWrapper wraps the internal core.MainLoop to implement the public MainLoop interface.
type mainLoopWrapper struct {
	inner *core.MainLoop
}

func (mlw mainLoopWrapper) Run() int32 {
	return mlw.inner.Run()
}

func (mlw mainLoopWrapper) Destroy() {
	mlw.inner.Destroy()
}

// Init initializes the PipeWire runtime and returns a Runtime instance.
// This function must be called before using any other PipeWire functionality.
// Returns an error if the PipeWire libraries cannot be loaded.
func Init() (Runtime, error) {
	if err := capi.Register(); err != nil {
		return nil, err
	}
	capiInstance := capi.Default()
	if capiInstance == nil {
		return nil, errors.New("failed to get default CAPI instance")
	}
	r := core.NewRuntime(capiInstance)
	if err := r.Init(); err != nil {
		return nil, err
	}
	return runtimeWrapper{inner: r}, nil
}
