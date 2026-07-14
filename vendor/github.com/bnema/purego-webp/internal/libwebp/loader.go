package libwebp

import (
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/bnema/purego"
)

var (
	loadOnce sync.Once
	loadErr  error
)

func EnsureLoaded() error {
	loadOnce.Do(func() {
		h, err := openLib()
		if err != nil {
			loadErr = err
			return
		}

		if err := registerAll(h); err != nil {
			loadErr = err
		}
	})

	return loadErr
}

func Available() bool {
	return EnsureLoaded() == nil
}

func register(lib uintptr, fnPtr interface{}, symbol string) error {
	addr, err := purego.Dlsym(lib, symbol)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", symbol, err)
	}
	purego.RegisterFunc(fnPtr, addr)
	return nil
}

// registerOptional resolves symbol from lib and registers fnPtr if found.
// Missing symbols are silently ignored; the function pointer is left nil.
func registerOptional(lib uintptr, fnPtr interface{}, symbol string) {
	addr, err := purego.Dlsym(lib, symbol)
	if err != nil {
		return
	}
	purego.RegisterFunc(fnPtr, addr)
}

// ValidateDecoderConfigAvailable reports whether WebPValidateDecoderConfig
// was found in the loaded libwebp. It was added in libwebp 1.6.0 (2025-03).
func ValidateDecoderConfigAvailable() bool {
	return EnsureLoaded() == nil && xWebPValidateDecoderConfig != nil
}

func openLib() (uintptr, error) {
	var errs []error
	for _, name := range candidateLibNames() {
		lib, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			return lib, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", name, err))
	}

	return 0, errors.Join(errs...)
}

func candidateLibNames() []string {
	switch runtime.GOOS {
	case "linux":
		return []string{"libwebp.so", "libwebp.so.8", "libwebp.so.7", "libwebp.so.6"}
	case "darwin":
		return []string{"libwebp.dylib"}
	case "windows":
		return []string{"libwebp.dll", "webp.dll"}
	default:
		return []string{"libwebp.so"}
	}
}
