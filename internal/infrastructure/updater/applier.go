package updater

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
)

const (
	// Staging directory name within XDG_STATE_HOME.
	stagingDirName = "pending-update"
	// Staged binary filename.
	stagedBinaryName = "dumber"
	// Backup suffix for old binary.
	backupSuffix = ".old"
	// File permission for directories and executables.
	applierExecPerm = 0o755
	// Execute permission bit for owner.
	execBit = 0o100
)

// Applier implements UpdateApplier for self-updating the binary.
type Applier struct {
	stateDir string
}

// NewApplier creates a new update applier.
// stateDir is the XDG state directory (e.g., ~/.local/state/dumber).
func NewApplier(stateDir string) *Applier {
	return &Applier{
		stateDir: stateDir,
	}
}

// NewApplierFromXDG creates a new update applier using XDG directories.
func NewApplierFromXDG() (*Applier, error) {
	stateDir, err := config.GetStateDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get state directory: %w", err)
	}
	return NewApplier(stateDir), nil
}

// stagingDir returns the path to the staging directory.
func (a *Applier) stagingDir() string {
	return filepath.Join(a.stateDir, stagingDirName)
}

// stagedBinaryPath returns the path to the staged binary.
func (a *Applier) stagedBinaryPath() string {
	return filepath.Join(a.stagingDir(), stagedBinaryName)
}

// CanSelfUpdate checks if the current binary is writable by the current user.
func (a *Applier) CanSelfUpdate(ctx context.Context) bool {
	log := logging.FromContext(ctx)

	binaryPath, err := a.GetBinaryPath()
	if err != nil {
		log.Debug().Err(err).Msg("failed to get binary path")
		return false
	}

	// Check if we have write permission on the binary file.
	err = unix.Access(binaryPath, unix.W_OK)
	if err != nil {
		log.Debug().
			Str("path", binaryPath).
			Err(err).
			Msg("binary is not writable, self-update disabled")
		return false
	}

	log.Debug().Str("path", binaryPath).Msg("binary is writable, self-update enabled")
	return true
}

// GetBinaryPath returns the path to the currently running binary.
func (*Applier) GetBinaryPath() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve any symlinks to get the actual binary path.
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	return resolved, nil
}

// StageUpdate copies the new binary to the staging directory.
func (a *Applier) StageUpdate(ctx context.Context, newBinaryPath string) error {
	log := logging.FromContext(ctx)

	// Create staging directory.
	stagingDir := a.stagingDir()
	if err := os.MkdirAll(stagingDir, applierExecPerm); err != nil {
		return fmt.Errorf("failed to create staging directory: %w", err)
	}

	// Read the new binary.
	newBinary, err := os.ReadFile(newBinaryPath)
	if err != nil {
		return fmt.Errorf("failed to read new binary: %w", err)
	}

	// Write to staging location.
	stagedPath := a.stagedBinaryPath()
	if err := os.WriteFile(stagedPath, newBinary, applierExecPerm); err != nil {
		return fmt.Errorf("failed to write staged binary: %w", err)
	}

	log.Info().
		Str("staged_path", stagedPath).
		Int("size", len(newBinary)).
		Msg("update staged for exit")

	return nil
}

// HasStagedUpdate checks if there is an update staged and ready to apply.
func (a *Applier) HasStagedUpdate(_ context.Context) bool {
	stagedPath := a.stagedBinaryPath()
	info, err := os.Stat(stagedPath)
	if err != nil {
		return false
	}
	// Ensure it's a regular file with execute permission.
	return info.Mode().IsRegular() && info.Mode()&execBit != 0
}

// ApplyOnExit replaces the current binary with the staged update.
// This should be called during graceful shutdown.
func (a *Applier) ApplyOnExit(ctx context.Context) (string, error) {
	log := logging.FromContext(ctx)

	if !a.HasStagedUpdate(ctx) {
		return "", fmt.Errorf("no staged update found")
	}

	binaryPath, err := a.GetBinaryPath()
	if err != nil {
		return "", err
	}

	stagedPath := a.stagedBinaryPath()
	backupPath := binaryPath + backupSuffix

	log.Info().
		Str("current", binaryPath).
		Str("staged", stagedPath).
		Str("backup", backupPath).
		Msg("applying staged update")

	// Step 1: Remove old backup if exists.
	_ = os.Remove(backupPath)

	// Step 2: Rename current binary to backup.
	if err := os.Rename(binaryPath, backupPath); err != nil {
		return "", fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Step 3: Rename staged binary to current.
	if err := os.Rename(stagedPath, binaryPath); err != nil {
		// Try to restore backup.
		if restoreErr := os.Rename(backupPath, binaryPath); restoreErr != nil {
			log.Error().Err(restoreErr).Msg("failed to restore backup after update failure")
		}
		return "", fmt.Errorf("failed to install new binary: %w", err)
	}

	// Step 4: Clean up staging directory.
	_ = os.RemoveAll(a.stagingDir())

	log.Info().
		Str("binary", binaryPath).
		Str("backup", backupPath).
		Msg("update applied successfully")

	return backupPath, nil
}

// ClearStagedUpdate removes any staged update without applying it.
func (a *Applier) ClearStagedUpdate(ctx context.Context) error {
	log := logging.FromContext(ctx)

	stagingDir := a.stagingDir()
	if err := os.RemoveAll(stagingDir); err != nil {
		return fmt.Errorf("failed to clear staged update: %w", err)
	}

	log.Debug().Str("path", stagingDir).Msg("cleared staged update")
	return nil
}
