package pipewire

// Context is the public interface for PipeWire context operations.
// Contexts manage resources and connections to the PipeWire server.
type Context interface {
	Destroy()
	Connect() (Core, error)
}
