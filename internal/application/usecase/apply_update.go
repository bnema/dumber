package usecase

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
)

const (
	// File permission for directories.
	updateDirPerm = 0o755
)

// ApplyUpdateInput holds the input for the apply update use case.
type ApplyUpdateInput struct {
	// DownloadURL is the URL to download the update from.
	DownloadURL string
}

// ApplyUpdateOutput holds the result of the update application.
type ApplyUpdateOutput struct {
	// Status is the current update status.
	Status entity.UpdateStatus
	// Message is a human-readable status message.
	Message string
	// StagedPath is the path to the staged binary (if status is Ready).
	StagedPath string
}

// ApplyUpdateUseCase downloads and stages updates for application on exit.
type ApplyUpdateUseCase struct {
	downloader port.UpdateDownloader
	applier    port.UpdateApplier
	cacheDir   string
}

// NewApplyUpdateUseCase creates a new apply update use case.
func NewApplyUpdateUseCase(
	downloader port.UpdateDownloader,
	applier port.UpdateApplier,
	cacheDir string,
) *ApplyUpdateUseCase {
	return &ApplyUpdateUseCase{
		downloader: downloader,
		applier:    applier,
		cacheDir:   cacheDir,
	}
}

// Execute downloads and stages the update for application on exit.
func (uc *ApplyUpdateUseCase) Execute(ctx context.Context, input ApplyUpdateInput) (*ApplyUpdateOutput, error) {
	log := logging.FromContext(ctx)

	// Check if we can self-update.
	if !uc.applier.CanSelfUpdate(ctx) {
		reason := uc.applier.SelfUpdateBlockedReason(ctx)
		var message string
		switch reason {
		case port.SelfUpdateBlockedFlatpak:
			message = "Cannot auto-update: installed via Flatpak (use 'flatpak update' instead)"
		case port.SelfUpdateBlockedPacman:
			message = "Cannot auto-update: installed via pacman/AUR (use your package manager instead)"
		default:
			message = "Cannot auto-update: binary is not writable"
		}
		return &ApplyUpdateOutput{
			Status:  entity.UpdateStatusFailed,
			Message: message,
		}, nil
	}

	// Create download directory.
	downloadDir := filepath.Join(uc.cacheDir, "updates")
	if err := os.MkdirAll(downloadDir, updateDirPerm); err != nil {
		return nil, fmt.Errorf("failed to create download directory: %w", err)
	}

	log.Info().Str("url", input.DownloadURL).Msg("downloading update")

	// Download the archive.
	archivePath, err := uc.downloader.Download(ctx, input.DownloadURL, downloadDir)
	if err != nil {
		return &ApplyUpdateOutput{
			Status:  entity.UpdateStatusFailed,
			Message: fmt.Sprintf("Download failed: %v", err),
		}, nil
	}

	log.Info().Str("archive", archivePath).Msg("extracting update")

	// Extract the binary.
	binaryPath, err := uc.downloader.Extract(ctx, archivePath, downloadDir)
	if err != nil {
		_ = os.Remove(archivePath)
		return &ApplyUpdateOutput{
			Status:  entity.UpdateStatusFailed,
			Message: fmt.Sprintf("Extraction failed: %v", err),
		}, nil
	}

	log.Info().Str("binary", binaryPath).Msg("staging update")

	// Stage the update.
	if err := uc.applier.StageUpdate(ctx, binaryPath); err != nil {
		_ = os.Remove(archivePath)
		_ = os.Remove(binaryPath)
		return &ApplyUpdateOutput{
			Status:  entity.UpdateStatusFailed,
			Message: fmt.Sprintf("Staging failed: %v", err),
		}, nil
	}

	// Clean up download artifacts.
	_ = os.Remove(archivePath)
	_ = os.Remove(binaryPath)

	log.Info().Msg("update staged successfully, will apply on exit")

	return &ApplyUpdateOutput{
		Status:  entity.UpdateStatusReady,
		Message: "Update ready - will apply on exit",
	}, nil
}

// FinalizeOnExit applies the staged update during shutdown.
// This should be called from the app's shutdown handler.
func (uc *ApplyUpdateUseCase) FinalizeOnExit(ctx context.Context) error {
	log := logging.FromContext(ctx)

	if !uc.applier.HasStagedUpdate(ctx) {
		log.Debug().Msg("no staged update to apply")
		return nil
	}

	backupPath, err := uc.applier.ApplyOnExit(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to apply update on exit")
		return err
	}

	log.Info().
		Str("backup", backupPath).
		Msg("update applied successfully")

	return nil
}

// HasPendingUpdate checks if there's an update waiting to be applied on exit.
func (uc *ApplyUpdateUseCase) HasPendingUpdate(ctx context.Context) bool {
	return uc.applier.HasStagedUpdate(ctx)
}

// ClearPendingUpdate removes any staged update.
func (uc *ApplyUpdateUseCase) ClearPendingUpdate(ctx context.Context) error {
	return uc.applier.ClearStagedUpdate(ctx)
}
