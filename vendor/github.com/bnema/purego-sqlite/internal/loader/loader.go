package loader

import (
	"errors"
	"fmt"
	"os"

	"github.com/bnema/purego"
)

var dlopen = purego.Dlopen

var defaultCandidates = []string{"libsqlite3.so", "libsqlite3.so.0"}

// Open loads libsqlite3 and returns the dlopen handle.
// SQLITE_LIB_PATH is an exclusive library path override. Without an override,
// Open tries the unversioned development name and then the runtime SONAME.
func Open() (uintptr, error) {
	if override := os.Getenv("SQLITE_LIB_PATH"); override != "" {
		return open(override)
	}

	errs := make([]error, 0, len(defaultCandidates))
	for _, candidate := range defaultCandidates {
		handle, err := open(candidate)
		if err == nil {
			return handle, nil
		}
		errs = append(errs, err)
	}
	return 0, fmt.Errorf("dlopen SQLite candidates: %w", errors.Join(errs...))
}

func open(lib string) (uintptr, error) {
	handle, err := dlopen(lib, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return 0, fmt.Errorf("dlopen %s: %w", lib, err)
	}
	return handle, nil
}
