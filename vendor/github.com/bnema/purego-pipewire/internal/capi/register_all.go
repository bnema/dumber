package capi

import "github.com/bnema/purego-pipewire/internal/loader"

// registerAll registers all PipeWire symbol bindings.
// This is the central registration point; add new module registrations here.
func registerAll(h loader.Handles) error {
	registerInit(h.PipeWire)
	registerLoop(h.PipeWire)
	registerLoopLock(h.PipeWire)
	registerStream_playback(h.PipeWire)
	return nil
}
