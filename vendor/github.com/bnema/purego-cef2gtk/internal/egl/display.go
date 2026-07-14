package egl

import (
	"errors"
	"fmt"
)

const (
	queryVendor     int32 = 0x3053
	queryVersion    int32 = 0x3054
	queryExtensions int32 = 0x3055
	queryClientAPIs int32 = 0x308D
)

var (
	ErrNoCurrentDisplay      = errors.New("no current EGL display")
	ErrMissingDisplaySupport = errors.New("missing EGL display support")
)

// DisplayInfo describes the EGL display associated with the current thread's context.
type DisplayInfo struct {
	Display         Display
	Vendor          string
	Version         string
	ClientAPIs      string
	ExtensionString string
	Extensions      Extensions
}

// CurrentDisplayInfo returns information for eglGetCurrentDisplay(). It does not create a display.
func CurrentDisplayInfo() (DisplayInfo, error) {
	b, err := defaultBackend()
	if err != nil {
		return DisplayInfo{}, err
	}
	return currentDisplayInfo(b)
}

func currentDisplayInfo(b backend) (DisplayInfo, error) {
	d := b.currentDisplay()
	if !d.Valid() {
		return DisplayInfo{}, ErrNoCurrentDisplay
	}
	extString := b.queryString(d, queryExtensions)
	return DisplayInfo{
		Display:         d,
		Vendor:          b.queryString(d, queryVendor),
		Version:         b.queryString(d, queryVersion),
		ClientAPIs:      b.queryString(d, queryClientAPIs),
		ExtensionString: extString,
		Extensions:      ParseExtensions(extString),
	}, nil
}

func (i DisplayInfo) HasExtension(name string) bool { return i.Extensions.Has(name) }

func (i DisplayInfo) RequireExtensions(required ...string) error {
	if !i.Display.Valid() {
		return ErrNoCurrentDisplay
	}
	missing := i.Extensions.Missing(required...)
	if len(missing) > 0 {
		return fmt.Errorf("%w: %v", ErrMissingDisplaySupport, missing)
	}
	return nil
}

func (i DisplayInfo) SupportsDMABUFImport() bool {
	return i.HasExtension(ExtensionDMABUFImport)
}
