// internal/core/engine.go
package core

import (
	"sync/atomic"

	portout "github.com/bnema/purego-cef/internal/ports/out"
)

// Engine is the core domain. It holds no global state — all state
// flows through the instance. The cef/ composition root creates one
// Engine at Init() time and holds it as a package-level variable.
type Engine struct {
	capi        portout.CAPI
	refs        *RefManager
	initialized atomic.Bool
}

// New creates an Engine with the given CAPI adapter.
func New(capi portout.CAPI) *Engine {
	e := &Engine{capi: capi}
	e.refs = NewRefManager(capi)
	return e
}

// Refs returns the engine's RefManager.
func (e *Engine) Refs() *RefManager {
	return e.refs
}
