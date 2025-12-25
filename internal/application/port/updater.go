// Package port defines interfaces for external dependencies.
package port

import (
	"context"

	"github.com/bnema/dumber/internal/domain/entity"
)

// UpdateChecker checks for available updates from a remote source.
type UpdateChecker interface {
	// CheckForUpdate compares the current version with the latest available release.
	// Returns update info including whether a newer version is available.
	CheckForUpdate(ctx context.Context, currentVersion string) (*entity.UpdateInfo, error)
}

// UpdateDownloader downloads and extracts update archives.
type UpdateDownloader interface {
	// Download fetches the update archive to the specified destination directory.
	// Returns the path to the downloaded archive file.
	Download(ctx context.Context, downloadURL string, destDir string) (archivePath string, err error)

	// Extract extracts the binary from the downloaded archive.
	// Returns the path to the extracted binary.
	Extract(ctx context.Context, archivePath string, destDir string) (binaryPath string, err error)
}

// UpdateApplier stages and applies updates.
type UpdateApplier interface {
	// CanSelfUpdate checks if the current binary is writable by the current user.
	// Returns true if auto-update is possible, false if user lacks write permission.
	CanSelfUpdate(ctx context.Context) bool

	// GetBinaryPath returns the path to the currently running binary.
	GetBinaryPath() (string, error)

	// StageUpdate prepares the new binary to be applied on exit.
	// The staged update will be applied when ApplyOnExit is called.
	StageUpdate(ctx context.Context, newBinaryPath string) error

	// HasStagedUpdate checks if there is an update staged and ready to apply.
	HasStagedUpdate(ctx context.Context) bool

	// ApplyOnExit replaces the current binary with the staged update.
	// This should be called during graceful shutdown.
	// Returns the path to the backup of the old binary.
	ApplyOnExit(ctx context.Context) (backupPath string, err error)

	// ClearStagedUpdate removes any staged update without applying it.
	ClearStagedUpdate(ctx context.Context) error
}
