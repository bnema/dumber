package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/port/mocks"
)

func TestMigrateConfigUseCase_Check_NoMigrationNeeded(t *testing.T) {
	mockMigrator := mocks.NewMockConfigMigrator(t)
	mockMigrator.EXPECT().CheckMigration().Return(nil, nil)

	uc := NewMigrateConfigUseCase(mockMigrator)
	ctx := context.Background()

	result, err := uc.Check(ctx, CheckConfigMigrationInput{})

	require.NoError(t, err)
	assert.False(t, result.NeedsMigration)
	assert.Empty(t, result.MissingKeys)
}

func TestMigrateConfigUseCase_Check_MigrationNeeded(t *testing.T) {
	mockMigrator := mocks.NewMockConfigMigrator(t)

	migrationResult := &port.MigrationResult{
		MissingKeys: []string{"key1", "key2"},
		ConfigFile:  "/path/to/config.toml",
	}
	mockMigrator.EXPECT().CheckMigration().Return(migrationResult, nil)

	// Mock GetKeyInfo for each missing key
	mockMigrator.EXPECT().GetKeyInfo("key1").Return(port.KeyInfo{
		Key:          "key1",
		Type:         "bool",
		DefaultValue: "true",
	})
	mockMigrator.EXPECT().GetKeyInfo("key2").Return(port.KeyInfo{
		Key:          "key2",
		Type:         "string",
		DefaultValue: `"default"`,
	})

	uc := NewMigrateConfigUseCase(mockMigrator)
	ctx := context.Background()

	result, err := uc.Check(ctx, CheckConfigMigrationInput{})

	require.NoError(t, err)
	assert.True(t, result.NeedsMigration)
	assert.Len(t, result.MissingKeys, 2)
	assert.Equal(t, "/path/to/config.toml", result.ConfigFile)

	// Verify key info
	assert.Equal(t, "key1", result.MissingKeys[0].Key)
	assert.Equal(t, "bool", result.MissingKeys[0].Type)
	assert.Equal(t, "key2", result.MissingKeys[1].Key)
	assert.Equal(t, "string", result.MissingKeys[1].Type)
}

func TestMigrateConfigUseCase_Check_Error(t *testing.T) {
	mockMigrator := mocks.NewMockConfigMigrator(t)
	expectedErr := errors.New("check failed")
	mockMigrator.EXPECT().CheckMigration().Return(nil, expectedErr)

	uc := NewMigrateConfigUseCase(mockMigrator)
	ctx := context.Background()

	result, err := uc.Check(ctx, CheckConfigMigrationInput{})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, expectedErr, err)
}

func TestMigrateConfigUseCase_Execute_NoMigrationNeeded(t *testing.T) {
	mockMigrator := mocks.NewMockConfigMigrator(t)
	mockMigrator.EXPECT().CheckMigration().Return(nil, nil)

	uc := NewMigrateConfigUseCase(mockMigrator)
	ctx := context.Background()

	result, err := uc.Execute(ctx, MigrateConfigInput{})

	require.NoError(t, err)
	assert.Empty(t, result.AddedKeys)
}

func TestMigrateConfigUseCase_Execute_Success(t *testing.T) {
	mockMigrator := mocks.NewMockConfigMigrator(t)

	migrationResult := &port.MigrationResult{
		MissingKeys: []string{"key1", "key2"},
		ConfigFile:  "/path/to/config.toml",
	}
	mockMigrator.EXPECT().CheckMigration().Return(migrationResult, nil)
	mockMigrator.EXPECT().Migrate().Return([]string{"key1", "key2"}, nil)

	uc := NewMigrateConfigUseCase(mockMigrator)
	ctx := context.Background()

	result, err := uc.Execute(ctx, MigrateConfigInput{})

	require.NoError(t, err)
	assert.Len(t, result.AddedKeys, 2)
	assert.Equal(t, "/path/to/config.toml", result.ConfigFile)
}

func TestMigrateConfigUseCase_Execute_MigrateError(t *testing.T) {
	mockMigrator := mocks.NewMockConfigMigrator(t)

	migrationResult := &port.MigrationResult{
		MissingKeys: []string{"key1"},
		ConfigFile:  "/path/to/config.toml",
	}
	mockMigrator.EXPECT().CheckMigration().Return(migrationResult, nil)

	expectedErr := errors.New("migrate failed")
	mockMigrator.EXPECT().Migrate().Return(nil, expectedErr)

	uc := NewMigrateConfigUseCase(mockMigrator)
	ctx := context.Background()

	result, err := uc.Execute(ctx, MigrateConfigInput{})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, expectedErr, err)
}

func TestMigrateConfigUseCase_Execute_CheckError(t *testing.T) {
	mockMigrator := mocks.NewMockConfigMigrator(t)

	expectedErr := errors.New("check failed")
	mockMigrator.EXPECT().CheckMigration().Return(nil, expectedErr)

	uc := NewMigrateConfigUseCase(mockMigrator)
	ctx := context.Background()

	result, err := uc.Execute(ctx, MigrateConfigInput{})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, expectedErr, err)
}

func TestMigrateConfigUseCase_Check_EmptyMissingKeys(t *testing.T) {
	mockMigrator := mocks.NewMockConfigMigrator(t)

	// Return a result with empty missing keys
	migrationResult := &port.MigrationResult{
		MissingKeys: []string{},
		ConfigFile:  "/path/to/config.toml",
	}
	mockMigrator.EXPECT().CheckMigration().Return(migrationResult, nil)

	uc := NewMigrateConfigUseCase(mockMigrator)
	ctx := context.Background()

	result, err := uc.Check(ctx, CheckConfigMigrationInput{})

	require.NoError(t, err)
	assert.False(t, result.NeedsMigration)
	assert.Empty(t, result.MissingKeys)
}

func TestNewMigrateConfigUseCase(t *testing.T) {
	mockMigrator := mocks.NewMockConfigMigrator(t)
	uc := NewMigrateConfigUseCase(mockMigrator)

	assert.NotNil(t, uc)
	assert.Equal(t, mockMigrator, uc.migrator)
}

// Ensure mock expectations are set up correctly
func TestMigrateConfigUseCase_MockExpectations(t *testing.T) {
	mockMigrator := mocks.NewMockConfigMigrator(t)

	// Set up expectations with mock.Anything for flexibility
	mockMigrator.On("CheckMigration").Return(&port.MigrationResult{
		MissingKeys: []string{"test.key"},
		ConfigFile:  "/test/config.toml",
	}, nil).Once()

	mockMigrator.On("GetKeyInfo", mock.AnythingOfType("string")).Return(port.KeyInfo{
		Key:          "test.key",
		Type:         "string",
		DefaultValue: `"test"`,
	}).Once()

	uc := NewMigrateConfigUseCase(mockMigrator)
	ctx := context.Background()

	result, err := uc.Check(ctx, CheckConfigMigrationInput{})

	require.NoError(t, err)
	assert.True(t, result.NeedsMigration)
	mockMigrator.AssertExpectations(t)
}

func TestMigrateConfigUseCase_DetectChanges_NoChanges(t *testing.T) {
	mockMigrator := mocks.NewMockConfigMigrator(t)
	mockMigrator.EXPECT().DetectChanges().Return(nil, nil)

	uc := NewMigrateConfigUseCase(mockMigrator)
	ctx := context.Background()

	result, err := uc.DetectChanges(ctx, DetectChangesInput{})

	require.NoError(t, err)
	assert.False(t, result.HasChanges)
	assert.Empty(t, result.Changes)
	assert.Equal(t, "No changes detected.", result.DiffText)
}

func TestMigrateConfigUseCase_DetectChanges_WithAddedKey(t *testing.T) {
	mockMigrator := mocks.NewMockConfigMigrator(t)

	changes := []port.KeyChange{
		{
			Type:     port.KeyChangeAdded,
			NewKey:   "workspace.styling.mode_indicator_toaster_enabled",
			NewValue: "true",
		},
	}
	mockMigrator.EXPECT().DetectChanges().Return(changes, nil)

	uc := NewMigrateConfigUseCase(mockMigrator)
	ctx := context.Background()

	result, err := uc.DetectChanges(ctx, DetectChangesInput{})

	require.NoError(t, err)
	assert.True(t, result.HasChanges)
	assert.Len(t, result.Changes, 1)
	assert.Contains(t, result.DiffText, "+ workspace.styling.mode_indicator_toaster_enabled")
}

func TestMigrateConfigUseCase_DetectChanges_WithRenamedKey(t *testing.T) {
	mockMigrator := mocks.NewMockConfigMigrator(t)

	changes := []port.KeyChange{
		{
			Type:     port.KeyChangeRenamed,
			OldKey:   "workspace.styling.pane_mode_border_color",
			NewKey:   "workspace.styling.pane_mode_color",
			OldValue: `"#4A90E2"`,
		},
	}
	mockMigrator.EXPECT().DetectChanges().Return(changes, nil)

	uc := NewMigrateConfigUseCase(mockMigrator)
	ctx := context.Background()

	result, err := uc.DetectChanges(ctx, DetectChangesInput{})

	require.NoError(t, err)
	assert.True(t, result.HasChanges)
	assert.Len(t, result.Changes, 1)
	assert.Contains(t, result.DiffText, "~ workspace.styling.pane_mode_border_color -> workspace.styling.pane_mode_color")
}

func TestMigrateConfigUseCase_DetectChanges_WithRemovedKey(t *testing.T) {
	mockMigrator := mocks.NewMockConfigMigrator(t)

	changes := []port.KeyChange{
		{
			Type:     port.KeyChangeRemoved,
			OldKey:   "workspace.styling.pane_mode_border_width",
			OldValue: "4",
		},
	}
	mockMigrator.EXPECT().DetectChanges().Return(changes, nil)

	uc := NewMigrateConfigUseCase(mockMigrator)
	ctx := context.Background()

	result, err := uc.DetectChanges(ctx, DetectChangesInput{})

	require.NoError(t, err)
	assert.True(t, result.HasChanges)
	assert.Len(t, result.Changes, 1)
	assert.Contains(t, result.DiffText, "- workspace.styling.pane_mode_border_width")
	assert.Contains(t, result.DiffText, "(deprecated)")
}

func TestMigrateConfigUseCase_DetectChanges_Error(t *testing.T) {
	mockMigrator := mocks.NewMockConfigMigrator(t)
	expectedErr := errors.New("detect changes failed")
	mockMigrator.EXPECT().DetectChanges().Return(nil, expectedErr)

	uc := NewMigrateConfigUseCase(mockMigrator)
	ctx := context.Background()

	result, err := uc.DetectChanges(ctx, DetectChangesInput{})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, expectedErr, err)
}

func TestMigrateConfigUseCase_DetectChanges_MultipleChanges(t *testing.T) {
	mockMigrator := mocks.NewMockConfigMigrator(t)

	changes := []port.KeyChange{
		{
			Type:     port.KeyChangeAdded,
			NewKey:   "workspace.styling.mode_border_width",
			NewValue: "4",
		},
		{
			Type:     port.KeyChangeRenamed,
			OldKey:   "workspace.styling.pane_mode_border_color",
			NewKey:   "workspace.styling.pane_mode_color",
			OldValue: `"#4A90E2"`,
		},
		{
			Type:     port.KeyChangeRemoved,
			OldKey:   "workspace.styling.pane_mode_border_width",
			OldValue: "4",
		},
	}
	mockMigrator.EXPECT().DetectChanges().Return(changes, nil)

	uc := NewMigrateConfigUseCase(mockMigrator)
	ctx := context.Background()

	result, err := uc.DetectChanges(ctx, DetectChangesInput{})

	require.NoError(t, err)
	assert.True(t, result.HasChanges)
	assert.Len(t, result.Changes, 3)

	// Verify diff contains all change types
	assert.Contains(t, result.DiffText, "+ workspace.styling.mode_border_width")
	assert.Contains(t, result.DiffText, "~ workspace.styling.pane_mode_border_color")
	assert.Contains(t, result.DiffText, "- workspace.styling.pane_mode_border_width")
}

func TestFormatChangesAsDiff_EmptyChanges(t *testing.T) {
	result := formatChangesAsDiff(nil)
	assert.Equal(t, "No changes detected.", result)

	result = formatChangesAsDiff([]port.KeyChange{})
	assert.Equal(t, "No changes detected.", result)
}

func TestFormatChangesAsDiff_AllChangeTypes(t *testing.T) {
	changes := []port.KeyChange{
		{
			Type:     port.KeyChangeAdded,
			NewKey:   "new.key",
			NewValue: `"value"`,
		},
		{
			Type:     port.KeyChangeRemoved,
			OldKey:   "old.key",
			OldValue: `"old_value"`,
		},
		{
			Type:     port.KeyChangeRenamed,
			OldKey:   "renamed.old",
			NewKey:   "renamed.new",
			OldValue: `"renamed_value"`,
		},
		{
			Type:   port.KeyChangeConsolidated,
			OldKey: "consolidated.old",
			NewKey: "consolidated.new",
		},
	}

	result := formatChangesAsDiff(changes)

	assert.Contains(t, result, "Config migration changes:")
	assert.Contains(t, result, `+ new.key = "value"`)
	assert.Contains(t, result, `- old.key = "old_value" (deprecated)`)
	assert.Contains(t, result, "~ renamed.old -> renamed.new")
	assert.Contains(t, result, `(value: "renamed_value")`)
	assert.Contains(t, result, "> consolidated.old -> consolidated.new")
}
