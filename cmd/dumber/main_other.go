//go:build !linux && !darwin

package main

import "context"

func enableCrashForensics() {}

func logCoreDumpLimits(_ context.Context) {}
