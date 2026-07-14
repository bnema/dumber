package pipewire

import (
	"unsafe"
)

// Registry is the public interface for PipeWire registry operations.
// The registry is used to discover and bind to global objects.
type Registry interface {
	Destroy()
	Bind(name uint32, t string, version uint32) (unsafe.Pointer, error)
}
