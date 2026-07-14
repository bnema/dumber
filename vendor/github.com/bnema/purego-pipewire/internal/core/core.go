package core

import (
	"errors"
	"unsafe"
)

var ErrCoreConnect = errors.New("failed to connect core")
var ErrRegistryUnavailable = errors.New("registry unavailable")

type Core struct {
	ptr      unsafe.Pointer
	registry *Registry
}

func (c *Core) Disconnect() {
	// Placeholder for core disconnection
}

func (c *Core) Registry() (*Registry, error) {
	if c.registry == nil {
		return nil, ErrRegistryUnavailable
	}
	return c.registry, nil
}

func (c *Core) Ptr() unsafe.Pointer {
	return c.ptr
}
