package main

import (
	"fmt"
	"os"
	"syscall"

	"github.com/bnema/dumber/internal/infrastructure/process"
)

func environmentValue(environ []string, name string) string {
	return process.EnvironmentValue(environ, name)
}

func replaceEnvironmentValue(environ []string, name, value string) []string {
	return process.ReplaceEnvironmentValue(environ, name, value)
}

func runtimeSafetyEnvironment(environ []string) ([]string, bool) {
	return process.MergeRuntimeSafety(environ)
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
	//nolint:gosec // G702: os.Executable and the current process argv are required for self-re-exec.
	if err := syscall.Exec(executable, os.Args, environ); err != nil {
		return fmt.Errorf("runtime safety re-exec: %w", err)
	}
	return nil
}
