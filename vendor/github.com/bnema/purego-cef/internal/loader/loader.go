// internal/loader/loader.go
package loader

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/bnema/purego"
)

const defaultCEFVersion = 147
const cefVersionInfoChromeMajor = 4

var (
	userHomeDir         = os.UserHomeDir
	systemCEFRuntimeDir = "/usr/lib/cef"
	pathExists          = defaultPathExists
)

// Open loads libcef.so and validates the CEF version.
// The returned handle is used to register all C API symbols.
func Open(cefDir string) (uintptr, error) {
	runtimeDir, err := resolveDir(cefDir)
	if err != nil {
		return 0, err
	}
	libPath := filepath.Join(runtimeDir, "libcef.so")
	handle, err := purego.Dlopen(libPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return 0, fmt.Errorf("dlopen %s: %w", libPath, err)
	}
	ok := false
	defer func() {
		if !ok {
			purego.Dlclose(handle)
		}
	}()
	if os.Getenv("CEF_SKIP_VERSION_CHECK") != "1" {
		if err := validateVersion(handle); err != nil {
			return 0, err
		}
	}
	if err := configureAPIVersion(handle); err != nil {
		return 0, err
	}
	ok = true
	return handle, nil
}

func resolveDir(arg string) (string, error) {
	if env := os.Getenv("CEF_DIR"); env != "" {
		return env, nil
	}
	if arg != "" {
		return arg, nil
	}
	if pathExists(filepath.Join(systemCEFRuntimeDir, "libcef.so")) {
		return systemCEFRuntimeDir, nil
	}
	home, err := userHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".local", "share", "cef"), nil
}

func defaultPathExists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, os.ErrNotExist)
}

func targetMajor() int32 {
	if raw := os.Getenv("CEF_VERSION"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 32); err == nil && n >= 0 {
			return int32(n)
		}
	}
	return defaultCEFVersion
}

// configureAPIVersion calls cef_api_hash to configure the API version.
// CEF 133+ requires this before cef_initialize — without it,
// cef_api_version() returns -1 and every versioned CToCpp wrapper FATALs.
func configureAPIVersion(handle uintptr) error {
	sym, err := purego.Dlsym(handle, "cef_api_hash")
	if err != nil {
		return fmt.Errorf("resolve cef_api_hash: %w", err)
	}
	var apiHash func(int32, int32) uintptr
	purego.RegisterFunc(&apiHash, sym)
	// 999999 = CEF_API_VERSION_EXPERIMENTAL (use all available API).
	// entry 0 = CEF_API_HASH_PLATFORM; return value ignored.
	apiHash(999999, 0)
	return nil
}

func validateVersion(handle uintptr) error {
	sym, err := purego.Dlsym(handle, "cef_version_info")
	if err != nil {
		return fmt.Errorf("resolve cef_version_info: %w", err)
	}
	var versionInfo func(int32) int32
	purego.RegisterFunc(&versionInfo, sym)
	return ensureMinimumVersion(versionInfo(cefVersionInfoChromeMajor), targetMajor())
}

func ensureMinimumVersion(got, minimum int32) error {
	if got < minimum {
		return fmt.Errorf("unsupported CEF runtime: chrome major=%d minimum=%d", got, minimum)
	}
	return nil
}
