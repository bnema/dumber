package bootstrap

import (
	"fmt"

	"bnema.dev/dumber/internal/infrastructure/config"
	"bnema.dev/dumber/internal/infrastructure/runtimeprofile"
)

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
