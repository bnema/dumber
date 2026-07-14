package pipewire

import (
	"unsafe"
)

// Core is the public interface for PipeWire core connection operations.
// A Core represents a connection to the PipeWire server.
type Core interface {
	Disconnect()
	Registry() (Registry, error)
	NewStream(name string, props unsafe.Pointer) (Stream, error)
}
