package core

import (
	"fmt"
	"sync"

	portout "github.com/bnema/purego-pipewire/internal/ports/out"
)

type Runtime struct {
	mu          sync.Mutex
	capi        portout.CAPI
	initialized bool
}

var (
	pipewireMu     sync.Mutex
	pipewireRefCnt int
	pipewireCAPI   portout.CAPI
)

func NewRuntime(capi portout.CAPI) *Runtime {
	return &Runtime{capi: capi}
}

func (r *Runtime) Init() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.initialized {
		return nil
	}
	if err := initPipeWire(r.capi); err != nil {
		return err
	}
	r.initialized = true
	return nil
}

// initPipeWire initializes the process-level PipeWire API once.
func initPipeWire(capi portout.CAPI) error {
	pipewireMu.Lock()
	defer pipewireMu.Unlock()

	if capi == nil {
		return fmt.Errorf("runtime requires a non-nil CAPI implementation")
	}

	if pipewireRefCnt > 0 && pipewireCAPI != nil && capi != pipewireCAPI {
		return fmt.Errorf("runtime already initialized with a different CAPI implementation")
	}

	if pipewireRefCnt == 0 {
		pipewireCAPI = capi
		capi.PWInit(nil, nil)
	}
	pipewireRefCnt++
	return nil
}

func (r *Runtime) Deinit() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.initialized {
		return
	}
	r.initialized = false

	pipewireMu.Lock()
	defer pipewireMu.Unlock()

	if pipewireRefCnt == 0 {
		return
	}

	pipewireRefCnt--
	if pipewireRefCnt == 0 {
		pipewireCAPI.PWDeinit()
		pipewireCAPI = nil
	}
}
