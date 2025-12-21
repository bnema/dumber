package deps

import (
	"context"
	"os/exec"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
)

// PkgConfigProbe uses pkg-config to query module versions.
type PkgConfigProbe struct{}

func NewPkgConfigProbe() *PkgConfigProbe {
	return &PkgConfigProbe{}
}

func (p *PkgConfigProbe) PkgConfigModVersion(ctx context.Context, pkgName, prefix string) (string, error) {
	if p == nil {
		return "", &port.PkgConfigError{
			Kind:    port.PkgConfigErrorKindCommandMissing,
			Package: pkgName,
			Err:     port.ErrPkgConfigMissing,
		}
	}
	pc, err := exec.LookPath("pkg-config")
	if err != nil {
		return "", &port.PkgConfigError{
			Kind:    port.PkgConfigErrorKindCommandMissing,
			Package: pkgName,
			Err:     port.ErrPkgConfigMissing,
		}
	}

	cmd := exec.CommandContext(ctx, pc, "--modversion", pkgName)
	cmd.Env = CommandEnvWithPrefix(prefix)

	out, err := cmd.CombinedOutput()
	if err != nil {
		output := strings.TrimSpace(string(out))
		return "", &port.PkgConfigError{
			Kind:    port.PkgConfigErrorKindPackageMissing,
			Package: pkgName,
			Output:  output,
			Err:     port.ErrPkgConfigPackageMissing,
		}
	}

	return string(out), nil
}
