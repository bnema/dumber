package updater

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplier_SelfUpdateBlockedReason(t *testing.T) {
	// Create a temporary directory for test state
	tmpDir := t.TempDir()
	applier := NewApplier(tmpDir)

	ctx := context.Background()

	// Get the reason - the actual result depends on the environment
	reason := applier.SelfUpdateBlockedReason(ctx)

	// Verify it returns a valid reason type
	validReasons := []port.SelfUpdateBlockedReason{
		port.SelfUpdateAllowed,
		port.SelfUpdateBlockedFlatpak,
		port.SelfUpdateBlockedPacman,
		port.SelfUpdateBlockedNotWritable,
	}

	found := false
	for _, valid := range validReasons {
		if reason == valid {
			found = true
			break
		}
	}
	assert.True(t, found, "SelfUpdateBlockedReason returned invalid reason: %q", reason)
}

func TestApplier_CanSelfUpdate_ConsistentWithReason(t *testing.T) {
	tmpDir := t.TempDir()
	applier := NewApplier(tmpDir)
	ctx := context.Background()

	canUpdate := applier.CanSelfUpdate(ctx)
	reason := applier.SelfUpdateBlockedReason(ctx)

	// If CanSelfUpdate returns true, reason should be SelfUpdateAllowed
	if canUpdate {
		assert.Equal(t, port.SelfUpdateAllowed, reason,
			"CanSelfUpdate=true but SelfUpdateBlockedReason=%q", reason)
	} else {
		// If CanSelfUpdate returns false, reason should NOT be SelfUpdateAllowed
		assert.NotEqual(t, port.SelfUpdateAllowed, reason,
			"CanSelfUpdate=false but SelfUpdateBlockedReason is empty")
	}
}

func TestApplier_StagingPaths(t *testing.T) {
	tmpDir := t.TempDir()
	applier := NewApplier(tmpDir)

	// Test staging directory path
	stagingDir := applier.stagingDir()
	assert.Equal(t, filepath.Join(tmpDir, stagingDirName), stagingDir)

	// Test staged binary path
	stagedPath := applier.stagedBinaryPath()
	assert.Equal(t, filepath.Join(tmpDir, stagingDirName, stagedBinaryName), stagedPath)
}

func TestApplier_HasStagedUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	applier := NewApplier(tmpDir)
	ctx := context.Background()

	// Initially no staged update
	assert.False(t, applier.HasStagedUpdate(ctx), "should have no staged update initially")

	// Create staging directory and file
	stagingDir := applier.stagingDir()
	require.NoError(t, os.MkdirAll(stagingDir, 0o755))

	stagedPath := applier.stagedBinaryPath()

	// Create a non-executable file (should not count as staged)
	require.NoError(t, os.WriteFile(stagedPath, []byte("test"), 0o644))
	assert.False(t, applier.HasStagedUpdate(ctx), "non-executable file should not count as staged")

	// Make it executable
	require.NoError(t, os.Chmod(stagedPath, 0o755))
	assert.True(t, applier.HasStagedUpdate(ctx), "executable file should count as staged")
}

func TestApplier_ClearStagedUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	applier := NewApplier(tmpDir)
	ctx := context.Background()

	// Create staged update
	stagingDir := applier.stagingDir()
	require.NoError(t, os.MkdirAll(stagingDir, 0o755))
	stagedPath := applier.stagedBinaryPath()
	require.NoError(t, os.WriteFile(stagedPath, []byte("test"), 0o755))

	assert.True(t, applier.HasStagedUpdate(ctx), "should have staged update")

	// Clear it
	require.NoError(t, applier.ClearStagedUpdate(ctx))

	assert.False(t, applier.HasStagedUpdate(ctx), "should have no staged update after clear")

	// Verify directory was removed
	_, err := os.Stat(stagingDir)
	assert.True(t, os.IsNotExist(err), "staging directory should be removed")
}

func TestApplier_StageUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	applier := NewApplier(tmpDir)
	ctx := context.Background()

	// Create a fake binary to stage
	srcBinary := filepath.Join(tmpDir, "new-binary")
	binaryContent := []byte("#!/bin/sh\necho 'new version'\n")
	require.NoError(t, os.WriteFile(srcBinary, binaryContent, 0o755))

	// Stage the update
	require.NoError(t, applier.StageUpdate(ctx, srcBinary))

	// Verify it was staged
	assert.True(t, applier.HasStagedUpdate(ctx), "should have staged update")

	// Verify content was copied
	stagedContent, err := os.ReadFile(applier.stagedBinaryPath())
	require.NoError(t, err)
	assert.Equal(t, binaryContent, stagedContent, "staged content should match source")

	// Verify permissions
	info, err := os.Stat(applier.stagedBinaryPath())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm(), "staged binary should be executable")
}

func TestNewApplierFromXDG(t *testing.T) {
	// This test verifies NewApplierFromXDG doesn't panic and returns valid applier
	applier, err := NewApplierFromXDG()
	require.NoError(t, err)
	require.NotNil(t, applier)

	// Verify it has a valid state directory
	assert.NotEmpty(t, applier.stateDir)
}

func TestSelfUpdateBlockedReason_Constants(t *testing.T) {
	// Verify the constants are distinct
	reasons := []port.SelfUpdateBlockedReason{
		port.SelfUpdateAllowed,
		port.SelfUpdateBlockedFlatpak,
		port.SelfUpdateBlockedPacman,
		port.SelfUpdateBlockedNotWritable,
	}

	seen := make(map[port.SelfUpdateBlockedReason]bool)
	for _, r := range reasons {
		assert.False(t, seen[r], "duplicate reason: %q", r)
		seen[r] = true
	}

	// Verify SelfUpdateAllowed is empty string
	assert.Equal(t, port.SelfUpdateBlockedReason(""), port.SelfUpdateAllowed,
		"SelfUpdateAllowed should be empty string")
}
