// Package usecase contains application business logic.
package usecase

import (
	"context"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

const (
	defaultMinGTK4Version      = "4.20"
	defaultMinWebKitGTKVersion = "2.50"
	defaultMinGLibVersion      = "2.84"
)

type RuntimeDependencyID string

const (
	RuntimeDependencyGTK4      RuntimeDependencyID = "gtk4"
	RuntimeDependencyWebKitGTK RuntimeDependencyID = "webkitgtk-6.0"
)

// RuntimeDependencyStatus contains the result of checking a runtime dependency.
type RuntimeDependencyStatus struct {
	ID            RuntimeDependencyID
	PkgConfigName string
	DisplayName   string

	Installed bool
	Version   string

	RequiredVersion  string
	MeetsRequirement bool

	Error string
}

// CheckRuntimeDependenciesUseCase validates runtime requirements for the GUI browser.
type CheckRuntimeDependenciesUseCase struct {
	probe port.RuntimeVersionProbe
}

// NewCheckRuntimeDependenciesUseCase creates a new use case.
func NewCheckRuntimeDependenciesUseCase(probe port.RuntimeVersionProbe) *CheckRuntimeDependenciesUseCase {
	return &CheckRuntimeDependenciesUseCase{probe: probe}
}

// CheckRuntimeDependenciesInput contains options for runtime dependency checks.
type CheckRuntimeDependenciesInput struct {
	// Prefix optionally points to a custom runtime prefix (e.g. /opt/webkitgtk).
	// Implementations may use this to influence pkg-config search paths.
	Prefix string

	// Min versions. If empty, defaults are used.
	MinGTK4Version      string
	MinWebKitGTKVersion string
	MinGLibVersion      string
}

// CheckRuntimeDependenciesOutput contains the result of the runtime dependency checks.
type CheckRuntimeDependenciesOutput struct {
	Prefix string
	OK     bool
	Checks []RuntimeDependencyStatus
}

// Execute checks WebKitGTK and GTK runtime versions.
func (uc *CheckRuntimeDependenciesUseCase) Execute(ctx context.Context, input CheckRuntimeDependenciesInput) (*CheckRuntimeDependenciesOutput, error) {
	log := logging.FromContext(ctx).With().Str("component", "runtime-check").Logger()

	minGTK := input.MinGTK4Version
	if minGTK == "" {
		minGTK = defaultMinGTK4Version
	}
	minWebKit := input.MinWebKitGTKVersion
	if minWebKit == "" {
		minWebKit = defaultMinWebKitGTKVersion
	}
	minGLib := input.MinGLibVersion
	if minGLib == "" {
		minGLib = defaultMinGLibVersion
	}

	checks := []RuntimeDependencyStatus{
		{
			ID:              RuntimeDependencyGTK4,
			PkgConfigName:   "gtk4",
			DisplayName:     "GTK4",
			RequiredVersion: minGTK,
		},
		{
			ID:              RuntimeDependencyWebKitGTK,
			PkgConfigName:   "webkitgtk-6.0",
			DisplayName:     "WebKitGTK 6.0",
			RequiredVersion: minWebKit,
		},
		{
			ID:              "glib-2.0",
			PkgConfigName:   "glib-2.0",
			DisplayName:     "GLib",
			RequiredVersion: minGLib,
		},
		{
			ID:              "gio-2.0",
			PkgConfigName:   "gio-2.0",
			DisplayName:     "GIO",
			RequiredVersion: minGLib,
		},
	}

	allOK := true
	for i := range checks {
		status := &checks[i]

		version, err := uc.probe.PkgConfigModVersion(ctx, status.PkgConfigName, input.Prefix)
		if err != nil {
			status.Installed = false
			status.MeetsRequirement = false
			status.Error = err.Error()
			allOK = false
			continue
		}

		status.Installed = true
		status.Version = strings.TrimSpace(version)

		cmp, ok := compareVersion(status.Version, status.RequiredVersion)
		if !ok {
			status.MeetsRequirement = false
			status.Error = "could not parse version"
			allOK = false
			continue
		}

		status.MeetsRequirement = cmp >= 0
		if !status.MeetsRequirement {
			allOK = false
		}
	}

	log.Debug().Bool("ok", allOK).Str("prefix", input.Prefix).Msg("runtime dependency check complete")
	return &CheckRuntimeDependenciesOutput{Prefix: input.Prefix, OK: allOK, Checks: checks}, nil
}

// compareVersion compares two version strings.
// Returns 1 if a > b, 0 if a == b, -1 if a < b. ok is false if either cannot be parsed.
func compareVersion(a, b string) (cmp int, ok bool) {
	av, ok := parseVersionPrefix(a)
	if !ok {
		return 0, false
	}
	bv, ok := parseVersionPrefix(b)
	if !ok {
		return 0, false
	}

	max := len(av)
	if len(bv) > max {
		max = len(bv)
	}

	for i := 0; i < max; i++ {
		x := 0
		if i < len(av) {
			x = av[i]
		}
		y := 0
		if i < len(bv) {
			y = bv[i]
		}
		switch {
		case x > y:
			return 1, true
		case x < y:
			return -1, true
		}
	}
	return 0, true
}

// parseVersionPrefix parses a dotted numeric version prefix (e.g. 4.20.3).
// It stops at the first non-digit/dot after a numeric segment.
func parseVersionPrefix(s string) ([]int, bool) {
	var parts []int
	cur := 0
	inNum := false

loop:
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			inNum = true
			cur = cur*10 + int(c-'0')
		case c == '.':
			if !inNum {
				return nil, false
			}
			parts = append(parts, cur)
			cur = 0
			inNum = false
		default:
			break loop
		}
	}

	if inNum {
		parts = append(parts, cur)
	}
	if len(parts) == 0 {
		return nil, false
	}
	return parts, true
}
