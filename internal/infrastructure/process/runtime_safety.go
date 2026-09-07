// Package process provides dependency-leaf process-environment helpers.
//
// This file must not import bootstrap, desktop, or any other internal
// package: both cmd/dumber and internal/infrastructure/desktop import it, and
// bootstrap already imports desktop. Keep it to the standard library so the
// merge rule can be shared without an import cycle.
package process

import "strings"

// RuntimeSafetySetting is the GODEBUG key that must be set before any
// goroutine or CEF subprocess is created. Go stack shrinking can invalidate
// frames retained across foreign CEF/GTK stacks.
const RuntimeSafetySetting = "gcshrinkstackoff"

// EnvironmentValue returns the value for name in an environ slice.
func EnvironmentValue(environ []string, name string) string {
	prefix := name + "="
	for _, entry := range environ {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

// ReplaceEnvironmentValue returns a copy of environ with name set to value.
func ReplaceEnvironmentValue(environ []string, name, value string) []string {
	prefix := name + "="
	updated := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		if !strings.HasPrefix(entry, prefix) {
			updated = append(updated, entry)
		}
	}
	return append(updated, prefix+value)
}

// MergeRuntimeSafety returns a copy of environ with RuntimeSafetySetting=1
// merged into GODEBUG, preserving all unrelated entries (including other
// GODEBUG keys). It reports whether a change was applied. Duplicate or
// conflicting gcshrinkstackoff entries collapse to a single =1 entry.
func MergeRuntimeSafety(environ []string) ([]string, bool) {
	updated := append([]string(nil), environ...)
	parts := strings.Split(EnvironmentValue(updated, "GODEBUG"), ",")
	kept := make([]string, 0, len(parts)+1)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, _, _ := strings.Cut(part, "=")
		if key == RuntimeSafetySetting {
			continue
		}
		kept = append(kept, part)
	}
	merged := strings.Join(append(kept, RuntimeSafetySetting+"=1"), ",")
	if EnvironmentValue(updated, "GODEBUG") == merged {
		return updated, false
	}
	return ReplaceEnvironmentValue(updated, "GODEBUG", merged), true
}
