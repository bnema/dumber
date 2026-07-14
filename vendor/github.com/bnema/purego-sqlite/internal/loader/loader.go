package loader

import (
	"fmt"
	"os"

	"github.com/bnema/purego"
)

// Open loads libsqlite3 and returns the dlopen handle.
// Override the library path with SQLITE_LIB_PATH env var.
func Open() (uintptr, error) {
	lib := os.Getenv("SQLITE_LIB_PATH")
	if lib == "" {
		lib = "libsqlite3.so"
	}
	handle, err := purego.Dlopen(lib, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return 0, fmt.Errorf("dlopen %s: %w", lib, err)
	}
	return handle, nil
}
