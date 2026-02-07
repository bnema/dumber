package usecase

import (
	"context"
	"errors"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/build"
	"github.com/bnema/dumber/internal/logging"
)

// CheckUpdateInput holds the input for the check update use case.
type CheckUpdateInput struct{}

// CheckUpdateOutput holds the result of the update check.
type CheckUpdateOutput struct {
	// UpdateAvailable is true if a newer version exists.
	UpdateAvailable bool
	// CanAutoUpdate is true if the binary is writable and auto-update is possible.
	CanAutoUpdate bool
	// CurrentVersion is the version of the running binary.
	CurrentVersion string
	// LatestVersion is the latest available version.
	LatestVersion string
	// ReleaseURL is the URL to the GitHub release page.
	ReleaseURL string
	// DownloadURL is the direct download URL for the archive.
	DownloadURL string
}

// CheckUpdateUseCase checks for available updates.
type CheckUpdateUseCase struct {
	checker   port.UpdateChecker
	applier   port.UpdateApplier
	buildInfo build.Info
}

// NewCheckUpdateUseCase creates a new check update use case.
func NewCheckUpdateUseCase(
	checker port.UpdateChecker,
	applier port.UpdateApplier,
	buildInfo build.Info,
) *CheckUpdateUseCase {
	return &CheckUpdateUseCase{
		checker:   checker,
		applier:   applier,
		buildInfo: buildInfo,
	}
}

// Execute checks for available updates.
func (uc *CheckUpdateUseCase) Execute(ctx context.Context, _ CheckUpdateInput) (*CheckUpdateOutput, error) {
	log := logging.FromContext(ctx)

	info, err := uc.checker.CheckForUpdate(ctx, uc.buildInfo.Version)
	if err != nil {
		if errors.Is(err, port.ErrUpdateCheckTransient) {
			log.Debug().Err(err).Msg("transient update check failure")
			return &CheckUpdateOutput{
				UpdateAvailable: false,
				CanAutoUpdate:   false,
				CurrentVersion:  uc.buildInfo.Version,
				LatestVersion:   uc.buildInfo.Version,
			}, nil
		}
		log.Warn().Err(err).Msg("update check failed")
		return nil, err
	}

	canAutoUpdate := false
	if info.IsNewer {
		canAutoUpdate = uc.applier.CanSelfUpdate(ctx)
	}

	log.Debug().
		Str("current", uc.buildInfo.Version).
		Str("latest", info.LatestVersion).
		Bool("update_available", info.IsNewer).
		Bool("can_auto_update", canAutoUpdate).
		Msg("update check completed")

	return &CheckUpdateOutput{
		UpdateAvailable: info.IsNewer,
		CanAutoUpdate:   canAutoUpdate,
		CurrentVersion:  info.CurrentVersion,
		LatestVersion:   info.LatestVersion,
		ReleaseURL:      info.ReleaseURL,
		DownloadURL:     info.DownloadURL,
	}, nil
}
