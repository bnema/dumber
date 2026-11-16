package bootstrap

import (
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/desktop"
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
