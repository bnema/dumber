package pipewire

// Runtime is the public interface for PipeWire runtime lifecycle management.
type Runtime interface {
	Init() error
	Deinit()
	NewMainLoop() (MainLoop, error)
}

// MainLoop is the public interface for PipeWire main loop operations.
type MainLoop interface {
	Run() int32
	Destroy()
}
