package desktop

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
)

type sessionRestoreLauncher struct {
	relay port.SessionRestoreRelay
	start func(entity.SessionID) error
}

// NewSessionRestoreLauncher keeps profile selection identical for delivery and cold launch.
func NewSessionRestoreLauncher(relay port.SessionRestoreRelay, profile, instance string) port.SessionRestoreLauncher {
	return &sessionRestoreLauncher{relay: relay, start: func(id entity.SessionID) error {
		exe, err := getExecutablePath()
		if err != nil {
			return err
		}
		args := []string{"browse"}
		if profile != "" {
			args = append(args, "--profile", profile)
		}
		if instance != "" {
			args = append(args, "--instance", instance)
		}
		child := exec.Command(exe, args...)
		child.Env = append(withoutEnvKeys(sanitizedChildEnv(os.Environ()), RestoreSessionEnvVar), RestoreSessionEnvVar+"="+string(id))
		if err := child.Start(); err != nil {
			return fmt.Errorf("start browser: %w", err)
		}
		return child.Process.Release()
	}}
}

func (l *sessionRestoreLauncher) RestoreSession(ctx context.Context, id entity.SessionID) (bool, error) {
	if id == "" {
		return false, fmt.Errorf("session id required")
	}
	delivered, err := l.relay.DeliverRestoreSession(ctx, id)
	if err != nil {
		return false, fmt.Errorf("deliver session restore: %w", err)
	}
	if delivered {
		return false, nil
	}
	if err := l.start(id); err != nil {
		return false, err
	}
	return true, nil
}
