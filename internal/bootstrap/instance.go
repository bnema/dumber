package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
)

// NewInstanceProfile derives deterministic coordination paths for a named instance.
func NewInstanceProfile(cfg *config.Config, name string) (runtimeprofile.Profile, error) {
	profile, err := ResolveRuntimeProfile(cfg)
	if err != nil {
		return runtimeprofile.Profile{}, err
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = filepath.Join(os.TempDir(), "dumber-runtime-"+strconv.Itoa(os.Geteuid()))
	}
	profile, err = profile.WithInstance(name, base)
	if err != nil {
		return runtimeprofile.Profile{}, fmt.Errorf("resolve named instance: %w", err)
	}
	return profile, nil
}

func NewBrowserProfile(cfg *config.Config, name string) (runtimeprofile.Profile, error) {
	profile, err := ResolveRuntimeProfile(cfg)
	if err != nil {
		return runtimeprofile.Profile{}, err
	}
	profile, err = profile.WithBrowserProfile(name, ResolveXDGRuntimeDir(profile))
	if err != nil {
		return runtimeprofile.Profile{}, fmt.Errorf("resolve browser profile: %w", err)
	}
	return profile, nil
}

func NewEphemeralBrowserProfile(cfg *config.Config) (runtimeprofile.Profile, error) {
	profile, err := ResolveRuntimeProfile(cfg)
	if err != nil {
		return runtimeprofile.Profile{}, err
	}
	profile, err = profile.WithEphemeralBrowserProfile(ResolveXDGRuntimeDir(profile))
	if err != nil {
		return runtimeprofile.Profile{}, fmt.Errorf("resolve ephemeral browser profile: %w", err)
	}
	return profile, nil
}
