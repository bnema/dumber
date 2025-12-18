// Package port defines interfaces for external dependencies.
package port

import (
	"context"
	"errors"
	"fmt"
)

// PkgConfigErrorKind describes the category of a pkg-config failure.
type PkgConfigErrorKind string

const (
	PkgConfigErrorKindCommandMissing PkgConfigErrorKind = "command_missing"
	PkgConfigErrorKindPackageMissing PkgConfigErrorKind = "package_missing"
	PkgConfigErrorKindUnknown        PkgConfigErrorKind = "unknown"
)

var (
	// ErrPkgConfigMissing indicates pkg-config is not available on the host.
	ErrPkgConfigMissing = errors.New("pkg-config missing")
	// ErrPkgConfigPackageMissing indicates the requested .pc package was not found.
	ErrPkgConfigPackageMissing = errors.New("pkg-config package missing")
)

// PkgConfigError wraps an error returned by pkg-config probing.
type PkgConfigError struct {
	Kind    PkgConfigErrorKind
	Package string
	Output  string
	Err     error
}

func (e *PkgConfigError) Error() string {
	if e == nil {
		return "pkg-config error"
	}
	msg := fmt.Sprintf("pkg-config (%s): %s", e.Kind, e.Package)
	if e.Output != "" {
		msg += ": " + e.Output
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *PkgConfigError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// RuntimeVersionProbe provides runtime dependency discovery.
//
// Implementations may use pkg-config and may apply a prefix override to influence
// pkg-config search paths.
type RuntimeVersionProbe interface {
	PkgConfigModVersion(ctx context.Context, pkgName string, prefix string) (string, error)
}
