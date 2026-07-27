package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

func environmentValue(environ []string, name string) string {
	prefix := name + "="
	for _, entry := range environ {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func replaceEnvironmentValue(environ []string, name, value string) []string {
	prefix := name + "="
	updated := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		if !strings.HasPrefix(entry, prefix) {
			updated = append(updated, entry)
		}
	}
	return append(updated, prefix+value)
}

func runtimeSafetyEnvironment(environ []string) ([]string, bool) {
	updated := append([]string(nil), environ...)
	const setting = "gcshrinkstackoff"
	parts := strings.Split(environmentValue(updated, "GODEBUG"), ",")
	kept := make([]string, 0, len(parts)+1)
	alreadySafe := false
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, found := strings.Cut(part, "=")
		if key == setting {
			alreadySafe = found && value == "1"
			continue
		}
		kept = append(kept, part)
	}
	if alreadySafe {
		return updated, false
	}
	kept = append(kept, setting+"=1")
	updated = replaceEnvironmentValue(updated, "GODEBUG", strings.Join(kept, ","))
	return updated, true
}

func ensureRuntimeSafety() error {
	environ, reexec := runtimeSafetyEnvironment(os.Environ())
	if !reexec {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable for runtime safety re-exec: %w", err)
	}
	if err := syscall.Exec(executable, os.Args, environ); err != nil {
		return fmt.Errorf("runtime safety re-exec: %w", err)
	}
	return nil
}
