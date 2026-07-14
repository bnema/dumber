package cef

import (
	"errors"
	"os"
	"path/filepath"
)

const nextStartSafetyMarkerFile = "cef-next-start-safe"

// writeNextStartSafetyMarker records a one-shot recovery request for the next
// CEF start. The marker lives beside CEF's root cache so it follows explicit
// per-session cache roots as well as the normal profile.
func writeNextStartSafetyMarker(stateRoot string) error {
	if stateRoot == "" {
		return errors.New("CEF state root is empty")
	}
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		return err
	}

	marker := filepath.Join(stateRoot, nextStartSafetyMarkerFile)
	temporary, err := os.CreateTemp(stateRoot, ".cef-next-start-safe-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.WriteString("safe\n"); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, marker)
}

// consumeNextStartSafetyMarker consumes a one-shot marker. It deliberately
// does not follow symlinks, so an untrusted cache entry cannot redirect the
// recovery check outside the CEF state root.
func consumeNextStartSafetyMarker(stateRoot string) bool {
	if stateRoot == "" {
		return false
	}
	marker := filepath.Join(stateRoot, nextStartSafetyMarkerFile)
	info, err := os.Lstat(marker)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return os.Remove(marker) == nil
}

// applyNextStartSafetyEnvironment leaves the Vulkan render stack intact while
// returning Chromium to its hardware-decode safety defaults for one start.
func applyNextStartSafetyEnvironment() {
	_ = os.Setenv(cefEnableVAAPIEnvVar, "0")
	_ = os.Unsetenv(cefChromiumFlagsEnvVar)
	_ = os.Unsetenv(cefRenderNodeEnvVar)
}
