package bootstrap

import (
	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/infrastructure/config"
	"bnema.dev/dumber/internal/infrastructure/desktop"
)

func NewSessionRestoreLauncher(cfg *config.Config, profileName, instance string) (port.SessionRestoreLauncher, error) {
	profile, err := ResolveRuntimeProfile(cfg)
	if profileName != "" {
		profile, err = NewBrowserProfile(cfg, profileName)
	}
	if err != nil {
		return nil, err
	}
	relay := desktop.NewBrowserLaunchRelay(profile.IPC).(port.SessionRestoreRelay)
	return desktop.NewSessionRestoreLauncher(relay, profileName, instance), nil
}
