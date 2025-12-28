package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
)

func TestGetConfigSchemaUseCase_Execute(t *testing.T) {
	t.Run("returns schema keys from provider", func(t *testing.T) {
		// Arrange
		mockProvider := mocks.NewMockConfigSchemaProvider(t)
		expectedKeys := []entity.ConfigKeyInfo{
			{
				Key:         "appearance.color_scheme",
				Type:        "string",
				Default:     "default",
				Description: "Theme preference",
				Values:      []string{"default", "prefer-dark", "prefer-light"},
				Section:     "Appearance",
			},
			{
				Key:         "logging.level",
				Type:        "string",
				Default:     "info",
				Description: "Log verbosity level",
				Values:      []string{"trace", "debug", "info", "warn", "error", "fatal"},
				Section:     "Logging",
			},
		}
		mockProvider.EXPECT().GetSchema().Return(expectedKeys)

		uc := usecase.NewGetConfigSchemaUseCase(mockProvider)

		// Act
		result, err := uc.Execute(context.Background(), usecase.GetConfigSchemaInput{})

		// Assert
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result.Keys, 2)
		assert.Equal(t, "appearance.color_scheme", result.Keys[0].Key)
		assert.Equal(t, "logging.level", result.Keys[1].Key)
		mock.AssertExpectationsForObjects(t, mockProvider)
	})

	t.Run("returns empty slice when no keys", func(t *testing.T) {
		// Arrange
		mockProvider := mocks.NewMockConfigSchemaProvider(t)
		mockProvider.EXPECT().GetSchema().Return([]entity.ConfigKeyInfo{})

		uc := usecase.NewGetConfigSchemaUseCase(mockProvider)

		// Act
		result, err := uc.Execute(context.Background(), usecase.GetConfigSchemaInput{})

		// Assert
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.Keys)
		mock.AssertExpectationsForObjects(t, mockProvider)
	})

	t.Run("keys include all expected fields", func(t *testing.T) {
		// Arrange
		mockProvider := mocks.NewMockConfigSchemaProvider(t)
		expectedKeys := []entity.ConfigKeyInfo{
			{
				Key:         "appearance.default_font_size",
				Type:        "int",
				Default:     "16",
				Description: "Default font size in CSS pixels",
				Range:       "1-72",
				Section:     "Appearance",
			},
		}
		mockProvider.EXPECT().GetSchema().Return(expectedKeys)

		uc := usecase.NewGetConfigSchemaUseCase(mockProvider)

		// Act
		result, err := uc.Execute(context.Background(), usecase.GetConfigSchemaInput{})

		// Assert
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result.Keys, 1)

		key := result.Keys[0]
		assert.Equal(t, "appearance.default_font_size", key.Key)
		assert.Equal(t, "int", key.Type)
		assert.Equal(t, "16", key.Default)
		assert.Equal(t, "Default font size in CSS pixels", key.Description)
		assert.Equal(t, "1-72", key.Range)
		assert.Equal(t, "Appearance", key.Section)
		assert.Empty(t, key.Values) // No enum values for int type
		mock.AssertExpectationsForObjects(t, mockProvider)
	})
}
