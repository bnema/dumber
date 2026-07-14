package loader

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bnema/purego"
)

type Handles struct {
	PipeWire uintptr
}

// Open loads the PipeWire library with RTLD_GLOBAL so plugins can resolve symbols.
// Falls back to system library paths unless PIPEWIRE_LIB_PATH is set.
func Open() (Handles, error) {
	pw, err := openOne(resolveLib("PIPEWIRE_LIB_PATH", libFileName("libpipewire-0.3", "0")))
	if err != nil {
		return Handles{}, err
	}
	return Handles{PipeWire: pw}, nil
}

func openOne(path string) (uintptr, error) {
	// RTLD_GLOBAL is required: PipeWire plugins need to resolve symbols from the main library.
	h, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return 0, fmt.Errorf("dlopen %s: %w", path, err)
	}
	return h, nil
}

func resolveLib(envName, fallback string) string {
	if v := os.Getenv(envName); v != "" {
		return v
	}
	return fallback
}

func libFileName(base, soversion string) string {
	return filepath.Base(base) + ".so." + soversion
}
