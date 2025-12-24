// Package desktop provides desktop environment integration for Linux (XDG).
package desktop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
)

const (
	appName         = "dumber"
	desktopFileName = "dumber.desktop"
	iconFileName    = "dumber.svg"
	filePerm        = 0644
	dirPerm         = 0755
)

// desktopFileTemplate is the freedesktop.org desktop entry format.
// %s placeholder for executable path.
const desktopFileTemplate = `[Desktop Entry]
Version=1.1
Type=Application
Name=Dumber
GenericName=Dumber Browser
Comment=It's a browser but dumber
Exec=%s browse %%U
Icon=dumber
Terminal=false
Categories=Network;WebBrowser;
MimeType=text/html;text/xml;application/xhtml+xml;x-scheme-handler/http;x-scheme-handler/https;
StartupNotify=true
StartupWMClass=dumber
`

// Adapter implements port.DesktopIntegration using XDG tools.
type Adapter struct {
	xdgSettingsPath string
	updateDesktopDB string
}

// New creates a new desktop integration adapter.
func New() port.DesktopIntegration {
	a := &Adapter{}

	// Detect xdg-settings
	if path, err := exec.LookPath("xdg-settings"); err == nil {
		a.xdgSettingsPath = path
	}

	// Detect update-desktop-database (optional)
	if path, err := exec.LookPath("update-desktop-database"); err == nil {
		a.updateDesktopDB = path
	}

	return a
}

// getApplicationsDir returns the XDG applications directory.
func getApplicationsDir() (string, error) {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home dir: %w", err)
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "applications"), nil
}

// getDesktopFilePath returns the full path to the desktop file.
func getDesktopFilePath() (string, error) {
	appDir, err := getApplicationsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(appDir, desktopFileName), nil
}

// getIconFilePath returns the full path to the icon file.
// Uses hicolor theme scalable apps directory.
func getIconFilePath() (string, error) {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home dir: %w", err)
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "icons", "hicolor", "scalable", "apps", iconFileName), nil
}

// getExecutablePath returns the path to the dumber executable.
func getExecutablePath() (string, error) {
	// First try the running executable
	execPath, err := os.Executable()
	if err == nil {
		// Resolve symlinks
		resolved, symlinkErr := filepath.EvalSymlinks(execPath)
		if symlinkErr == nil {
			execPath = resolved
		}
		return execPath, nil
	}

	// Fallback to PATH lookup
	path, err := exec.LookPath(appName)
	if err != nil {
		return "", fmt.Errorf("cannot find %s executable: %w", appName, err)
	}
	return path, nil
}

// GetStatus checks the current desktop integration state.
func (a *Adapter) GetStatus(ctx context.Context) (*port.DesktopIntegrationStatus, error) {
	log := logging.FromContext(ctx)
	status := &port.DesktopIntegrationStatus{}

	// Check desktop file
	desktopPath, err := getDesktopFilePath()
	if err != nil {
		return nil, err
	}
	status.DesktopFilePath = desktopPath

	if _, statErr := os.Stat(desktopPath); statErr == nil {
		status.DesktopFileInstalled = true
	}

	// Check icon file
	iconPath, err := getIconFilePath()
	if err != nil {
		return nil, err
	}
	status.IconFilePath = iconPath

	if _, iconStatErr := os.Stat(iconPath); iconStatErr == nil {
		status.IconInstalled = true
	}

	// Check executable
	execPath, err := getExecutablePath()
	if err == nil {
		status.ExecutablePath = execPath
	}

	// Check default browser
	if a.xdgSettingsPath != "" {
		out, err := exec.CommandContext(ctx, a.xdgSettingsPath, "get", "default-web-browser").Output()
		if err == nil {
			currentDefault := strings.TrimSpace(string(out))
			status.IsDefaultBrowser = currentDefault == desktopFileName
		}
	}

	log.Debug().
		Bool("desktop_installed", status.DesktopFileInstalled).
		Bool("icon_installed", status.IconInstalled).
		Bool("default", status.IsDefaultBrowser).
		Str("desktop_path", status.DesktopFilePath).
		Str("icon_path", status.IconFilePath).
		Str("exec_path", status.ExecutablePath).
		Msg("desktop integration status")

	return status, nil
}

// InstallDesktopFile writes the desktop file to XDG applications directory.
func (a *Adapter) InstallDesktopFile(ctx context.Context) (string, error) {
	log := logging.FromContext(ctx)

	execPath, err := getExecutablePath()
	if err != nil {
		return "", err
	}

	desktopPath, err := getDesktopFilePath()
	if err != nil {
		return "", err
	}

	// Ensure applications directory exists
	appDir := filepath.Dir(desktopPath)
	if err := os.MkdirAll(appDir, dirPerm); err != nil {
		return "", fmt.Errorf("create applications dir: %w", err)
	}

	// Generate desktop file content
	content := fmt.Sprintf(desktopFileTemplate, execPath)

	// Write desktop file
	if err := os.WriteFile(desktopPath, []byte(content), filePerm); err != nil {
		return "", fmt.Errorf("write desktop file: %w", err)
	}

	log.Info().Str("path", desktopPath).Msg("desktop file installed")

	// Update desktop database (optional, helps with some DEs)
	if a.updateDesktopDB != "" {
		if err := exec.CommandContext(ctx, a.updateDesktopDB, appDir).Run(); err != nil {
			log.Debug().Err(err).Msg("update-desktop-database failed (non-fatal)")
		}
	}

	return desktopPath, nil
}

// RemoveDesktopFile removes the desktop file from XDG applications directory.
func (a *Adapter) RemoveDesktopFile(ctx context.Context) error {
	log := logging.FromContext(ctx)

	desktopPath, err := getDesktopFilePath()
	if err != nil {
		return err
	}

	// Check if exists
	if _, err := os.Stat(desktopPath); os.IsNotExist(err) {
		log.Debug().Str("path", desktopPath).Msg("desktop file not found (already removed)")
		return nil
	}

	if err := os.Remove(desktopPath); err != nil {
		return fmt.Errorf("remove desktop file: %w", err)
	}

	log.Info().Str("path", desktopPath).Msg("desktop file removed")

	// Update desktop database
	if a.updateDesktopDB != "" {
		appDir := filepath.Dir(desktopPath)
		_ = exec.CommandContext(ctx, a.updateDesktopDB, appDir).Run()
	}

	return nil
}

// InstallIcon writes the icon file to XDG icons directory.
func (a *Adapter) InstallIcon(ctx context.Context, svgData []byte) (string, error) {
	if a == nil {
		return "", fmt.Errorf("desktop adapter is nil")
	}
	log := logging.FromContext(ctx)

	iconPath, err := getIconFilePath()
	if err != nil {
		return "", err
	}

	// Ensure icons directory exists
	iconDir := filepath.Dir(iconPath)
	if err := os.MkdirAll(iconDir, dirPerm); err != nil {
		return "", fmt.Errorf("create icons dir: %w", err)
	}

	// Write icon file
	if err := os.WriteFile(iconPath, svgData, filePerm); err != nil {
		return "", fmt.Errorf("write icon file: %w", err)
	}

	log.Info().Str("path", iconPath).Msg("icon file installed")

	return iconPath, nil
}

// RemoveIcon removes the icon file from XDG icons directory.
func (a *Adapter) RemoveIcon(ctx context.Context) error {
	if a == nil {
		return fmt.Errorf("desktop adapter is nil")
	}
	log := logging.FromContext(ctx)

	iconPath, err := getIconFilePath()
	if err != nil {
		return err
	}

	// Check if exists
	if _, err := os.Stat(iconPath); os.IsNotExist(err) {
		log.Debug().Str("path", iconPath).Msg("icon file not found (already removed)")
		return nil
	}

	if err := os.Remove(iconPath); err != nil {
		return fmt.Errorf("remove icon file: %w", err)
	}

	log.Info().Str("path", iconPath).Msg("icon file removed")

	return nil
}

// SetAsDefaultBrowser sets dumber as the default web browser.
func (a *Adapter) SetAsDefaultBrowser(ctx context.Context) error {
	log := logging.FromContext(ctx)

	if a.xdgSettingsPath == "" {
		return fmt.Errorf("xdg-settings not found (install xdg-utils)")
	}

	// Check if desktop file is installed
	desktopPath, err := getDesktopFilePath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(desktopPath); os.IsNotExist(err) {
		return fmt.Errorf("desktop file not installed - run 'dumber setup install' first")
	}

	// Set default browser
	cmd := exec.CommandContext(ctx, a.xdgSettingsPath, "set", "default-web-browser", desktopFileName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("xdg-settings failed: %s", strings.TrimSpace(string(out)))
	}

	log.Info().Msg("dumber set as default browser")
	return nil
}

// UnsetAsDefaultBrowser resets default browser if dumber is currently default.
func (a *Adapter) UnsetAsDefaultBrowser(ctx context.Context) error {
	log := logging.FromContext(ctx)

	if a.xdgSettingsPath == "" {
		return nil // No xdg-settings, nothing to do
	}

	// Check current default
	out, err := exec.CommandContext(ctx, a.xdgSettingsPath, "get", "default-web-browser").Output()
	if err != nil {
		return nil // Can't determine, skip
	}

	currentDefault := strings.TrimSpace(string(out))
	if currentDefault != desktopFileName {
		log.Debug().Str("current", currentDefault).Msg("dumber is not default browser, nothing to reset")
		return nil
	}

	// xdg-settings doesn't have a "reset" - user must set another browser
	log.Warn().Msg("dumber was default browser - user should set a new default")
	return nil
}

// SessionSpawner implements port.SessionSpawner by launching a new dumber process.
type SessionSpawner struct {
	ctx context.Context
}

// NewSessionSpawner creates a new session spawner.
func NewSessionSpawner(ctx context.Context) *SessionSpawner {
	return &SessionSpawner{ctx: ctx}
}

// SpawnWithSession starts a new dumber instance to restore a session.
func (s *SessionSpawner) SpawnWithSession(sessionID entity.SessionID) error {
	log := logging.FromContext(s.ctx)

	execPath, err := getExecutablePath()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	// Start a detached process with the --restore-session flag
	cmd := exec.Command(execPath, "browse", "--restore-session", string(sessionID))

	// Detach from current process group so the new process survives
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn session: %w", err)
	}

	// Release the process so it continues running after we close
	if err := cmd.Process.Release(); err != nil {
		log.Warn().Err(err).Msg("failed to release spawned process (non-fatal)")
	}

	log.Info().
		Str("session_id", string(sessionID)).
		Int("pid", cmd.Process.Pid).
		Msg("spawned dumber with session restoration")

	return nil
}
