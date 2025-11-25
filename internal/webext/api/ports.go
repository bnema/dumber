package api

// PortCallbacks are invoked when a background port sends data or disconnects.
type PortCallbacks struct {
	OnMessage    func(interface{})
	OnDisconnect func()
}

// PortDescriptor describes a port connection request to the background context.
type PortDescriptor struct {
	ID        string
	Name      string
	Sender    MessageSender
	Callbacks PortCallbacks
}
