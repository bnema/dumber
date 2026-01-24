package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/application/usecase"
)

func TestGetKeybindingsUseCase_Execute(t *testing.T) {
	t.Run("returns keybindings from provider", func(t *testing.T) {
		// Arrange
		mockProvider := mocks.NewMockKeybindingsProvider(t)
		expected := port.KeybindingsConfig{
			Groups: []port.KeybindingGroup{
				{
					Mode:        "global",
					DisplayName: "Global Shortcuts",
					Bindings: []port.KeybindingEntry{
						{Action: "close_pane", Keys: []string{"ctrl+w"}},
					},
				},
			},
		}
		mockProvider.EXPECT().GetKeybindings(mock.Anything).Return(expected, nil)

		uc := usecase.NewGetKeybindingsUseCase(mockProvider)

		// Act
		result, err := uc.Execute(context.Background())

		// Assert
		require.NoError(t, err)
		assert.Equal(t, expected, result)
		mock.AssertExpectationsForObjects(t, mockProvider)
	})

	t.Run("returns error when provider fails", func(t *testing.T) {
		// Arrange
		mockProvider := mocks.NewMockKeybindingsProvider(t)
		mockProvider.EXPECT().GetKeybindings(mock.Anything).Return(port.KeybindingsConfig{}, errors.New("provider error"))

		uc := usecase.NewGetKeybindingsUseCase(mockProvider)

		// Act
		_, err := uc.Execute(context.Background())

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider error")
		mock.AssertExpectationsForObjects(t, mockProvider)
	})

	t.Run("returns error when provider is nil", func(t *testing.T) {
		// Arrange
		uc := usecase.NewGetKeybindingsUseCase(nil)

		// Act
		_, err := uc.Execute(context.Background())

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider is nil")
	})
}

func TestSetKeybindingUseCase_Execute(t *testing.T) {
	t.Run("successfully sets keybinding", func(t *testing.T) {
		// Arrange
		mockProvider := mocks.NewMockKeybindingsProvider(t)
		mockSaver := mocks.NewMockKeybindingsSaver(t)
		req := port.SetKeybindingRequest{
			Mode:   "pane",
			Action: "split-right",
			Keys:   []string{"r"},
		}
		mockProvider.EXPECT().CheckConflicts(mock.Anything, req.Mode, req.Action, req.Keys).Return(nil, nil)
		mockSaver.EXPECT().SetKeybinding(mock.Anything, req).Return(nil)

		uc := usecase.NewSetKeybindingUseCase(mockProvider, mockSaver)

		// Act
		resp, err := uc.Execute(context.Background(), req)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, resp.Conflicts)
		mock.AssertExpectationsForObjects(t, mockProvider, mockSaver)
	})

	t.Run("returns conflicts when detected", func(t *testing.T) {
		// Arrange
		mockProvider := mocks.NewMockKeybindingsProvider(t)
		mockSaver := mocks.NewMockKeybindingsSaver(t)
		req := port.SetKeybindingRequest{
			Mode:   "pane",
			Action: "split-right",
			Keys:   []string{"r"},
		}
		conflicts := []port.KeybindingConflict{
			{ConflictingAction: "other-action", ConflictingMode: "pane", Key: "r"},
		}
		mockProvider.EXPECT().CheckConflicts(mock.Anything, req.Mode, req.Action, req.Keys).Return(conflicts, nil)
		mockSaver.EXPECT().SetKeybinding(mock.Anything, req).Return(nil)

		uc := usecase.NewSetKeybindingUseCase(mockProvider, mockSaver)

		// Act
		resp, err := uc.Execute(context.Background(), req)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, conflicts, resp.Conflicts)
		mock.AssertExpectationsForObjects(t, mockProvider, mockSaver)
	})

	t.Run("returns error when saver fails", func(t *testing.T) {
		// Arrange
		mockProvider := mocks.NewMockKeybindingsProvider(t)
		mockSaver := mocks.NewMockKeybindingsSaver(t)
		req := port.SetKeybindingRequest{
			Mode:   "pane",
			Action: "split-right",
			Keys:   []string{"r"},
		}
		mockProvider.EXPECT().CheckConflicts(mock.Anything, req.Mode, req.Action, req.Keys).Return(nil, nil)
		mockSaver.EXPECT().SetKeybinding(mock.Anything, req).Return(errors.New("save error"))

		uc := usecase.NewSetKeybindingUseCase(mockProvider, mockSaver)

		// Act
		_, err := uc.Execute(context.Background(), req)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "save error")
		mock.AssertExpectationsForObjects(t, mockProvider, mockSaver)
	})

	t.Run("returns error when saver is nil", func(t *testing.T) {
		// Arrange
		uc := usecase.NewSetKeybindingUseCase(nil, nil)
		req := port.SetKeybindingRequest{
			Mode:   "pane",
			Action: "split-right",
			Keys:   []string{"r"},
		}

		// Act
		_, err := uc.Execute(context.Background(), req)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "saver is nil")
	})

	t.Run("returns error when mode is empty", func(t *testing.T) {
		// Arrange
		mockProvider := mocks.NewMockKeybindingsProvider(t)
		mockSaver := mocks.NewMockKeybindingsSaver(t)
		req := port.SetKeybindingRequest{
			Mode:   "",
			Action: "split-right",
			Keys:   []string{"r"},
		}

		uc := usecase.NewSetKeybindingUseCase(mockProvider, mockSaver)

		// Act
		_, err := uc.Execute(context.Background(), req)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mode is required")
	})

	t.Run("returns error when action is empty", func(t *testing.T) {
		// Arrange
		mockProvider := mocks.NewMockKeybindingsProvider(t)
		mockSaver := mocks.NewMockKeybindingsSaver(t)
		req := port.SetKeybindingRequest{
			Mode:   "pane",
			Action: "",
			Keys:   []string{"r"},
		}

		uc := usecase.NewSetKeybindingUseCase(mockProvider, mockSaver)

		// Act
		_, err := uc.Execute(context.Background(), req)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "action is required")
	})

	t.Run("returns error for invalid mode", func(t *testing.T) {
		// Arrange
		mockProvider := mocks.NewMockKeybindingsProvider(t)
		mockSaver := mocks.NewMockKeybindingsSaver(t)
		req := port.SetKeybindingRequest{
			Mode:   "invalid",
			Action: "split-right",
			Keys:   []string{"r"},
		}

		uc := usecase.NewSetKeybindingUseCase(mockProvider, mockSaver)

		// Act
		_, err := uc.Execute(context.Background(), req)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid mode")
	})
}

func TestResetKeybindingUseCase_Execute(t *testing.T) {
	t.Run("successfully resets keybinding", func(t *testing.T) {
		// Arrange
		mockSaver := mocks.NewMockKeybindingsSaver(t)
		req := port.ResetKeybindingRequest{
			Mode:   "pane",
			Action: "split-right",
		}
		mockSaver.EXPECT().ResetKeybinding(mock.Anything, req).Return(nil)

		uc := usecase.NewResetKeybindingUseCase(mockSaver)

		// Act
		err := uc.Execute(context.Background(), req)

		// Assert
		require.NoError(t, err)
		mock.AssertExpectationsForObjects(t, mockSaver)
	})

	t.Run("returns error when saver fails", func(t *testing.T) {
		// Arrange
		mockSaver := mocks.NewMockKeybindingsSaver(t)
		req := port.ResetKeybindingRequest{
			Mode:   "pane",
			Action: "split-right",
		}
		mockSaver.EXPECT().ResetKeybinding(mock.Anything, req).Return(errors.New("reset error"))

		uc := usecase.NewResetKeybindingUseCase(mockSaver)

		// Act
		err := uc.Execute(context.Background(), req)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reset error")
		mock.AssertExpectationsForObjects(t, mockSaver)
	})

	t.Run("returns error when saver is nil", func(t *testing.T) {
		// Arrange
		uc := usecase.NewResetKeybindingUseCase(nil)
		req := port.ResetKeybindingRequest{
			Mode:   "pane",
			Action: "split-right",
		}

		// Act
		err := uc.Execute(context.Background(), req)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "saver is nil")
	})

	t.Run("returns error when mode is empty", func(t *testing.T) {
		// Arrange
		mockSaver := mocks.NewMockKeybindingsSaver(t)
		req := port.ResetKeybindingRequest{
			Mode:   "",
			Action: "split-right",
		}

		uc := usecase.NewResetKeybindingUseCase(mockSaver)

		// Act
		err := uc.Execute(context.Background(), req)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mode is required")
	})
}

func TestResetAllKeybindingsUseCase_Execute(t *testing.T) {
	t.Run("successfully resets all keybindings", func(t *testing.T) {
		// Arrange
		mockSaver := mocks.NewMockKeybindingsSaver(t)
		mockSaver.EXPECT().ResetAllKeybindings(mock.Anything).Return(nil)

		uc := usecase.NewResetAllKeybindingsUseCase(mockSaver)

		// Act
		err := uc.Execute(context.Background())

		// Assert
		require.NoError(t, err)
		mock.AssertExpectationsForObjects(t, mockSaver)
	})

	t.Run("returns error when saver fails", func(t *testing.T) {
		// Arrange
		mockSaver := mocks.NewMockKeybindingsSaver(t)
		mockSaver.EXPECT().ResetAllKeybindings(mock.Anything).Return(errors.New("reset all error"))

		uc := usecase.NewResetAllKeybindingsUseCase(mockSaver)

		// Act
		err := uc.Execute(context.Background())

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reset all error")
		mock.AssertExpectationsForObjects(t, mockSaver)
	})

	t.Run("returns error when saver is nil", func(t *testing.T) {
		// Arrange
		uc := usecase.NewResetAllKeybindingsUseCase(nil)

		// Act
		err := uc.Execute(context.Background())

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "saver is nil")
	})
}
