package pipewire

import (
	"unsafe"
)

// Stream is the public interface for PipeWire stream operations.
// Streams are used to transfer audio/video data to and from PipeWire.
type Stream interface {
	Destroy()
	Connect(direction int, id uint32, flags uint32, params unsafe.Pointer) error
	Disconnect()
}
