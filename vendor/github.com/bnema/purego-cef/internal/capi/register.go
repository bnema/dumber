package capi

import "github.com/bnema/purego"

// tryRegisterLibFunc registers a C function symbol if it exists in the library.
// Unlike purego.RegisterLibFunc, it silently skips symbols not found in the
// shared library, which is essential because optional CEF APIs may vary across
// builds and versions.
func tryRegisterLibFunc(fptr any, handle uintptr, name string) {
	sym, err := purego.Dlsym(handle, name)
	if err != nil {
		return // symbol not available in this build
	}
	purego.RegisterFunc(fptr, sym)
}
