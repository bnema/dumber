package capi

import (
	"sync"

	"github.com/bnema/purego-pipewire/internal/loader"
	portout "github.com/bnema/purego-pipewire/internal/ports/out"
)

// Compile-time check: defaultCAPI satisfies portout.CAPI.
var _ portout.CAPI = (*defaultCAPI)(nil)

var (
	defaultAPI   portout.CAPI
	registerOnce sync.Once
	registerErr  error
)

// Register loads the PipeWire libraries and registers all function bindings.
// It is safe to call from multiple goroutines; the initialization will only happen once.
func Register() error {
	registerOnce.Do(func() {
		h, err := loader.Open()
		if err != nil {
			registerErr = err
			return
		}
		if err := registerAll(h); err != nil {
			registerErr = err
			return
		}
		defaultAPI = newDefaultCAPI()
	})
	return registerErr
}

// Default returns the default CAPI instance after Register() has been called.
// Returns nil if Register() has not been called successfully.
func Default() portout.CAPI {
	return defaultAPI
}
