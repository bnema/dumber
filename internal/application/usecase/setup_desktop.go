package usecase

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// InstallDesktopInput contains the input for the install operation.
type InstallDesktopInput struct {
	IconData []byte
}

// InstallDesktopUseCase installs the desktop file for system integration.
type InstallDesktopUseCase struct {
	desktop port.DesktopIntegration
}

// NewInstallDesktopUseCase creates a new InstallDesktopUseCase.
func NewInstallDesktopUseCase(desktop port.DesktopIntegration) *InstallDesktopUseCase {
	return &InstallDesktopUseCase{desktop: desktop}
}

// InstallDesktopOutput contains the result of the install operation.
type InstallDesktopOutput struct {
	DesktopPath        string
	IconPath           string
	WasDesktopExisting bool
	WasIconExisting    bool
}

// Execute installs the desktop file and icon.
func (uc *InstallDesktopUseCase) Execute(ctx context.Context, input InstallDesktopInput) (*InstallDesktopOutput, error) {
	log := logging.FromContext(ctx)

	// Check current status first
	status, err := uc.desktop.GetStatus(ctx)
	if err != nil {
		return nil, err
	}

	output := &InstallDesktopOutput{
		WasDesktopExisting: status.DesktopFileInstalled,
		WasIconExisting:    status.IconInstalled,
	}

	// Install desktop file
	desktopPath, err := uc.desktop.InstallDesktopFile(ctx)
	if err != nil {
		return nil, err
	}
	output.DesktopPath = desktopPath

	// Install icon if data provided
	if len(input.IconData) > 0 {
		iconPath, err := uc.desktop.InstallIcon(ctx, input.IconData)
		if err != nil {
			return nil, err
		}
		output.IconPath = iconPath
	}

	log.Info().
		Str("desktop_path", output.DesktopPath).
		Str("icon_path", output.IconPath).
		Bool("was_desktop_existing", output.WasDesktopExisting).
		Bool("was_icon_existing", output.WasIconExisting).
		Msg("desktop install complete")

	return output, nil
}

// SetDefaultBrowserUseCase sets dumber as the default web browser.
type SetDefaultBrowserUseCase struct {
	desktop port.DesktopIntegration
}

// NewSetDefaultBrowserUseCase creates a new SetDefaultBrowserUseCase.
func NewSetDefaultBrowserUseCase(desktop port.DesktopIntegration) *SetDefaultBrowserUseCase {
	return &SetDefaultBrowserUseCase{desktop: desktop}
}

// SetDefaultOutput contains the result of the set-default operation.
type SetDefaultOutput struct {
	WasAlreadyDefault bool
}

// Execute sets dumber as the default browser.
func (uc *SetDefaultBrowserUseCase) Execute(ctx context.Context) (*SetDefaultOutput, error) {
	log := logging.FromContext(ctx)

	// Check current status
	status, err := uc.desktop.GetStatus(ctx)
	if err != nil {
		return nil, err
	}

	if status.IsDefaultBrowser {
		log.Debug().Msg("dumber is already default browser")
		return &SetDefaultOutput{WasAlreadyDefault: true}, nil
	}

	// Set as default
	if err := uc.desktop.SetAsDefaultBrowser(ctx); err != nil {
		return nil, err
	}

	return &SetDefaultOutput{WasAlreadyDefault: false}, nil
}

// RemoveDesktopUseCase removes desktop integration files.
type RemoveDesktopUseCase struct {
	desktop port.DesktopIntegration
}

// NewRemoveDesktopUseCase creates a new RemoveDesktopUseCase.
func NewRemoveDesktopUseCase(desktop port.DesktopIntegration) *RemoveDesktopUseCase {
	return &RemoveDesktopUseCase{desktop: desktop}
}

// RemoveDesktopOutput contains the result of the remove operation.
type RemoveDesktopOutput struct {
	WasDesktopInstalled bool
	WasIconInstalled    bool
	WasDefault          bool
	RemovedDesktopPath  string
	RemovedIconPath     string
}

// Execute removes desktop integration.
func (uc *RemoveDesktopUseCase) Execute(ctx context.Context) (*RemoveDesktopOutput, error) {
	log := logging.FromContext(ctx)

	// Check current status
	status, err := uc.desktop.GetStatus(ctx)
	if err != nil {
		return nil, err
	}

	output := &RemoveDesktopOutput{
		WasDesktopInstalled: status.DesktopFileInstalled,
		WasIconInstalled:    status.IconInstalled,
		WasDefault:          status.IsDefaultBrowser,
		RemovedDesktopPath:  status.DesktopFilePath,
		RemovedIconPath:     status.IconFilePath,
	}

	// Unset as default browser first (if applicable)
	if status.IsDefaultBrowser {
		_ = uc.desktop.UnsetAsDefaultBrowser(ctx)
	}

	// Remove desktop file
	if err := uc.desktop.RemoveDesktopFile(ctx); err != nil {
		return nil, err
	}

	// Remove icon file
	if err := uc.desktop.RemoveIcon(ctx); err != nil {
		return nil, err
	}

	log.Info().
		Bool("was_desktop_installed", output.WasDesktopInstalled).
		Bool("was_icon_installed", output.WasIconInstalled).
		Bool("was_default", output.WasDefault).
		Msg("desktop integration removed")

	return output, nil
}
